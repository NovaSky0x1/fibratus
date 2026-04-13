# Agent Auto-Update

The Fleet Server supports automatic agent updates, allowing administrators to deploy new agent versions across the fleet without manual intervention on each endpoint.

## Overview

The auto-update system detects new releases on GitHub, notifies agents through the heartbeat protocol, and agents perform a self-update by downloading and installing the new MSI package. The system handles tamper protection automatically — temporarily disabling it for the update, then re-enabling on restart.

## Update Flow

1. **Release checker** polls GitHub `/releases/latest` every 15 minutes, detects new version
2. **Account updated** — `latest_agent_version` and `latest_agent_msi_url` stored per account
3. **Heartbeat triggers check** — on each agent heartbeat, server compares agent version vs latest
4. **Dedup guard** — atomic SQL query checks for existing pending/running/recently-completed update commands (prevents duplicate commands)
5. **Update command queued** — single `update_agent` command created in PostgreSQL
6. **Agent receives command** via gRPC command channel stream
7. **Tamper protection disabled** — if tamper is active, agent disables all 8 protection layers
8. **MSI downloaded** from GitHub Releases to temp directory
9. **Update script spawned** via WMI `Win32_Process.Create` (fully independent process that survives service death)
10. **Script stops service** — `sc.exe stop fibratus`, then force-kill if still running
11. **MSI installed silently** — `msiexec /i ... /quiet /norestart`, with fallback to binary extraction if MSI upgrade fails
12. **Service restarted** — `sc.exe start fibratus`
13. **Agent heartbeats** with new version number, tamper protection re-enables from persisted state

## Triggering Updates

### Automatic (Recommended)

Enable auto-update in Management > Account settings:
- Toggle **Auto-Update Agents** to ON
- Set the GitHub repository (default: `NovaSky0x1/fibratus`)
- The release checker runs every 15 minutes
- When a new release is detected, all agents update on their next heartbeat

### Per-Agent Manual

From the Agent Detail page:
1. The overview section shows the current agent version
2. If a newer version is available, an update badge appears
3. Click the **Update** button
4. Agent receives the update command within seconds

### Account-Wide Manual

From the Management > Account settings:
1. Click **Update All Agents**
2. All agents in the account receive update commands
3. Updates roll out as each agent heartbeats (within 30 seconds)

## Release Detection

The server's release checker:
- Polls `https://api.github.com/repos/{repo}/releases/latest` every 15 minutes
- Extracts version from the release tag (strips leading `v`)
- Finds the `.msi` asset and captures its download URL
- Updates all accounts that use that repository
- Only updates if the version actually changed

The MSI download endpoint (`/api/v1/agent/msi`) dynamically resolves the URL from the account's latest release — no hardcoded URLs.

## Command Deduplication

The server prevents duplicate update commands using an atomic SQL check:

- Before queuing an update, checks: is there a `pending` or `running` update command for this agent?
- Also checks: was an update `completed` within the last hour?
- If any of these are true, the update is skipped
- Uses `context.Background()` for the async goroutine (not the HTTP request context, which would be cancelled after the response)
- Single SQL `COUNT` query — no race condition between concurrent heartbeat goroutines

## Tamper Protection Interaction

When tamper protection is active, the update executor:

1. Checks `protector.IsEnabled()` before starting the update
2. If tamper is on, calls `protector.DisableProtection()` — this removes all 8 protection layers (service DACL, process DACL, install dir lockdown, ARP hiding, registry protection, watchdog, integrity monitor)
3. Records `tamper_was_enabled: true` in the command result for audit
4. Proceeds with the update
5. After the new version starts, tamper protection re-enables automatically from persisted state (the tamper enabled/disabled flag survives across service restarts)

This is safe because the update command is server-initiated (authenticated via gRPC) — the agent never disables tamper protection on its own initiative.

## Update Script Details

The update script is spawned via **WMI `Win32_Process.Create`** to create a fully independent process:
- Survives the death of the fibratus service (unlike child processes which may be killed with the parent)
- Falls back to direct `exec.Command` if WMI is unavailable
- Writes progress to `Logs/update.log` for debugging
- Cleans up the downloaded MSI and the script itself after completion

If the MSI upgrade fails (e.g., product code mismatch from a non-MSI install), the script falls back to:
1. Administrative install (`msiexec /a`) to extract the binary from the MSI
2. Direct file copy of the extracted binary to the install directory

## Dashboard Integration

### Enrollment Tab

Shows the current agent download version with a status banner:
- **Green banner** (Auto-Update ON): "New installs and existing agents will use the latest release automatically"
- **Amber banner** (Auto-Update OFF): "Enable auto-update in Account settings to keep agents current"

### Agent Overview

- **Current version** — agent's reported version from heartbeat
- **Version badge** — visual indicator if outdated compared to latest release
- **Update button** — trigger update for individual agent

## Configuration

| Setting | Location | Description |
|---------|----------|-------------|
| **Auto-Update Agents** | Management > Account | Toggle automatic updates on/off |
| **GitHub Repository** | Management > Account | Source repo for release detection (default: `NovaSky0x1/fibratus`) |
| **Release Check Interval** | Server config | 15 minutes (hardcoded) |
| **Dedup Cooldown** | Server code | 1 hour after last completed update |

## Resilience

- If the MSI install fails, the script falls back to binary extraction
- If the binary extraction fails, the old binary is still in place and the service restarts with it
- Windows Service Manager's recovery policy (restart on failure) ensures the agent comes back
- Enrollment data is preserved across updates (DPAPI-encrypted registry, not affected by MSI)
- Tamper protection re-enables automatically from persisted state after restart
