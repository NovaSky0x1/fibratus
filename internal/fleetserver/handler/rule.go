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
	"strconv"
	"strings"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/qlparser"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/internal/fleetserver/validator"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// validateAndSetStatus runs condition validation on a rule using the
// real Fibratus QL parser with macro support and sets validation status.
func (h *RuleHandler) validateAndSetStatus(r *http.Request, rule *fleet.Rule) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	macros := h.loadMacros(r, orgID)
	result := validator.ValidateConditionWithMacros(rule.Condition, macros)
	if result.Valid {
		rule.ValidationStatus = "valid"
		rule.ValidationErrors = json.RawMessage(`[]`)
	} else {
		rule.ValidationStatus = "invalid"
		errJSON, _ := json.Marshal(result.Errors)
		rule.ValidationErrors = errJSON
	}
}

// loadMacros loads org macros and converts them for the QL parser.
func (h *RuleHandler) loadMacros(r *http.Request, orgID string) map[string]*qlparser.Macro {
	dbMacros, err := h.macros.List(r.Context(), orgID)
	if err != nil || len(dbMacros) == 0 {
		return nil
	}
	macros := make(map[string]*qlparser.Macro, len(dbMacros))
	for _, m := range dbMacros {
		macros[m.Name] = &qlparser.Macro{
			ID:   m.Name,
			Expr: m.Expr,
			List: m.List,
		}
	}
	return macros
}

// RuleHandler handles rule management API requests.
// RuleChangeCallback is called when rules are created, updated, or deleted.
// The orgID identifies which organization's agents need a rule push.
type RuleChangeCallback func(orgID string)

type RuleHandler struct {
	rules          store.RuleStore
	agents         store.AgentStore
	macros         store.MacroStore
	detections     store.DetectionStore
	audit          store.AuditStore
	users          store.UserStore
	orgs           store.OrgStore
	onRuleChange   RuleChangeCallback
}

// SetOrgStore sets the org store for cross-org aggregation.
func (h *RuleHandler) SetOrgStore(orgs store.OrgStore) { h.orgs = orgs }

// SetDetectionStore sets the detection store for noisy-rule queries.
func (h *RuleHandler) SetDetectionStore(dets store.DetectionStore) { h.detections = dets }

// NewRuleHandler creates a new rule handler.
func NewRuleHandler(rules store.RuleStore, agents store.AgentStore, macros store.MacroStore, audit store.AuditStore, users store.UserStore) *RuleHandler {
	return &RuleHandler{rules: rules, agents: agents, macros: macros, audit: audit, users: users}
}

// SetRuleChangeCallback registers a callback for rule change notifications.
func (h *RuleHandler) SetRuleChangeCallback(cb RuleChangeCallback) {
	h.onRuleChange = cb
}

// notifyRuleChange triggers the rule push callback if set.
func (h *RuleHandler) notifyRuleChange(orgID string) {
	if h.onRuleChange != nil {
		go h.onRuleChange(orgID)
	}
}

