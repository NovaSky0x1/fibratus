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
		Require2FA bool `json:"require_2fa"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.accounts.UpdateSettings(r.Context(), accountID, req.Require2FA); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update account")
		return
	}

	log.Infof("fleet: account %s updated by root: require_2fa=%v", accountID, req.Require2FA)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]bool{"require_2fa": req.Require2FA}})
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

	for _, u := range users {
		u.Password = ""
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: users})
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
