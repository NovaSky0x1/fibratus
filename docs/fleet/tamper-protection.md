# Tamper Protection & Network Isolation

The Fleet Server provides tamper protection to prevent adversaries from disabling the agent, and WFP-based network isolation for containing compromised endpoints.

## Tamper Protection

### Overview

Tamper protection prevents unauthorized modification or termination of the Fibratus agent on an endpoint. When enabled, the agent protects its own process, service, and configuration from tampering.

### Enabling Tamper Protection

Tamper protection can be enabled at multiple levels:

| Level | Scope | Description |
|-------|-------|-------------|
| **Per-agent** | Single endpoint | Toggle on the agent detail page |
| **Per-organization** | All agents in org | Enable in Management → Settings |
| **Per-account** | All agents in account | Enable in Super Admin → Account Settings |

When enabled at the organization or account level:
- The setting is propagated to all agents via the heartbeat response
- Agents receive the tamper protection policy command
- New agents inherit the policy automatically

### How It Works

The tamper protection engine on Windows:
1. Protects the Fibratus service from being stopped or modified
2. Protects the Fibratus process from being terminated
3. Protects configuration files from modification
4. Monitors for tampering attempts

### Dashboard Status

The agent detail page shows tamper protection status:
- **Protected** — green shield indicator
- **Unprotected** — no indicator
- Status updates in real time via heartbeat

## Network Isolation (WFP)

### Overview

Network isolation uses the Windows Filtering Platform (WFP) to block all network traffic on an endpoint except communication with the Fleet Server. This is used for incident response to contain a compromised endpoint while maintaining management access.

### How It Works

**Normal State**: Endpoint has full network access (Internet, LAN, Fleet Server, everything).

**Isolated State**: Endpoint can ONLY communicate with the Fleet Server. All other traffic (Internet, LAN, other hosts) is blocked at the kernel level by WFP filters.

When isolation is activated:

1. Server resolves its own hostname to an IP address
2. WFP filter rules are created on the endpoint:
   - **Block all** inbound and outbound traffic
   - **Allow** traffic to/from the Fleet Server IP
   - **Allow** DNS resolution (for server hostname)
   - **Allow** DHCP (to maintain IP address)
3. Agent continues heartbeating and accepting commands
4. All other network access is blocked

### Isolation Whitelist

The isolation whitelist allows additional IPs or ranges to communicate during isolation:

Configure in Management → Settings:
- Add IP addresses or CIDR ranges
- Useful for allowing DNS servers, domain controllers, or forensic tools
- Whitelist is propagated to agents alongside the isolate command

### Isolating an Agent

**From the Agent Detail page:**
1. Click the **Isolate** button
2. Confirm the action in the dialog
3. Agent receives the command within 5 seconds (next command poll)
4. WFP rules are applied
5. Agent status changes to "Isolated" (amber badge)

**From Active Response commands:**
- The `isolate` command can also be triggered programmatically

### Unisolating an Agent

1. Click the **Unisolate** button on an isolated agent
2. Confirm the action
3. Agent removes WFP filter rules
4. Full network connectivity is restored
5. Agent status returns to "Online"

### Account/Org Level Isolation

Isolation settings can be configured at the account or organization level:
- Propagated to agents via heartbeat policy
- Useful for emergency fleet-wide isolation
- Can be overridden per-agent

## Agent Detail Page Integration

The agent overview section displays:

### Tamper Protection Status
- **Enabled**: Green badge with shield icon
- **Disabled**: Gray indicator
- Toggle button to enable/disable

### Isolation Status
- **Isolated**: Amber badge with lock icon
- **Not Isolated**: No indicator
- Isolate/Unisolate button

### Settings Page Integration

The Management page includes:

#### Tamper Protection Settings
- Account-wide toggle
- Per-org toggle
- Status indicator showing current policy

#### Isolation Whitelist
- List of allowed IPs/CIDR ranges during isolation
- Add/remove entries
- Applied to all isolation commands

## Security Considerations

### Tamper Protection
- Only the Fleet Server can disable tamper protection via policy
- Local attempts to stop the service are blocked
- Protects against malware attempting to disable EDR

### Network Isolation
- Ensures compromised endpoints cannot exfiltrate data
- Maintains management channel for investigation
- WFP rules survive service restart
- Unisolation requires authenticated server command

### Enrollment Tamper Protection
- The `fibratus setup` command configures enrollment-time tamper protection
- Ensures the agent cannot be modified immediately after installation
- Protects enrollment credentials in the registry
