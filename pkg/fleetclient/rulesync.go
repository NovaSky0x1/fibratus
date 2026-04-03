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
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
)

// RuleSyncCallback is called when rules are updated from the server.
// The path points to the directory containing the downloaded rule files.
type RuleSyncCallback func(rulesDir string) error

// PullRules downloads the current ruleset from the fleet server.
// Returns the rules directory path, whether rules changed, and any error.
func (c *Client) PullRules() (string, bool, error) {
	c.mu.RLock()
	agentID := c.agentID
	c.mu.RUnlock()

	path := "/agent/rules"
	url := fmt.Sprintf("%s/api/%s%s", c.baseURL, apiVersion, path)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", false, err
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-API-Key", c.config.APIKey)
	if c.config.OrgID != "" {
		req.Header.Set("X-Org-ID", c.config.OrgID)
	}
	if agentID != "" {
		req.Header.Set("X-Agent-ID", agentID)
	}

	// Send ETag for conditional request
	etagFile := filepath.Join(c.dataDir, "rules-etag")
	if etag, err := os.ReadFile(etagFile); err == nil {
		req.Header.Set("If-None-Match", string(etag))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("fleet rule sync: %w", err)
	}
	defer resp.Body.Close()

	rulesDir := filepath.Join(c.dataDir, "rules")

	// 304 Not Modified — rules haven't changed
	if resp.StatusCode == http.StatusNotModified {
		return rulesDir, false, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", false, fmt.Errorf("fleet rule sync: server returned %d: %s", resp.StatusCode, string(body))
	}

	// Read rule YAML from response
	rulesYAML, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20)) // 50MB max
	if err != nil {
		return "", false, fmt.Errorf("fleet rule sync: read body: %w", err)
	}

	// Write rules to local cache directory
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		return "", false, fmt.Errorf("fleet rule sync: create rules dir: %w", err)
	}

	// Split macros from rules — macros start with "- macro:" and must
	// go in a separate file so the rule compiler can load them independently.
	macrosDir := filepath.Join(rulesDir, "Macros")
	os.MkdirAll(macrosDir, 0o755)

	// Remove old fleet rule files before writing new ones (handles rule deletions)
	oldRules, _ := filepath.Glob(filepath.Join(rulesDir, "fleet-rule-*.yml"))
	for _, f := range oldRules {
		os.Remove(f)
	}
	// Also clean up legacy single-file format
	os.Remove(filepath.Join(rulesDir, "fleet-rules.yml"))

	var macrosBuf bytes.Buffer
	var ruleDocs []string
	docs := strings.Split(string(rulesYAML), "\n---\n")
	for _, doc := range docs {
		trimmed := strings.TrimSpace(doc)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "- macro:") {
			macrosBuf.WriteString(doc)
			macrosBuf.WriteString("\n")
		} else if strings.HasPrefix(trimmed, "name:") {
			ruleDocs = append(ruleDocs, doc)
		}
	}

	if macrosBuf.Len() > 0 {
		if err := os.WriteFile(filepath.Join(macrosDir, "macros.yml"), macrosBuf.Bytes(), 0o644); err != nil {
			return "", false, fmt.Errorf("fleet rule sync: write macros: %w", err)
		}
		log.Infof("fleet: wrote macros (%d bytes)", macrosBuf.Len())
	}

	// Write each rule as a separate file — the rule compiler's LoadFilters
	// calls decodeFilter per file and yaml.Unmarshal only parses the first
	// YAML document, so one-rule-per-file is required.
	for i, doc := range ruleDocs {
		ruleFile := filepath.Join(rulesDir, fmt.Sprintf("fleet-rule-%03d.yml", i+1))
		if err := os.WriteFile(ruleFile, []byte(doc), 0o644); err != nil {
			return "", false, fmt.Errorf("fleet rule sync: write rule %d: %w", i+1, err)
		}
	}
	log.Infof("fleet: wrote %d rule files", len(ruleDocs))

	// Save ETag for next request
	if etag := resp.Header.Get("ETag"); etag != "" {
		os.WriteFile(etagFile, []byte(etag), 0o644)
	}

	log.Infof("fleet: downloaded %d bytes of rules from server", len(rulesYAML))
	return rulesDir, true, nil
}

// StartRuleSync registers a rule sync callback that runs on every heartbeat.
// Rules are checked every heartbeat interval (~30s) using ETag caching
// so unchanged rules don't cause unnecessary downloads.
func (c *Client) StartRuleSync(onUpdate RuleSyncCallback) {
	c.mu.Lock()
	c.ruleSyncCallback = onUpdate
	c.mu.Unlock()

	// Initial sync immediately
	c.syncRules(onUpdate)
}

func (c *Client) syncRules(onUpdate RuleSyncCallback) {
	rulesDir, changed, err := c.PullRules()
	if err != nil {
		log.Warnf("fleet: rule sync failed: %v", err)
		return
	}
	if !changed {
		return
	}
	if onUpdate != nil {
		if err := onUpdate(rulesDir); err != nil {
			log.Errorf("fleet: rule update callback failed: %v", err)
		}
	}
}

// RulesDir returns the path where fleet-synced rules are cached.
func (c *Client) RulesDir() string {
	return filepath.Join(c.dataDir, "rules")
}
