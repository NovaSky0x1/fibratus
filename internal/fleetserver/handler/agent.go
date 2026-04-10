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

package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// AgentHandler handles agent-related API requests.
type AgentHandler struct {
	agents         store.AgentStore
	accounts       store.AccountStore
	orgs           store.OrgStore
	commands       store.CommandStore
	eventlogPolicy store.EventLogPolicyStore
	onCmdCreated   CommandPushCallback
}

// NewAgentHandler creates a new agent handler.
func NewAgentHandler(agents store.AgentStore, accounts store.AccountStore, orgs store.OrgStore) *AgentHandler {
	return &AgentHandler{agents: agents, accounts: accounts, orgs: orgs}
}

// SetCommandDeps sets the command store and push callback for handlers that need to push commands.
func (h *AgentHandler) SetCommandDeps(commands store.CommandStore, eventlogPolicy store.EventLogPolicyStore, cb CommandPushCallback) {
	h.commands = commands
	h.eventlogPolicy = eventlogPolicy
	h.onCmdCreated = cb
}

// Register handles POST /api/v1/agents/register
// For Phase 1 agents using API key auth. The agent provides its
// org affiliation via X-Agent-Org header or defaults to finding
// one by hostname.
func (h *AgentHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req fleet.RegisterRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Hostname == "" {
		writeError(w, http.StatusBadRequest, "hostname is required")
		return
	}

	// For legacy API key agents, org_id comes from header or defaults
	orgID := r.Header.Get("X-Org-ID")
	if orgID == "" {
		orgID = "default"
	}

	now := time.Now().UTC()

	// Check if agent with same hostname already exists in this org
	existing, err := h.agents.GetByHostname(r.Context(), orgID, req.Hostname)
	if err != nil {
		log.Errorf("fleet: register lookup error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var agent *fleet.Agent

	if existing != nil {
		existing.OSVersion = req.OSVersion
		existing.EngineVersion = req.EngineVersion
		existing.Status = fleet.AgentOnline
		existing.LastHeartbeat = now
		existing.Tags = req.Tags
		if err := h.agents.Update(r.Context(), existing); err != nil {
			log.Errorf("fleet: register update error: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		agent = existing
	} else {
		agent = &fleet.Agent{
			ID:            GenerateID(),
			OrgID:         orgID,
			Hostname:      req.Hostname,
			OSVersion:     req.OSVersion,
			EngineVersion: req.EngineVersion,
			Tags:          req.Tags,
			Status:        fleet.AgentOnline,
			LastHeartbeat: now,
			RegisteredAt:  now,
		}
		if err := h.agents.Create(r.Context(), agent); err != nil {
			log.Errorf("fleet: register create error: %v", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	log.Infof("fleet: agent registered: %s (%s) in org %s", agent.Hostname, agent.ID, orgID)

	resp := fleet.RegisterResponse{AgentID: agent.ID}
	writeJSON(w, http.StatusCreated, fleet.Response{Data: resp})
}

// Heartbeat handles POST /api/v1/agents/{id}/heartbeat
func (h *AgentHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	agentID := extractPathParam(r.URL.Path, "/api/v1/agents/", "/heartbeat")
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}

	var hb fleet.Heartbeat
	if err := decodeBody(r, &hb); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if hb.Timestamp.IsZero() {
		hb.Timestamp = time.Now().UTC()
	}

	// For agent routes, org_id comes from agent identity or header
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		orgID = r.Header.Get("X-Org-ID")
	}

	if err := h.agents.UpdateHeartbeat(r.Context(), orgID, agentID, &hb); err != nil {
		log.Errorf("fleet: heartbeat error for %s: %v", agentID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := fleet.HeartbeatResponse{Status: "ok"}
	writeJSON(w, http.StatusOK, fleet.Response{Data: resp})
}

// List handles GET /api/v1/orgs/{org_id}/agents
func (h *AgentHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}

	opts := fleet.AgentListOptions{
		ListOptions: fleet.ListOptions{
			Page:    intParam(r, "page", 1),
			PerPage: intParam(r, "per_page", 50),
			Search:  r.URL.Query().Get("search"),
		},
		GroupID: r.URL.Query().Get("group_id"),
		Status:  fleet.AgentStatus(r.URL.Query().Get("status")),
	}

	agents, total, err := h.agents.List(r.Context(), orgID, opts)
	if err != nil {
		log.Errorf("fleet: list agents error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{
		Data: agents,
		Meta: &fleet.Pagination{Total: total, Page: opts.Page, PerPage: opts.PerPage},
	})
}

// Get handles GET /api/v1/orgs/{org_id}/agents/{id}
func (h *AgentHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}

	// Extract agent ID from the path — last segment after /agents/
	parts := strings.Split(r.URL.Path, "/agents/")
	if len(parts) < 2 || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}
	agentID := strings.TrimSuffix(parts[1], "/")

	agent, err := h.agents.Get(r.Context(), orgID, agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: agent})
}

// Delete handles DELETE /api/v1/orgs/{org_id}/agents/{id}
func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}

	parts := strings.Split(r.URL.Path, "/agents/")
	if len(parts) < 2 || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}
	agentID := strings.TrimSuffix(parts[1], "/")

	if err := h.agents.Delete(r.Context(), orgID, agentID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SetTamperProtection handles PUT /api/v1/orgs/{org_id}/agents/{id}/tamper-protection
func (h *AgentHandler) SetTamperProtection(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}

	parts := strings.Split(r.URL.Path, "/agents/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}
	agentID := strings.TrimSuffix(parts[1], "/tamper-protection")
	agentID = strings.TrimSuffix(agentID, "/")

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	agent, err := h.agents.Get(r.Context(), orgID, agentID)
	if err != nil || agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	// Block disabling if account-wide or org-wide tamper protection is enforced
	if !req.Enabled {
		accountID := ctxutil.AccountIDFromContext(r.Context())
		if accountID != "" {
			account, _ := h.accounts.Get(r.Context(), accountID)
			if account != nil && account.TamperProtectionEnabled {
				writeError(w, http.StatusForbidden, "tamper protection is enforced account-wide and cannot be disabled per-agent")
				return
			}
		}
		org, _ := h.orgs.Get(r.Context(), orgID)
		if org != nil && org.TamperProtectionEnabled {
			writeError(w, http.StatusForbidden, "tamper protection is enforced for this organization and cannot be disabled per-agent")
			return
		}
	}

	agent.TamperProtection = req.Enabled
	if err := h.agents.Update(r.Context(), agent); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update tamper protection")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]bool{"tamper_protection": req.Enabled}})
}

// SetEventLogCollection handles PUT /api/v1/orgs/{org_id}/agents/{id}/eventlog-collection
func (h *AgentHandler) SetEventLogCollection(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}

	parts := strings.Split(r.URL.Path, "/agents/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}
	agentID := strings.TrimSuffix(parts[1], "/eventlog-collection")
	agentID = strings.TrimSuffix(agentID, "/")

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	agent, err := h.agents.Get(r.Context(), orgID, agentID)
	if err != nil || agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	// Block disabling if account-wide event log collection is enforced
	if !req.Enabled {
		accountID := ctxutil.AccountIDFromContext(r.Context())
		if accountID != "" {
			account, _ := h.accounts.Get(r.Context(), accountID)
			if account != nil && account.EventLogEnabled {
				writeError(w, http.StatusForbidden, "event log collection is enforced account-wide and cannot be disabled per-agent")
				return
			}
		}
	}

	agent.EventLogCollection = req.Enabled
	if err := h.agents.Update(r.Context(), agent); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update event log collection")
		return
	}

	// Push the actual event log policy command to the agent
	if h.commands != nil && h.eventlogPolicy != nil {
		// Get the org's channel config (or use defaults)
		policy, _ := h.eventlogPolicy.Get(r.Context(), orgID)
		if policy == nil {
			policy = &fleet.EventLogPolicy{
				ID:      GenerateID(),
				OrgID:   orgID,
				Enabled: req.Enabled,
				Channels: []fleet.EventLogPolicyChannel{
					{Name: "Security", CollectAll: true},
					{Name: "System", CollectAll: true},
					{Name: "Microsoft-Windows-PowerShell/Operational", CollectAll: true},
					{Name: "Microsoft-Windows-Sysmon/Operational", CollectAll: true},
					{Name: "Microsoft-Windows-Windows Defender/Operational", CollectAll: true},
				},
			}
			h.eventlogPolicy.Upsert(r.Context(), policy)
		} else {
			policy.Enabled = req.Enabled
		}

		payload, _ := json.Marshal(policy)
		cmd := &fleet.Command{
			ID:        GenerateID(),
			OrgID:     orgID,
			AgentID:   agentID,
			Type:      fleet.CmdSetEventLogPolicy,
			Payload:   payload,
			Status:    fleet.CmdStatusPending,
			CreatedBy: "system",
			CreatedAt: time.Now().UTC(),
		}
		if err := h.commands.Create(r.Context(), cmd); err != nil {
			log.Errorf("fleet: failed to queue eventlog policy for agent %s: %v", agentID, err)
		} else if h.onCmdCreated != nil {
			if h.onCmdCreated(agentID, cmd.ID, cmd.Type, cmd.Payload) {
				h.commands.MarkRunning(r.Context(), cmd.ID)
			}
		}
		log.Infof("fleet: pushed eventlog policy (enabled=%v) to agent %s", req.Enabled, agent.Hostname)
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]bool{"eventlog_collection": req.Enabled}})
}

// HeartbeatHistory handles GET /api/v1/orgs/{org_id}/agents/{id}/heartbeat-history
func (h *AgentHandler) HeartbeatHistory(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}

	parts := strings.Split(r.URL.Path, "/agents/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}
	agentID := strings.TrimSuffix(parts[1], "/heartbeat-history")
	agentID = strings.TrimSuffix(agentID, "/")

	limit := intParam(r, "limit", 60)

	// Type-assert to access GetHeartbeatHistory (only on postgres.AgentStore)
	type heartbeatHistorian interface {
		GetHeartbeatHistory(ctx interface{}, orgID, agentID string, limit int) ([]fleet.Heartbeat, error)
	}
	historian, ok := h.agents.(heartbeatHistorian)
	if !ok {
		writeError(w, http.StatusNotImplemented, "heartbeat history not supported")
		return
	}

	history, err := historian.GetHeartbeatHistory(r.Context(), orgID, agentID, limit)
	if err != nil {
		log.Errorf("fleet: heartbeat history error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: history})
}

func intParam(r *http.Request, key string, defaultVal int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}
