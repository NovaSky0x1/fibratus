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

// Package clickhousecloud is a thin client for the ClickHouse Cloud Console
// API (https://api.clickhouse.cloud/v1). It covers the surface needed for the
// dashboard's "connect to a Cloud service" flow:
//
//   - List the organisations accessible to a Key ID + Secret pair
//   - List, fetch, and create services within an organisation
//   - Reset a service's password (the only way to recover credentials for an
//     existing service — the API does not return passwords on reads)
//
// Auth is HTTP Basic with the Key ID as username and Key Secret as password.
// All requests are JSON in / JSON out, time-bounded by a context.
package clickhousecloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

// DefaultBaseURL is the public ClickHouse Cloud API base.
const DefaultBaseURL = "https://api.clickhouse.cloud/v1"

// Client speaks to the ClickHouse Cloud API.
type Client struct {
	baseURL    string
	keyID      string
	keySecret  string
	httpClient *http.Client
}

// NewClient builds a client. baseURL may be empty — DefaultBaseURL is used.
// httpClient may be nil — a default with a 30s timeout is used.
func NewClient(keyID, keySecret, baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		baseURL:    baseURL,
		keyID:      keyID,
		keySecret:  keySecret,
		httpClient: httpClient,
	}
}

// APIError carries a structured error returned by the ClickHouse Cloud API.
type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("clickhouse cloud: %s (HTTP %d, code=%s)", e.Message, e.Status, e.Code)
	}
	return fmt.Sprintf("clickhouse cloud: %s (HTTP %d)", e.Message, e.Status)
}

// Endpoint describes one of a service's network endpoints. Protocol is one of
// "nativesecure", "https", etc. Use the nativesecure endpoint for our
// telemetry connection (clickhouse-go v2 native protocol).
type Endpoint struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
}

// Organization identifies a ClickHouse Cloud organisation accessible to the
// API key.
type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// Service is a ClickHouse Cloud service (one logical cluster within an org).
type Service struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Provider   string     `json:"provider"`
	Region     string     `json:"region"`
	State      string     `json:"state"`
	Endpoints  []Endpoint `json:"endpoints"`
	IPAccessList []IPAccessEntry `json:"ipAccessList,omitempty"`
}

// IPAccessEntry is one CIDR allowed to reach a service.
type IPAccessEntry struct {
	Source      string `json:"source"`
	Description string `json:"description,omitempty"`
}

// NativeSecureEndpoint returns the (host, port) of the service's TLS native
// endpoint, or empty values + false if the service hasn't published one (rare;
// happens during provisioning).
func (s *Service) NativeSecureEndpoint() (host string, port int, ok bool) {
	for _, e := range s.Endpoints {
		if e.Protocol == "nativesecure" {
			return e.Host, e.Port, true
		}
	}
	return "", 0, false
}

// CreateServiceRequest is the body for POST /organizations/{id}/services.
// Tier is "development" or "production"; MinReplicaMemoryGB / MaxReplicaMemoryGB
// shape the autoscaler. Password is optional — if empty, ClickHouse Cloud
// generates one and returns it on the response.
type CreateServiceRequest struct {
	Name              string          `json:"name"`
	Provider          string          `json:"provider"`
	Region            string          `json:"region"`
	Tier              string          `json:"tier"`
	IPAccessList      []IPAccessEntry `json:"ipAccessList,omitempty"`
	MinReplicaMemoryGB int            `json:"minReplicaMemoryGb,omitempty"`
	MaxReplicaMemoryGB int            `json:"maxReplicaMemoryGb,omitempty"`
	Password          string          `json:"password,omitempty"`
}

// CreateServiceResponse mirrors the API's response for service creation. The
// Password field is populated when the API generates one (i.e. the request
// did not supply one) — it is only ever returned at creation time and cannot
// be re-fetched.
type CreateServiceResponse struct {
	Service  Service `json:"service"`
	Password string  `json:"password,omitempty"`
}

// ListOrganizations returns all organisations accessible to the credentials.
func (c *Client) ListOrganizations(ctx context.Context) ([]Organization, error) {
	var resp struct {
		Result []Organization `json:"result"`
	}
	if err := c.do(ctx, http.MethodGet, "/organizations", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Result, nil
}

// ListServices returns all services in the given organisation.
func (c *Client) ListServices(ctx context.Context, orgID string) ([]Service, error) {
	var resp struct {
		Result []Service `json:"result"`
	}
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/organizations/%s/services", orgID), nil, &resp); err != nil {
		return nil, err
	}
	return resp.Result, nil
}

// GetService fetches one service by ID.
func (c *Client) GetService(ctx context.Context, orgID, serviceID string) (*Service, error) {
	var resp struct {
		Result Service `json:"result"`
	}
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/organizations/%s/services/%s", orgID, serviceID), nil, &resp); err != nil {
		return nil, err
	}
	return &resp.Result, nil
}

// CreateService provisions a new service. The returned password is the only
// time it is ever exposed by the API — store it before discarding the response.
func (c *Client) CreateService(ctx context.Context, orgID string, req CreateServiceRequest) (*CreateServiceResponse, error) {
	var resp struct {
		Result CreateServiceResponse `json:"result"`
	}
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/organizations/%s/services", orgID), req, &resp); err != nil {
		return nil, err
	}
	return &resp.Result, nil
}

// ResetServicePassword sets a new password for the service's default user and
// returns it. If newPassword is empty, the API generates one.
func (c *Client) ResetServicePassword(ctx context.Context, orgID, serviceID, newPassword string) (string, error) {
	body := struct {
		NewPassword string `json:"newPasswordHash,omitempty"`
	}{newPassword}
	var resp struct {
		Result struct {
			Password string `json:"password"`
		} `json:"result"`
	}
	if err := c.do(ctx, http.MethodPatch, fmt.Sprintf("/organizations/%s/services/%s/password", orgID, serviceID), body, &resp); err != nil {
		return "", err
	}
	return resp.Result.Password, nil
}

// truncateForLog clips long bodies so the journal stays readable.
func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// do is the shared transport: build the request, attach basic auth, decode JSON.
func (c *Client) do(ctx context.Context, method, path string, body, out interface{}) error {
	if c.keyID == "" || c.keySecret == "" {
		return errors.New("clickhouse cloud: API credentials not set")
	}
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("clickhouse cloud: marshal body: %w", err)
		}
		reqBody = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("clickhouse cloud: build request: %w", err)
	}
	req.SetBasicAuth(c.keyID, c.keySecret)
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("clickhouse cloud: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("clickhouse cloud: read response: %w", err)
	}
	// TEMP debug — remove once response-shape parsing is verified.
	log.Infof("clickhouse cloud: %s %s -> HTTP %d body=%s", method, path, resp.StatusCode, truncateForLog(string(respBody), 800))
	if resp.StatusCode >= 400 {
		apiErr := &APIError{Status: resp.StatusCode}
		// Best-effort decode; the response may not be JSON for some errors.
		var errEnvelope struct {
			Error APIError `json:"error"`
		}
		if json.Unmarshal(respBody, &errEnvelope) == nil && errEnvelope.Error.Message != "" {
			apiErr.Code = errEnvelope.Error.Code
			apiErr.Message = errEnvelope.Error.Message
		} else {
			apiErr.Message = string(respBody)
		}
		return apiErr
	}
	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("clickhouse cloud: decode response: %w (body=%s)", err, string(respBody))
		}
	}
	return nil
}
