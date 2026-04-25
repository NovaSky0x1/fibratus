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

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/fleetauth"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// ClickHouseConfigDTO is the wire format for ClickHouse connection config used
// by the management UI. Mode is derived (not stored on disk): "local" when
// Secure is false, "cloud" otherwise — the dashboard uses it to render the
// right form. Password is omitted on read; on write, an empty string means
// "do not change the existing password".
type ClickHouseConfigDTO struct {
	Mode            string `json:"mode"`
	Enabled         bool   `json:"enabled"`
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Database        string `json:"database"`
	User            string `json:"user"`
	Password        string `json:"password,omitempty"`
	Secure          bool   `json:"secure"`
	SkipVerify      bool   `json:"skip_verify"`
	DialTimeoutSecs int    `json:"dial_timeout_secs"`
	MaxOpenConns    int    `json:"max_open_conns"`
	MaxIdleConns    int    `json:"max_idle_conns"`
	ConnMaxLifetime int    `json:"conn_max_lifetime_secs"`
}

// CHTestResult is returned from the test-connection endpoint.
type CHTestResult struct {
	OK        bool   `json:"ok"`
	Version   string `json:"version,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// CHConfigHandler exposes admin endpoints for inspecting, testing, and saving
// the ClickHouse connection configuration. The fleetserver package wires the
// closures so this handler does not need to import the server type (avoids
// import cycles).
type CHConfigHandler struct {
	get  func() ClickHouseConfigDTO
	save func(ClickHouseConfigDTO) error
	test func(ClickHouseConfigDTO) CHTestResult
}

// NewCHConfigHandler constructs the handler with the supplied closures.
func NewCHConfigHandler(
	get func() ClickHouseConfigDTO,
	save func(ClickHouseConfigDTO) error,
	test func(ClickHouseConfigDTO) CHTestResult,
) *CHConfigHandler {
	return &CHConfigHandler{get: get, save: save, test: test}
}

// Get returns the current ClickHouse config (password redacted).
// GET /api/v1/admin/db/clickhouse/config
func (h *CHConfigHandler) Get(w http.ResponseWriter, r *http.Request) {
	if !rootGuard(w, r) {
		return
	}
	cfg := h.get()
	cfg.Password = "" // never expose the stored password
	writeJSON(w, http.StatusOK, fleet.Response{Data: cfg})
}

// Test attempts a connection with the supplied config without persisting it.
// POST /api/v1/admin/db/clickhouse/test
func (h *CHConfigHandler) Test(w http.ResponseWriter, r *http.Request) {
	if !rootGuard(w, r) {
		return
	}
	var req ClickHouseConfigDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// If password omitted on the wire, fall back to the stored one so the user
	// can re-test an existing connection without re-entering credentials.
	if req.Password == "" {
		req.Password = h.get().Password
	}
	result := h.test(req)
	if !result.OK {
		log.Infof("fleet: ClickHouse test connection failed (host=%s:%d secure=%v): %s",
			req.Host, req.Port, req.Secure, result.Error)
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: result})
}

// Save persists the supplied ClickHouse config to disk. The change requires a
// server restart to take effect — the response includes restart_required:true
// so the dashboard can surface that to the operator.
// PUT /api/v1/admin/db/clickhouse/config
func (h *CHConfigHandler) Save(w http.ResponseWriter, r *http.Request) {
	if !rootGuard(w, r) {
		return
	}
	var req ClickHouseConfigDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Password == "" {
		req.Password = h.get().Password // preserve existing password
	}
	if err := h.save(req); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Infof("fleet: ClickHouse config updated by root (host=%s:%d secure=%v) — restart required",
		req.Host, req.Port, req.Secure)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{
		"saved":            true,
		"restart_required": true,
	}})
}

func rootGuard(w http.ResponseWriter, r *http.Request) bool {
	role := ctxutil.RoleFromContext(r.Context())
	if !fleetauth.IsRoot(role) {
		writeError(w, http.StatusForbidden, "root access required")
		return false
	}
	return true
}
