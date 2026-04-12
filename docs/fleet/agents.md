# Agent Management

The Agents page provides real-time visibility into all enrolled endpoints, their status, and deep inspection capabilities.

## Agent Lifecycle

```
Enrollment → Registration → Online (heartbeating) → Offline (missed heartbeats)
                                  │
                                  ├── Isolated (network isolation active)
                                  ├── Updating (self-update in progress)
                                  └── Uninstalled (remote uninstall)
```

## Agent Status

| Status | Description |
|--------|-------------|
| **Online** | Agent is actively heartbeating (last heartbeat within 60 seconds) |
| **Offline** | Agent has missed heartbeats (no heartbeat for >60 seconds) |
| **Isolated** | Agent is online but network-isolated via WFP (only server communication allowed) |
| **Updating** | Agent is performing a self-update |

## Agent List

The Agents page displays all enrolled agents with:

- **Hostname** — the endpoint's computer name
- **Status** — online/offline badge with color coding
- **OS** — Windows version and build number
- **IP Address** — primary network interface IP
- **Agent Version** — installed Fibratus version
- **Last Heartbeat** — relative timestamp of last heartbeat
- **Organization** — the org the agent belongs to (visible in cross-org view)

Features:
- **Search** — filter agents by hostname, IP, or OS
- **Sorting** — click column headers to sort
- **Delete** — remove agent from fleet (triggers auto-uninstall on next connection)

## Agent Detail Page

Clicking an agent opens the detail page with comprehensive inspection tabs organized into sections:

### Overview Section

The default view showing:
- **System Information** — hostname, OS version, architecture, uptime
- **Agent Version** — with update badge if newer version available
- **Tamper Protection Status** — enabled/disabled indicator
- **Network Isolation Status** — isolated/not isolated indicator
- **IP Addresses** — all network interfaces
- **Last Heartbeat** — precise timestamp

### Respond Section

Active response capabilities organized into tabs:

#### Processes
Lists all running processes on the endpoint with:
- Process name, PID, parent PID
- Memory usage
- CPU time
- Path to executable
- **Kill** action button for individual processes

#### File Browser
Interactive file system navigator:
- Browse any directory on the endpoint
- File size, modification date, attributes
- Download individual files
- Navigate up/down the directory tree

#### Terminal
Full remote command shell:
- Execute arbitrary PowerShell commands
- Real-time output display
- Command history

#### Network
Network adapter information:
- Adapter name and type
- IP addresses (IPv4/IPv6)
- MAC address
- Link speed
- Connection status

#### Services
Windows services enumeration:
- Service name and display name
- Status (running/stopped)
- Start type (automatic/manual/disabled)
- Service account

#### Drivers
Loaded kernel drivers:
- Driver name and path
- Signature status
- Known vulnerable driver (LOLDriver) detection

#### Software
Installed software inventory:
- Application name and version
- Publisher
- Install date

#### Users
Local user accounts:
- Username
- Group membership
- Account status (active/disabled)
- Last login

#### Autoruns
Persistence mechanisms:
- Registry run keys
- Scheduled tasks
- Startup folder entries
- Service entries

### Detections Tab
Agent-specific detections:
- Detection rule name and severity
- Timestamp
- Alert text with template substitution
- Link to full detection detail

### Live Events Tab
Real-time kernel events from this specific agent:
- Filterable by event type
- Expandable event detail
- Process context for each event

### Captures Tab
Live kernel event capture interface:
- Start/stop capture with duration or manual control
- Event type filtering
- Quick filter presets
- Real Fibratus QL filter engine
- Export to JSON, CSV, or .kcap format
- Terminal-style event output with color coding

### Event Log Tab
Windows Event Log browser:
- Browse Security, System, and Sysmon channels
- Event ID, level, source, message
- Enable/disable per-agent event log collection

### Command History Tab
Complete command history for this agent:
- Command type and parameters
- Execution status (pending/running/completed/failed)
- User who issued the command
- Timestamps and results

## Heartbeat Protocol

The agent sends heartbeats every 30 seconds via gRPC containing:

```protobuf
message HeartbeatRequest {
  string agent_id = 1;
  string hostname = 2;
  string os_version = 3;
  string agent_version = 4;
  string primary_ip = 5;
  bool tamper_protected = 6;
  bool isolated = 7;
}
```

The server responds with policy updates:

```protobuf
message HeartbeatResponse {
  bool tamper_protection = 1;
  bool check_auto_update = 2;
  repeated string eventlog_channels = 3;
  // ... other policy fields
}
```

## Agent Deletion

Deleting an agent from the fleet:

1. Click the delete button on the Agents list page
2. Confirm the deletion
3. Agent record is removed from the database
4. If the agent reconnects, it receives an uninstall command
5. The agent performs MSI uninstall and removes itself

## Cross-Organization View

When the "All Organizations" option is selected in the org switcher:
- Agents from all organizations are aggregated
- An "Organization" column appears in the table
- All agent detail features work across organizations
- Useful for root users managing multiple tenants
