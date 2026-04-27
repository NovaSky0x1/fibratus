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
)

// SettingsStore stores server-wide non-sensitive settings (signup gating,
// feature flags). For sensitive values (passwords, API keys) use SecretStore
// instead — that one encrypts at rest.
type SettingsStore struct {
	db *sql.DB
}

func NewSettingsStore(db *sql.DB) *SettingsStore {
	return &SettingsStore{db: db}
}

// ErrSettingNotFound — no row for the requested name.
var ErrSettingNotFound = errors.New("setting not found")

// Get returns the raw string value or ErrSettingNotFound.
func (s *SettingsStore) Get(ctx context.Context, name string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE name = $1`, name).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrSettingNotFound
	}
	if err != nil {
		return "", fmt.Errorf("settings: read %s: %w", name, err)
	}
	return v, nil
}

// GetBoolOr returns the boolean value of name, or fallback if no row exists.
// Anything that doesn't parse as bool is treated as the fallback.
func (s *SettingsStore) GetBoolOr(ctx context.Context, name string, fallback bool) bool {
	v, err := s.Get(ctx, name)
	if err != nil {
		return fallback
	}
	switch v {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		return fallback
	}
}

// Set writes a setting. updatedBy is recorded for audit.
func (s *SettingsStore) Set(ctx context.Context, name, value, updatedBy string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO system_settings (name, value, updated_at, updated_by)
		VALUES ($1, $2, NOW(), $3)
		ON CONFLICT (name) DO UPDATE
		SET value = EXCLUDED.value,
		    updated_at = NOW(),
		    updated_by = EXCLUDED.updated_by
	`, name, value, updatedBy)
	if err != nil {
		return fmt.Errorf("settings: write %s: %w", name, err)
	}
	return nil
}

// SetBool persists a boolean setting as the literal "true" / "false".
func (s *SettingsStore) SetBool(ctx context.Context, name string, value bool, updatedBy string) error {
	v := "false"
	if value {
		v = "true"
	}
	return s.Set(ctx, name, v, updatedBy)
}
