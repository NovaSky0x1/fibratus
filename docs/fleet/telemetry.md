# Telemetry Pipeline

The telemetry pipeline streams security-relevant kernel events from agents to ClickHouse for storage, search, and investigation. It handles high-volume event ingestion while filtering out noise and enriching events with process context.

## Architecture

**Agent Pipeline:**
1. ETW Kernel Trace captures raw events
2. Event Processors enrich events (process state, hashes, callstacks)
3. Aggregator batches events
4. Fleet Telemetry Output filters to security-relevant events and streams via gRPC

**Server Pipeline:**
1. gRPC/NATS Telemetry Handler receives event batches
2. UTF-8 sanitization cleans event data
3. Per-org table routing directs events to the correct ClickHouse table
4. Buffered bulk insert writes events to ClickHouse (columnar storage with compression)

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

Editing retention from either Super Admin → Accounts or Management → Account fires `ALTER TABLE … MODIFY TTL` against whichever ClickHouse profile is currently active (local or Cloud). The metadata change is instant; the actual row deletion happens during background merges (within hours of the threshold).

### Local vs. ClickHouse Cloud

Telemetry can target either a self-hosted ClickHouse on the fleet host or a managed [ClickHouse Cloud](https://clickhouse.com/cloud) service. Both are stored as **profiles** (`local` and `cloud`) in the `clickhouse_profiles` table; switching is one click in the dashboard with no service restart — the running pipeline drains its in-memory buffer, opens a fresh connection to the new profile, and swaps atomically.

The full setup flow (paste a Console API key, list services, rotate password, hot-swap) is documented in [ClickHouse Profiles & Cloud](fleet/clickhouse-profiles.md).

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
- **Stack enrichment**: Enabled — callstack data is collected for eligible events (CreateProcess, LoadImage, CreateThread, registry ops, etc.). The StackwalkDecorator uses a 2-phase flush to prevent deadlocks under high throughput. Events wait up to 10 seconds for matching StackWalk events; if none arrive, they are flushed without callstack data. This enables 40% of detection rules that depend on `thread.callstack.*` fields and all evasion detection (direct/indirect syscall analysis).
- **Console output**: Fleet telemetry output auto-enables even when console output is configured
- **Event type enablement**: All ETW event types enabled for comprehensive coverage
- **Drop masks disabled**: Rule compile result is set to nil so the ETW source collects all event types, even before rules are synced from the server

## Memory Optimization

After rules are compiled, the engine releases compile-only fields from memory to reduce the agent's RAM footprint:

- **Fields zeroed after compilation**: `Condition` (source expression string), `References` (URL lists), `Notes`, `Authors`, `MinEngineVersion`, `Version`
- **Fields retained for runtime**: `Name`, `ID`, `Description`, `Output` (alert template), `Severity`, `Labels`, `Tags`, `Action`
- **Impact**: ~40% reduction in working set memory (e.g., 400 MB down to ~250 MB with 2,100 compiled rules)
- **How it works**: The compiled filter ASTs are all the runtime needs — the source YAML text that produced them is no longer referenced after compilation and is eligible for garbage collection
