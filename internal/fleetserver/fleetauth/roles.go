package fleetauth

import "sort"

// Roles define the permission levels for fleet server users.
// Only "root" has inherent permissions. All other users get permissions
// exclusively from their user group memberships.
const (
	RoleRoot    = "root"    // Super admin: cross-account, bypasses all permission checks
	RoleMember  = "member"  // Default role for all non-root users — permissions come from groups
	// Legacy roles kept for backward compatibility (treated same as member)
	RoleAdmin   = "admin"
	RoleAnalyst = "analyst"
	RoleViewer  = "viewer"
)

// Permission defines a specific action that can be checked against a user's group memberships.
type Permission string

const (
	// ═══════════════════════════════════════════════════════════
	// Page Access
	// ═══════════════════════════════════════════════════════════
	PermPageOverview    Permission = "page:overview"
	PermPageAgents      Permission = "page:agents"
	PermPageDetections  Permission = "page:detections"
	PermPageEvents      Permission = "page:events"
	PermPageRules       Permission = "page:rules"
	PermPageMacros      Permission = "page:macros"
	PermPageAudit       Permission = "page:audit"
	PermPageManagement  Permission = "page:management"
	PermPageProcessTree Permission = "page:process_tree"

	// ═══════════════════════════════════════════════════════════
	// Agents
	// ═══════════════════════════════════════════════════════════
	PermViewAgents   Permission = "agents:view"
	PermManageAgents Permission = "agents:manage"
	PermDeleteAgents Permission = "agents:delete"

	// ═══════════════════════════════════════════════════════════
	// Agent Detail Tabs
	// ═══════════════════════════════════════════════════════════
	PermAgentsViewEvents     Permission = "agents:view_events"
	PermAgentsViewDetections Permission = "agents:view_detections"
	PermAgentsViewProcesses  Permission = "agents:view_processes"
	PermAgentsViewNetwork    Permission = "agents:view_network"
	PermAgentsViewServices   Permission = "agents:view_services"
	PermAgentsViewDrivers    Permission = "agents:view_drivers"
	PermAgentsViewAutoruns   Permission = "agents:view_autoruns"
	PermAgentsViewSoftware   Permission = "agents:view_software"
	PermAgentsViewUsers      Permission = "agents:view_users"
	PermAgentsViewFiles      Permission = "agents:view_files"
	PermAgentsViewRegistry   Permission = "agents:view_registry"
	PermAgentsViewEventLog   Permission = "agents:view_eventlog"
	PermAgentsViewTerminal   Permission = "agents:view_terminal"
	PermAgentsViewCaptures   Permission = "agents:view_captures"
	PermAgentsViewHistory    Permission = "agents:view_history"

	// ═══════════════════════════════════════════════════════════
	// Detections
	// ═══════════════════════════════════════════════════════════
	PermViewDetections   Permission = "detections:view"
	PermManageDetections Permission = "detections:manage"

	// ═══════════════════════════════════════════════════════════
	// Events
	// ═══════════════════════════════════════════════════════════
	PermViewEvents Permission = "events:view"

	// ═══════════════════════════════════════════════════════════
	// Rules
	// ═══════════════════════════════════════════════════════════
	PermViewRules   Permission = "rules:view"
	PermManageRules Permission = "rules:manage"

	// ═══════════════════════════════════════════════════════════
	// Active Response
	// ═══════════════════════════════════════════════════════════
	PermViewCommands    Permission = "commands:view"
	PermExecuteCommands Permission = "commands:execute"

	// ═══════════════════════════════════════════════════════════
	// Response Actions (granular)
	// ═══════════════════════════════════════════════════════════
	PermIsolateAgent     Permission = "response:isolate"
	PermUnisolateAgent   Permission = "response:unisolate"
	PermKillProcess      Permission = "response:kill_process"
	PermRunCommand       Permission = "response:run_command"
	PermBrowseFilesystem Permission = "response:browse_files"
	PermDownloadFile     Permission = "response:download_file"
	PermCollectInfo      Permission = "response:collect_info"
	PermUninstallAgent   Permission = "response:uninstall"

	// ═══════════════════════════════════════════════════════════
	// Captures
	// ═══════════════════════════════════════════════════════════
	PermViewCaptures   Permission = "captures:view"
	PermCreateCaptures Permission = "captures:create"
	PermDeleteCaptures Permission = "captures:delete"

	// ═══════════════════════════════════════════════════════════
	// Telemetry
	// ═══════════════════════════════════════════════════════════
	PermViewTelemetry      Permission = "telemetry:view"
	PermConfigureTelemetry Permission = "telemetry:configure"

	// ═══════════════════════════════════════════════════════════
	// Settings
	// ═══════════════════════════════════════════════════════════
	PermViewSettings   Permission = "settings:view"
	PermManageSettings Permission = "settings:manage"
	PermManageMacros   Permission = "settings:macros"

	// ═══════════════════════════════════════════════════════════
	// Enrollment
	// ═══════════════════════════════════════════════════════════
	PermViewEnrollment   Permission = "enrollment:view"
	PermManageEnrollment Permission = "enrollment:manage"

	// ═══════════════════════════════════════════════════════════
	// GitHub Sync
	// ═══════════════════════════════════════════════════════════
	PermViewGitHubSync   Permission = "github_sync:view"
	PermManageGitHubSync Permission = "github_sync:manage"

	// ═══════════════════════════════════════════════════════════
	// User Management
	// ═══════════════════════════════════════════════════════════
	PermManageUsers  Permission = "users:manage"
	PermManageGroups Permission = "users:groups"
	PermViewAuditLog Permission = "audit:view"

	// ═══════════════════════════════════════════════════════════
	// Organization Management
	// ═══════════════════════════════════════════════════════════
	PermManageOrganizations Permission = "organizations:manage"

	// ═══════════════════════════════════════════════════════════
	// System Administration (root only)
	// ═══════════════════════════════════════════════════════════
	PermAdminPanel     Permission = "admin:panel"
	PermManageAccounts Permission = "accounts:manage"
)

