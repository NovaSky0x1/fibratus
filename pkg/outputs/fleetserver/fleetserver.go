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

package fleetserver

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rabbitstack/fibratus/pkg/event"
	"github.com/rabbitstack/fibratus/pkg/outputs"
	"github.com/rabbitstack/fibratus/pkg/util/tls"
	"github.com/rabbitstack/fibratus/pkg/util/version"
	log "github.com/sirupsen/logrus"
)

var userAgent = version.ProductToken()

type fleetOutput struct {
	client    *http.Client
	serverURL string
	apiKey    string
	orgID     string
	agentID   string
}

func init() {
	outputs.Register(outputs.FleetServer, initFleetServer)
}

func initFleetServer(config outputs.Config) (outputs.OutputGroup, error) {
	cfg, ok := config.Output.(Config)
	if !ok {
		return outputs.Fail(outputs.ErrInvalidConfig(outputs.FleetServer, config.Output))
	}

	if cfg.ServerURL == "" {
		// Try to load from enrollment data
		cfg.ServerURL = loadEnrollmentFile("server-url")
		cfg.OrgID = loadEnrollmentFile("org-id")
		cfg.AgentID = loadEnrollmentFile("agent-id")
	}

	if cfg.ServerURL == "" {
		return outputs.Fail(fmt.Errorf("fleet server URL not configured and no enrollment data found"))
	}

	tlsCert := ""
	tlsKey := ""
	certDir := enrollmentCertDir()
	if certDir != "" {
		certFile := filepath.Join(certDir, "agent.crt")
		keyFile := filepath.Join(certDir, "agent.key")
		if fileExists(certFile) && fileExists(keyFile) {
			tlsCert = certFile
			tlsKey = keyFile
		}
	}

	tlsConfig, err := tls.MakeConfig(tlsCert, tlsKey, cfg.TLSCA, cfg.TLSInsecureSkipVerify)
	if err != nil {
		return outputs.Fail(fmt.Errorf("fleet output: TLS config: %v", err))
	}

	httpClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
		Timeout:   30 * time.Second,
	}

	client := &fleetOutput{
		client:    httpClient,
		serverURL: strings.TrimRight(cfg.ServerURL, "/"),
		apiKey:    cfg.APIKey,
		orgID:     cfg.OrgID,
		agentID:   cfg.AgentID,
	}

	return outputs.Success(client), nil
}

func (f *fleetOutput) Connect() error {
	log.Infof("fleet output: connected to %s", f.serverURL)
	return nil
}

func (f *fleetOutput) Close() error { return nil }

// telemetryEventNames is the set of events always stored on the fleet
// server. Everything else is only sent if it triggered a detection rule
// or evasion flag.
var telemetryEventNames = map[string]bool{
	"CreateProcess": true, // process spawn — core EDR visibility
	"Connect":       true, // outbound connections — C2, lateral movement
	"QueryDns":      true, // DNS resolution — C2/exfil domain detection
	"ReplyDns":      true, // DNS answers
	"LoadImage":     true, // module/DLL loads — sideloading, injection detection
	"CreateFile":    true, // file creation — malware drops, staging
	"DeleteFile":    true, // file deletion — covering tracks
	"RenameFile":    true, // file renames — evasion techniques
	"RegCreateKey":  true, // registry key creation — persistence
	"RegDeleteKey":  true, // registry key deletion — defense evasion
	"RegDeleteValue": true, // registry value deletion — defense evasion
}

// telemetryDropNames is a fast-reject set for noisy events that should
// never be sent, even if they somehow have metadata attached.
var telemetryDropNames = map[string]bool{
	"SubmitThreadpoolWork":     true,
	"SubmitThreadpoolCallback": true,
	"SetThreadpoolTimer":       true,
	"VirtualAlloc":             true,
	"VirtualFree":              true,
	"MapViewFile":              true,
	"UnmapViewFile":            true,
	"CreateHandle":             true,
	"CloseHandle":              true,
	"DuplicateHandle":          true,
	"ReadFile":                 true,
	"CloseFile":                true,
	"ReleaseFile":              true,
	"EnumDirectory":            true,
	"FileOpEnd":                true,
	"FileRundown":              true,
	"SetFileInformation":       true,
	"RegOpenKey":               true,
	"RegCloseKey":              true,
	"RegQueryKey":              true,
	"RegQueryValue":            true,
	"RegSetValue":              true,
	"RegKCBRundown":            true,
	"RegCreateKCB":             true,
	"MapFileRundown":           true,
	"OpenProcess":              true,
	"TerminateProcess":         true,
	"Accept":                   true,
	"UnloadImage":              true,
	"CreateThread":             true,
	"TerminateThread":          true,
	"OpenThread":               true,
	"ThreadRundown":            true,
	"SetThreadContext":         true,
	"ProcessRundown":           true,
	"ImageRundown":             true,
	"StackWalk":                true,
	"CreateSymbolicLinkObject": true,
}

// securityRelevant filters the batch to only include events worth
// storing on the fleet server.
func securityRelevant(batch *event.Batch) *event.Batch {
	filtered := make([]*event.Event, 0, len(batch.Events)/4)
	for _, evt := range batch.Events {
		if telemetryDropNames[evt.Name] {
			continue
		}
		// Always send events with rule matches or evasion flags
		if len(evt.Metadata) > 0 || evt.Evasions > 0 {
			filtered = append(filtered, evt)
			continue
		}
		if telemetryEventNames[evt.Name] {
			filtered = append(filtered, evt)
		}
	}
	return &event.Batch{Events: filtered}
}

// Publish sends a batch of events to the fleet server telemetry endpoint.
// Events are filtered to security-relevant types, serialized as JSON,
// and gzip-compressed.
func (f *fleetOutput) Publish(batch *event.Batch) error {
	batch = securityRelevant(batch)
	if len(batch.Events) == 0 {
		return nil
	}
	buf := batch.MarshalJSON()
	if len(buf) == 0 {
		return nil
	}

	// Gzip compress
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(buf); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/agent/telemetry", f.serverURL)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &compressed)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	if f.apiKey != "" {
		req.Header.Set("X-API-Key", f.apiKey)
	}
	// Load agent ID fresh — it may not exist at output init time since
	// the fleet client registers after the output is created.
	agentID := f.agentID
	if agentID == "" {
		agentID = loadEnrollmentFile("agent-id")
	}
	if agentID != "" {
		req.Header.Set("X-Agent-ID", agentID)
	}
	if f.orgID != "" {
		req.Header.Set("X-Org-ID", f.orgID)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("fleet output: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("fleet output: server returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Config for the fleet server output.
type Config struct {
	Enabled              bool   `mapstructure:"enabled"`
	ServerURL            string `mapstructure:"server-url"`
	APIKey               string `mapstructure:"api-key"`
	OrgID                string `mapstructure:"org-id"`
	AgentID              string `mapstructure:"agent-id"`
	TLSCA                string `mapstructure:"tls-ca"`
	TLSInsecureSkipVerify bool  `mapstructure:"tls-insecure-skip-verify"`
}

func loadEnrollmentFile(name string) string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dataDir := filepath.Join(filepath.Dir(exe), "..", "data")
	data, err := os.ReadFile(filepath.Join(dataDir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func enrollmentCertDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "..", "data", "certs")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
