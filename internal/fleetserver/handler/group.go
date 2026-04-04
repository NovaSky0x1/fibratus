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

// GroupHandler handles user group management API requests.
type GroupHandler struct {
	groups store.UserGroupStore
}

// NewGroupHandler creates a new group handler.
func NewGroupHandler(groups store.UserGroupStore) *GroupHandler {
	return &GroupHandler{groups: groups}
}

// List handles GET /api/v1/orgs/{org_id}/groups
func (h *GroupHandler) List(w http.ResponseWriter, r *http.Request) {
	accountID := ctxutil.AccountIDFromContext(r.Context())
	if accountID == "" {
		writeError(w, http.StatusBadRequest, "account context required")
		return
	}

	groups, err := h.groups.List(r.Context(), accountID)
	if err != nil {
		log.Errorf("fleet: list groups error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: groups})
}

// Create handles POST /api/v1/orgs/{org_id}/groups
func (h *GroupHandler) Create(w http.ResponseWriter, r *http.Request) {
	accountID := ctxutil.AccountIDFromContext(r.Context())
	if accountID == "" {
		writeError(w, http.StatusBadRequest, "account context required")
		return
	}

	var req struct {
		Name            string   `json:"name"`
		Description     string   `json:"description"`
		Permissions     []string `json:"permissions"`
		OrgRestrictions []string `json:"org_restrictions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	now := time.Now().UTC()
	group := &fleet.UserGroup{
		ID:              GenerateID(),
		AccountID:       accountID,
		Name:            req.Name,
		Description:     req.Description,
		Permissions:     req.Permissions,
		OrgRestrictions: req.OrgRestrictions,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := h.groups.Create(r.Context(), group); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create group")
		return
	}

	log.Infof("fleet: group created: %s (%s)", group.Name, group.ID)
	writeJSON(w, http.StatusCreated, fleet.Response{Data: group})
}

// Update handles PUT /api/v1/orgs/{org_id}/groups/{id}
func (h *GroupHandler) Update(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/groups/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "group ID required")
		return
	}
	groupID := strings.TrimSuffix(parts[1], "/")

	var req struct {
		Name            string   `json:"name"`
		Description     string   `json:"description"`
		Permissions     []string `json:"permissions"`
		OrgRestrictions []string `json:"org_restrictions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	group := &fleet.UserGroup{
		ID:              groupID,
		Name:            req.Name,
		Description:     req.Description,
		Permissions:     req.Permissions,
		OrgRestrictions: req.OrgRestrictions,
	}

	if err := h.groups.Update(r.Context(), group); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update group")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "updated"}})
}

// Delete handles DELETE /api/v1/orgs/{org_id}/groups/{id}
func (h *GroupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/groups/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "group ID required")
		return
	}
	groupID := strings.TrimSuffix(parts[1], "/")

	if err := h.groups.Delete(r.Context(), groupID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete group")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AddMember handles POST /api/v1/orgs/{org_id}/groups/{id}/members
func (h *GroupHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/groups/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "group ID required")
		return
	}
	groupID := strings.TrimSuffix(parts[1], "/members")
	groupID = strings.TrimSuffix(groupID, "/")

	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	if err := h.groups.AddMember(r.Context(), req.UserID, groupID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add member")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "added"}})
}

// RemoveMember handles DELETE /api/v1/orgs/{org_id}/groups/{id}/members/{user_id}
func (h *GroupHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/groups/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "group ID required")
		return
	}
	rest := parts[1]
	memberParts := strings.Split(rest, "/members/")
	if len(memberParts) < 2 {
		writeError(w, http.StatusBadRequest, "user ID required")
		return
	}
	groupID := memberParts[0]
	userID := strings.TrimSuffix(memberParts[1], "/")

	if err := h.groups.RemoveMember(r.Context(), userID, groupID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove member")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetPermissions handles GET /api/v1/orgs/{org_id}/permissions — returns all available permissions.
func (h *GroupHandler) GetPermissions(w http.ResponseWriter, r *http.Request) {
	permissions := []map[string]string{
		{"id": "agents:view", "name": "View Agents", "category": "Agents"},
		{"id": "agents:manage", "name": "Manage Agents", "category": "Agents"},
		{"id": "agents:delete", "name": "Delete Agents", "category": "Agents"},
		{"id": "detections:view", "name": "View Detections", "category": "Detections"},
		{"id": "events:view", "name": "View Events", "category": "Events"},
		{"id": "rules:view", "name": "View Rules", "category": "Rules"},
		{"id": "rules:manage", "name": "Create/Edit/Delete Rules", "category": "Rules"},
		{"id": "commands:view", "name": "View Commands", "category": "Active Response"},
		{"id": "commands:execute", "name": "Execute Commands (isolate, kill, shell)", "category": "Active Response"},
		{"id": "settings:view", "name": "View Settings", "category": "Settings"},
		{"id": "settings:manage", "name": "Manage Settings (tokens, sync, macros)", "category": "Settings"},
		{"id": "users:manage", "name": "Manage Users", "category": "User Management"},
		{"id": "audit:view", "name": "View Audit Log", "category": "Audit"},
		{"id": "organizations:manage", "name": "Manage Organizations", "category": "Organizations"},
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: permissions})
}
