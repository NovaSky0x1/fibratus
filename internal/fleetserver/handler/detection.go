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

// DetectionHandler handles detection-related API requests.
type DetectionHandler struct {
	detections store.DetectionStore
	agents     store.AgentStore
}

// NewDetectionHandler creates a new detection handler.
func NewDetectionHandler(detections store.DetectionStore, agents store.AgentStore) *DetectionHandler {
	return &DetectionHandler{detections: detections, agents: agents}
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
