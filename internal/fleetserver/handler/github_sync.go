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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/internal/fleetserver/validator"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// GitHubSyncConfig holds config for syncing rules from a GitHub repository.
type GitHubSyncConfig struct {
	RepoURL  string `json:"repo_url"`  // e.g., "https://api.github.com/repos/owner/repo"
	Branch   string `json:"branch"`    // e.g., "main"
	Path     string `json:"path"`      // e.g., "rules/" (directory within repo)
	Token    string `json:"token"`     // GitHub PAT (optional for public repos)
	Interval int    `json:"interval"`  // sync interval in minutes
	Enabled  bool   `json:"enabled"`
}

// GitHubSyncHandler manages GitHub-based detection rule synchronization.
type GitHubSyncHandler struct {
	rules  store.RuleStore
	audit  store.AuditStore
	users  store.UserStore
	client *http.Client
}

// NewGitHubSyncHandler creates a new GitHub sync handler.
func NewGitHubSyncHandler(rules store.RuleStore, audit store.AuditStore, users store.UserStore) *GitHubSyncHandler {
	return &GitHubSyncHandler{
		rules:  rules,
		audit:  audit,
		users:  users,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// GetConfig handles GET /api/v1/orgs/{org_id}/github-sync
func (h *GitHubSyncHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	cfg := loadGitHubSyncConfig(orgID)
	// Never expose the token
	cfg.Token = ""
	if cfg.RepoURL != "" {
		cfg.Token = "***configured***"
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: cfg})
}

// SaveConfig handles PUT /api/v1/orgs/{org_id}/github-sync
func (h *GitHubSyncHandler) SaveConfig(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	userID := ctxutil.UserIDFromContext(r.Context())

	var cfg GitHubSyncConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid config")
		return
	}

	if cfg.Branch == "" {
		cfg.Branch = "main"
	}
	if cfg.Path == "" {
		cfg.Path = "rules/"
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 30
	}

	saveGitHubSyncConfig(orgID, &cfg)
	logAudit(r, h.audit, h.users, userID, orgID, "update", "github_sync", "", "GitHub sync config", nil)
	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "saved"}})
}

// TriggerSync handles POST /api/v1/orgs/{org_id}/github-sync/trigger
func (h *GitHubSyncHandler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	userID := ctxutil.UserIDFromContext(r.Context())
	cfg := loadGitHubSyncConfig(orgID)

	if cfg.RepoURL == "" {
		writeError(w, http.StatusBadRequest, "GitHub sync not configured")
		return
	}

	result, err := h.syncFromGitHub(r.Context(), orgID, cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "sync failed: "+err.Error())
		return
	}

	logAudit(r, h.audit, h.users, userID, orgID, "execute", "github_sync", "", "Manual sync trigger", result)
	writeJSON(w, http.StatusOK, fleet.Response{Data: result})
}

// SyncResult contains the outcome of a GitHub sync operation.
type SyncResult struct {
	Created  int      `json:"created"`
	Updated  int      `json:"updated"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
	Duration string   `json:"duration"`
}

func (h *GitHubSyncHandler) syncFromGitHub(ctx context.Context, orgID string, cfg *GitHubSyncConfig) (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{}

	// Fetch file list from GitHub API
	apiURL := fmt.Sprintf("%s/contents/%s?ref=%s", cfg.RepoURL, cfg.Path, cfg.Branch)
	files, err := h.fetchGitHubDir(apiURL, cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("fetch repo: %w", err)
	}

	for _, file := range files {
		if !strings.HasSuffix(file.Name, ".yml") && !strings.HasSuffix(file.Name, ".yaml") {
			continue
		}
		// Skip macros directory
		if strings.Contains(file.Path, "macros/") || strings.Contains(file.Path, "Macros/") {
			continue
		}

		content, err := h.fetchGitHubFile(file.DownloadURL, cfg.Token)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: fetch error: %v", file.Name, err))
			result.Skipped++
			continue
		}

		// Validate before importing
		if err := validator.ValidateRuleYAML(content); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", file.Name, err))
			result.Skipped++
			continue
		}

		var rule fleet.Rule
		if err := parseYAMLRule(content, &rule); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: parse error: %v", file.Name, err))
			result.Skipped++
			continue
		}
		rule.RawYAML = string(content)
		rule.OrgID = orgID
		if rule.Version == "" {
			rule.Version = "1.0.0"
		}
		if rule.Severity == "" {
			rule.Severity = "medium"
		}
		rule.Enabled = true

		// Check if rule exists (by ID)
		existing, _ := h.rules.Get(ctx, orgID, rule.ID)
		if existing != nil {
			if err := h.rules.Update(ctx, &rule); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: update error: %v", file.Name, err))
				result.Skipped++
			} else {
				result.Updated++
			}
		} else {
			if err := h.rules.Create(ctx, &rule); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: create error: %v", file.Name, err))
				result.Skipped++
			} else {
				result.Created++
			}
		}
	}

	result.Duration = time.Since(start).String()
	log.Infof("fleet: GitHub sync completed for org %s: %d created, %d updated, %d skipped, %d errors",
		orgID, result.Created, result.Updated, result.Skipped, len(result.Errors))

	return result, nil
}

type githubFile struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
}

func (h *GitHubSyncHandler) fetchGitHubDir(url, token string) ([]githubFile, error) {
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var files []githubFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Filter to files only (skip subdirectories for now)
	result := make([]githubFile, 0)
	for _, f := range files {
		if f.Type == "file" {
			result = append(result, f)
		}
	}
	return result, nil
}

func (h *GitHubSyncHandler) fetchGitHubFile(url, token string) ([]byte, error) {
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// In-memory config store (per-org). In production, this would be in PostgreSQL.
var githubSyncConfigs = make(map[string]*GitHubSyncConfig)

func loadGitHubSyncConfig(orgID string) *GitHubSyncConfig {
	cfg, ok := githubSyncConfigs[orgID]
	if !ok {
		return &GitHubSyncConfig{Branch: "main", Path: "rules/", Interval: 30}
	}
	return cfg
}

func saveGitHubSyncConfig(orgID string, cfg *GitHubSyncConfig) {
	githubSyncConfigs[orgID] = cfg
}

// StartPeriodicSync starts background goroutines for each configured org.
// Call this from server startup.
func (h *GitHubSyncHandler) StartPeriodicSync(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute) // check for configs every 5 minutes
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				for orgID, cfg := range githubSyncConfigs {
					if !cfg.Enabled || cfg.RepoURL == "" {
						continue
					}
					result, err := h.syncFromGitHub(ctx, orgID, cfg)
					if err != nil {
						log.Warnf("fleet: GitHub periodic sync failed for org %s: %v", orgID, err)
					} else if result.Created > 0 || result.Updated > 0 {
						log.Infof("fleet: GitHub periodic sync for org %s: %d created, %d updated", orgID, result.Created, result.Updated)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
