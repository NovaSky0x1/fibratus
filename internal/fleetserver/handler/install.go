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
	installDir := h.installDir

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
$msiArgs = "/i `"$tempMSI`" /qn ENROLLMENT_TOKEN=$enrollToken SERVER_URL=$serverURL /l*! `"$env:TEMP\fibratus-install.log`""
$proc = Start-Process msiexec -ArgumentList $msiArgs -Wait -PassThru
if ($proc.ExitCode -ne 0) {
    Write-Host "MSI install exited with code $($proc.ExitCode). Check $env:TEMP\fibratus-install.log" -ForegroundColor Red
    exit 1
}
Write-Host "  MSI installation complete" -ForegroundColor Green

# Verify
Write-Host "[3/3] Verifying..." -ForegroundColor Yellow
Start-Sleep -Seconds 3
$svc = Get-Service fibratus -ErrorAction SilentlyContinue
if ($svc) {
    Write-Host "  Service status: $($svc.Status)" -ForegroundColor Green
} else {
    Write-Host "  Warning: Service not found" -ForegroundColor Yellow
}

# Cleanup
Remove-Item $tempMSI -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "=== Installation Complete ===" -ForegroundColor Cyan
Write-Host "Server: $serverURL"
Write-Host ""
Write-Host "To check status:  sc.exe query fibratus"
Write-Host "To check enrollment: type `"C:\Program Files\Fibratus\data\agent-id`""
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

// MSI handles GET /api/v1/agent/msi — serves the agent MSI installer.
func (h *InstallHandler) MSI(w http.ResponseWriter, r *http.Request) {
	paths := []string{
		"/opt/fibratus-fleet/downloads/fibratus-1.0.0-slim-amd64.msi",
		"build/msi/fibratus-1.0.0-slim-amd64.msi",
	}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		defer f.Close()
		stat, _ := f.Stat()
		w.Header().Set("Content-Type", "application/x-msi")
		w.Header().Set("Content-Disposition", "attachment; filename=fibratus.msi")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
		http.ServeContent(w, r, "fibratus.msi", stat.ModTime(), f)
		return
	}
	writeError(w, http.StatusNotFound, "agent MSI not available")
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
