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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/qlparser"
	"github.com/rabbitstack/fibratus/internal/fleetserver/sigma"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/internal/fleetserver/validator"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

const (
	sigmahqRepoURL  = "https://github.com/SigmaHQ/sigma.git"
	sigmahqRulesDir = "rules/windows"
	sigmahqSource   = "sigmahq"
)

// noisySigmaRules are SIGMA rules that produce excessive false positives on
// virtually every Windows system. They are imported but disabled by default —
// users can enable them if their environment warrants it.
var noisySigmaRules = map[string]bool{
	"87e3c4e8-a6a8-4ad9-bb4f-46e7ff99a180": true, // Change PowerShell Policies to an Insecure Level
	"4d07b1f4-cb00-4470-b9f8-b0191d48ff52": true, // DNS Query To Remote Access Software Domain From Non-Browser App
	"65236ec7-ace0-4f0c-82fd-737b04fd4dcb": true, // EVTX Created In Uncommon Location
	"71158e3f-df67-472b-930e-7d287acaa3e1": true, // Execution Of Non-Existing File
	"d88d0ab2-e696-4d40-a2ed-9790064e66b3": true, // Modification of IE Registry Settings
	"f4bbd493-b796-416e-bbf2-121235348529": true, // Non Interactive PowerShell Process Spawned
	"1027d292-dd87-4a1a-8701-2abe04d7783c": true, // PSScriptPolicyTest Creation By Uncommon Process
	"7047d730-036f-4f40-b9d8-1c63e36d5e62": true, // Potential Binary Or Script Dropper Via PowerShell
	"3c1b5fb0-c72f-45ba-abd1-4d4c353144ab": true, // Process Creation Using Sysnative Folder
	"3037d961-21e9-4732-b27a-637bcc7bf539": true, // Suspicious High IntegrityLevel Conhost Legacy Option
	"e4a6b256-3e47-40fc-89d2-7a477edd6915": true, // System File Execution Location Anomaly
	"2267fe65-0681-42ad-9a6d-46553d3f3480": true, // WSL Child Process Anomaly
	"b8fd0e93-ff58-4cbd-8f48-1c114e342e62": true, // Windows Binaries Write Suspicious Extensions
}

// SigmaHQHandler manages the SigmaHQ local integration.
// The server keeps a local git clone of the SigmaHQ repo, periodically
// pulls updates, and converts rules on demand when users enable the integration.
type SigmaHQHandler struct {
	rules    store.RuleStore
	macros   store.MacroStore
	accounts store.AccountStore
	orgs     store.OrgStore
	audit    store.AuditStore
	users    store.UserStore
	repoPath string
	mu       sync.Mutex
}

// NewSigmaHQHandler creates a new SigmaHQ integration handler.
func NewSigmaHQHandler(rules store.RuleStore, macros store.MacroStore, accounts store.AccountStore, orgs store.OrgStore, audit store.AuditStore, users store.UserStore, repoPath string) *SigmaHQHandler {
	return &SigmaHQHandler{
		rules:    rules,
		macros:   macros,
		accounts: accounts,
		orgs:     orgs,
		audit:    audit,
		users:    users,
		repoPath: repoPath,
	}
}

// SigmaHQStatus is the response for the status endpoint.
type SigmaHQStatus struct {
	Enabled      bool   `json:"enabled"`
	Available    bool   `json:"available"`
	RuleCount    int    `json:"rule_count"`
	LastCommit   string `json:"last_commit,omitempty"`
	LastUpdated  string `json:"last_updated,omitempty"`
	RepoPath     string `json:"repo_path,omitempty"`
	TotalFiles   int    `json:"total_files"`
}

// SigmaHQSyncResult holds the outcome of a SigmaHQ sync operation.
type SigmaHQSyncResult struct {
	Converted int      `json:"converted"`
	Skipped   int      `json:"skipped"`
	Failed    int      `json:"failed"`
	Invalid   int      `json:"invalid"`
	Errors    []string `json:"errors,omitempty"`
	Duration  string   `json:"duration"`
}