// List handles GET /api/v1/orgs/{org_id}/rules
func (h *RuleHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())

	opts := fleet.ListOptions{
		Page:    intParam(r, "page", 1),
		PerPage: intParam(r, "per_page", 100),
	}

	// Cross-org aggregation when no org is selected
	if orgID == "" {
		orgIDs := accountOrgIDs(r, h.orgs)
		if len(orgIDs) == 0 {
			writeError(w, http.StatusBadRequest, "org or account context required")
			return
		}
		allRules := make([]*fleet.Rule, 0)
		total := 0
		for _, oid := range orgIDs {
			rules, t, err := h.rules.List(r.Context(), oid, opts)
			if err != nil {
				continue
			}
			allRules = append(allRules, rules...)
			total += t
		}
		writeJSON(w, http.StatusOK, fleet.Response{
			Data: allRules,
			Meta: &fleet.Pagination{Total: total, Page: opts.Page, PerPage: opts.PerPage},
		})
		return
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
		// Validate YAML against schema before parsing
		if err := validator.ValidateRuleYAML(body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := parseYAMLRule(body, &rule); err != nil {
			writeError(w, http.StatusBadRequest, "invalid rule YAML: "+err.Error())
			return
		}
		rule.RawYAML = string(body)
	} else {
		if err := json.Unmarshal(body, &rule); err != nil {
			writeError(w, http.StatusBadRequest, "invalid rule JSON: "+err.Error())
			return
		}
	}

	// Validate rule fields
	if err := validator.ValidateRuleFields(rule.Name, rule.Condition, rule.Severity); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if rule.ID == "" {
		rule.ID = GenerateID()
	}
	rule.OrgID = orgID
	rule.AccountID = ctxutil.AccountIDFromContext(r.Context())
	if rule.Version == "" {
		rule.Version = "1.0.0"
	}
	if rule.Severity == "" {
		rule.Severity = "medium"
	}

	// Run condition validation
	h.validateAndSetStatus(r, &rule)
	if rule.ValidationStatus == "invalid" {
		rule.Enabled = false // invalid rules cannot be enabled
	} else {
		rule.Enabled = true
	}

	if err := h.rules.Create(r.Context(), &rule); err != nil {
		log.Errorf("fleet: create rule error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create rule")
		return
	}

	userID := ctxutil.UserIDFromContext(r.Context())
	logAudit(r, h.audit, h.users, userID, orgID, "create", "rule", rule.ID, rule.Name, nil)
	log.Infof("fleet: rule created: %s (%s) in org %s", rule.Name, rule.ID, orgID)
	h.notifyRuleChange(orgID)
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
		if err := validator.ValidateRuleYAML(body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
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
		// Validate fields for JSON updates (skip for enable/disable toggles)
		if rule.Condition != "" {
			if err := validator.ValidateRuleFields(rule.Name, rule.Condition, rule.Severity); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
	}

	rule.ID = ruleID
	rule.OrgID = orgID

	// Fetch existing rule for merge and modification tracking.
	existing, _ := h.rules.Get(r.Context(), orgID, ruleID)

	// For JSON partial updates (e.g., just {enabled: false}), merge with existing rule
	// to prevent overwriting all fields with empty values.
	if !strings.Contains(contentType, "yaml") {
		if existing == nil {
			writeError(w, http.StatusNotFound, "rule not found")
			return
		}
		// Merge: only overwrite fields that were explicitly set in the request
		if rule.Name == "" { rule.Name = existing.Name }
		if rule.Version == "" { rule.Version = existing.Version }
		if rule.Description == "" { rule.Description = existing.Description }
		if rule.Condition == "" { rule.Condition = existing.Condition }
		if rule.Output == "" { rule.Output = existing.Output }
		if rule.Severity == "" { rule.Severity = existing.Severity }
		if rule.RawYAML == "" { rule.RawYAML = existing.RawYAML }
		if rule.Labels == nil { rule.Labels = existing.Labels }
		if rule.Tags == nil { rule.Tags = existing.Tags }
		if rule.References == nil { rule.References = existing.References }
	}

	// Track user modifications to synced rules
	if existing != nil && existing.Source != "manual" {
		if rule.Condition != existing.Condition || rule.Name != existing.Name || rule.Description != existing.Description {
			rule.UserModified = true
		}
		if !rule.Enabled && existing.Enabled {
			rule.UserDisabled = true
		}
		if rule.Enabled && !existing.Enabled {
			rule.UserDisabled = false
		}
	}

	// Re-validate condition
	h.validateAndSetStatus(r, &rule)
	if rule.ValidationStatus == "invalid" {
		rule.Enabled = false // invalid rules cannot be enabled
	}

	// Propagate update to ALL orgs in the account (rules are managed per-account)
	accountID := ctxutil.AccountIDFromContext(r.Context())
	if accountID != "" {
		if _, err := h.rules.UpdateAcrossAccount(r.Context(), accountID, &rule); err != nil {
			log.Errorf("fleet: update rule across account error: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to update rule")
			return
		}
		// Notify all orgs in account of rule change
		if h.orgs != nil {
			orgList, _ := h.orgs.ListByAccount(r.Context(), accountID)
			for _, o := range orgList {
				h.notifyRuleChange(o.ID)
			}
		}
	} else {
		if err := h.rules.Update(r.Context(), &rule); err != nil {
			log.Errorf("fleet: update rule error: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to update rule")
			return
		}
		h.notifyRuleChange(orgID)
	}

	userID := ctxutil.UserIDFromContext(r.Context())
	logAudit(r, h.audit, h.users, userID, orgID, "update", "rule", ruleID, rule.Name, nil)
	writeJSON(w, http.StatusOK, fleet.Response{Data: rule})
}

// Delete handles DELETE /api/v1/orgs/{org_id}/rules/{id}
func (h *RuleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	userID := ctxutil.UserIDFromContext(r.Context())
	parts := strings.Split(r.URL.Path, "/rules/")
	if len(parts) < 2 || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "rule ID required")
		return
	}
	ruleID := strings.TrimSuffix(parts[1], "/")

	// Get name for audit before deleting
	existing, _ := h.rules.Get(r.Context(), orgID, ruleID)
	name := ""
	if existing != nil {
		name = existing.Name
		// Record deletion of synced rules so they aren't re-created on next sync
		if existing.Source != "manual" {
			// Record deletion for ALL orgs in the account
			accountID := ctxutil.AccountIDFromContext(r.Context())
			if accountID != "" && h.orgs != nil {
				orgList, _ := h.orgs.ListByAccount(r.Context(), accountID)
				for _, o := range orgList {
					h.rules.RecordSyncDeletion(r.Context(), o.ID, ruleID, existing.Source)
				}
			} else {
				h.rules.RecordSyncDeletion(r.Context(), orgID, ruleID, existing.Source)
			}
		}
	}

	// Delete from ALL orgs in the account (rules are managed per-account)
	accountID := ctxutil.AccountIDFromContext(r.Context())
	if accountID != "" {
		if err := h.rules.DeleteAcrossAccount(r.Context(), accountID, ruleID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete rule")
			return
		}
		if h.orgs != nil {
			orgList, _ := h.orgs.ListByAccount(r.Context(), accountID)
			for _, o := range orgList {
				h.notifyRuleChange(o.ID)
			}
		}
	} else {
		if err := h.rules.Delete(r.Context(), orgID, ruleID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete rule")
			return
		}
		h.notifyRuleChange(orgID)
	}
	logAudit(r, h.audit, h.users, userID, orgID, "delete", "rule", ruleID, name, nil)
	w.WriteHeader(http.StatusNoContent)
}

