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

package store

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// bufferedEvent holds a parsed event plus its routing metadata so
// the flush goroutine can insert it in one large batch.
type bufferedEvent struct {
	OrgID    string
	AgentID  string
	Hostname string
	Raw      json.RawMessage
}

// BufferedTelemetryStore wraps a TelemetryStore and accumulates events
// from all agents in an in-memory buffer. A background goroutine
// flushes the buffer to the underlying store periodically or when
// the buffer reaches a threshold. This converts many small per-agent
// HTTP batches (~25 events each) into large ClickHouse-friendly
// batches (10K-50K+ events).
type BufferedTelemetryStore struct {
	store TelemetryStore

	mu     sync.Mutex
	buf    []bufferedEvent
	stopCh chan struct{}
	done   chan struct{}

	// Tuning knobs
	flushInterval time.Duration
	flushSize     int
}

// BufferConfig configures the telemetry buffer.
type BufferConfig struct {
	FlushInterval time.Duration // How often to flush (default 2s)
	FlushSize     int           // Max events before forced flush (default 50000)
}

// NewBufferedTelemetryStore creates a buffered wrapper around a TelemetryStore.
// Call Start() to begin the flush goroutine, and Stop() on shutdown.
func NewBufferedTelemetryStore(store TelemetryStore, cfg BufferConfig) *BufferedTelemetryStore {
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 2 * time.Second
	}
	if cfg.FlushSize <= 0 {
		cfg.FlushSize = 50000
	}
	return &BufferedTelemetryStore{
		store:         store,
		buf:           make([]bufferedEvent, 0, cfg.FlushSize),
		stopCh:        make(chan struct{}),
		done:          make(chan struct{}),
		flushInterval: cfg.FlushInterval,
		flushSize:     cfg.FlushSize,
	}
}

// Start begins the background flush goroutine.
func (b *BufferedTelemetryStore) Start() {
	go b.flushLoop()
	log.Infof("fleet: telemetry buffer started (flush every %v or %d events)", b.flushInterval, b.flushSize)
}

// Stop signals the flush goroutine to drain and exit.
func (b *BufferedTelemetryStore) Stop() {
	close(b.stopCh)
	<-b.done
}

// BulkIngest adds events to the buffer. Returns immediately.
func (b *BufferedTelemetryStore) BulkIngest(ctx context.Context, orgID, agentID, hostname string, events []json.RawMessage) error {
	b.mu.Lock()
	for _, raw := range events {
		b.buf = append(b.buf, bufferedEvent{
			OrgID:    orgID,
			AgentID:  agentID,
			Hostname: hostname,
			Raw:      raw,
		})
	}
	shouldFlush := len(b.buf) >= b.flushSize
	b.mu.Unlock()

	// Flush immediately if we hit the size threshold.
	if shouldFlush {
		b.flush()
	}
	return nil
}

// Search delegates directly to the underlying store.
func (b *BufferedTelemetryStore) Search(ctx context.Context, orgID string, opts TelemetrySearchOpts) ([]TelemetryEvent, int, error) {
	return b.store.Search(ctx, orgID, opts)
}

// GetLatestForAgent delegates directly to the underlying store.
func (b *BufferedTelemetryStore) GetLatestForAgent(ctx context.Context, orgID, agentID string, limit int) ([]TelemetryEvent, error) {
	return b.store.GetLatestForAgent(ctx, orgID, agentID, limit)
}

// Purge delegates directly to the underlying store.
func (b *BufferedTelemetryStore) Purge(ctx context.Context, retentionDays int) (int64, error) {
	return b.store.Purge(ctx, retentionDays)
}

// CountByAgent delegates directly to the underlying store.
func (b *BufferedTelemetryStore) CountByAgent(ctx context.Context, orgID string) (map[string]int64, error) {
	return b.store.CountByAgent(ctx, orgID)
}

func (b *BufferedTelemetryStore) GetFieldValues(ctx context.Context, orgID string) (map[string][]string, error) {
	return b.store.GetFieldValues(ctx, orgID)
}

func (b *BufferedTelemetryStore) flushLoop() {
	defer close(b.done)
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.flush()
		case <-b.stopCh:
			// Final drain
			b.flush()
			return
		}
	}
}

func (b *BufferedTelemetryStore) flush() {
	b.mu.Lock()
	if len(b.buf) == 0 {
		b.mu.Unlock()
		return
	}
	// Swap buffer so we don't hold the lock during the insert.
	batch := b.buf
	b.buf = make([]bufferedEvent, 0, b.flushSize)
	b.mu.Unlock()

	// Group events by org+agent for the underlying store's BulkIngest.
	type key struct {
		orgID, agentID, hostname string
	}
	groups := make(map[key][]json.RawMessage)
	for _, evt := range batch {
		k := key{evt.OrgID, evt.AgentID, evt.Hostname}
		groups[k] = append(groups[k], evt.Raw)
	}

	start := time.Now()
	total := 0
	for k, events := range groups {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := b.store.BulkIngest(ctx, k.orgID, k.agentID, k.hostname, events); err != nil {
			log.Errorf("fleet: buffer flush error (agent=%s, events=%d): %v", k.agentID, len(events), err)
		}
		total += len(events)
		cancel()
	}

	elapsed := time.Since(start)
	if total > 0 {
		log.Infof("fleet: buffer flushed %d events from %d agents in %v", total, len(groups), elapsed)
	}
}
