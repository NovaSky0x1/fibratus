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
	"database/sql"
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
	RepoURL   string `json:"repo_url"`
	Branch    string `json:"branch"`
	Path      string `json:"path"`
	Token     string `json:"token"`
	Interval  int    `json:"interval"`
	Enabled   bool   `json:"enabled"`
	Scope     string `json:"scope"`      // "account" (apply to all orgs) or "org" (apply to this org only)
	AccountID string `json:"account_id"` // which account this config belongs to
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
	accountID := ctxutil.AccountIDFromContext(r.Context())
	userID := ctxutil.UserIDFromContext(r.Context())

	var cfg GitHubSyncConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid config")
		return
	}

	// Auto-convert GitHub web URLs to API URLs
	// e.g., "https://github.com/owner/repo" → "https://api.github.com/repos/owner/repo"
	if strings.HasPrefix(cfg.RepoURL, "https://github.com/") {
		path := strings.TrimPrefix(cfg.RepoURL, "https://github.com/")
		path = strings.TrimSuffix(path, "/")
		// Remove /tree/branch/path if present
		if idx := strings.Index(path, "/tree/"); idx >= 0 {
			path = path[:idx]
		}
		cfg.RepoURL = "https://api.github.com/repos/" + path
	}
	cfg.RepoURL = strings.TrimSuffix(cfg.RepoURL, "/")

	if cfg.Branch == "" {
		cfg.Branch = "main"
	}
	cfg.Path = strings.TrimSuffix(cfg.Path, "/")
	if cfg.Interval <= 0 {
		cfg.Interval = 30
	}
	if cfg.Scope == "" {
		cfg.Scope = "account"
	}
	cfg.AccountID = accountID

	saveGitHubSyncConfig(orgID, &cfg)
	logAudit(r, h.audit, h.users, userID, orgID, "update", "github_sync", "", "GitHub sync config (scope="+cfg.Scope+")", nil)
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

	// Determine target orgs: account-scope = all orgs in account, org-scope = just this org
	targetOrgIDs := []string{orgID}
	if cfg.Scope == "account" && cfg.AccountID != "" && githubSyncDB != nil {
		rows, err := githubSyncDB.QueryContext(ctx,
			`SELECT id FROM organizations WHERE account_id = $1`, cfg.AccountID)
		if err == nil {
			targetOrgIDs = nil
			for rows.Next() {
				var oid string
				if rows.Scan(&oid) == nil {
					targetOrgIDs = append(targetOrgIDs, oid)
				}
			}
			rows.Close()
		}
	}

	for _, file := range files {
		if !strings.HasSuffix(file.Name, ".yml") && !strings.HasSuffix(file.Name, ".yaml") {
			continue
		}
		if strings.Contains(file.Path, "macros/") || strings.Contains(file.Path, "Macros/") {
			continue
		}

		content, err := h.fetchGitHubFile(file.DownloadURL, cfg.Token)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: fetch error: %v", file.Name, err))
			result.Skipped++
			continue
		}

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
		if rule.Version == "" {
			rule.Version = "1.0.0"
		}
		if rule.Severity == "" {
			rule.Severity = "medium"
		}
		rule.Enabled = true

		// Apply to each target org
		for _, targetOrg := range targetOrgIDs {
			rule.OrgID = targetOrg
			existing, _ := h.rules.Get(ctx, targetOrg, rule.ID)
			if existing != nil {
				if err := h.rules.Update(ctx, &rule); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("%s (org %s): update error: %v", file.Name, targetOrg, err))
					result.Skipped++
				} else {
					result.Updated++
				}
			} else {
				if err := h.rules.Create(ctx, &rule); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("%s (org %s): create error: %v", file.Name, targetOrg, err))
					result.Skipped++
				} else {
					result.Created++
				}
			}
		}
	}

	result.Duration = time.Since(start).String()
	scope := cfg.Scope
	if scope == "" {
		scope = "org"
	}
	log.Infof("fleet: GitHub sync completed (scope=%s, orgs=%d): %d created, %d updated, %d skipped, %d errors",
		scope, len(targetOrgIDs), result.Created, result.Updated, result.Skipped, len(result.Errors))

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

	// Recursively fetch files from subdirectories too
	result := make([]githubFile, 0)
	for _, f := range files {
		if f.Type == "file" {
			result = append(result, f)
		} else if f.Type == "dir" {
			// Recurse into subdirectories
			subURL := fmt.Sprintf("%s?ref=%s", f.Path, "main")
			// Build proper API URL for subdirectory
			parts := strings.SplitN(url, "/contents/", 2)
			if len(parts) == 2 {
				baseURL := parts[0]
				subDirURL := fmt.Sprintf("%s/contents/%s", baseURL, f.Path)
				if !strings.Contains(subDirURL, "?ref=") {
					ref := ""
					if qIdx := strings.Index(url, "?ref="); qIdx >= 0 {
						ref = url[qIdx:]
					}
					subDirURL += ref
				}
				_ = subURL // suppress unused
				subFiles, err := h.fetchGitHubDir(subDirURL, token)
				if err == nil {
					result = append(result, subFiles...)
				}
			}
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

// Database-backed config store
var githubSyncDB *sql.DB

// SetGitHubSyncDB sets the database connection for config persistence.
func SetGitHubSyncDB(db *sql.DB) {
	githubSyncDB = db
}

// loadGitHubSyncConfig loads config — tries account-level first, then org-level
func loadGitHubSyncConfig(orgID string) *GitHubSyncConfig {
	cfg := &GitHubSyncConfig{Branch: "main", Path: "", Interval: 30, Scope: "account"}
	if githubSyncDB == nil {
		return cfg
	}
	// Try org-specific first, then account-level
	row := githubSyncDB.QueryRow(
		`SELECT repo_url, branch, path, token, interval_min, enabled, COALESCE(scope, 'account'), COALESCE(account_id, '')
		 FROM github_sync_configs WHERE org_id = $1 OR account_id = (SELECT account_id FROM organizations WHERE id = $1)
		 ORDER BY CASE WHEN org_id = $1 THEN 0 ELSE 1 END LIMIT 1`, orgID)
	if err := row.Scan(&cfg.RepoURL, &cfg.Branch, &cfg.Path, &cfg.Token, &cfg.Interval, &cfg.Enabled, &cfg.Scope, &cfg.AccountID); err != nil {
		return cfg
	}
	return cfg
}

// loadAccountSyncConfig loads config specifically for an account
func loadAccountSyncConfig(accountID string) *GitHubSyncConfig {
	cfg := &GitHubSyncConfig{Branch: "main", Path: "", Interval: 30, Scope: "account", AccountID: accountID}
	if githubSyncDB == nil {
		return cfg
	}
	row := githubSyncDB.QueryRow(
		`SELECT repo_url, branch, path, token, interval_min, enabled FROM github_sync_configs WHERE account_id = $1 AND scope = 'account' LIMIT 1`, accountID)
	row.Scan(&cfg.RepoURL, &cfg.Branch, &cfg.Path, &cfg.Token, &cfg.Interval, &cfg.Enabled)
	return cfg
}

func saveGitHubSyncConfig(orgID string, cfg *GitHubSyncConfig) {
	if githubSyncDB == nil {
		return
	}
	accountID := cfg.AccountID
	if accountID == "" {
		// Look up account from org
		githubSyncDB.QueryRow(`SELECT account_id FROM organizations WHERE id = $1`, orgID).Scan(&accountID)
	}
	scope := cfg.Scope
	if scope == "" {
		scope = "account"
	}

	// Delete existing configs for this account/org to avoid duplicates
	if scope == "account" {
		githubSyncDB.Exec(`DELETE FROM github_sync_configs WHERE account_id = $1 AND scope = 'account'`, accountID)
	} else {
		githubSyncDB.Exec(`DELETE FROM github_sync_configs WHERE org_id = $1 AND scope = 'org'`, orgID)
	}

	githubSyncDB.Exec(
		`INSERT INTO github_sync_configs (account_id, org_id, repo_url, branch, path, token, interval_min, enabled, scope, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())`,
		accountID, orgID, cfg.RepoURL, cfg.Branch, cfg.Path, cfg.Token, cfg.Interval, cfg.Enabled, scope)
}

// StartPeriodicSync starts background goroutines for each configured org.
// Call this from server startup.
func (h *GitHubSyncHandler) StartPeriodicSync(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if githubSyncDB == nil {
					continue
				}
				rows, err := githubSyncDB.QueryContext(ctx,
					`SELECT DISTINCT COALESCE(account_id, org_id) as key, account_id, org_id, scope FROM github_sync_configs WHERE enabled = true AND repo_url != ''`)
				if err != nil {
					continue
				}
				type syncJob struct {
					accountID, orgID, scope string
				}
				var jobs []syncJob
				for rows.Next() {
					var j syncJob
					var key string
					if rows.Scan(&key, &j.accountID, &j.orgID, &j.scope) == nil {
						jobs = append(jobs, j)
					}
				}
				rows.Close()

				for _, job := range jobs {
					var cfg *GitHubSyncConfig
					if job.scope == "account" {
						cfg = loadAccountSyncConfig(job.accountID)
					} else {
						cfg = loadGitHubSyncConfig(job.orgID)
					}
					if cfg.RepoURL == "" {
						continue
					}
					result, err := h.syncFromGitHub(ctx, job.orgID, cfg)
					if err != nil {
						log.Warnf("fleet: GitHub periodic sync failed: %v", err)
					} else if result.Created > 0 || result.Updated > 0 {
						log.Infof("fleet: GitHub periodic sync: %d created, %d updated", result.Created, result.Updated)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
