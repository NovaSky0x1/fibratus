# Windows Event Log Collection

The Fleet Server supports collecting Windows Event Log entries from endpoints and streaming them as telemetry events. This provides centralized visibility into Security, System, and Sysmon logs alongside kernel events.

## Overview

Windows Event Log collection runs on the agent as part of the event pipeline. It subscribes to configured event log channels, reads new entries, and forwards them through the telemetry pipeline to ClickHouse.

## Architecture

```
Agent:
┌─────────────────────────────────────────────┐
│ Event Log Collector                          │
│                                              │
│  EvtSubscribe(Security) ──┐                  │
│  EvtSubscribe(System)   ──┼──▶ Read Loop ──▶ Event Pipeline ──▶ Telemetry
│  EvtSubscribe(Sysmon)   ──┘                  │
│                                              │
│  Bookmark-based resume (survives restart)    │
└─────────────────────────────────────────────┘
```

## Supported Channels

| Channel | Description |
|---------|-------------|
| **Security** | Authentication, authorization, audit events |
| **System** | Service, driver, and system component events |
| **Sysmon** | Sysmon operational events (if Sysmon is installed) |

Default channels can be configured in the agent configuration or overridden via server policy.

## Server-Driven Policy

Event log collection can be controlled from the server:

### Per-Agent Toggle

On the Agent Detail page → **Event Log** tab:
- Toggle event log collection on/off per agent
- Configure which channels to collect
- Changes take effect on the next heartbeat

### How Policy Works

1. Admin toggles event log collection in the dashboard
2. Server stores the policy and sends a command to the agent
3. Agent receives channel configuration via heartbeat response
4. Agent starts/stops event log subscriptions accordingly

## Event Log Collector Implementation

### Windows Event Log API

The collector uses the Windows Event Log API (EvtSubscribe):

- **Subscription model**: Push-based using manual-reset events for notification
- **Polling**: EvtNext retrieves events from the subscription
- **Bookmark-based resume**: After restart, collection resumes from the last processed event
- **INFINITE timeout**: EvtNext blocks until events are available

### Read Loop

The read loop processes events:

1. Wait for signal from EvtSubscribe notification event
2. Call EvtNext to retrieve batch of events
3. Parse XML event data
4. Convert to Fibratus event format
5. Forward through the unified output channel to the aggregator

### Event Parsing

Windows Event Log entries are parsed from XML:
- Event ID, level, source/provider
- Channel name
- Timestamp
- Event data fields
- Message text

### Backoff and Reliability

- Exponential backoff on subscription errors
- Automatic reconnection after transient failures
- Diagnostic logging for debugging collection issues
- Proper cleanup of subscription handles on shutdown

## Dashboard Display

### Events Page

Event log events appear on the Events page with:
- **Event ID badge** — prominently displayed event ID
- **Category color** — distinct color for event log events
- **Channel** — Security, System, or Sysmon source
- **Level** — Information, Warning, Error, Critical
- **Provider** — event source name
- **Message** — event log message content

### Event Detail

Expanded event detail shows:
- Full XML event data
- Parsed fields in structured view
- Event ID and source information
- Associated process context (if available)

### Agent Event Log Tab

The dedicated Event Log tab on the agent detail page:
- Browse Windows Event Log entries from the specific agent
- Filter by channel, event ID, level
- Dedicated layout for event log content
- Toggle collection on/off

### Search Integration

Event log fields are searchable via Fibratus QL:

```
# Find specific event IDs
eventlog.event_id = 4624

# Filter by provider
eventlog.provider = 'Microsoft-Windows-Security-Auditing'

# Filter by channel
eventlog.channel = 'Security'

# Filter by level
eventlog.level = 'Error'
```

## Telemetry Pipeline Integration

Event log events flow through the same telemetry pipeline as kernel events:

1. Collector produces events on the unified output channel
2. Aggregator batches events for efficient transmission
3. Fleet telemetry output streams to server
4. Server stores in ClickHouse with other telemetry

Event log events are included in the security-relevant event whitelist, ensuring they are always streamed to the server.

## Configuration

### Agent-Side Default

```yaml
eventsource:
  eventlog:
    channels:
      - Security
      - System
      - Microsoft-Windows-Sysmon/Operational
```

### Server Override

The server can override channel configuration via heartbeat policy, allowing centralized management of which channels are collected across the fleet.
