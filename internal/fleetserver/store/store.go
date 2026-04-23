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

package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rabbitstack/fibratus/pkg/fleet"
)

// AccountStore manages account persistence.
type AccountStore interface {
	Create(ctx context.Context, account *fleet.Account) error
	Get(ctx context.Context, id string) (*fleet.Account, error)
	ListAll(ctx context.Context) ([]*fleet.Account, error)
	Delete(ctx context.Context, id string) error
	UpdateSettings(ctx context.Context, id string, require2FA bool, tamperProtection *bool, isolationWhitelist []string, eventlogEnabled *bool) error
	UpdateRetention(ctx context.Context, id string, days int) error
	UpdateFilePolicy(ctx context.Context, id string, extensions []string) error
	UpdateProfile(ctx context.Context, id, name, plan string) error
	UpdateAgentVersion(ctx context.Context, id, version, msiURL string, autoUpdate bool) error
	UpdateAgentRepo(ctx context.Context, id, repo string) error
}

// OrgStore manages organization persistence.
type OrgStore interface {
	Create(ctx context.Context, org *fleet.Organization) error
	Get(ctx context.Context, id string) (*fleet.Organization, error)
	ListByAccount(ctx context.Context, accountID string) ([]*fleet.Organization, error)
	Delete(ctx context.Context, id string) error
	UpdateTamperProtection(ctx context.Context, id string, enabled bool) error
	UpdateRetention(ctx context.Context, id string, days int) error
}

// UserStore manages user persistence and authentication.
type UserStore interface {
	Create(ctx context.Context, user *fleet.User) error
	GetByEmail(ctx context.Context, email string) (*fleet.User, error)
	Get(ctx context.Context, id string) (*fleet.User, error)
	AddOrgAccess(ctx context.Context, userID, orgID, role string) error
	GetOrgAccess(ctx context.Context, userID string) ([]fleet.UserOrg, error)
	HasOrgAccess(ctx context.Context, userID, orgID string) (bool, error)
	ListAll(ctx context.Context) ([]*fleet.User, error)
	ListByAccount(ctx context.Context, accountID string) ([]*fleet.User, error)
	UpdateProfile(ctx context.Context, userID, name, email string) error
	UpdatePassword(ctx context.Context, userID, hashedPassword string) error
	SetOrgRestrictions(ctx context.Context, userID string, orgRestrictions string) error
	SetAccount(ctx context.Context, userID, accountID string) error
	IncrementLoginAttempts(ctx context.Context, userID string) error
	LockAccount(ctx context.Context, userID string, until time.Time) error
	ResetLoginAttempts(ctx context.Context, userID string) error
	SetTOTP(ctx context.Context, userID, secret string, enabled bool, recoveryCodes string) error
	ListByOrg(ctx context.Context, orgID string) ([]*fleet.User, error)
	Delete(ctx context.Context, id string) error
	UpdateRole(ctx context.Context, userID, orgID, role string) error
	SetRole(ctx context.Context, userID, role string) error
	ListPendingUsers(ctx context.Context) ([]*fleet.User, error)
	SetStatus(ctx context.Context, userID, status string) error
}

// EnrollmentTokenStore manages enrollment token persistence.
type EnrollmentTokenStore interface {
	Create(ctx context.Context, token *fleet.EnrollmentToken) error
	Get(ctx context.Context, id string) (*fleet.EnrollmentToken, error)
	IncrementUses(ctx context.Context, id string) error
	ListByOrg(ctx context.Context, orgID string) ([]*fleet.EnrollmentToken, error)
	Delete(ctx context.Context, id string) error
}

