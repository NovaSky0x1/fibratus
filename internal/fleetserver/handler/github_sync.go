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
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/qlparser"
	"github.com/rabbitstack/fibratus/internal/fleetserver/sigma"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/internal/fleetserver/validator"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// GitHubSyncConfig holds config for syncing rules from a GitHub repository.
type GitHubSyncConfig struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`        // user-friendly label
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
	macros store.MacroStore
	audit  store.AuditStore
	users  store.UserStore
	client *http.Client
}

// NewGitHubSyncHandler creates a new GitHub sync handler.
func NewGitHubSyncHandler(rules store.RuleStore, macros store.MacroStore, audit store.AuditStore, users store.UserStore) *GitHubSyncHandler {
	return &GitHubSyncHandler{
		rules:  rules,
		macros: macros,
		audit:  audit,
		users:  users,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// loadMacros loads org macros and converts them for the QL parser.
func (h *GitHubSyncHandler) loadMacros(ctx context.Context, orgID string) map[string]*qlparser.Macro {
	dbMacros, err := h.macros.List(ctx, orgID)
	if err != nil || len(dbMacros) == 0 {
		return nil
	}
	macros := make(map[string]*qlparser.Macro, len(dbMacros))
	for _, m := range dbMacros {
		macros[m.Name] = &qlparser.Macro{
			ID:   m.Name,
			Expr: m.Expr,
			List: m.List,
		}
	}
	return macros
}

// ListConfigs handles GET /api/v1/orgs/{org_id}/github-sync
func (h *GitHubSyncHandler) ListConfigs(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	configs := loadAllSyncConfigs(orgID)
	for i := range configs {
		if configs[i].Token != "" {
			configs[i].Token = "***configured***"
		}
	}
	writeJSON(w, http.StatusOK, fleet.Response{Data: configs})
}

// SaveConfig handles POST /api/v1/orgs/{org_id}/github-sync
func (h *GitHubSyncHandler) SaveConfig(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	accountID := ctxutil.AccountIDFromContext(r.Context())
	userID := ctxutil.UserIDFromContext(r.Context())

	var cfg GitHubSyncConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid config")
		return
	}

	if strings.HasPrefix(cfg.RepoURL, "https://github.com/") {
		path := strings.TrimPrefix(cfg.RepoURL, "https://github.com/")
		path = strings.TrimSuffix(path, "/")
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
	if cfg.Name == "" {
		parts := strings.Split(cfg.RepoURL, "/")
		if len(parts) > 0 {
			cfg.Name = parts[len(parts)-1]
		}
	}
	cfg.AccountID = accountID
	saveSyncConfig(orgID, &cfg)
	logAudit(r, h.audit, h.users, userID, orgID, "update", "github_sync", cfg.ID, cfg.Name, nil)
	writeJSON(w, http.StatusOK, fleet.Response{Data: cfg})
}

// DeleteConfig handles DELETE /api/v1/orgs/{org_id}/github-sync/{id}
func (h *GitHubSyncHandler) DeleteConfig(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	accountID := ctxutil.AccountIDFromContext(r.Context())
	parts := strings.Split(r.URL.Path, "/github-sync/")
	if len(parts) < 2 || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "config ID required")
		return
	}
	configID := strings.TrimSuffix(strings.TrimSuffix(parts[1], "/"), "/trigger")

	// Load the config to get the repo URL before deleting
	cfg := loadSyncConfigByID(orgID, configID)
	if cfg == nil {
		writeError(w, http.StatusNotFound, "sync config not found")
		return
	}

	// Determine which orgs were affected
	source := "github:" + cfg.RepoURL
	targetOrgIDs := []string{orgID}
	if cfg.Scope == "account" && accountID != "" && githubSyncDB != nil {
		rows, _ := githubSyncDB.Query(`SELECT id FROM organizations WHERE account_id = $1`, accountID)
		if rows != nil {
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

	// Delete all rules from this source across affected orgs
	for _, oid := range targetOrgIDs {
		deleted, err := h.rules.DeleteBySource(r.Context(), oid, source)
		if err != nil {
			log.Warnf("fleet: failed to delete rules from source %s in org %s: %v", source, oid, err)
		} else if deleted > 0 {
			log.Infof("fleet: deleted %d rules from source %s in org %s", deleted, source, oid)
		}
	}

	// Delete the config itself
	if githubSyncDB != nil {
		githubSyncDB.Exec(`DELETE FROM github_sync_configs WHERE id = $1 AND org_id = $2`, configID, orgID)
	}
	log.Infof("fleet: deleted GitHub sync config %s and its rules (source=%s)", configID, source)
	w.WriteHeader(http.StatusNoContent)
}

// TriggerSync handles POST /api/v1/orgs/{org_id}/github-sync/trigger — syncs ALL sources
func (h *GitHubSyncHandler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	userID := ctxutil.UserIDFromContext(r.Context())
	configs := loadAllSyncConfigs(orgID)
	if len(configs) == 0 {
		writeError(w, http.StatusBadRequest, "no GitHub sync sources configured")
		return
	}
	allResults := &SyncResult{}
	for _, cfg := range configs {
		if cfg.RepoURL == "" {
			continue
		}
		result, err := h.syncFromGitHub(r.Context(), orgID, cfg)
		if err != nil {
			allResults.Errors = append(allResults.Errors, cfg.Name+": "+err.Error())
			continue
		}
		allResults.Created += result.Created
		allResults.Updated += result.Updated
		allResults.Deleted += result.Deleted
		allResults.Skipped += result.Skipped
		allResults.Invalid += result.Invalid
		allResults.Errors = append(allResults.Errors, result.Errors...)
		allResults.ValidationErrors = append(allResults.ValidationErrors, result.ValidationErrors...)
	}
	allResults.Duration = time.Since(time.Now()).String()
	logAudit(r, h.audit, h.users, userID, orgID, "execute", "github_sync", "", "Sync all sources", allResults)
	writeJSON(w, http.StatusOK, fleet.Response{Data: allResults})
}

// TriggerSyncOne handles POST /api/v1/orgs/{org_id}/github-sync/{id}/trigger
func (h *GitHubSyncHandler) TriggerSyncOne(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	userID := ctxutil.UserIDFromContext(r.Context())
	parts := strings.Split(r.URL.Path, "/github-sync/")
	if len(parts) < 2 {
		writeError(w, http.StatusBadRequest, "config ID required")
		return
	}
	configID := strings.TrimSuffix(parts[1], "/trigger")
	cfg := loadSyncConfigByID(orgID, configID)
	if cfg == nil || cfg.RepoURL == "" {
		writeError(w, http.StatusBadRequest, "sync source not found")
		return
	}
	result, err := h.syncFromGitHub(r.Context(), orgID, cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "sync failed: "+err.Error())
		return
	}
	logAudit(r, h.audit, h.users, userID, orgID, "execute", "github_sync", configID, "Sync: "+cfg.Name, result)
	writeJSON(w, http.StatusOK, fleet.Response{Data: result})
}

// SyncResult contains the outcome of a GitHub sync operation.
type SyncResult struct {
	Created          int              `json:"created"`
	Updated          int              `json:"updated"`
	Deleted          int              `json:"deleted"`
	Skipped          int              `json:"skipped"`
	Invalid          int              `json:"invalid"`
	Errors           []string         `json:"errors,omitempty"`
	ValidationErrors []RuleSyncError  `json:"validation_errors,omitempty"`
	Duration         string           `json:"duration"`
}

// RuleSyncError describes a validation failure for a specific rule during sync.
type RuleSyncError struct {
	RuleName string `json:"rule_name"`
	FileName string `json:"file_name"`
	Error    string `json:"error"`
}

func (h *GitHubSyncHandler) syncFromGitHub(ctx context.Context, orgID string, cfg *GitHubSyncConfig) (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{}
	source := "github:" + cfg.RepoURL
	syncedIDs := make(map[string][]string) // orgID -> rule IDs synced

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

		var rule fleet.Rule
		isSigma := false

		if err := validator.ValidateRuleYAML(content); err != nil {
			// Not a valid Fibratus rule — try SIGMA conversion
			convResult := sigma.Convert(content)
			if convResult.Success && convResult.FibratusYAML != "" {
				isSigma = true
				converted := []byte(convResult.FibratusYAML)
				if parseErr := parseYAMLRule(converted, &rule); parseErr != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("%s: sigma conversion parse error: %v", file.Name, parseErr))
					result.Skipped++
					continue
				}
				rule.RawYAML = convResult.FibratusYAML
				log.Infof("fleet: GitHub sync: converted SIGMA rule %q → %q", convResult.SigmaTitle, rule.Name)
			} else if convResult.Unconvertible {
				result.Skipped++
				continue
			} else {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", file.Name, err))
				result.Skipped++
				continue
			}
		} else {
			if err := parseYAMLRule(content, &rule); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: parse error: %v", file.Name, err))
				result.Skipped++
				continue
			}
			rule.RawYAML = string(content)
		}
		if rule.Version == "" {
			rule.Version = "1.0.0"
		}
		if rule.Severity == "" {
			rule.Severity = "medium"
		}

		// Run condition validation using the real QL parser with macros
		macros := h.loadMacros(ctx, orgID)
		log.Infof("fleet: GitHub sync: validating %q with %d macros (org=%s)", rule.Name, len(macros), orgID)
		condResult := validator.ValidateConditionWithMacros(rule.Condition, macros)
		if condResult.Valid {
			rule.ValidationStatus = "valid"
			rule.ValidationErrors = json.RawMessage(`[]`)
			rule.Enabled = true
		} else {
			rule.ValidationStatus = "invalid"
			errJSON, _ := json.Marshal(condResult.Errors)
			rule.ValidationErrors = errJSON
			rule.Enabled = false
			result.Invalid++
			for _, e := range condResult.Errors {
				syncErr := RuleSyncError{
					RuleName: rule.Name,
					FileName: file.Name,
					Error:    e.Message,
				}
				result.ValidationErrors = append(result.ValidationErrors, syncErr)
			}
			log.Warnf("fleet: GitHub sync: rule %q failed validation: %d errors", rule.Name, len(condResult.Errors))
		}

		// Apply to each target org
		rule.Source = source
		if isSigma {
			rule.Source = "sigma:" + source
		}
		for _, targetOrg := range targetOrgIDs {
			rule.OrgID = targetOrg
			syncedIDs[targetOrg] = append(syncedIDs[targetOrg], rule.ID)
			existing, _ := h.rules.Get(ctx, targetOrg, rule.ID)
			if existing != nil {
				// Preserve user's local enabled/disabled state — don't overwrite manual edits
				rule.Enabled = existing.Enabled
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

	// Clean sync: remove rules from this source that weren't in the current sync
	for _, targetOrg := range targetOrgIDs {
		deleted, err := h.rules.DeleteBySourceExcept(ctx, targetOrg, source, syncedIDs[targetOrg])
		if err != nil {
			log.Warnf("fleet: clean sync delete error for org %s: %v", targetOrg, err)
		} else if deleted > 0 {
			result.Deleted += deleted
			log.Infof("fleet: clean sync removed %d stale rules from org %s", deleted, targetOrg)
		}
	}

	result.Duration = time.Since(start).String()
	scope := cfg.Scope
	if scope == "" {
		scope = "org"
	}
	log.Infof("fleet: GitHub sync completed (scope=%s, orgs=%d): %d created, %d updated, %d skipped, %d invalid, %d errors",
		scope, len(targetOrgIDs), result.Created, result.Updated, result.Skipped, result.Invalid, len(result.Errors))

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

// loadAllSyncConfigs returns all sync configs for an org (including account-level).
func loadAllSyncConfigs(orgID string) []*GitHubSyncConfig {
	if githubSyncDB == nil {
		return nil
	}
	rows, err := githubSyncDB.Query(
		`SELECT COALESCE(id,''), COALESCE(name,''), repo_url, branch, path, token, interval_min, enabled, COALESCE(scope,'account'), COALESCE(account_id,'')
		 FROM github_sync_configs
		 WHERE org_id = $1 OR account_id = (SELECT account_id FROM organizations WHERE id = $1)
		 ORDER BY name ASC`, orgID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var configs []*GitHubSyncConfig
	for rows.Next() {
		cfg := &GitHubSyncConfig{}
		if rows.Scan(&cfg.ID, &cfg.Name, &cfg.RepoURL, &cfg.Branch, &cfg.Path, &cfg.Token, &cfg.Interval, &cfg.Enabled, &cfg.Scope, &cfg.AccountID) == nil {
			configs = append(configs, cfg)
		}
	}
	return configs
}

// loadSyncConfigByID returns a single sync config by ID.
func loadSyncConfigByID(orgID, id string) *GitHubSyncConfig {
	if githubSyncDB == nil {
		return nil
	}
	cfg := &GitHubSyncConfig{}
	row := githubSyncDB.QueryRow(
		`SELECT COALESCE(id,''), COALESCE(name,''), repo_url, branch, path, token, interval_min, enabled, COALESCE(scope,'account'), COALESCE(account_id,'')
		 FROM github_sync_configs WHERE id = $1 AND (org_id = $2 OR account_id = (SELECT account_id FROM organizations WHERE id = $2))`, id, orgID)
	if row.Scan(&cfg.ID, &cfg.Name, &cfg.RepoURL, &cfg.Branch, &cfg.Path, &cfg.Token, &cfg.Interval, &cfg.Enabled, &cfg.Scope, &cfg.AccountID) != nil {
		return nil
	}
	return cfg
}

func saveSyncConfig(orgID string, cfg *GitHubSyncConfig) {
	if githubSyncDB == nil {
		return
	}
	accountID := cfg.AccountID
	if accountID == "" {
		githubSyncDB.QueryRow(`SELECT account_id FROM organizations WHERE id = $1`, orgID).Scan(&accountID)
	}
	if cfg.Scope == "" {
		cfg.Scope = "account"
	}
	if cfg.ID == "" {
		// New config — generate ID
		b := make([]byte, 16)
		rand.Read(b)
		cfg.ID = fmt.Sprintf("%x", b)
	}

	// Upsert: try update first, then insert
	res, _ := githubSyncDB.Exec(
		`UPDATE github_sync_configs SET name=$3, repo_url=$4, branch=$5, path=$6, token=CASE WHEN $7='' THEN token ELSE $7 END,
			interval_min=$8, enabled=$9, scope=$10, updated_at=NOW()
		 WHERE id=$1 AND org_id=$2`,
		cfg.ID, orgID, cfg.Name, cfg.RepoURL, cfg.Branch, cfg.Path, cfg.Token, cfg.Interval, cfg.Enabled, cfg.Scope)
	if n, _ := res.RowsAffected(); n == 0 {
		token := cfg.Token
		if token == "" {
			token = ""
		}
		githubSyncDB.Exec(
			`INSERT INTO github_sync_configs (id, account_id, org_id, name, repo_url, branch, path, token, interval_min, enabled, scope, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())`,
			cfg.ID, accountID, orgID, cfg.Name, cfg.RepoURL, cfg.Branch, cfg.Path, token, cfg.Interval, cfg.Enabled, cfg.Scope)
	}
}

// StartPeriodicSync starts background goroutines for each configured sync source.
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
					`SELECT COALESCE(id,''), org_id, COALESCE(name,''), repo_url, branch, path, token, interval_min, COALESCE(scope,'account'), COALESCE(account_id,'')
					 FROM github_sync_configs WHERE enabled = true AND repo_url != ''`)
				if err != nil {
					continue
				}
				var configs []*GitHubSyncConfig
				var orgIDs []string
				for rows.Next() {
					cfg := &GitHubSyncConfig{}
					var oid string
					if rows.Scan(&cfg.ID, &oid, &cfg.Name, &cfg.RepoURL, &cfg.Branch, &cfg.Path, &cfg.Token, &cfg.Interval, &cfg.Scope, &cfg.AccountID) == nil {
						configs = append(configs, cfg)
						orgIDs = append(orgIDs, oid)
					}
				}
				rows.Close()

				for i, cfg := range configs {
					result, err := h.syncFromGitHub(ctx, orgIDs[i], cfg)
					if err != nil {
						log.Warnf("fleet: periodic sync %q failed: %v", cfg.Name, err)
					} else if result.Created > 0 || result.Updated > 0 || result.Deleted > 0 {
						log.Infof("fleet: periodic sync %q: %d created, %d updated, %d deleted", cfg.Name, result.Created, result.Updated, result.Deleted)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
