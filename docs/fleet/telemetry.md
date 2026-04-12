# Telemetry Pipeline

The telemetry pipeline streams security-relevant kernel events from agents to ClickHouse for storage, search, and investigation. It handles high-volume event ingestion while filtering out noise and enriching events with process context.

## Architecture

```
Agent                                Server
┌─────────────────────┐    ┌─────────────────────────────┐
│ ETW Kernel Trace     │    │                             │
│       │              │    │  gRPC/NATS Telemetry Handler│
│  Event Processors    │    │         │                   │
│       │              │    │  UTF-8 Sanitization         │
│  Aggregator          │    │         │                   │
│       │              │    │  Per-Org Table Routing       │
│  Fleet Telemetry     │───▶│         │                   │
│  Output              │    │  Buffered Bulk Insert       │
│                      │    │         │                   │
│  (security-relevant  │    │  ClickHouse                 │
│   events only)       │    │  (columnar storage)         │
└─────────────────────┘    └─────────────────────────────┘
```

## Event Coverage

### Security-Relevant Events (Streamed to Server)

The agent filters events to only stream security-relevant activity:

| Event Type | Events Included |
|------------|----------------|
| **Process** | CreateProcess, TerminateProcess |
| **Thread** | CreateThread, TerminateThread |
| **File** | CreateFile, WriteFile (security-relevant extensions only), DeleteFile, RenameFile |
| **Registry** | CreateKey, DeleteKey, SetValue, DeleteValue |
| **Network** | Connect, Accept, Send, Recv, Disconnect |
| **DNS** | QueryDns, ReplyDns |
| **Image/Module** | LoadImage (DLL loading) |
| **Memory** | VirtualAlloc, VirtualFree (injection-related) |
| **Windows Event Log** | Security, System, Sysmon channels |

### File Extension Filtering

File creation and write events are filtered to 180+ security-relevant extensions including:

- **Executables**: `.exe`, `.dll`, `.sys`, `.scr`, `.com`, `.drv`
- **Scripts**: `.ps1`, `.bat`, `.cmd`, `.vbs`, `.js`, `.wsh`, `.hta`
- **Documents**: `.docm`, `.xlsm`, `.pptm` (macro-enabled)
- **Archives**: `.zip`, `.rar`, `.7z`, `.cab`
- **Web**: `.html`, `.htm`, `.asp`, `.aspx`, `.php`, `.jsp`
- **C2/Malware**: `.dll`, `.ocx`, `.cpl`, `.msi`, `.appx`
- **And many more** categories including AutoIt, WMI, CHM, ClickOnce, Electron

### Excluded Events

High-volume noise events are excluded from telemetry:
- `OpenProcess` (200K+/5min)
- `Threadpool` events
- Non-security file operations (e.g., opening existing files)

## Event Enrichment

Each telemetry event includes enriched context:

### Process State Block
Every event carries full process context:
- Process name, PID, parent PID
- Command line
- Executable path
- Current working directory
- **SHA256 and MD5 hashes** of the process executable
- Code signing status (is_signed, is_trusted)
- SID and session ID
- Environment variables

### Process Ancestry Chain
Events include the full parent chain:
- Parent process name, PID, path, hashes
- Grandparent and further ancestors
- Enables tracing the full execution chain

### Callstack Data
For events that support callstacks:
- Return addresses
- Module names
- Symbol names (when available)

### File Hashes
- SHA256 and MD5 computed for all files and loaded images
- Hashes injected into LoadImage event parameters
- Available in the process state block for every event

## ClickHouse Storage

### Per-Organization Tables

Each organization gets its own ClickHouse table for data isolation:

```sql
CREATE TABLE telemetry_<org_name> (
    timestamp DateTime64(3),
    agent_id String,
    event_type String,
    event_category String,
    pid UInt32,
    process_name String,
    params String,          -- JSON-encoded event parameters
    ps_info String,         -- JSON-encoded process state block
    raw_event String,       -- Full serialized event
    -- ... additional columns
) ENGINE = MergeTree()
ORDER BY (timestamp, agent_id, event_type)
```

### Compression and Performance

ClickHouse provides excellent compression for telemetry data:
- **Columnar storage**: Only reads columns needed for each query
- **Compression codecs**: LZ4 for fast queries, ZSTD for long-term storage
- **MergeTree engine**: Optimized for time-series data with automatic background merges
- **Connection pooling**: Configurable connection pool for concurrent ingestion
- **Buffered ingestion**: Events are batched before bulk insert for throughput

### Configurable Retention

Telemetry retention is configurable per-account from the Super Admin panel:

| Setting | Description | Default |
|---------|-------------|---------|
| **Retention period** | How long to keep telemetry events | 7 days |
| **Per-account control** | Each account can have different retention | Yes |
| **Automatic cleanup** | ClickHouse TTL-based automatic deletion | Enabled |

New accounts default to 1 day retention to prevent unexpected storage growth.

## Querying Telemetry

### Fibratus QL Integration

The Events page supports the full Fibratus Query Language, which is translated to SQL WHERE clauses for ClickHouse:

```
# Fibratus QL query
ps.name imatches 'powershell*' and net.dport = 443

# Translated to SQL
WHERE lower(process_name) LIKE 'powershell%'
  AND JSON_EXTRACT(params, '$.net.dport') = 443
```

Supported operators:
- `=`, `!=`, `>`, `<`, `>=`, `<=`
- `imatches` (case-insensitive glob)
- `contains`, `icontains` (substring search)
- `in`, `not in`
- `and`, `or`, `not`

### JSON Field Search

Event parameters stored as JSON are searchable:
- DNS names: `dns.name contains 'malicious.com'`
- File paths: `file.path imatches '*\\System32\\*'`
- Registry keys: `registry.key contains 'Run'`
- Network destinations: `net.dip = '10.0.0.1'`

### Search Fallback

If a Fibratus QL query fails to parse, the system falls back to ClickHouse ILIKE text search across all event fields.

## Fleet Mode Configuration

In fleet mode, the agent automatically configures telemetry:

- **enqueueAlways**: All events are passed to the aggregator (not just rule matches)
- **Stack enrichment**: Disabled in fleet mode for full event coverage without performance impact
- **Console output**: Fleet telemetry output auto-enables even when console output is configured
- **Event type enablement**: All ETW event types enabled for comprehensive coverage
