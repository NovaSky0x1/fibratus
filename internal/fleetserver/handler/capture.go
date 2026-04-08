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

// CaptureHandler handles kernel capture API requests.
type CaptureHandler struct {
	captures     store.CaptureStore
	agents       store.AgentStore
	commands     store.CommandStore
	audit        store.AuditStore
	users        store.UserStore
	onCmdCreated CommandPushCallback
}

// NewCaptureHandler creates a new capture handler.
func NewCaptureHandler(captures store.CaptureStore, agents store.AgentStore, commands store.CommandStore, audit store.AuditStore, users store.UserStore) *CaptureHandler {
	return &CaptureHandler{captures: captures, agents: agents, commands: commands, audit: audit, users: users}
}

// SetCommandPushCallback registers a callback for instant command delivery.
func (h *CaptureHandler) SetCommandPushCallback(cb CommandPushCallback) {
	h.onCmdCreated = cb
}

// CreateCapture handles POST /api/v1/orgs/{org_id}/agents/{agent_id}/captures
func (h *CaptureHandler) CreateCapture(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	agentID := extractCaptureAgentID(r.URL.Path)
	if orgID == "" || agentID == "" {
		writeError(w, http.StatusBadRequest, "org and agent ID required")
		return
	}

	var req struct {
		Filter      string `json:"filter"`
		DurationSec int    `json:"duration_sec"`
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

	userID := ctxutil.UserIDFromContext(r.Context())

	captureID := GenerateID()
	cap := &fleet.Capture{
		ID:            captureID,
		OrgID:         orgID,
		AgentID:       agentID,
		AgentHostname: agent.Hostname,
		Filter:        req.Filter,
		Status:        fleet.CaptureActive,
		DurationSec:   req.DurationSec,
		CreatedBy:     userID,
		StartedAt:     time.Now().UTC(),
	}

	if err := h.captures.Create(r.Context(), cap); err != nil {
		log.Errorf("fleet: create capture error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create capture")
		return
	}

	// Send start_capture command to agent with capture_id
	payload, _ := json.Marshal(map[string]interface{}{
		"capture_id":  captureID,
		"filter":      req.Filter,
		"duration":    req.DurationSec,
	})

	cmd := &fleet.Command{
		ID:        GenerateID(),
		OrgID:     orgID,
		AgentID:   agentID,
		Type:      fleet.CmdStartCapture,
		Payload:   payload,
		Status:    fleet.CmdStatusPending,
		CreatedBy: userID,
		CreatedAt: time.Now().UTC(),
	}
	if err := h.commands.Create(r.Context(), cmd); err != nil {
		log.Errorf("fleet: create capture command error: %v", err)
	}

	// Push command instantly via gRPC if agent connected
	if h.onCmdCreated != nil {
		if h.onCmdCreated(agentID, cmd.ID, cmd.Type, cmd.Payload) {
			h.commands.MarkRunning(r.Context(), cmd.ID)
		}
	}

	logAudit(r, h.audit, h.users, userID, orgID, "start", "capture", captureID, "kernel capture",
		map[string]interface{}{"agent_id": agentID, "filter": req.Filter, "duration_sec": req.DurationSec})

	log.WithFields(log.Fields{
		"capture": captureID,
		"agent":   agentID,
		"filter":  req.Filter,
	}).Info("fleet: capture started")

	writeJSON(w, http.StatusCreated, fleet.Response{Data: cap})
}

// ListCaptures handles GET /api/v1/orgs/{org_id}/agents/{agent_id}/captures
func (h *CaptureHandler) ListCaptures(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	agentID := extractCaptureAgentID(r.URL.Path)
	if orgID == "" || agentID == "" {
		writeError(w, http.StatusBadRequest, "org and agent ID required")
		return
	}

	captures, err := h.captures.ListByAgent(r.Context(), orgID, agentID)
	if err != nil {
		log.Errorf("fleet: list captures error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: captures})
}

// GetCapture handles GET /api/v1/orgs/{org_id}/captures/{id}
func (h *CaptureHandler) GetCapture(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	captureID := extractLastPathSegment(r.URL.Path)
	if orgID == "" || captureID == "" {
		writeError(w, http.StatusBadRequest, "org and capture ID required")
		return
	}

	cap, err := h.captures.Get(r.Context(), orgID, captureID)
	if err != nil {
		writeError(w, http.StatusNotFound, "capture not found")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: cap})
}

// GetCaptureEvents handles GET /api/v1/orgs/{org_id}/captures/{id}/events
func (h *CaptureHandler) GetCaptureEvents(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	// Path: /captures/{id}/events — extract capture ID
	captureID := extractPathParam(r.URL.Path, "/captures/", "/events")
	if orgID == "" || captureID == "" {
		writeError(w, http.StatusBadRequest, "org and capture ID required")
		return
	}

	q := r.URL.Query()
	opts := store.CaptureEventSearchOpts{
		EventName:   q.Get("event_name"),
		ProcessName: q.Get("process_name"),
		Search:      q.Get("search"),
		Limit:       200,
	}

	if v := q.Get("after_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			opts.AfterId = id
		}
	}
	if v := q.Get("pid"); v != "" {
		if pid, err := strconv.Atoi(v); err == nil {
			opts.PID = pid
		}
	}
	if v := q.Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 {
			opts.Limit = l
		}
	}

	events, total, err := h.captures.GetEvents(r.Context(), captureID, opts)
	if err != nil {
		log.Errorf("fleet: get capture events error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{
		Data: events,
		Meta: &fleet.Pagination{Total: total},
	})
}