// AgentStore manages agent persistence. All operations are org-scoped.
type AgentStore interface {
	Create(ctx context.Context, agent *fleet.Agent) error
	Get(ctx context.Context, orgID, id string) (*fleet.Agent, error)
	GetByHostname(ctx context.Context, orgID, hostname string) (*fleet.Agent, error)
	List(ctx context.Context, orgID string, opts fleet.AgentListOptions) ([]*fleet.Agent, int, error)
	Update(ctx context.Context, agent *fleet.Agent) error
	Delete(ctx context.Context, orgID, id string) error
	UpdateHeartbeat(ctx context.Context, orgID, id string, hb *fleet.Heartbeat) error
	CountByStatus(ctx context.Context, orgID string) (map[fleet.AgentStatus]int, error)
	MarkOfflineAgents(ctx context.Context, timeout time.Duration) (int, error)
}

// RuleStore manages rule persistence. All operations are org-scoped
// except the *AcrossAccount methods which propagate to all orgs.
type RuleStore interface {
	Create(ctx context.Context, rule *fleet.Rule) error
	Get(ctx context.Context, orgID, id string) (*fleet.Rule, error)
	List(ctx context.Context, orgID string, opts fleet.ListOptions) ([]*fleet.Rule, int, error)
	Update(ctx context.Context, rule *fleet.Rule) error
	UpdateAcrossAccount(ctx context.Context, accountID string, rule *fleet.Rule) (int, error)
	Delete(ctx context.Context, orgID, id string) error
	DeleteAcrossAccount(ctx context.Context, accountID, ruleID string) error
	DeleteBySource(ctx context.Context, orgID, source string) (int, error)
	DeleteBySourceExcept(ctx context.Context, orgID, source string, keepIDs []string) (int, error)
	CountBySource(ctx context.Context, orgID, source string) (int, error)
	GetForAgent(ctx context.Context, orgID, agentID string) ([]*fleet.Rule, string, error)
	ListUserModifiedIDs(ctx context.Context, orgID, source string) (modified, disabled map[string]bool, err error)
	RecordSyncDeletion(ctx context.Context, orgID, ruleID, source string) error
	ListDeletedSyncIDs(ctx context.Context, orgID, source string) (map[string]bool, error)
}

// YARARuleStore manages server-side YARA rule persistence. Account-scoped:
// a single rule set is shared across every org the account owns. Used by
// the fleet yara_scan active-response command — the currently enabled rule
// set is embedded inline in its payload at command-creation time.
type YARARuleStore interface {
	Create(ctx context.Context, rule *fleet.YARARule) error
	Get(ctx context.Context, accountID, id string) (*fleet.YARARule, error)
	List(ctx context.Context, accountID string) ([]*fleet.YARARule, error)
	ListEnabled(ctx context.Context, accountID string) ([]*fleet.YARARule, error)
	Update(ctx context.Context, rule *fleet.YARARule) error
	Delete(ctx context.Context, accountID, id string) error
}

// GroupStore manages agent group persistence. All operations are org-scoped.
type GroupStore interface {
	Create(ctx context.Context, group *fleet.AgentGroup) error
	Get(ctx context.Context, orgID, id string) (*fleet.AgentGroup, error)
	List(ctx context.Context, orgID string) ([]*fleet.AgentGroup, error)
	Update(ctx context.Context, group *fleet.AgentGroup) error
	Delete(ctx context.Context, orgID, id string) error
	AssignRules(ctx context.Context, groupID string, ruleIDs []string) error
}

// DetectionStore manages detection persistence and querying. All operations are org-scoped.
type DetectionStore interface {
	Create(ctx context.Context, det *fleet.Detection) error
	Get(ctx context.Context, orgID, id string) (*fleet.Detection, error)
	List(ctx context.Context, orgID string, opts fleet.DetectionListOptions) ([]*fleet.Detection, int, error)
	Count24h(ctx context.Context, orgID string) (int, error)
	CountBySeverity(ctx context.Context, orgID string) (map[string]int, error)
	Timeline(ctx context.Context, orgID string, from, to time.Time, interval string) ([]fleet.TimelineBucket, error)
	MitreHeatmap(ctx context.Context, orgID string, from, to time.Time) ([]fleet.MitreCell, error)
	TopNoisyRules(ctx context.Context, orgID string, limit int) ([]fleet.RuleDetectionCount, error)
}

