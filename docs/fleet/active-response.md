# Active Response

Active Response provides real-time incident response capabilities on enrolled endpoints directly from the Fleet Server dashboard. When a detection fires or an investigation begins, operators can take immediate action without needing RDP or physical access to the endpoint.

## Capabilities Overview

| Category | Commands | Description |
|----------|----------|-------------|
| **Containment** | `isolate`, `unisolate` | Network isolation via WFP kernel filters |
| **Remediation** | `kill_process` | Terminate malicious processes |
| **Investigation** | `list_directory`, `get_file`, `run_command` | File browsing, evidence collection, remote shell |
| **Forensics** | `start_capture`, `stop_capture`, `yara_scan` | Live kernel captures; on-demand YARA scans (processes/files) |
| **Reconnaissance** | `collect_info`, `get_processes`, `get_network`, `get_services`, `get_drivers`, `get_autoruns`, `get_software`, `get_users` | Full endpoint inventory |
| **Logging** | `set_eventlog_policy`, `list_eventlog_channels`, `query_eventlog` | Windows Event Log management |
| **Protection** | `set_tamper_protection` | Enable/disable tamper protection |
| **Lifecycle** | `update_agent`, `uninstall`, `logoff_user` | Agent management |

## Network Isolation

Network isolation uses **Windows Filtering Platform (WFP)** to block all network traffic at the kernel level — much harder to bypass than firewall rules.

### Isolating an Agent

From the Agent Detail page, click **Isolate**. The agent:

1. Creates WFP filter rules blocking all inbound and outbound traffic
2. Adds exceptions for the Fleet Server IP (resolved from hostname) and any configured whitelist IPs
3. Allows DNS and DHCP to maintain connectivity
4. Continues heartbeating and accepting commands while isolated
5. Status changes to **Isolated** (amber badge) in the dashboard

### Unisolating

Click **Unisolate** on an isolated agent. The agent removes all WFP filters and restores full network access.

### Isolation Whitelist

Configure additional allowed IPs in Management > Account settings. Common entries:
- DNS servers
- Domain controllers
- Forensic tool servers
- SIEM/log collectors

### Important Notes

- Isolation **persists across service restarts** — the state is saved to disk
- In fleet mode, do NOT restart the service to revert isolation — use the `unisolate` command
- Isolation is applied at the kernel level; user-space tools cannot bypass it
- The agent remains fully manageable while isolated (heartbeat, commands, rule sync all continue)

## Process Termination

From the Agent Detail > Processes tab:

1. View all running processes with PID, name, path, memory, username
2. Click **Kill** on any process
3. Agent executes `taskkill /F /PID <pid>`
4. Killed processes show with strikethrough styling, "KILLED" badge, and faded row
5. Requires confirmation dialog

## File Browser

The Agent Detail > File Browser tab provides interactive filesystem navigation:

- Browse any directory on the endpoint
- View file name, size, modification date, and attributes
- Download individual files for evidence collection
- Navigate with breadcrumb path and click-to-enter directories
- CMMC/HIPAA file access compliance: downloads restricted to allowed file extensions when policy is active

## Remote Shell

The Agent Detail > Terminal tab provides PowerShell command execution:

- Execute arbitrary PowerShell commands
- Real-time output display
- 30-second default timeout per command (configurable for long-running operations)
- Command output is gzip-compressed for efficient transfer
- All commands are logged in the audit trail

## System Information Collection

The `collect_info` command gathers comprehensive endpoint inventory:

| Data | Source | Details |
|------|--------|---------|
| **Processes** | WMI `Win32_Process` (CIM fallback) | PID, name, path, memory, CPU, username, command line |
| **Services** | WMI `Win32_Service` | Name, state, start mode, account, PID, path |
| **Drivers** | WMI `Win32_SystemDriver` | Name, state, path, start mode, signature status |
| **Software** | Uninstall registry (x64 + x86) | Name, version, publisher, install date |
| **Users** | PowerShell + WMI | Local users, admin group membership, active sessions |
| **Network** | `Get-NetTCPConnection` | Local/remote address, port, state, owning process |
| **Autoruns** | Registry + Scheduled Tasks | Run keys, startup folder, scheduled tasks |
| **AV Products** | WMI SecurityCenter2 | Name, state, definition status |
| **RMM Tools** | Process pattern matching | ConnectWise, TeamViewer, AnyDesk, Splashtop, etc. |

