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
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/fleetauth"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store/postgres"
	"github.com/rabbitstack/fibratus/internal/fleetserver/validator"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// AuthHandler handles authentication API requests.
// RetentionCallback is called when telemetry retention changes to update ClickHouse TTL.
// orgID identifies which organization's table to update.
type RetentionCallback func(orgID string, days int) error

type AuthHandler struct {
	accounts       store.AccountStore
	orgs           store.OrgStore
	users          store.UserStore
	agents         store.AgentStore
	commands       store.CommandStore
	eventlogPolicy store.EventLogPolicyStore
	groups         store.UserGroupStore
	apiKeys        store.APIKeyStore
	macros         store.MacroStore
	rules          store.RuleStore
	jwtSecret      string
	onCmdCreated   CommandPushCallback
	onRetentionChange RetentionCallback
	settings       store.SettingsStore
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(accounts store.AccountStore, orgs store.OrgStore, users store.UserStore, agents store.AgentStore, commands store.CommandStore, jwtSecret string) *AuthHandler {
	return &AuthHandler{
		accounts:  accounts,
		orgs:      orgs,
		users:     users,
		agents:    agents,
		commands:  commands,
		jwtSecret: jwtSecret,
	}
}

// SetEventLogPolicyStore sets the event log policy store for account-level propagation.
func (h *AuthHandler) SetEventLogPolicyStore(s store.EventLogPolicyStore) {
	h.eventlogPolicy = s
}

// SetSettingsStore wires the server-wide settings store so signup gating and
// other policy decisions can read non-secret config without the handler
// importing a concrete store.
func (h *AuthHandler) SetSettingsStore(s store.SettingsStore) {
	h.settings = s
}

// SignupRequiresApproval reads the server-wide signup approval policy. Defaults
// to true (preserve current behaviour) when the setting is unset or unreachable.
func (h *AuthHandler) SignupRequiresApproval(ctx context.Context) bool {
	if h.settings == nil {
		return true
	}
	return h.settings.GetBoolOr(ctx, "signup.require_approval", true)
}

// SetRetentionCallback registers a callback for when telemetry retention changes.
func (h *AuthHandler) SetRetentionCallback(cb RetentionCallback) {
	h.onRetentionChange = cb
}

// ApplyRetentionToAccountOrgs walks every org in the account and fires the
// retention callback so the live ClickHouse table TTL is updated for each.
// Returns nil even if individual orgs fail — failures are logged. Used by the
// root admin path to propagate retention edits made via /admin/accounts/{id}.
func (h *AuthHandler) ApplyRetentionToAccountOrgs(ctx context.Context, accountID string, days int) {
	if h.onRetentionChange == nil || h.orgs == nil {
		return
	}
	orgs, err := h.orgs.ListByAccount(ctx, accountID)
	if err != nil {
		log.Warnf("fleet: list orgs for retention update (account=%s): %v", accountID, err)
		return
	}
	for _, org := range orgs {
		if err := h.onRetentionChange(org.ID, days); err != nil {
			log.Warnf("fleet: clickhouse TTL update failed for org %s: %v", org.ID, err)
		}
	}
}

// SetCommandPushCallback registers a callback for instant command delivery.
func (h *AuthHandler) SetCommandPushCallback(cb CommandPushCallback) {
	h.onCmdCreated = cb
}

// SetMacroStore sets the macro store for seeding defaults on signup.
func (h *AuthHandler) SetMacroStore(s store.MacroStore) {
	h.macros = s
}

// SetRuleStore sets the rule store for seeding defaults on signup.
func (h *AuthHandler) SetRuleStore(s store.RuleStore) {
	h.rules = s
}

// SetGroupStore sets the user group store for permission resolution.
func (h *AuthHandler) SetGroupStore(s store.UserGroupStore) {
	h.groups = s
}

// SetAPIKeyStore sets the API key store for programmatic access key management.
func (h *AuthHandler) SetAPIKeyStore(s store.APIKeyStore) {
	h.apiKeys = s
}

// propagateTamperProtection queues set_tamper_protection commands to all
// agents in the given org. This ensures that toggling tamper protection at
// the account or org level actually enables it on the agents, not just in
// the database.
func (h *AuthHandler) propagateTamperProtection(ctx context.Context, orgID string, enabled bool) {
	agents, _, err := h.agents.List(ctx, orgID, fleet.AgentListOptions{
		ListOptions: fleet.ListOptions{PerPage: 10000},
	})
	if err != nil {
		log.Errorf("fleet: failed to list agents for tamper propagation in org %s: %v", orgID, err)
		return
	}
	payload, _ := json.Marshal(map[string]bool{"enabled": enabled})
	for _, agent := range agents {
		cmd := &fleet.Command{
			ID:        GenerateID(),
			OrgID:     orgID,
			AgentID:   agent.ID,
			Type:      fleet.CmdSetTamperProtection,
			Payload:   payload,
			Status:    fleet.CmdStatusPending,
			CreatedBy: "system",
			CreatedAt: time.Now().UTC(),
		}
		if err := h.commands.Create(ctx, cmd); err != nil {
			log.Errorf("fleet: failed to queue tamper command for agent %s: %v", agent.ID, err)
			continue
		}
		// Update the agent DB record to reflect the new state
		agent.TamperProtection = enabled
		if err := h.agents.Update(ctx, agent); err != nil {
			log.Errorf("fleet: failed to update tamper state for agent %s: %v", agent.ID, err)
		}
		// Try instant push via gRPC if available
		if h.onCmdCreated != nil {
			if h.onCmdCreated(agent.ID, cmd.ID, cmd.Type, cmd.Payload) {
				h.commands.MarkRunning(ctx, cmd.ID)
			}
		}
		log.Infof("fleet: queued tamper protection %v for agent %s (%s)", enabled, agent.Hostname, agent.ID)
	}
}

// propagateEventLogPolicy pushes the event log collection policy to all agents
// in the given org when the account-level toggle changes.
func (h *AuthHandler) propagateEventLogPolicy(ctx context.Context, orgID string, enabled bool) {
	// Get the org's policy (or use recommended defaults if none exists)
	var policy *fleet.EventLogPolicy
	if h.eventlogPolicy != nil {
		policy, _ = h.eventlogPolicy.Get(ctx, orgID)
	}
	if policy == nil {
		// Create default policy with core channels
		channels := []fleet.EventLogPolicyChannel{
			{Name: "Security", CollectAll: true},
			{Name: "System", CollectAll: true},
			{Name: "Microsoft-Windows-Sysmon/Operational", CollectAll: true},
		}
		policy = &fleet.EventLogPolicy{
			ID:       GenerateID(),
			OrgID:    orgID,
			Enabled:  enabled,
			Channels: channels,
		}
		if h.eventlogPolicy != nil {
			h.eventlogPolicy.Upsert(ctx, policy)
		}
	} else {
		policy.Enabled = enabled
		if h.eventlogPolicy != nil {
			h.eventlogPolicy.Upsert(ctx, policy)
		}
	}

	agents, _, err := h.agents.List(ctx, orgID, fleet.AgentListOptions{
		ListOptions: fleet.ListOptions{PerPage: 10000},
	})
	if err != nil {
		log.Errorf("fleet: failed to list agents for eventlog policy propagation in org %s: %v", orgID, err)
		return
	}

	payload, _ := json.Marshal(policy)
	for _, agent := range agents {
		cmd := &fleet.Command{
			ID:        GenerateID(),
			OrgID:     orgID,
			AgentID:   agent.ID,
			Type:      fleet.CmdSetEventLogPolicy,
			Payload:   payload,
			Status:    fleet.CmdStatusPending,
			CreatedBy: "system",
			CreatedAt: time.Now().UTC(),
		}
		if err := h.commands.Create(ctx, cmd); err != nil {
			log.Errorf("fleet: failed to queue eventlog policy command for agent %s: %v", agent.ID, err)
			continue
		}
		if h.onCmdCreated != nil {
			if h.onCmdCreated(agent.ID, cmd.ID, cmd.Type, cmd.Payload) {
				h.commands.MarkRunning(ctx, cmd.ID)
			}
		}
		log.Infof("fleet: queued eventlog policy for agent %s (%s)", agent.Hostname, agent.ID)
	}
}

// Signup handles POST /api/v1/auth/signup
func (h *AuthHandler) Signup(w http.ResponseWriter, r *http.Request) {
	var req fleet.SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate required fields
	if req.Email == "" || req.Name == "" || req.Password == "" || req.AccountName == "" {
		writeError(w, http.StatusBadRequest, "email, name, password, and account_name are required")
		return
	}
	if !emailRegex.MatchString(req.Email) {
		writeError(w, http.StatusBadRequest, "invalid email address")
		return
	}
	if err := fleetauth.ValidatePasswordPolicy(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Check if email is already taken
	existing, err := h.users.GetByEmail(r.Context(), req.Email)
	if err != nil {
		log.Errorf("fleet: signup lookup error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, "email already registered")
		return
	}

	// Hash password
	hashedPassword, err := fleetauth.HashPassword(req.Password)
	if err != nil {
		log.Errorf("fleet: signup hash error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	now := time.Now().UTC()

	// Create account
	accountID := GenerateID()
	account := &fleet.Account{
		ID:        accountID,
		Name:      req.AccountName,
		Plan:      "free",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.accounts.Create(r.Context(), account); err != nil {
		log.Errorf("fleet: signup create account error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Create organization
	orgName := req.OrgName
	if orgName == "" {
		orgName = req.AccountName
	}
	orgID := GenerateID()
	org := &fleet.Organization{
		ID:        orgID,
		AccountID: accountID,
		Name:      orgName,
		Slug:      slugify(orgName),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.orgs.Create(r.Context(), org); err != nil {
		log.Errorf("fleet: signup create org error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Seed default macros and official rules for the new org
	go h.seedOrgDefaults(context.Background(), orgID)

	// Decide signup approval policy. Default is "require approval" (server-wide
	// safety default); operators can flip the signup.require_approval setting
	// to false to enable open registration (e.g. for a public preview).
	requireApproval := h.SignupRequiresApproval(r.Context())
	initialStatus := fleet.UserStatusApproved
	if requireApproval {
		initialStatus = fleet.UserStatusPending
	}

	userID := GenerateID()
	user := &fleet.User{
		ID:        userID,
		Email:     req.Email,
		Name:      req.Name,
		Password:  hashedPassword,
		AccountID: accountID,
		Role:      fleetauth.RoleMember,
		Status:    initialStatus,
		CreatedAt: now,
	}
	if err := h.users.Create(r.Context(), user); err != nil {
		log.Errorf("fleet: signup create user error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Add user org access
	if err := h.users.AddOrgAccess(r.Context(), userID, orgID, fleetauth.RoleMember); err != nil {
		log.Errorf("fleet: signup add org access error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Seed default groups for the new account and add user to Administrators.
	// When approval is required these permissions take effect post-approval;
	// when approval is off the user immediately has admin rights on their own
	// account (which is the whole point — self-service signup).
	if h.groups != nil {
		adminGroupID := h.seedDefaultGroupsForSignup(r.Context(), accountID)
		if adminGroupID != "" {
			h.groups.AddMember(r.Context(), userID, adminGroupID)
			log.Infof("fleet: added signup user %s to Administrators group %s", userID, adminGroupID)
		}
	}

	if requireApproval {
		log.Infof("fleet: pending signup: %s (%s) — awaiting root admin approval", account.Name, accountID)
		// Intentionally DO NOT mint a JWT. The user cannot log in until a root
		// admin transitions the row to 'approved' via /api/v1/admin/pending-users.
		resp := fleet.SignupResponse{
			AccountID:        accountID,
			OrgID:            orgID,
			UserID:           userID,
			PendingApproval:  true,
			MFASetupRequired: false,
		}
		writeJSON(w, http.StatusCreated, fleet.Response{Data: resp})
		return
	}

	// Open-signup path: mint a JWT immediately so the new user is logged in
	// without an admin step. MFA setup is handled by the standard login flow
	// if the account requires 2FA.
	log.Infof("fleet: open signup approved: %s (%s)", account.Name, accountID)
	token, err := fleetauth.GenerateJWT(h.jwtSecret, user.ID, user.AccountID, user.Role)
	if err != nil {
		log.Errorf("fleet: signup mint token error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	resp := fleet.SignupResponse{
		AccountID:        accountID,
		OrgID:            orgID,
		UserID:           userID,
		PendingApproval:  false,
		MFASetupRequired: account.Require2FA,
		Token:            token,
	}
	writeJSON(w, http.StatusCreated, fleet.Response{Data: resp})
}

// Login handles POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req fleet.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, err := h.users.GetByEmail(r.Context(), req.Email)
	if err != nil {
		log.Errorf("fleet: login lookup error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Signup approval gate: block non-approved users before credential check
	// so password timing does not leak account status.
	switch user.Status {
	case fleet.UserStatusPending, "":
		if user.Status == fleet.UserStatusPending {
			writeError(w, http.StatusForbidden, "your account is awaiting administrator approval")
			return
		}
	case fleet.UserStatusRejected:
		writeError(w, http.StatusForbidden, "your account was rejected")
		return
	case fleet.UserStatusSuspended:
		writeError(w, http.StatusForbidden, "your account is suspended")
		return
	}

	// Check account lockout
	if !user.LockedUntil.IsZero() && time.Now().UTC().Before(user.LockedUntil) {
		remaining := time.Until(user.LockedUntil).Round(time.Second)
		writeError(w, http.StatusTooManyRequests,
			fmt.Sprintf("account locked, try again in %s", remaining))
		return
	}

	// Verify password
	if err := fleetauth.CheckPassword(user.Password, req.Password); err != nil {
		// Increment failed attempts
		h.users.IncrementLoginAttempts(r.Context(), user.ID)
		attempts := user.LoginAttempts + 1

		// Lock after 5 failed attempts (exponential backoff: 5min, 10min, 20min...)
		if attempts >= 5 {
			lockDuration := time.Duration(5*math.Pow(2, float64(attempts/5-1))) * time.Minute
			if lockDuration > time.Hour {
				lockDuration = time.Hour
			}
			lockUntil := time.Now().UTC().Add(lockDuration)
			h.users.LockAccount(r.Context(), user.ID, lockUntil)
			log.Warnf("fleet: account locked for %s: %s (%d attempts)", lockDuration, user.Email, attempts)
		}

		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Password correct — check TOTP if enabled
	if user.TOTPEnabled {
		if req.TOTPCode == "" {
			// Signal the client that TOTP is required
			writeJSON(w, http.StatusOK, fleet.Response{Data: fleet.LoginResponse{
				TOTPRequired: true,
			}})
			return
		}
		// Validate TOTP code
		if !fleetauth.ValidateTOTP(user.TOTPSecret, req.TOTPCode) {
			// Check recovery codes
			if !useRecoveryCode(r.Context(), h.users, user, req.TOTPCode) {
				h.users.IncrementLoginAttempts(r.Context(), user.ID)
				writeError(w, http.StatusUnauthorized, "invalid TOTP code")
				return
			}
		}
	}

	// Success — reset lockout state
	h.users.ResetLoginAttempts(r.Context(), user.ID)

	token, err := fleetauth.GenerateJWT(h.jwtSecret, user.ID, user.AccountID, user.Role)
	if err != nil {
		log.Errorf("fleet: login generate token error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	log.Infof("fleet: user logged in: %s (%s)", user.Email, user.ID)

	user.Password = ""
	resp := fleet.LoginResponse{
		Token: token,
		User:  *user,
	}

	// 2FA is mandatory — if user hasn't set it up, flag for setup during login
	if !user.TOTPEnabled {
		resp.MFASetupRequired = true
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: resp})
}

// useRecoveryCode checks if the code matches a recovery code, and removes it if so.
func useRecoveryCode(ctx context.Context, users store.UserStore, user *fleet.User, code string) bool {
	if user.RecoveryCodes == "" {
		return false
	}
	codes := strings.Split(user.RecoveryCodes, ",")
	for i, c := range codes {
		if strings.EqualFold(strings.TrimSpace(c), strings.TrimSpace(code)) {
			// Remove used code
			remaining := append(codes[:i], codes[i+1:]...)
			users.SetTOTP(ctx, user.ID, user.TOTPSecret, user.TOTPEnabled, strings.Join(remaining, ","))
			return true
		}
	}
	return false
}

// GetCurrentUser handles GET /api/v1/auth/me
func (h *AuthHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID := ctxutil.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	user, err := h.users.Get(r.Context(), userID)
	if err != nil || user == nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	user.Password = ""

	// Include account settings (2FA enforcement)
	account, _ := h.accounts.Get(r.Context(), user.AccountID)
	resp := map[string]interface{}{
		"id":           user.ID,
		"email":        user.Email,
		"name":         user.Name,
		"account_id":   user.AccountID,
		"role":         user.Role,
		"totp_enabled": user.TOTPEnabled,
		"created_at":   user.CreatedAt,
	}
	if account != nil {
		resp["account_name"] = account.Name
		resp["require_2fa"] = account.Require2FA
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: resp})
}

// UpdateMyProfile handles PUT /api/v1/auth/me — updates the current user's name.
func (h *AuthHandler) UpdateMyProfile(w http.ResponseWriter, r *http.Request) {
	userID := ctxutil.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	user, err := h.users.Get(r.Context(), userID)
	if err != nil || user == nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.users.UpdateProfile(r.Context(), userID, req.Name, user.Email); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}

	log.Infof("fleet: user %s updated own profile", userID)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "updated"}})
}

// ChangeMyPassword handles PUT /api/v1/auth/me/password — changes the current user's password.
func (h *AuthHandler) ChangeMyPassword(w http.ResponseWriter, r *http.Request) {
	userID := ctxutil.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "current_password and new_password are required")
		return
	}

	user, err := h.users.Get(r.Context(), userID)
	if err != nil || user == nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := fleetauth.CheckPassword(user.Password, req.CurrentPassword); err != nil {
		writeError(w, http.StatusForbidden, "current password is incorrect")
		return
	}

	if err := fleetauth.ValidatePasswordPolicy(req.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	hashed, err := fleetauth.HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.users.UpdatePassword(r.Context(), userID, hashed); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change password")
		return
	}

	log.Infof("fleet: user %s changed own password", userID)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "password_changed"}})
}

// GetMyPermissions handles GET /api/v1/auth/me/permissions
// Returns the effective permission set for the current user (role + group permissions).
func (h *AuthHandler) GetMyPermissions(w http.ResponseWriter, r *http.Request) {
	role := ctxutil.RoleFromContext(r.Context())
	userID := ctxutil.UserIDFromContext(r.Context())

	if userID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Root gets all permissions
	if fleetauth.IsRoot(role) {
		writeJSON(w, http.StatusOK, fleet.Response{Data: fleetauth.AllPermissions()})
		return
	}

	// Start with role-based permissions
	permSet := make(map[string]bool)
	for _, p := range fleetauth.RolePermissions(role) {
		permSet[string(p)] = true
	}

	// Add group-based permissions
	if h.groups != nil {
		groupPerms, err := h.groups.GetEffectivePermissions(r.Context(), userID)
		if err == nil {
			for _, p := range groupPerms {
				permSet[p] = true
			}
		}
	}

	perms := make([]string, 0, len(permSet))
	for p := range permSet {
		perms = append(perms, p)
	}
	sort.Strings(perms)

	writeJSON(w, http.StatusOK, fleet.Response{Data: perms})
}

// UpdateAccountSettings handles PUT /api/v1/account/settings
func (h *AuthHandler) UpdateAccountSettings(w http.ResponseWriter, r *http.Request) {
	accountID := ctxutil.AccountIDFromContext(r.Context())
	if accountID == "" {
		writeError(w, http.StatusUnauthorized, "account context required")
		return
	}

	var req struct {
		Require2FA              bool     `json:"require_2fa"`
		TamperProtectionEnabled *bool    `json:"tamper_protection_enabled,omitempty"`
		EventLogEnabled         *bool    `json:"eventlog_enabled,omitempty"`
		IsolationWhitelist      []string `json:"isolation_whitelist,omitempty"`
		AllowedFileExtensions   []string `json:"allowed_file_extensions,omitempty"`
		LatestAgentVersion      string   `json:"latest_agent_version,omitempty"`
		LatestAgentMSIURL       string   `json:"latest_agent_msi_url,omitempty"`
		AutoUpdateAgents        *bool    `json:"auto_update_agents,omitempty"`
		AgentUpdateRepo         string   `json:"agent_update_repo,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.accounts.UpdateSettings(r.Context(), accountID, req.Require2FA, req.TamperProtectionEnabled, req.IsolationWhitelist, req.EventLogEnabled); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update settings")
		return
	}

	// Update file access compliance policy (empty array = permissive/no restrictions)
	if req.AllowedFileExtensions != nil {
		if err := h.accounts.UpdateFilePolicy(r.Context(), accountID, req.AllowedFileExtensions); err != nil {
			log.Warnf("fleet: failed to update file policy for account %s: %v", accountID, err)
		}
	}

	// Update agent update settings if provided
	if req.AutoUpdateAgents != nil || req.AgentUpdateRepo != "" {
		// Read current account to preserve values not being changed
		acct, _ := h.accounts.Get(r.Context(), accountID)
		ver := req.LatestAgentVersion
		msi := req.LatestAgentMSIURL
		autoUpdate := false
		if acct != nil {
			if ver == "" { ver = acct.LatestAgentVersion }
			if msi == "" { msi = acct.LatestAgentMSIURL }
			autoUpdate = acct.AutoUpdateAgents
		}
		if req.AutoUpdateAgents != nil {
			autoUpdate = *req.AutoUpdateAgents
		}
		if err := h.accounts.UpdateAgentVersion(r.Context(), accountID, ver, msi, autoUpdate); err != nil {
			log.Warnf("fleet: failed to update agent version for account %s: %v", accountID, err)
		}
		// Update repo if changed
		if req.AgentUpdateRepo != "" {
			h.accounts.UpdateAgentRepo(r.Context(), accountID, req.AgentUpdateRepo)
		}
	}

	log.Infof("fleet: account %s settings updated (2fa=%v, tamper=%v, eventlog=%v)", accountID, req.Require2FA, req.TamperProtectionEnabled, req.EventLogEnabled)

	// Propagate tamper protection state change to all agents across all orgs
	if req.TamperProtectionEnabled != nil {
		orgs, _ := h.orgs.ListByAccount(r.Context(), accountID)
		for _, org := range orgs {
			h.propagateTamperProtection(r.Context(), org.ID, *req.TamperProtectionEnabled)
		}
	}

	// Propagate event log policy when toggled at account level
	if req.EventLogEnabled != nil && h.eventlogPolicy != nil {
		orgs, _ := h.orgs.ListByAccount(r.Context(), accountID)
		for _, org := range orgs {
			h.propagateEventLogPolicy(r.Context(), org.ID, *req.EventLogEnabled)
		}
	}

	account, err := h.accounts.Get(r.Context(), accountID)
	if err != nil || account == nil {
		writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]bool{"require_2fa": req.Require2FA}})
		return
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{
		"require_2fa":                account.Require2FA,
		"tamper_protection_enabled":  account.TamperProtectionEnabled,
		"eventlog_enabled":           account.EventLogEnabled,
		"isolation_whitelist":        account.IsolationWhitelist,
		"allowed_file_extensions":    account.AllowedFileExtensions,
		"latest_agent_version":       account.LatestAgentVersion,
		"latest_agent_msi_url":       account.LatestAgentMSIURL,
		"auto_update_agents":         account.AutoUpdateAgents,
		"agent_update_repo":          account.AgentUpdateRepo,
	}})
}

// GetAccountSettings handles GET /api/v1/account/settings
func (h *AuthHandler) GetAccountSettings(w http.ResponseWriter, r *http.Request) {
	accountID := ctxutil.AccountIDFromContext(r.Context())
	if accountID == "" {
		writeError(w, http.StatusUnauthorized, "account context required")
		return
	}

	account, err := h.accounts.Get(r.Context(), accountID)
	if err != nil || account == nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	retDays := account.TelemetryRetentionDays
	if retDays <= 0 {
		retDays = 1
	}

	// Include org-level settings
	orgs, _ := h.orgs.ListByAccount(r.Context(), accountID)
	orgProtection := make([]map[string]interface{}, 0, len(orgs))
	for _, org := range orgs {
		orgRetDays := org.TelemetryRetentionDays
		if orgRetDays <= 0 {
			orgRetDays = retDays // fall back to account default
		}
		orgProtection = append(orgProtection, map[string]interface{}{
			"id":                        org.ID,
			"name":                      org.Name,
			"tamper_protection_enabled": org.TamperProtectionEnabled,
			"telemetry_retention_days":  orgRetDays,
		})
	}
	// nil = never configured (use defaults), empty slice = permissive (no restrictions)
	fileExts := account.AllowedFileExtensions
	if fileExts == nil {
		fileExts = fleet.DefaultAllowedFileExtensions
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{
		"require_2fa":                account.Require2FA,
		"account_name":               account.Name,
		"plan":                        account.Plan,
		"tamper_protection_enabled":   account.TamperProtectionEnabled,
		"eventlog_enabled":            account.EventLogEnabled,
		"telemetry_retention_days":    retDays,
		"isolation_whitelist":         account.IsolationWhitelist,
		"allowed_file_extensions":     fileExts,
		"org_protection":             orgProtection,
		"latest_agent_version":       account.LatestAgentVersion,
		"latest_agent_msi_url":       account.LatestAgentMSIURL,
		"auto_update_agents":         account.AutoUpdateAgents,
		"agent_update_repo":          account.AgentUpdateRepo,
	}})
}

// UpdateTelemetryRetention handles PUT /api/v1/account/telemetry-retention
func (h *AuthHandler) UpdateTelemetryRetention(w http.ResponseWriter, r *http.Request) {
	accountID := ctxutil.AccountIDFromContext(r.Context())
	if accountID == "" {
		writeError(w, http.StatusUnauthorized, "account context required")
		return
	}

	var req struct {
		Days int `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Days < 1 || req.Days > 365 {
		writeError(w, http.StatusBadRequest, "retention must be between 1 and 365 days")
		return
	}

	if err := h.accounts.UpdateRetention(r.Context(), accountID, req.Days); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update retention")
		return
	}

	// Apply to ClickHouse TTL for all orgs in this account
	if h.onRetentionChange != nil {
		orgs, _ := h.orgs.ListByAccount(r.Context(), accountID)
		for _, org := range orgs {
			if err := h.onRetentionChange(org.ID, req.Days); err != nil {
				log.Warnf("fleet: failed to update ClickHouse TTL for org %s: %v", org.ID, err)
			}
		}
	}

	log.Infof("fleet: telemetry retention updated to %d days for account %s", req.Days, accountID)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]int{"telemetry_retention_days": req.Days}})
}

// UpdateOrgTamperProtection handles PUT /api/v1/account/orgs/{org_id}/tamper-protection
func (h *AuthHandler) UpdateOrgTamperProtection(w http.ResponseWriter, r *http.Request) {
	accountID := ctxutil.AccountIDFromContext(r.Context())
	if accountID == "" {
		writeError(w, http.StatusUnauthorized, "account context required")
		return
	}

	// Extract org ID from path
	parts := strings.Split(r.URL.Path, "/orgs/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "org ID required")
		return
	}
	orgID := strings.TrimSuffix(parts[len(parts)-1], "/tamper-protection")
	orgID = strings.TrimSuffix(orgID, "/")

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	org, err := h.orgs.Get(r.Context(), orgID)
	if err != nil || org == nil || org.AccountID != accountID {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}

	if err := h.orgs.UpdateTamperProtection(r.Context(), orgID, req.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update org tamper protection")
		return
	}

	// Propagate tamper protection state change to all agents in this org
	h.propagateTamperProtection(r.Context(), orgID, req.Enabled)

	log.Infof("fleet: org %s tamper protection set to %v", orgID, req.Enabled)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]bool{"tamper_protection_enabled": req.Enabled}})
}

// UpdateOrgRetention handles PUT /api/v1/account/orgs/{org_id}/telemetry-retention
func (h *AuthHandler) UpdateOrgRetention(w http.ResponseWriter, r *http.Request) {
	accountID := ctxutil.AccountIDFromContext(r.Context())
	if accountID == "" {
		writeError(w, http.StatusUnauthorized, "account context required")
		return
	}

	parts := strings.Split(r.URL.Path, "/orgs/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "org ID required")
		return
	}
	orgID := strings.TrimSuffix(parts[len(parts)-1], "/telemetry-retention")
	orgID = strings.TrimSuffix(orgID, "/")

	var req struct {
		Days int `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Days < 0 || req.Days > 365 {
		writeError(w, http.StatusBadRequest, "retention must be between 0 and 365 days")
		return
	}

	org, err := h.orgs.Get(r.Context(), orgID)
	if err != nil || org == nil {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	// Root users can manage any org; regular users only their own account's orgs
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) && org.AccountID != accountID {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}

	if err := h.orgs.UpdateRetention(r.Context(), orgID, req.Days); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update org retention")
		return
	}

	// Apply to ClickHouse TTL for this org's table
	if h.onRetentionChange != nil {
		if err := h.onRetentionChange(orgID, req.Days); err != nil {
			log.Warnf("fleet: failed to update ClickHouse TTL for org %s: %v", orgID, err)
		}
	}

	log.Infof("fleet: org %s telemetry retention set to %d days", orgID, req.Days)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]int{"telemetry_retention_days": req.Days}})
}

// ListOrganizations handles GET /api/v1/account/organizations
func (h *AuthHandler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	accountID := ctxutil.AccountIDFromContext(r.Context())
	if accountID == "" {
		writeError(w, http.StatusUnauthorized, "account context required")
		return
	}

	orgs, err := h.orgs.ListByAccount(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: orgs})
}

// CreateOrganization handles POST /api/v1/account/organizations
func (h *AuthHandler) CreateOrganization(w http.ResponseWriter, r *http.Request) {
	accountID := ctxutil.AccountIDFromContext(r.Context())
	if accountID == "" {
		writeError(w, http.StatusUnauthorized, "account context required")
		return
	}

	var req fleet.CreateOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Slug == "" {
		req.Slug = slugify(req.Name)
	}

	org := &fleet.Organization{
		ID:        GenerateID(),
		AccountID: accountID,
		Name:      req.Name,
		Slug:      req.Slug,
	}
	if err := h.orgs.Create(r.Context(), org); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create organization")
		return
	}

	// Seed default macros and official rules for the new org
	go h.seedOrgDefaults(context.Background(), org.ID)

	// Grant current user access
	userID := ctxutil.UserIDFromContext(r.Context())
	if userID != "" {
		h.users.AddOrgAccess(r.Context(), userID, org.ID, "admin")
	}

	writeJSON(w, http.StatusCreated, fleet.Response{Data: org})
}

// DeleteOrganization handles DELETE /api/v1/account/organizations/{id}
func (h *AuthHandler) DeleteOrganization(w http.ResponseWriter, r *http.Request) {
	accountID := ctxutil.AccountIDFromContext(r.Context())
	if accountID == "" {
		writeError(w, http.StatusUnauthorized, "account context required")
		return
	}

	// Extract org ID from the URL path
	orgID := strings.TrimPrefix(r.URL.Path, "/api/v1/account/organizations/")
	if orgID == "" || orgID == r.URL.Path {
		writeError(w, http.StatusBadRequest, "organization ID required")
		return
	}

	// Verify the org belongs to this account
	org, err := h.orgs.Get(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if org == nil || org.AccountID != accountID {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}

	if err := h.orgs.Delete(r.Context(), orgID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete organization")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// CreateAPIKey handles POST /api/v1/auth/api-keys
func (h *AuthHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	userID := ctxutil.UserIDFromContext(r.Context())
	accountID := ctxutil.AccountIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Generate a random API key: fib_<32 random hex chars>
	plainKey := "fib_" + generateRandomHex(32)
	keyHash := hashAPIKey(plainKey)

	key := &fleet.APIKey{
		ID:        GenerateID(),
		UserID:    userID,
		AccountID: accountID,
		Name:      req.Name,
		KeyPrefix: plainKey[:12], // "fib_" + first 8 hex chars
		KeyHash:   keyHash,
		PlainKey:  plainKey,
		CreatedAt: time.Now().UTC(),
	}

	if err := h.apiKeys.Create(r.Context(), key); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create API key")
		return
	}

	log.Infof("fleet: API key created by user %s: %s (%s)", userID, key.Name, key.KeyPrefix)
	writeJSON(w, http.StatusCreated, fleet.Response{Data: key})
}

// ListAPIKeys handles GET /api/v1/auth/api-keys
func (h *AuthHandler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	userID := ctxutil.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	keys, err := h.apiKeys.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: keys})
}

// DeleteAPIKey handles DELETE /api/v1/auth/api-keys/{id}
func (h *AuthHandler) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	userID := ctxutil.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	parts := strings.Split(r.URL.Path, "/api-keys/")
	if len(parts) < 2 || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "key ID required")
		return
	}
	keyID := strings.TrimSuffix(parts[1], "/")
	if err := h.apiKeys.Delete(r.Context(), keyID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete key")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// generateRandomHex returns n random bytes encoded as a hex string.
func generateRandomHex(n int) string {
	bytes := make([]byte, n)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// hashAPIKey returns the SHA-256 hash of an API key as a hex string.
func hashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// slugify converts a name to a URL-friendly slug.
// seedDefaultGroupsForSignup creates default groups for a new account on signup.
// Returns the Administrators group ID so the signup user can be added to it.
func (h *AuthHandler) seedDefaultGroupsForSignup(ctx context.Context, accountID string) string {
	if h.groups == nil {
		return ""
	}

	adminGroupID := ""
	defaults := []struct {
		name        string
		description string
		permissions []string
		isAdmin     bool
	}{
		{
			name: "Administrators", description: "Full access to all features within this account", isAdmin: true,
			permissions: []string{
				"page:overview", "page:agents", "page:detections", "page:events",
				"page:rules", "page:macros", "page:audit", "page:management", "page:process_tree",
				"agents:view", "agents:manage", "agents:delete",
				"agents:view_events", "agents:view_detections", "agents:view_processes",
				"agents:view_network", "agents:view_services", "agents:view_drivers",
				"agents:view_autoruns", "agents:view_software", "agents:view_users",
				"agents:view_files", "agents:view_registry", "agents:view_eventlog",
				"agents:view_terminal", "agents:view_captures", "agents:view_history",
				"detections:view", "detections:manage", "events:view",
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
			name: "Analysts", description: "Investigation and rule management",
			permissions: []string{
				"page:overview", "page:agents", "page:detections", "page:events",
				"page:rules", "page:macros", "page:audit", "page:process_tree",
				"agents:view",
				"agents:view_events", "agents:view_detections", "agents:view_processes",
				"agents:view_network", "agents:view_services", "agents:view_drivers",
				"agents:view_autoruns", "agents:view_software", "agents:view_users",
				"agents:view_files", "agents:view_registry", "agents:view_eventlog",
				"agents:view_captures", "agents:view_history",
				"detections:view", "detections:manage", "events:view",
				"rules:view", "rules:manage", "commands:view",
				"captures:view", "telemetry:view", "enrollment:view",
				"github_sync:view", "audit:view",
			},
		},
		{
			name: "Read Only", description: "View-only access",
			permissions: []string{
				"page:overview", "page:agents", "page:detections", "page:events",
				"page:rules", "page:audit", "page:process_tree",
				"agents:view",
				"agents:view_events", "agents:view_detections", "agents:view_processes",
				"agents:view_network", "agents:view_services", "agents:view_drivers",
				"agents:view_autoruns", "agents:view_software", "agents:view_users",
				"agents:view_files", "agents:view_registry", "agents:view_eventlog",
				"agents:view_captures", "agents:view_history",
				"detections:view", "events:view", "rules:view",
				"commands:view", "captures:view", "telemetry:view",
			},
		},
	}

	for _, d := range defaults {
		id := GenerateID()
		now := time.Now().UTC()
		g := &fleet.UserGroup{
			ID: id, AccountID: accountID, Name: d.name, Description: d.description,
			Permissions: d.permissions, CreatedAt: now, UpdatedAt: now,
		}
		if err := h.groups.Create(ctx, g); err != nil {
			log.Warnf("fleet: signup: failed to seed group %q: %v", d.name, err)
			continue
		}
		if d.isAdmin {
			adminGroupID = id
		}
	}
	log.Infof("fleet: seeded default groups for signup account %s", accountID)
	return adminGroupID
}

// seedOrgDefaults loads the default Fibratus macros and official detection rules
// into a newly created organization. Called during signup and org creation.
func (h *AuthHandler) seedOrgDefaults(ctx context.Context, orgID string) {
	h.seedOrgMacros(ctx, orgID)
	h.seedOrgRules(ctx, orgID)
}

func (h *AuthHandler) seedOrgMacros(ctx context.Context, orgID string) {
	if h.macros == nil {
		return
	}
	pStore, ok := h.macros.(*postgres.MacroStore)
	if !ok {
		return
	}
	for _, path := range []string{
		"rules/macros/macros.yml",
		"/opt/fibratus-fleet/src/rules/macros/macros.yml",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		n, err := pStore.ImportFromYAML(ctx, orgID, data)
		if err != nil {
			log.Warnf("fleet: failed to seed macros for org %s: %v", orgID, err)
			return
		}
		log.Infof("fleet: seeded %d macros for org %s", n, orgID)
		return
	}
}

func (h *AuthHandler) seedOrgRules(ctx context.Context, orgID string) {
	if h.rules == nil {
		return
	}
	// Find rules directory
	var rulesDir string
	for _, dir := range []string{
		"rules",
		"/opt/fibratus-fleet/src/rules",
	} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			rulesDir = dir
			break
		}
	}
	if rulesDir == "" {
		return
	}

	// Load macros for validation
	var macros map[string]interface{}
	if h.macros != nil {
		macros = make(map[string]interface{})
	}

	var count int
	filepath.Walk(rulesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		// Skip macros directory
		if strings.Contains(path, "macros/") || strings.Contains(path, "macros\\") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		// Parse as Fibratus rule YAML
		var ruleData struct {
			Name        string            `yaml:"name"`
			ID          string            `yaml:"id"`
			Version     string            `yaml:"version"`
			Description string            `yaml:"description"`
			Condition   string            `yaml:"condition"`
			Output      string            `yaml:"output"`
			Severity    string            `yaml:"severity"`
			Labels      map[string]string `yaml:"labels"`
			Tags        []string          `yaml:"tags"`
			References  []string          `yaml:"references"`
		}
		if err := yaml.Unmarshal(data, &ruleData); err != nil || ruleData.Name == "" {
			return nil
		}
		rule := &fleet.Rule{
			ID:          ruleData.ID,
			OrgID:       orgID,
			Name:        ruleData.Name,
			Version:     ruleData.Version,
			Description: ruleData.Description,
			Condition:   ruleData.Condition,
			Output:      ruleData.Output,
			Severity:    ruleData.Severity,
			Labels:      ruleData.Labels,
			Tags:        ruleData.Tags,
			References:  ruleData.References,
			RawYAML:     string(data),
			Enabled:     true,
			Source:       "official",
		}
		if rule.ID == "" {
			rule.ID = GenerateID()
		}
		if rule.Version == "" {
			rule.Version = "1.0.0"
		}
		if rule.Severity == "" {
			rule.Severity = "medium"
		}
		// Validate condition
		_ = macros // validator uses DB macros via the handler
		condResult := validator.ValidateCondition(rule.Condition)
		if condResult.Valid {
			rule.ValidationStatus = "valid"
			rule.ValidationErrors = json.RawMessage(`[]`)
		} else {
			rule.ValidationStatus = "invalid"
			errJSON, _ := json.Marshal(condResult.Errors)
			rule.ValidationErrors = errJSON
			rule.Enabled = false
		}
		if err := h.rules.Create(ctx, rule); err != nil {
			// Likely duplicate — skip silently
			return nil
		}
		count++
		return nil
	})
	if count > 0 {
		log.Infof("fleet: seeded %d official rules for org %s", count, orgID)
	}
}

func slugify(name string) string {
	slug := strings.ToLower(name)
	slug = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		if r == ' ' || r == '-' {
			return '-'
		}
		return -1
	}, slug)
	// Collapse consecutive hyphens
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	return slug
}
