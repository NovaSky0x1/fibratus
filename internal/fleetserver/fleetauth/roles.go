package fleetauth

// Roles define the permission levels for fleet server users.
const (
	RoleAdmin   = "admin"   // Full access: manage users, rules, agents, settings, active response
	RoleAnalyst = "analyst" // Investigation access: view/manage detections, events, rules. No active response or settings.
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
	PermExecuteCommands     Permission = "commands:execute"     // Active response
	PermViewSettings        Permission = "settings:view"
	PermManageSettings      Permission = "settings:manage"      // Enrollment tokens, GitHub sync, macros
	PermManageUsers         Permission = "users:manage"
	PermViewAuditLog        Permission = "audit:view"
	PermManageOrganizations Permission = "organizations:manage"
)

// rolePermissions maps each role to its allowed permissions.
var rolePermissions = map[string]map[Permission]bool{
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
