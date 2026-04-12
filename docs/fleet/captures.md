# Live Kernel Captures

The Fleet Server enables live kernel event captures on remote endpoints. Captures provide a forensic snapshot of system activity that can be viewed in real time, replayed, and exported for analysis.

## Overview

Live captures record kernel events directly on the agent and stream them to the dashboard for real-time display. Captures can be filtered using the Fibratus Query Language and exported to multiple formats.

## Capture Flow

```
Dashboard: Start Capture → API → Command to Agent
                                      │
Agent: Start ETW Capture → Filter Events → Buffer
                                      │
Dashboard: Poll Events → Display → Stop Capture
                                      │
Export: JSON / CSV / .kcap download
```

## Starting a Capture

From the Agent Detail page → **Captures** tab:

### Duration Options

| Option | Description |
|--------|-------------|
| **30 seconds** | Quick snapshot |
| **1 minute** | Short capture |
| **5 minutes** | Medium capture |
| **15 minutes** | Extended capture |
| **Manual** | Unlimited duration, stop manually |

### Event Type Filters

Filter which event types to capture:
- Process events
- File events
- Registry events
- Network events
- DNS events
- Image/DLL events
- Memory events

### Quick Filter Presets

Pre-configured filter combinations:
- **All Events** — capture everything
- **Process Activity** — process creation and termination
- **Network Activity** — connections, DNS queries
- **File System** — file creation, write, delete, rename
- **Registry** — key and value operations
- **Suspicious** — common attack patterns

### Custom Filters

Use Fibratus QL expressions for precise filtering:

```
# Capture only PowerShell network activity
ps.name = 'powershell.exe' and (evt.type = 'Connect' or evt.type = 'QueryDns')

# Capture file writes to temp directories
evt.type = 'WriteFile' and file.path imatches '*\\Temp\\*'

# Capture registry persistence
evt.type = 'SetValue' and registry.key.name icontains 'Run'
```

The filter engine uses the real Fibratus QL compiler (not a simplified subset), so all operators and functions are available.

## Real-Time Display

During capture, events are displayed in a terminal-style output:

### Color Coding
Events are color-coded by type, matching the local Fibratus console color palette:
- Process events: one color
- Network events: another color
- File events: another color
- And so on for each event type

### Event Format
Each event shows:
- Timestamp
- Event type
- Process name and PID
- Key parameters (file path, network destination, registry key, etc.)

### Browse Limit
The capture viewer displays up to 5000 events. Event type filtering is applied on the agent side to reduce data volume.

## Export Formats

### JSON
Export captured events as a JSON array:
- Full event data with all parameters
- Process context included
- Suitable for ingestion into other tools

### CSV
Export as comma-separated values:
- Flattened event structure
- Headers for common fields
- Importable into spreadsheets and SIEM tools

### .kcap (Fibratus Capture File)

Export as a native Fibratus capture file:
- Binary format matching the standard `.kcap` specification
- Can be replayed locally with `fibratus replay`
- Full event fidelity including all metadata

The .kcap writer is implemented in pure Go — no CGO or build tags required.

## Capture Architecture

### Pure-Go .kcap Writer

The capture file writer was reimplemented without CGO dependencies:
- No `cap` build tag needed
- Uses section constants inline to avoid import cycles
- Compatible with the existing `.kcap` reader for replay

### Agent-Side Filtering

Event type filters are applied on the agent before buffering:
- Reduces data volume sent to the server
- Uses the capture callback pattern to avoid import cycles
- Filter compilation happens on the agent with the full QL engine

### Capture Metadata

Each capture includes metadata:
- Capture ID (UUID)
- Agent ID
- Start/stop timestamps
- Filter expression used
- Event count
- Organization context

## Replay and Search

Captured events in the dashboard support:
- **Search** — filter captured events by text search
- **Event type filtering** — show only specific event types
- **Expandable detail** — click events for full parameter view

## Download Options

The download menu provides:
- **Download JSON** — JSON array of all captured events
- **Download CSV** — flattened CSV format
- **Download .kcap** — native capture file format

The download dropdown opens upward to prevent UI clipping at the bottom of the page.
