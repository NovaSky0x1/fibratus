# Fibratus - Project Instructions

## What This Is
Fibratus is a Windows-native security tool for adversary tradecraft detection, protection, and hunting. It consumes ETW (Event Tracing for Windows) kernel events, evaluates them against YAML detection rules mapped to MITRE ATT&CK, scans process memory with YARA signatures, and supports forensic capture/replay via `.kcap` files. It runs as a CLI tool or Windows Service.

## Environment
- This is a **fork** of [rabbitstack/fibratus](https://github.com/rabbitstack/fibratus). Default upstream branch is `master`.
- **Windows-only runtime** — the codebase uses `_windows.go` / `_unsupported.go` suffixes extensively. All ETW, syscall, and kernel interactions require Windows.
- **Build system**: `make.bat` (Windows batch script, NOT GNU Make). Run from PowerShell.
- **Go version**: 1.26.x (check `go.mod` for exact)
- **C dependencies** (for full build): MSYS2/MinGW toolchain, Python 3.7.9 headers (filament), libyara 4.2.x
- **Build tags**: `filament` (Python support), `cap` (capture files), `yara`+`yara_static` (YARA scanning). Set via `$env:TAGS` before `./make.bat`.

## Code Change Rules
- Use minimal, precise edits — no unnecessary whitespace, restructuring, or reformatting
- For complex changes, explain the approach before applying edits so I can review
- Explore relevant files and dependencies before making changes — this is a large, interconnected codebase
- Follow senior Go developer standards: idiomatic Go, clear error handling, interface-driven design
- **Always list all files changed at the end of every task**
- **NEVER attribute anything to Claude** — no `Co-Authored-By: Claude` in commits, no mention of AI in PR descriptions, commit messages, code comments, or anywhere else. All work is attributed to the human author only.
- **NEVER use browser-native `window.confirm()` / `window.alert()` / `window.prompt()` in the dashboard.** They look terrible and break the visual language of the app. Use the existing `ConfirmDialog` component (or build an equivalent inline confirmation/toast) instead. Applies to every page, every flow, every "are you sure?" — no exceptions.

## Building & Testing
```powershell
# Basic build (no optional features)
./make.bat build

# Full build with all features
$env:TAGS="filament,cap,yara,yara_static"; ./make.bat build

# Run tests
./make.bat test    # go test -timeout=10m -v -gcflags=all=-d=checkptr=0

# Lint
./make.bat lint    # golangci-lint v2.9.0

# Format
./make.bat fmt     # gofmt -e -s -l -w
```

## Project Structure

### Entry Points
- `cmd/fibratus/` — Main CLI binary. `main_windows.go` detects service vs CLI mode.
- `cmd/fibratus/app/` — Cobra command definitions (`run`, `capture`, `replay`, `rules`, `list`, `stats`, `config`, `service`)
- `cmd/systray/` — System tray notification binary (named pipe IPC)

### Core Pipeline Architecture
```
ETW Kernel Trace → Consumer → Processor Chain → Filter → Rules Engine → Alert Senders
                                                    ↓                        ↓
                                               Aggregator → Transformers → Outputs
```

### Key Packages

| Package | Purpose |
|---------|---------|
| `pkg/event/` | Core event data structures, serialization, batching, queuing |
| `pkg/filter/` | Filter engine — compilation, evaluation, field accessors |
| `pkg/filter/ql/` | Custom query language — lexer, parser, AST, 24+ built-in functions |
| `pkg/rules/` | Detection rule engine — compiler, evaluator, sequence FSM |
| `pkg/ps/` | Process snapshotter — real-time process/thread/module state |
| `pkg/source/` | EventSource interface definition |
| `pkg/aggregator/` | Event batching, transformer pipeline, output dispatch |
| `pkg/outputs/` | Output sinks — Console, Elasticsearch, AMQP, HTTP, EventLog |
| `pkg/alertsender/` | Alert delivery — Slack, Email, Systray, EventLog |
| `pkg/sys/` | Low-level Windows syscall bindings (9 DLLs) |
| `pkg/sys/etw/` | ETW API bindings (StartTrace, EnableTrace, ProcessTrace) |
| `pkg/pe/` | Portable Executable analysis |
| `pkg/handle/` | Windows handle enumeration and introspection |
| `pkg/cap/` | Capture file (.kcap) reader/writer |
| `pkg/filament/` | Python filament engine (CPython bindings) |
| `pkg/yara/` | YARA memory scanning engine |
| `pkg/config/` | Configuration management (Viper + JSON Schema validation) |
| `pkg/util/` | 30+ utility sub-packages |
| `internal/bootstrap/` | App orchestration — wires entire pipeline together |
| `internal/etw/` | ETW event source implementation + processor chain |
| `internal/etw/processors/` | Event processors: Process, FS, Registry, Net, Handle, Module, Memory |
| `internal/evasion/` | Evasion detection (direct/indirect syscall analysis) |

### Other Important Directories
- `rules/` — 126 YAML detection rules + `macros/` directory
- `configs/` — `fibratus.yml` and `fibratus.json` configuration files
- `docs/` — Docsify-based documentation site
- `filaments/` — Python extension modules
- `build/msi/` — WiX Toolset MSI packaging manifests
- `pkg-config/` — `.pc` files for C dependencies

## Architectural Patterns

### Factory/Registry Pattern
Outputs, alert senders, and transformers use self-registering factories:
```go
// In implementation package (e.g., pkg/outputs/console/)
func init() {
    outputs.Register(outputs.Console, newConsole)
}
```
Register new implementations via `init()` — they auto-register with the factory.

### Interface-First Design
Major subsystems define interfaces first, then concrete implementations:
- `source.EventSource` — event acquisition
- `ps.Snapshotter` — process state (14 methods)
- `handle.Snapshotter` — handle enumeration
- `filter.Accessor` — field access for filter evaluation
- `outputs.Output` — event output sinks
- `alertsender.Sender` — alert delivery

### Functional Options
The bootstrap `App` uses functional options: `WithDebugPrivilege()`, `WithSignalHandler()`, `WithCaptureReplay()`, etc.

### Processor Chain (Middleware)
`internal/etw/processors/` — each processor implements `ProcessEvent() -> (event, continue, error)`. Seven types: Process, FS, Registry, Image, Network, Handle, Memory.

### Platform-Specific Files
- `*_windows.go` — Windows implementation (the majority of code)
- `*_unsupported.go` — Stub/no-op for non-Windows builds
- `zsyscall_windows.go` — Auto-generated by `go generate` using `mkwinsyscall`

### Sequence State Machine
`pkg/rules/sequence.go` — Deterministic FSM for multi-event rule matching with temporal constraints, slot-based partial tracking, and automatic expiration/GC.

## Filter Query Language
Custom DSL in `pkg/filter/ql/` with:
- **Lexer**: Based on InfluxQL scanner, 3-token circular buffer
- **Parser**: Recursive descent, binary expression trees
- **24 built-in functions**: String ops, path ops, crypto (md5), network (cidr), registry (get_reg_value), security (yara, minidump, regex, entropy)
- **12 field accessor types**: Event, Process, Thread, File, Module, Registry, Network, Handle, PE, Memory, DNS, Threadpool
- **Sequence support**: `BY` (join), `AS` (alias) clauses, `maxspan` temporal constraints

To add a new filter function: implement in `pkg/filter/ql/functions/`, register in `pkg/filter/ql/function.go`.

To add a new filter field: add to `pkg/filter/fields/`, implement accessor in `pkg/filter/accessor_windows.go`.

## Detection Rules
YAML files in `rules/`, categorized by MITRE ATT&CK tactic. Structure:
```yaml
name: <descriptive name>
id: <UUID>
version: <semver>
description: <what it detects>
labels:
  tactic.id: <MITRE tactic ID>
  tactic.name: <MITRE tactic name>
  technique.id: <MITRE technique ID>
  technique.name: <MITRE technique name>
condition: >
  spawn_process and
  (ps.name imatches '...' and ps.cmdline imatches '...')
min-engine-version: 3.0.0
severity: low|medium|high|critical
output: <alert template string>
```
**CI enforces version bumps** when rules are modified. Validated via JSON Schema (`pkg/config/rules.schema.json`).

## Windows Internals Reference

### ETW (Event Tracing for Windows)
- Bindings in `pkg/sys/etw/` — `StartTrace`, `EnableTrace`, `ProcessTrace`, `ControlTrace`
- Source implementation in `internal/etw/source.go`
- Kernel providers for: Process, Thread, File I/O, Registry, Network, DNS, Image/DLL loading, Handle, Memory, ALPC, Threadpool

### System DLL Bindings (`pkg/sys/zsyscall_windows.go`)
| DLL | Purpose |
|-----|---------|
| `dbghelp.dll` | Symbol resolution (SymInitialize, SymFromAddr) |
| `kernel32.dll` | Core process/thread APIs |
| `ntdll.dll` | Native API (NtAlpcQueryInformation, NtCreateSection, NtQueryObject) |
| `psapi.dll` | Process/memory utilities (EnumDeviceDrivers, GetMappedFileName) |
| `wintrust.dll` | Code signing verification (WinVerifyTrust) |
| `wtsapi32.dll` | Terminal Services session queries |
| `shell32.dll`, `shlwapi.dll`, `user32.dll` | Shell/UI operations |

### Key Concepts Used
- **PEB (Process Environment Block)**: `pkg/ps/peb.go` — reads process env vars, command lines
- **SE_DEBUG privilege**: Elevated access for inspecting other processes
- **Handle introspection**: NtQueryObject for handle type/name resolution
- **ALPC monitoring**: NtAlpcQueryInformation for IPC tracking
- **Code signing**: WinVerifyTrust for module/driver signature validation
- **LOLDrivers**: `pkg/util/loldrivers/` — known vulnerable driver detection
- **Evasion detection**: `internal/evasion/` — direct/indirect syscall analysis via call stack inspection

## Testing Conventions
- Standard Go `testing` + `github.com/stretchr/testify` (`assert` + `require`)
- `require.NoError()` for fatal assertions, `assert.Equal()` for soft checks
- `_fixtures/` directories for test data (YAML, configs, PE files)
- Mock files co-located: `*_mock.go` alongside real implementations
- Table-driven tests used alongside individual test functions
- Tests are Windows-only — they interact with ETW, Windows APIs, process management

## Configuration
- Primary: `configs/fibratus.yml` (YAML), alternative: `configs/fibratus.json`
- Loaded via Viper with Cobra CLI flag integration
- Validated against JSON Schema files in `pkg/config/`
- Key sections: `aggregator`, `alertsenders`, `api`, `filters`, `eventsource`, `output`, `pe`, `yara`, `evasion`, `transformers`

## Commit & PR Conventions
- **Conventional Commits** required: `feat(scope): Description`, `fix(scope): Description`, `refactor(scope): Description`
- **Scopes**: instrumentation, telemetry, rule-engine, filters, yara, event, captures, alertsenders, outputs, rules, filaments, config, cli, tests, ci, build, deps, evasion
- Clean commit history — squash fix commits with `git reset --soft`, not additional commits
- Rebase onto master for conflict resolution

## Linting (`.golangci.yml`)
Enabled linters: bodyclose, errcheck, goconst, goprintffuncname, govet, ineffassign, nakedret, noctx, nolintlint, rowserrcheck, staticcheck, unconvert, unparam, unused, whitespace. Test files have relaxed errcheck/staticcheck/noctx rules.

## Fork & Repo Layout
- **Upstream**: `rabbitstack/fibratus` (branch: `master`)
- **Our fork**: `NovaSky0x1/fibratus` on GitHub
- **Local clone**: `fibratus-repo/` subdirectory (remotes: `origin` = our fork, `upstream` = rabbitstack)
- Use skills (`/add-rule`, `/add-output`, etc.) for guided development workflows

## Upstream Backlog & Roadmap

### Open Feature Requests (upstream issues)
| # | Feature | Scope | Complexity |
|---|---------|-------|------------|
| 645 | Publish packages to winget | MSI/packaging | Medium |
| 644 | Publish packages to Chocolatey | MSI/packaging | Medium |
| 643 | Publish packages to Scoop | MSI/packaging | Medium |
| 506 | Transformers interact with non-kparams fields | Transformers | Medium |
| 410 | New `logs` CLI command | CLI | Low |
| 243 | Filament rule action (Python-based rule actions) | Filaments/YARA | High |
| 242 | Allow filaments with system Python interpreter | Filaments/Config | Medium |
| 208 | Process token impersonation level filter field | Filters/Process | Low-Medium |
| 207 | Process token privileges filter field | Filters/Process | Low-Medium |
| 68 | `ancestor_of`/`descendant_of` process ancestry functions | Filters | Medium |
| 52 | Encrypt capture files (.kcap) | Captures | Medium |
| 48 | Watch/monitor ETW kernel logger session health | Events | Medium |
| 44 | CLI `config edit` command | CLI | Low |
| 43 | New `encode` transformer | Transformers | Low |
| 42 | `scan_proc`/`scan_file` filament functions | Filaments/YARA | Medium |
| 36 | User SID for file/registry events | Events | Medium |
| 35 | New `encrypt` transformer | Transformers | Medium |
| 34 | New `mask` transformer (data masking) | Transformers | Low |
| 32 | ALPC event support | Events/Filters | High |
| 31 | MongoDB output sink | Outputs | Medium |
| 30 | `find_handle`/`find_handles` filament functions | Filaments/Handle | Medium |
| 29 | `find_process`/`find_processes` filament functions | Filaments | Medium |
| 28 | Submit capture to S3 bucket | Captures | Medium |
| 9 | Splunk output sink | Outputs | Medium |

### Open Bugs
| # | Bug | Scope |
|---|-----|-------|
| 570 | False positives on clean systems out of the box | Rules |
| 568 | Users can read sensitive information | Security |
| 514 | Replaying fails converting big file to JSON | Captures |

### Recent Development Direction (merged PRs, early 2026)
- **Performance**: Overlapped I/O for file reads, symbolizer caching, thread pool for working set queries, increased ETW session buffers
- **Rules**: Heavy focus on UAC bypass detection rules (10+ new rules), false positive tuning
- **Console output colorization**: New feature for readable console output
- **Callstack analysis**: Improved working set queries and symbolization
- **CI**: SHA256 checksums in releases, benchmark jobs

### Good First Issues (tagged by upstream)
- #208 — Process token impersonation level (needs: filters, docs)
- #207 — Process token privileges (needs: filters, docs)

### Missing Output Sinks (community-requested)
- Splunk (#9) — high demand
- MongoDB (#31)
- Kafka (label exists, no issue)

### Missing Transformers
- `encode` (#43) — encoding transformation
- `encrypt` (#35) — field encryption
- `mask` (#34) — data masking/redaction

## Adding New Components — Quick Reference

### New Output Sink
1. Create `pkg/outputs/<name>/` with implementation of `outputs.Output` interface
2. Self-register via `init()` calling `outputs.Register()`
3. Add config section to `pkg/config/output.go`

### New Alert Sender
1. Create `pkg/alertsender/<name>/` implementing `alertsender.Sender`
2. Self-register via `init()` calling `alertsender.Register()`
3. Add config section to `pkg/config/alertsender.go`

### New Filter Function
1. Implement in `pkg/filter/ql/functions/<name>.go`
2. Register in `pkg/filter/ql/function.go`

### New Filter Field
1. Define field in `pkg/filter/fields/fields_windows.go`
2. Implement accessor logic in `pkg/filter/accessor_windows.go`

### New Event Processor
1. Create `internal/etw/processors/<name>_windows.go` implementing `processor.Processor`
2. Wire into chain in `internal/etw/processors/chain_windows.go`

### New Detection Rule
1. Create `rules/<tactic>_<technique>_<description>.yml` following the YAML schema
2. Assign UUID, semver version, MITRE ATT&CK labels
3. CI validates rule syntax and enforces version bumps on modifications

## Fleet Management Server (`feat/fleet-server` branch)

### Overview
Centralized EDR fleet management server — agents enroll, send heartbeats, stream telemetry, receive rules and commands from the server. Dashboard at `https://edr.novasky.io`.

### Architecture
```
Agent (Windows Service) → HTTPS → Nginx (:443, Let's Encrypt) → Go backend (:8443) → PostgreSQL (fleet state)
                                                                                    → ClickHouse (telemetry — planned, currently PostgreSQL)
```

### Server Components
| Directory | Purpose |
|-----------|---------|
| `cmd/fleet-server/` | Server binary — serve, migrate, bootstrap commands |
| `internal/fleetserver/` | Server core — routing, middleware, config |
| `internal/fleetserver/handler/` | HTTP handlers — agent, auth, command, detection, enrollment, rule, telemetry |
| `internal/fleetserver/store/postgres/` | PostgreSQL store implementations |
| `internal/fleetserver/ca/` | Per-org Certificate Authority for mTLS enrollment |
| `internal/fleetserver/ctxutil/` | Request context helpers (org/user/agent identity) |
| `internal/fleetserver/fleetauth/` | JWT + bcrypt auth utilities |
| `web/dashboard/` | React 18 + TypeScript + Vite + Tailwind dashboard |
| `deploy/` | Install script, Docker Compose, Nginx config |

### Agent Fleet Components
| Directory | Purpose |
|-----------|---------|
| `pkg/fleet/` | Shared types (Agent, Detection, Command, Rule, etc.) |
| `pkg/fleetclient/` | Fleet client — registration, heartbeat, commands, rule sync |
| `pkg/fleetclient/commands.go` | Command polling and execution loop |
| `pkg/fleetclient/executor_windows.go` | Windows command executor (isolate, kill, browse, shell) |
| `pkg/fleetclient/rulesync.go` | Rule sync with ETag caching |
| `pkg/outputs/fleetserver/` | Telemetry output sink — streams kernel events to server |
| `pkg/alertsender/fleetserver/` | Alert sender — forwards detections to server |
| `cmd/fibratus/app/enroll/` | `fibratus enroll` CLI command |

### Data Storage (Current: PostgreSQL, Telemetry moving to ClickHouse)
- **PostgreSQL**: accounts, organizations, users, agents, agent_groups, rules, global_rules, detections, commands, enrollment_tokens, org_cas
- **ClickHouse** (planned): telemetry_events — high-volume kernel event storage with columnar compression

### Agent Enrollment Flow
1. Admin creates enrollment token in dashboard (Settings page)
2. On endpoint: `fibratus enroll --token <TOKEN> --server https://edr.novasky.io`
3. Agent generates RSA key pair + CSR, server signs with org CA
4. Certs + agent-id + org-id + server-url saved to `data/` directory
5. On next service start, agent auto-detects enrollment data — zero config needed
6. Agent registers, starts heartbeat (30s), rule sync (5m), command polling (5s), telemetry streaming

### Active Response Commands
`isolate`, `unisolate`, `kill_process`, `list_directory`, `get_file`, `run_command`, `collect_info`, `uninstall`

### Dashboard Pages
Overview, Agents (detail + active response + terminal + command history), Detections, Events (live stream), Rules, Settings (enrollment tokens, orgs)

### Deployment
- Install script: `sudo bash deploy/install-fleet-server.sh` (interactive: domain, TLS mode)
- Nginx reverse proxy handles TLS, serves dashboard static files
- Go backend on localhost:8443
- Systemd service: `fibratus-fleet`
- Let's Encrypt auto-renewal via certbot timer

### Key Design Decisions
- **ClickHouse for telemetry** — columnar storage optimized for high-volume time-series event data with excellent compression
- **PostgreSQL for fleet state** — relational data (accounts, orgs, agents, rules)
- **Nginx reverse proxy** — TLS termination, static file serving, no setcap/port-binding issues
- **Enrollment-based auth** — agents get identity from enrollment, not config files
- **Agent auto-detection** — reads enrollment data from disk, enables fleet mode automatically
