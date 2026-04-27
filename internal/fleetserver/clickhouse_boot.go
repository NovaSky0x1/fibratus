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
	"errors"

	"github.com/rabbitstack/fibratus/internal/fleetserver/store/postgres"
	log "github.com/sirupsen/logrus"
)

// migrateLegacyClickHouseConfig is a one-time bootstrap that converts the
// legacy ClickHouse config (single ClickHouseConfig in YAML + a single
// clickhouse.password secret) into the new two-profile model.
//
// Behaviour:
//   - If a "local" profile row already exists, do nothing.
//   - Otherwise seed a "local" row from s.config.ClickHouse and copy any
//     pre-existing clickhouse.password secret into clickhouse.local.password.
//   - Always seed a default "cloud" profile row (TLS-on, port 9440) so
//     operators can bind to it without having to type host/port from scratch.
//   - If no active profile is set, point at "local" so behaviour is identical
//     to pre-migration boots.
func (s *Server) migrateLegacyClickHouseConfig(ctx context.Context) error {
	// Local profile.
	if _, err := s.profileStore.Get(ctx, ProfileLocal); errors.Is(err, postgres.ErrProfileNotFound) {
		c := s.config.ClickHouse
		// Sane defaults if YAML had nothing.
		if c.Host == "" {
			c.Host = "localhost"
		}
		if c.Port == 0 {
			c.Port = 9000
		}
		if c.Database == "" {
			c.Database = "fibratus"
		}
		if c.User == "" {
			c.User = "default"
		}
		if c.MaxOpenConns == 0 {
			c.MaxOpenConns = 20
		}
		if c.MaxIdleConns == 0 {
			c.MaxIdleConns = 10
		}
		if c.ConnMaxLifetime == 0 {
			c.ConnMaxLifetime = 3600
		}
		if c.DialTimeoutSecs == 0 {
			c.DialTimeoutSecs = 10
		}
		local := &postgres.ClickHouseProfile{
			Name:                ProfileLocal,
			Enabled:             c.Enabled,
			Host:                c.Host,
			Port:                c.Port,
			Database:            c.Database,
			User:                c.User,
			Secure:              c.Secure,
			SkipVerify:          c.SkipVerify,
			DialTimeoutSecs:     c.DialTimeoutSecs,
			MaxOpenConns:        c.MaxOpenConns,
			MaxIdleConns:        c.MaxIdleConns,
			ConnMaxLifetimeSecs: c.ConnMaxLifetime,
			UpdatedBy:           "boot-migration",
		}
		if err := s.profileStore.Upsert(ctx, local); err != nil {
			return err
		}
		log.Info("fleet: seeded clickhouse profile 'local' from legacy YAML config")

		// Copy password if one was present, either in YAML (already loaded
		// into c.Password by initSecretStore) or in the legacy single-key
		// secret. The single-key secret has already been deprecated; we
		// keep this branch for installs that never went through the
		// previous migration.
		if c.Password != "" {
			if err := s.secretStore.Set(ctx, SecretProfilePassword(ProfileLocal), c.Password, "boot-migration"); err != nil {
				return err
			}
		}
	} else if err != nil {
		return err
	}

	// Cloud profile — seed an empty shell with sensible defaults so the
	// dashboard can edit fields in place rather than starting from nothing.
	if _, err := s.profileStore.Get(ctx, ProfileCloud); errors.Is(err, postgres.ErrProfileNotFound) {
		cloud := &postgres.ClickHouseProfile{
			Name:                ProfileCloud,
			Enabled:             false,
			Host:                "",
			Port:                9440,
			Database:            "default",
			User:                "default",
			Secure:              true,
			SkipVerify:          false,
			DialTimeoutSecs:     10,
			MaxOpenConns:        20,
			MaxIdleConns:        10,
			ConnMaxLifetimeSecs: 3600,
			UpdatedBy:           "boot-migration",
		}
		if err := s.profileStore.Upsert(ctx, cloud); err != nil {
			return err
		}
		log.Info("fleet: seeded empty clickhouse profile 'cloud'")
	} else if err != nil {
		return err
	}

	// Active pointer — default to local.
	if cur, _ := s.secretStore.GetOr(ctx, SecretActiveProfile, ""); cur == "" {
		if err := s.secretStore.Set(ctx, SecretActiveProfile, ProfileLocal, "boot-migration"); err != nil {
			return err
		}
	}
	return nil
}
