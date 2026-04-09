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
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/fleetauth"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// AuthHandler handles authentication API requests.
type AuthHandler struct {
	accounts  store.AccountStore
	orgs      store.OrgStore
	users     store.UserStore
	jwtSecret string
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(accounts store.AccountStore, orgs store.OrgStore, users store.UserStore, jwtSecret string) *AuthHandler {
	return &AuthHandler{
		accounts:  accounts,
		orgs:      orgs,
		users:     users,
		jwtSecret: jwtSecret,
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

	// Create user
	userID := GenerateID()
	user := &fleet.User{
		ID:        userID,
		Email:     req.Email,
		Name:      req.Name,
		Password:  hashedPassword,
		AccountID: accountID,
		Role:      "admin",
		CreatedAt: now,
	}
	if err := h.users.Create(r.Context(), user); err != nil {
		log.Errorf("fleet: signup create user error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Add user org access
	if err := h.users.AddOrgAccess(r.Context(), userID, orgID, "admin"); err != nil {
		log.Errorf("fleet: signup add org access error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Generate JWT
	token, err := fleetauth.GenerateJWT(h.jwtSecret, userID, accountID, "admin")
	if err != nil {
		log.Errorf("fleet: signup generate token error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	log.Infof("fleet: account created: %s (%s)", account.Name, accountID)

	resp := fleet.SignupResponse{
		AccountID: accountID,
		OrgID:     orgID,
		UserID:    userID,
		Token:     token,
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
		IsolationWhitelist      []string `json:"isolation_whitelist,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.accounts.UpdateSettings(r.Context(), accountID, req.Require2FA, req.TamperProtectionEnabled, req.IsolationWhitelist); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update settings")
		return
	}

	log.Infof("fleet: account %s settings updated (2fa=%v, tamper=%v)", accountID, req.Require2FA, req.TamperProtectionEnabled)

	account, err := h.accounts.Get(r.Context(), accountID)
	if err != nil || account == nil {
		writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]bool{"require_2fa": req.Require2FA}})
		return
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{
		"require_2fa":                account.Require2FA,
		"tamper_protection_enabled":  account.TamperProtectionEnabled,
		"isolation_whitelist":        account.IsolationWhitelist,
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

	// Include org-level tamper protection status
	orgs, _ := h.orgs.ListByAccount(r.Context(), accountID)
	orgProtection := make([]map[string]interface{}, 0, len(orgs))
	for _, org := range orgs {
		orgProtection = append(orgProtection, map[string]interface{}{
			"id":                        org.ID,
			"name":                      org.Name,
			"tamper_protection_enabled": org.TamperProtectionEnabled,
		})
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{
		"require_2fa":                account.Require2FA,
		"account_name":               account.Name,
		"plan":                        account.Plan,
		"tamper_protection_enabled":   account.TamperProtectionEnabled,
		"isolation_whitelist":         account.IsolationWhitelist,
		"org_protection":             orgProtection,
	}})
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

	log.Infof("fleet: org %s tamper protection set to %v", orgID, req.Enabled)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]bool{"tamper_protection_enabled": req.Enabled}})
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

// slugify converts a name to a URL-friendly slug.
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
