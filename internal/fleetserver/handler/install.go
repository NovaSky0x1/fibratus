package handler

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	log "github.com/sirupsen/logrus"
)

// InstallHandler serves agent install scripts and binaries.
type InstallHandler struct {
	tokens     store.EnrollmentTokenStore
	accounts   store.AccountStore
	orgs       store.OrgStore
	serverURL  string // External URL (e.g., https://edr.novasky.io)
	binaryPath string // Path to agent EXE on disk
	installDir string // Target dir on endpoints
}

// NewInstallHandler creates a new install handler.
func NewInstallHandler(tokens store.EnrollmentTokenStore, serverURL, binaryPath, installDir string) *InstallHandler {
	return &InstallHandler{
		tokens:     tokens,
		serverURL:  strings.TrimRight(serverURL, "/"),
		binaryPath: binaryPath,
		installDir: installDir,
	}
}

// SetAccountStore sets the account store for dynamic MSI URL resolution.
func (h *InstallHandler) SetAccountStore(s store.AccountStore) { h.accounts = s }

// SetOrgStore sets the org store for resolving token org to account.
func (h *InstallHandler) SetOrgStore(s store.OrgStore) { h.orgs = s }

// Script handles GET /install/{token} — returns a PowerShell install script.
// This is a public endpoint (no auth). The enrollment token IS the auth.
func (h *InstallHandler) Script(w http.ResponseWriter, r *http.Request) {
	// Extract token from path: /install/{token}
	tokenID := strings.TrimPrefix(r.URL.Path, "/install/")
	tokenID = strings.TrimSuffix(tokenID, "/")
	if tokenID == "" {
		http.Error(w, "enrollment token required", http.StatusBadRequest)
		return
	}

	token, err := h.tokens.Get(r.Context(), tokenID)
	if err != nil || token == nil {
		http.Error(w, "invalid enrollment token", http.StatusNotFound)
		return
	}
	if !token.IsValid() {
		http.Error(w, "enrollment token expired or exhausted", http.StatusGone)
		return
	}

	serverURL := h.serverURL

	script := fmt.Sprintf(`# Fibratus EDR Agent Installer
# Organization: %s | Token: %s
# Run this script in an elevated PowerShell session.

$ErrorActionPreference = "Stop"
$serverURL = "%s"
$enrollToken = "%s"
$tempMSI = "$env:TEMP\fibratus.msi"

Write-Host "=== Fibratus EDR Agent Installer ===" -ForegroundColor Cyan
Write-Host "Server: $serverURL"
Write-Host ""

# Check admin
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "ERROR: This script must be run as Administrator." -ForegroundColor Red
    exit 1
}

# Download MSI
Write-Host "[1/3] Downloading MSI installer..." -ForegroundColor Yellow
try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    Invoke-WebRequest -Uri "$serverURL/api/v1/agent/msi?token=$enrollToken" -OutFile $tempMSI -UseBasicParsing
    Write-Host "  Downloaded fibratus.msi ($([math]::Round((Get-Item $tempMSI).Length / 1MB, 1)) MB)" -ForegroundColor Green
} catch {
    Write-Host "ERROR: Failed to download MSI: $_" -ForegroundColor Red
    exit 1
}

# Install MSI with enrollment parameters
Write-Host "[2/3] Installing Fibratus (this may take a moment)..." -ForegroundColor Yellow
$msiArgs = "/i ""$tempMSI"" /qn ENROLLMENT_TOKEN=$enrollToken SERVER_URL=$serverURL /l*! ""$env:TEMP\fibratus-install.log"""
$proc = Start-Process msiexec -ArgumentList $msiArgs -Wait -PassThru
if ($proc.ExitCode -ne 0) {
    Write-Host "MSI install failed (exit $($proc.ExitCode)). Log: $env:TEMP\fibratus-install.log" -ForegroundColor Red
    exit 1
}
Write-Host "  MSI installation complete" -ForegroundColor Green

# Enroll agent. Drop ErrorActionPreference to Continue around the native call:
# fibratus writes info-level logs to stderr (logrus default), and PowerShell
# under $ErrorActionPreference=Stop treats any stderr line from a native
# command as a terminating NativeCommandError — which would abort the script
# even on a successful exit-0 enrollment.
Write-Host "[3/5] Enrolling agent..." -ForegroundColor Yellow
Stop-Service fibratus -ErrorAction SilentlyContinue
$prevAction = $ErrorActionPreference
$ErrorActionPreference = "Continue"
try {
    $enrollOut = & "C:\Program Files\Fibratus\Bin\fibratus.exe" enroll --token $enrollToken --server $serverURL --insecure 2>&1
    $enrollExit = $LASTEXITCODE
} finally {
    $ErrorActionPreference = $prevAction
}
$enrollOut | ForEach-Object { Write-Host "  $_" }
if ($enrollExit -ne 0) {
    Write-Host "  Enrollment failed (exit $enrollExit)" -ForegroundColor Red
} else {
    Write-Host "  Enrollment complete" -ForegroundColor Green
}

# Start service. Capture the actual start error if it fails so the operator
# isn't left with "Service status: Stopped" and no clue why.
Write-Host "[4/5] Starting service..." -ForegroundColor Yellow
try {
    Start-Service fibratus -ErrorAction Stop
} catch {
    Write-Host "  Start-Service raised: $_" -ForegroundColor Red
}
Start-Sleep -Seconds 3

# Verify
Write-Host "[5/5] Verifying..." -ForegroundColor Yellow
$svc = Get-Service fibratus -ErrorAction SilentlyContinue
if ($svc -and $svc.Status -eq "Running") {
    Write-Host "  Service RUNNING" -ForegroundColor Green
} elseif ($svc) {
    Write-Host "  Service status: $($svc.Status)" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "  Diagnostic: sc.exe query fibratus" -ForegroundColor DarkGray
    sc.exe query fibratus | ForEach-Object { Write-Host "    $_" -ForegroundColor DarkGray }
    foreach ($logPath in @(
        "C:\Program Files\Fibratus\fibratus.log",
        "C:\ProgramData\Fibratus\fibratus.log",
        "C:\Program Files\Fibratus\Bin\fibratus.log"
    )) {
        if (Test-Path $logPath) {
            Write-Host ""
            Write-Host "  Tail of $logPath" -ForegroundColor DarkGray
            Get-Content $logPath -Tail 20 -ErrorAction SilentlyContinue | ForEach-Object { Write-Host "    $_" -ForegroundColor DarkGray }
        }
    }
} else {
    Write-Host "  Warning: Service not found" -ForegroundColor Yellow
}

if (Test-Path "C:\Program Files\Fibratus\data\agent-id") {
    Write-Host "  Agent ID: $(Get-Content 'C:\Program Files\Fibratus\data\agent-id')" -ForegroundColor Green
}

# Cleanup
Remove-Item $tempMSI -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "=== Installation Complete ===" -ForegroundColor Cyan
Write-Host "Server: $serverURL"
Write-Host ""
Write-Host "To check status:  sc.exe query fibratus"
`, token.OrgName, tokenID, serverURL, tokenID)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "inline")
	w.Write([]byte(script))

	log.Infof("fleet: install script served for token %s (org: %s)", tokenID, token.OrgID)
}