// GlobalRuleStore manages system-wide rules that apply to all organizations.
type GlobalRuleStore interface {
	Create(ctx context.Context, rule *fleet.Rule) error
	Get(ctx context.Context, id string) (*fleet.Rule, error)
	List(ctx context.Context, opts fleet.ListOptions) ([]*fleet.Rule, int, error)
	Update(ctx context.Context, rule *fleet.Rule) error
	Delete(ctx context.Context, id string) error
	GetForOrg(ctx context.Context, orgID string) ([]*fleet.Rule, error)
	SetOrgOverride(ctx context.Context, orgID, ruleID string, enabled bool) error
	GetOrgOverrides(ctx context.Context, orgID string) (map[string]bool, error)
}

// CommandStore manages command queue persistence.
type CommandStore interface {
	Create(ctx context.Context, cmd *fleet.Command) error
	GetPendingForAgent(ctx context.Context, agentID string) ([]*fleet.Command, error)
	MarkRunning(ctx context.Context, id string) error
	SetResult(ctx context.Context, id string, status string, result json.RawMessage, errMsg string) error
	ListByAgent(ctx context.Context, orgID, agentID string, limit int) ([]*fleet.Command, error)
	Get(ctx context.Context, orgID, id string) (*fleet.Command, error)
	HasRecentCommand(ctx context.Context, agentID, cmdType string, cooldown time.Duration) (bool, error)
}

// MacroStore manages macro persistence. All operations are org-scoped.
type MacroStore interface {
	Create(ctx context.Context, macro *fleet.Macro) error
	Get(ctx context.Context, orgID, id string) (*fleet.Macro, error)
	List(ctx context.Context, orgID string) ([]*fleet.Macro, error)
	Update(ctx context.Context, macro *fleet.Macro) error
	Delete(ctx context.Context, orgID, id string) error
	ImportFromYAML(ctx context.Context, orgID string, data []byte) (int, error)
	GetAllForOrg(ctx context.Context, orgID string) (string, error) // returns combined YAML for agent sync
}

// AuditStore manages audit log persistence.
type AuditStore interface {
	Log(ctx context.Context, entry *fleet.AuditEntry) error
	List(ctx context.Context, orgID string, opts fleet.ListOptions) ([]*fleet.AuditEntry, int, error)
}

// APIKeyStore manages user API key persistence.
type APIKeyStore interface {
	Create(ctx context.Context, key *fleet.APIKey) error
	List(ctx context.Context, userID string) ([]*fleet.APIKey, error)
	Delete(ctx context.Context, id, userID string) error
	ValidateKey(ctx context.Context, keyHash string) (*fleet.APIKey, error)
}

// UserGroupStore manages user group persistence.
type UserGroupStore interface {
	Create(ctx context.Context, group *fleet.UserGroup) error
	Get(ctx context.Context, id string) (*fleet.UserGroup, error)
	List(ctx context.Context, accountID string) ([]*fleet.UserGroup, error)
	Update(ctx context.Context, group *fleet.UserGroup) error
	Delete(ctx context.Context, id string) error
	AddMember(ctx context.Context, userID, groupID string) error
	RemoveMember(ctx context.Context, userID, groupID string) error
	GetUserGroups(ctx context.Context, userID string) ([]fleet.UserGroupMembership, error)
	GetEffectivePermissions(ctx context.Context, userID string) ([]string, error)
}

// CaptureStore manages kernel capture session persistence.
type CaptureStore interface {
	Create(ctx context.Context, cap *fleet.Capture) error
	Get(ctx context.Context, orgID, id string) (*fleet.Capture, error)
	ListByAgent(ctx context.Context, orgID, agentID string) ([]*fleet.Capture, error)
	Update(ctx context.Context, cap *fleet.Capture) error
	Delete(ctx context.Context, orgID, id string) error
	IngestEvents(ctx context.Context, captureID, orgID string, events []json.RawMessage) error
	GetEvents(ctx context.Context, captureID string, opts CaptureEventSearchOpts) ([]CaptureEvent, int, error)
	IncrementEventCount(ctx context.Context, captureID string, count int) error
}

