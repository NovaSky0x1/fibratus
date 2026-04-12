# Rule Management

The Fleet Server provides centralized rule management — rules are authored, validated, and stored on the server, then automatically synced to all enrolled agents. This enables Detection-as-Code workflows with GitHub integration, SIGMA rule conversion, and server-side validation.

## Rule Lifecycle

1. **Author rule** — write YAML rule with Fibratus QL condition
2. **Upload or sync** — upload via dashboard, or sync from GitHub repository
3. **Server-side validation** — QL parser validates condition syntax and macro expansion
4. **Store in DB** — rule saved in PostgreSQL with validation status
5. **Agent rule sync** — agents poll every 5 minutes (ETag caching), receive updated rules
6. **In-memory compilation** — agent compiles rules in DPAPI-encrypted memory (never written to disk)
7. **Rule engine evaluation** — compiled rules evaluate against live ETW events
8. **Detection** — matches are forwarded to the server as detection alerts

## Rule Storage

Rules are stored in PostgreSQL with the following attributes:

| Field | Description |
|-------|-------------|
| **ID** | UUID, auto-generated or from YAML |
| **Name** | Descriptive rule name |
| **Version** | Semantic version (e.g., 1.0.0) |
| **Description** | What the rule detects |
| **Condition** | Fibratus QL filter expression |
| **Output** | Alert template string with variable substitution |
| **Severity** | low, medium, high, or critical |
| **Labels** | MITRE ATT&CK tactic/technique mapping |
| **Tags** | Categorization tags |
| **References** | External references (URLs, CVEs) |
| **Min Engine Version** | Minimum Fibratus version required |
| **Enabled** | Whether the rule is active |
| **Source** | official, sigma, github, or custom |
| **Raw YAML** | Original YAML content |
| **Validation Status** | valid, invalid, or pending |
| **Organization** | Scoped to specific org |
| **Account** | Account-wide rules available to all orgs |

## Rule Operations

### Upload Rules

Individual rules or bulk uploads via the dashboard:

1. **Single rule**: Paste YAML in the editor, validate, and save
2. **File upload**: Upload one or more `.yml` files
3. **Folder upload**: Upload an entire directory of rules
4. **ZIP upload**: Upload a ZIP file containing rules

### Edit Rules

The Rules page includes a YAML editor with:
- Syntax highlighting
- **Two-step validate-then-save** workflow
- Real-time validation feedback showing Valid/Invalid badge
- Condition extraction and QL parser checking
- Macro expansion validation

### Delete Rules

Individual or bulk rule deletion with confirmation dialog.

### Download Rules

- **Download All**: Downloads all rules as a ZIP file
- Individual rule YAML download

### Enable/Disable Rules

Toggle individual rules on/off without deleting them.

## Server-Side Validation

The Fleet Server includes a full Fibratus QL parser for Linux-based validation:

### Validation Process

1. **YAML schema check** — validates structure against the rule JSON Schema
2. **Condition parsing** — parses the condition through the Fibratus QL lexer and parser
3. **QL escape checking** — validates proper escaping in the condition expression
4. **Macro expansion** — resolves macro references and validates the expanded condition
5. **Engine version check** — ensures min-engine-version is compatible

### Validation API

The validation API is available for external tools:

```bash
# Validate a single rule
curl -X POST https://server/api/v1/orgs/{orgId}/rules/validate \
  -H "Authorization: Bearer <JWT>" \
  -H "Content-Type: application/json" \
  -d '{"raw_yaml": "name: Test Rule\ncondition: spawn_process\n..."}'

# Validate with macros
curl -X POST https://server/api/v1/orgs/{orgId}/rules/validate \
  -H "Authorization: Bearer <JWT>" \
  -H "Content-Type: application/json" \
  -d '{"raw_yaml": "...", "macros": [{"name": "macro1", "expr": "..."}]}'
```

### API Key Authentication

For CI/CD integration, use API keys instead of JWT:

```bash
curl -X POST https://server/api/v1/orgs/{orgId}/rules/validate \
  -H "X-API-Key: <API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"raw_yaml": "..."}'
```

Use cases:
- **VS Code integration** — validate rules in your editor
- **Pre-commit hooks** — validate rules before committing
- **CI pipelines** — validate rules in GitHub Actions

### Validate All

The "Validate All" button re-validates every rule in the organization, showing an inline progress indicator.

## GitHub Sync (Detection as Code)

Sync rules directly from GitHub repositories:

### Configuration

Set up GitHub sync in the Management → Rules tab:

1. **Repository URL** — GitHub repository containing rules (HTTPS or API URL)
2. **Path** — subdirectory within the repo (optional)
3. **Branch** — branch to sync from (default: main)
4. **Token** — GitHub personal access token for private repos
5. **Scope** — account-wide or organization-specific

