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

// AgentStatus represents the current status of an agent in the fleet.
type AgentStatus string

const (
	// AgentOnline indicates the agent is actively sending heartbeats.
	AgentOnline AgentStatus = "online"
	// AgentOffline indicates the agent has missed heartbeat thresholds.
	AgentOffline AgentStatus = "offline"
	// AgentStale indicates the agent has been offline for an extended period.
	AgentStale AgentStatus = "stale"
)

// Agent represents a Fibratus agent registered with the fleet server.
type Agent struct {
	ID             string            `json:"id"`
	Hostname       string            `json:"hostname"`
	OSVersion      string            `json:"os_version"`
	EngineVersion  string            `json:"engine_version"`
	GroupID        string            `json:"group_id,omitempty"`
	GroupName      string            `json:"group_name,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
	Status         AgentStatus       `json:"status"`
	LastHeartbeat  time.Time         `json:"last_heartbeat"`
	RegisteredAt   time.Time         `json:"registered_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
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
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description,omitempty"`
	Condition   string            `json:"condition"`
	Output      string            `json:"output,omitempty"`
	Severity    string            `json:"severity"`
	Labels      map[string]string `json:"labels,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	References  []string          `json:"references,omitempty"`
	RawYAML     string            `json:"raw_yaml"`
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// AgentGroup represents a logical grouping of agents for rule assignment.
type AgentGroup struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	RuleIDs     []string `json:"rule_ids,omitempty"`
	AgentCount  int      `json:"agent_count,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// FleetOverview contains aggregate fleet statistics for the dashboard.
type FleetOverview struct {
	TotalAgents       int            `json:"total_agents"`
	OnlineAgents      int            `json:"online_agents"`
	OfflineAgents     int            `json:"offline_agents"`
	TotalDetections24h int           `json:"total_detections_24h"`
	SeverityBreakdown map[string]int `json:"severity_breakdown"`
}

// TimelineBucket represents a single time bucket in a detection timeline.
type TimelineBucket struct {
	Timestamp  time.Time `json:"timestamp"`
	Count      int       `json:"count"`
	BySeverity map[string]int `json:"by_severity,omitempty"`
}

// MitreCell represents detection counts for a specific MITRE ATT&CK technique.
type MitreCell struct {
	TacticID    string `json:"tactic_id"`
	TacticName  string `json:"tactic_name"`
	TechniqueID string `json:"technique_id"`
	TechniqueName string `json:"technique_name"`
	Count       int    `json:"count"`
}
