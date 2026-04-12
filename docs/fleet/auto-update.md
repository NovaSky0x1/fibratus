# Agent Auto-Update

The Fleet Server supports automatic agent updates, allowing administrators to deploy new agent versions across the fleet without manual intervention on each endpoint.

## Overview

The auto-update system detects new releases on GitHub, notifies agents through the heartbeat protocol, and agents perform a self-update by downloading and installing the new MSI package.

## Update Flow

```
1. Server detects new release on GitHub Releases
2. Admin triggers update (per-agent or account-wide)
                    │
3. Server sets auto-update flag in heartbeat response
                    │
4. Agent receives update signal on next heartbeat (30s)
                    │
5. Agent downloads new MSI from GitHub Releases
                    │
6. Agent force-kills itself
                    │
7. MSI installer runs silently, installs new version
                    │
8. Windows Service auto-restarts with new binary
                    │
9. Agent heartbeats with new version number
```

## Triggering Updates

### Per-Agent Update

From the Agent Detail page:
1. The overview section shows the current agent version
2. If a newer version is available, an update badge appears
3. Click the **Update** button
4. Agent receives the update command on next heartbeat

### Account-Wide Update

From the Super Admin panel:
1. Navigate to account settings
2. Click **Update All Agents**
3. All agents in the account receive the update signal
4. Updates roll out as each agent heartbeats (within 30 seconds)

## Version Detection

The server automatically detects the latest release:
- Queries the GitHub Releases API for the repository
- Compares with each agent's reported version
- No manual version fields needed — fully automatic

## Self-Update Process

The agent self-update happens in the background:

1. **Download**: Agent downloads the new MSI from GitHub Releases
2. **Background context**: Update runs in a separate goroutine to not block the main agent
3. **Force kill**: Agent terminates its own process forcefully
4. **MSI install**: The MSI installer runs silently (`msiexec /i ... /qn`)
5. **Service restart**: Windows Service Manager auto-restarts the service
6. **Re-registration**: Agent heartbeats with the new version number

### Deduplication

The server prevents duplicate auto-update commands:
- Only one update command is pushed per agent per heartbeat
- Prevents repeated update attempts if the agent restarts before completing

## Dashboard Integration

### Agent Overview

The agent detail page shows:
- **Current version** — agent's reported version
- **Version badge** — visual indicator if outdated
- **Update button** — click to trigger update

### Agent List

The agents list page can show version information:
- Version column shows each agent's version
- Easy to identify outdated agents

## Update Safety

### Resilience

- If the update fails, the existing version continues running
- Windows Service Manager's restart policy handles crashes
- Agent re-enrolls automatically after update (enrollment data persists in registry)

### Rollback

If an update causes issues:
- The previous MSI can be manually installed on the endpoint
- Service restart with the previous binary restores functionality
- Enrollment data is preserved across updates (DPAPI-encrypted registry)

## Configuration

No special configuration is needed:
- Auto-update capability is built into the agent
- Server detects latest release from GitHub automatically
- Update commands flow through the existing heartbeat/command infrastructure
