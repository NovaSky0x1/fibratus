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
	// Local profile. Always seeded with safe self-hosted defaults. We deliberately
	// ignore s.config.ClickHouse here: that YAML block historically pointed at a
	// ClickHouse Cloud endpoint on some installs, and copying those values into
	// the "local" slot meant the slot was never actually local — a fresh deploy
	// could end up with both slots pointing at the same remote host. Always
	// seeding 127.0.0.1:9000 plaintext makes "local" mean local for everyone
	// who runs deploy/install-fleet-server.sh.
	if _, err := s.profileStore.Get(ctx, ProfileLocal); errors.Is(err, postgres.ErrProfileNotFound) {
		local := &postgres.ClickHouseProfile{
			Name:                ProfileLocal,
			Enabled:             true,
			Host:                "127.0.0.1",
			Port:                9000,
			Database:            "default",
			User:                "default",
			Secure:              false,
			SkipVerify:          false,
			DialTimeoutSecs:     10,
			MaxOpenConns:        20,
			MaxIdleConns:        10,
			ConnMaxLifetimeSecs: 3600,
			UpdatedBy:           "boot-migration",
		}
		if err := s.profileStore.Upsert(ctx, local); err != nil {
			return err
		}
		log.Info("fleet: seeded clickhouse profile 'local' (127.0.0.1:9000, plaintext)")

		// If the legacy YAML had a password, preserve it under the new
		// per-profile secret name so installs that did configure auth on
		// localhost don't lose it across the migration.
		if c := s.config.ClickHouse; c.Password != "" {
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