// Validate handles POST /api/v1/orgs/{org_id}/rules/{id}/validate
// Re-validates a rule's condition and updates its validation status.
func (h *RuleHandler) Validate(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	// Extract rule ID from path: .../rules/{id}/validate
	path := r.URL.Path
	parts := strings.Split(path, "/rules/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "rule ID required")
		return
	}
	ruleID := strings.TrimSuffix(strings.TrimSuffix(parts[1], "/validate"), "/")

	existing, err := h.rules.Get(r.Context(), orgID, ruleID)
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}

	// Run validation with macros
	h.validateAndSetStatus(r, existing)

	if err := h.rules.Update(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update rule")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{
		"rule_id":           ruleID,
		"validation_status": existing.ValidationStatus,
		"validation_errors": existing.ValidationErrors,
	}})
}

// ValidateCondition handles POST /api/v1/orgs/{org_id}/rules/validate-condition
// Validates a condition string without creating a rule. Used for live editor feedback.
func (h *RuleHandler) ValidateCondition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Condition string `json:"condition"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	orgID := ctxutil.OrgIDFromContext(r.Context())
	macros := h.loadMacros(r, orgID)
	result := validator.ValidateConditionWithMacros(req.Condition, macros)
	writeJSON(w, http.StatusOK, fleet.Response{Data: result})
}

// ValidateAll handles POST /api/v1/orgs/{org_id}/rules/validate-all
// Re-validates all rules for the org. Useful after migration or bulk import.
func (h *RuleHandler) ValidateAll(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	rules, _, err := h.rules.List(r.Context(), orgID, fleet.ListOptions{Page: 1, PerPage: 10000})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list rules")
		return
	}

	validated := 0
	invalid := 0
	for _, rule := range rules {
		h.validateAndSetStatus(r, rule)
		if rule.ValidationStatus == "invalid" {
			rule.Enabled = false
			invalid++
		}
		if err := h.rules.Update(r.Context(), rule); err != nil {
			log.Warnf("fleet: failed to update rule %s validation: %v", rule.ID, err)
		}
		validated++
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{
		"validated": validated,
		"invalid":   invalid,
		"valid":     validated - invalid,
	}})
}

// NoisyRules handles GET /api/v1/orgs/{org_id}/rules/noisy
// Returns the rules with the highest detection counts.
func (h *RuleHandler) NoisyRules(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	results, err := h.detections.TopNoisyRules(r.Context(), orgID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query noisy rules")
		return
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: results})
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

	// Prepend macros from DB so rules can reference them
	if h.macros != nil {
		macrosYAML, err := h.macros.GetAllForOrg(r.Context(), orgID)
		if err != nil {
			log.Warnf("fleet: failed to load macros for agent sync: %v", err)
		} else if macrosYAML != "" {
			w.Write([]byte(macrosYAML))
			w.Write([]byte("\n---\n"))
		}
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

// ═══════════════════════════════════════════════════════════════
// Public Rule Validation API (no authentication required)
// ═══════════════════════════════════════════════════════════════

// ValidateRuleAPI handles POST /api/v1/validate-rule
// Validates rule YAML + optional macros YAML. Requires API key or JWT.
//
// Accepts two formats:
//   1. JSON: { "rules": "<yaml>", "macros": "<yaml>" }
//   2. Raw YAML body (Content-Type: application/yaml) — rules only, no macros
func ValidateRuleAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1024*1024)) // 1MB max
	if err != nil || len(body) == 0 {
		writeError(w, http.StatusBadRequest, "request body required")
		return
	}

	var rulesYAML []byte
	var macros map[string]*qlparser.Macro

	// Detect format: JSON wrapper or raw YAML
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") || (len(body) > 0 && body[0] == '{') {
		var req struct {
			Rules  string `json:"rules"`
			Macros string `json:"macros"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: expected {\"rules\": \"<yaml>\", \"macros\": \"<yaml>\"}")
			return
		}
		rulesYAML = []byte(req.Rules)
		if req.Macros != "" {
			macros = parseMacrosYAML([]byte(req.Macros))
		}
	} else {
		// Raw YAML — check for X-Macros header pointing to inline macros
		rulesYAML = body
	}

	if len(rulesYAML) == 0 {
		writeError(w, http.StatusBadRequest, "rules field is required")
		return
	}

	type RuleResult struct {
		Name            string      `json:"name"`
		Valid           bool        `json:"valid"`
		SchemaErrors    []string    `json:"schema_errors,omitempty"`
		ConditionValid  bool        `json:"condition_valid"`
		ConditionErrors interface{} `json:"condition_errors,omitempty"`
		Severity        string      `json:"severity,omitempty"`
		Version         string      `json:"version,omitempty"`
	}

	results := make([]RuleResult, 0)
	docs := splitYAMLDocs(rulesYAML)
	for _, doc := range docs {
		if len(strings.TrimSpace(string(doc))) == 0 {
			continue
		}

		result := RuleResult{}

		if err := validator.ValidateRuleYAML(doc); err != nil {
			result.SchemaErrors = append(result.SchemaErrors, err.Error())
		}

		var rule fleet.Rule
		if err := parseYAMLRule(doc, &rule); err != nil {
			result.SchemaErrors = append(result.SchemaErrors, "YAML parse error: "+err.Error())
			results = append(results, result)
			continue
		}
		result.Name = rule.Name
		result.Severity = rule.Severity
		result.Version = rule.Version

		if err := validator.ValidateRuleFields(rule.Name, rule.Condition, rule.Severity); err != nil {
			result.SchemaErrors = append(result.SchemaErrors, err.Error())
		}

		if rule.Condition != "" {
			var condResult *validator.ConditionValidationResult
			if macros != nil {
				condResult = validator.ValidateConditionWithMacros(rule.Condition, macros)
			} else {
				condResult = validator.ValidateCondition(rule.Condition)
			}
			result.ConditionValid = condResult.Valid
			if !condResult.Valid {
				result.ConditionErrors = condResult.Errors
			}
		}

		result.Valid = len(result.SchemaErrors) == 0 && result.ConditionValid
		results = append(results, result)
	}

	allValid := true
	for _, r := range results {
		if !r.Valid {
			allValid = false
			break
		}
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{
		"valid":   allValid,
		"count":   len(results),
		"rules":   results,
	}})
}

// parseMacrosYAML parses a macros YAML file into a map for condition validation.
func parseMacrosYAML(data []byte) map[string]*qlparser.Macro {
	var rawMacros []struct {
		Macro string   `yaml:"macro"`
		Expr  string   `yaml:"expr"`
		List  []string `yaml:"list"`
	}
	if err := yaml.Unmarshal(data, &rawMacros); err != nil {
		return nil
	}
	macros := make(map[string]*qlparser.Macro, len(rawMacros))
	for _, m := range rawMacros {
		if m.Macro != "" {
			macros[m.Macro] = &qlparser.Macro{
				ID:   m.Macro,
				Expr: m.Expr,
				List: m.List,
			}
		}
	}
	return macros
}

// splitYAMLDocs splits a multi-document YAML byte slice by "---" separator.
func splitYAMLDocs(data []byte) [][]byte {
	parts := strings.Split(string(data), "\n---")
	docs := make([][]byte, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" && trimmed != "---" {
			docs = append(docs, []byte(trimmed))
		}
	}
	if len(docs) == 0 && len(data) > 0 {
		docs = append(docs, data)
	}
	return docs
}
