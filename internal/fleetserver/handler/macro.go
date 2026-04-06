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
	"strings"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// MacroHandler handles macro management API requests.
type MacroHandler struct {
	macros store.MacroStore
	audit  store.AuditStore
	users  store.UserStore
}

// NewMacroHandler creates a new macro handler.
func NewMacroHandler(macros store.MacroStore, audit store.AuditStore, users store.UserStore) *MacroHandler {
	return &MacroHandler{macros: macros, audit: audit, users: users}
}

// List handles GET /api/v1/orgs/{org_id}/macros
func (h *MacroHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	macros, err := h.macros.List(r.Context(), orgID)
	if err != nil {
		log.Errorf("fleet: list macros error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if macros == nil {
		macros = make([]*fleet.Macro, 0)
	}
	writeJSON(w, http.StatusOK, fleet.Response{
		Data: macros,
		Meta: &fleet.Pagination{Total: len(macros), Page: 1, PerPage: len(macros)},
	})
}

// Create handles POST /api/v1/orgs/{org_id}/macros
func (h *MacroHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	userID := ctxutil.UserIDFromContext(r.Context())

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var macro fleet.Macro
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "yaml") || strings.Contains(contentType, "x-yaml") {
		if err := parseMacroYAML(body, &macro); err != nil {
			writeError(w, http.StatusBadRequest, "invalid macro YAML: "+err.Error())
			return
		}
		macro.RawYAML = string(body)
	} else {
		if err := json.Unmarshal(body, &macro); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
	}

	if macro.Name == "" || macro.Expr == "" {
		writeError(w, http.StatusBadRequest, "macro name and expr are required")
		return
	}
	if macro.ID == "" {
		macro.ID = GenerateID()
	}
	macro.OrgID = orgID
	if macro.RawYAML == "" {
		macro.RawYAML = buildMacroYAML(&macro)
	}

	if err := h.macros.Create(r.Context(), &macro); err != nil {
		log.Errorf("fleet: create macro error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create macro")
		return
	}

	logAudit(r, h.audit, h.users, userID, orgID, "create", "macro", macro.ID, macro.Name, nil)
	log.Infof("fleet: macro created: %s (%s) in org %s", macro.Name, macro.ID, orgID)
	writeJSON(w, http.StatusCreated, fleet.Response{Data: macro})
}

// Update handles PUT /api/v1/orgs/{org_id}/macros/{id}
func (h *MacroHandler) Update(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	userID := ctxutil.UserIDFromContext(r.Context())
	parts := strings.Split(r.URL.Path, "/macros/")
	if len(parts) < 2 || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "macro ID required")
		return
	}
	macroID := strings.TrimSuffix(parts[1], "/")

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var macro fleet.Macro
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "yaml") || strings.Contains(contentType, "x-yaml") {
		if err := parseMacroYAML(body, &macro); err != nil {
			writeError(w, http.StatusBadRequest, "invalid macro YAML: "+err.Error())
			return
		}
		macro.RawYAML = string(body)
	} else {
		if err := json.Unmarshal(body, &macro); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
	}

	macro.ID = macroID
	macro.OrgID = orgID
	if macro.RawYAML == "" {
		macro.RawYAML = buildMacroYAML(&macro)
	}

	if err := h.macros.Update(r.Context(), &macro); err != nil {
		log.Errorf("fleet: update macro error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update macro")
		return
	}

	logAudit(r, h.audit, h.users, userID, orgID, "update", "macro", macroID, macro.Name, nil)
	writeJSON(w, http.StatusOK, fleet.Response{Data: macro})
}

// Delete handles DELETE /api/v1/orgs/{org_id}/macros/{id}
func (h *MacroHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	userID := ctxutil.UserIDFromContext(r.Context())
	parts := strings.Split(r.URL.Path, "/macros/")
	if len(parts) < 2 || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "macro ID required")
		return
	}
	macroID := strings.TrimSuffix(parts[1], "/")

	// Get name for audit log
	existing, _ := h.macros.Get(r.Context(), orgID, macroID)
	name := ""
	if existing != nil {
		name = existing.Name
	}

	if err := h.macros.Delete(r.Context(), orgID, macroID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete macro")
		return
	}

	logAudit(r, h.audit, h.users, userID, orgID, "delete", "macro", macroID, name, nil)
	w.WriteHeader(http.StatusNoContent)
}

// Upload handles POST /api/v1/orgs/{org_id}/macros/upload
// Accepts a raw YAML macro file (same format as rules/macros/macros.yml)
// and imports all macros, replacing existing ones with the same name.
func (h *MacroHandler) Upload(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	userID := ctxutil.UserIDFromContext(r.Context())

	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20)) // 5MB max
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	imported, err := h.macros.ImportFromYAML(r.Context(), orgID, body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse macros: "+err.Error())
		return
	}

	logAudit(r, h.audit, h.users, userID, orgID, "upload", "macros", "", fmt.Sprintf("Uploaded %d macros", imported), nil)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{
		"imported": imported,
	}})
}

func parseMacroYAML(data []byte, macro *fleet.Macro) error {
	var raw struct {
		Name        string `yaml:"name"`
		ID          string `yaml:"id"`
		Expr        string `yaml:"expr"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}
	macro.Name = raw.Name
	macro.ID = raw.ID
	macro.Expr = raw.Expr
	macro.Description = raw.Description
	return nil
}

func buildMacroYAML(m *fleet.Macro) string {
	b, _ := yaml.Marshal(map[string]string{
		"name": m.Name,
		"id":   m.ID,
		"expr": m.Expr,
	})
	return string(b)
}