Data sources use CIM as primary with WMI and `ipconfig` as fallbacks for environments where CIM is unavailable.

## Live Kernel Captures

The Agent Detail > Captures tab provides live kernel event capture:

- **Duration options**: 30s, 1m, 5m, 15m, or manual (unlimited)
- **Event type filters**: Process, File, Registry, Network, DNS, Image, Memory
- **Custom filters**: Full Fibratus QL expressions (e.g., `ps.name = 'powershell.exe' and evt.type = 'Connect'`)
- **Quick presets**: All Events, Process Activity, Network Activity, File System, Registry, Suspicious
- **Real-time display**: Terminal-style color-coded event output
- **Export**: JSON, CSV, or native `.kcap` format

The capture system uses a pure-Go `.kcap` writer (no CGO) compatible with `fibratus replay`.

## YARA Scan

The Agent Detail > **YARA Scan** tab runs on-demand YARA scanning against a running process or a file/directory on the endpoint. Rules come from the account's server-managed rule set (see [YARA Rule Management](yara-rules.md)) and travel with the command — agents do not load rules from disk and do not perform inline YARA matching on the event stream.

### Triggering a scan

1. Open the **YARA Scan** tab on an Agent Detail page.
2. Pick a target:
   - **Process (PID)** — scans live memory via libyara's `yr_scanner_scan_proc`.
   - **File / Directory** — scans an on-disk path; directories are walked recursively.
3. Optionally narrow the rule set using the checkbox list (empty = all enabled rules in the account).
4. Click **Run scan**. Results appear inline within ~60 seconds.

### Result display

Each match card shows the rule name + namespace, any tags, rule metadata (`threat_name`, `severity`, `score`, `author`), and matched strings with hex offsets and the decoded bytes. No matches surface as a single "No matches" panel with the count of rules applied. See [Match Results](../yara/alerts.md) for the wire format.

### Supported libyara modules

The agent binary includes `pe`, `elf`, `hash`, `math`, `magic`, `dotnet`, and `cuckoo`. Rules that `import` an unavailable module return a compile error in the scan response.

### Test paths

| Purpose | Target |
|---|---|
| Negative control | `C:\Windows\System32\notepad.exe` |
| Positive (marker file) | Create `C:\Users\Public\yara-test.txt` containing `ReflectiveLoader` — scan should match `Meterpreter_Stage_Marker` and `Reflective_DLL_Loader` |

Scan results are visible only to the operator who initiated the scan (and in the audit log) — they do not create rows in the Detections tab.

## Windows Event Log

### Per-Agent Collection Toggle

Enable or disable event log collection per agent from the Agent Detail > Event Log tab. Configure which channels to collect (Security, System, Sysmon).

### Event Log Browser

Browse collected Windows Event Log entries with:
- Event ID, level, source, channel, message
- Search and filter capabilities
- Integration with the Events (SIEM) page

### Additional Commands

- `list_eventlog_channels` — list available WEL channels on the endpoint
- `query_eventlog` — query events from a specific channel
- `export_evtx` — export `.evtx` file for offline analysis

## Tamper Protection

Enable or disable the 8-layer tamper protection system:

1. Service DACL — blocks SERVICE_STOP/DELETE for non-SYSTEM
2. Process DACL — denies PROCESS_TERMINATE to Everyone
3. Install directory lockdown — SYSTEM=full, Admins=read+execute
4. Add/Remove Programs hiding — sets SystemComponent=1
5. Registry key protection — deny write to service keys
6. Watchdog process — detached monitor that auto-restarts on death
7. Integrity monitor — 5-minute verification cycle
8. Persisted state — survives reboots

## Agent Lifecycle

### Self-Update

See [Auto-Update](/fleet/auto-update) for the complete self-update flow with tamper protection support.

### Remote Uninstall

The `uninstall` command:
1. Force-kills the Fibratus process
2. Deletes the Windows service
3. Runs MSI uninstall
4. Cleans up data files and enrollment data
5. Dashboard redirects to the agent list

### Session Management

`logoff_user` logs off a user session by session ID — useful for forcing re-authentication or terminating suspicious sessions.

## Security Model

- All commands require appropriate RBAC permissions (e.g., `agent:isolate`, `agent:shell`, `agent:kill_process`)
- Every command is logged in the audit trail with the issuing user
- Commands are authenticated via gRPC — only the fleet server can issue them
- Confirmation dialogs prevent accidental destructive actions (kill, isolate, uninstall)
- Root user commands are filtered from the audit log display
