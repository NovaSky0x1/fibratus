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
	"os"
	"path/filepath"
	"strings"
	"time"

	pb "github.com/rabbitstack/fibratus/pkg/fleet/pb"
	log "github.com/sirupsen/logrus"
)

// RuleSyncCallback is called when rules are updated from the server.
// The path points to the directory containing the downloaded rule files.
type RuleSyncCallback func(rulesDir string) error

// StartRuleSync opens a persistent gRPC stream to receive rule updates
// pushed by the server. Falls back to periodic polling on stream errors.
func (c *Client) StartRuleSync(onUpdate RuleSyncCallback) {
	c.mu.Lock()
	c.ruleSyncCallback = onUpdate
	c.mu.Unlock()

	c.wg.Add(1)
	go c.ruleStreamLoop(onUpdate)
}

// ruleStreamLoop maintains a persistent SubscribeRules stream.
// On disconnect, it reconnects with exponential backoff.
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

		err := c.subscribeRules(onUpdate)
		if err != nil {
			log.Warnf("fleet: rule stream error: %v (reconnecting in %s)", err, backoff)
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

	// Load current rules version from disk
	currentVersion := loadFile(filepath.Join(c.dataDir, "rules-etag"))

	stream, err := c.agentClient.SubscribeRules(c.grpcCtx(), &pb.RuleSubscription{
		AgentId:        agentID,
		OrgId:          orgID,
		CurrentVersion: currentVersion,
	})
	if err != nil {
		return fmt.Errorf("open rule stream: %w", err)
	}

	log.Info("fleet: rule subscription stream opened")

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

// applyRuleUpdate writes rules and macros to disk and triggers the callback.
func (c *Client) applyRuleUpdate(update *pb.RuleUpdate, onUpdate RuleSyncCallback) error {
	rulesDir := filepath.Join(c.dataDir, "rules")
	macrosDir := filepath.Join(rulesDir, "Macros")
	os.MkdirAll(rulesDir, 0o755)
	os.MkdirAll(macrosDir, 0o755)

	// Remove old fleet rule files
	oldRules, _ := filepath.Glob(filepath.Join(rulesDir, "fleet-rule-*.yml"))
	for _, f := range oldRules {
		os.Remove(f)
	}

	// Write macros
	if len(update.MacrosYaml) > 0 {
		if err := os.WriteFile(filepath.Join(macrosDir, "macros.yml"), update.MacrosYaml, 0o644); err != nil {
			return fmt.Errorf("write macros: %w", err)
		}
		log.Infof("fleet: wrote macros (%d bytes)", len(update.MacrosYaml))
	}

	// Split rules by YAML document separator and write each as a separate file
	var ruleDocs []string
	docs := strings.Split(string(update.RulesYaml), "\n---\n")
	for _, doc := range docs {
		trimmed := strings.TrimSpace(doc)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "- macro:") {
			// Macro snuck into rules YAML — write to macros file
			var buf bytes.Buffer
			buf.WriteString(doc)
			buf.WriteString("\n")
			os.WriteFile(filepath.Join(macrosDir, "macros.yml"), buf.Bytes(), 0o644)
		} else if strings.HasPrefix(trimmed, "name:") {
			ruleDocs = append(ruleDocs, doc)
		}
	}

	for i, doc := range ruleDocs {
		ruleFile := filepath.Join(rulesDir, fmt.Sprintf("fleet-rule-%03d.yml", i+1))
		if err := os.WriteFile(ruleFile, []byte(doc), 0o644); err != nil {
			return fmt.Errorf("write rule %d: %w", i+1, err)
		}
	}
	log.Infof("fleet: wrote %d rule files (version: %s)", len(ruleDocs), update.Version)

	// Save version for next reconnect
	if update.Version != "" {
		os.WriteFile(filepath.Join(c.dataDir, "rules-etag"), []byte(update.Version), 0o644)
	}

	// Trigger callback
	if onUpdate != nil {
		if err := onUpdate(rulesDir); err != nil {
			return fmt.Errorf("rule update callback: %w", err)
		}
	}

	return nil
}

// RulesDir returns the path where fleet-synced rules are cached.
func (c *Client) RulesDir() string {
	return filepath.Join(c.dataDir, "rules")
}
