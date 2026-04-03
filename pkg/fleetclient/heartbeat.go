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
	"time"

	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// HeartbeatCollector provides runtime metrics for inclusion in heartbeat payloads.
type HeartbeatCollector interface {
	// RulesVersion returns a hash identifying the currently active ruleset.
	RulesVersion() string
	// ActiveRules returns the number of compiled rules.
	ActiveRules() int
}

// StartHeartbeat begins sending periodic heartbeats to the fleet server.
// It blocks until the client is closed.
func (c *Client) StartHeartbeat(collector HeartbeatCollector) {
	c.wg.Add(1)
	go c.heartbeatLoop(collector)
}

func (c *Client) heartbeatLoop(collector HeartbeatCollector) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.HeartbeatInterval)
	defer ticker.Stop()

	// Send initial heartbeat immediately
	c.sendHeartbeat(collector)

	for {
		select {
		case <-ticker.C:
			c.sendHeartbeat(collector)
		case <-c.stopCh:
			log.Info("fleet: heartbeat stopped")
			return
		}
	}
}

func (c *Client) sendHeartbeat(collector HeartbeatCollector) {
	hb := fleet.Heartbeat{
		Timestamp: time.Now().UTC(),
	}
	if collector != nil {
		hb.RulesVersion = collector.RulesVersion()
		hb.ActiveRules = collector.ActiveRules()
	}
	if err := c.SendHeartbeat(hb); err != nil {
		log.Warnf("fleet: heartbeat failed: %v", err)
	}

	// Check for rule updates on every heartbeat (ETag ensures no-op if unchanged)
	c.mu.RLock()
	cb := c.ruleSyncCallback
	c.mu.RUnlock()
	if cb != nil {
		c.syncRules(cb)
	}
}
