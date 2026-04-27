package handler

import (
	"context"
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
	accounts    store.AccountStore
	orgs        store.OrgStore
	users       store.UserStore
	groups      store.UserGroupStore
	authHandler *AuthHandler
}

// NewAdminHandler creates a new admin handler.
func NewAdminHandler(accounts store.AccountStore, orgs store.OrgStore, users store.UserStore) *AdminHandler {
	return &AdminHandler{accounts: accounts, orgs: orgs, users: users}
}

// SetGroupStore sets the user group store for default group seeding.
func (h *AdminHandler) SetGroupStore(s store.UserGroupStore) {
	h.groups = s
}

// SetAuthHandler sets the auth handler for org defaults seeding on account creation.
func (h *AdminHandler) SetAuthHandler(a *AuthHandler) {
	h.authHandler = a
}

// seedDefaultGroups creates the default user groups for a new account.
// Returns the Administrators group ID so the initial user can be added.
func (h *AdminHandler) seedDefaultGroups(ctx context.Context, accountID string) string {
	var adminGroupID string
	if h.groups == nil {
		return ""
	}

	defaults := []struct {
		name        string
		description string
		permissions []string
	}{
		{
			name:        "Administrators",
			description: "Full access to all features within this account",
			permissions: []string{
				"page:overview", "page:agents", "page:detections", "page:events",
				"page:rules", "page:macros", "page:audit", "page:management", "page:process_tree",
				"agents:view", "agents:manage", "agents:delete",
				"agents:view_events", "agents:view_detections", "agents:view_processes",
				"agents:view_network", "agents:view_services", "agents:view_drivers",
				"agents:view_autoruns", "agents:view_software", "agents:view_users",
				"agents:view_files", "agents:view_registry", "agents:view_eventlog",
				"agents:view_terminal", "agents:view_captures", "agents:view_history",
				"detections:view", "detections:manage",
				"events:view",
				"rules:view", "rules:manage",
				"commands:view", "commands:execute",
				"response:isolate", "response:unisolate", "response:kill_process",
				"response:run_command", "response:browse_files", "response:download_file",
				"response:collect_info", "response:uninstall",
				"captures:view", "captures:create", "captures:delete",
				"telemetry:view", "telemetry:configure",
				"settings:view", "settings:manage", "settings:macros",
				"enrollment:view", "enrollment:manage",
				"github_sync:view", "github_sync:manage",
				"users:manage", "users:groups", "audit:view",
				"organizations:manage",
			},
		},
		{
			name:        "Analysts",
			description: "Investigation and rule management — view telemetry, manage detection rules",
			permissions: []string{
				"page:overview", "page:agents", "page:detections", "page:events",
				"page:rules", "page:macros", "page:audit", "page:process_tree",
				"agents:view",
				"agents:view_events", "agents:view_detections", "agents:view_processes",
				"agents:view_network", "agents:view_services", "agents:view_drivers",
				"agents:view_autoruns", "agents:view_software", "agents:view_users",
				"agents:view_files", "agents:view_registry", "agents:view_eventlog",
				"agents:view_captures", "agents:view_history",
				"detections:view", "detections:manage",
				"events:view",
				"rules:view", "rules:manage",
				"commands:view",
				"captures:view",
				"telemetry:view",
				"enrollment:view",
				"github_sync:view",
				"audit:view",
			},
		},
		{
			name:        "Read Only",
			description: "View-only access to agents, detections, events, and rules",
			permissions: []string{
				"page:overview", "page:agents", "page:detections", "page:events",
				"page:rules", "page:audit", "page:process_tree",
				"agents:view",
				"agents:view_events", "agents:view_detections", "agents:view_processes",
				"agents:view_network", "agents:view_services", "agents:view_drivers",
				"agents:view_autoruns", "agents:view_software", "agents:view_users",
				"agents:view_files", "agents:view_registry", "agents:view_eventlog",
				"agents:view_captures", "agents:view_history",
				"detections:view",
				"events:view",
				"rules:view",
				"commands:view",
				"captures:view",
				"telemetry:view",
			},
		},
	}

	for _, d := range defaults {
		id := GenerateID()
		now := time.Now().UTC()
		g := &fleet.UserGroup{
			ID:          id,
			AccountID:   accountID,
			Name:        d.name,
			Description: d.description,
			Permissions: d.permissions,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := h.groups.Create(ctx, g); err != nil {
			log.Warnf("fleet: failed to seed default group %q for account %s: %v", d.name, accountID, err)
		}
		if d.name == "Administrators" {
			adminGroupID = id
		}
	}
	log.Infof("fleet: seeded default groups for account %s", accountID)
	return adminGroupID
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
		Name     string `json:"name"`
		Plan     string `json:"plan"`
		Email    string `json:"email"`
		Password string `json:"password"`
		UserName string `json:"user_name"`
		OrgName  string `json:"org_name"`
		UserRole string `json:"user_role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Plan == "" {
		req.Plan = "free"
	}
	if req.OrgName == "" {
		req.OrgName = req.Name
	}

	now := time.Now().UTC()
	accountID := GenerateID()
	account := &fleet.Account{
		ID:        accountID,
		Name:      req.Name,
		Plan:      req.Plan,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.accounts.Create(r.Context(), account); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create account")
		return
	}

	// Create default organization
	orgID := GenerateID()
	org := &fleet.Organization{
		ID:        orgID,
		AccountID: accountID,
		Name:      req.OrgName,
		Slug:      slugify(req.OrgName),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.orgs.Create(r.Context(), org); err != nil {
		log.Warnf("fleet: admin create account: failed to create org: %v", err)
	}

	// Seed default groups
	adminGroupID := h.seedDefaultGroups(r.Context(), accountID)

	// Create initial admin user if email + password provided
	var userID string
	if req.Email != "" && req.Password != "" {
		hashedPassword, err := fleetauth.HashPassword(req.Password)
		if err != nil {
			log.Warnf("fleet: admin create account: failed to hash password: %v", err)
		} else {
			userID = GenerateID()
			userRole := fleetauth.RoleMember
			if req.UserRole == "root" {
				userRole = "root"
			}
			userName := req.UserName
			if userName == "" {
				userName = req.Email
			}
			user := &fleet.User{
				ID:        userID,
				Email:     req.Email,
				Name:      userName,
				Password:  hashedPassword,
				AccountID: accountID,
				Role:      userRole,
				CreatedAt: now,
			}
			if err := h.users.Create(r.Context(), user); err != nil {
				log.Warnf("fleet: admin create account: failed to create user: %v", err)
			} else {
				h.users.AddOrgAccess(r.Context(), userID, orgID, "admin")
				if adminGroupID != "" {
					h.groups.AddMember(r.Context(), userID, adminGroupID)
				}
			}
		}
	}

	// Seed default macros and rules for the org (background)
	if h.authHandler != nil {
		go h.authHandler.seedOrgDefaults(context.Background(), orgID)
	}

	log.Infof("fleet: account created by root: %s (%s) with org %s, user %s", account.Name, accountID, orgID, userID)
	writeJSON(w, http.StatusCreated, fleet.Response{Data: map[string]interface{}{
		"account": account,
		"org_id":  orgID,
		"user_id": userID,
	}})
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

	// Update retention if provided. Persist to Postgres, then propagate to
	// the active ClickHouse via the auth handler's retention callback so the
	// per-org TTL on the live tables (Cloud or local) reflects the new value.
	// Without the callback the DB row would be correct but the table TTL on
	// the running ClickHouse would silently keep the old value.
	if req.TelemetryRetentionDays != nil && *req.TelemetryRetentionDays > 0 {
		if err := h.accounts.UpdateRetention(r.Context(), accountID, *req.TelemetryRetentionDays); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update retention")
			return
		}
		if h.authHandler != nil {
			h.authHandler.ApplyRetentionToAccountOrgs(r.Context(), accountID, *req.TelemetryRetentionDays)
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

// CreateUser handles POST /api/v1/admin/users
// Creates a user in any account — root can create users including other root users.
func (h *AdminHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}

	var req struct {
		Email     string `json:"email"`
		Name      string `json:"name"`
		Password  string `json:"password"`
		Role      string `json:"role"`
		AccountID string `json:"account_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" || req.AccountID == "" {
		writeError(w, http.StatusBadRequest, "email, password, and account_id are required")
		return
	}
	if req.Name == "" {
		req.Name = req.Email
	}
	if req.Role == "" {
		req.Role = fleetauth.RoleMember
	}

	// Verify account exists
	account, err := h.accounts.Get(r.Context(), req.AccountID)
	if err != nil || account == nil {
		writeError(w, http.StatusBadRequest, "account not found")
		return
	}

	hashedPassword, err := fleetauth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	now := time.Now().UTC()
	userID := GenerateID()
	user := &fleet.User{
		ID:        userID,
		Email:     req.Email,
		Name:      req.Name,
		Password:  hashedPassword,
		AccountID: req.AccountID,
		Role:      req.Role,
		CreatedAt: now,
	}
	if err := h.users.Create(r.Context(), user); err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			writeError(w, http.StatusConflict, "a user with this email already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	// Grant access to all orgs in the account
	orgs, _ := h.orgs.ListByAccount(r.Context(), req.AccountID)
	for _, org := range orgs {
		h.users.AddOrgAccess(r.Context(), userID, org.ID, "admin")
	}

	// Add to Administrators group if one exists
	if h.groups != nil {
		groups, _ := h.groups.List(r.Context(), req.AccountID)
		for _, g := range groups {
			if g.Name == "Administrators" {
				h.groups.AddMember(r.Context(), userID, g.ID)
				break
			}
		}
	}

	log.Infof("fleet: user %s (%s) created by root in account %s with role %s", req.Email, userID, req.AccountID, req.Role)
	writeJSON(w, http.StatusCreated, fleet.Response{Data: user})
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

// ListPendingUsers handles GET /api/v1/admin/pending-users — root-only.
// Returns users awaiting signup approval, newest first.
func (h *AdminHandler) ListPendingUsers(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}
	users, err := h.users.ListPendingUsers(r.Context())
	if err != nil {
		log.Errorf("fleet: list pending users: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: users})
}

// ApprovePendingUser handles POST /api/v1/admin/pending-users/{id}/approve.
// Transitions the user to 'approved' so they can log in.
func (h *AdminHandler) ApprovePendingUser(w http.ResponseWriter, r *http.Request) {
	h.setPendingUserStatus(w, r, fleet.UserStatusApproved, "approved")
}

// RejectPendingUser handles POST /api/v1/admin/pending-users/{id}/reject.
// Transitions the user to 'rejected'. Login stays blocked.
func (h *AdminHandler) RejectPendingUser(w http.ResponseWriter, r *http.Request) {
	h.setPendingUserStatus(w, r, fleet.UserStatusRejected, "rejected")
}

func (h *AdminHandler) setPendingUserStatus(w http.ResponseWriter, r *http.Request, status, action string) {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return
	}
	// Path: /api/v1/admin/pending-users/{id}/approve|reject
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 6 {
		writeError(w, http.StatusBadRequest, "user id required")
		return
	}
	userID := parts[4]
	user, err := h.users.Get(r.Context(), userID)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if user.Status != fleet.UserStatusPending {
		writeError(w, http.StatusConflict, "user is not pending approval")
		return
	}
	if err := h.users.SetStatus(r.Context(), userID, status); err != nil {
		log.Errorf("fleet: set status: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	log.Infof("fleet: pending user %s (%s) %s by root", user.Email, userID, action)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": status}})
}
