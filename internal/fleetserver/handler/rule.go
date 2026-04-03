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
	"os"
	"strings"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// RuleHandler handles rule management API requests.
type RuleHandler struct {
	rules      store.RuleStore
	agents     store.AgentStore
	macrosYAML string
}

// NewRuleHandler creates a new rule handler. It loads macros from the
// rules/macros directory so they can be prepended to agent rule syncs.
func NewRuleHandler(rules store.RuleStore, agents store.AgentStore) *RuleHandler {
	h := &RuleHandler{rules: rules, agents: agents}
	// Try to load macros from well-known locations
	for _, path := range []string{
		"rules/macros/macros.yml",
		"/opt/fibratus-fleet/src/rules/macros/macros.yml",
	} {
		data, err := os.ReadFile(path)
		if err == nil {
			h.macrosYAML = string(data)
			log.Infof("fleet: loaded macros from %s (%d bytes)", path, len(data))
			break
		}
	}
	return h
}

// List handles GET /api/v1/orgs/{org_id}/rules
func (h *RuleHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}

	opts := fleet.ListOptions{
		Page:    intParam(r, "page", 1),
		PerPage: intParam(r, "per_page", 100),
	}

	rules, total, err := h.rules.List(r.Context(), orgID, opts)
	if err != nil {
		log.Errorf("fleet: list rules error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{
		Data: rules,
		Meta: &fleet.Pagination{Total: total, Page: opts.Page, PerPage: opts.PerPage},
	})
}

// Create handles POST /api/v1/orgs/{org_id}/rules
// Accepts either a JSON rule object or raw YAML in the body.
func (h *RuleHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB max
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var rule fleet.Rule

	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "yaml") || strings.Contains(contentType, "x-yaml") {
		// Parse YAML rule
		if err := parseYAMLRule(body, &rule); err != nil {
			writeError(w, http.StatusBadRequest, "invalid rule YAML: "+err.Error())
			return
		}
		rule.RawYAML = string(body)
	} else {
		// Parse JSON
		if err := json.Unmarshal(body, &rule); err != nil {
			writeError(w, http.StatusBadRequest, "invalid rule JSON: "+err.Error())
			return
		}
	}

	if rule.Name == "" || rule.Condition == "" {
		writeError(w, http.StatusBadRequest, "rule name and condition are required")
		return
	}

	if rule.ID == "" {
		rule.ID = generateID()
	}
	rule.OrgID = orgID
	if rule.Version == "" {
		rule.Version = "1.0.0"
	}
	if rule.Severity == "" {
		rule.Severity = "medium"
	}
	rule.Enabled = true

	if err := h.rules.Create(r.Context(), &rule); err != nil {
		log.Errorf("fleet: create rule error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create rule")
		return
	}

	log.Infof("fleet: rule created: %s (%s) in org %s", rule.Name, rule.ID, orgID)
	writeJSON(w, http.StatusCreated, fleet.Response{Data: rule})
}

// Get handles GET /api/v1/orgs/{org_id}/rules/{id}
func (h *RuleHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	parts := strings.Split(r.URL.Path, "/rules/")
	if len(parts) < 2 || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "rule ID required")
		return
	}
	ruleID := strings.TrimSuffix(parts[1], "/")

	rule, err := h.rules.Get(r.Context(), orgID, ruleID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if rule == nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: rule})
}

// Update handles PUT /api/v1/orgs/{org_id}/rules/{id}
// Accepts JSON (for field updates like enable/disable) or YAML (for full rule edits).
func (h *RuleHandler) Update(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	parts := strings.Split(r.URL.Path, "/rules/")
	if len(parts) < 2 || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "rule ID required")
		return
	}
	ruleID := strings.TrimSuffix(parts[1], "/")

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var rule fleet.Rule
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "yaml") || strings.Contains(contentType, "x-yaml") {
		if err := parseYAMLRule(body, &rule); err != nil {
			writeError(w, http.StatusBadRequest, "invalid rule YAML: "+err.Error())
			return
		}
		rule.RawYAML = string(body)
	} else {
		if err := json.Unmarshal(body, &rule); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	rule.ID = ruleID
	rule.OrgID = orgID

	if err := h.rules.Update(r.Context(), &rule); err != nil {
		log.Errorf("fleet: update rule error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update rule")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: rule})
}

// Delete handles DELETE /api/v1/orgs/{org_id}/rules/{id}
func (h *RuleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	parts := strings.Split(r.URL.Path, "/rules/")
	if len(parts) < 2 || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "rule ID required")
		return
	}
	ruleID := strings.TrimSuffix(parts[1], "/")

	if err := h.rules.Delete(r.Context(), orgID, ruleID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete rule")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetForAgent handles GET /api/v1/agent/rules
// Called by enrolled agents to pull their rule set. Supports ETag
// caching so agents don't re-download unchanged rules.
func (h *RuleHandler) GetForAgent(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-ID")
	if orgID == "" {
		orgID = ctxutil.OrgIDFromContext(r.Context())
	}
	agentID := r.Header.Get("X-Agent-ID")
	if agentID == "" {
		agentID = ctxutil.AgentIDFromContext(r.Context())
	}

	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org identity required")
		return
	}

	rules, etag, err := h.rules.GetForAgent(r.Context(), orgID, agentID)
	if err != nil {
		log.Errorf("fleet: get rules for agent error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// ETag caching
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Return rules as YAML array (what the agent expects)
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)

	// Prepend macros so rules can reference them
	if h.macrosYAML != "" {
		w.Write([]byte(h.macrosYAML))
		w.Write([]byte("\n---\n"))
	}

	for _, rule := range rules {
		if rule.RawYAML != "" {
			w.Write([]byte(rule.RawYAML))
			w.Write([]byte("\n---\n"))
		}
	}
}

// parseYAMLRule extracts rule fields from a YAML document.
func parseYAMLRule(data []byte, rule *fleet.Rule) error {
	var raw struct {
		Name        string            `yaml:"name"`
		ID          string            `yaml:"id"`
		Version     string            `yaml:"version"`
		Description string            `yaml:"description"`
		Condition   string            `yaml:"condition"`
		Output      string            `yaml:"output"`
		Severity    string            `yaml:"severity"`
		Labels      map[string]string `yaml:"labels"`
		Tags        []string          `yaml:"tags"`
		References  []string          `yaml:"references"`
		Enabled     *bool             `yaml:"enabled"`
	}

	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}

	rule.Name = raw.Name
	rule.ID = raw.ID
	rule.Version = raw.Version
	rule.Description = raw.Description
	rule.Condition = raw.Condition
	rule.Output = raw.Output
	rule.Severity = raw.Severity
	rule.Labels = raw.Labels
	rule.Tags = raw.Tags
	rule.References = raw.References
	if raw.Enabled != nil {
		rule.Enabled = *raw.Enabled
	} else {
		rule.Enabled = true
	}
	return nil
}
