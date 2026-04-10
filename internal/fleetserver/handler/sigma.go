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
	"fmt"
	"io"
	"net/http"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/qlparser"
	"github.com/rabbitstack/fibratus/internal/fleetserver/sigma"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/internal/fleetserver/validator"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// SigmaHandler handles SIGMA rule conversion and import endpoints.
type SigmaHandler struct {
	rules  store.RuleStore
	macros store.MacroStore
	audit  store.AuditStore
	users  store.UserStore
}

// NewSigmaHandler creates a new SIGMA handler.
func NewSigmaHandler(rules store.RuleStore, macros store.MacroStore, audit store.AuditStore, users store.UserStore) *SigmaHandler {
	return &SigmaHandler{
		rules:  rules,
		macros: macros,
		audit:  audit,
		users:  users,
	}
}

// Convert handles POST /api/v1/orgs/{org_id}/sigma/convert
// Accepts a single SIGMA rule (YAML) and returns the converted Fibratus rule.
func (h *SigmaHandler) Convert(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 512*1024)) // 512KB max
	if err != nil || len(body) == 0 {
		writeError(w, http.StatusBadRequest, "request body required")
		return
	}

	// Accept either raw YAML or JSON { "sigma_yaml": "..." }
	sigmaYAML := body
	if r.Header.Get("Content-Type") == "application/json" {
		var req struct {
			SigmaYAML string `json:"sigma_yaml"`
		}
		if err := json.Unmarshal(body, &req); err == nil && req.SigmaYAML != "" {
			sigmaYAML = []byte(req.SigmaYAML)
		}
	}

	result := sigma.Convert(sigmaYAML)

	// If conversion succeeded, validate the generated condition against the QL parser
	if result.Success && result.Condition != "" {
		orgID := ctxutil.OrgIDFromContext(r.Context())
		macros := h.loadMacros(r, orgID)
		condResult := validator.ValidateConditionWithMacros(result.Condition, macros)
		if !condResult.Valid {
			for _, e := range condResult.Errors {
				result.Warnings = append(result.Warnings, "Fibratus QL validation: "+e.Message)
			}
		}
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: result})
}

