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
    Invoke-WebRequest -Uri "$serverURL/api/v1/agent/msi" -OutFile $tempMSI -UseBasicParsing
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

# Enroll agent
Write-Host "[3/5] Enrolling agent..." -ForegroundColor Yellow
Stop-Service fibratus -ErrorAction SilentlyContinue
$enrollOut = & "C:\Program Files\Fibratus\Bin\fibratus.exe" enroll --token $enrollToken --server $serverURL --insecure 2>&1
$enrollOut | ForEach-Object { Write-Host "  $_" }
if ($LASTEXITCODE -ne 0) {
    Write-Host "  Enrollment failed (exit $LASTEXITCODE)" -ForegroundColor Red
} else {
    Write-Host "  Enrollment complete" -ForegroundColor Green
}

# Start service
Write-Host "[4/5] Starting service..." -ForegroundColor Yellow
Start-Service fibratus -ErrorAction SilentlyContinue
Start-Sleep -Seconds 3

# Verify
Write-Host "[5/5] Verifying..." -ForegroundColor Yellow
$svc = Get-Service fibratus -ErrorAction SilentlyContinue
if ($svc -and $svc.Status -eq "Running") {
    Write-Host "  Service RUNNING" -ForegroundColor Green
} elseif ($svc) {
    Write-Host "  Service status: $($svc.Status)" -ForegroundColor Yellow
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

// MSI handles GET /api/v1/agent/msi — redirects to the GitHub release MSI.
func (h *InstallHandler) MSI(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "https://github.com/NovaSky0x1/fibratus/releases/download/v3.0.0-rc4/fibratus-3.0.0-rc4-slim-amd64.msi", http.StatusFound)
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