// allPermissions is the canonical list of every permission in the system.
var allPermissions = []Permission{
	// Pages
	PermPageOverview, PermPageAgents, PermPageDetections, PermPageEvents,
	PermPageRules, PermPageMacros, PermPageAudit, PermPageManagement, PermPageProcessTree,
	// Agents
	PermViewAgents, PermManageAgents, PermDeleteAgents,
	// Agent Detail
	PermAgentsViewEvents, PermAgentsViewDetections, PermAgentsViewProcesses,
	PermAgentsViewNetwork, PermAgentsViewServices, PermAgentsViewDrivers,
	PermAgentsViewAutoruns, PermAgentsViewSoftware, PermAgentsViewUsers,
	PermAgentsViewFiles, PermAgentsViewRegistry, PermAgentsViewEventLog,
	PermAgentsViewTerminal, PermAgentsViewCaptures, PermAgentsViewHistory,
	// Detections
	PermViewDetections, PermManageDetections,
	// Events
	PermViewEvents,
	// Rules
	PermViewRules, PermManageRules,
	// Active Response
	PermViewCommands, PermExecuteCommands,
	// Response Actions
	PermIsolateAgent, PermUnisolateAgent, PermKillProcess, PermRunCommand,
	PermBrowseFilesystem, PermDownloadFile, PermCollectInfo, PermUninstallAgent,
	// Captures
	PermViewCaptures, PermCreateCaptures, PermDeleteCaptures,
	// Telemetry
	PermViewTelemetry, PermConfigureTelemetry,
	// Settings
	PermViewSettings, PermManageSettings, PermManageMacros,
	// Enrollment
	PermViewEnrollment, PermManageEnrollment,
	// GitHub Sync
	PermViewGitHubSync, PermManageGitHubSync,
	// User Management
	PermManageUsers, PermManageGroups, PermViewAuditLog,
	// Organizations
	PermManageOrganizations,
	// System Admin
	PermAdminPanel, PermManageAccounts,
}

// rolePermissions maps roles to their default permissions.
// Only root has inherent permissions. All other roles get permissions from groups.
var rolePermissions = map[string]map[Permission]bool{
	RoleRoot: func() map[Permission]bool {
		m := make(map[Permission]bool, len(allPermissions))
		for _, p := range allPermissions {
			m[p] = true
		}
		return m
	}(),
	RoleMember:  {},
	RoleAdmin:   {},
	RoleAnalyst: {},
	RoleViewer:  {},
}

// AllPermissions returns all defined permission strings, sorted.
func AllPermissions() []string {
	perms := make([]string, 0, len(allPermissions))
	for _, p := range allPermissions {
		perms = append(perms, string(p))
	}
	sort.Strings(perms)
	return perms
}

// RolePermissions returns the permissions granted to a given role.
func RolePermissions(role string) []Permission {
	perms, ok := rolePermissions[role]
	if !ok {
		return nil
	}
	result := make([]Permission, 0, len(perms))
	for p := range perms {
		result = append(result, p)
	}
	return result
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
