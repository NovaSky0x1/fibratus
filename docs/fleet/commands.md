# Active Response Commands

The Fleet Server supports remote command execution on enrolled agents for incident response, forensic investigation, and endpoint management. Commands are issued through the dashboard and executed by the agent's command executor.

## Command Flow

1. **User issues command** in the dashboard (e.g., clicks "Isolate")
2. **REST API** creates a command record in PostgreSQL (status: `pending`)
3. **Agent polls** for pending commands every 5 seconds via gRPC `CommandChannel`
4. **Agent executes** the command using the appropriate handler
5. **Agent reports result** back to the server (status: `completed` or `failed`)
6. **Dashboard updates** — result visible in Command History

## Available Commands

### Network Isolation

#### `isolate`
Activates Windows Filtering Platform (WFP) network isolation:
- Blocks all inbound and outbound network traffic
- Allows communication only with the Fleet Server IP
- Server hostname is resolved to IP before applying WFP rules
- Agent continues heartbeating and accepting commands while isolated
- Agent status changes to "Isolated" in the dashboard

#### `unisolate`
Removes WFP network isolation:
- Restores full network connectivity
- Removes all WFP filter rules
- Agent status returns to "Online"

See [Tamper Protection & Network Isolation](fleet/tamper-protection.md) for details.

### Process Management

#### `kill_process`
Terminates a process on the endpoint:
- Specify process by PID
- Force kill (no graceful shutdown)
- Dashboard shows killed process with strikethrough styling, "KILLED" badge, and faded row
- Confirmation dialog before execution

### File System

#### `list_directory`
Lists the contents of a directory:
- Returns files and subdirectories with metadata
- File name, size, modification date, attributes
- Used by the interactive File Browser component

#### `get_file`
Downloads a file from the endpoint:
- Specify file path
- File is retrieved and made available for download
- Used for evidence collection

### Remote Shell

#### `run_command`
Executes a PowerShell command on the endpoint:
- Full PowerShell command execution
- Real-time output streaming
- Used by the Remote Terminal component
- Output is gzip-compressed for efficient transfer

### System Information

#### `collect_info`
Collects comprehensive system information:
- **Processes** — all running processes with PID, name, path, memory, CPU
- **Services** — Windows services with name, status, start type, account
- **Drivers** — loaded kernel drivers with name, path, signature status
- **Software** — installed applications with name, version, publisher
- **Users** — local user accounts with group membership
- **Network Adapters** — interfaces with IP, MAC, speed, status
- **Autoruns** — persistence mechanisms (run keys, scheduled tasks, startup)
- **AV Products** — detected antivirus products via WMI/CIM
- **RMM Tools** — detected remote management tools

The command uses CIM/WMI with fallbacks:
- Primary: CIM cmdlets (`Get-CimInstance`)
- Fallback: Direct WMI queries
- Fallback: `ipconfig` for network info when CIM/WMI unavailable

### Agent Lifecycle

#### `uninstall`
Remotely uninstalls the agent:
- Force-kills the Fibratus process
- Runs MSI uninstall
- Agent record deleted from database
- Dashboard redirects to agent list

#### `auto_update`
Triggers agent self-update:
- Server detects latest release from GitHub Releases
- Update command pushed to agent via heartbeat response
- Agent downloads new MSI from GitHub
- Agent force-kills itself, installs new MSI
- Service auto-restarts with new version
- Can be triggered per-agent or account-wide

See [Auto-Update](fleet/auto-update.md) for details.

## Command Execution

### Agent-Side Executor

The command executor on Windows (`executor_windows.go`) processes commands:

1. Receives command from server via command polling
2. Validates command type and parameters
3. Executes the appropriate handler
4. Collects output/results
5. Reports result back to server (gzip-compressed if needed)

### Command Result Handling

- **Success**: Result stored in PostgreSQL, displayed in dashboard
- **Failure**: Error message stored, visible in command history
- **Timeout**: Commands that don't complete are marked as timed out
- **User attribution**: Every command records which user issued it

### Root Command Filtering

Commands issued by the root system user are filtered from:
- Audit log display
- Command history in the dashboard
- Prevents internal system commands from cluttering the user interface

## Dashboard Integration

### Process List

The Processes tab on the agent detail page:
- Shows all running processes
- Each process row has a **Kill** button
- Killed processes show with strikethrough and "KILLED" badge
- Process list refreshes after kill to reflect changes

### File Browser

The File Browser tab provides an interactive file system navigator:
- Navigate directories by clicking
- Breadcrumb navigation
- Download individual files
- File metadata display (size, date, attributes)

### Remote Terminal

The Terminal tab provides a full command shell:
- PowerShell command input
- Real-time output display
- Command history within the session
- Styled as a terminal interface

### Command History

The Command History tab shows all commands for the agent:
- Command type and parameters
- User who issued the command
- Timestamps (issued, completed)
- Execution status (pending, running, completed, failed)
- Result output (expandable)

## Security Considerations

- Commands require appropriate RBAC permissions
- All commands are logged in the audit trail
- Network isolation commands have special handling for server connectivity
- Kill and uninstall commands require confirmation
- Root user commands are hidden from regular audit views
