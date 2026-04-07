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
	"sync"

	pb "github.com/rabbitstack/fibratus/pkg/fleet/pb"
	log "github.com/sirupsen/logrus"
)

// StreamManager tracks connected agents with open gRPC streams for
// server-push of rules and commands.
type StreamManager struct {
	mu sync.RWMutex

	// agentID → buffered channel for rule updates
	ruleStreams map[string]chan *pb.RuleUpdate
	// agentID → buffered channel for command pushes
	cmdStreams map[string]chan *pb.CommandPush
	// agentID → orgID for org-wide rule broadcasts
	agentOrgs map[string]string
}

// NewStreamManager creates a new stream manager.
func NewStreamManager() *StreamManager {
	return &StreamManager{
		ruleStreams: make(map[string]chan *pb.RuleUpdate),
		cmdStreams:  make(map[string]chan *pb.CommandPush),
		agentOrgs:  make(map[string]string),
	}
}

// RegisterRuleStream registers an agent's rule subscription stream.
// Returns a channel the gRPC handler reads from.
func (sm *StreamManager) RegisterRuleStream(agentID, orgID string) <-chan *pb.RuleUpdate {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Close existing stream if reconnecting
	if old, ok := sm.ruleStreams[agentID]; ok {
		close(old)
	}

	ch := make(chan *pb.RuleUpdate, 8)
	sm.ruleStreams[agentID] = ch
	sm.agentOrgs[agentID] = orgID
	log.Infof("streams: agent %s subscribed to rule updates (org %s)", agentID, orgID)
	return ch
}

// UnregisterRuleStream removes an agent's rule subscription.
func (sm *StreamManager) UnregisterRuleStream(agentID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if ch, ok := sm.ruleStreams[agentID]; ok {
		close(ch)
		delete(sm.ruleStreams, agentID)
	}
	// Clean up org mapping only if no command stream exists either
	if _, hasCmdStream := sm.cmdStreams[agentID]; !hasCmdStream {
		delete(sm.agentOrgs, agentID)
	}
	log.Infof("streams: agent %s unsubscribed from rule updates", agentID)
}

// PushRulesToOrg sends a rule update to all connected agents in the given org.
func (sm *StreamManager) PushRulesToOrg(orgID string, update *pb.RuleUpdate) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	pushed := 0
	for agentID, ch := range sm.ruleStreams {
		if sm.agentOrgs[agentID] == orgID {
			select {
			case ch <- update:
				pushed++
			default:
				log.Warnf("streams: rule push to agent %s dropped (buffer full)", agentID)
			}
		}
	}
	if pushed > 0 {
		log.Infof("streams: pushed rule update to %d agent(s) in org %s", pushed, orgID)
	}
}

// RegisterCommandStream registers an agent's command channel.
func (sm *StreamManager) RegisterCommandStream(agentID, orgID string) <-chan *pb.CommandPush {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if old, ok := sm.cmdStreams[agentID]; ok {
		close(old)
	}

	ch := make(chan *pb.CommandPush, 32)
	sm.cmdStreams[agentID] = ch
	sm.agentOrgs[agentID] = orgID
	log.Infof("streams: agent %s opened command channel (org %s)", agentID, orgID)
	return ch
}

// UnregisterCommandStream removes an agent's command channel.
func (sm *StreamManager) UnregisterCommandStream(agentID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if ch, ok := sm.cmdStreams[agentID]; ok {
		close(ch)
		delete(sm.cmdStreams, agentID)
	}
	if _, hasRuleStream := sm.ruleStreams[agentID]; !hasRuleStream {
		delete(sm.agentOrgs, agentID)
	}
	log.Infof("streams: agent %s closed command channel", agentID)
}

// PushCommand sends a command to a specific agent.
func (sm *StreamManager) PushCommand(agentID string, cmd *pb.CommandPush) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	ch, ok := sm.cmdStreams[agentID]
	if !ok {
		return false
	}

	select {
	case ch <- cmd:
		log.Infof("streams: pushed command %s to agent %s", cmd.Id, agentID)
		return true
	default:
		log.Warnf("streams: command push to agent %s dropped (buffer full)", agentID)
		return false
	}
}

// ConnectedAgents returns the number of agents with at least one active stream.
func (sm *StreamManager) ConnectedAgents() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.agentOrgs)
}

// IsAgentConnected checks if an agent has an active stream.
func (sm *StreamManager) IsAgentConnected(agentID string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	_, ok := sm.agentOrgs[agentID]
	return ok
}
