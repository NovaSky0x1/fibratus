# YARA Rule Management

YARA rules are authored and managed on the fleet server. They are **account-scoped** — one rule set is shared across every organization in an account — and embedded inline in `yara_scan` active-response commands at the moment the scan is triggered. Agents do not sync the rule set in the background; rules live exclusively in the `yara_rules` PostgreSQL table.

## Dashboard page

Navigate to **YARA Rules** in the main sidebar (between *Rules* and *Macros*).

| Column | Meaning |
|---|---|
| Name | Rule identifier. Must be unique per account. |
| Source | `manual` for dashboard-authored, `github:<repo>` when synced from a GitHub sync config. |
| Status | `enabled` or `disabled`. Disabled rules are excluded from scans. |
| Validation | Green `valid` or red `invalid` (with the compiler message on hover). Invalid rules are force-disabled. |

### Authoring

Click **New rule** to open the editor:

- **Name** — unique rule identifier; matches the `rule NAME { … }` declaration inside the content (best practice).
- **Description** — free-form operator note, shown in rule cards.
- **Content** — the full `.yar` text including `import` statements and one or more `rule NAME { meta / strings / condition }` blocks. The editor seeds a `rule ExampleMarker { … }` template.
- **Enabled** — checkbox. If the syntax check fails on save the rule is force-disabled regardless of this toggle.

The server runs a lightweight structural check on save (balanced braces, presence of at least one `rule NAME` declaration). Deep validation — module availability, string syntax, condition semantics — happens at scan time on the agent via the real `libyara` compiler.

### Editing and deletion

- Click a row to open it in the editor.
- Saving flips `user_modified=true` on rules with a `github:*` source so subsequent GitHub syncs won't stomp the edit.
- The trash icon deletes after confirmation. For github-sourced rules, a deletion is recorded as a tombstone so the sync won't re-create it on the next cycle.

## GitHub sync

Reuses the existing **GitHub Sync** config page. For any configured repo:

- `.yml` / `.yaml` files → `rules` table (detection rules, unchanged).
- `.yar` / `.yara` files → `yara_rules` table (account-scoped).

Multi-rule `.yar` files — very common in public rule repositories — are split into individual DB rows by scanning for `rule NAME { ... }` declarations. The synced rule's `source` column is set to `github:<repo>`, and `description` gets an auto-filled pointer to the file of origin.

### Popular public rule sets

| Repository | Path | Count (approx.) |
|---|---|---|
| [Neo23x0/signature-base](https://github.com/Neo23x0/signature-base) | `yara` | 2000+ rules |
| [elastic/protections-artifacts](https://github.com/elastic/protections-artifacts) | `yara` | 500+ rules |
| [Yara-Rules/rules](https://github.com/Yara-Rules/rules) | subdir tree | 500+ rules |

Add a sync config pointing at any of these. Use a personal access token if you'll cross GitHub's unauthenticated rate limit of 60 requests per hour per IP (large repos typically need a token).

### Preservation semantics

| Event | Result |
|---|---|
| Re-sync with no repo changes | No DB writes (rule content unchanged) |
| Repo adds a new rule | Inserted as `source=github:<repo>`, `enabled=true`, `validation_status=valid/invalid` |
| Repo changes a rule's content | Overwritten — unless the rule has `user_modified=true`, in which case the re-sync is skipped for that row |
| Repo removes a rule | Deleted — unless `user_modified=true`, in which case the row is preserved |
| User deletes a github-sourced rule | Tombstone recorded; re-sync does not re-create it |

## Account-wide scope

Any rule created, edited, enabled, or disabled from any organization in an account takes effect across every agent in that account. The `yara_rules` table is keyed by `account_id`; queries in the agent path resolve the account from the agent's org at scan time.

## Baseline rule set

On first server startup each account is seeded with four narrow, threat-specific rules so the dashboard isn't empty:

- `Meterpreter_Stage_Marker` — score 75, severity high
- `Cobalt_Strike_Beacon_Strings` — score 85, severity high
- `Mimikatz_Markers` — score 95, severity critical
- `Reflective_DLL_Loader` — score 60, severity medium

The seed only runs for accounts with zero rules — existing accounts and accounts that imported from GitHub are untouched.

## Supported libyara modules

The agent binary ships with `pe`, `elf`, `hash`, `math`, `magic`, `dotnet`, and `cuckoo` modules compiled in. Rules like the following compile and execute at scan time:

```yara
import "hash"
import "pe"

rule KnownLoader
{
    meta:
        severity = "high"
        score = 80
    condition:
        pe.imphash() == "d41d8cd98f00b204e9800998ecf8427e"
        and hash.sha256(0, filesize) == "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
}
```

Rules that `import` an unavailable module will compile-fail at scan time; the scan response returns the libyara error message directly.

## API

All operations are account-scoped.

```
GET    /api/v1/account/yara-rules
POST   /api/v1/account/yara-rules
GET    /api/v1/account/yara-rules/{id}
PUT    /api/v1/account/yara-rules/{id}
DELETE /api/v1/account/yara-rules/{id}
POST   /api/v1/account/yara-rules/validate   (dry-run structural check)
```

Request body for create/update:

```json
{
  "name": "CustomBackdoor",
  "description": "Internal IR rule for threat actor X",
  "content": "rule CustomBackdoor { strings: $a = \"marker\" condition: $a }",
  "enabled": true
}
```

All endpoints require the `PermManageRules` permission (granted by default to the Administrators group).
