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

// DetectionHandler handles detection-related API requests.
type DetectionHandler struct {
	detections store.DetectionStore
	agents     store.AgentStore
	telemetry  store.TelemetryStore
}

// NewDetectionHandler creates a new detection handler.
func NewDetectionHandler(detections store.DetectionStore, agents store.AgentStore, telemetry store.TelemetryStore) *DetectionHandler {
	return &DetectionHandler{detections: detections, agents: agents, telemetry: telemetry}
}

// Ingest handles POST /api/v1/detections (agent route, API key auth)
func (h *DetectionHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	agentID := r.Header.Get("X-Agent-ID")
	orgID := r.Header.Get("X-Org-ID")

	// Fall back to context for mTLS-authenticated agents
	if agentID == "" {
		agentID = ctxutil.AgentIDFromContext(r.Context())
	}
	if orgID == "" {
		orgID = ctxutil.OrgIDFromContext(r.Context())
	}

	var alertData struct {
		ID          string            `json:"id"`
		Title       string            `json:"title"`
		Severity    string            `json:"severity"`
		Text        string            `json:"text"`
		Description string            `json:"description"`
		Labels      map[string]string `json:"labels"`
		Events      json.RawMessage   `json:"events"`
	}

	if err := decodeBody(r, &alertData); err != nil {
		writeError(w, http.StatusBadRequest, "invalid alert JSON: "+err.Error())
		return
	}

	// Resolve agent hostname
	hostname := ""
	if agentID != "" && orgID != "" {
		agent, _ := h.agents.Get(r.Context(), orgID, agentID)
		if agent != nil {
			hostname = agent.Hostname
			if orgID == "" {
				orgID = agent.OrgID
			}
		}
	}

	det := &fleet.Detection{
		ID:            GenerateID(),
		OrgID:         orgID,
		AgentID:       agentID,
		AgentHostname: hostname,
		RuleID:        alertData.ID,
		RuleName:      alertData.Title,
		Title:         alertData.Title,
		Text:          alertData.Text,
		Description:   alertData.Description,
		Severity:      alertData.Severity,
		Labels:        alertData.Labels,
		Events:        alertData.Events,
		Timestamp:     time.Now().UTC(),
	}

	if err := h.detections.Create(r.Context(), det); err != nil {
		log.Errorf("fleet: detection ingest error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to store detection")
		return
	}

	log.WithFields(log.Fields{
		"agent":    agentID,
		"org":      orgID,
		"rule":     alertData.Title,
		"severity": alertData.Severity,
	}).Info("fleet: detection ingested")

	writeJSON(w, http.StatusCreated, fleet.Response{Data: map[string]string{"id": det.ID}})
}

// List handles GET /api/v1/orgs/{org_id}/detections
func (h *DetectionHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}

	opts := fleet.DetectionListOptions{
		ListOptions: fleet.ListOptions{
			Page:    intParam(r, "page", 1),
			PerPage: intParam(r, "per_page", 50),
		},
		AgentID:  r.URL.Query().Get("agent_id"),
		Severity: r.URL.Query().Get("severity"),
		RuleID:   r.URL.Query().Get("rule_id"),
		From:     r.URL.Query().Get("from"),
		To:       r.URL.Query().Get("to"),
	}

	detections, total, err := h.detections.List(r.Context(), orgID, opts)
	if err != nil {
		log.Errorf("fleet: list detections error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{
		Data: detections,
		Meta: &fleet.Pagination{Total: total, Page: opts.Page, PerPage: opts.PerPage},
	})
}

// Get handles GET /api/v1/orgs/{org_id}/detections/{id}
func (h *DetectionHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}

	parts := strings.Split(r.URL.Path, "/detections/")
	if len(parts) < 2 || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "detection ID required")
		return
	}
	detID := strings.TrimSuffix(parts[1], "/")

	det, err := h.detections.Get(r.Context(), orgID, detID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if det == nil {
		writeError(w, http.StatusNotFound, "detection not found")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: det})
}

