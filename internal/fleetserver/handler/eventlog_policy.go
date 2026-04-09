/*
 * Copyright 2021-2026 by Nedim Sabic Sabic
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
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// EventLogPolicyHandler handles event log collection policy API requests.
type EventLogPolicyHandler struct {
	policies     store.EventLogPolicyStore
	agents       store.AgentStore
	commands     store.CommandStore
	onCmdCreated CommandPushCallback
}

// NewEventLogPolicyHandler creates a new event log policy handler.
func NewEventLogPolicyHandler(
	policies store.EventLogPolicyStore,
	agents store.AgentStore,
	commands store.CommandStore,
) *EventLogPolicyHandler {
	return &EventLogPolicyHandler{
		policies: policies,
		agents:   agents,
		commands: commands,
	}
}

// SetCommandPushCallback registers a callback for instant command delivery.
func (h *EventLogPolicyHandler) SetCommandPushCallback(cb CommandPushCallback) {
	h.onCmdCreated = cb
}

// Get returns the event log policy for the org.
// GET /api/v1/orgs/{org_id}/eventlog-policy
func (h *EventLogPolicyHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id required")
		return
	}

	policy, err := h.policies.Get(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if policy == nil {
		writeJSON(w, http.StatusOK, fleet.Response{Data: &fleet.EventLogPolicy{
			OrgID:   orgID,
			Enabled: false,
		}})
		return
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: policy})
}

// Upsert creates or updates the event log policy and propagates to agents.
// PUT /api/v1/orgs/{org_id}/eventlog-policy
func (h *EventLogPolicyHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id required")
		return
	}

	var req fleet.EventLogPolicy
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.OrgID = orgID
	if req.ID == "" {
		req.ID = GenerateID()
	}

	if err := h.policies.Upsert(r.Context(), &req); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	log.Infof("fleet: eventlog policy updated for org %s (enabled=%v, channels=%d)", orgID, req.Enabled, len(req.Channels))

	// Propagate to all agents in the org
	h.propagateEventLogPolicy(r.Context(), orgID, &req)

	saved, _ := h.policies.Get(r.Context(), orgID)
	if saved != nil {
		writeJSON(w, http.StatusOK, fleet.Response{Data: saved})
	} else {
		writeJSON(w, http.StatusOK, fleet.Response{Data: req})
	}
}

// propagateEventLogPolicy queues set_eventlog_policy commands to all agents
// in the given org. Follows the same pattern as tamper protection propagation.
func (h *EventLogPolicyHandler) propagateEventLogPolicy(ctx context.Context, orgID string, policy *fleet.EventLogPolicy) {
	agents, _, err := h.agents.List(ctx, orgID, fleet.AgentListOptions{
		ListOptions: fleet.ListOptions{PerPage: 10000},
	})
	if err != nil {
		log.Errorf("fleet: failed to list agents for eventlog policy propagation in org %s: %v", orgID, err)
		return
	}

	payload, _ := json.Marshal(policy)
	for _, agent := range agents {
		cmd := &fleet.Command{
			ID:        GenerateID(),
			OrgID:     orgID,
			AgentID:   agent.ID,
			Type:      fleet.CmdSetEventLogPolicy,
			Payload:   payload,
			Status:    fleet.CmdStatusPending,
			CreatedBy: "system",
			CreatedAt: time.Now().UTC(),
		}
		if err := h.commands.Create(ctx, cmd); err != nil {
			log.Errorf("fleet: failed to queue eventlog policy command for agent %s: %v", agent.ID, err)
			continue
		}
		if h.onCmdCreated != nil {
			if h.onCmdCreated(agent.ID, cmd.ID, cmd.Type, cmd.Payload) {
				h.commands.MarkRunning(ctx, cmd.ID)
			}
		}
		log.Infof("fleet: queued eventlog policy for agent %s (%s)", agent.Hostname, agent.ID)
	}
}