// Status handles GET /api/v1/sigmahq/status
func (h *SigmaHQHandler) Status(w http.ResponseWriter, r *http.Request) {
	accountID := ctxutil.AccountIDFromContext(r.Context())

	status := SigmaHQStatus{
		Available: h.isRepoAvailable(),
		RepoPath:  h.repoPath,
	}

	// Check if enabled for this account
	if accountID != "" && h.accounts != nil {
		account, err := h.accounts.Get(r.Context(), accountID)
		if err == nil && account != nil {
			status.Enabled = account.SigmaHQEnabled
		}
	}

	// Count existing SigmaHQ rules
	if h.rules != nil {
		orgIDs := h.getAccountOrgIDs(r.Context(), accountID)
		for _, oid := range orgIDs {
			count, _ := h.rules.CountBySource(r.Context(), oid, sigmahqSource)
			status.RuleCount += count
			break // Just count for first org, they're the same across orgs
		}
	}

	// Get repo info
	if status.Available {
		status.LastCommit = h.getLastCommit()
		status.LastUpdated = h.getLastPullTime()
		status.TotalFiles = h.countRuleFiles()
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: status})
}

// Enable handles POST /api/v1/sigmahq/enable
func (h *SigmaHQHandler) Enable(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	accountID := ctxutil.AccountIDFromContext(r.Context())
	userID := ctxutil.UserIDFromContext(r.Context())

	if !h.isRepoAvailable() {
		writeError(w, http.StatusServiceUnavailable, "SigmaHQ repository not available on server")
		return
	}

	// Update account setting
	if err := h.setSigmaHQEnabled(r.Context(), accountID, true); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update setting: "+err.Error())
		return
	}

	// Convert and sync rules to all orgs
	result := h.syncRulesToAccount(r.Context(), accountID)

	logAudit(r, h.audit, h.users, userID, "", "update", "sigmahq", "", "SigmaHQ enabled", result)
	log.Infof("fleet: SigmaHQ enabled for account %s: %d converted, %d skipped, %d failed",
		accountID, result.Converted, result.Skipped, result.Failed)

	writeJSON(w, http.StatusOK, fleet.Response{Data: result})
}

// Disable handles POST /api/v1/sigmahq/disable
func (h *SigmaHQHandler) Disable(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	accountID := ctxutil.AccountIDFromContext(r.Context())
	userID := ctxutil.UserIDFromContext(r.Context())

	// Update account setting
	if err := h.setSigmaHQEnabled(r.Context(), accountID, false); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update setting: "+err.Error())
		return
	}

	// Remove all SigmaHQ rules from all orgs
	orgIDs := h.getAccountOrgIDs(r.Context(), accountID)
	totalDeleted := 0
	for _, oid := range orgIDs {
		deleted, err := h.rules.DeleteBySource(r.Context(), oid, sigmahqSource)
		if err != nil {
			log.Warnf("fleet: failed to delete sigmahq rules from org %s: %v", oid, err)
		}
		totalDeleted += deleted
	}

	logAudit(r, h.audit, h.users, userID, "", "update", "sigmahq", "", "SigmaHQ disabled",
		map[string]int{"deleted": totalDeleted})
	log.Infof("fleet: SigmaHQ disabled for account %s: %d rules deleted", accountID, totalDeleted)

	writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]interface{}{
		"deleted": totalDeleted,
	}})
}

// Refresh handles POST /api/v1/sigmahq/refresh
// Pulls latest from SigmaHQ and re-syncs if enabled.
func (h *SigmaHQHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	accountID := ctxutil.AccountIDFromContext(r.Context())
	userID := ctxutil.UserIDFromContext(r.Context())

	if !h.isRepoAvailable() {
		writeError(w, http.StatusServiceUnavailable, "SigmaHQ repository not available on server")
		return
	}

	// Git pull
	if err := h.gitPull(); err != nil {
		writeError(w, http.StatusInternalServerError, "git pull failed: "+err.Error())
		return
	}

	// Re-sync if enabled
	account, _ := h.accounts.Get(r.Context(), accountID)
	if account == nil || !account.SigmaHQEnabled {
		writeJSON(w, http.StatusOK, fleet.Response{Data: map[string]string{"status": "updated, not enabled"}})
		return
	}

	result := h.syncRulesToAccount(r.Context(), accountID)
	logAudit(r, h.audit, h.users, userID, "", "execute", "sigmahq", "", "SigmaHQ refresh", result)

	writeJSON(w, http.StatusOK, fleet.Response{Data: result})
}

