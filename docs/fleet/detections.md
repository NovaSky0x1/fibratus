# Detections

The detection pipeline provides centralized visibility into all rule matches across the fleet. When an agent's rule engine matches an event, the detection is forwarded to the server and displayed in the dashboard.

## Detection Flow

1. **Agent** processes ETW kernel events through the rule engine
2. **Rule match** triggers the fleet alert sender (non-blocking queue)
3. **Server** receives the detection, applies rate limiting (20/min per agent)
4. **PostgreSQL** stores the detection record with full event context
5. **Dashboard** displays the detection on the Detections page with severity, alert text, and process tree

## Detection Data

Each detection record contains:

| Field | Description |
|-------|-------------|
| **Rule Name** | The detection rule that matched |
| **Rule ID** | UUID of the rule |
| **Severity** | low, medium, high, or critical |
| **Alert Text** | Rendered alert template with event data substituted |
| **Agent** | Hostname and agent ID of the source endpoint |
| **Timestamp** | When the detection occurred |
| **Event Data** | Full event that triggered the detection (JSON) |
| **Process Context** | Process name, PID, command line, parent chain, hashes |
| **MITRE ATT&CK** | Tactic and technique from rule labels |
| **Tags** | Rule tags for categorization |

## Rate Limiting

To prevent alert fatigue, detections are rate-limited:

- **Default limit**: 20 detections per minute per agent
- **Burst handling**: Excess detections are dropped at the server
- **Condition formatting**: Complex conditions are formatted with line breaks at depth 3 for readability

## Detections Dashboard

### List View

The main Detections page shows all detections across the fleet:

- **Severity badges** — color-coded (critical=red, high=orange, medium=yellow, low=blue)
- **Search** — filter by rule name, agent hostname, alert text
- **Time range** — filter by detection time
- **Rule linking** — click rule name to navigate to the Rules page
- **Agent linking** — click agent hostname to navigate to the Agent detail page

### Detail Panel

Clicking a detection opens a slide-out panel with:

- **Full alert text** — rendered with line breaks
- **Event parameters** — all event fields that triggered the detection
- **Process information** — name, PID, command line, path, hashes, signing status
- **Parent process** — parent name, PID, path, hashes
- **MITRE ATT&CK mapping** — tactic and technique details
- **Raw event JSON** — expandable raw event data

### URL State

Detection selection is preserved in the URL, so:
- Browser back button returns to the selected detection
- Deep links to specific detections work (e.g., `/detections?id=<uuid>`)

## Detection Process Tree

From the detection detail panel, you can view the process tree:

- **Process chain** — visual representation of the process ancestry
- **Activity events** — kernel events associated with each process in the chain
- **Category grouping** — events grouped by type (process, file, network, registry, DNS)
- **Click to expand** — click any process node to see its events
- **Full-screen view** — navigate to the dedicated process tree page

See [Process Tree](fleet/process-tree.md) for detailed documentation on the visualization.

## Async Detection Delivery

Detections are delivered asynchronously to prevent blocking the agent's event pipeline:

1. Agent rule engine matches an event
2. Alert is queued in a non-blocking channel
3. Fleet alert sender forwards to server with timeout
4. Server acknowledges receipt
5. If server is unreachable, alert is dropped (agent continues operating)

This design ensures that detection delivery never impacts the agent's core ETW event processing.

## Cross-Organization View

When viewing "All Organizations":
- Detections from all organizations are aggregated
- Organization column appears in the table
- Process tree works across organization boundaries
- Useful for security teams monitoring multiple tenants

## Noisy Rules

The server tracks rules that generate excessive detections:

- **Noisy Rules API** — query which rules are generating the most alerts
- **Per-org tracking** — noisy rule counts are organization-scoped
- **Tuning tab** — the Rules page includes a tuning tab for identifying and managing noisy rules
- **SIGMA rules** — known-noisy SIGMA rules are disabled by default after conversion
