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

// AdminHandler handles system administration API requests (root only).
type AdminHandler struct {
	accounts store.AccountStore
	orgs     store.OrgStore
	users    store.UserStore
}

// NewAdminHandler creates a new admin handler.
func NewAdminHandler(accounts store.AccountStore, orgs store.OrgStore, users store.UserStore) *AdminHandler {
	return &AdminHandler{accounts: accounts, orgs: orgs, users: users}
}

// ListAccounts handles GET /api/v1/admin/accounts
func (h *AdminHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}

	accounts, err := h.accounts.ListAll(r.Context())
	if err != nil {
		log.Errorf("fleet: list accounts error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: accounts})
}

// CreateAccount handles POST /api/v1/admin/accounts
func (h *AdminHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}

	var req struct {
		Name string `json:"name"`
		Plan string `json:"plan"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Plan == "" {
		req.Plan = "free"
	}

	now := time.Now().UTC()
	account := &fleet.Account{
		ID:        GenerateID(),
		Name:      req.Name,
		Plan:      req.Plan,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.accounts.Create(r.Context(), account); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create account")
		return
	}

	log.Infof("fleet: account created by root: %s (%s)", account.Name, account.ID)
	writeJSON(w, http.StatusCreated, fleet.Response{Data: account})
}

// DeleteAccount handles DELETE /api/v1/admin/accounts/{id}
func (h *AdminHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}

	accountID := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/accounts/")
	if accountID == "" {
		writeError(w, http.StatusBadRequest, "account ID required")
		return
	}

	if err := h.accounts.Delete(r.Context(), accountID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete account")
		return
	}

	log.Infof("fleet: account deleted by root: %s", accountID)
	w.WriteHeader(http.StatusNoContent)
}

// UpdateAccount handles PUT /api/v1/admin/accounts/{id}
func (h *AdminHandler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}

	accountID := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/accounts/")
	accountID = strings.TrimSuffix(accountID, "/")
	if accountID == "" || strings.Contains(accountID, "/") {
		writeError(w, http.StatusBadRequest, "account ID required")
		return
	}

	var req struct {
		Name                   string `json:"name"`
		Plan                   string `json:"plan"`
		Require2FA             *bool  `json:"require_2fa"`
		TelemetryRetentionDays *int   `json:"telemetry_retention_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Update 2FA if provided
	if req.Require2FA != nil {
		if err := h.accounts.UpdateSettings(r.Context(), accountID, *req.Require2FA, nil, nil, nil); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update account")
			return
		}
	}

	// Update retention if provided
	if req.TelemetryRetentionDays != nil && *req.TelemetryRetentionDays > 0 {
		if err := h.accounts.UpdateRetention(r.Context(), accountID, *req.TelemetryRetentionDays); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update retention")
			return
		}
	}

	// Update name/plan if provided
	if req.Name != "" || req.Plan != "" {
		h.accounts.UpdateProfile(r.Context(), accountID, req.Name, req.Plan)
	}

	log.Infof("fleet: account %s updated by root", accountID)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "updated"}})
}

// ListAccountOrgs handles GET /api/v1/admin/accounts/{id}/orgs
func (h *AdminHandler) ListAccountOrgs(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}

	// Extract account ID: /api/v1/admin/accounts/{id}/orgs
	parts := strings.Split(r.URL.Path, "/accounts/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "account ID required")
		return
	}
	accountID := strings.TrimSuffix(parts[1], "/orgs")
	accountID = strings.TrimSuffix(accountID, "/")

	orgs, err := h.orgs.ListByAccount(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: orgs})
}

// ListAccountUsers handles GET /api/v1/admin/accounts/{id}/users
func (h *AdminHandler) ListAccountUsers(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}

	parts := strings.Split(r.URL.Path, "/accounts/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "account ID required")
		return
	}
	accountID := strings.TrimSuffix(parts[1], "/users")
	accountID = strings.TrimSuffix(accountID, "/")

	users, err := h.users.ListByAccount(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	for _, u := range users {
		u.Password = ""
		u.IsLocked = !u.LockedUntil.IsZero() && time.Now().UTC().Before(u.LockedUntil)
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: users})
}

// ListAllUsers handles GET /api/v1/admin/users
func (h *AdminHandler) ListAllUsers(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}

	users, err := h.users.ListAll(r.Context())
	if err != nil {
		log.Errorf("fleet: list all users error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Build account name lookup
	accountNames := make(map[string]string)
	if accounts, err := h.accounts.ListAll(r.Context()); err == nil {
		for _, a := range accounts {
			accountNames[a.ID] = a.Name
		}
	}

	type userWithAccount struct {
		*fleet.User
		AccountName string `json:"account_name"`
	}
	result := make([]userWithAccount, 0, len(users))
	for _, u := range users {
		u.Password = ""
		u.IsLocked = !u.LockedUntil.IsZero() && time.Now().UTC().Before(u.LockedUntil)
		result = append(result, userWithAccount{
			User:        u,
			AccountName: accountNames[u.AccountID],
		})
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: result})
}