// syncRulesToAccount converts all local SigmaHQ rules and syncs to all orgs in the account.
func (h *SigmaHQHandler) syncRulesToAccount(ctx context.Context, accountID string) *SigmaHQSyncResult {
	start := time.Now()
	result := &SigmaHQSyncResult{}

	rulesPath := filepath.Join(h.repoPath, sigmahqRulesDir)
	if _, err := os.Stat(rulesPath); os.IsNotExist(err) {
		result.Errors = append(result.Errors, "rules directory not found: "+rulesPath)
		return result
	}

	// Get target orgs
	orgIDs := h.getAccountOrgIDs(ctx, accountID)
	if len(orgIDs) == 0 {
		result.Errors = append(result.Errors, "no organizations found for account")
		return result
	}

	// Load macros from first org for validation
	macros := h.loadMacros(ctx, orgIDs[0])

	// Walk the rules directory and convert each YAML file
	var convertedRules []fleet.Rule
	syncedIDs := make([]string, 0)

	err := filepath.Walk(rulesPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".yml") && !strings.HasSuffix(info.Name(), ".yaml") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			result.Failed++
			return nil
		}

		// Convert SIGMA rule
		convResult := sigma.Convert(content)
		if !convResult.Success {
			if convResult.Unconvertible {
				result.Skipped++
			} else {
				result.Failed++
				if len(result.Errors) < 20 { // cap error list
					result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", info.Name(), strings.Join(convResult.Errors, "; ")))
				}
			}
			return nil
		}

		// Parse converted YAML into rule struct
		var rule fleet.Rule
		if err := parseYAMLRule([]byte(convResult.FibratusYAML), &rule); err != nil {
			result.Failed++
			return nil
		}

		// Validate condition
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
		}

		rule.RawYAML = convResult.FibratusYAML
		rule.Source = sigmahqSource

		// Disable known-noisy rules by default — they fire on virtually
		// every Windows system and generate excessive false positives.
		if noisySigmaRules[rule.ID] {
			rule.Enabled = false
		}

		convertedRules = append(convertedRules, rule)
		syncedIDs = append(syncedIDs, rule.ID)
		result.Converted++

		return nil
	})
	if err != nil {
		result.Errors = append(result.Errors, "walk error: "+err.Error())
	}

	// Apply to all orgs (upsert + clean sync)
	for _, orgID := range orgIDs {
		for i := range convertedRules {
			rule := convertedRules[i]
			rule.OrgID = orgID
			existing, _ := h.rules.Get(ctx, orgID, rule.ID)
			if existing != nil {
				rule.Enabled = existing.Enabled // preserve user's toggle
				h.rules.Update(ctx, &rule)
			} else {
				h.rules.Create(ctx, &rule)
			}
		}
		// Clean: remove SigmaHQ rules no longer in the repo
		h.rules.DeleteBySourceExcept(ctx, orgID, sigmahqSource, syncedIDs)
	}

	result.Duration = time.Since(start).String()
	return result
}