// StopCapture handles POST /api/v1/orgs/{org_id}/captures/{id}/stop
func (h *CaptureHandler) StopCapture(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	captureID := extractPathParam(r.URL.Path, "/captures/", "/stop")
	if orgID == "" || captureID == "" {
		writeError(w, http.StatusBadRequest, "org and capture ID required")
		return
	}

	cap, err := h.captures.Get(r.Context(), orgID, captureID)
	if err != nil {
		writeError(w, http.StatusNotFound, "capture not found")
		return
	}

	if cap.Status != fleet.CaptureActive {
		writeError(w, http.StatusBadRequest, "capture is not active")
		return
	}

	now := time.Now().UTC()
	cap.Status = fleet.CaptureCompleted
	cap.CompletedAt = &now
	if err := h.captures.Update(r.Context(), cap); err != nil {
		log.Errorf("fleet: stop capture error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to stop capture")
		return
	}

	// Send stop_capture command to agent
	payload, _ := json.Marshal(map[string]interface{}{
		"capture_id": captureID,
	})
	cmd := &fleet.Command{
		ID:        GenerateID(),
		OrgID:     orgID,
		AgentID:   cap.AgentID,
		Type:      fleet.CmdStopCapture,
		Payload:   payload,
		Status:    fleet.CmdStatusPending,
		CreatedBy: ctxutil.UserIDFromContext(r.Context()),
		CreatedAt: time.Now().UTC(),
	}
	if err := h.commands.Create(r.Context(), cmd); err != nil {
		log.Errorf("fleet: create stop capture command error: %v", err)
	}
	if h.onCmdCreated != nil {
		if h.onCmdCreated(cap.AgentID, cmd.ID, cmd.Type, cmd.Payload) {
			h.commands.MarkRunning(r.Context(), cmd.ID)
		}
	}

	userID := ctxutil.UserIDFromContext(r.Context())
	logAudit(r, h.audit, h.users, userID, orgID, "stop", "capture", captureID, "kernel capture",
		map[string]interface{}{"agent_id": cap.AgentID, "event_count": cap.EventCount})

	log.WithFields(log.Fields{
		"capture":     captureID,
		"agent":       cap.AgentID,
		"event_count": cap.EventCount,
	}).Info("fleet: capture stopped")

	writeJSON(w, http.StatusOK, fleet.Response{Data: cap})
}

// DeleteCapture handles DELETE /api/v1/orgs/{org_id}/captures/{id}
func (h *CaptureHandler) DeleteCapture(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	captureID := extractLastPathSegment(r.URL.Path)
	if orgID == "" || captureID == "" {
		writeError(w, http.StatusBadRequest, "org and capture ID required")
		return
	}

	if err := h.captures.Delete(r.Context(), orgID, captureID); err != nil {
		log.Errorf("fleet: delete capture error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete capture")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "deleted"}})
}

// extractCaptureAgentID extracts the agent ID from paths like /agents/{id}/captures
func extractCaptureAgentID(path string) string {
	parts := strings.Split(path, "/agents/")
	if len(parts) < 2 {
		return ""
	}
	rest := parts[1]
	if idx := strings.Index(rest, "/"); idx > 0 {
		return rest[:idx]
	}
	return rest
}

// extractLastPathSegment returns the last non-empty path segment.
func extractLastPathSegment(path string) string {
	path = strings.TrimRight(path, "/")
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}
