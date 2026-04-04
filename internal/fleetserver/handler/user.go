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
	"github.com/rabbitstack/fibratus/internal/fleetserver/fleetauth"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// UserHandler handles user management API requests.
type UserHandler struct {
	users store.UserStore
}

// NewUserHandler creates a new user handler.
func NewUserHandler(users store.UserStore) *UserHandler {
	return &UserHandler{users: users}
}

// Me handles GET /api/v1/auth/me — returns the current user.
func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := ctxutil.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	user, err := h.users.Get(r.Context(), userID)
	if err != nil || user == nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	user.Password = ""
	writeJSON(w, http.StatusOK, fleet.Response{Data: user})
}

// List handles GET /api/v1/orgs/{org_id}/users — lists users in the org.
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}
	users, err := h.users.ListByOrg(r.Context(), orgID)
	if err != nil {
		log.Errorf("fleet: list users error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	for _, u := range users {
		u.Password = ""
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: users})
}

// Create handles POST /api/v1/orgs/{org_id}/users — creates a new user and assigns to org.
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	accountID := ctxutil.AccountIDFromContext(r.Context())
	if orgID == "" || accountID == "" {
		writeError(w, http.StatusBadRequest, "org and account context required")
		return
	}

	var req struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Name == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email, name, and password are required")
		return
	}
	if err := fleetauth.ValidatePasswordPolicy(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Role == "" {
		req.Role = fleetauth.RoleViewer
	}
	if !fleetauth.ValidRole(req.Role) {
		writeError(w, http.StatusBadRequest, "invalid role: must be admin, analyst, or viewer")
		return
	}

	existing, _ := h.users.GetByEmail(r.Context(), req.Email)
	if existing != nil {
		writeError(w, http.StatusConflict, "email already registered")
		return
	}

	hashed, err := fleetauth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	user := &fleet.User{
		ID: GenerateID(), Email: req.Email, Name: req.Name,
		Password: hashed, AccountID: accountID, Role: req.Role,
		CreatedAt: time.Now().UTC(),
	}
	if err := h.users.Create(r.Context(), user); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	if err := h.users.AddOrgAccess(r.Context(), user.ID, orgID, req.Role); err != nil {
		log.Errorf("fleet: add org access error: %v", err)
	}

	log.Infof("fleet: user created: %s role=%s org=%s", user.Email, req.Role, orgID)
	user.Password = ""
	writeJSON(w, http.StatusCreated, fleet.Response{Data: user})
}

// UpdateRole handles PUT /api/v1/orgs/{org_id}/users/{id}/role — changes a user's role.
func (h *UserHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}
	parts := strings.Split(r.URL.Path, "/users/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "user ID required")
		return
	}
	userID := strings.TrimSuffix(parts[1], "/role")
	userID = strings.TrimSuffix(userID, "/")

	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Role == "" {
		writeError(w, http.StatusBadRequest, "role is required")
		return
	}
	if !fleetauth.ValidRole(req.Role) {
		writeError(w, http.StatusBadRequest, "invalid role: must be admin, analyst, or viewer")
		return
	}
	currentUserID := ctxutil.UserIDFromContext(r.Context())
	if userID == currentUserID {
		writeError(w, http.StatusBadRequest, "cannot change your own role")
		return
	}

	if err := h.users.UpdateRole(r.Context(), userID, orgID, req.Role); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update role")
		return
	}
	log.Infof("fleet: user %s role updated to %s", userID, req.Role)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "updated", "role": req.Role}})
}

// Delete handles DELETE /api/v1/orgs/{org_id}/users/{id} — deletes a user.
func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org context required")
		return
	}
	parts := strings.Split(r.URL.Path, "/users/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "user ID required")
		return
	}
	userID := strings.TrimSuffix(parts[1], "/")
	currentUserID := ctxutil.UserIDFromContext(r.Context())
	if userID == currentUserID {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	if err := h.users.Delete(r.Context(), userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	log.Infof("fleet: user %s deleted from org %s", userID, orgID)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "deleted"}})
}
