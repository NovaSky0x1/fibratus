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
	"errors"
	"fmt"

	"github.com/rabbitstack/fibratus/internal/fleetserver/secrets"
)

// SecretStore persists encrypted server-wide secrets in Postgres. Values are
// always sealed before they touch the DB and opened on read, so nothing in the
// underlying table is human-readable without the master key.
type SecretStore struct {
	db  *sql.DB
	enc secrets.Encryptor
}

// NewSecretStore constructs the store. enc must be a fully initialised
// Encryptor — pass nil only in tests, in which case Get/Set will panic.
func NewSecretStore(db *sql.DB, enc secrets.Encryptor) *SecretStore {
	return &SecretStore{db: db, enc: enc}
}

// ErrSecretNotFound is returned when no row exists for the requested name.
var ErrSecretNotFound = errors.New("secret not found")

// Get returns the decrypted value. ErrSecretNotFound when no row exists.
func (s *SecretStore) Get(ctx context.Context, name string) (string, error) {
	var sealed []byte
	err := s.db.QueryRowContext(ctx, `SELECT value_encrypted FROM system_secrets WHERE name = $1`, name).Scan(&sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrSecretNotFound
	}
	if err != nil {
		return "", fmt.Errorf("secrets: read %s: %w", name, err)
	}
	plaintext, err := s.enc.Decrypt(sealed)
	if err != nil {
		return "", fmt.Errorf("secrets: decrypt %s: %w", name, err)
	}
	return string(plaintext), nil
}

// GetOr returns the decrypted value or fallback if the secret is not stored.
// Errors other than ErrSecretNotFound are surfaced.
func (s *SecretStore) GetOr(ctx context.Context, name, fallback string) (string, error) {
	v, err := s.Get(ctx, name)
	if errors.Is(err, ErrSecretNotFound) {
		return fallback, nil
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

// Set writes (or overwrites) the secret. updatedBy is recorded for audit.
func (s *SecretStore) Set(ctx context.Context, name, value, updatedBy string) error {
	sealed, err := s.enc.Encrypt([]byte(value))
	if err != nil {
		return fmt.Errorf("secrets: encrypt %s: %w", name, err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO system_secrets (name, value_encrypted, updated_at, updated_by)
		VALUES ($1, $2, NOW(), $3)
		ON CONFLICT (name) DO UPDATE
		SET value_encrypted = EXCLUDED.value_encrypted,
		    updated_at = NOW(),
		    updated_by = EXCLUDED.updated_by
	`, name, sealed, updatedBy)
	if err != nil {
		return fmt.Errorf("secrets: write %s: %w", name, err)
	}
	return nil
}

// Delete removes the secret. Not-found is not an error.
func (s *SecretStore) Delete(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM system_secrets WHERE name = $1`, name)
	if err != nil {
		return fmt.Errorf("secrets: delete %s: %w", name, err)
	}
	return nil
}

// Has returns true if a row exists for name. Cheaper than Get when the caller
// only needs to know whether something is configured (e.g. for status badges).
func (s *SecretStore) Has(ctx context.Context, name string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM system_secrets WHERE name = $1)`, name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("secrets: exists %s: %w", name, err)
	}
	return exists, nil
}