// UnlockUser handles POST /api/v1/admin/users/{id}/unlock
func (h *AdminHandler) UnlockUser(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}

	userID := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/users/")
	userID = strings.TrimSuffix(userID, "/unlock")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user ID required")
		return
	}

	if err := h.users.ResetLoginAttempts(r.Context(), userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to unlock user")
		return
	}

	log.Infof("fleet: user %s unlocked by root", userID)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "unlocked"}})
}

// UpdateUser handles PUT /api/v1/admin/users/{id}
func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}

	userID := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/users/")
	userID = strings.TrimSuffix(userID, "/")
	if userID == "" || strings.Contains(userID, "/") {
		writeError(w, http.StatusBadRequest, "user ID required")
		return
	}

	var req struct {
		Name            string   `json:"name"`
		Email           string   `json:"email"`
		Role            string   `json:"role"`
		AccountID       string   `json:"account_id"`
		OrgRestrictions []string `json:"org_restrictions"` // null/empty = all orgs, array = specific
		SetOrgRestrict  *bool    `json:"set_org_restrictions"` // explicit flag to clear restrictions
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != "" || req.Email != "" {
		h.users.UpdateProfile(r.Context(), userID, req.Name, req.Email)
	}
	if req.Role != "" && fleetauth.ValidRole(req.Role) {
		// Update role on the users table
		h.users.SetRole(r.Context(), userID, req.Role)
		// Update role in user_orgs for all their org memberships
		access, _ := h.users.GetOrgAccess(r.Context(), userID)
		for _, uo := range access {
			h.users.AddOrgAccess(r.Context(), userID, uo.OrgID, req.Role)
		}
	}
	if req.AccountID != "" {
		h.users.SetAccount(r.Context(), userID, req.AccountID)
	}
	// Handle org restrictions
	if req.SetOrgRestrict != nil || len(req.OrgRestrictions) > 0 {
		var orgJSON string
		if len(req.OrgRestrictions) > 0 {
			b, _ := json.Marshal(req.OrgRestrictions)
			orgJSON = string(b)
		}
		h.users.SetOrgRestrictions(r.Context(), userID, orgJSON)
		// Also update org access entries
		for _, oid := range req.OrgRestrictions {
			role := "viewer"
			if req.Role != "" {
				role = req.Role
			}
			h.users.AddOrgAccess(r.Context(), userID, oid, role)
		}
	}

	log.Infof("fleet: user %s updated by root", userID)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "updated"}})
}

// DeleteUser handles DELETE /api/v1/admin/users/{id}
func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}

	userID := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/users/")
	userID = strings.TrimSuffix(userID, "/")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user ID required")
		return
	}

	if err := h.users.Delete(r.Context(), userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	log.Infof("fleet: user %s deleted by root", userID)
	w.WriteHeader(http.StatusNoContent)
}

// ResetUserPassword handles POST /api/v1/admin/users/{id}/reset-password
func (h *AdminHandler) ResetUserPassword(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}

	userID := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/users/")
	userID = strings.TrimSuffix(userID, "/reset-password")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user ID required")
		return
	}

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

	log.Infof("fleet: user %s password reset by root", userID)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "password_reset"}})
}

// DisableUserTOTP handles POST /api/v1/admin/users/{id}/disable-totp
func (h *AdminHandler) DisableUserTOTP(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}

	userID := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/users/")
	userID = strings.TrimSuffix(userID, "/disable-totp")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user ID required")
		return
	}

	if err := h.users.SetTOTP(r.Context(), userID, "", false, ""); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to disable 2FA")
		return
	}

	log.Infof("fleet: user %s 2FA disabled by root", userID)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "totp_disabled"}})
}

// SwitchAccount handles POST /api/v1/admin/switch-account
// Root users call this to get a new JWT scoped to a different account.
func (h *AdminHandler) SwitchAccount(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}

	var req struct {
		AccountID string `json:"account_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AccountID == "" {
		writeError(w, http.StatusBadRequest, "account_id is required")
		return
	}

	// Verify account exists
	account, err := h.accounts.Get(r.Context(), req.AccountID)
	if err != nil || account == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{
		"account_id":   account.ID,
		"account_name": account.Name,
	}})
}
