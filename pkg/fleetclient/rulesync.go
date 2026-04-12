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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	pb "github.com/rabbitstack/fibratus/pkg/fleet/pb"
	"github.com/rabbitstack/fibratus/pkg/fleet/tamper"
	log "github.com/sirupsen/logrus"
)

// RuleSyncCallback is called when rules are updated from the server.
// Rules are provided as in-memory byte slices — never written to disk.
// ruleDocs: individual YAML rule documents, macrosYAML: combined macros YAML.
type RuleSyncCallback func(ruleDocs [][]byte, macrosYAML []byte) error

// EncryptedRuleStore holds rules encrypted in memory using DPAPI.
// Rules are decrypted only when passed to the rule compiler.
type EncryptedRuleStore struct {
	mu           sync.RWMutex
	encRuleDocs  [][]byte // each entry is a DPAPI-encrypted rule YAML
	encMacros    []byte   // DPAPI-encrypted macros YAML
	version      string
	ruleCount    int
}

// NewEncryptedRuleStore creates a new in-memory encrypted rule store.
func NewEncryptedRuleStore() *EncryptedRuleStore {
	return &EncryptedRuleStore{}
}

// Store encrypts and stores rule documents and macros in memory.
func (s *EncryptedRuleStore) Store(ruleDocs [][]byte, macrosYAML []byte, version string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.encRuleDocs = make([][]byte, len(ruleDocs))
	for i, doc := range ruleDocs {
		s.encRuleDocs[i] = protectMemory(doc)
	}
	if len(macrosYAML) > 0 {
		s.encMacros = protectMemory(macrosYAML)
	} else {
		s.encMacros = nil
	}
	s.version = version
	s.ruleCount = len(ruleDocs)
}

// Decrypt returns decrypted copies of the rules and macros.
// The caller should zero the returned slices after use.
func (s *EncryptedRuleStore) Decrypt() (ruleDocs [][]byte, macrosYAML []byte) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ruleDocs = make([][]byte, len(s.encRuleDocs))
	for i, enc := range s.encRuleDocs {
		ruleDocs[i] = unprotectMemory(enc)
	}
	if s.encMacros != nil {
		macrosYAML = unprotectMemory(s.encMacros)
	}
	return
}

// ClearRawDocs zeroes and releases the encrypted rule YAML docs from memory.
// Call this after successful compilation — the compiled ASTs in the rule engine
// are all the agent needs. The encrypted macros are kept (small, needed for recompile).
func (s *EncryptedRuleStore) ClearRawDocs() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.encRuleDocs {
		for j := range s.encRuleDocs[i] {
			s.encRuleDocs[i][j] = 0
		}
		s.encRuleDocs[i] = nil
	}
	s.encRuleDocs = nil
}

// Version returns the current ruleset version hash.
func (s *EncryptedRuleStore) Version() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.version
}

// RuleCount returns the number of stored rules.
func (s *EncryptedRuleStore) RuleCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ruleCount
}

// StartRuleSync opens a persistent gRPC stream to receive rule updates
// pushed by the server. Rules are stored encrypted in memory — never on disk.
func (c *Client) StartRuleSync(onUpdate RuleSyncCallback) {
	c.mu.Lock()
	c.ruleSyncCallback = onUpdate
	c.mu.Unlock()

	c.wg.Add(1)
	go c.ruleStreamLoop(onUpdate)
}

// ruleStreamLoop maintains a persistent SubscribeRules stream.
func (c *Client) ruleStreamLoop(onUpdate RuleSyncCallback) {
	defer c.wg.Done()

	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		start := time.Now()
		err := c.subscribeRules(onUpdate)
		elapsed := time.Since(start)
		if err != nil {
			log.Warnf("fleet: rule stream error: %v (reconnecting in %s)", err, backoff)
		}

		// Reset backoff if the stream ran for more than 30s (successful connection)
		if elapsed > 30*time.Second {
			backoff = time.Second
		}

		select {
		case <-time.After(backoff):
			backoff = backoff * 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		case <-c.stopCh:
			return
		}
	}
}

// subscribeRules opens one SubscribeRules stream and processes updates until error.
func (c *Client) subscribeRules(onUpdate RuleSyncCallback) error {
	c.mu.RLock()
	agentID := c.agentID
	orgID := c.orgID
	c.mu.RUnlock()

	currentVersion := tamper.LoadState("RulesETag")
	if currentVersion == "" {
		currentVersion = loadFile(filepath.Join(c.dataDir, "rules-etag")) // legacy fallback
	}

	stream, err := c.agentClient.SubscribeRules(c.grpcCtx(), &pb.RuleSubscription{
		AgentId:        agentID,
		OrgId:          orgID,
		CurrentVersion: currentVersion,
	})
	if err != nil {
		return fmt.Errorf("open rule stream: %w", err)
	}

	log.Info("fleet: rule subscription stream opened (in-memory mode)")

	for {
		update, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("receive rule update: %w", err)
		}

		if err := c.applyRuleUpdate(update, onUpdate); err != nil {
			log.Errorf("fleet: failed to apply rule update: %v", err)
		}
	}
}

// applyRuleUpdate parses the YAML, stores rules encrypted in memory,
// and triggers the callback. No files are written to disk.
func (c *Client) applyRuleUpdate(update *pb.RuleUpdate, onUpdate RuleSyncCallback) error {
	// Parse rule documents from YAML
	var ruleDocs [][]byte
	docs := strings.Split(string(update.RulesYaml), "\n---\n")
	for _, doc := range docs {
		trimmed := strings.TrimSpace(doc)
		if trimmed == "" {
			continue
		}
		// Skip macros that leaked into rules YAML
		if strings.HasPrefix(trimmed, "- macro:") {
			continue
		}
		if strings.HasPrefix(trimmed, "name:") {
			ruleDocs = append(ruleDocs, []byte(doc))
		}
	}

	macrosYAML := update.MacrosYaml

	log.Infof("fleet: received %d rules (version: %s) — loaded to encrypted memory, no disk", len(ruleDocs), update.Version)

	// Save only the version etag to disk (not the rules themselves)
	if update.Version != "" {
		tamper.StoreState("RulesETag", update.Version)
		os.Remove(filepath.Join(c.dataDir, "rules-etag")) // clean up legacy file
	}

	// Clean up any old rule files that may exist from previous versions
	cleanupLegacyRuleFiles(c.dataDir)

	// Trigger callback with in-memory rules
	if onUpdate != nil {
		if err := onUpdate(ruleDocs, macrosYAML); err != nil {
			return fmt.Errorf("rule update callback: %w", err)
		}
	}

	return nil
}

// cleanupLegacyRuleFiles removes any rule files written by older agent versions.
func cleanupLegacyRuleFiles(dataDir string) {
	rulesDir := filepath.Join(dataDir, "rules")
	files, _ := filepath.Glob(filepath.Join(rulesDir, "fleet-rule-*.yml"))
	for _, f := range files {
		os.Remove(f)
	}
	os.Remove(filepath.Join(rulesDir, "fleet-rules.yml"))
	// Remove macros subdirectory
	os.RemoveAll(filepath.Join(rulesDir, "Macros"))
	// Remove empty rules directory
	os.Remove(rulesDir)
}

// RulesDir is kept for backward compatibility but returns empty string
// in memory-only mode.
func (c *Client) RulesDir() string {
	return ""
}
