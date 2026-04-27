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
)

// TelemetryHolder is a thin proxy that lets us hot-swap the underlying
// TelemetryStore without restarting handlers. Every interface method is
// forwarded to the current store under an RLock, so reads and ingest stay
// concurrent; only Replace takes the write lock for the swap itself.
//
// Use Replace to install a new pipeline (typically: open a new chDB, build a
// fresh chstore + buffered store, Start() it, then Replace and Stop() the
// returned previous store to drain its buffer cleanly).
type TelemetryHolder struct {
	mu      sync.RWMutex
	current TelemetryStore
}

// NewTelemetryHolder wraps an initial store.
func NewTelemetryHolder(initial TelemetryStore) *TelemetryHolder {
	return &TelemetryHolder{current: initial}
}

// Current returns the active store. Useful for callers that need to invoke
// methods that aren't on the TelemetryStore interface (e.g. Stop on a
// BufferedTelemetryStore) — the returned value is a snapshot under RLock.
func (h *TelemetryHolder) Current() TelemetryStore {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.current
}

// Replace atomically swaps the active store and returns the previous one so
// the caller can drain / Close it after the swap.
func (h *TelemetryHolder) Replace(next TelemetryStore) TelemetryStore {
	h.mu.Lock()
	defer h.mu.Unlock()
	prev := h.current
	h.current = next
	return prev
}

// ─── TelemetryStore interface forwarders ────────────────────────────────────

func (h *TelemetryHolder) BulkIngest(ctx context.Context, orgID, agentID, hostname string, events []json.RawMessage) error {
	h.mu.RLock()
	s := h.current
	h.mu.RUnlock()
	return s.BulkIngest(ctx, orgID, agentID, hostname, events)
}

func (h *TelemetryHolder) Search(ctx context.Context, orgID string, opts TelemetrySearchOpts) ([]TelemetryEvent, int, error) {
	h.mu.RLock()
	s := h.current
	h.mu.RUnlock()
	return s.Search(ctx, orgID, opts)
}

func (h *TelemetryHolder) GetLatestForAgent(ctx context.Context, orgID, agentID string, limit int) ([]TelemetryEvent, error) {
	h.mu.RLock()
	s := h.current
	h.mu.RUnlock()
	return s.GetLatestForAgent(ctx, orgID, agentID, limit)
}

func (h *TelemetryHolder) Purge(ctx context.Context, retentionDays int) (int64, error) {
	h.mu.RLock()
	s := h.current
	h.mu.RUnlock()
	return s.Purge(ctx, retentionDays)
}

func (h *TelemetryHolder) CountByAgent(ctx context.Context, orgID string) (map[string]int64, error) {
	h.mu.RLock()
	s := h.current
	h.mu.RUnlock()
	return s.CountByAgent(ctx, orgID)
}

func (h *TelemetryHolder) GetFieldValues(ctx context.Context, orgID string) (map[string][]string, error) {
	h.mu.RLock()
	s := h.current
	h.mu.RUnlock()
	return s.GetFieldValues(ctx, orgID)
}
