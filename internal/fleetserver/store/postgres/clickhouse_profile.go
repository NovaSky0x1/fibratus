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
	"time"
)

// ClickHouseProfile is one named connection slot. Two profiles are typical —
// "local" (self-hosted) and "cloud" (ClickHouse Cloud service). Passwords are
// not stored on the profile; they live in system_secrets under the key
// "clickhouse.<name>.password".
type ClickHouseProfile struct {
	Name                 string
	Enabled              bool
	Host                 string
	Port                 int
	Database             string
	User                 string
	Secure               bool
	SkipVerify           bool
	DialTimeoutSecs      int
	MaxOpenConns         int
	MaxIdleConns         int
	ConnMaxLifetimeSecs  int
	CloudOrgID           string
	CloudServiceID       string
	UpdatedAt            time.Time
	UpdatedBy            string
}

// ClickHouseProfileStore CRUDs the clickhouse_profiles table.
type ClickHouseProfileStore struct {
	db *sql.DB
}

func NewClickHouseProfileStore(db *sql.DB) *ClickHouseProfileStore {
	return &ClickHouseProfileStore{db: db}
}

// ErrProfileNotFound — profile name has no row.
var ErrProfileNotFound = errors.New("clickhouse profile not found")

// Get returns the profile by name. ErrProfileNotFound when absent.
func (s *ClickHouseProfileStore) Get(ctx context.Context, name string) (*ClickHouseProfile, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT name, enabled, host, port, database, "user", secure, skip_verify,
		       dial_timeout_secs, max_open_conns, max_idle_conns, conn_max_lifetime_secs,
		       cloud_org_id, cloud_service_id, updated_at, COALESCE(updated_by, '')
		FROM clickhouse_profiles WHERE name = $1
	`, name)
	p := &ClickHouseProfile{}
	err := row.Scan(
		&p.Name, &p.Enabled, &p.Host, &p.Port, &p.Database, &p.User, &p.Secure, &p.SkipVerify,
		&p.DialTimeoutSecs, &p.MaxOpenConns, &p.MaxIdleConns, &p.ConnMaxLifetimeSecs,
		&p.CloudOrgID, &p.CloudServiceID, &p.UpdatedAt, &p.UpdatedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrProfileNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("clickhouse profile %s: %w", name, err)
	}
	return p, nil
}

// List returns every profile, ordered by name (so "cloud" before "local").
func (s *ClickHouseProfileStore) List(ctx context.Context) ([]*ClickHouseProfile, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT name, enabled, host, port, database, "user", secure, skip_verify,
		       dial_timeout_secs, max_open_conns, max_idle_conns, conn_max_lifetime_secs,
		       cloud_org_id, cloud_service_id, updated_at, COALESCE(updated_by, '')
		FROM clickhouse_profiles ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list clickhouse profiles: %w", err)
	}
	defer rows.Close()
	var out []*ClickHouseProfile
	for rows.Next() {
		p := &ClickHouseProfile{}
		if err := rows.Scan(
			&p.Name, &p.Enabled, &p.Host, &p.Port, &p.Database, &p.User, &p.Secure, &p.SkipVerify,
			&p.DialTimeoutSecs, &p.MaxOpenConns, &p.MaxIdleConns, &p.ConnMaxLifetimeSecs,
			&p.CloudOrgID, &p.CloudServiceID, &p.UpdatedAt, &p.UpdatedBy,
		); err != nil {
			return nil, fmt.Errorf("scan clickhouse profile: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Upsert creates or updates a profile by name.
func (s *ClickHouseProfileStore) Upsert(ctx context.Context, p *ClickHouseProfile) error {
	if p.Name == "" {
		return errors.New("profile name is required")
	}
	if p.Port <= 0 {
		return errors.New("profile port must be positive")
	}
	if p.Host == "" {
		return errors.New("profile host is required")
	}
	if p.Database == "" {
		p.Database = "default"
	}
	if p.User == "" {
		p.User = "default"
	}
	if p.DialTimeoutSecs <= 0 {
		p.DialTimeoutSecs = 10
	}
	if p.MaxOpenConns <= 0 {
		p.MaxOpenConns = 20
	}
	if p.MaxIdleConns <= 0 {
		p.MaxIdleConns = 10
	}
	if p.ConnMaxLifetimeSecs <= 0 {
		p.ConnMaxLifetimeSecs = 3600
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO clickhouse_profiles
		    (name, enabled, host, port, database, "user", secure, skip_verify,
		     dial_timeout_secs, max_open_conns, max_idle_conns, conn_max_lifetime_secs,
		     cloud_org_id, cloud_service_id, updated_at, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW(), $15)
		ON CONFLICT (name) DO UPDATE SET
		    enabled                = EXCLUDED.enabled,
		    host                   = EXCLUDED.host,
		    port                   = EXCLUDED.port,
		    database               = EXCLUDED.database,
		    "user"                 = EXCLUDED."user",
		    secure                 = EXCLUDED.secure,
		    skip_verify            = EXCLUDED.skip_verify,
		    dial_timeout_secs      = EXCLUDED.dial_timeout_secs,
		    max_open_conns         = EXCLUDED.max_open_conns,
		    max_idle_conns         = EXCLUDED.max_idle_conns,
		    conn_max_lifetime_secs = EXCLUDED.conn_max_lifetime_secs,
		    cloud_org_id           = EXCLUDED.cloud_org_id,
		    cloud_service_id       = EXCLUDED.cloud_service_id,
		    updated_at             = NOW(),
		    updated_by             = EXCLUDED.updated_by
	`, p.Name, p.Enabled, p.Host, p.Port, p.Database, p.User, p.Secure, p.SkipVerify,
		p.DialTimeoutSecs, p.MaxOpenConns, p.MaxIdleConns, p.ConnMaxLifetimeSecs,
		p.CloudOrgID, p.CloudServiceID, p.UpdatedBy)
	if err != nil {
		return fmt.Errorf("upsert clickhouse profile %s: %w", p.Name, err)
	}
	return nil
}
