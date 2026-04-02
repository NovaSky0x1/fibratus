# Fibratus Fleet — API Test Script
# Usage: powershell -ExecutionPolicy Bypass -File test-api.ps1 -Server <url> -Password <pass>

param(
    [Parameter(Mandatory=$true)][string]$Server,
    [Parameter(Mandatory=$true)][string]$Password,
    [string]$Email = "admin@fibratus.local"
)

Write-Host "Fibratus Fleet — API Test" -ForegroundColor Cyan
Write-Host ""

# Test 1: Health check
Write-Host "  [1/3] Health check..." -ForegroundColor Gray -NoNewline
try {
    $health = Invoke-RestMethod -Uri "$Server/health"
    Write-Host " $($health.status)" -ForegroundColor Green
} catch {
    Write-Host " FAILED" -ForegroundColor Red
    Write-Host "        $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

# Test 2: Login
Write-Host "  [2/3] Login..." -ForegroundColor Gray -NoNewline
$loginBody = @{email=$Email; password=$Password} | ConvertTo-Json
try {
    $login = Invoke-RestMethod -Uri "$Server/api/v1/auth/login" -Method POST -Body $loginBody -ContentType "application/json"
    $token = $login.data.token
    Write-Host " OK (JWT received)" -ForegroundColor Green
} catch {
    Write-Host " FAILED" -ForegroundColor Red
    Write-Host "        $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

# Test 3: Dashboard overview
Write-Host "  [3/3] Dashboard API..." -ForegroundColor Gray -NoNewline
$headers = @{Authorization = "Bearer $token"}

# First get orgs to find the org ID
try {
    $orgs = Invoke-RestMethod -Uri "$Server/api/v1/account/organizations" -Headers $headers
    $orgId = $orgs.data[0].id
    $orgName = $orgs.data[0].name
    $overview = Invoke-RestMethod -Uri "$Server/api/v1/orgs/$orgId/dashboard/overview" -Headers $headers
    $d = $overview.data
    Write-Host " OK" -ForegroundColor Green
} catch {
    Write-Host " FAILED" -ForegroundColor Red
    Write-Host "        $($_.Exception.Message)" -ForegroundColor Red
    if ($_.ErrorDetails) { Write-Host "        $($_.ErrorDetails.Message)" -ForegroundColor Red }
    exit 1
}

Write-Host ""
Write-Host "  All tests passed!" -ForegroundColor Green
Write-Host ""
Write-Host "  Organization:    $orgName ($orgId)"
Write-Host "  Total agents:    $($d.total_agents)"
Write-Host "  Online agents:   $($d.online_agents)"
Write-Host "  Detections 24h:  $($d.total_detections_24h)"
Write-Host ""
