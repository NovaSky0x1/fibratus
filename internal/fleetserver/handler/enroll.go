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
	"time"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ca"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// EnrollHandler handles agent enrollment requests.
type EnrollHandler struct {
	tokens store.EnrollmentTokenStore
	agents store.AgentStore
	ca     *ca.Manager
}

// NewEnrollHandler creates a new enrollment handler.
func NewEnrollHandler(tokens store.EnrollmentTokenStore, agents store.AgentStore, ca *ca.Manager) *EnrollHandler {
	return &EnrollHandler{tokens: tokens, agents: agents, ca: ca}
}

// Enroll handles POST /api/v1/enroll
// This endpoint requires NO authentication — the enrollment token IS the credential.
// Flow:
//  1. Validate the enrollment token (exists, not expired, uses remaining)
//  2. Get or create the org's CA
//  3. Sign the agent's CSR
//  4. Create the agent record
//  5. Increment token usage
//  6. Return signed cert + CA cert + agent ID
func (h *EnrollHandler) Enroll(w http.ResponseWriter, r *http.Request) {
	var req fleet.EnrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Token == "" {
		writeError(w, http.StatusBadRequest, "enrollment token is required")
		return
	}
	if req.Hostname == "" {
		writeError(w, http.StatusBadRequest, "hostname is required")
		return
	}
	if req.CSR == "" {
		writeError(w, http.StatusBadRequest, "CSR is required")
		return
	}

	ctx := r.Context()

	// 1. Validate token
	token, err := h.tokens.Get(ctx, req.Token)
	if err != nil {
		log.Errorf("fleet: enroll token lookup error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if token == nil {
		writeError(w, http.StatusUnauthorized, "invalid enrollment token")
		return
	}
	if !token.IsValid() {
		writeError(w, http.StatusUnauthorized, "enrollment token expired or exhausted")
		return
	}

	// 2. Get or create org CA
	orgCA, err := h.ca.GetOrCreateCA(ctx, token.OrgID, token.AccountID)
	if err != nil {
		log.Errorf("fleet: enroll CA error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to initialize org CA")
		return
	}

	// 3. Create agent record
	agentID := generateID()
	now := time.Now().UTC()

	agent := &fleet.Agent{
		ID:            agentID,
		OrgID:         token.OrgID,
		Hostname:      req.Hostname,
		OSVersion:     req.OSVersion,
		EngineVersion: req.EngineVersion,
		Status:        fleet.AgentOnline,
		LastHeartbeat: now,
		RegisteredAt:  now,
	}

	if err := h.agents.Create(ctx, agent); err != nil {
		log.Errorf("fleet: enroll agent create error: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create agent")
		return
	}

	// 4. Sign CSR
	signedCert, err := h.ca.SignCSR(orgCA, []byte(req.CSR), agentID, token.OrgID, token.AccountID)
	if err != nil {
		log.Errorf("fleet: enroll sign CSR error: %v", err)
		writeError(w, http.StatusBadRequest, "failed to sign CSR: "+err.Error())
		return
	}

	// 5. Increment token usage
	if err := h.tokens.IncrementUses(ctx, token.ID); err != nil {
		log.Warnf("fleet: enroll token increment error: %v", err)
	}

	log.WithFields(log.Fields{
		"agent":    agentID,
		"hostname": req.Hostname,
		"org":      token.OrgID,
		"token":    token.ID[:12] + "...",
	}).Info("fleet: agent enrolled")

	resp := fleet.EnrollResponse{
		AgentID:    agentID,
		OrgID:      token.OrgID,
		SignedCert: string(signedCert),
		CACert:     string(orgCA.CertPEM),
	}

	writeJSON(w, http.StatusCreated, fleet.Response{Data: resp})
}
