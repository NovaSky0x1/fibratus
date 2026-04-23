# Agent Enrollment

Enrollment is the process by which a Fibratus agent on a Windows endpoint registers with the Fleet Server. After enrollment, the agent requires zero configuration — it automatically connects, streams telemetry, receives rules, and responds to commands.

## Enrollment Flow

1. **Admin creates enrollment token** in the dashboard (Management > Enrollment > Create Token)
2. **Run enrollment command** on the endpoint: `fibratus enroll --token <TOKEN> --server https://server`
3. **Agent generates RSA key pair + CSR** — private key stays on endpoint, CSR sent to server
4. **Server validates token, signs CSR** with the organization's CA — returns signed cert, agent-id, org-id, server URL
5. **Agent stores enrollment data** in DPAPI-encrypted Windows registry (`HKLM\SOFTWARE\Fibratus\Enrollment`)
6. **On next service start**, agent auto-detects enrollment — zero config needed, reads from registry, connects to server

## Creating Enrollment Tokens

Enrollment tokens are created by administrators through the dashboard:

1. Navigate to **Management** → **Enrollment** tab
2. Click **Create Token**
3. The token is displayed once — copy it securely
4. Tokens are scoped to a specific organization
5. Tokens can be revoked at any time

Tokens can also be created programmatically via the API:

```bash
curl -X POST https://server/api/v1/orgs/{orgId}/enrollment-tokens \
  -H "Authorization: Bearer <JWT>" \
  -H "Content-Type: application/json"
```

## Enrolling an Agent

### Using the Install Script

The recommended method is the PowerShell one-liner displayed in the dashboard:

```powershell
# Run in an elevated PowerShell window.
# Downloads the install script from the server, which in turn
# downloads the MSI from GitHub Releases, installs, and enrolls.
irm https://server/install/<ENROLLMENT_TOKEN> | iex
```

### Manual Enrollment

If the agent is already installed via MSI:

```powershell
# Run enrollment (as Administrator)
& "C:\Program Files\Fibratus\fibratus.exe" enroll --token <TOKEN> --server https://server
```

### MSI with Enrollment Parameters

The MSI installer supports enrollment parameters for automated deployment:

```powershell
msiexec /i fibratus-3.0.0-amd64.msi /qn ENROLLMENT_TOKEN=<TOKEN> FLEET_SERVER=https://server
```

## Credential Storage

### DPAPI-Encrypted Registry

Enrollment credentials are stored in the Windows registry, encrypted with DPAPI (Data Protection API):

```
HKLM\SOFTWARE\Fibratus\Fleet
├── AgentID          (DPAPI-encrypted)
├── OrgID            (DPAPI-encrypted)
├── ServerURL        (DPAPI-encrypted)
├── Certificate      (DPAPI-encrypted)
├── PrivateKey       (DPAPI-encrypted)
└── CACertificate    (DPAPI-encrypted)
```

DPAPI encryption ensures that:
- Credentials are bound to the machine — cannot be copied to another host
- Only the SYSTEM account (which runs the service) can decrypt them
- No plaintext credentials exist on disk

### Auto-Detection

When the Fibratus service starts, it checks for enrollment data in the registry:

1. If enrollment data exists → fleet mode is enabled automatically
2. Fleet client initializes: registration, heartbeat, rule sync, command polling, telemetry
3. No YAML configuration changes needed — everything is derived from enrollment data

## Per-Organization Certificate Authority

Each organization in the Fleet Server has its own Certificate Authority (CA):

- **CA generation**: When an organization is created, a unique RSA CA key pair is generated
- **CSR signing**: During enrollment, the agent's CSR is signed by the organization's CA
- **mTLS**: The signed certificate can be used for mutual TLS authentication
- **Isolation**: Certificates from one organization cannot authenticate to another

## Post-Enrollment Agent Behavior

Once enrolled, the agent automatically:

| Activity | Interval | Description |
|----------|----------|-------------|
| **Registration** | Once | Registers with server, receives agent record |
| **Heartbeat** | 30 seconds | Reports status, OS info, version; receives policy updates |
| **Rule Sync** | 5 minutes | Checks for rule updates using ETag caching |
| **Command Polling** | 5 seconds | Checks for pending active response commands |
| **Telemetry** | Continuous | Streams security-relevant kernel events |
| **Detection Forwarding** | On detection | Forwards rule match alerts to the server |

## Troubleshooting Enrollment

### Common Issues

**"Enrollment failed" error:**
- Verify the enrollment token hasn't been revoked
- Ensure the server URL is reachable from the endpoint
- Check that port 443 is not blocked by firewall

**Agent not appearing in dashboard:**
- The agent registers on the next service start after enrollment
- Check agent logs: `C:\Program Files\Fibratus\Logs\fibratus.log`
- Verify the Windows service is running: `sc query fibratus`

**Certificate errors:**
- The server uses Let's Encrypt certificates trusted by the Windows system CA store
- If using self-signed certificates, ensure the CA is added to the endpoint's trusted roots
