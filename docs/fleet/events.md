# Events (SIEM)

The Events page provides a SIEM-style interface for searching, filtering, and investigating telemetry events across the fleet. It supports the full Fibratus Query Language with real-time event streaming and rich event detail.

## Events Page Layout

The Events page has three main sections:

- **Top bar**: Fibratus QL query input, time range selector, search button, and Live mode toggle
- **Filter pills**: Active filters shown as removable badges below the query bar
- **Left sidebar**: Dynamic fields browser organized by category (evt.*, ps.*, file.*, net.*, reg.*, dns.*)
- **Main area**: Event table with columns for Timestamp, Type, Category, Process, Preview, and Agent
- **Pagination**: Page navigation at the bottom

## Fibratus QL Query Bar

The query bar accepts Fibratus Query Language expressions:

### Basic Queries

```
# Process events
ps.name = 'powershell.exe'

# Network connections
net.dport = 443 and net.dip != '10.0.0.0/8'

# File operations
file.path imatches '*\\System32\\*' and evt.type = 'CreateFile'

# DNS queries
dns.name icontains 'suspicious.com'

# Registry modifications
registry.key.name icontains 'Run'

# Combined queries
ps.name = 'cmd.exe' and ps.parent.name = 'winword.exe'
```

### Context-Aware Autocomplete

The query bar features intelligent autocomplete:

- **Field names** — suggests valid fields as you type (e.g., `ps.` shows `ps.name`, `ps.exe`, `ps.cmdline`, etc.)
- **Operators** — suggests valid operators for the field type
- **Keywords** — suggests `and`, `or`, `not`
- **Values** — suggests common values for known fields
- **Type badges** — each suggestion shows its type (field, operator, keyword, value)

### Supported Fields

| Prefix | Fields |
|--------|--------|
| `evt.*` | `evt.type`, `evt.category`, `evt.timestamp` |
| `ps.*` | `ps.name`, `ps.exe`, `ps.cmdline`, `ps.pid`, `ps.ppid`, `ps.parent.name`, `ps.parent.exe`, `ps.username`, `ps.sid` |
| `file.*` | `file.name`, `file.path`, `file.operation` |
| `net.*` | `net.dip`, `net.dport`, `net.sip`, `net.sport`, `net.protocol` |
| `dns.*` | `dns.name`, `dns.type`, `dns.rcode` |
| `registry.*` | `registry.key.name`, `registry.value`, `registry.path` |
| `eventlog.*` | `eventlog.event_id`, `eventlog.provider`, `eventlog.level`, `eventlog.channel` |

### Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `=` | Exact match | `ps.name = 'cmd.exe'` |
| `!=` | Not equal | `ps.name != 'svchost.exe'` |
| `>`, `<`, `>=`, `<=` | Numeric comparison | `net.dport > 1024` |
| `imatches` | Case-insensitive glob | `ps.exe imatches '*\\temp\\*'` |
| `contains` | Substring (case-sensitive) | `ps.cmdline contains '-enc'` |
| `icontains` | Substring (case-insensitive) | `dns.name icontains 'evil'` |
| `in` | Set membership | `net.dport in (80, 443, 8080)` |
| `matches` | Regex match | `ps.name matches '^(cmd|powershell)\.exe$'` |

## Dynamic Fields Sidebar

The left sidebar shows available fields with:
- Categorized by type (event, process, file, network, registry, DNS)
- Click a field to add it as a filter
- Shows field value distribution for the current result set

## Event Table

### Columns

| Column | Description |
|--------|-------------|
| **Timestamp** | Event time with millisecond precision |
| **Type** | Event type (CreateProcess, Connect, SetValue, etc.) |
| **Category** | Event category with color coding |
| **Process** | Process name that generated the event |
| **Preview** | Inline preview of key parameters |
| **Agent** | Hostname of the source endpoint |
| **Organization** | Org name (visible in cross-org view) |

### Event Preview

Each row shows an inline preview of the most important parameters:
- **Process events**: PID, command line snippet
- **Network events**: destination IP and port
- **DNS events**: queried domain name
- **File events**: file path
- **Registry events**: key name and value
- **Event Log events**: Event ID badge, source, message

### Expanded Event Detail

Click an event row to expand it and see:
- **Full parameters** — all event parameters in a structured view
- **Process context** — complete process state block (name, PID, path, hashes, signing, parent chain)
- **Callstack** — if available, the kernel/user callstack
- **Raw JSON** — full raw event data

### Event Log Events

Windows Event Log events have a dedicated display:
- **Event ID badge** — prominently displayed
- **Channel** — Security, System, or Sysmon
- **Level** — Information, Warning, Error, Critical
- **Provider** — event source
- **Message** — event log message content

## Time Range Filtering

| Option | Description |
|--------|-------------|
| **Last 15 minutes** | Events from the last 15 minutes |
| **Last 1 hour** | Events from the last hour |
| **Last 4 hours** | Events from the last 4 hours |
| **Last 24 hours** | Events from the last 24 hours |
| **Last 7 days** | Events from the last 7 days |
| **Custom range** | Specify custom start and end timestamps |

## Live Mode

Toggle "Live" mode to stream events in real time:
- Events appear as they are ingested
- Auto-scrolls to show newest events
- Pause/resume streaming
- Combine with filters for targeted monitoring

## Filter Pills

Active filters appear as removable pills below the query bar:
- Each filter shows field, operator, and value
- Click the × to remove a filter
- Multiple filters combine with AND logic

## Search Fallback

If a Fibratus QL query fails to parse:
- The system falls back to full-text ILIKE search across all event fields
- Both the QL query and plain text search parameters are sent to the server
- Ensures the user always gets results even with imperfect syntax

## Per-Agent Events

The Agent Detail page has a dedicated Events tab that:
- Shows only events from that specific agent
- Supports the same query and filter capabilities
- Includes agent-specific event type filtering
- Multi-expand support for viewing multiple events simultaneously
