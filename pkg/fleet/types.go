/*
 * Copyright 2021-2022 by Nedim Sabic Sabic
 * https://www.fibratus.io
 * All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package fleet

import (
	"encoding/json"
	"time"
)

// ═══════════════════════════════════════════════════════════════
// Multi-tenancy: Account > Organization hierarchy
// ═══════════════════════════════════════════════════════════════

// Account represents a top-level billing entity (company/customer).
type Account struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Plan            string    `json:"plan"`
	Require2FA              bool     `json:"require_2fa"`
	TamperProtectionEnabled bool     `json:"tamper_protection_enabled"`
	EventLogEnabled         bool     `json:"eventlog_enabled"`
	IsolationWhitelist      []string `json:"isolation_whitelist,omitempty"`
	OrgCount                int      `json:"org_count,omitempty"`
	UserCount       int       `json:"user_count,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Organization represents a logical unit within an account
// (e.g., "Production", "Staging", "US-East"). Each org has
// its own agents, rules, detections, and enrollment tokens.
type Organization struct {
	ID                      string    `json:"id"`
	AccountID               string    `json:"account_id"`
	Name                    string    `json:"name"`
	Slug                    string    `json:"slug"`
	AgentCount              int       `json:"agent_count,omitempty"`
	TamperProtectionEnabled bool      `json:"tamper_protection_enabled"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

// User represents a dashboard user with access to one or more organizations.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Password  string    `json:"-"` // never serialized
	AccountID string    `json:"account_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`

	OrgRestrictions string `json:"org_restrictions,omitempty"` // JSON array of org IDs, empty = all
	Groups          []UserGroupMembership `json:"groups,omitempty"`

	LoginAttempts int       `json:"login_attempts,omitempty"`
	LockedUntil   time.Time `json:"locked_until,omitempty"`
	IsLocked      bool      `json:"is_locked,omitempty"`
	TOTPSecret    string    `json:"-"`
	TOTPEnabled   bool      `json:"totp_enabled"`
	RecoveryCodes string    `json:"-"`
}

// UserGroup defines a permission group with optional org restrictions.
type UserGroup struct {
	ID              string    `json:"id"`
	AccountID       string    `json:"account_id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Permissions     []string  `json:"permissions"`      // list of permission strings
	OrgRestrictions []string  `json:"org_restrictions"`  // org IDs, null/empty = all
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// UserGroupMembership is a lightweight reference for user → group.
type UserGroupMembership struct {
	GroupID   string `json:"group_id"`
	GroupName string `json:"group_name"`
}

// UserOrg maps a user's access and role within a specific organization.
type UserOrg struct {
	UserID string `json:"user_id"`
	OrgID  string `json:"org_id"`
	Role   string `json:"role"`
}

// ═══════════════════════════════════════════════════════════════
// Enrollment and authentication
// ═══════════════════════════════════════════════════════════════

// EnrollmentToken is a one-time-use (or limited-use) token that
// allows an agent to enroll with a specific organization. Admin
// creates these in the dashboard and distributes to endpoint admins.
type EnrollmentToken struct {
	ID        string    `json:"id"`
	AccountID string    `json:"account_id"`
	OrgID     string    `json:"org_id"`
	OrgName   string    `json:"org_name,omitempty"`
	Name      string    `json:"name"`
	MaxUses   int       `json:"max_uses"`
	UsesCount int       `json:"uses_count"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// IsValid checks whether the enrollment token can still be used.
func (t EnrollmentToken) IsValid() bool {
	if t.UsesCount >= t.MaxUses {
		return false
	}
	if time.Now().After(t.ExpiresAt) {
		return false
	}
	return true
}

// ═══════════════════════════════════════════════════════════════
// Fleet entities (all org-scoped)
// ═══════════════════════════════════════════════════════════════

// AgentStatus represents the current status of an agent in the fleet.
type AgentStatus string

const (
	AgentOnline  AgentStatus = "online"
	AgentOffline AgentStatus = "offline"
	AgentStale   AgentStatus = "stale"
)

// Agent represents a Fibratus agent registered with the fleet server.
type Agent struct {
	ID            string            `json:"id"`
	OrgID         string            `json:"org_id"`
	Hostname      string            `json:"hostname"`
	OSVersion     string            `json:"os_version"`
	EngineVersion string            `json:"engine_version"`
	GroupID       string            `json:"group_id,omitempty"`
	GroupName     string            `json:"group_name,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
	Status           AgentStatus       `json:"status"`
	LastHeartbeat    time.Time         `json:"last_heartbeat"`
	RegisteredAt     time.Time         `json:"registered_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	TamperProtection   bool              `json:"tamper_protection"`
	Isolated           bool              `json:"isolated"`
	EventLogCollection bool              `json:"eventlog_collection"`
}

// Heartbeat contains periodic status information sent by an agent.
type Heartbeat struct {
	Timestamp    time.Time `json:"timestamp"`
	RulesVersion string    `json:"rules_version"`
	CPUPercent   float64   `json:"cpu_pct"`
	MemoryMB     uint64    `json:"mem_mb"`
	EventsPerSec float64  `json:"events_per_sec"`
	ActiveRules  int       `json:"active_rules"`
}

// Detection represents a rule match reported by an agent.
type Detection struct {
	ID            string            `json:"id"`
	OrgID         string            `json:"org_id"`
	AgentID       string            `json:"agent_id"`
	AgentHostname string            `json:"agent_hostname"`
	RuleID        string            `json:"rule_id"`
	RuleName      string            `json:"rule_name"`
	Title         string            `json:"title"`
	Text          string            `json:"text"`
	Description   string            `json:"description,omitempty"`
	Severity      string            `json:"severity"`
	Labels        map[string]string `json:"labels,omitempty"`
	Tags          []string          `json:"tags,omitempty"`
	Events        json.RawMessage   `json:"events"`
	Timestamp     time.Time         `json:"timestamp"`
}

// Rule represents a detection rule managed by the fleet server.
type Rule struct {
	ID               string            `json:"id"`
	OrgID            string            `json:"org_id,omitempty"`
	Name             string            `json:"name"`
	Version          string            `json:"version"`
	Description      string            `json:"description,omitempty"`
	Condition        string            `json:"condition"`
	Output           string            `json:"output,omitempty"`
	Severity         string            `json:"severity"`
	Labels           map[string]string `json:"labels,omitempty"`
	Tags             []string          `json:"tags,omitempty"`
	References       []string          `json:"references,omitempty"`
	RawYAML          string            `json:"raw_yaml"`
	Enabled          bool              `json:"enabled"`
	Source           string            `json:"source,omitempty"`  // "manual", "github:<repo>" — tracks where the rule came from
	ValidationStatus string            `json:"validation_status"` // "valid", "invalid", "pending"
	ValidationErrors json.RawMessage   `json:"validation_errors,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// AgentGroup represents a logical grouping of agents for rule assignment.
type AgentGroup struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	RuleIDs     []string  `json:"rule_ids,omitempty"`
	AgentCount  int       `json:"agent_count,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ═══════════════════════════════════════════════════════════════
// Command queue: active response
// ═══════════════════════════════════════════════════════════════

// Command types for active response
const (
	CmdIsolate     = "isolate"
	CmdUnisolate   = "unisolate"
	CmdKillProcess = "kill_process"
	CmdUninstall   = "uninstall"
	CmdListDir     = "list_directory"
	CmdGetFile     = "get_file"
	CmdRunCommand  = "run_command"
	CmdCollectInfo = "collect_info"

	// Structured data-gathering commands
	CmdGetProcesses = "get_processes"
	CmdGetNetwork   = "get_network"
	CmdGetServices  = "get_services"
	CmdGetDrivers   = "get_drivers"
	CmdGetAutoruns  = "get_autoruns"
	CmdGetSoftware  = "get_software"
	CmdGetUsers     = "get_users"
	CmdGetRegistry  = "get_registry"

	// Capture and scanning commands
	CmdStartCapture        = "start_capture"
	CmdStopCapture         = "stop_capture"
	CmdYaraScan            = "yara_scan"
	CmdSetTamperProtection  = "set_tamper_protection"
	CmdSetEventLogPolicy    = "set_eventlog_policy"
	CmdLogoffUser           = "logoff_user"
	CmdListEventLogChannels = "list_eventlog_channels"
	CmdQueryEventLog        = "query_eventlog"
	CmdExportEvtx           = "export_evtx"
)

// Command status values
const (
	CmdStatusPending   = "pending"
	CmdStatusRunning   = "running"
	CmdStatusCompleted = "completed"
	CmdStatusFailed    = "failed"
)

// Command represents a queued command for an agent to execute.
type Command struct {
	ID           string          `json:"id"`
	OrgID        string          `json:"org_id"`
	AgentID      string          `json:"agent_id"`
	Type         string          `json:"type"`
	Payload      json.RawMessage `json:"payload"`
	Status       string          `json:"status"`
	Result       json.RawMessage `json:"result,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
	CreatedBy      string          `json:"created_by,omitempty"`
	CreatedByEmail string          `json:"created_by_email,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
}

// ═══════════════════════════════════════════════════════════════
// Event Log Policies: server-managed WEL collection configuration
// ═══════════════════════════════════════════════════════════════

// EventLogPolicyChannel represents a single channel in the event log collection policy.
type EventLogPolicyChannel struct {
	Name       string   `json:"name"`
	CollectAll bool     `json:"collect_all"`
	EventIDs   []uint16 `json:"event_ids,omitempty"`
}

// EventLogPolicy represents the server-managed event log collection configuration.
type EventLogPolicy struct {
	ID        string                  `json:"id"`
	OrgID     string                  `json:"org_id"`
	Enabled   bool                    `json:"enabled"`
	Channels  []EventLogPolicyChannel `json:"channels"`
	Version   int                     `json:"version"`
	CreatedAt time.Time               `json:"created_at"`
	UpdatedAt time.Time               `json:"updated_at"`
}

// ═══════════════════════════════════════════════════════════════
// Captures: kernel event capture sessions
// ═══════════════════════════════════════════════════════════════

// Capture status values
const (
	CaptureActive    = "active"
	CaptureCompleted = "completed"
	CaptureFailed    = "failed"
	CaptureCancelled = "cancelled"
)

// Capture represents a kernel event capture session on an agent.
type Capture struct {
	ID            string     `json:"id"`
	OrgID         string     `json:"org_id"`
	AgentID       string     `json:"agent_id"`
	AgentHostname string     `json:"agent_hostname"`
	Filter        string     `json:"filter"`
	Status        string     `json:"status"`
	EventCount    int64      `json:"event_count"`
	DurationSec   int        `json:"duration_sec"`
	KcapPath      string     `json:"kcap_path,omitempty"`
	CreatedBy     string     `json:"created_by,omitempty"`
	StartedAt     time.Time  `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

// ═══════════════════════════════════════════════════════════════
// Macros: reusable filter expressions for detection rules
// ═══════════════════════════════════════════════════════════════

// Macro represents a reusable filter expression that can be referenced in rule conditions.
type Macro struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id,omitempty"`
	Name        string    `json:"name"`
	Expr        string    `json:"expr"`
	List        []string  `json:"list,omitempty"`
	Description string    `json:"description,omitempty"`
	RawYAML     string    `json:"raw_yaml"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ═══════════════════════════════════════════════════════════════
// Audit log: tracks all admin actions
// ═══════════════════════════════════════════════════════════════

// AuditEntry represents a single auditable action taken in the portal.
type AuditEntry struct {
	ID           string          `json:"id"`
	OrgID        string          `json:"org_id"`
	UserID       string          `json:"user_id,omitempty"`
	UserEmail    string          `json:"user_email,omitempty"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id,omitempty"`
	ResourceName string          `json:"resource_name,omitempty"`
	Details      json.RawMessage `json:"details,omitempty"`
	IPAddress    string          `json:"ip_address,omitempty"`
	Timestamp    time.Time       `json:"timestamp"`
}

// ═══════════════════════════════════════════════════════════════
// Dashboard aggregates
// ═══════════════════════════════════════════════════════════════

// FleetOverview contains aggregate fleet statistics for the dashboard.
type FleetOverview struct {
	TotalAgents        int            `json:"total_agents"`
	OnlineAgents       int            `json:"online_agents"`
	OfflineAgents      int            `json:"offline_agents"`
	TotalDetections24h int            `json:"total_detections_24h"`
	SeverityBreakdown  map[string]int `json:"severity_breakdown"`
}

// TimelineBucket represents a single time bucket in a detection timeline.
type TimelineBucket struct {
	Timestamp  time.Time      `json:"timestamp"`
	Count      int            `json:"count"`
	BySeverity map[string]int `json:"by_severity,omitempty"`
}

// MitreCell represents detection counts for a specific MITRE ATT&CK technique.
type MitreCell struct {
	TacticID      string `json:"tactic_id"`
	TacticName    string `json:"tactic_name"`
	TechniqueID   string `json:"technique_id"`
	TechniqueName string `json:"technique_name"`
	Count         int    `json:"count"`
}
