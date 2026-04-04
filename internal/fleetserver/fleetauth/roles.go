package fleetauth

// Roles define the permission levels for fleet server users.
const (
	RoleRoot    = "root"    // Super admin: cross-account access, manage all accounts/orgs/users
	RoleAdmin   = "admin"   // Account admin: manage users, rules, agents, settings, active response within their account
	RoleAnalyst = "analyst" // Investigation: view/manage detections, events, rules. No active response or settings.
	RoleViewer  = "viewer"  // Read-only: view detections, events, agents. No modifications.
)

// Permission defines a specific action that can be checked against a role.
type Permission string

const (
	PermViewAgents          Permission = "agents:view"
	PermManageAgents        Permission = "agents:manage"
	PermDeleteAgents        Permission = "agents:delete"
	PermViewDetections      Permission = "detections:view"
	PermViewEvents          Permission = "events:view"
	PermViewRules           Permission = "rules:view"
	PermManageRules         Permission = "rules:manage"
	PermViewCommands        Permission = "commands:view"
	PermExecuteCommands     Permission = "commands:execute"
	PermViewSettings        Permission = "settings:view"
	PermManageSettings      Permission = "settings:manage"
	PermManageUsers         Permission = "users:manage"
	PermViewAuditLog        Permission = "audit:view"
	PermManageOrganizations Permission = "organizations:manage"
	PermAdminPanel          Permission = "admin:panel"   // Access the system admin panel
	PermManageAccounts      Permission = "accounts:manage" // Create/delete accounts (root only)
)

// rolePermissions maps each role to its allowed permissions.
var rolePermissions = map[string]map[Permission]bool{
	RoleRoot: {
		PermViewAgents:          true,
		PermManageAgents:        true,
		PermDeleteAgents:        true,
		PermViewDetections:      true,
		PermViewEvents:          true,
		PermViewRules:           true,
		PermManageRules:         true,
		PermViewCommands:        true,
		PermExecuteCommands:     true,
		PermViewSettings:        true,
		PermManageSettings:      true,
		PermManageUsers:         true,
		PermViewAuditLog:        true,
		PermManageOrganizations: true,
		PermAdminPanel:          true,
		PermManageAccounts:      true,
	},
	RoleAdmin: {
		PermViewAgents:          true,
		PermManageAgents:        true,
		PermDeleteAgents:        true,
		PermViewDetections:      true,
		PermViewEvents:          true,
		PermViewRules:           true,
		PermManageRules:         true,
		PermViewCommands:        true,
		PermExecuteCommands:     true,
		PermViewSettings:        true,
		PermManageSettings:      true,
		PermManageUsers:         true,
		PermViewAuditLog:        true,
		PermManageOrganizations: true,
	},
	RoleAnalyst: {
		PermViewAgents:     true,
		PermViewDetections: true,
		PermViewEvents:     true,
		PermViewRules:      true,
		PermManageRules:    true,
		PermViewCommands:   true,
		PermViewAuditLog:   true,
	},
	RoleViewer: {
		PermViewAgents:     true,
		PermViewDetections: true,
		PermViewEvents:     true,
		PermViewRules:      true,
		PermViewCommands:   true,
	},
}

// HasPermission checks if a role has a specific permission.
func HasPermission(role string, perm Permission) bool {
	perms, ok := rolePermissions[role]
	if !ok {
		return false
	}
	return perms[perm]
}

// ValidRole checks if a role name is valid.
func ValidRole(role string) bool {
	_, ok := rolePermissions[role]
	return ok
}

// IsRoot checks if a role is the root/super admin role.
func IsRoot(role string) bool {
	return role == RoleRoot
}
