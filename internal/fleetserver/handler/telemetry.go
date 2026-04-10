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
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// TelemetryHandler handles telemetry event API requests.
type TelemetryHandler struct {
	telemetry store.TelemetryStore
	agents    store.AgentStore
	orgs      store.OrgStore
}

// SetOrgStore sets the org store for cross-org aggregation.
func (h *TelemetryHandler) SetOrgStore(orgs store.OrgStore) { h.orgs = orgs }

// NewTelemetryHandler creates a new telemetry handler.
func NewTelemetryHandler(telemetry store.TelemetryStore, agents store.AgentStore) *TelemetryHandler {
	return &TelemetryHandler{telemetry: telemetry, agents: agents}
}

// Ingest handles POST /api/v1/agent/telemetry
func (h *TelemetryHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	agentID := r.Header.Get("X-Agent-ID")
	orgID := r.Header.Get("X-Org-ID")

	// Fall back to context for mTLS-authenticated agents
	if agentID == "" {
		agentID = ctxutil.AgentIDFromContext(r.Context())
	}
	if orgID == "" {
		orgID = ctxutil.OrgIDFromContext(r.Context())
	}

	var reader io.Reader = io.LimitReader(r.Body, 50<<20) // 50 MB limit

	// Support gzip-compressed bodies
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(reader)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid gzip body")
			return
		}
		defer gz.Close()
		reader = gz
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var events []json.RawMessage
	if err := json.Unmarshal(body, &events); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON array")
		return
	}

	if len(events) == 0 {
		writeJSON(w, http.StatusAccepted, fleet.Response{Data: map[string]string{"status": "ok", "ingested": "0"}})
		return
	}

	// Resolve agent hostname
	hostname := ""
	if agentID != "" && orgID != "" {
		agent, _ := h.agents.Get(r.Context(), orgID, agentID)
		if agent != nil {
			hostname = agent.Hostname
		}
	}

	if err := h.telemetry.BulkIngest(r.Context(), orgID, agentID, hostname, events); err != nil {
		log.Errorf("fleet: telemetry ingest error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to ingest telemetry")
		return
	}

	log.WithFields(log.Fields{
		"agent":  agentID,
		"org":    orgID,
		"events": len(events),
	}).Info("fleet: telemetry ingested")

	writeJSON(w, http.StatusAccepted, fleet.Response{Data: map[string]interface{}{"status": "ok", "ingested": len(events)}})
}

// Search handles GET /api/v1/orgs/{org_id}/telemetry
func (h *TelemetryHandler) Search(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		// Cross-org: search first org (ClickHouse per-org tables can't easily aggregate)
		orgIDs := accountOrgIDs(r, h.orgs)
		if len(orgIDs) > 0 {
			orgID = orgIDs[0] // Use first org for now
		} else {
			writeError(w, http.StatusBadRequest, "org context required")
			return
		}
	}

	q := r.URL.Query()
	from := parseTime(q.Get("from"), time.Now().UTC().Add(-1*time.Hour))
	to := parseTime(q.Get("to"), time.Now().UTC())

	opts := store.TelemetrySearchOpts{
		AgentID:     q.Get("agent_id"),
		EventName:   q.Get("event_name"),
		ProcessName: q.Get("process_name"),
		PID:         intParam(r, "pid", 0),
		Search:      q.Get("search"),
		Query:       q.Get("query"),
		From:        from,
		To:          to,
		Limit:       intParam(r, "limit", 100),
		Offset:      intParam(r, "offset", 0),
	}

	events, total, err := h.telemetry.Search(r.Context(), orgID, opts)
	if err != nil {
		log.Errorf("fleet: telemetry search error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{
		Data: events,
		Meta: &fleet.Pagination{Total: total, Page: opts.Offset/opts.Limit + 1, PerPage: opts.Limit},
	})
}

// GetFieldValues handles GET /api/v1/orgs/{org_id}/telemetry/fields
// Returns distinct values for indexable fields (event types, categories, top processes, agents)
func (h *TelemetryHandler) GetFieldValues(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		orgIDs := accountOrgIDs(r, h.orgs)
		if len(orgIDs) > 0 {
			orgID = orgIDs[0]
		} else {
			writeError(w, http.StatusBadRequest, "org context required")
			return
		}
	}

	values, err := h.telemetry.GetFieldValues(r.Context(), orgID)
	if err != nil {
		log.Errorf("fleet: field values error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: values})
}

// GetLiveEvents handles GET /api/v1/orgs/{org_id}/agents/{id}/events
func (h *TelemetryHandler) GetLiveEvents(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}

	// Extract agent ID: path is /api/v1/orgs/{org_id}/agents/{agent_id}/events
	parts := strings.Split(r.URL.Path, "/agents/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}
	agentID := strings.TrimSuffix(parts[1], "/events")
	agentID = strings.TrimSuffix(agentID, "/events/")
	agentID = strings.Trim(agentID, "/")
	if agentID == "" || strings.Contains(agentID, "/") {
		writeError(w, http.StatusBadRequest, "agent ID required")
		return
	}

	events, err := h.telemetry.GetLatestForAgent(r.Context(), orgID, agentID, 100)
	if err != nil {
		log.Errorf("fleet: get live events error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: events})
}

// ProcessTree handles GET /api/v1/orgs/{org_id}/telemetry/process-tree?agent_id=X&pid=Y&timestamp=Z
// Returns process events around a specific PID for process tree visualization.
func (h *TelemetryHandler) ProcessTree(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	agentID := r.URL.Query().Get("agent_id")
	pidStr := r.URL.Query().Get("pid")
	tsStr := r.URL.Query().Get("timestamp")

	if agentID == "" || pidStr == "" {
		writeError(w, http.StatusBadRequest, "agent_id and pid required")
		return
	}

	// Parse timestamp or use now
	var ts time.Time
	if tsStr != "" {
		var err error
		ts, err = time.Parse(time.RFC3339Nano, tsStr)
		if err != nil {
			ts = time.Now().UTC()
		}
	} else {
		ts = time.Now().UTC()
	}

	from := ts.Add(-1 * time.Hour)
	to := ts.Add(1 * time.Hour)

	events, _, err := h.telemetry.Search(r.Context(), orgID, store.TelemetrySearchOpts{
		AgentID: agentID,
		From:    from,
		To:      to,
		Limit:   5000,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query telemetry")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{
		"events":    events,
		"focus_pid": pidStr,
	}})
}
