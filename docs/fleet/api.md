# API Reference

The Fleet Server exposes a RESTful HTTP API for all operations. The dashboard communicates exclusively through this API, and external tools can integrate using the same endpoints.

## Authentication

### JWT Token

Most endpoints require a JWT token:

```bash
# Login to get a token
curl -X POST https://server/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "admin@example.com", "password": "...", "totp_code": "123456"}'

# Response
{
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "user": { "id": "...", "email": "...", "role": "admin" }
  }
}

# Use token in subsequent requests
curl https://server/api/v1/orgs/{orgId}/agents \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIs..."
```

### API Key

For programmatic access:

```bash
curl https://server/api/v1/orgs/{orgId}/rules \
  -H "X-API-Key: <API_KEY>"
```

## Base URL

All API endpoints are prefixed with `/api/v1`.

## Endpoints

### Authentication

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/auth/login` | Login with email, password, TOTP code |
| `POST` | `/auth/signup` | Create new account (self-signup) |
| `GET` | `/auth/me` | Get current user profile |
| `PUT` | `/auth/me` | Update current user profile |
| `PUT` | `/auth/me/password` | Change own password |
| `POST` | `/auth/me/totp/setup` | Start TOTP 2FA setup |
| `POST` | `/auth/me/totp/verify` | Verify TOTP setup code |

### Agents

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/orgs/{orgId}/agents` | List agents in organization |
| `GET` | `/orgs/{orgId}/agents/{agentId}` | Get agent details |
| `DELETE` | `/orgs/{orgId}/agents/{agentId}` | Delete agent |
| `POST` | `/agents/register` | Register new agent (public endpoint) |
| `POST` | `/agents/heartbeat` | Agent heartbeat |

### Commands

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/orgs/{orgId}/agents/{agentId}/commands` | Issue command to agent |
| `GET` | `/orgs/{orgId}/agents/{agentId}/commands` | List agent command history |
| `GET` | `/orgs/{orgId}/agents/{agentId}/commands/pending` | Get pending commands |
| `PUT` | `/orgs/{orgId}/commands/{commandId}/result` | Submit command result |

### Rules

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/orgs/{orgId}/rules` | List rules |
| `POST` | `/orgs/{orgId}/rules` | Create rule |
| `PUT` | `/orgs/{orgId}/rules/{ruleId}` | Update rule |
| `DELETE` | `/orgs/{orgId}/rules/{ruleId}` | Delete rule |
| `PUT` | `/orgs/{orgId}/rules/{ruleId}/toggle` | Enable/disable rule |
| `POST` | `/orgs/{orgId}/rules/validate` | Validate rule YAML |
| `POST` | `/orgs/{orgId}/rules/validate-all` | Validate all rules |
| `GET` | `/orgs/{orgId}/rules/noisy` | Get noisy rules |

### Macros

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/orgs/{orgId}/macros` | List macros |
| `POST` | `/orgs/{orgId}/macros` | Create macro |
| `PUT` | `/orgs/{orgId}/macros/{macroId}` | Update macro |
| `DELETE` | `/orgs/{orgId}/macros/{macroId}` | Delete macro |

### Detections

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/orgs/{orgId}/detections` | List detections |
| `GET` | `/orgs/{orgId}/detections/{detectionId}` | Get detection detail |
| `POST` | `/orgs/{orgId}/detections` | Create detection (agent use) |

### Telemetry

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/orgs/{orgId}/telemetry/events` | Query telemetry events |
| `GET` | `/orgs/{orgId}/telemetry/process-tree` | Get process tree data |
| `POST` | `/orgs/{orgId}/telemetry/events` | Ingest telemetry events |

### Enrollment

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/enroll` | Enroll agent (public endpoint) |
| `GET` | `/orgs/{orgId}/enrollment-tokens` | List enrollment tokens |
| `POST` | `/orgs/{orgId}/enrollment-tokens` | Create enrollment token |
| `DELETE` | `/orgs/{orgId}/enrollment-tokens/{tokenId}` | Revoke token |

