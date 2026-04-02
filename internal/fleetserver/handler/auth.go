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
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
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
	accountID := generateID()
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
	orgID := generateID()
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
	userID := generateID()
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

	if err := fleetauth.CheckPassword(user.Password, req.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := fleetauth.GenerateJWT(h.jwtSecret, user.ID, user.AccountID, user.Role)
	if err != nil {
		log.Errorf("fleet: login generate token error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	log.Infof("fleet: user logged in: %s (%s)", user.Email, user.ID)

	// Clear password before returning user in response
	user.Password = ""
	resp := fleet.LoginResponse{
		Token: token,
		User:  *user,
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: resp})
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
		ID:        generateID(),
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
