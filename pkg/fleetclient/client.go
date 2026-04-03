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

package fleetclient

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	cryptotls "crypto/tls"

	"github.com/rabbitstack/fibratus/pkg/fleet"
	"github.com/rabbitstack/fibratus/pkg/util/version"
	log "github.com/sirupsen/logrus"
)

const (
	agentIDFile    = "agent-id"
	apiVersion     = "v1"
	maxRetries     = 3
	retryBaseDelay = time.Second
)

var userAgent = version.ProductToken()

// Client handles communication with the fleet server.
type Client struct {
	httpClient       *http.Client
	config           Config
	agentID          string
	hostname         string
	baseURL          string
	dataDir          string
	mu               sync.RWMutex
	stopCh           chan struct{}
	wg               sync.WaitGroup
	ruleSyncCallback RuleSyncCallback
}

// New creates a new fleet client with the given configuration.
// The dataDir is used to persist the agent ID and certificates across restarts.
func New(config Config, dataDir string) (*Client, error) {
	// Check if enrollment certificates exist (from `fibratus enroll`)
	certFile := filepath.Join(dataDir, "certs", "agent.crt")
	keyFile := filepath.Join(dataDir, "certs", "agent.key")

	// Build TLS: use system CA pool for server trust (Let's Encrypt).
	// Optionally load enrollment client cert for mTLS.
	// DO NOT replace RootCAs — nil means system CA pool.
	var tlsCfg *cryptotls.Config

	if fileExists(certFile) && fileExists(keyFile) {
		cert, err := cryptotls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			log.Warnf("fleet: failed to load enrollment certs: %v", err)
		} else {
			tlsCfg = &cryptotls.Config{
				Certificates:       []cryptotls.Certificate{cert},
				InsecureSkipVerify: config.TLSInsecureSkipVerify,
			}
			log.Info("fleet: using enrollment certificates for mTLS")
		}
	}

	if tlsCfg == nil && config.TLSInsecureSkipVerify {
		tlsCfg = &cryptotls.Config{InsecureSkipVerify: true}
	}

	// tlsCfg with nil RootCAs = Go uses system CA pool (trusts Let's Encrypt)
	transport := &http.Transport{
		TLSClientConfig: tlsCfg,
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
	}

	// Load persisted enrollment data — overrides config file settings.
	// After `fibratus enroll`, these files contain the canonical identity.
	if serverURL := loadFile(filepath.Join(dataDir, "server-url")); serverURL != "" {
		config.ServerURL = serverURL
		log.Infof("fleet: loaded server URL from enrollment: %s", serverURL)
	}
	if orgID := loadFile(filepath.Join(dataDir, "org-id")); orgID != "" {
		config.OrgID = orgID
	}

	baseURL := strings.TrimRight(config.ServerURL, "/")

	c := &Client{
		httpClient: httpClient,
		config:     config,
		baseURL:    baseURL,
		dataDir:    dataDir,
		stopCh:     make(chan struct{}),
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	c.hostname = hostname

	// Load persisted agent ID
	c.agentID = c.loadAgentID()

	return c, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}


