# SIGMA Rule Integration

The Fleet Server includes a built-in SIGMA to Fibratus rule converter and optional SigmaHQ community rule integration. This allows security teams to leverage the extensive SIGMA rule library alongside native Fibratus detection rules.

## SIGMA Converter

### Overview

The converter translates SIGMA detection rules into native Fibratus YAML rules:

```
SIGMA YAML → Parser → Field Mapping → Condition Generation → Fibratus YAML
```

### Conversion Example

**SIGMA rule (input):**
```yaml
title: Suspicious PowerShell Download Cradle
status: experimental
logsource:
  category: process_creation
  product: windows
detection:
  selection:
    Image|endswith: '\powershell.exe'
    CommandLine|contains|all:
      - 'Net.WebClient'
      - 'DownloadString'
  condition: selection
level: high
tags:
  - attack.execution
  - attack.t1059.001
```

**Fibratus rule (output):**
```yaml
name: Suspicious PowerShell Download Cradle
id: <generated-uuid>
version: 1.0.0
description: Suspicious PowerShell Download Cradle
labels:
  tactic.id: TA0002
  tactic.name: Execution
  technique.id: T1059.001
  technique.name: Command and Scripting Interpreter - PowerShell
condition: >-
  spawn_process and
  (ps.exe imatches '*\powershell.exe' and
   ps.cmdline icontains 'Net.WebClient' and
   ps.cmdline icontains 'DownloadString')
severity: high
min-engine-version: 3.0.0
output: >-
  Suspicious PowerShell Download Cradle detected on %ps.exe
  (cmdline=%ps.cmdline)
```

### Field Mapping

| SIGMA Field | Fibratus Equivalent |
|-------------|-------------------|
| `Image` | `ps.exe` |
| `OriginalFileName` | `ps.name` |
| `ParentImage` | `ps.parent.exe` |
| `CommandLine` | `ps.cmdline` |
| `ParentCommandLine` | `ps.parent.cmdline` |
| `User` | `ps.username` |
| `TargetFilename` | `file.name` |
| `TargetObject` | `registry.key.name` |
| `Details` | `registry.value` |
| `DestinationIp` | `net.dip` |
| `DestinationPort` | `net.dport` |
| `SourceIp` | `net.sip` |
| `SourcePort` | `net.sport` |
| `QueryName` | `dns.name` |
| `Hashes` | `ps.exe.sha256` |
| `md5` | `ps.exe.md5` |
| `sha256` | `ps.exe.sha256` |
| `ImageLoaded` | `module.name` |

### Modifier Support

| SIGMA Modifier | Fibratus Translation |
|---------------|---------------------|
| `contains` | `icontains 'value'` |
| `startswith` | `imatches 'value*'` |
| `endswith` | `imatches '*value'` |
| `re` | `matches 'regex'` |
| `all` | Multiple conditions joined with `and` |
| `windash` | Handles `-`/`/` dash variants automatically |
| `base64` | Not yet supported |
| `base64offset` | Not yet supported |

### Logic Conversion

| SIGMA Logic | Fibratus Logic |
|-------------|---------------|
| `selection` | Direct condition mapping |
| `selection1 or selection2` | `(selection1) or (selection2)` |
| `selection and not filter` | `(selection) and not (filter)` |
| `1 of selection*` | `(sel1) or (sel2) or ...` |
| `all of selection*` | `(sel1) and (sel2) and ...` |

### Negation Handling

SIGMA `not` operator requires careful precedence handling:
- `not` is scoped with parentheses: `(true and not (expr))`
- Prevents false matches from incorrect operator binding
- String-literal aware formatting preserves quoted strings

### Condition Formatting

Converted conditions are formatted for readability:
- Line breaks at logical depth 3
- Proper indentation
- Parentheses for operator precedence
- String literals preserved intact

## SigmaHQ Community Integration

### Enabling SigmaHQ

Toggle SigmaHQ integration from the Management → Rules page:

1. Enable the SigmaHQ toggle
2. The server clones/updates the SigmaHQ community repository
3. Rules are automatically converted to Fibratus format
4. Converted rules appear with a "sigma" source badge

### Noisy Rule Handling

Known-noisy SIGMA rules are handled automatically:
- Rules that generate excessive false positives are disabled by default after conversion
- The Tuning tab on the Rules page identifies noisy rules
- Administrators can selectively enable/disable converted rules

### Rule Sources

The Rules page tabs show rule sources:

| Tab | Description |
|-----|-------------|
| **All** | All rules from all sources |
| **Official** | Built-in Fibratus rules |
| **SIGMA** | Converted SIGMA rules |
| **Custom** | User-created rules |
| **GitHub** | GitHub-synced rules |
| **Tuning** | Noisy rule analysis |

Each rule shows a source badge for easy identification.

## Validation

Converted SIGMA rules go through the same validation pipeline as native rules:
1. YAML schema validation
2. Condition parsing with the Fibratus QL parser
3. Macro expansion (if referenced)
4. Engine version compatibility check

Rules that fail conversion or validation are flagged with errors.
