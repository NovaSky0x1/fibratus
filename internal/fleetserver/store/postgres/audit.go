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

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/rabbitstack/fibratus/pkg/fleet"
)

// AuditStore implements store.AuditStore using PostgreSQL.
type AuditStore struct {
	db *sql.DB
}

// NewAuditStore creates a new PostgreSQL audit store.
func NewAuditStore(db *sql.DB) *AuditStore {
	return &AuditStore{db: db}
}

func (s *AuditStore) Log(ctx context.Context, entry *fleet.AuditEntry) error {
	details, _ := json.Marshal(entry.Details)
	if string(details) == "null" {
		details = []byte("{}")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_log (id, org_id, user_id, user_email, action, resource_type, resource_id, resource_name, details, ip_address, timestamp)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())`,
		entry.ID, entry.OrgID, entry.UserID, entry.UserEmail, entry.Action,
		entry.ResourceType, entry.ResourceID, entry.ResourceName, details, entry.IPAddress,
	)
	return err
}

func (s *AuditStore) List(ctx context.Context, orgID string, opts fleet.ListOptions) ([]*fleet.AuditEntry, int, error) {
	var total int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE org_id=$1`, orgID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	limit := opts.PerPage
	if limit <= 0 {
		limit = 50
	}
	offset := (opts.Page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, user_id, user_email, action, resource_type, resource_id, resource_name, details, ip_address, timestamp
		 FROM audit_log WHERE org_id=$1 ORDER BY timestamp DESC LIMIT $2 OFFSET $3`,
		orgID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []*fleet.AuditEntry
	for rows.Next() {
		e := &fleet.AuditEntry{}
		var details []byte
		if err := rows.Scan(&e.ID, &e.OrgID, &e.UserID, &e.UserEmail, &e.Action,
			&e.ResourceType, &e.ResourceID, &e.ResourceName, &details, &e.IPAddress, &e.Timestamp); err != nil {
			return nil, 0, err
		}
		e.Details = details
		entries = append(entries, e)
	}
	return entries, total, nil
}
