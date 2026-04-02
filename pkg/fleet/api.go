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

// Response is the standard API response envelope.
type Response struct {
	Data  interface{} `json:"data,omitempty"`
	Error *APIError   `json:"error,omitempty"`
	Meta  *Pagination `json:"meta,omitempty"`
}

// APIError represents an error returned by the fleet API.
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Pagination contains pagination metadata for list endpoints.
type Pagination struct {
	Total   int `json:"total"`
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
}

// RegisterRequest is the payload sent by an agent during registration.
type RegisterRequest struct {
	Hostname      string            `json:"hostname"`
	OSVersion     string            `json:"os_version"`
	EngineVersion string            `json:"engine_version"`
	AgentGroup    string            `json:"agent_group,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
}

// RegisterResponse is returned to the agent upon successful registration.
type RegisterResponse struct {
	AgentID   string `json:"agent_id"`
	RulesETag string `json:"rules_etag,omitempty"`
}

// HeartbeatResponse is returned to the agent after a heartbeat.
type HeartbeatResponse struct {
	Status   string   `json:"status"`
	Commands []string `json:"commands,omitempty"`
}

// DetectionRequest is the payload for reporting a detection.
type DetectionRequest struct {
	AgentID       string `json:"agent_id"`
	AgentHostname string `json:"agent_hostname"`
	Alert         []byte `json:"alert"`
}

// ListOptions contains common query parameters for list endpoints.
type ListOptions struct {
	Page    int    `json:"page"`
	PerPage int    `json:"per_page"`
	Search  string `json:"search,omitempty"`
	SortBy  string `json:"sort_by,omitempty"`
	Order   string `json:"order,omitempty"`
}

// AgentListOptions contains query parameters for the agent list endpoint.
type AgentListOptions struct {
	ListOptions
	GroupID string      `json:"group_id,omitempty"`
	Status  AgentStatus `json:"status,omitempty"`
}

// DetectionListOptions contains query parameters for the detection list endpoint.
type DetectionListOptions struct {
	ListOptions
	AgentID    string `json:"agent_id,omitempty"`
	Severity   string `json:"severity,omitempty"`
	TacticID   string `json:"tactic_id,omitempty"`
	RuleID     string `json:"rule_id,omitempty"`
	From       string `json:"from,omitempty"`
	To         string `json:"to,omitempty"`
}
