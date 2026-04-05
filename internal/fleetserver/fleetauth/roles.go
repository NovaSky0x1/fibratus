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
	// Agent permissions
	PermViewAgents          Permission = "agents:view"
	PermManageAgents        Permission = "agents:manage"
	PermDeleteAgents        Permission = "agents:delete"

	// Detection permissions
	PermViewDetections      Permission = "detections:view"

	// Event permissions
	PermViewEvents          Permission = "events:view"

	// Rule permissions
	PermViewRules           Permission = "rules:view"
	PermManageRules         Permission = "rules:manage"

	// Active Response — granular command permissions
	PermViewCommands        Permission = "commands:view"
	PermExecuteCommands     Permission = "commands:execute"       // Generic execute (fallback)
	PermIsolateAgent        Permission = "response:isolate"       // Network isolation
	PermUnisolateAgent      Permission = "response:unisolate"     // Remove isolation
	PermKillProcess         Permission = "response:kill_process"  // Kill a process by PID
	PermRunCommand          Permission = "response:run_command"   // Execute shell commands
	PermBrowseFilesystem    Permission = "response:browse_files"  // List directories remotely
	PermDownloadFile        Permission = "response:download_file" // Download/collect files
	PermCollectInfo         Permission = "response:collect_info"  // Collect system information
	PermUninstallAgent      Permission = "response:uninstall"     // Uninstall the agent

	// Settings & configuration
	PermViewSettings        Permission = "settings:view"
	PermManageSettings      Permission = "settings:manage"
	PermManageEnrollment    Permission = "settings:enrollment"    // Manage enrollment tokens
	PermManageGitHubSync    Permission = "settings:github_sync"   // Configure GitHub rule sync
	PermManageMacros        Permission = "settings:macros"        // Manage macros

	// User & group management
	PermManageUsers         Permission = "users:manage"
	PermManageGroups        Permission = "users:groups"           // Manage user groups
	PermViewAuditLog        Permission = "audit:view"

	// Organization management
	PermManageOrganizations Permission = "organizations:manage"

	// System administration (root only)
	PermAdminPanel          Permission = "admin:panel"
	PermManageAccounts      Permission = "accounts:manage"
)

// rolePermissions maps each role to its allowed permissions.
var rolePermissions = map[string]map[Permission]bool{
	RoleRoot: {
		PermViewAgents: true, PermManageAgents: true, PermDeleteAgents: true,
		PermViewDetections: true, PermViewEvents: true,
		PermViewRules: true, PermManageRules: true,
		PermViewCommands: true, PermExecuteCommands: true,
		PermIsolateAgent: true, PermUnisolateAgent: true, PermKillProcess: true,
		PermRunCommand: true, PermBrowseFilesystem: true, PermDownloadFile: true,
		PermCollectInfo: true, PermUninstallAgent: true,
		PermViewSettings: true, PermManageSettings: true,
		PermManageEnrollment: true, PermManageGitHubSync: true, PermManageMacros: true,
		PermManageUsers: true, PermManageGroups: true, PermViewAuditLog: true,
		PermManageOrganizations: true,
		PermAdminPanel: true, PermManageAccounts: true,
	},
	RoleAdmin: {
		PermViewAgents: true, PermManageAgents: true, PermDeleteAgents: true,
		PermViewDetections: true, PermViewEvents: true,
		PermViewRules: true, PermManageRules: true,
		PermViewCommands: true, PermExecuteCommands: true,
		PermIsolateAgent: true, PermUnisolateAgent: true, PermKillProcess: true,
		PermRunCommand: true, PermBrowseFilesystem: true, PermDownloadFile: true,
		PermCollectInfo: true, PermUninstallAgent: true,
		PermViewSettings: true, PermManageSettings: true,
		PermManageEnrollment: true, PermManageGitHubSync: true, PermManageMacros: true,
		PermManageUsers: true, PermManageGroups: true, PermViewAuditLog: true,
		PermManageOrganizations: true,
	},
	RoleAnalyst: {
		PermViewAgents: true,
		PermViewDetections: true, PermViewEvents: true,
		PermViewRules: true, PermManageRules: true,
		PermViewCommands: true,
		PermViewAuditLog: true,
	},
	RoleViewer: {
		PermViewAgents: true,
		PermViewDetections: true,
		PermViewEvents: true,
		PermViewRules: true,
		PermViewCommands: true,
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
