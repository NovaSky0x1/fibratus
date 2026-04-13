# What is Fibratus?

[Fibratus](https://github.com/rabbitstack/fibratus) detects, protects, and eradicates advanced adversary tradecraft by scrutinizing
and asserting a wide spectrum of system events against a behavior-driven [rule engine](/filters/rules) and [YARA](/yara/introduction) memory scanner.

Events can also be shipped to a wide array of [output sinks](/outputs/introduction) or dumped to [capture](/captures/introduction) files for local inspection and forensics analysis. You can use [filaments](/filaments/introduction) to extend Fibratus with your own arsenal of tools and so leverage the power of the Python ecosystem.

In a nutshell, the Fibratus mantra is defined by the pillars of **realtime behavior detection**, **memory scanning**, and **forensics capabilities**.

## Fleet Management

Fibratus can be deployed at scale through [Fleet Server](/fleet/overview), a centralized EDR management platform. Fleet Server provides:

- **Centralized rule management** — rules authored on the server, automatically synced to all agents
- **Real-time telemetry** — kernel events streamed to ClickHouse for SIEM-style search and investigation
- **Active response** — network isolation, remote shell, file browsing, process termination, and live kernel captures from the dashboard
- **Enterprise security** — multi-tenant accounts, group-based RBAC with 60 permissions, mandatory 2FA, DPAPI-encrypted credentials
- **Auto-update** — agents self-update from GitHub Releases with tamper protection support
