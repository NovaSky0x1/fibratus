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

// AccountStore implements store.AccountStore backed by PostgreSQL.
type AccountStore struct {
	db *sql.DB
}

// NewAccountStore creates a new account store.
func NewAccountStore(db *sql.DB) *AccountStore {
	return &AccountStore{db: db}
}

func (s *AccountStore) Create(ctx context.Context, account *fleet.Account) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO accounts (id, name, plan, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		account.ID, account.Name, account.Plan, account.CreatedAt, account.UpdatedAt,
	)
	return err
}

func (s *AccountStore) Get(ctx context.Context, id string) (*fleet.Account, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, plan, COALESCE(require_2fa, false), COALESCE(tamper_protection_enabled, false), COALESCE(isolation_whitelist, '[]'::jsonb), COALESCE(eventlog_enabled, false), COALESCE(telemetry_retention_days, 7), created_at, updated_at
		 FROM accounts WHERE id = $1`, id)

	a := &fleet.Account{}
	var wlJSON []byte
	err := row.Scan(&a.ID, &a.Name, &a.Plan, &a.Require2FA, &a.TamperProtectionEnabled, &wlJSON, &a.EventLogEnabled, &a.TelemetryRetentionDays, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if len(wlJSON) > 0 {
		json.Unmarshal(wlJSON, &a.IsolationWhitelist)
	}
	return a, nil
}

func (s *AccountStore) ListAll(ctx context.Context) ([]*fleet.Account, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.id, a.name, a.plan, COALESCE(a.require_2fa, false), COALESCE(a.telemetry_retention_days, 7), a.created_at, a.updated_at,
			COALESCE((SELECT COUNT(*) FROM organizations o WHERE o.account_id = a.id), 0) as org_count,
			COALESCE((SELECT COUNT(*) FROM users u WHERE u.account_id = a.id), 0) as user_count
		 FROM accounts a ORDER BY a.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := make([]*fleet.Account, 0)
	for rows.Next() {
		a := &fleet.Account{}
		var orgCount, userCount int
		if err := rows.Scan(&a.ID, &a.Name, &a.Plan, &a.Require2FA, &a.TelemetryRetentionDays, &a.CreatedAt, &a.UpdatedAt, &orgCount, &userCount); err != nil {
			return nil, err
		}
		a.OrgCount = orgCount
		a.UserCount = userCount
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (s *AccountStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM accounts WHERE id = $1`, id)
	return err
}

func (s *AccountStore) UpdateSettings(ctx context.Context, id string, require2FA bool, tamperProtection *bool, isolationWhitelist []string, eventlogEnabled *bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET require_2fa = $2, updated_at = NOW() WHERE id = $1`,
		id, require2FA)
	if err != nil {
		return err
	}
	if tamperProtection != nil {
		_, err = s.db.ExecContext(ctx,
			`UPDATE accounts SET tamper_protection_enabled = $2, updated_at = NOW() WHERE id = $1`,
			id, *tamperProtection)
		if err != nil {
			return err
		}
	}
	if isolationWhitelist != nil {
		wl, _ := json.Marshal(isolationWhitelist)
		_, err = s.db.ExecContext(ctx,
			`UPDATE accounts SET isolation_whitelist = $2, updated_at = NOW() WHERE id = $1`,
			id, wl)
		if err != nil {
			return err
		}
	}
	if eventlogEnabled != nil {
		_, err = s.db.ExecContext(ctx,
			`UPDATE accounts SET eventlog_enabled = $2, updated_at = NOW() WHERE id = $1`,
			id, *eventlogEnabled)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *AccountStore) UpdateRetention(ctx context.Context, id string, days int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET telemetry_retention_days = $2, updated_at = NOW() WHERE id = $1`, id, days)
	return err
}

func (s *AccountStore) UpdateProfile(ctx context.Context, id, name, plan string) error {
	if name != "" && plan != "" {
		_, err := s.db.ExecContext(ctx,
			`UPDATE accounts SET name = $2, plan = $3, updated_at = NOW() WHERE id = $1`, id, name, plan)
		return err
	}
	if name != "" {
		_, err := s.db.ExecContext(ctx,
			`UPDATE accounts SET name = $2, updated_at = NOW() WHERE id = $1`, id, name)
		return err
	}
	if plan != "" {
		_, err := s.db.ExecContext(ctx,
			`UPDATE accounts SET plan = $2, updated_at = NOW() WHERE id = $1`, id, plan)
		return err
	}
	return nil
}

// OrgStore implements store.OrgStore backed by PostgreSQL.
type OrgStore struct {
	db *sql.DB
}

// NewOrgStore creates a new organization store.
func NewOrgStore(db *sql.DB) *OrgStore {
	return &OrgStore{db: db}
}

func (s *OrgStore) Create(ctx context.Context, org *fleet.Organization) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO organizations (id, account_id, name, slug, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		org.ID, org.AccountID, org.Name, org.Slug, org.CreatedAt, org.UpdatedAt,
	)
	return err
}

func (s *OrgStore) Get(ctx context.Context, id string) (*fleet.Organization, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, account_id, name, slug, COALESCE(tamper_protection_enabled, false), COALESCE(telemetry_retention_days, 0), created_at, updated_at
		 FROM organizations WHERE id = $1`, id)

	o := &fleet.Organization{}
	err := row.Scan(&o.ID, &o.AccountID, &o.Name, &o.Slug, &o.TamperProtectionEnabled, &o.TelemetryRetentionDays, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return o, nil
}

func (s *OrgStore) ListByAccount(ctx context.Context, accountID string) ([]*fleet.Organization, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT o.id, o.account_id, o.name, o.slug,
			COALESCE((SELECT COUNT(*) FROM agents a WHERE a.org_id = o.id), 0) as agent_count,
			COALESCE(o.tamper_protection_enabled, false),
			COALESCE(o.telemetry_retention_days, 0),
			o.created_at, o.updated_at
		 FROM organizations o
		 WHERE o.account_id = $1
		 ORDER BY o.created_at`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orgs := make([]*fleet.Organization, 0)
	for rows.Next() {
		o := &fleet.Organization{}
		if err := rows.Scan(&o.ID, &o.AccountID, &o.Name, &o.Slug, &o.AgentCount, &o.TamperProtectionEnabled, &o.TelemetryRetentionDays, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		orgs = append(orgs, o)
	}
	return orgs, rows.Err()
}

func (s *OrgStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM organizations WHERE id = $1`, id)
	return err
}

func (s *OrgStore) UpdateTamperProtection(ctx context.Context, id string, enabled bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE organizations SET tamper_protection_enabled = $2, updated_at = NOW() WHERE id = $1`,
		id, enabled)
	return err
}

func (s *OrgStore) UpdateRetention(ctx context.Context, id string, days int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE organizations SET telemetry_retention_days = $2, updated_at = NOW() WHERE id = $1`,
		id, days)
	return err
}
