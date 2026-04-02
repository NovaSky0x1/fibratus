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
	"io"
	"net/http"
	"strings"
	"time"

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

// Ingest handles POST /api/v1/detections
func (h *DetectionHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	agentID := r.Header.Get("X-Agent-ID")

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10MB max
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	// Parse the alert JSON from the agent
	var alertData struct {
		ID          string            `json:"id"`
		Title       string            `json:"title"`
		Severity    string            `json:"severity"`
		Text        string            `json:"text"`
		Description string            `json:"description"`
		Labels      map[string]string `json:"labels"`
		Events      json.RawMessage   `json:"events"`
	}

	if err := json.Unmarshal(body, &alertData); err != nil {
		writeError(w, http.StatusBadRequest, "invalid alert JSON")
		return
	}

	// Resolve agent hostname
	hostname := ""
	if agentID != "" {
		agent, _ := h.agents.Get(r.Context(), agentID)
		if agent != nil {
			hostname = agent.Hostname
		}
	}

	det := &fleet.Detection{
		ID:            generateID(),
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
		"rule":     alertData.Title,
		"severity": alertData.Severity,
	}).Info("fleet: detection ingested")

	writeJSON(w, http.StatusCreated, fleet.Response{Data: map[string]string{"id": det.ID}})
}

// List handles GET /api/v1/detections
func (h *DetectionHandler) List(w http.ResponseWriter, r *http.Request) {
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

	detections, total, err := h.detections.List(r.Context(), opts)
	if err != nil {
		log.Errorf("fleet: list detections error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{
		Data: detections,
		Meta: &fleet.Pagination{
			Total:   total,
			Page:    opts.Page,
			PerPage: opts.PerPage,
		},
	})
}

// Get handles GET /api/v1/detections/{id}
func (h *DetectionHandler) Get(w http.ResponseWriter, r *http.Request) {
	detID := strings.TrimPrefix(r.URL.Path, "/api/v1/detections/")
	if detID == "" {
		writeError(w, http.StatusBadRequest, "detection ID required")
		return
	}

	det, err := h.detections.Get(r.Context(), detID)
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

// Timeline handles GET /api/v1/detections/timeline
func (h *DetectionHandler) Timeline(w http.ResponseWriter, r *http.Request) {
	from := parseTime(r.URL.Query().Get("from"), time.Now().UTC().Add(-24*time.Hour))
	to := parseTime(r.URL.Query().Get("to"), time.Now().UTC())
	interval := r.URL.Query().Get("interval")
	if interval == "" {
		interval = "hour"
	}

	buckets, err := h.detections.Timeline(r.Context(), from, to, interval)
	if err != nil {
		log.Errorf("fleet: timeline error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: buckets})
}

// MitreHeatmap handles GET /api/v1/detections/mitre
func (h *DetectionHandler) MitreHeatmap(w http.ResponseWriter, r *http.Request) {
	from := parseTime(r.URL.Query().Get("from"), time.Now().UTC().Add(-30*24*time.Hour))
	to := parseTime(r.URL.Query().Get("to"), time.Now().UTC())

	cells, err := h.detections.MitreHeatmap(r.Context(), from, to)
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