// CaptureEvent represents a single kernel event within a capture session.
type CaptureEvent struct {
	ID             int64           `json:"id"`
	CaptureID      string          `json:"capture_id"`
	OrgID          string          `json:"org_id"`
	Seq            int64           `json:"seq"`
	Timestamp      time.Time       `json:"timestamp"`
	EventName      string          `json:"event_name"`
	EventCategory  string          `json:"event_category"`
	PID            int             `json:"pid"`
	ProcessName    string          `json:"process_name"`
	ProcessExe     string          `json:"process_exe"`
	ProcessCmdline string          `json:"process_cmdline"`
	ParentPID      int             `json:"parent_pid"`
	ParentName     string          `json:"parent_name"`
	Params         json.RawMessage `json:"params"`
	RawEvent       json.RawMessage `json:"raw_event"`
}

// CaptureEventSearchOpts defines search filters for capture events.
type CaptureEventSearchOpts struct {
	EventName   string
	ProcessName string
	PID         int
	Search      string // full text search
	AfterId     int64  // cursor-based pagination: return events with id > AfterId
	Limit       int
	Offset      int
}

// TelemetryStore manages telemetry event persistence and search.
type TelemetryStore interface {
	BulkIngest(ctx context.Context, orgID, agentID, hostname string, events []json.RawMessage) error
	Search(ctx context.Context, orgID string, opts TelemetrySearchOpts) ([]TelemetryEvent, int, error)
	GetLatestForAgent(ctx context.Context, orgID, agentID string, limit int) ([]TelemetryEvent, error)
	Purge(ctx context.Context, retentionDays int) (int64, error)
	CountByAgent(ctx context.Context, orgID string) (map[string]int64, error)
	GetFieldValues(ctx context.Context, orgID string) (map[string][]string, error)
}

// TelemetryEvent represents a kernel event forwarded by an agent.
type TelemetryEvent struct {
	ID             int64           `json:"id"`
	OrgID          string          `json:"org_id"`
	AgentID        string          `json:"agent_id"`
	AgentHostname  string          `json:"agent_hostname"`
	Seq            int64           `json:"seq"`
	Timestamp      time.Time       `json:"timestamp"`
	EventName      string          `json:"event_name"`
	EventCategory  string          `json:"event_category"`
	PID            int             `json:"pid"`
	TID            int             `json:"tid"`
	ProcessName    string          `json:"process_name"`
	ProcessExe     string          `json:"process_exe"`
	ProcessCmdline string          `json:"process_cmdline"`
	ParentPID      int             `json:"parent_pid"`
	ParentName     string          `json:"parent_name"`
	Params         json.RawMessage `json:"params"`
	Metadata       json.RawMessage `json:"metadata"`
	RawEvent       json.RawMessage `json:"raw_event"`
}

// EventLogPolicyStore manages event log collection policies.
type EventLogPolicyStore interface {
	Get(ctx context.Context, orgID string) (*fleet.EventLogPolicy, error)
	Upsert(ctx context.Context, policy *fleet.EventLogPolicy) error
}

// TelemetrySearchOpts defines search filters for telemetry events.
type TelemetrySearchOpts struct {
	AgentID     string
	EventName   string
	ProcessName string
	PID         int
	ParentPID   int
	Search      string // full text search across process_name, process_exe, process_cmdline, event_name
	Query       string // Fibratus QL expression (e.g., "ps.name = 'cmd.exe' and kevt.name = 'CreateProcess'")
	From        time.Time
	To          time.Time
	Limit       int
	Offset      int
}
