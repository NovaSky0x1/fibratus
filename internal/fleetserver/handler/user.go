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
	users  store.UserStore
	groups store.UserGroupStore
}

// NewUserHandler creates a new user handler.
func NewUserHandler(users store.UserStore, groups store.UserGroupStore) *UserHandler {
	return &UserHandler{users: users, groups: groups}
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
		Email           string   `json:"email"`
		Name            string   `json:"name"`
		Password        string   `json:"password"`
		Role            string   `json:"role"`
		OrgRestrictions []string `json:"org_restrictions"` // specific org IDs, empty = all orgs in account
		GroupIDs        []string `json:"group_ids"`        // assign to groups on creation
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
	// All non-root users get the "member" role — permissions come from groups
	req.Role = fleetauth.RoleMember

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

	// Serialize org restrictions
	orgRestrictions := ""
	if len(req.OrgRestrictions) > 0 {
		orBytes, _ := json.Marshal(req.OrgRestrictions)
		orgRestrictions = string(orBytes)
	}

	user := &fleet.User{
		ID: GenerateID(), Email: req.Email, Name: req.Name,
		Password: hashed, AccountID: accountID, Role: req.Role,
		OrgRestrictions: orgRestrictions,
		CreatedAt:       time.Now().UTC(),
	}
	if err := h.users.Create(r.Context(), user); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	// Add org access — if org restrictions specified, add to each; otherwise add to current org
	if len(req.OrgRestrictions) > 0 {
		for _, oid := range req.OrgRestrictions {
			h.users.AddOrgAccess(r.Context(), user.ID, oid, req.Role)
		}
	} else {
		h.users.AddOrgAccess(r.Context(), user.ID, orgID, req.Role)
	}

	// Assign to groups
	for _, gid := range req.GroupIDs {
		h.groups.AddMember(r.Context(), user.ID, gid)
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

// Update handles PUT /api/v1/orgs/{org_id}/users/{id} — updates user name/email.
func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/users/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "user ID required")
		return
	}
	userID := strings.TrimSuffix(parts[1], "/")

	var req struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.users.UpdateProfile(r.Context(), userID, req.Name, req.Email); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	log.Infof("fleet: user %s updated", userID)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "updated"}})
}

// ResetPassword handles PUT /api/v1/orgs/{org_id}/users/{id}/password — admin resets a user's password.
func (h *UserHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/users/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "user ID required")
		return
	}
	userID := strings.TrimSuffix(parts[1], "/password")
	userID = strings.TrimSuffix(userID, "/")

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}

	if err := fleetauth.ValidatePasswordPolicy(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	hashed, err := fleetauth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.users.UpdatePassword(r.Context(), userID, hashed); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset password")
		return
	}

	log.Infof("fleet: user %s password reset by admin", userID)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "password_reset"}})
}

// DisableTOTP handles DELETE /api/v1/orgs/{org_id}/users/{id}/totp — admin force-disables 2FA.
func (h *UserHandler) DisableTOTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/users/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "user ID required")
		return
	}
	userID := strings.TrimSuffix(parts[1], "/totp")
	userID = strings.TrimSuffix(userID, "/")

	if err := h.users.SetTOTP(r.Context(), userID, "", false, ""); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to disable 2FA")
		return
	}

	log.Infof("fleet: user %s 2FA disabled by admin", userID)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "totp_disabled"}})
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

// GetUserGroups handles GET /api/v1/orgs/{org_id}/users/{id}/groups
func (h *UserHandler) GetUserGroups(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/users/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "user ID required")
		return
	}
	userID := strings.TrimSuffix(parts[1], "/groups")
	userID = strings.TrimSuffix(userID, "/")

	memberships, err := h.groups.GetUserGroups(r.Context(), userID)
	if err != nil {
		log.Errorf("fleet: get user groups error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: memberships})
}

// UpdateUserGroups handles PUT /api/v1/orgs/{org_id}/users/{id}/groups
func (h *UserHandler) UpdateUserGroups(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/users/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "user ID required")
		return
	}
	userID := strings.TrimSuffix(parts[1], "/groups")
	userID = strings.TrimSuffix(userID, "/")

	var req struct {
		GroupIDs []string `json:"group_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get current memberships
	current, err := h.groups.GetUserGroups(r.Context(), userID)
	if err != nil {
		log.Errorf("fleet: get user groups error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Build sets for diff
	currentSet := make(map[string]bool, len(current))
	for _, m := range current {
		currentSet[m.GroupID] = true
	}
	desiredSet := make(map[string]bool, len(req.GroupIDs))
	for _, gid := range req.GroupIDs {
		desiredSet[gid] = true
	}

	// Add new memberships
	for _, gid := range req.GroupIDs {
		if !currentSet[gid] {
			if err := h.groups.AddMember(r.Context(), userID, gid); err != nil {
				log.Errorf("fleet: add group member error: %v", err)
			}
		}
	}

	// Remove old memberships
	for _, m := range current {
		if !desiredSet[m.GroupID] {
			if err := h.groups.RemoveMember(r.Context(), userID, m.GroupID); err != nil {
				log.Errorf("fleet: remove group member error: %v", err)
			}
		}
	}

	log.Infof("fleet: user %s groups updated to %v", userID, req.GroupIDs)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "updated"}})
}
