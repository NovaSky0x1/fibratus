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

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/fleetauth"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// TOTPHandler handles TOTP 2FA setup and management.
type TOTPHandler struct {
	users store.UserStore
}

// NewTOTPHandler creates a new TOTP handler.
func NewTOTPHandler(users store.UserStore) *TOTPHandler {
	return &TOTPHandler{users: users}
}

// Setup handles POST /api/v1/auth/totp/setup — generates a TOTP secret.
func (h *TOTPHandler) Setup(w http.ResponseWriter, r *http.Request) {
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

	if user.TOTPEnabled {
		writeError(w, http.StatusBadRequest, "2FA is already enabled")
		return
	}

	secret, err := fleetauth.GenerateTOTPSecret()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate secret")
		return
	}

	uri := fleetauth.TOTPProvisioningURI(secret, user.Email, "FibratusFleet")

	// Store the secret but don't enable yet — user must verify first
	if err := h.users.SetTOTP(r.Context(), userID, secret, false, ""); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{
		"secret": secret,
		"uri":    uri,
	}})
}

// Verify handles POST /api/v1/auth/totp/verify — confirms TOTP setup with a code.
func (h *TOTPHandler) Verify(w http.ResponseWriter, r *http.Request) {
	userID := ctxutil.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	user, err := h.users.Get(r.Context(), userID)
	if err != nil || user == nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if user.TOTPSecret == "" {
		writeError(w, http.StatusBadRequest, "call /totp/setup first")
		return
	}

	if !fleetauth.ValidateTOTP(user.TOTPSecret, req.Code) {
		writeError(w, http.StatusBadRequest, "invalid code — check your authenticator app")
		return
	}

	// Generate recovery codes
	codes, err := fleetauth.GenerateRecoveryCodes(10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate recovery codes")
		return
	}

	if err := h.users.SetTOTP(r.Context(), userID, user.TOTPSecret, true, strings.Join(codes, ",")); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	log.Infof("fleet: 2FA enabled for user %s", user.Email)

	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{
		"enabled":        true,
		"recovery_codes": codes,
	}})
}

// Disable handles POST /api/v1/auth/totp/disable — disables 2FA.
func (h *TOTPHandler) Disable(w http.ResponseWriter, r *http.Request) {
	userID := ctxutil.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required to disable 2FA")
		return
	}

	user, err := h.users.Get(r.Context(), userID)
	if err != nil || user == nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := fleetauth.CheckPassword(user.Password, req.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}

	if err := h.users.SetTOTP(r.Context(), userID, "", false, ""); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	log.Infof("fleet: 2FA disabled for user %s", user.Email)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "disabled"}})
}

// Status handles GET /api/v1/auth/totp/status — returns 2FA status.
func (h *TOTPHandler) Status(w http.ResponseWriter, r *http.Request) {
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

	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]bool{
		"enabled": user.TOTPEnabled,
	}})
}
