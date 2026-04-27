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
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/rabbitstack/fibratus/internal/fleetserver/secrets"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store/postgres"
	log "github.com/sirupsen/logrus"
)

// Server-wide secret names. Keep stable — they live in system_secrets.name.
const (
	SecretClickHousePassword       = "clickhouse.password"
	SecretClickHouseCloudKeyID     = "clickhouse_cloud.api_key_id"
	SecretClickHouseCloudKeySecret = "clickhouse_cloud.api_key_secret"
	SecretClickHouseCloudOrgID     = "clickhouse_cloud.organization_id"
)

// Server-wide non-secret setting keys. Live in system_settings.name.
const (
	SettingSignupRequireApproval = "signup.require_approval"
)

// initSecretStore loads the master key, constructs the SecretStore, and runs
// any one-time migrations that move plaintext config into the encrypted store.
//
// Specifically: if the YAML still contains a ClickHouse password but the
// encrypted store does not, the password is copied into the store and the
// in-memory config keeps using it. After the operator commits the config
// without the inline password (or rotates it via the dashboard), the store
// becomes the source of truth.
func (s *Server) initSecretStore(ctx context.Context, db *sql.DB) error {
	keyPath := ""
	if s.configPath != "" {
		keyPath = filepath.Join(filepath.Dir(s.configPath), "master.key")
	}
	key, err := secrets.LoadKey(keyPath)
	if err != nil {
		return fmt.Errorf("fleet: load master key: %w (set %s to a 64-char hex string, or let the server auto-generate at %s; manual generate: openssl rand -hex 32)", err, secrets.EnvKeyName, keyPath)
	}
	enc, err := secrets.New(key)
	if err != nil {
		return fmt.Errorf("fleet: init encryptor: %w", err)
	}
	s.secretStore = postgres.NewSecretStore(db, enc)
	log.Info("fleet: encrypted secret store ready")

	// One-time migration: move ClickHouse password from YAML into the store.
	// On subsequent boots the store wins and the YAML field is ignored.
	if s.config.ClickHouse.Password != "" {
		exists, err := s.secretStore.Has(ctx, SecretClickHousePassword)
		if err != nil {
			return fmt.Errorf("fleet: check clickhouse password secret: %w", err)
		}
		if !exists {
			if err := s.secretStore.Set(ctx, SecretClickHousePassword, s.config.ClickHouse.Password, "boot-migration"); err != nil {
				return fmt.Errorf("fleet: persist clickhouse password to secret store: %w", err)
			}
			log.Info("fleet: migrated ClickHouse password from YAML into encrypted store")
		}
	}

	// Resolve the ClickHouse password from the store; fall through to YAML if
	// nothing was migrated (e.g. fresh install without a CH config yet).
	pw, err := s.secretStore.Get(ctx, SecretClickHousePassword)
	if err == nil {
		s.config.ClickHouse.Password = pw
	} else if !errors.Is(err, postgres.ErrSecretNotFound) {
		return fmt.Errorf("fleet: read clickhouse password: %w", err)
	}
	return nil
}
