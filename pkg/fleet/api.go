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

// ═══════════════════════════════════════════════════════════════
// Authentication
// ═══════════════════════════════════════════════════════════════

// SignupRequest creates a new account, organization, and admin user.
type SignupRequest struct {
	AccountName string `json:"account_name"`
	OrgName     string `json:"org_name"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	Password    string `json:"password"`
}

// SignupResponse is returned after successful account creation.
type SignupResponse struct {
	AccountID string `json:"account_id"`
	OrgID     string `json:"org_id"`
	UserID    string `json:"user_id"`
	Token     string `json:"token"` // JWT
}

// LoginRequest authenticates a dashboard user.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse is returned after successful authentication.
type LoginResponse struct {
	Token string `json:"token"` // JWT
	User  User   `json:"user"`
}

// ═══════════════════════════════════════════════════════════════
// Enrollment (agent → server)
// ═══════════════════════════════════════════════════════════════

// EnrollRequest is sent by an agent during enrollment.
type EnrollRequest struct {
	Token         string `json:"token"`
	Hostname      string `json:"hostname"`
	OSVersion     string `json:"os_version"`
	EngineVersion string `json:"engine_version"`
	CSR           string `json:"csr"` // PEM-encoded certificate signing request
}

// EnrollResponse is returned to the agent after successful enrollment.
type EnrollResponse struct {
	AgentID    string `json:"agent_id"`
	OrgID      string `json:"org_id"`
	SignedCert string `json:"signed_cert"` // PEM-encoded signed certificate
	CACert     string `json:"ca_cert"`     // PEM-encoded CA certificate
}

// ═══════════════════════════════════════════════════════════════
// Agent API (mTLS authenticated)
// ═══════════════════════════════════════════════════════════════

// RegisterRequest is the payload sent by an agent during registration.
// Kept for backward compatibility; new agents use EnrollRequest.
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

// ═══════════════════════════════════════════════════════════════
// Dashboard API (JWT authenticated, org-scoped)
// ═══════════════════════════════════════════════════════════════

// CreateEnrollmentTokenRequest creates a new enrollment token for an org.
type CreateEnrollmentTokenRequest struct {
	Name      string        `json:"name"`
	MaxUses   int           `json:"max_uses"`
	ExpiresIn time.Duration `json:"expires_in"` // e.g., 24h
}

// CreateOrgRequest creates a new organization within an account.
type CreateOrgRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
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
	AgentID  string `json:"agent_id,omitempty"`
	Severity string `json:"severity,omitempty"`
	TacticID string `json:"tactic_id,omitempty"`
	RuleID   string `json:"rule_id,omitempty"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
}

// ═══════════════════════════════════════════════════════════════
// Command queue (active response)
// ═══════════════════════════════════════════════════════════════

// CreateCommandRequest is sent by dashboard to queue a command for an agent.
type CreateCommandRequest struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// CommandResultRequest is sent by an agent to report command execution results.
type CommandResultRequest struct {
	Status       string          `json:"status"`
	Result       json.RawMessage `json:"result,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
}