// Timeline handles GET /api/v1/orgs/{org_id}/detections/timeline
func (h *DetectionHandler) Timeline(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}

	from := parseTime(r.URL.Query().Get("from"), time.Now().UTC().Add(-24*time.Hour))
	to := parseTime(r.URL.Query().Get("to"), time.Now().UTC())
	interval := r.URL.Query().Get("interval")
	if interval == "" {
		interval = "hour"
	}

	buckets, err := h.detections.Timeline(r.Context(), orgID, from, to, interval)
	if err != nil {
		log.Errorf("fleet: timeline error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: buckets})
}

// MitreHeatmap handles GET /api/v1/orgs/{org_id}/detections/mitre
func (h *DetectionHandler) MitreHeatmap(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}

	from := parseTime(r.URL.Query().Get("from"), time.Now().UTC().Add(-30*24*time.Hour))
	to := parseTime(r.URL.Query().Get("to"), time.Now().UTC())

	cells, err := h.detections.MitreHeatmap(r.Context(), orgID, from, to)
	if err != nil {
		log.Errorf("fleet: mitre heatmap error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: cells})
}

// ProcessTree handles GET /api/v1/orgs/{org_id}/detections/{id}/process-tree
// Returns telemetry events scoped to the detection's process chain — only
// the triggering process, its ancestors, and its descendants.
func (h *DetectionHandler) ProcessTree(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}

	parts := strings.Split(r.URL.Path, "/detections/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "detection ID required")
		return
	}
	detID := strings.TrimSuffix(parts[1], "/process-tree")
	detID = strings.TrimSuffix(detID, "/process-tree/")
	if detID == "" || strings.Contains(detID, "/") {
		writeError(w, http.StatusBadRequest, "detection ID required")
		return
	}

	det, err := h.detections.Get(r.Context(), orgID, detID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if det == nil {
		writeError(w, http.StatusNotFound, "detection not found")
		return
	}

	// Extract the triggering PID(s) and the known ancestry from detection events.
	triggerPIDs, ancestryPIDs := extractDetectionPIDs(det.Events)
	log.Infof("fleet: process tree for detection %s: triggerPIDs=%v ancestryPIDs=%v events_raw_len=%d", det.ID, triggerPIDs, ancestryPIDs, len(det.Events))

	// Query telemetry directly for the known PIDs from the detection.
	// This avoids the 5000-event limit issue where relevant PIDs get lost.
	from := det.Timestamp.Add(-1 * time.Hour)
	to := det.Timestamp.Add(1 * time.Hour)

	// Collect all known PIDs from the detection
	pids := make([]int, 0, len(ancestryPIDs))
	for pid := range ancestryPIDs {
		pids = append(pids, pid)
	}

	// Query events for each known PID
	var filtered []store.TelemetryEvent
	for _, pid := range pids {
		events, _, err := h.telemetry.Search(r.Context(), orgID, store.TelemetrySearchOpts{
			AgentID: det.AgentID,
			PID:     pid,
			From:    from,
			To:      to,
			Limit:   500,
		})
		if err != nil {
			continue
		}
		filtered = append(filtered, events...)
	}

	// Also fetch children of trigger PIDs by querying for parent_pid
	// (events where parent_pid matches our trigger PIDs)
	for pid := range triggerPIDs {
		events, _, err := h.telemetry.Search(r.Context(), orgID, store.TelemetrySearchOpts{
			AgentID:   det.AgentID,
			ParentPID: pid,
			From:      from,
			To:        to,
			Limit:     200,
		})
		if err != nil {
			continue
		}
		filtered = append(filtered, events...)
	}

	log.Infof("fleet: process tree: %d PIDs queried, %d total events returned", len(pids), len(filtered))

	writeJSON(w, http.StatusOK, fleet.Response{
		Data: map[string]interface{}{
			"detection":  det,
			"events":     filtered,
			"focus_pids": triggerPIDs,
		},
	})
}

// extractDetectionPIDs parses the detection events JSON and returns:
//   - triggerPIDs: the PIDs that directly triggered the detection (proc.pid)
//   - allPIDs: triggerPIDs + parent PIDs + ancestor PIDs (the full lineage)
func extractDetectionPIDs(eventsRaw json.RawMessage) (triggerPIDs, allPIDs map[int]bool) {
	triggerPIDs = make(map[int]bool)
	allPIDs = make(map[int]bool)

	var events []struct {
		Proc struct {
			PID       int      `json:"pid"`
			PPID      int      `json:"ppid"`
			Ancestors []string `json:"ancestors"`
		} `json:"proc"`
	}
	if err := json.Unmarshal(eventsRaw, &events); err != nil {
		return
	}

	for _, evt := range events {
		if evt.Proc.PID > 0 {
			triggerPIDs[evt.Proc.PID] = true
			allPIDs[evt.Proc.PID] = true
		}
		if evt.Proc.PPID > 0 {
			allPIDs[evt.Proc.PPID] = true
		}
		for _, a := range evt.Proc.Ancestors {
			if idx := strings.LastIndex(a, "("); idx >= 0 {
				pidStr := strings.TrimSuffix(strings.TrimSpace(a[idx+1:]), ")")
				if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
					allPIDs[pid] = true
				}
			}
		}
	}

	return
}

// ProcessContext handles GET /api/v1/orgs/{org_id}/detections/{id}/process-context?pid=X
// Returns events for a specific PID, its parent, and its children within the
// detection's time window. Used for live-loading tree expansion.
func (h *DetectionHandler) ProcessContext(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}

	// Extract detection ID: .../detections/{id}/process-context
	parts := strings.Split(r.URL.Path, "/detections/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "detection ID required")
		return
	}
	detID := strings.TrimSuffix(parts[1], "/process-context")
	detID = strings.TrimSuffix(detID, "/process-context/")

	targetPID := intParam(r, "pid", 0)
	ancestorsOnly := r.URL.Query().Get("ancestors") == "true"
	if targetPID <= 0 {
		writeError(w, http.StatusBadRequest, "pid parameter required")
		return
	}

	det, err := h.detections.Get(r.Context(), orgID, detID)
	if err != nil || det == nil {
		writeError(w, http.StatusNotFound, "detection not found")
		return
	}

	from := det.Timestamp.Add(-1 * time.Hour)
	to := det.Timestamp.Add(1 * time.Hour)

	// Find parent PID from target's events
	targetEvents, _, _ := h.telemetry.Search(r.Context(), orgID, store.TelemetrySearchOpts{
		AgentID: det.AgentID, PID: targetPID, From: from, To: to, Limit: 100,
	})
	parentPID := 0
	for _, evt := range targetEvents {
		if evt.ParentPID > 0 && parentPID == 0 {
			parentPID = evt.ParentPID
		}
	}

	var filtered []store.TelemetryEvent

	if ancestorsOnly {
		// Only return parent + grandparent events (NOT the target itself — caller already has it)
		if parentPID > 0 {
			parentEvents, _, _ := h.telemetry.Search(r.Context(), orgID, store.TelemetrySearchOpts{
				AgentID: det.AgentID, PID: parentPID, From: from, To: to, Limit: 500,
			})
			filtered = append(filtered, parentEvents...)
			// Load grandparent too
			for _, evt := range parentEvents {
				if evt.ParentPID > 0 && evt.ParentPID != parentPID {
					gpEvents, _, _ := h.telemetry.Search(r.Context(), orgID, store.TelemetrySearchOpts{
						AgentID: det.AgentID, PID: evt.ParentPID, From: from, To: to, Limit: 200,
					})
					filtered = append(filtered, gpEvents...)
					break
				}
			}
		}
	} else {
		filtered = append(filtered, targetEvents...)
		if parentPID > 0 {
			parentEvents, _, _ := h.telemetry.Search(r.Context(), orgID, store.TelemetrySearchOpts{
				AgentID: det.AgentID, PID: parentPID, From: from, To: to, Limit: 500,
			})
			filtered = append(filtered, parentEvents...)
		}
	}

	// Query children (skip if ancestors-only mode)
	children := make(map[int]bool)
	if !ancestorsOnly {
		childEvents, _, _ := h.telemetry.Search(r.Context(), orgID, store.TelemetrySearchOpts{
			AgentID: det.AgentID, ParentPID: targetPID, From: from, To: to, Limit: 500,
		})
		for _, evt := range childEvents {
			if evt.PID != targetPID {
				children[evt.PID] = true
			}
		}
		filtered = append(filtered, childEvents...)
	}

	writeJSON(w, http.StatusOK, fleet.Response{
		Data: map[string]interface{}{
			"events":     filtered,
			"target_pid": targetPID,
			"parent_pid": parentPID,
			"child_pids": children,
		},
	})
}

func parseTime(s string, defaultVal time.Time) time.Time {
	if s == "" {
		return defaultVal
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return defaultVal
	}
	return t
}