// Binary handles GET /api/v1/agent/binary — serves the agent EXE.
func (h *InstallHandler) Binary(w http.ResponseWriter, r *http.Request) {
	if h.binaryPath == "" {
		writeError(w, http.StatusNotFound, "agent binary not configured on server")
		return
	}

	f, err := os.Open(h.binaryPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent binary not found")
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read binary")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=fibratus.exe")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	http.ServeContent(w, r, "fibratus.exe", stat.ModTime(), f)
}

// MSI handles GET /api/v1/agent/msi — redirects to the latest GitHub release MSI.
// The URL is resolved dynamically from account settings (set by the release checker).
// If a ?token= query param is provided, the URL is resolved from that token's account.
// Otherwise, it uses the first account with a configured MSI URL.
func (h *InstallHandler) MSI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var msiURL string

	// Try to resolve from enrollment token → org → account
	if tokenID := r.URL.Query().Get("token"); tokenID != "" && h.tokens != nil && h.orgs != nil && h.accounts != nil {
		if token, err := h.tokens.Get(ctx, tokenID); err == nil && token != nil {
			if org, err := h.orgs.Get(ctx, token.OrgID); err == nil && org != nil {
				if acct, err := h.accounts.Get(ctx, org.AccountID); err == nil && acct != nil && acct.LatestAgentMSIURL != "" {
					msiURL = acct.LatestAgentMSIURL
				}
			}
		}
	}

	// Fall back to any account with a configured MSI URL
	if msiURL == "" && h.accounts != nil {
		if accounts, err := h.accounts.ListAll(ctx); err == nil {
			for _, acct := range accounts {
				if acct.LatestAgentMSIURL != "" {
					msiURL = acct.LatestAgentMSIURL
					break
				}
			}
		}
	}

	if msiURL == "" {
		writeError(w, http.StatusServiceUnavailable, "no agent MSI release configured — check Management > Account > Agent Updates")
		return
	}

	http.Redirect(w, r, msiURL, http.StatusFound)
}

// Config handles GET /api/v1/agent/config — serves the default agent config.
func (h *InstallHandler) Config(w http.ResponseWriter, r *http.Request) {
	// Try common config paths
	paths := []string{
		"configs/fibratus.yml",
		"/opt/fibratus-fleet/src/configs/fibratus.yml",
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err == nil {
			w.Header().Set("Content-Type", "text/yaml")
			w.Write(data)
			return
		}
	}
	writeError(w, http.StatusNotFound, "default config not available")
}
