# LinkedIn Post — Fibratus Fleet release

I'm releasing **Fibratus Fleet** — a fully open-source EDR platform for Windows, self-hosted, multi-tenant, with everything you'd expect from a commercial endpoint product and nothing locked behind a license.

This started as a question: Nedim Šabić Šabić's [Fibratus](https://github.com/rabbitstack/fibratus) is one of the most impressive Windows kernel-level detection engines I've ever used — ETW telemetry, MITRE-mapped YAML rules, in-memory YARA, sequence detections, kernel captures — but it runs **per host**. There was no way to manage a fleet of Windows endpoints with it.

So I built one.

**Fibratus Fleet adds:**

🛡️ **Centralized server, multi-tenant by default**
Self-hosted Go backend on Ubuntu Linux. PostgreSQL for fleet state, ClickHouse for telemetry. Accounts → organisations → users with full RBAC. Optional 2FA enforcement per account. Optional self-service open signup for public previews.

🧠 **Deep Windows kernel telemetry investigations**
Per-org ClickHouse tables with TTL-driven retention and a buffered ingest layer. Switch between local self-hosted ClickHouse and managed ClickHouse Cloud at runtime — drains the buffer, opens a fresh connection, swaps atomically, no service restart.

⚡ **Active response commands, on demand from the dashboard:**
- **Containment:** isolate / unisolate (via Windows Filtering Platform host firewall, with always-allow whitelist for management traffic), kill_process, set_tamper_protection
- **Forensic acquisition:** start_capture / stop_capture (live Windows kernel `.kcap` files), get_file (with CMMC/HIPAA-compliant file-extension allowlist), list_directory, export_evtx
- **Live triage:** run_command (PowerShell), collect_info, get_processes, get_network, get_services, get_drivers, get_autoruns, get_software, get_users, get_registry, query_eventlog, list_eventlog_channels
- **Detection:** yara_scan (server-managed YARA rule set, no client-side rule files), set_eventlog_policy
- **Lifecycle:** update_agent, uninstall, logoff_user

🌳 **Process tree investigations**
Walk the parent/child graph of every process the agent has observed — joined with the detection that fired, joined with every kernel event each node emitted. Click any node to pivot across the entire fleet.

📜 **Detection-as-code workflows**
Wire a GitHub repo + branch + path; the server polls and imports rules with macro-aware validation. Plus native [SIGMA](https://github.com/SigmaHQ/sigma) integration — paste a Sigma rule, the server converts it to Fibratus QL and ships it to your fleet.

🔒 **Tamper protection that actually heals**
Service ACLs locked to SYSTEM. Enrollment data DPAPI-encrypted in registry. Re-enrollment self-heals the lockdown via take-ownership flow — no manual cleanup needed when an operator re-binds a host to a different account.

🧬 **Encrypted secret store on the server**
Every persisted secret (ClickHouse passwords, Cloud API keys) is sealed with AES-256-GCM under a master key that never lives in YAML.

📊 **The dashboard**
React 18 + TypeScript + Vite + Tailwind. Detection triage, process tree, live SIEM-style event view, agent detail with active response, remote shell, remote file browser, Windows Event Log management, detector creation, Git-synced rules — all in one UI.

---

**Built on:**
- 🙏 [Fibratus](https://github.com/rabbitstack/fibratus) by Nedim Šabić Šabić — the kernel engine that powers the entire agent
- 🙏 [SIGMA](https://github.com/SigmaHQ/sigma) by Florian Roth and the SigmaHQ community — the generic detection rule format

**Project:** https://github.com/NovaSky0x1/fibratus
**Branch:** `feat/fleet-server`

If you've ever wanted a real EDR you can self-host, audit end to end, run multi-tenant for your customers, and tweak to fit your environment — clone it, deploy it, break it, and tell me what's missing.

#OpenSource #EDR #Windows #DFIR #BlueTeam #Cybersecurity #DetectionEngineering #ETW #YARA #SIGMA #InfoSec