// ConvertBatch handles POST /api/v1/orgs/{org_id}/sigma/convert/batch
// Accepts multiple SIGMA rules and returns batch conversion results.
func (h *SigmaHandler) ConvertBatch(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 5*1024*1024)) // 5MB max
	if err != nil || len(body) == 0 {
		writeError(w, http.StatusBadRequest, "request body required")
		return
	}

	var req struct {
		Rules []string `json:"rules"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: expected {\"rules\": [\"yaml1\", \"yaml2\", ...]}")
		return
	}

	rawRules := make([][]byte, len(req.Rules))
	for i, r := range req.Rules {
		rawRules[i] = []byte(r)
	}

	batchResult := sigma.ConvertBatch(rawRules)

	// Validate each successful conversion against the QL parser
	orgID := ctxutil.OrgIDFromContext(r.Context())
	macros := h.loadMacros(r, orgID)
	for i := range batchResult.Results {
		res := &batchResult.Results[i]
		if res.Success && res.Condition != "" {
			condResult := validator.ValidateConditionWithMacros(res.Condition, macros)
			if !condResult.Valid {
				for _, e := range condResult.Errors {
					res.Warnings = append(res.Warnings, "Fibratus QL validation: "+e.Message)
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: batchResult})
}

// ConvertAndImport handles POST /api/v1/orgs/{org_id}/sigma/import
// Converts a SIGMA rule and imports it as a Fibratus rule if valid.
func (h *SigmaHandler) ConvertAndImport(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	userID := ctxutil.UserIDFromContext(r.Context())

	body, err := io.ReadAll(io.LimitReader(r.Body, 512*1024))
	if err != nil || len(body) == 0 {
		writeError(w, http.StatusBadRequest, "request body required")
		return
	}

	// Accept raw YAML or JSON
	sigmaYAML := body
	if r.Header.Get("Content-Type") == "application/json" {
		var req struct {
			SigmaYAML string `json:"sigma_yaml"`
		}
		if err := json.Unmarshal(body, &req); err == nil && req.SigmaYAML != "" {
			sigmaYAML = []byte(req.SigmaYAML)
		}
	}

	result := sigma.Convert(sigmaYAML)
	if !result.Success {
		writeJSON(w, http.StatusOK, fleet.Response{Data: result})
		return
	}

	// Validate the converted condition
	macros := h.loadMacros(r, orgID)
	condResult := validator.ValidateConditionWithMacros(result.Condition, macros)

	// Parse the generated YAML into a rule
	var rule fleet.Rule
	if err := parseYAMLRule([]byte(result.FibratusYAML), &rule); err != nil {
		result.Errors = append(result.Errors, "failed to parse generated YAML: "+err.Error())
		result.Success = false
		writeJSON(w, http.StatusOK, fleet.Response{Data: result})
		return
	}

	rule.OrgID = orgID
	rule.RawYAML = result.FibratusYAML
	rule.Source = "sigma"

	if condResult.Valid {
		rule.ValidationStatus = "valid"
		rule.ValidationErrors = json.RawMessage(`[]`)
		rule.Enabled = true
	} else {
		rule.ValidationStatus = "invalid"
		errJSON, _ := json.Marshal(condResult.Errors)
		rule.ValidationErrors = errJSON
		rule.Enabled = false
		for _, e := range condResult.Errors {
			result.Warnings = append(result.Warnings, "validation: "+e.Message)
		}
	}

	// Check if rule with same ID already exists
	existing, _ := h.rules.Get(r.Context(), orgID, rule.ID)
	if existing != nil {
		if err := h.rules.Update(r.Context(), &rule); err != nil {
			result.Errors = append(result.Errors, "failed to update existing rule: "+err.Error())
			result.Success = false
			writeJSON(w, http.StatusOK, fleet.Response{Data: result})
			return
		}
	} else {
		if err := h.rules.Create(r.Context(), &rule); err != nil {
			result.Errors = append(result.Errors, "failed to create rule: "+err.Error())
			result.Success = false
			writeJSON(w, http.StatusOK, fleet.Response{Data: result})
			return
		}
	}

	logAudit(r, h.audit, h.users, userID, orgID, "create", "rule", rule.ID, rule.Name+" (sigma import)", nil)
	log.Infof("fleet: imported SIGMA rule %q as Fibratus rule %s (org=%s)", result.SigmaTitle, rule.ID, orgID)

	writeJSON(w, http.StatusOK, fleet.Response{Data: result})
}

// SupportedLogsources handles GET /api/v1/sigma/logsources
// Returns the list of SIGMA logsource categories that can be converted.
func (h *SigmaHandler) SupportedLogsources(w http.ResponseWriter, r *http.Request) {
	mappings := sigma.GetSupportedLogsources()
	writeJSON(w, http.StatusOK, fleet.Response{Data: mappings})
}

// FieldMappings handles GET /api/v1/sigma/field-mappings
// Returns the complete SIGMA → Fibratus field mapping table.
func (h *SigmaHandler) FieldMappings(w http.ResponseWriter, r *http.Request) {
	mappings := sigma.GetFieldMappings()
	writeJSON(w, http.StatusOK, fleet.Response{Data: mappings})
}

// Validate handles POST /api/v1/orgs/{org_id}/sigma/validate
// Validates a SIGMA rule can be converted without actually importing it.
func (h *SigmaHandler) Validate(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 512*1024))
	if err != nil || len(body) == 0 {
		writeError(w, http.StatusBadRequest, "request body required")
		return
	}

	sigmaYAML := body
	if r.Header.Get("Content-Type") == "application/json" {
		var req struct {
			SigmaYAML string `json:"sigma_yaml"`
		}
		if err := json.Unmarshal(body, &req); err == nil && req.SigmaYAML != "" {
			sigmaYAML = []byte(req.SigmaYAML)
		}
	}

	result := sigma.Convert(sigmaYAML)

	// Run condition validation too
	if result.Success && result.Condition != "" {
		orgID := ctxutil.OrgIDFromContext(r.Context())
		macros := h.loadMacros(r, orgID)
		condResult := validator.ValidateConditionWithMacros(result.Condition, macros)
		if !condResult.Valid {
			for _, e := range condResult.Errors {
				result.Warnings = append(result.Warnings, "QL validation: "+e.Message)
			}
		}
	}

	resp := map[string]interface{}{
		"convertible":   result.Success,
		"unconvertible": result.Unconvertible,
		"reason":        result.Reason,
		"errors":        result.Errors,
		"warnings":      result.Warnings,
		"condition":     result.Condition,
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: resp})
}

// loadMacros loads org macros for condition validation.
func (h *SigmaHandler) loadMacros(r *http.Request, orgID string) map[string]*qlparser.Macro {
	if orgID == "" {
		return nil
	}
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

// ImportBatch handles POST /api/v1/orgs/{org_id}/sigma/import/batch
// Converts and imports multiple SIGMA rules at once.
func (h *SigmaHandler) ImportBatch(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	userID := ctxutil.UserIDFromContext(r.Context())

	body, err := io.ReadAll(io.LimitReader(r.Body, 5*1024*1024))
	if err != nil || len(body) == 0 {
		writeError(w, http.StatusBadRequest, "request body required")
		return
	}

	var req struct {
		Rules []string `json:"rules"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	macros := h.loadMacros(r, orgID)

	batchResult := sigma.BatchConversionResult{Total: len(req.Rules)}
	for _, rawYAML := range req.Rules {
		result := sigma.Convert([]byte(rawYAML))
		if !result.Success {
			if result.Unconvertible {
				batchResult.Skipped++
			} else {
				batchResult.Failed++
			}
			batchResult.Results = append(batchResult.Results, *result)
			continue
		}

		// Validate condition
		condResult := validator.ValidateConditionWithMacros(result.Condition, macros)

		// Parse and store rule
		var rule fleet.Rule
		if err := parseYAMLRule([]byte(result.FibratusYAML), &rule); err != nil {
			result.Errors = append(result.Errors, "parse generated YAML: "+err.Error())
			result.Success = false
			batchResult.Failed++
			batchResult.Results = append(batchResult.Results, *result)
			continue
		}

		rule.OrgID = orgID
		rule.RawYAML = result.FibratusYAML
		rule.Source = "sigma"

		if condResult.Valid {
			rule.ValidationStatus = "valid"
			rule.ValidationErrors = json.RawMessage(`[]`)
			rule.Enabled = true
		} else {
			rule.ValidationStatus = "invalid"
			errJSON, _ := json.Marshal(condResult.Errors)
			rule.ValidationErrors = errJSON
			rule.Enabled = false
		}

		existing, _ := h.rules.Get(r.Context(), orgID, rule.ID)
		if existing != nil {
			h.rules.Update(r.Context(), &rule)
		} else {
			h.rules.Create(r.Context(), &rule)
		}

		batchResult.Converted++
		batchResult.Results = append(batchResult.Results, *result)
	}

	logAudit(r, h.audit, h.users, userID, orgID, "create", "rule", "",
		fmt.Sprintf("sigma batch import: %d converted, %d failed, %d skipped",
			batchResult.Converted, batchResult.Failed, batchResult.Skipped), nil)

	writeJSON(w, http.StatusOK, fleet.Response{Data: batchResult})
}