// Register registers this agent with the fleet server. If the agent
// is already registered (has a persisted ID), it re-registers to
// update its metadata.
func (c *Client) Register() error {
	req := fleet.RegisterRequest{
		Hostname:      c.hostname,
		OSVersion:     osVersion(),
		EngineVersion: version.Get(),
		AgentGroup:    c.config.AgentGroup,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("fleet register: marshal error: %w", err)
	}

	resp, err := c.doRequest(http.MethodPost, "/agents/register", body)
	if err != nil {
		return fmt.Errorf("fleet register: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return c.readError(resp)
	}

	var apiResp struct {
		Data fleet.RegisterResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("fleet register: decode response: %w", err)
	}

	agentID := apiResp.Data.AgentID
	if agentID == "" {
		return fmt.Errorf("fleet register: server returned empty agent ID")
	}

	c.mu.Lock()
	c.agentID = agentID
	c.mu.Unlock()

	c.persistAgentID(agentID)

	log.Infof("fleet: registered with server as agent %s", agentID)
	return nil
}

// SendHeartbeat sends a single heartbeat to the fleet server.
func (c *Client) SendHeartbeat(hb fleet.Heartbeat) error {
	c.mu.RLock()
	agentID := c.agentID
	c.mu.RUnlock()

	if agentID == "" {
		return fmt.Errorf("fleet heartbeat: agent not registered")
	}

	body, err := json.Marshal(hb)
	if err != nil {
		return fmt.Errorf("fleet heartbeat: marshal error: %w", err)
	}

	path := fmt.Sprintf("/agents/%s/heartbeat", agentID)
	resp, err := c.doRequest(http.MethodPost, path, body)
	if err != nil {
		return fmt.Errorf("fleet heartbeat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.readError(resp)
	}
	return nil
}

// SendDetection reports a detection (alert) to the fleet server.
func (c *Client) SendDetection(alertJSON []byte) error {
	c.mu.RLock()
	agentID := c.agentID
	c.mu.RUnlock()

	resp, err := c.doRequestWithHeaders(http.MethodPost, "/detections", alertJSON, map[string]string{
		"X-Agent-ID": agentID,
	})
	if err != nil {
		return fmt.Errorf("fleet detection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return c.readError(resp)
	}
	return nil
}

// SendTelemetry forwards a batch of events to the fleet server.
func (c *Client) SendTelemetry(eventsJSON []byte) error {
	c.mu.RLock()
	agentID := c.agentID
	c.mu.RUnlock()

	resp, err := c.doRequestWithHeaders(http.MethodPost, "/telemetry", eventsJSON, map[string]string{
		"X-Agent-ID": agentID,
	})
	if err != nil {
		return fmt.Errorf("fleet telemetry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return c.readError(resp)
	}
	return nil
}

// AgentID returns the current agent ID.
func (c *Client) AgentID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.agentID
}

// Hostname returns the agent hostname.
func (c *Client) Hostname() string {
	return c.hostname
}

// Close shuts down the fleet client and stops background goroutines.
func (c *Client) Close() error {
	close(c.stopCh)
	c.wg.Wait()
	return nil
}

// doRequest performs an HTTP request to the fleet server API.
func (c *Client) doRequest(method, path string, body []byte) (*http.Response, error) {
	return c.doRequestWithHeaders(method, path, body, nil)
}

// doRequestWithHeaders performs an HTTP request with additional headers.
func (c *Client) doRequestWithHeaders(method, path string, body []byte, headers map[string]string) (*http.Response, error) {
	url := fmt.Sprintf("%s/api/%s%s", c.baseURL, apiVersion, path)

	var bodyReader io.Reader
	isGzipped := false

	if body != nil && c.config.EnableGzip && len(body) > 512 {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(body); err != nil {
			return nil, err
		}
		if err := gz.Close(); err != nil {
			return nil, err
		}
		bodyReader = &buf
		isGzipped = true
	} else if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("X-API-Key", c.config.APIKey)
	}
	if c.config.OrgID != "" {
		req.Header.Set("X-Org-ID", c.config.OrgID)
	}
	c.mu.RLock()
	if c.agentID != "" {
		req.Header.Set("X-Agent-ID", c.agentID)
	}
	c.mu.RUnlock()

	if isGzipped {
		req.Header.Set("Content-Encoding", "gzip")
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(retryBaseDelay * time.Duration(1<<(attempt-1))):
			case <-c.stopCh:
				return nil, fmt.Errorf("fleet client shutting down")
			}
			// Re-create request body for retry
			if body != nil {
				if isGzipped {
					var buf bytes.Buffer
					gz := gzip.NewWriter(&buf)
					gz.Write(body)
					gz.Close()
					req.Body = io.NopCloser(&buf)
				} else {
					req.Body = io.NopCloser(bytes.NewReader(body))
				}
			}
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			log.Warnf("fleet: request %s %s attempt %d failed: %v", method, path, attempt+1, err)
			continue
		}

		// Don't retry on non-server errors
		if resp.StatusCode < 500 {
			return resp, nil
		}

		// Server error — retry
		resp.Body.Close()
		lastErr = fmt.Errorf("server returned %d", resp.StatusCode)
		log.Warnf("fleet: request %s %s attempt %d got %d", method, path, attempt+1, resp.StatusCode)
	}

	return nil, fmt.Errorf("fleet: request %s %s failed after %d attempts: %w", method, path, maxRetries+1, lastErr)
}

// readError extracts an error message from a non-success HTTP response.
func (c *Client) readError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("fleet server returned %d: %s", resp.StatusCode, string(body))
}

// loadAgentID loads a previously persisted agent ID from disk.
func (c *Client) loadAgentID() string {
	return loadFile(filepath.Join(c.dataDir, agentIDFile))
}

// loadFile reads a single-value text file, returning empty string on error.
func loadFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// persistAgentID saves the agent ID to disk for persistence across restarts.
func (c *Client) persistAgentID(id string) {
	if c.dataDir == "" {
		return
	}
	path := filepath.Join(c.dataDir, agentIDFile)
	if err := os.MkdirAll(c.dataDir, 0o755); err != nil {
		log.Warnf("fleet: failed to create data dir: %v", err)
		return
	}
	if err := os.WriteFile(path, []byte(id), 0o600); err != nil {
		log.Warnf("fleet: failed to persist agent ID: %v", err)
	}
}
