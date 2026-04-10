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

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// AuditHandler handles audit log API requests.
type AuditHandler struct {
	audit store.AuditStore
	orgs  store.OrgStore
}

// NewAuditHandler creates a new audit handler.
func NewAuditHandler(audit store.AuditStore) *AuditHandler {
	return &AuditHandler{audit: audit}
}

// SetOrgStore sets the org store for cross-org aggregation.
func (h *AuditHandler) SetOrgStore(orgs store.OrgStore) { h.orgs = orgs }

// List handles GET /api/v1/orgs/{org_id}/audit-log
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())

	opts := fleet.ListOptions{
		Page:    intParam(r, "page", 1),
		PerPage: intParam(r, "per_page", 50),
	}

	// Cross-org aggregation when no org is selected
	if orgID == "" {
		orgIDs := accountOrgIDs(r, h.orgs)
		if len(orgIDs) == 0 {
			writeError(w, http.StatusBadRequest, "org or account context required")
			return
		}
		allEntries := make([]*fleet.AuditEntry, 0)
		total := 0
		for _, oid := range orgIDs {
			entries, t, err := h.audit.List(r.Context(), oid, opts)
			if err != nil {
				continue
			}
			allEntries = append(allEntries, entries...)
			total += t
		}
		writeJSON(w, http.StatusOK, fleet.Response{
			Data: allEntries,
			Meta: &fleet.Pagination{Total: total, Page: opts.Page, PerPage: opts.PerPage},
		})
		return
	}

	entries, total, err := h.audit.List(r.Context(), orgID, opts)
	if err != nil {
		log.Errorf("fleet: list audit log error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{
		Data: entries,
		Meta: &fleet.Pagination{Total: total, Page: opts.Page, PerPage: opts.PerPage},
	})
}

// logAudit records an audit entry. It resolves the user email from the user ID
// and captures the client IP. This is fire-and-forget — errors are logged but
// don't affect the response.
func logAudit(r *http.Request, auditStore store.AuditStore, userStore store.UserStore,
	userID, orgID, action, resourceType, resourceID, resourceName string, details interface{}) {

	if auditStore == nil {
		return
	}

	email := ""
	if userStore != nil && userID != "" {
		user, err := userStore.Get(r.Context(), userID)
		if err == nil && user != nil {
			email = user.Email
		}
	}

	var detailsJSON json.RawMessage
	if details != nil {
		b, _ := json.Marshal(details)
		detailsJSON = b
	}

	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}

	entry := &fleet.AuditEntry{
		ID:           GenerateID(),
		OrgID:        orgID,
		UserID:       userID,
		UserEmail:    email,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Details:      detailsJSON,
		IPAddress:    ip,
	}

	if err := auditStore.Log(r.Context(), entry); err != nil {
		log.Warnf("fleet: audit log error: %v", err)
	}
}
