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
	"time"

	"github.com/rabbitstack/fibratus/pkg/fleet"
)

// APIKeyStore manages user API key persistence in PostgreSQL.
type APIKeyStore struct {
	db *sql.DB
}

// NewAPIKeyStore creates a new API key store.
func NewAPIKeyStore(db *sql.DB) *APIKeyStore {
	return &APIKeyStore{db: db}
}

// Create inserts a new API key record.
func (s *APIKeyStore) Create(ctx context.Context, key *fleet.APIKey) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_api_keys (id, user_id, account_id, name, key_prefix, key_hash, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		key.ID, key.UserID, key.AccountID, key.Name, key.KeyPrefix, key.KeyHash, key.CreatedAt, key.ExpiresAt)
	return err
}

// List returns all API keys for a given user, ordered by creation date descending.
func (s *APIKeyStore) List(ctx context.Context, userID string) ([]*fleet.APIKey, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, account_id, name, key_prefix, created_at, last_used_at, expires_at
		 FROM user_api_keys WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := make([]*fleet.APIKey, 0)
	for rows.Next() {
		k := &fleet.APIKey{}
		var lastUsed, expires sql.NullTime
		if err := rows.Scan(&k.ID, &k.UserID, &k.AccountID, &k.Name, &k.KeyPrefix,
			&k.CreatedAt, &lastUsed, &expires); err != nil {
			return nil, err
		}
		if lastUsed.Valid {
			k.LastUsedAt = &lastUsed.Time
		}
		if expires.Valid {
			k.ExpiresAt = &expires.Time
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// Delete removes an API key by ID, scoped to the owning user.
func (s *APIKeyStore) Delete(ctx context.Context, id, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM user_api_keys WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

// ValidateKey looks up a key by its SHA-256 hash and returns the associated key record.
// Returns nil if not found or expired. Updates last_used_at on successful validation.
func (s *APIKeyStore) ValidateKey(ctx context.Context, keyHash string) (*fleet.APIKey, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, account_id, name, key_prefix, created_at, last_used_at, expires_at
		 FROM user_api_keys WHERE key_hash = $1`, keyHash)

	k := &fleet.APIKey{}
	var lastUsed, expires sql.NullTime
	err := row.Scan(&k.ID, &k.UserID, &k.AccountID, &k.Name, &k.KeyPrefix,
		&k.CreatedAt, &lastUsed, &expires)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if lastUsed.Valid {
		k.LastUsedAt = &lastUsed.Time
	}
	if expires.Valid {
		k.ExpiresAt = &expires.Time
	}

	// Check expiry
	if k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now()) {
		return nil, nil
	}

	// Update last_used_at
	_, _ = s.db.ExecContext(ctx, `UPDATE user_api_keys SET last_used_at = NOW() WHERE id = $1`, k.ID)

	return k, nil
}
