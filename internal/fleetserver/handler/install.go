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
$installDir = "%s"
$binDir = "$installDir\Bin"
$dataDir = "$installDir\data"
$configDir = "$installDir\Config"
$serverURL = "%s"
$enrollToken = "%s"

Write-Host "=== Fibratus EDR Agent Installer ===" -ForegroundColor Cyan
Write-Host "Server:  $serverURL"
Write-Host "Install: $installDir"
Write-Host ""

# Check admin
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "ERROR: This script must be run as Administrator." -ForegroundColor Red
    exit 1
}

# Create directories
Write-Host "[1/5] Creating directories..." -ForegroundColor Yellow
New-Item -ItemType Directory -Force -Path $binDir | Out-Null
New-Item -ItemType Directory -Force -Path $dataDir | Out-Null
New-Item -ItemType Directory -Force -Path $configDir | Out-Null

# Download agent binary
Write-Host "[2/5] Downloading agent binary..." -ForegroundColor Yellow
$binURL = "$serverURL/api/v1/agent/binary"
try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    Invoke-WebRequest -Uri $binURL -OutFile "$binDir\fibratus.exe" -UseBasicParsing
    Write-Host "  Downloaded fibratus.exe" -ForegroundColor Green
} catch {
    Write-Host "ERROR: Failed to download agent binary: $_" -ForegroundColor Red
    exit 1
}

# Download default config if not present
if (-not (Test-Path "$configDir\fibratus.yml")) {
    Write-Host "  Downloading default config..." -ForegroundColor Yellow
    try {
        Invoke-WebRequest -Uri "$serverURL/api/v1/agent/config" -OutFile "$configDir\fibratus.yml" -UseBasicParsing
    } catch {
        Write-Host "  Warning: Could not download config, using defaults." -ForegroundColor Yellow
    }
}

# Enroll agent
Write-Host "[3/5] Enrolling agent..." -ForegroundColor Yellow
try {
    & "$binDir\fibratus.exe" enroll --token $enrollToken --server $serverURL 2>&1
    Write-Host "  Enrollment successful" -ForegroundColor Green
} catch {
    Write-Host "ERROR: Enrollment failed: $_" -ForegroundColor Red
    exit 1
}

# Install Windows service
Write-Host "[4/5] Installing Windows service..." -ForegroundColor Yellow
try {
    & "$binDir\fibratus.exe" service install 2>&1
    Write-Host "  Service installed" -ForegroundColor Green
} catch {
    Write-Host "  Service may already exist, continuing..." -ForegroundColor Yellow
}

# Start service
Write-Host "[5/5] Starting service..." -ForegroundColor Yellow
try {
    Start-Service fibratus -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
    $svc = Get-Service fibratus -ErrorAction SilentlyContinue
    if ($svc.Status -eq "Running") {
        Write-Host "  Service running (PID: $((Get-Process fibratus -ErrorAction SilentlyContinue).Id))" -ForegroundColor Green
    } else {
        Write-Host "  Service status: $($svc.Status)" -ForegroundColor Yellow
    }
} catch {
    Write-Host "  Warning: Could not start service: $_" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "=== Installation Complete ===" -ForegroundColor Cyan
Write-Host "Agent installed at: $installDir"
Write-Host "Server: $serverURL"
Write-Host ""
Write-Host "To check status:  sc.exe query fibratus"
Write-Host "To view logs:     Get-Content '$installDir\Logs\fibratus.log' -Tail 20"
`, token.OrgName, tokenID, installDir, serverURL, tokenID)

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
