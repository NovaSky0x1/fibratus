# Process Tree

The process tree visualization provides interactive exploration of process hierarchies and their associated kernel events. It is used for investigating detections, understanding attack chains, and tracing process ancestry.

## Overview

The process tree renders a visual graph of process parent-child relationships using the React Flow library with automatic layout via dagre and ELK layout engines.

## Access Points

Process trees can be accessed from:

1. **Detection detail panel** — click "Process Tree" to see the detection's process chain
2. **Dedicated page** — full-screen process tree at `/process-tree`
3. **Agent detail page** — process tree tab for agent-specific investigation

## Tree Layout

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ explorer.exe │────▶│  cmd.exe    │────▶│ whoami.exe  │
│  PID: 1234   │     │  PID: 5678  │     │  PID: 9012  │
│  SYSTEM      │     │  admin      │     │  admin      │
└─────────────┘     └──────┬──────┘     └─────────────┘
                           │
                    ┌──────▼──────┐
                    │powershell.exe│
                    │  PID: 3456  │
                    │  admin      │
                    │             │
                    │ ┌─────────┐ │
                    │ │ Events: │ │
                    │ │ 🔵 Net 3│ │
                    │ │ 📁 File 7│ │
                    │ │ 🔑 Reg 2│ │
                    │ └─────────┘ │
                    └─────────────┘
```

## Process Nodes

Each process node displays:

| Field | Description |
|-------|-------------|
| **Process name** | Executable name |
| **PID** | Process ID |
| **Command line** | Truncated command line (expand for full) |
| **User** | Account running the process |
| **Hashes** | SHA256/MD5 of the executable |
| **Signing status** | Whether the binary is signed and trusted |

## Event Categories

Events associated with each process are grouped by category:

| Category | Icon | Events Included |
|----------|------|-----------------|
| **Process** | 🔵 | CreateProcess, TerminateProcess |
| **Network** | 🌐 | Connect, Accept, Send, Recv, DNS |
| **File** | 📁 | CreateFile, WriteFile, DeleteFile, RenameFile |
| **Registry** | 🔑 | CreateKey, SetValue, DeleteKey, DeleteValue |
| **Image** | 📦 | LoadImage (DLL loading) |
| **Memory** | 💾 | VirtualAlloc, VirtualFree |

Category cards show the count of events in each category. Click a category to expand and see all events.

## Interaction

### Click to Expand
- Click a process node to fetch and display its associated events
- Events appear as category cards within or below the node
- Click a category card to see individual events

### Event Detail
- Click an individual event to open a detail panel
- Shows all event parameters
- DNS events display the queried domain name
- Network events show source/destination IP and port
- File events show the file path
- Registry events show key name and value

### Navigation
- Pan by dragging the canvas
- Zoom with scroll wheel
- Fit-to-view button
- MiniMap for orientation in large trees

### Expand Ancestors
- Process ancestry is loaded recursively
- Missing parent processes are fetched from telemetry
- Synthetic nodes are created for parents not found in the current time window

## Data Retrieval

### Detection Process Tree
When viewing a detection's process tree:

1. Extract PIDs from the detection event data
2. Query ClickHouse for `CreateProcess` events to build the tree structure
3. Query activity events (file, network, registry, DNS) for each PID
4. Build the graph with parent-child edges

### Time Windows
- **Ancestry**: Wide time window (hours/days) to find parent processes
- **Activity**: Narrow time window around the detection for relevant events
- **PID-specific queries**: Events are queried by specific PIDs, not generic time ranges

### Fallback Logic
- If `CreateProcess` is missing for a PID, falls back to any event with that PID
- Ancestors are walked recursively until the tree root is found
- Event limits per PID are set to 5000 to capture all event types

## Layout Engines

### Dagre
Default layout engine for general process trees:
- Hierarchical left-to-right layout
- Spline edge routing to prevent line overlap
- Configurable node spacing

### ELK (Eclipse Layout Kernel)
Used for complex trees requiring more sophisticated layout:
- Better handling of large graphs
- Layer-based layout algorithm
- Reduced edge crossings

## Full-Screen Process Tree

The dedicated `/process-tree` page provides:
- Full viewport for the tree visualization
- Search and filter capabilities
- Event detail panel on the right
- Back button to return to the source page (detection or agent)
- URL parameters preserve the tree context

## Cross-Organization Support

Process trees work across organization boundaries:
- Detection process trees fetch events from the correct org's ClickHouse table
- Cross-org detections correctly route to the appropriate telemetry store
