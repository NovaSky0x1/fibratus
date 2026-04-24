# Running an On-Demand YARA Scan

A scan is triggered from the fleet dashboard, routed through the gRPC command channel to a specific agent, executed with the account's server-managed rule set, and returned inline to the dashboard.

## Dashboard flow

1. Navigate to **Agents** and click into the target endpoint.
2. Open the **YARA Scan** tab (under *Respond* in the agent sidebar).
3. Choose the target type:
   - **Process (PID)** — a decimal PID on the endpoint. The scan reads the process's memory via `libyara`'s `yr_scanner_scan_proc`.
   - **File / Directory** — absolute path on the endpoint. Files are read via `yr_scanner_scan_file`; directory paths are walked recursively by libyara.
4. Optionally narrow the rule set using the checkbox list. Empty selection means "every enabled, valid rule in the account."
5. Click **Run scan**. The dashboard polls the command result for up to 60 seconds.

## Result format

Matches are displayed as cards with:

- **Rule name + namespace** (e.g., `Mimikatz_Markers [default]`)
- **Tags** as small pill badges
- **Metadata** key/value pairs (`threat_name`, `severity`, `score`, `author`, etc.)
- **Matched strings** — identifier (`$s1`), hex offset, and the matched bytes (decoded to printable ASCII or `\xHH` escapes for non-printable)

No matches surface as a single green "No matches" panel with the rule count that was applied.

## Under the hood

```
POST /api/v1/orgs/{org}/agents/{agent}/commands
    { "type": "yara_scan", "payload": { "pid": 1234 } }
          │
          ▼
   server.CreateCommand:
     - fetch account's enabled yara_rules (optionally filtered by rule_ids)
     - concatenate rule content into one rules_yara blob
     - embed in command payload
          │
          ▼
   gRPC stream push to agent ──▶ WindowsExecutor.yaraScan
                                      │
                                      ▼
                             Scanner.ScanTargetInline:
                               - yara.NewCompiler()
                               - AddString(rules_yara, "inline")
                               - GetRules() -> yr_scanner
                               - scan target PID/path
                               - return MatchRules
                                      │
                                      ▼
                            Command result written back via gRPC
                                      │
                                      ▼
                         Dashboard polls getCommand(id) and renders matches
```

## Limits

| Aspect | Limit |
|---|---|
| Scan wall-clock timeout | 120 seconds (agent-side) |
| Dashboard poll window | 60 seconds (150 × 400 ms) |
| Target types | `uint32` PID, file path, directory path |
| Inline rules in payload | No explicit size cap; bounded by the command row size in Postgres |
| Rule compile errors | Returned as a command error — dashboard surfaces the libyara message |

## Supported rule syntax

Any YARA 4.3 rule that compiles with the agent's libyara build. Modules available at scan time: `pe`, `elf`, `hash`, `math`, `magic`, `dotnet`, `cuckoo`. Rules that `import` unavailable modules fail to compile and the scan returns a single `yara scan failed: …` error with the compiler message, leaving other rules un-evaluated — split unrelated rules across separate dashboard entries to isolate compilation errors.

## Test paths

Once agents are enrolled and rules are authored, verify the pipeline with:

**Negative control** — `C:\Windows\System32\notepad.exe`. Expected: 0 matches; confirms the scan path is healthy and the rule set compiles.

**Positive test** — drop a marker string on the endpoint first:

```powershell
'ReflectiveLoader' | Out-File -FilePath 'C:\Users\Public\yara-test.txt' -Encoding ASCII -NoNewline
```

Scan `C:\Users\Public\yara-test.txt` with the baseline rules enabled. Expected: matches on `Meterpreter_Stage_Marker` and `Reflective_DLL_Loader` (both rules trigger on the `ReflectiveLoader` ASCII string). Remember to delete the test file afterwards.
