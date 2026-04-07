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
	"strings"
	"time"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// allowedCommandTypes defines the set of valid command types.
var allowedCommandTypes = map[string]bool{
	fleet.CmdIsolate:     true,
	fleet.CmdUnisolate:   true,
	fleet.CmdKillProcess: true,
	fleet.CmdUninstall:   true,
	fleet.CmdListDir:     true,
	fleet.CmdGetFile:     true,
	fleet.CmdRunCommand:  true,
	fleet.CmdCollectInfo: true,

	fleet.CmdGetProcesses: true,
	fleet.CmdGetNetwork:   true,
	fleet.CmdGetServices:  true,
	fleet.CmdGetDrivers:   true,
	fleet.CmdGetAutoruns:  true,
	fleet.CmdGetSoftware:  true,
	fleet.CmdGetUsers:     true,
	fleet.CmdGetRegistry:  true,
	fleet.CmdStartCapture: true,
	fleet.CmdStopCapture:  true,
	fleet.CmdYaraScan:     true,
}

// CommandHandler handles command queue API requests.
// CommandPushCallback is called when a new command is created to push it
// to the agent's gRPC command stream (if connected).
type CommandPushCallback func(agentID, cmdID, cmdType string, payload []byte) bool

type CommandHandler struct {
	commands     store.CommandStore
	agents       store.AgentStore
	audit        store.AuditStore
	users        store.UserStore
	onCmdCreated CommandPushCallback
}

// NewCommandHandler creates a new command handler.
func NewCommandHandler(commands store.CommandStore, agents store.AgentStore, audit store.AuditStore, users store.UserStore) *CommandHandler {
	return &CommandHandler{commands: commands, agents: agents, audit: audit, users: users}
}

// SetCommandPushCallback registers a callback for instant command delivery.
func (h *CommandHandler) SetCommandPushCallback(cb CommandPushCallback) {
	h.onCmdCreated = cb
}

// CreateCommand handles POST /api/v1/orgs/{org_id}/agents/{id}/commands
func (h *CommandHandler) CreateCommand(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}

	// Extract agent ID from the path — segment between /agents/ and /commands
	parts := strings.Split(r.URL.Path, "/agents/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}
	agentID := strings.TrimSuffix(parts[1], "/commands")
	agentID = strings.TrimSuffix(agentID, "/commands/")
	agentID = strings.Trim(agentID, "/")
	if agentID == "" || strings.Contains(agentID, "/") {
		writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}

	var req fleet.CreateCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !allowedCommandTypes[req.Type] {
		writeError(w, http.StatusBadRequest, "invalid command type")
		return
	}

	// Verify the agent exists
	agent, err := h.agents.Get(r.Context(), orgID, agentID)
	if err != nil {
		log.Errorf("fleet: create command agent lookup error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	userID := ctxutil.UserIDFromContext(r.Context())
	userEmail := ""
	if h.users != nil && userID != "" {
		user, err := h.users.Get(r.Context(), userID)
		if err == nil && user != nil {
			userEmail = user.Email
		}
	}

	cmd := &fleet.Command{
		ID:             GenerateID(),
		OrgID:          orgID,
		AgentID:        agentID,
		Type:           req.Type,
		Payload:        req.Payload,
		Status:         fleet.CmdStatusPending,
		CreatedBy:      userID,
		CreatedByEmail: userEmail,
		CreatedAt:      time.Now().UTC(),
	}

	if err := h.commands.Create(r.Context(), cmd); err != nil {
		log.Errorf("fleet: create command error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create command")
		return
	}

	logAudit(r, h.audit, h.users, userID, orgID, "execute", "command", cmd.ID, req.Type,
		map[string]interface{}{"agent_id": agentID, "agent_hostname": agent.Hostname, "command_type": req.Type})

	// Try to push command directly to agent's gRPC stream
	if h.onCmdCreated != nil {
		if h.onCmdCreated(agentID, cmd.ID, cmd.Type, cmd.Payload) {
			cmd.Status = fleet.CmdStatusRunning
			h.commands.MarkRunning(r.Context(), cmd.ID)
		}
	}

	log.WithFields(log.Fields{
		"command": cmd.ID,
		"agent":   agentID,
		"type":    req.Type,
		"org":     orgID,
		"user":    userEmail,
	}).Info("fleet: command queued")

	writeJSON(w, http.StatusCreated, fleet.Response{Data: cmd})
}

// ListCommands handles GET /api/v1/orgs/{org_id}/agents/{id}/commands
func (h *CommandHandler) ListCommands(w http.ResponseWriter, r *http.Request) {
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
	agentID := strings.TrimSuffix(parts[1], "/commands")
	agentID = strings.TrimSuffix(agentID, "/commands/")
	agentID = strings.Trim(agentID, "/")
	if agentID == "" || strings.Contains(agentID, "/") {
		writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}

	commands, err := h.commands.ListByAgent(r.Context(), orgID, agentID, 50)
	if err != nil {
		log.Errorf("fleet: list commands error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: commands})
}

// PollCommands handles GET /api/v1/agent/commands
func (h *CommandHandler) PollCommands(w http.ResponseWriter, r *http.Request) {
	agentID := ctxutil.AgentIDFromContext(r.Context())
	if agentID == "" {
		agentID = r.Header.Get("X-Agent-ID")
	}
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent identity required")
		return
	}

	commands, err := h.commands.GetPendingForAgent(r.Context(), agentID)
	if err != nil {
		log.Errorf("fleet: poll commands error for %s: %v", agentID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Mark polled commands as running
	for _, cmd := range commands {
		if err := h.commands.MarkRunning(r.Context(), cmd.ID); err != nil {
			log.Errorf("fleet: mark running error for command %s: %v", cmd.ID, err)
		}
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: commands})
}

// ReportResult handles POST /api/v1/agent/commands/{id}/result
func (h *CommandHandler) ReportResult(w http.ResponseWriter, r *http.Request) {
	// Extract command ID from the path
	cmdID := extractPathParam(r.URL.Path, "/api/v1/agent/commands/", "/result")
	if cmdID == "" {
		writeError(w, http.StatusBadRequest, "command ID required")
		return
	}

	var req fleet.CommandResultRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Status != fleet.CmdStatusCompleted && req.Status != fleet.CmdStatusFailed {
		writeError(w, http.StatusBadRequest, "status must be 'completed' or 'failed'")
		return
	}

	if err := h.commands.SetResult(r.Context(), cmdID, req.Status, req.Result, req.ErrorMessage); err != nil {
		log.Errorf("fleet: report result error for command %s: %v", cmdID, err)
		writeError(w, http.StatusInternalServerError, "failed to update command result")
		return
	}

	log.WithFields(log.Fields{
		"command": cmdID,
		"status":  req.Status,
	}).Info("fleet: command result reported")

	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "ok"}})
}
