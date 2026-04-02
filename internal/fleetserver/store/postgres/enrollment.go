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

	"github.com/rabbitstack/fibratus/pkg/fleet"
)

// EnrollmentTokenStore implements store.EnrollmentTokenStore backed by PostgreSQL.
type EnrollmentTokenStore struct {
	db *sql.DB
}

// NewEnrollmentTokenStore creates a new enrollment token store.
func NewEnrollmentTokenStore(db *sql.DB) *EnrollmentTokenStore {
	return &EnrollmentTokenStore{db: db}
}

func (s *EnrollmentTokenStore) Create(ctx context.Context, token *fleet.EnrollmentToken) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO enrollment_tokens (id, account_id, org_id, name, max_uses, uses_count, expires_at, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		token.ID, token.AccountID, token.OrgID, token.Name,
		token.MaxUses, token.UsesCount, token.ExpiresAt, token.CreatedBy, token.CreatedAt,
	)
	return err
}

func (s *EnrollmentTokenStore) Get(ctx context.Context, id string) (*fleet.EnrollmentToken, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT t.id, t.account_id, t.org_id, COALESCE(o.name, '') as org_name,
			t.name, t.max_uses, t.uses_count, t.expires_at, t.created_by, t.created_at
		 FROM enrollment_tokens t
		 LEFT JOIN organizations o ON t.org_id = o.id
		 WHERE t.id = $1`, id)

	t := &fleet.EnrollmentToken{}
	err := row.Scan(
		&t.ID, &t.AccountID, &t.OrgID, &t.OrgName,
		&t.Name, &t.MaxUses, &t.UsesCount, &t.ExpiresAt, &t.CreatedBy, &t.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

func (s *EnrollmentTokenStore) IncrementUses(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE enrollment_tokens SET uses_count = uses_count + 1 WHERE id = $1`, id)
	return err
}

func (s *EnrollmentTokenStore) ListByOrg(ctx context.Context, orgID string) ([]*fleet.EnrollmentToken, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.account_id, t.org_id, COALESCE(o.name, '') as org_name,
			t.name, t.max_uses, t.uses_count, t.expires_at, t.created_by, t.created_at
		 FROM enrollment_tokens t
		 LEFT JOIN organizations o ON t.org_id = o.id
		 WHERE t.org_id = $1
		 ORDER BY t.created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]*fleet.EnrollmentToken, 0)
	for rows.Next() {
		t := &fleet.EnrollmentToken{}
		if err := rows.Scan(
			&t.ID, &t.AccountID, &t.OrgID, &t.OrgName,
			&t.Name, &t.MaxUses, &t.UsesCount, &t.ExpiresAt, &t.CreatedBy, &t.CreatedAt,
		); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}
