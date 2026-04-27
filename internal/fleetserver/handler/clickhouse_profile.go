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

	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// ClickHouseProfileDTO is the wire format for one connection profile. Password
// is write-only (never returned on reads — operators see "configured: true"
// instead). Active is true for the live profile.
type ClickHouseProfileDTO struct {
	Name                string `json:"name"`
	Active              bool   `json:"active"`
	Enabled             bool   `json:"enabled"`
	Host                string `json:"host"`
	Port                int    `json:"port"`
	Database            string `json:"database"`
	User                string `json:"user"`
	HasPassword         bool   `json:"has_password"`
	Password            string `json:"password,omitempty"` // write only
	Secure              bool   `json:"secure"`
	SkipVerify          bool   `json:"skip_verify"`
	DialTimeoutSecs     int    `json:"dial_timeout_secs"`
	MaxOpenConns        int    `json:"max_open_conns"`
	MaxIdleConns        int    `json:"max_idle_conns"`
	ConnMaxLifetimeSecs int    `json:"conn_max_lifetime_secs"`
	CloudOrgID          string `json:"cloud_org_id,omitempty"`
	CloudServiceID      string `json:"cloud_service_id,omitempty"`
}

// ProfileTestResult mirrors CHTestResult for the per-profile test endpoint —
// kept as a separate type so future per-profile diagnostics can grow without
// touching the legacy CH test endpoint.
type ProfileTestResult struct {
	OK        bool   `json:"ok"`
	Version   string `json:"version,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// CHProfileHandlerDeps wires the dashboard-facing handler to the server's
// profile store + pipeline manager via closures so the handler stays free of
// store and pipeline imports.
type CHProfileHandlerDeps struct {
	List     func() ([]ClickHouseProfileDTO, error)
	Get      func(name string) (*ClickHouseProfileDTO, error)
	Upsert   func(req ClickHouseProfileDTO) error
	Activate func(name string) error
	Test     func(name string) ProfileTestResult
}

// CHProfileHandler exposes /admin/clickhouse-profiles routes.
type CHProfileHandler struct{ d CHProfileHandlerDeps }

func NewCHProfileHandler(d CHProfileHandlerDeps) *CHProfileHandler {
	return &CHProfileHandler{d: d}
}

// List returns both profiles + which one is active.
// GET /api/v1/admin/clickhouse-profiles
func (h *CHProfileHandler) List(w http.ResponseWriter, r *http.Request) {
	if !rootGuard(w, r) {
		return
	}
	out, err := h.d.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: out})
}

// Get returns one profile.
// GET /api/v1/admin/clickhouse-profiles/{name}
func (h *CHProfileHandler) Get(w http.ResponseWriter, r *http.Request) {
	if !rootGuard(w, r) {
		return
	}
	name := profileNameFromPath(r.URL.Path, "")
	if name == "" {
		writeError(w, http.StatusBadRequest, "profile name is required")
		return
	}
	p, err := h.d.Get(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: p})
}

// Upsert updates a profile in place. Password is optional — leaving it blank
// keeps the existing password.
// PUT /api/v1/admin/clickhouse-profiles/{name}
func (h *CHProfileHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	if !rootGuard(w, r) {
		return
	}
	name := profileNameFromPath(r.URL.Path, "")
	if name == "" {
		writeError(w, http.StatusBadRequest, "profile name is required")
		return
	}
	var req ClickHouseProfileDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = name
	if err := h.d.Upsert(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	log.Infof("fleet: clickhouse profile %s saved", name)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{"saved": true}})
}

// Activate hot-swaps the named profile in. The ClickHouse pipeline drains the
// old buffer, opens a fresh connection, swaps atomically, then closes the
// old connection — all without restarting the process.
// POST /api/v1/admin/clickhouse-profiles/{name}/activate
func (h *CHProfileHandler) Activate(w http.ResponseWriter, r *http.Request) {
	if !rootGuard(w, r) {
		return
	}
	name := profileNameFromPath(r.URL.Path, "/activate")
	if name == "" {
		writeError(w, http.StatusBadRequest, "profile name is required")
		return
	}
	if err := h.d.Activate(name); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	log.Infof("fleet: clickhouse profile %s activated (hot-swap)", name)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{"active": name}})
}

// Test attempts a real connection with the profile's stored config.
// POST /api/v1/admin/clickhouse-profiles/{name}/test
func (h *CHProfileHandler) Test(w http.ResponseWriter, r *http.Request) {
	if !rootGuard(w, r) {
		return
	}
	name := profileNameFromPath(r.URL.Path, "/test")
	if name == "" {
		writeError(w, http.StatusBadRequest, "profile name is required")
		return
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: h.d.Test(name)})
}

// profileNameFromPath extracts the {name} segment from
// /api/v1/admin/clickhouse-profiles/{name}[<suffix>].
func profileNameFromPath(path, suffix string) string {
	const prefix = "/api/v1/admin/clickhouse-profiles/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	if suffix != "" {
		rest = strings.TrimSuffix(rest, suffix)
	}
	// Strip any trailing slash and reject sub-paths.
	rest = strings.TrimSuffix(rest, "/")
	if strings.Contains(rest, "/") {
		return ""
	}
	return rest
}
