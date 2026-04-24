# YARA Scanning

[YARA](https://virustotal.github.io/yara/) is a pattern-matching engine designed to classify and identify malware samples. In Fibratus Fleet the YARA engine is wired exclusively into the **on-demand active-response path** — rules are authored on the fleet server and travel with each scan request. The agent does not run inline YARA matching against the event stream and does not load rules from disk.

## What this means in practice

- **Rules live on the server.** Authored in the dashboard's *YARA Rules* page (or pulled via GitHub sync from a rule repository), stored account-scoped in the `yara_rules` PostgreSQL table, and shared across every organization the account owns.
- **Every scan is driven from the dashboard.** An operator picks an agent, opens *YARA Scan*, targets a PID or file path, and optionally narrows the rule set with per-rule checkboxes. The server packages the selected rules into the `yara_scan` command payload.
- **The agent compiles rules fresh per scan.** `libyara` is statically linked into the agent binary; when a `yara_scan` command arrives the agent's executor compiles the rules carried in the payload, scans the target, and returns structured match results (rule name, namespace, tags, metadata, matched strings with offsets). Nothing is cached on disk.
- **No automatic detections from YARA matches.** Match results surface inline in the scan response viewable in the dashboard; they do not generate entries in the Detections tab.

## libyara modules

The agent is built with the common libyara modules available, so rules from public sets (Neo23x0 signature-base, Elastic protections-artifacts, Yara-Rules/rules) compile and execute:

| Module | Typical use |
|---|---|
| `pe` | `pe.imphash`, `pe.sections`, `pe.entry_point`, signature checks |
| `elf` | ELF header + section parsing |
| `hash` | `hash.md5 / sha1 / sha256 / crc32(offset, size)` |
| `math` | `math.entropy`, `math.mean`, string-frequency helpers |
| `magic` | `magic.mime_type`, `magic.type` (libmagic-backed) |
| `dotnet` | .NET assembly metadata |
| `cuckoo` | Cuckoo sandbox behaviour tags |

## Comparison with upstream Fibratus

Upstream Fibratus supports **inline** YARA scanning that runs on every process creation, image load, and PE write, and emits alerts (`Memory Threat Detected`, `File Threat Detected`) via the alert-sender pipeline. The fleet build intentionally does not wire the scanner into the event stream — on a typical workstation, inline YARA produces thousands of false positives per day against legitimate Windows binaries unless operators curate tightly-narrow rules. The on-demand-only model keeps YARA available for investigation without the noise cost.

If you need inline YARA for a specific deployment, the scanner struct is still constructed from config (`yara.enabled: true`) and only the event-listener registration is omitted. Re-enabling it is a one-line change in `internal/bootstrap/bootstrap.go` plus a narrow rule set maintained on-disk in `C:\Program Files\Fibratus\Rules\yara\`.

## Where to next

- [Authoring and managing YARA rules](fleet/yara-rules.md)
- [Running a YARA scan from the dashboard](fleet/active-response.md#yara-scan)
