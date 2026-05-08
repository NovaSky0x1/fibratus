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
	serverURL  string // External URL (e.g., https://fleet.example.com)
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

# CRITICAL: stop the service and wipe stale enrollment registry BEFORE enroll.
#
# Without this guard the install races itself on any host that previously had
# an agent: the WiX manifest historically had Start="install" so msiexec
# auto-started the service the moment install finished. The service then
# loaded the old (decommissioned) DPAPI enrollment from HKLM\SOFTWARE\Fibratus,
# called the server, was told it was decommissioned, and self-uninstalled —
# all in the seconds before the enroll step below could write fresh creds.
#
# The MSI itself is fixed (Start="install" removed in fibratus.wxs) but we
# keep this belt-and-suspenders so a host running an older MSI still recovers.
$svcRunning = (Get-Service fibratus -ErrorAction SilentlyContinue)
if ($svcRunning) {
    sc.exe config fibratus start= demand 2>&1 | Out-Null
    if ($svcRunning.Status -ne 'Stopped') {
        $stopJob = Start-Job -ScriptBlock { Stop-Service fibratus -Force -ErrorAction SilentlyContinue }
        $finished = Wait-Job $stopJob -Timeout 10
        Remove-Job $stopJob -Force -ErrorAction SilentlyContinue
        if (-not $finished) {
            Write-Host "  Service did not stop in 10s — force-killing the process." -ForegroundColor DarkYellow
            Get-Process fibratus -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
            sc.exe stop fibratus 2>&1 | Out-Null
            Start-Sleep -Seconds 2
        }
    }
    # Wait for the process to actually exit so the registry isn't read again
    # between our clear and the enroll write. ForEach-Object's `return` only
    # skips one iteration, not the whole pipeline — use a for/break loop.
    for ($i = 0; $i -lt 20; $i++) {
        if (-not (Get-Process fibratus -ErrorAction SilentlyContinue)) { break }
        Start-Sleep -Milliseconds 500
    }
    Get-Process fibratus -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
}

# Wipe any leftover enrollment data from a previous install on this host.
# ProtectRegistryKeys() locks the keys to SYSTEM-only; the elevated MSI
# context can take ownership and delete them.
takeown /F "HKLM\SOFTWARE\Fibratus" /R 2>&1 | Out-Null
icacls "HKLM\SOFTWARE\Fibratus" /grant "Administrators:F" /T 2>&1 | Out-Null
Remove-Item HKLM:\SOFTWARE\Fibratus\Enrollment -Recurse -Force -ErrorAction SilentlyContinue
Remove-Item HKLM:\SOFTWARE\Fibratus\State -Recurse -Force -ErrorAction SilentlyContinue

# Enroll agent. Drop ErrorActionPreference to Continue around the native call:
# fibratus writes info-level logs to stderr (logrus default), and PowerShell
# under $ErrorActionPreference=Stop treats any stderr line from a native
# command as a terminating NativeCommandError — which would abort the script
# even on a successful exit-0 enrollment.
Write-Host "[3/5] Enrolling agent..." -ForegroundColor Yellow
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
# We forced start type to demand earlier to win the enroll-vs-start race.
# Restore it to auto so the service boots with the host on subsequent reboots.
sc.exe config fibratus start= auto 2>&1 | Out-Null
try {
    Start-Service fibratus -ErrorAction Stop
} catch {
    Write-Host "  Start-Service raised: $_" -ForegroundColor Red
}

# Poll for Running state. The agent does heavy bootstrap work (ETW provider
# registration, symbolizer init, fleet client connect) before reporting
# Running, so a single 3-second sleep is unreliable on slower hosts.
Write-Host "[5/5] Verifying..." -ForegroundColor Yellow
$svc = $null
$timeout = (Get-Date).AddSeconds(30)
$lastStatus = ""
while ((Get-Date) -lt $timeout) {
    $svc = Get-Service fibratus -ErrorAction SilentlyContinue
    if (-not $svc) { break }
    if ($svc.Status -eq "Running") { break }
    if ($svc.Status -eq "Stopped" -and $lastStatus -eq "Stopped") { break } # crashed early
    $lastStatus = $svc.Status
    Start-Sleep -Milliseconds 800
}

if ($svc -and $svc.Status -eq "Running") {
    Write-Host "  Service RUNNING" -ForegroundColor Green
    Write-Host ""
    Write-Host "=== Installation Complete ===" -ForegroundColor Cyan
    Write-Host "Server:      $serverURL"
    Write-Host "Agent log:   C:\Program Files\Fibratus\Logs\fibratus.log"
    Write-Host "Service ctl: sc.exe query fibratus  |  Stop-Service fibratus"
    Write-Host ""
    Write-Host "The agent will appear on the dashboard's Agents page within ~30s." -ForegroundColor Green
    exit 0
}

# Service did not reach Running — dump every diagnostic we can.
if ($svc) {
    Write-Host "  Service status: $($svc.Status) (did not reach Running within 30s)" -ForegroundColor Red
} else {
    Write-Host "  Warning: Service not found" -ForegroundColor Red
}
Write-Host ""
Write-Host "  --- sc.exe query fibratus ---" -ForegroundColor DarkGray
sc.exe query fibratus 2>&1 | ForEach-Object { Write-Host "    $_" -ForegroundColor DarkGray }
Write-Host ""
Write-Host "  --- sc.exe qc fibratus ---" -ForegroundColor DarkGray
sc.exe qc fibratus 2>&1 | ForEach-Object { Write-Host "    $_" -ForegroundColor DarkGray }

# Tail the agent log from every location we know about.
$logFound = $false
foreach ($logPath in @(
    "C:\Program Files\Fibratus\Logs\fibratus.log",
    "C:\Program Files\Fibratus\fibratus.log",
    "C:\ProgramData\Fibratus\Logs\fibratus.log",
    "C:\ProgramData\Fibratus\fibratus.log",
    "C:\Program Files\Fibratus\Bin\fibratus.log"
)) {
    if (Test-Path $logPath) {
        $logFound = $true
        Write-Host ""
        Write-Host "  --- Tail of $logPath ---" -ForegroundColor DarkGray
        Get-Content $logPath -Tail 30 -ErrorAction SilentlyContinue | ForEach-Object { Write-Host "    $_" -ForegroundColor DarkGray }
    }
}
if (-not $logFound) {
    Write-Host ""
    Write-Host "  No agent log file found — the binary likely crashed before logging was initialised." -ForegroundColor Yellow
}

# Recent SCM events for fibratus.
Write-Host ""
Write-Host "  --- Recent Service Control Manager events (fibratus) ---" -ForegroundColor DarkGray
try {
    Get-WinEvent -LogName System -MaxEvents 50 -ErrorAction Stop |
        Where-Object { $_.Message -match 'Fibratus' -or $_.Message -match 'fibratus' } |
        Select-Object -First 5 |
        ForEach-Object {
            Write-Host "    $($_.TimeCreated) Id=$($_.Id) $($_.LevelDisplayName)" -ForegroundColor DarkGray
            Write-Host "      $($_.Message.Split([Environment]::NewLine)[0])" -ForegroundColor DarkGray
        }
} catch {
    Write-Host "    (could not read System log: $_)" -ForegroundColor DarkGray
}

Write-Host ""
Write-Host "Installation completed but the service is not running." -ForegroundColor Red
Write-Host "Share the diagnostic block above to investigate further." -ForegroundColor Red
exit 1
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
