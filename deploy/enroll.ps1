# Fibratus Fleet — Agent Enrollment Script
# Usage: powershell -ExecutionPolicy Bypass -File enroll.ps1 -Token <token> -Server <url>

param(
    [Parameter(Mandatory=$true)][string]$Token,
    [Parameter(Mandatory=$true)][string]$Server
)

Write-Host "Fibratus Fleet — Enrolling agent..." -ForegroundColor Cyan
Write-Host "  Server:   $Server"
Write-Host "  Hostname: $env:COMPUTERNAME"
Write-Host ""

# Generate RSA key pair and CSR
Write-Host "  Generating key pair..." -ForegroundColor Gray
$rsa = [System.Security.Cryptography.RSA]::Create(2048)
$sub = "CN=$env:COMPUTERNAME"
$hash = [System.Security.Cryptography.HashAlgorithmName]::SHA256
$pad = [System.Security.Cryptography.RSASignaturePadding]::Pkcs1
$req = New-Object System.Security.Cryptography.X509Certificates.CertificateRequest($sub, $rsa, $hash, $pad)
$csrBytes = $req.CreateSigningRequest()
$csrB64 = [Convert]::ToBase64String($csrBytes, [Base64FormattingOptions]::InsertLineBreaks)
$csrPem = "-----BEGIN CERTIFICATE REQUEST-----`n$csrB64`n-----END CERTIFICATE REQUEST-----"

# Get OS version
$os = (Get-CimInstance Win32_OperatingSystem)
$osVer = "Windows $($os.Version) ($($os.Caption))"

# Send enrollment request
Write-Host "  Sending enrollment request..." -ForegroundColor Gray
$body = @{
    token = $Token
    hostname = $env:COMPUTERNAME
    os_version = $osVer
    engine_version = "dev"
    csr = $csrPem
} | ConvertTo-Json

try {
    $result = Invoke-RestMethod -Uri "$Server/api/v1/enroll" -Method POST -Body $body -ContentType "application/json"
} catch {
    Write-Host "  ERROR: Enrollment failed" -ForegroundColor Red
    Write-Host "  $($_.Exception.Message)" -ForegroundColor Red
    if ($_.ErrorDetails) { Write-Host "  $($_.ErrorDetails.Message)" -ForegroundColor Red }
    exit 1
}

$data = $result.data

# Save certificates to disk
$dataDir = "$env:ProgramData\Fibratus\fleet"
$certDir = "$dataDir\certs"
New-Item -ItemType Directory -Path $certDir -Force | Out-Null

# Export private key as PEM
$keyBytes = $rsa.ExportRSAPrivateKey()
$keyB64 = [Convert]::ToBase64String($keyBytes, [Base64FormattingOptions]::InsertLineBreaks)
$keyPem = "-----BEGIN RSA PRIVATE KEY-----`n$keyB64`n-----END RSA PRIVATE KEY-----"
Set-Content -Path "$certDir\agent.key" -Value $keyPem -NoNewline
Set-Content -Path "$certDir\agent.crt" -Value $data.signed_cert -NoNewline
Set-Content -Path "$certDir\ca.crt" -Value $data.ca_cert -NoNewline
Set-Content -Path "$dataDir\agent-id" -Value $data.agent_id -NoNewline
Set-Content -Path "$dataDir\org-id" -Value $data.org_id -NoNewline

Write-Host ""
Write-Host "  Enrollment successful!" -ForegroundColor Green
Write-Host ""
Write-Host "  Agent ID:     $($data.agent_id)"
Write-Host "  Organization: $($data.org_id)"
Write-Host "  Certificates: $certDir"
Write-Host ""
Write-Host "  Add to fibratus.yml:" -ForegroundColor Yellow
Write-Host ""
Write-Host "    fleet:"
Write-Host "      enabled: true"
Write-Host "      server-url: `"$Server`""
Write-Host "      org-id: `"$($data.org_id)`""
Write-Host "      agent-group: `"default`""
Write-Host ""
