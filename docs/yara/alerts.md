# Match Results

In the fleet build YARA does not emit alerts to the detection pipeline or alert-sender outputs — matches are only surfaced as the response to an on-demand scan command.

## Response schema

The `yara_scan` command returns a JSON object:

```json
{
  "pid": 1234,
  "path": "",
  "matches": [
    {
      "Rule": "Mimikatz_Markers",
      "Namespace": "default",
      "Tags": ["credential-access", "mimikatz"],
      "Metas": [
        { "Identifier": "threat_name", "Value": "Mimikatz" },
        { "Identifier": "severity",    "Value": "critical" },
        { "Identifier": "score",       "Value": 95 }
      ],
      "Strings": [
        { "Name": "$m1", "Offset": 4096, "Data": "bWltaWthdHo=" },
        { "Name": "$m2", "Offset": 9284, "Data": "c2VrdXJsc2E6OmxvZ29ucGFzc3dvcmRz" }
      ]
    }
  ],
  "match_count": 1,
  "scanned_at": "2026-04-24T18:00:00Z"
}
```

`Strings[].Data` is base64-encoded in transit (JSON can't represent raw binary). The dashboard decodes it to printable ASCII where possible and falls back to `\xHH` escapes for non-printable bytes.

## No automatic detection entries

A match does **not** create a row in the `detections` table. Unlike upstream Fibratus, there is no `File Threat Detected` / `Memory Threat Detected` alert-sender wiring in this build — the operator who initiated the scan is the only consumer of the result.

To turn a scan result into a persistent investigation record, copy the match metadata into a detection manually, or feed the scan output to a SIEM through the server's audit log (every executed scan is logged with operator identity, target, and command ID).

## Why the design change

Inline YARA scanning against the event stream generated ~800 matches per hour on a typical dev workstation, almost entirely from normal Windows process startup reading its own PEB or being a PE image. The on-demand-only model keeps YARA useful for investigation without the alert storm.

See [Pattern Matching Swiss Knife](introduction.md) for the broader rationale.
