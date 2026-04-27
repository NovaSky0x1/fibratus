# Configuration

## Server Configuration

The Fleet Server is configured via `/etc/fibratus/fleet-server.yml`:

```yaml
# Server settings
server:
  http_addr: "127.0.0.1:8443"    # HTTP API listen address
  grpc_addr: "127.0.0.1:8444"    # gRPC listen address

# PostgreSQL connection
database:
  host: "localhost"
  port: 5432
  name: "fibratus_fleet"
  user: "fibratus"
  password: "<generated>"
  sslmode: "disable"

# ClickHouse connection — bootstrap only.
# After the first successful boot, the active profile is read from the
# clickhouse_profiles table in Postgres. Edit profiles via the dashboard
# (Database → ClickHouse → Connection / Cloud Setup), not by hand-editing
# this file. The password field below is migrated to the encrypted secret
# store on first boot, then scrubbed from disk on the next dashboard save.
# See: ClickHouse Profiles & Cloud (fleet/clickhouse-profiles.md).
clickhouse:
  enabled: true
  host: "localhost"
  port: 9000
  database: "fibratus"
  user: "default"
  password: ""

# JWT authentication
auth:
  jwt_secret: "<generated>"
  token_expiry: "24h"

# API key for programmatic access
api:
  key: "<generated>"

# Logging
log:
  level: "info"
  file: "/var/log/fibratus-fleet/fleet-server.log"
```

### Configuration Fields

| Field | Description | Default |
|-------|-------------|---------|
| `server.http_addr` | HTTP API listen address | `127.0.0.1:8443` |
| `server.grpc_addr` | gRPC listen address | `127.0.0.1:8444` |
| `database.host` | PostgreSQL host | `localhost` |
| `database.port` | PostgreSQL port | `5432` |
| `database.name` | Database name | `fibratus_fleet` |
| `database.user` | Database user | `fibratus` |
| `database.password` | Database password | Generated at install |
| `database.sslmode` | SSL mode | `disable` |
| `clickhouse.host` / `port` | ClickHouse address. Bootstrap only — runtime values come from the active profile in Postgres. | `localhost:9000` |
| `clickhouse.database` | Bootstrap database | `fibratus` |
| `clickhouse.secure` | TLS for the bootstrap profile (Cloud needs `true`). Profile-level TLS settings override this on subsequent boots. | `false` |
| `auth.jwt_secret` | JWT signing secret | Generated at install |
| `auth.token_expiry` | JWT token expiry duration | `24h` |
| `api.key` | API key for programmatic access | Generated at install |
| `log.level` | Log verbosity (debug, info, warn, error) | `info` |
| `log.file` | Log file path | `/var/log/fibratus-fleet/fleet-server.log` |

## Agent Configuration

### Standalone Mode

When running without fleet enrollment, the agent uses `fibratus.yml`:

```yaml
# Standard Fibratus configuration
eventsource:
  etw:
    buffer-size: 64
    min-buffers: 32
    max-buffers: 128
    flush-period: 1s

output:
  console:
    enabled: true

alertsenders:
  - type: systray
    enabled: true
```

### Fleet Mode (Auto-Configured)

After enrollment, the agent auto-detects enrollment data and configures fleet mode. No YAML changes needed.

The agent automatically:
- Enables the fleet telemetry output
- Enables the fleet alert sender
- Starts the fleet client (heartbeat, commands, rule sync)
- Enables all ETW event types for comprehensive coverage (no drop masks)
- Enables stack enrichment for callstack-dependent detection rules and evasion detection
- Enables `enqueueAlways` so all events reach the telemetry output
- Zeroes compile-only rule fields after compilation to reduce memory (~40% RAM savings)

### Enrollment Data (DPAPI-Encrypted Registry)

```
HKLM\SOFTWARE\Fibratus\Fleet
├── AgentID
├── OrgID
├── ServerURL
├── Certificate
├── PrivateKey
└── CACertificate
```

All values are encrypted with DPAPI and only readable by the SYSTEM account.

### Configuration JSON Schema

The agent configuration is validated against a JSON Schema:
- The `fleet` section was added to the schema to pass validation
- Schema validation occurs at startup
- Invalid configuration prevents the agent from starting

## Telemetry Retention

### Per-Account Configuration

From the Super Admin panel:

| Setting | Description | Default |
|---------|-------------|---------|
| **Retention period** | Duration to keep telemetry data | 7 days |
| **New account default** | Default retention for new accounts | 1 day |

### ClickHouse TTL

Retention is enforced via ClickHouse TTL:

```sql
ALTER TABLE telemetry_<org>
MODIFY TTL timestamp + INTERVAL 7 DAY;
```

## Per-Organization Tables

Each organization gets its own ClickHouse table:

- Table name uses human-readable org name (sanitized)
- Isolated from other organizations
- Independent retention settings possible
- Separate query performance characteristics

## Event Filtering Configuration

### Security-Relevant Events

The agent filters which events are sent to the server. In fleet mode, the security-relevant whitelist includes:

- All process events (CreateProcess, TerminateProcess)
- Thread events (CreateThread, TerminateThread)
- File events (CreateFile, WriteFile with extension filtering, DeleteFile, RenameFile)
- Registry events (CreateKey, DeleteKey, SetValue, DeleteValue)
- Network events (Connect, Accept, Send, Recv, Disconnect)
- DNS events (QueryDns, ReplyDns)
- Image events (LoadImage)
- Memory events (VirtualAlloc, VirtualFree)
- Windows Event Log events (EventLogEvent)

### File Extension List

The 180+ security-relevant file extensions are configured in the agent code and cover executables, scripts, documents, archives, web files, and known malware file types.

## GitHub Sync Configuration

Stored in PostgreSQL, managed via the Management dashboard:

| Setting | Description |
|---------|-------------|
| **Repository URL** | GitHub repo URL (HTTPS or API) |
| **Path** | Subdirectory path within repo |
| **Branch** | Branch to sync from |
| **Token** | GitHub personal access token |
| **Scope** | Account-wide or organization-specific |
| **Enabled** | Whether sync is active |

Multiple sync sources are supported per account/organization.

## Tamper Protection Settings

Managed via the Management dashboard and propagated to agents:

| Setting | Level | Description |
|---------|-------|-------------|
| **Tamper protection** | Account/Org/Agent | Protect agent from modification |
| **Isolation whitelist** | Account/Org | IPs allowed during network isolation |
| **File access policy** | Account/Org | CMMC/HIPAA file access restrictions |
| **Event log channels** | Account/Org/Agent | Which WEL channels to collect |

Settings follow a hierarchy: account → organization → agent, with more specific settings overriding broader ones.
