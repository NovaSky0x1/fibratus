# Security & Access Control

The Fleet Server implements enterprise-grade security with multi-layer authentication, mandatory MFA, group-based RBAC with 60 granular permissions, and comprehensive audit logging.

## Authentication

### JWT Authentication

User authentication uses JSON Web Tokens (JWT):
- Login returns a JWT token stored in the browser
- Token included in `Authorization: Bearer <token>` header on all API requests
- Token expiration and refresh handled automatically
- 401 responses redirect to login page

### Password Policy

- Bcrypt password hashing with configurable cost factor
- Minimum password length enforcement
- Password change requires current password verification

### Account Lockout

Protection against brute-force attacks:
- Account locks after configurable number of failed attempts
- Lockout duration is configurable
- Locked accounts show status in user management
- Admin can manually unlock accounts

### Rate Limiting

Login endpoint rate limiting:
- Prevents rapid authentication attempts
- Per-IP rate limiting
- Returns 429 Too Many Requests when exceeded

## Multi-Factor Authentication (MFA)

### Mandatory 2FA

MFA is mandatory for all users:
- TOTP-based (Time-based One-Time Password)
- Compatible with any authenticator app (Google Authenticator, Authy, etc.)
- Users must set up 2FA during their first login
- Cannot access the dashboard without completing MFA setup

### MFA Setup Flow

1. User logs in with email/password
2. If 2FA not configured, setup screen appears
3. Server generates TOTP secret and QR code (server-side QR generation — no client-side npm dependency)
4. User scans QR code with authenticator app
5. User enters verification code to confirm setup
6. Subsequent logins require TOTP code

### MFA Reset

- Administrators can reset a user's MFA (forces re-enrollment)
- MFA cannot be disabled — only reset for re-enrollment
- Reset action logged in audit trail

## Role-Based Access Control (RBAC)

### Role Hierarchy

| Role | Description |
|------|-------------|
| **Root** | Super administrator with full system access, can switch between accounts |
| **Admin** | Organization administrator with management capabilities |
| **Analyst** | Standard user with investigation and response capabilities |
| **Viewer** | Read-only access to dashboards and events |

### Group-Based Permissions

Permissions are granted through groups, not directly to users:

- **Groups grant everything** — roles provide base access, groups add granular permissions
- Users can belong to multiple groups
- Permissions are additive across groups
- Default groups: Administrators, Analysts, Viewers

### 60 Granular Permissions

Permissions are organized by category:

#### Page Access
| Permission | Description |
|------------|-------------|
| `page:overview` | Access the Overview dashboard |
| `page:agents` | Access the Agents page |
| `page:detections` | Access the Detections page |
| `page:events` | Access the Events page |
| `page:rules` | Access the Rules page |
| `page:macros` | Access the Macros page |
| `page:audit` | Access the Audit Log |
| `page:management` | Access the Management console |

#### Agent Operations
| Permission | Description |
|------------|-------------|
| `agent:view` | View agent list and details |
| `agent:isolate` | Isolate/unisolate agents |
| `agent:kill_process` | Kill processes on agents |
| `agent:shell` | Remote terminal access |
| `agent:browse_files` | Browse agent file system |
| `agent:get_file` | Download files from agents |
| `agent:collect_info` | Collect system information |
| `agent:capture` | Start/stop kernel captures |
| `agent:delete` | Delete agents from fleet |
| `agent:update` | Trigger agent updates |
| `agent:uninstall` | Remote agent uninstall |

#### Rule Operations
| Permission | Description |
|------------|-------------|
| `rule:view` | View rules |
| `rule:create` | Create new rules |
| `rule:edit` | Edit existing rules |
| `rule:delete` | Delete rules |
| `rule:toggle` | Enable/disable rules |
| `rule:validate` | Validate rule syntax |
| `rule:sync` | Trigger GitHub sync |

#### Detection Operations
| Permission | Description |
|------------|-------------|
| `detection:view` | View detections |
| `detection:process_tree` | View process trees |

