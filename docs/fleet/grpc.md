# gRPC Agent Protocol

Agent-to-server communication uses gRPC with Protocol Buffers for efficient, typed, bidirectional communication.

## Overview

The Fleet Server exposes a gRPC service on port 8444 (proxied through Nginx on port 443). Agents use this service for registration, heartbeats, telemetry streaming, command retrieval, and result submission.

## Transport Architecture

**Agent (Windows)**: gRPC client with Protobuf serialization connects to port 443 (TLS).

**Server (Linux)**: Nginx receives gRPC on port 443, proxies to the Go gRPC server on port 8444 (localhost). The gRPC location match is based on the `/fleet.` path prefix in the protobuf package.

### Why gRPC?

The transport was migrated from HTTP REST to gRPC for:
- **Efficiency**: Protocol Buffers are 3-10x smaller than JSON
- **Streaming**: Bidirectional streaming for telemetry
- **Type safety**: Strongly typed message definitions
- **Performance**: HTTP/2 multiplexing, header compression
- **Connection reuse**: Long-lived connections reduce overhead

## Service Definition

```protobuf
service FleetAgent {
  // Agent registration (called once after enrollment)
  rpc Register(RegisterRequest) returns (RegisterResponse);

  // Periodic heartbeat (every 30 seconds)
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);

  // Telemetry event streaming
  rpc StreamTelemetry(stream TelemetryBatch) returns (TelemetryAck);

  // Command retrieval (polled every 5 seconds)
  rpc GetCommands(GetCommandsRequest) returns (GetCommandsResponse);

  // Command result submission
  rpc SubmitResult(SubmitResultRequest) returns (SubmitResultResponse);
}
```

## Messages

### Registration

```protobuf
message RegisterRequest {
  string agent_id = 1;
  string org_id = 2;
  string hostname = 3;
  string os_version = 4;
  string agent_version = 5;
  string primary_ip = 6;
}

message RegisterResponse {
  string agent_id = 1;
  string status = 2;
}
```

### Heartbeat

```protobuf
message HeartbeatRequest {
  string agent_id = 1;
  string hostname = 2;
  string os_version = 3;
  string agent_version = 4;
  string primary_ip = 5;
  bool tamper_protected = 6;
  bool isolated = 7;
}

message HeartbeatResponse {
  bool tamper_protection = 1;
  bool check_auto_update = 2;
  repeated string eventlog_channels = 3;
  repeated PolicyCommand commands = 4;
}
```

The heartbeat response carries policy updates:
- **Tamper protection**: Enable/disable tamper protection
- **Auto-update**: Signal to check for updates
- **Event log channels**: Which Windows Event Log channels to collect
- **Policy commands**: Additional policy-level commands

### Telemetry

```protobuf
message TelemetryBatch {
  string agent_id = 1;
  string org_id = 2;
  repeated TelemetryEvent events = 3;
  map<string, string> metadata = 4;
}

message TelemetryEvent {
  string event_type = 1;
  string category = 2;
  int64 timestamp = 3;
  uint32 pid = 4;
  string process_name = 5;
  string params = 6;       // JSON-encoded parameters
  string ps_info = 7;      // JSON-encoded process state
  string raw_event = 8;    // Full serialized event
}

message TelemetryAck {
  int32 received = 1;
}
```

### Commands

```protobuf
message GetCommandsRequest {
  string agent_id = 1;
}

message GetCommandsResponse {
  repeated Command commands = 1;
}

message Command {
  string id = 1;
  string type = 2;          // isolate, kill_process, run_command, etc.
  string parameters = 3;    // JSON-encoded parameters
  int64 created_at = 4;
}

message SubmitResultRequest {
  string command_id = 1;
  string agent_id = 2;
  string status = 3;        // completed, failed
  string output = 4;        // Command output (gzip-compressed for large results)
  string error = 5;
}
```

## Nginx gRPC Proxy

Nginx routes gRPC traffic based on the path prefix:

```nginx
# gRPC traffic (matching the protobuf package prefix)
location /fleet. {
    grpc_pass grpc://127.0.0.1:8444;
}
```

This allows gRPC to share port 443 with HTTPS traffic.

## Connection Management

### Stream Reconnection

If the gRPC connection drops:
1. Agent detects disconnection
2. Exponential backoff before reconnect
3. Backoff resets on successful reconnection
4. Rules are always pushed after reconnection (not just on change)

### Authentication

Agent identity is established through:
- Agent ID header (`X-Agent-ID`) on all requests
- Enrollment certificate for mTLS (optional)
- Server validates agent ID against the database

## NATS Integration

Telemetry events are routed through NATS for internal message passing:
- Agent streams events via gRPC
- Server publishes to NATS subjects
- ClickHouse ingestion consumes from NATS
- Decouples ingestion from storage for reliability

## Migration from HTTP

The transport was migrated from HTTP REST to gRPC:
- **Before**: HTTP POST for telemetry, HTTP GET for commands, HTTP POST for heartbeat
- **After**: gRPC service with all operations
- Agent ID was moved from HTTP headers to gRPC metadata
- Protobuf serialization replaced JSON for all agent communication
- HTTP REST API retained for dashboard/external tool access