// StartBackgroundUpdater runs a periodic git pull to keep the local SigmaHQ clone fresh.
// If any account has SigmaHQ enabled and the repo has new commits, it re-syncs.
func (h *SigmaHQHandler) StartBackgroundUpdater(ctx context.Context) {
	go func() {
		// Initial clone if needed
		if !h.isRepoAvailable() {
			log.Info("fleet: SigmaHQ repo not found, cloning...")
			if err := h.gitClone(); err != nil {
				log.Errorf("fleet: failed to clone SigmaHQ repo: %v", err)
			} else {
				log.Info("fleet: SigmaHQ repo cloned successfully")
			}
		}

		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !h.isRepoAvailable() {
					continue
				}
				oldCommit := h.getLastCommit()
				if err := h.gitPull(); err != nil {
					log.Warnf("fleet: SigmaHQ git pull failed: %v", err)
					continue
				}
				newCommit := h.getLastCommit()
				if oldCommit != newCommit {
					log.Infof("fleet: SigmaHQ updated from %s to %s, re-syncing enabled accounts", oldCommit[:8], newCommit[:8])
					h.resyncAllEnabledAccounts(ctx)
				}
			}
		}
	}()
}

// resyncAllEnabledAccounts re-syncs SigmaHQ rules for all accounts that have it enabled.
func (h *SigmaHQHandler) resyncAllEnabledAccounts(ctx context.Context) {
	if githubSyncDB == nil {
		return
	}
	rows, err := githubSyncDB.QueryContext(ctx, `SELECT id FROM accounts WHERE sigmahq_enabled = true`)
	if err != nil {
		log.Warnf("fleet: failed to query sigmahq-enabled accounts: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var accountID string
		if rows.Scan(&accountID) == nil {
			h.mu.Lock()
			result := h.syncRulesToAccount(ctx, accountID)
			h.mu.Unlock()
			log.Infof("fleet: SigmaHQ re-sync for account %s: %d converted, %d skipped", accountID, result.Converted, result.Skipped)
		}
	}
}

// ═══════════════════════════════════════════════════════════════
// Helper methods
// ═══════════════════════════════════════════════════════════════

func (h *SigmaHQHandler) isRepoAvailable() bool {
	gitDir := filepath.Join(h.repoPath, ".git")
	_, err := os.Stat(gitDir)
	return err == nil
}

func (h *SigmaHQHandler) gitClone() error {
	cmd := exec.Command("git", "clone", "--depth=1", sigmahqRepoURL, h.repoPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %s: %w", string(out), err)
	}
	return nil
}

func (h *SigmaHQHandler) gitPull() error {
	cmd := exec.Command("git", "-C", h.repoPath, "pull", "--ff-only")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull failed: %s: %w", string(out), err)
	}
	return nil
}

func (h *SigmaHQHandler) getLastCommit() string {
	cmd := exec.Command("git", "-C", h.repoPath, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (h *SigmaHQHandler) getLastPullTime() string {
	fetchHead := filepath.Join(h.repoPath, ".git", "FETCH_HEAD")
	info, err := os.Stat(fetchHead)
	if err != nil {
		return ""
	}
	return info.ModTime().UTC().Format(time.RFC3339)
}

func (h *SigmaHQHandler) countRuleFiles() int {
	count := 0
	rulesPath := filepath.Join(h.repoPath, sigmahqRulesDir)
	filepath.Walk(rulesPath, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && (strings.HasSuffix(info.Name(), ".yml") || strings.HasSuffix(info.Name(), ".yaml")) {
			count++
		}
		return nil
	})
	return count
}

func (h *SigmaHQHandler) setSigmaHQEnabled(ctx context.Context, accountID string, enabled bool) error {
	if githubSyncDB == nil {
		return fmt.Errorf("database not available")
	}
	_, err := githubSyncDB.ExecContext(ctx,
		`UPDATE accounts SET sigmahq_enabled = $2, updated_at = NOW() WHERE id = $1`,
		accountID, enabled)
	return err
}

func (h *SigmaHQHandler) getAccountOrgIDs(ctx context.Context, accountID string) []string {
	if h.orgs == nil || accountID == "" {
		return nil
	}
	orgList, err := h.orgs.ListByAccount(ctx, accountID)
	if err != nil || len(orgList) == 0 {
		return nil
	}
	ids := make([]string, len(orgList))
	for i, o := range orgList {
		ids[i] = o.ID
	}
	return ids
}

func (h *SigmaHQHandler) loadMacros(ctx context.Context, orgID string) map[string]*qlparser.Macro {
	if orgID == "" || h.macros == nil {
		return nil
	}
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
