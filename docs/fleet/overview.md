# Fleet Server Overview

Fibratus Fleet Server is a centralized EDR (Endpoint Detection and Response) fleet management platform. It transforms Fibratus from a standalone endpoint tool into a fully managed, multi-tenant security operations platform. Agents deployed on Windows endpoints enroll with the server, stream kernel-level telemetry, receive detection rules, and respond to active response commands — all managed through a modern web dashboard.

## Architecture

**Server Side (Linux)**

- **Nginx** (:443) — TLS termination (Let's Encrypt), serves dashboard static files, proxies API and gRPC
- **Go Backend** (:8443 HTTP, :8444 gRPC) — business logic, auth, API
- **PostgreSQL** — fleet state (accounts, orgs, users, agents, rules, detections, commands)
- **ClickHouse** — high-volume telemetry event storage (per-org tables, columnar compression)

**Agent Side (Windows)**

- **ETW Kernel Trace** — Process, Thread, File I/O, Registry, Network, DNS, Image/Module loading, Handle, Memory, Windows Event Log
- **Detection Engine** — YAML rules (server-synced), YARA memory scanning, evasion detection (direct/indirect syscall)
- **Fleet Client** — Heartbeat (30s), Rule sync (5m, ETag), Command polling (5s), Telemetry streaming

**Communication**: Agent connects to server via HTTPS (port 443) and gRPC (port 443, Nginx-proxied). All traffic TLS-encrypted.

## Core Components

### Server Side

| Component | Purpose |
|-----------|---------|
| **Go Backend** | HTTP REST API + gRPC agent service, handles all business logic |
| **PostgreSQL** | Stores fleet state: accounts, organizations, users, agents, rules, detections, commands, audit log |
| **ClickHouse** | Columnar storage for high-volume telemetry events with per-org tables and configurable retention |
| **Nginx** | TLS termination (Let's Encrypt), serves dashboard static files, reverse proxies API and gRPC |
| **React Dashboard** | Single-page application for fleet management, investigation, and response |

### Agent Side

| Component | Purpose |
|-----------|---------|
| **Fleet Client** | Manages enrollment, heartbeat, rule sync, command execution, telemetry streaming |
| **Telemetry Output** | Streams security-relevant kernel events to the server via the aggregator pipeline |
| **Alert Sender** | Forwards detection alerts to the server for centralized visibility |
| **Command Executor** | Executes active response commands (isolate, kill, browse, shell, capture) |
| **DPAPI Storage** | Encrypts enrollment credentials and in-memory rules using Windows DPAPI |

## Key Capabilities

### Multi-Tenant Fleet Management
- **Account hierarchy**: Accounts → Organizations → Agents
- **Cross-org views**: Aggregate data across all organizations from a single pane
- **Per-org isolation**: Each organization gets its own ClickHouse telemetry table and rule set

### Real-Time Telemetry
- All security-relevant kernel events stream to ClickHouse in real time
- Events include: process creation/termination, file I/O, registry operations, network connections, DNS queries, DLL loading, memory operations, Windows Event Log entries
- SHA256/MD5 hashing for all files and processes
- Process ancestry chain and callstack data included with events
- Fibratus QL query language translates directly to SQL for powerful searching

### Centralized Detection
- Rules managed server-side, synced to agents automatically
- Server-side rule validation with the real Fibratus QL parser
- SIGMA rule conversion and SigmaHQ community integration
- GitHub sync for Detection-as-Code workflows
- Detection rate limiting to prevent alert fatigue

### Active Response
- Network isolation via Windows Filtering Platform (WFP)
- Remote process termination
- Remote file system browsing and file retrieval
- Remote command shell
- System information collection (processes, services, drivers, software, users, network adapters, AV products, RMM tools)
- Live kernel event captures with export to JSON/CSV/kcap
- Agent self-update from GitHub releases
- Remote uninstall

### Enterprise Security
- JWT authentication with bcrypt password hashing
- Mandatory TOTP 2FA for all users
- Group-based RBAC with 60 granular permissions
- Account lockout and rate limiting
- API key authentication for CI/CD integration
- DPAPI-encrypted credential storage on endpoints
- Tamper protection engine
- CMMC/HIPAA compliant file access policies

## Data Flow

### Telemetry Pipeline

**Agent**: ETW Kernel Trace > Event Processors > Aggregator > Fleet Telemetry Output (gRPC stream)

**Server**: gRPC/NATS Telemetry Handler > ClickHouse (per-org table)

**Dashboard**: Events Page > Fibratus QL query > translated to SQL WHERE > ClickHouse query

### Detection Pipeline

**Agent**: ETW Events > Rule Engine > Match > Fleet Alert Sender (async queue)

**Server**: Detection Handler > Rate Limiter > PostgreSQL

**Dashboard**: Detections Page > Filtering by severity/rule > Detail Panel with process tree

### Command Pipeline

**Dashboard**: User action > REST API > Command Record in PostgreSQL (status: pending)

**Agent**: Command poll (5s) > Execute > Result > API > Update record (status: completed)

**Dashboard**: Command History with live result display

## Technology Stack

| Layer | Technology |
|-------|-----------|
| **Backend** | Go 1.26, gRPC + protobuf, JWT, bcrypt, TOTP |
| **Frontend** | React 18, TypeScript, Vite, Tailwind CSS, React Query, React Flow, Recharts |
| **Databases** | PostgreSQL (fleet state), ClickHouse (telemetry) |
| **Transport** | gRPC/protobuf (agent↔server), NATS (telemetry pipeline) |
| **Infrastructure** | Nginx, Let's Encrypt, systemd |
| **Agent** | Windows Service, ETW, DPAPI, WFP, Windows Event Log API |
| **Packaging** | WiX MSI installer, GitHub Releases |
