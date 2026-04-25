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

	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// CloudCredentialsStatusDTO is the wire format for the credentials-status
// endpoint. Secret values are never returned — only whether each is set, plus
// the (non-secret) Key ID for human display.
type CloudCredentialsStatusDTO struct {
	Configured bool   `json:"configured"`
	KeyID      string `json:"key_id,omitempty"`
	OrgID      string `json:"organization_id,omitempty"`
}

// CloudCredentialsRequest is the body for setting Cloud API credentials.
type CloudCredentialsRequest struct {
	KeyID     string `json:"key_id"`
	KeySecret string `json:"key_secret"`
	OrgID     string `json:"organization_id,omitempty"`
}

// CloudConnectRequest binds an existing or newly-created service to the fleet
// server's ClickHouse config. ServicePassword is required for existing services
// (since the API does not expose it on reads); for newly-created services the
// caller can use POST /services/new which returns the password as part of
// creation.
type CloudConnectRequest struct {
	OrgID           string `json:"organization_id"`
	ServiceID       string `json:"service_id"`
	Database        string `json:"database"`
	User            string `json:"user"`
	ServicePassword string `json:"service_password"`
}

// CloudCreateServiceRequest is forwarded to the Cloud API after the handler
// substitutes the stored credentials. It mirrors clickhousecloud.CreateServiceRequest
// but lives here to avoid a hard import cycle on the typed client from this
// package's tests.
type CloudCreateServiceRequest struct {
	OrgID              string `json:"organization_id"`
	Name               string `json:"name"`
	Provider           string `json:"provider"`
	Region             string `json:"region"`
	Tier               string `json:"tier"`
	MinReplicaMemoryGB int    `json:"min_replica_memory_gb,omitempty"`
	MaxReplicaMemoryGB int    `json:"max_replica_memory_gb,omitempty"`
	Database           string `json:"database"`
	User               string `json:"user"`
}

// CloudHandler exposes admin endpoints for ClickHouse Cloud setup. The
// fleetserver package wires the closures so this handler stays free of an
// import on the typed Cloud client and the secret store.
type CloudHandler struct {
	getStatus         func() CloudCredentialsStatusDTO
	saveCredentials   func(CloudCredentialsRequest) error
	deleteCredentials func() error
	listOrgs          func(r *http.Request) (interface{}, error)
	listServices      func(r *http.Request, orgID string) (interface{}, error)
	createService     func(req CloudCreateServiceRequest) (interface{}, error)
	connectService    func(req CloudConnectRequest) error
	resetPassword     func(orgID, serviceID string) error
	restart           func()
}

// CloudHandlerDeps bundles all closures the CloudHandler needs.
type CloudHandlerDeps struct {
	GetStatus         func() CloudCredentialsStatusDTO
	SaveCredentials   func(CloudCredentialsRequest) error
	DeleteCredentials func() error
	ListOrgs          func(r *http.Request) (interface{}, error)
	ListServices      func(r *http.Request, orgID string) (interface{}, error)
	CreateService     func(req CloudCreateServiceRequest) (interface{}, error)
	ConnectService    func(req CloudConnectRequest) error
	ResetPassword     func(orgID, serviceID string) error
	Restart           func()
}

// NewCloudHandler constructs the handler.
func NewCloudHandler(d CloudHandlerDeps) *CloudHandler {
	return &CloudHandler{
		getStatus:         d.GetStatus,
		saveCredentials:   d.SaveCredentials,
		deleteCredentials: d.DeleteCredentials,
		listOrgs:          d.ListOrgs,
		listServices:      d.ListServices,
		createService:     d.CreateService,
		connectService:    d.ConnectService,
		resetPassword:     d.ResetPassword,
		restart:           d.Restart,
	}
}

// GetCredentialsStatus reports whether Cloud API credentials are stored.
// GET /api/v1/admin/clickhouse-cloud/credentials
func (h *CloudHandler) GetCredentialsStatus(w http.ResponseWriter, r *http.Request) {
	if !rootGuard(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: h.getStatus()})
}

