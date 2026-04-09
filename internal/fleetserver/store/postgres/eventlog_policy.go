/*
 * Copyright 2021-2026 by Nedim Sabic Sabic
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

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/rabbitstack/fibratus/pkg/fleet"
)

// EventLogPolicyStore implements store.EventLogPolicyStore backed by PostgreSQL.
type EventLogPolicyStore struct {
	db *sql.DB
}

// NewEventLogPolicyStore creates a new event log policy store.
func NewEventLogPolicyStore(db *sql.DB) *EventLogPolicyStore {
	return &EventLogPolicyStore{db: db}
}

// Get returns the event log policy for the specified organization.
func (s *EventLogPolicyStore) Get(ctx context.Context, orgID string) (*fleet.EventLogPolicy, error) {
	var p fleet.EventLogPolicy
	var channelsJSON []byte

	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, enabled, channels, version, created_at, updated_at
		 FROM eventlog_policies WHERE org_id = $1`, orgID).
		Scan(&p.ID, &p.OrgID, &p.Enabled, &channelsJSON, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get eventlog policy: %w", err)
	}
	if err := json.Unmarshal(channelsJSON, &p.Channels); err != nil {
		return nil, fmt.Errorf("unmarshal channels: %w", err)
	}
	return &p, nil
}

// Upsert creates or updates the event log policy for an organization.
func (s *EventLogPolicyStore) Upsert(ctx context.Context, policy *fleet.EventLogPolicy) error {
	channelsJSON, err := json.Marshal(policy.Channels)
	if err != nil {
		return fmt.Errorf("marshal channels: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO eventlog_policies (id, org_id, enabled, channels, version, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		 ON CONFLICT (org_id) DO UPDATE SET
		   enabled = EXCLUDED.enabled,
		   channels = EXCLUDED.channels,
		   version = eventlog_policies.version + 1,
		   updated_at = NOW()`,
		policy.ID, policy.OrgID, policy.Enabled, channelsJSON, 1)
	if err != nil {
		return fmt.Errorf("upsert eventlog policy: %w", err)
	}
	return nil
}
