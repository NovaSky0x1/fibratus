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

// EnrollmentTokenHandler manages enrollment tokens via the dashboard API.
type EnrollmentTokenHandler struct {
	tokens store.EnrollmentTokenStore
}

// NewEnrollmentTokenHandler creates a new enrollment token handler.
func NewEnrollmentTokenHandler(tokens store.EnrollmentTokenStore) *EnrollmentTokenHandler {
	return &EnrollmentTokenHandler{tokens: tokens}
}

// Create handles POST /api/v1/orgs/{org_id}/enrollment-tokens
func (h *EnrollmentTokenHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	accountID := ctxutil.AccountIDFromContext(r.Context())
	userID := ctxutil.UserIDFromContext(r.Context())

	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}

	var req fleet.CreateEnrollmentTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.MaxUses <= 0 {
		req.MaxUses = 50
	}
	if req.ExpiresIn <= 0 {
		req.ExpiresIn = 24 * time.Hour
	}
	if req.Name == "" {
		req.Name = "enrollment-token"
	}

	token := &fleet.EnrollmentToken{
		ID:        "ft-enroll-" + GenerateID(),
		AccountID: accountID,
		OrgID:     orgID,
		Name:      req.Name,
		MaxUses:   req.MaxUses,
		ExpiresAt: time.Now().UTC().Add(req.ExpiresIn),
		CreatedBy: userID,
	}

	if err := h.tokens.Create(r.Context(), token); err != nil {
		log.Errorf("fleet: create enrollment token error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create token")
		return
	}

	log.Infof("fleet: enrollment token created: %s for org %s", token.ID, orgID)
	writeJSON(w, http.StatusCreated, fleet.Response{Data: token})
}

// List handles GET /api/v1/orgs/{org_id}/enrollment-tokens
func (h *EnrollmentTokenHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}

	tokens, err := h.tokens.ListByOrg(r.Context(), orgID)
	if err != nil {
		log.Errorf("fleet: list enrollment tokens error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: tokens})
}

// Delete handles DELETE /api/v1/orgs/{org_id}/enrollment-tokens/{id}
func (h *EnrollmentTokenHandler) Delete(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/enrollment-tokens/")
	if len(parts) < 2 || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "token ID required")
		return
	}
	tokenID := strings.TrimSuffix(parts[1], "/")

	if err := h.tokens.Delete(r.Context(), tokenID); err != nil {
		log.Errorf("fleet: delete enrollment token error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete token")
		return
	}

	log.Infof("fleet: enrollment token deleted: %s", tokenID)
	w.WriteHeader(http.StatusNoContent)
}