// PutCredentials stores the supplied Key ID + Secret in the encrypted store.
// PUT /api/v1/admin/clickhouse-cloud/credentials
func (h *CloudHandler) PutCredentials(w http.ResponseWriter, r *http.Request) {
	if !rootGuard(w, r) {
		return
	}
	var req CloudCredentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.KeyID == "" || req.KeySecret == "" {
		writeError(w, http.StatusBadRequest, "key_id and key_secret are required")
		return
	}
	if err := h.saveCredentials(req); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Infof("fleet: ClickHouse Cloud credentials stored (key_id=%s)", req.KeyID)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{"saved": true}})
}

// DeleteCredentials removes the stored Cloud API credentials.
// DELETE /api/v1/admin/clickhouse-cloud/credentials
func (h *CloudHandler) DeleteCredentials(w http.ResponseWriter, r *http.Request) {
	if !rootGuard(w, r) {
		return
	}
	if err := h.deleteCredentials(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Info("fleet: ClickHouse Cloud credentials deleted")
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{"deleted": true}})
}

// ListOrganizations proxies GET /organizations from the Cloud API.
// GET /api/v1/admin/clickhouse-cloud/organizations
func (h *CloudHandler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	if !rootGuard(w, r) {
		return
	}
	out, err := h.listOrgs(r)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: out})
}

// ListServices proxies GET /organizations/{id}/services. The orgID comes from
// the query string (?organization_id=) so the path stays static.
// GET /api/v1/admin/clickhouse-cloud/services?organization_id=...
func (h *CloudHandler) ListServices(w http.ResponseWriter, r *http.Request) {
	if !rootGuard(w, r) {
		return
	}
	orgID := r.URL.Query().Get("organization_id")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "organization_id query parameter is required")
		return
	}
	out, err := h.listServices(r, orgID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: out})
}

// CreateService provisions a new Cloud service and persists the resulting
// connection details + generated password to the local config + secret store.
// POST /api/v1/admin/clickhouse-cloud/services
func (h *CloudHandler) CreateService(w http.ResponseWriter, r *http.Request) {
	if !rootGuard(w, r) {
		return
	}
	var req CloudCreateServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.OrgID == "" || req.Name == "" || req.Region == "" {
		writeError(w, http.StatusBadRequest, "organization_id, name, and region are required")
		return
	}
	out, err := h.createService(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	log.Infof("fleet: ClickHouse Cloud service created (org=%s name=%s)", req.OrgID, req.Name)
	writeJSON(w, http.StatusOK, fleet.Response{Data: out})
}

// ConnectService binds an existing Cloud service to the local ClickHouse
// config and stores the supplied service password in the secret store.
// POST /api/v1/admin/clickhouse-cloud/connect
func (h *CloudHandler) ConnectService(w http.ResponseWriter, r *http.Request) {
	if !rootGuard(w, r) {
		return
	}
	var req CloudConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.OrgID == "" || req.ServiceID == "" || req.ServicePassword == "" {
		writeError(w, http.StatusBadRequest, "organization_id, service_id, and service_password are required")
		return
	}
	if err := h.connectService(req); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	log.Infof("fleet: bound ClickHouse Cloud service %s to local config", req.ServiceID)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{
		"bound":            true,
		"restart_required": true,
	}})
}

// ResetPassword resets a service's password and stores the new one.
// POST /api/v1/admin/clickhouse-cloud/services/reset-password?organization_id=...&service_id=...
func (h *CloudHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	if !rootGuard(w, r) {
		return
	}
	orgID := r.URL.Query().Get("organization_id")
	serviceID := r.URL.Query().Get("service_id")
	if orgID == "" || serviceID == "" {
		writeError(w, http.StatusBadRequest, "organization_id and service_id query parameters are required")
		return
	}
	if err := h.resetPassword(orgID, serviceID); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	log.Infof("fleet: rotated password for ClickHouse Cloud service %s", serviceID)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{
		"reset":            true,
		"restart_required": true,
	}})
}

// Restart asks the server process to exit cleanly so systemd can restart it.
// POST /api/v1/admin/restart
func (h *CloudHandler) Restart(w http.ResponseWriter, r *http.Request) {
	if !rootGuard(w, r) {
		return
	}
	log.Warn("fleet: restart requested via dashboard — exiting so systemd can respawn")
	// Reply before the process exits so the client sees a clean acknowledgement.
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{"restarting": true}})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go h.restart()
}