#### Management Operations
| Permission | Description |
|------------|-------------|
| `user:view` | View users |
| `user:create` | Create users |
| `user:edit` | Edit users |
| `user:delete` | Delete users |
| `group:view` | View groups |
| `group:manage` | Create/edit/delete groups |
| `enrollment:manage` | Create/revoke enrollment tokens |
| `org:manage` | Manage organizations |
| `admin:panel` | Access Super Admin panel |

### Permission Enforcement

Permissions are enforced at two levels:

1. **Dashboard (client-side)**: Navigation items and UI elements are hidden based on permissions
2. **API (server-side)**: Every API endpoint checks permissions before processing

The `PermissionContext` React context provides `hasPermission()` for client-side checks:

```tsx
const { hasPermission } = usePermissions()

if (hasPermission('agent:isolate')) {
  // Show isolate button
}
```

## User Management

### User Operations

| Operation | Description |
|-----------|-------------|
| **Create** | Create users with email, name, role, org assignment, group membership |
| **Edit** | Modify user details, role, groups, org restrictions |
| **Delete** | Remove user account |
| **Lock/Unlock** | Manually lock or unlock accounts |
| **Reset MFA** | Force MFA re-enrollment |
| **Reset Password** | Admin password reset |

### Organization Restrictions

Users can be restricted to specific organizations:
- **No restrictions** — user can access all orgs in the account
- **Org-restricted** — user can only access specified organizations
- Useful for multi-tenant environments with per-tenant analysts

### Self-Service

Users can manage their own profile:
- Change password (requires current password)
- Update name and email
- View group membership and permissions

## API Key Authentication

For programmatic access (CI/CD, automation, external tools):

```bash
curl -H "X-API-Key: <API_KEY>" https://server/api/v1/orgs/{orgId}/rules
```

- API keys bypass JWT authentication
- Scoped to specific accounts and organizations
- Used for rule validation API, CI/CD pipelines

## Audit Log

All security-relevant actions are logged:

| Event | Details Logged |
|-------|---------------|
| **Login** | User, IP, success/failure, MFA status |
| **User management** | Create/edit/delete/lock/unlock, who performed the action |
| **Rule changes** | Create/edit/delete/enable/disable, who made the change |
| **Commands** | Command type, target agent, issuing user |
| **Enrollment** | Token creation/revocation, agent enrollment |
| **Settings changes** | What changed, who changed it |

Root user actions are excluded from the audit log display to keep it clean.

## Account Hierarchy

- **Account** (Company level)
  - **Organization 1** (Department/Site) — has its own Agents, Rules, Detections, and ClickHouse telemetry table
  - **Organization 2** — same isolated set of resources
  - **Users** (account-wide) — Admin users can access all orgs; restricted users see only specific orgs

### Cross-Organization Access

- **All Organizations** view aggregates data across orgs
- Available when multiple organizations exist
- Root users can switch between accounts
- Account-level settings apply to all orgs

## File Access Compliance

CMMC/HIPAA compliant file access policies:

### Policy Settings

| Setting | Description |
|---------|-------------|
| **Allowed extensions** | Restrict file downloads to specific types |
| **Blocked extensions** | Prevent download of sensitive file types |
| **Max file size** | Limit file download size |
| **Audit all access** | Log every file access action |

### Policy States

- **Default** (nil policy) — standard defaults apply
- **Permissive** (empty policy) — no restrictions
- **Restrictive** — explicit allow/block lists

The UI shows the active policy state with a clear indicator.

## Super Admin Panel

Root users have access to the Super Admin panel with elevated capabilities:

- **Account management** — create/edit/delete accounts
- **User creation** — create users including root users
- **Database browser** — direct database inspection (phpMyAdmin-style)
- **Telemetry retention** — per-account retention configuration
- **System settings** — server-wide configuration
- **Organization management** — create/delete orgs across accounts

## Server-Side QR Code Generation

MFA QR codes are generated on the server in Go:
- No client-side QR code library needed
- QR code returned as base64 PNG
- Reduces dashboard JavaScript bundle size
- Prevents TOTP secret exposure in browser