Multiple sync sources are supported — you can sync from several repositories simultaneously.

### Sync Behavior

- **Automatic sync** — rules are synced periodically
- **Manual trigger** — click "Sync Now" in the dashboard
- **Recursive scanning** — subdirectories are scanned recursively for YAML files
- **Clean sync** — rules removed from the repo are deleted from the server
- **Local edit preservation** — locally modified rules are not overwritten
- **Macro sync** — macros in the repository are synced alongside rules
- **Git Tree API** — uses the Git Tree API to avoid GitHub API rate limits

### Auto-Convert URLs

GitHub web URLs are automatically converted to API URLs:
```
https://github.com/org/repo/tree/main/rules
  → https://api.github.com/repos/org/repo/git/trees/main?recursive=1
```

## SIGMA Rule Converter

The Fleet Server includes a built-in SIGMA to Fibratus rule converter:

### Conversion Process

1. **Parse SIGMA YAML** — extract detection logic, logsource, level, tags
2. **Map fields** — convert SIGMA field names to Fibratus field names
3. **Convert operators** — translate SIGMA detection modifiers to Fibratus QL
4. **Generate condition** — build Fibratus QL condition expression
5. **Map ATT&CK** — extract MITRE ATT&CK tags to Fibratus labels
6. **Format output** — generate valid Fibratus YAML rule

### Field Mappings

| SIGMA Field | Fibratus Field |
|-------------|---------------|
| `Image` | `ps.exe` |
| `ParentImage` | `ps.parent.exe` |
| `CommandLine` | `ps.cmdline` |
| `ParentCommandLine` | `ps.parent.cmdline` |
| `TargetFilename` | `file.name` |
| `TargetObject` | `registry.key.name` |
| `DestinationIp` | `net.dip` |
| `DestinationPort` | `net.dport` |
| `Hashes` | `ps.exe.sha256` |
| `User` | `ps.username` |

### Modifier Support

| SIGMA Modifier | Fibratus Equivalent |
|---------------|-------------------|
| `contains` | `icontains` |
| `startswith` | `imatches 'value*'` |
| `endswith` | `imatches '*value'` |
| `re` | `matches 'regex'` |
| `windash` | Handles `-`/`/` dash variants |
| `all` | `and` conjunction |
| `base64` | (not yet supported) |

### SigmaHQ Integration

Toggle the SigmaHQ community rules integration:

1. Enable SigmaHQ in the settings
2. The server clones/updates the SigmaHQ repository
3. SIGMA rules are converted to Fibratus format
4. Known-noisy rules are disabled by default
5. Converted rules appear with a "sigma" source badge

## Rule Sync to Agents

### Sync Protocol

1. Agent polls `/api/v1/orgs/{orgId}/rules` every 5 minutes
2. Request includes `If-None-Match` header with the previous ETag
3. If rules haven't changed, server returns `304 Not Modified`
4. If rules have changed, server returns full rule set with new ETag
5. Agent compiles rules in-memory using the Fibratus rule engine

### In-Memory Rule Storage

Rules are loaded directly into memory on the agent — never written to disk:

- **DPAPI encryption** — rules are encrypted in memory using Windows DPAPI
- **No disk artifacts** — detection logic never touches the filesystem
- **Hot compilation** — new rules are compiled without service restart
- **Panic recovery** — rule compilation errors are caught and logged without crashing
- **Macro preservation** — in-memory macros are preserved across sync cycles

### Macro Sync

Macros are synced alongside rules:
- Macros are delivered in YAML format
- The agent writes macros to the macros directory
- Rule compilation references macros for condition expansion

## Rules Dashboard

### Tabbed Interface

| Tab | Description |
|-----|-------------|
| **All** | All rules regardless of source |
| **Official** | Fibratus built-in rules |
| **SIGMA** | Converted SIGMA community rules |
| **Custom** | User-created rules |
| **GitHub** | Rules synced from GitHub |
| **Tuning** | Noisy rules analysis |

### Features

- **Search** — filter rules by name, condition, tags, with highlighting
- **Source badges** — visual indicator of rule origin (official/sigma/github/custom)
- **Severity badges** — color-coded severity indicators
- **Enable/Disable toggle** — quick enable/disable without editing
- **Validation status** — Valid/Invalid/Pending badge per rule
- **Bulk operations** — validate all, download all, bulk upload

## Default Rules and Macros

When a new account or organization is created:
- **Default macros** — standard Fibratus macros are seeded (e.g., `web_browser_binaries`, `office_binaries`)
- **Official rules** — built-in Fibratus detection rules are seeded
- Rules and macros are seeded per-organization for proper isolation

## Account-Wide Rule Management

Rules can be managed at the account level:
- **Account-scoped rules** — available to all organizations in the account
- **Org-scoped rules** — specific to one organization
- **GitHub sync scope** — sync sources can be account-wide or org-specific