### Organizations

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/orgs` | List organizations |
| `POST` | `/orgs` | Create organization |
| `PUT` | `/orgs/{orgId}` | Update organization |
| `DELETE` | `/orgs/{orgId}` | Delete organization |

### Users

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/orgs/{orgId}/users` | List users |
| `POST` | `/orgs/{orgId}/users` | Create user |
| `PUT` | `/orgs/{orgId}/users/{userId}` | Update user |
| `DELETE` | `/orgs/{orgId}/users/{userId}` | Delete user |
| `POST` | `/orgs/{orgId}/users/{userId}/reset-mfa` | Reset user MFA |
| `POST` | `/orgs/{orgId}/users/{userId}/reset-password` | Reset user password |

### Groups

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/orgs/{orgId}/groups` | List groups |
| `POST` | `/orgs/{orgId}/groups` | Create group |
| `PUT` | `/orgs/{orgId}/groups/{groupId}` | Update group |
| `DELETE` | `/orgs/{orgId}/groups/{groupId}` | Delete group |

### Dashboard

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/orgs/{orgId}/dashboard/overview` | Dashboard overview stats |
| `GET` | `/orgs/{orgId}/dashboard/timeline` | Detection timeline data |

### Audit Log

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/orgs/{orgId}/audit-log` | List audit log entries |

### GitHub Sync

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/orgs/{orgId}/github-sync` | List sync configurations |
| `POST` | `/orgs/{orgId}/github-sync` | Create sync configuration |
| `PUT` | `/orgs/{orgId}/github-sync/{configId}` | Update sync configuration |
| `DELETE` | `/orgs/{orgId}/github-sync/{configId}` | Delete sync configuration |
| `POST` | `/orgs/{orgId}/github-sync/{configId}/trigger` | Trigger sync |

### Admin Endpoints (Root Only)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/accounts` | List all accounts |
| `POST` | `/admin/accounts` | Create account |
| `PUT` | `/admin/accounts/{accountId}` | Update account |
| `DELETE` | `/admin/accounts/{accountId}` | Delete account |
| `GET` | `/admin/accounts/{accountId}/orgs` | List account orgs |
| `POST` | `/admin/users` | Create user (cross-account) |
| `GET` | `/admin/db/tables` | List database tables |
| `GET` | `/admin/db/tables/{table}` | Query table rows |
| `DELETE` | `/admin/db/tables/{table}` | Bulk delete rows |

## Response Format

All responses follow a standard envelope:

```json
{
  "data": { ... },
  "error": null,
  "meta": {
    "total": 100,
    "page": 1,
    "per_page": 50
  }
}
```

### Error Responses

```json
{
  "data": null,
  "error": {
    "code": 400,
    "message": "Validation failed: condition is invalid"
  }
}
```

### HTTP Status Codes

| Code | Description |
|------|-------------|
| `200` | Success with response body |
| `201` | Created |
| `204` | Success with no content |
| `400` | Bad request / validation error |
| `401` | Unauthorized (missing or expired token) |
| `403` | Forbidden (insufficient permissions) |
| `404` | Resource not found |
| `429` | Rate limited |
| `500` | Internal server error |

## gRPC Service

Agent communication uses gRPC on port 8444:

```protobuf
service FleetAgent {
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
  rpc StreamTelemetry(stream TelemetryBatch) returns (TelemetryAck);
  rpc GetCommands(GetCommandsRequest) returns (GetCommandsResponse);
  rpc SubmitResult(SubmitResultRequest) returns (SubmitResultResponse);
}
```

gRPC traffic is routed through Nginx on port 443, proxied to the Go backend on port 8444.

## Rule Validation API

The rule validation endpoint is designed for external tool integration:

```bash
# Validate a rule with macros
curl -X POST https://server/api/v1/orgs/{orgId}/rules/validate \
  -H "X-API-Key: <API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{
    "raw_yaml": "name: Test Rule\ncondition: >-\n  spawn_process and\n  ps.name = '\''cmd.exe'\''\nseverity: medium\noutput: test",
    "macros": [
      {"name": "suspicious_parents", "expr": "ps.parent.name in ('\''winword.exe'\'', '\''excel.exe'\'')"}
    ]
  }'

# Response
{
  "data": {
    "valid": true,
    "errors": []
  }
}
```

Use cases:
- VS Code extension for real-time rule validation
- Pre-commit hooks in Detection-as-Code workflows
- CI/CD pipeline validation gates
