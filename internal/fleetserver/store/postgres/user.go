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

// UserStore implements store.UserStore backed by PostgreSQL.
type UserStore struct {
	db *sql.DB
}

// NewUserStore creates a new user store.
func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) Create(ctx context.Context, user *fleet.User) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, email, name, password, account_id, role, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		user.ID, user.Email, user.Name, user.Password, user.AccountID, user.Role, user.CreatedAt,
	)
	return err
}

func (s *UserStore) GetByEmail(ctx context.Context, email string) (*fleet.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, password, account_id, role, created_at,
		        login_attempts, COALESCE(locked_until, '1970-01-01'::timestamptz),
		        totp_secret, totp_enabled, recovery_codes
		 FROM users WHERE email = $1`, email)

	u := &fleet.User{}
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Password, &u.AccountID, &u.Role, &u.CreatedAt,
		&u.LoginAttempts, &u.LockedUntil, &u.TOTPSecret, &u.TOTPEnabled, &u.RecoveryCodes)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

func (s *UserStore) Get(ctx context.Context, id string) (*fleet.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, password, account_id, role, created_at,
		        login_attempts, COALESCE(locked_until, '1970-01-01'::timestamptz),
		        totp_secret, totp_enabled, recovery_codes
		 FROM users WHERE id = $1`, id)

	u := &fleet.User{}
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Password, &u.AccountID, &u.Role, &u.CreatedAt,
		&u.LoginAttempts, &u.LockedUntil, &u.TOTPSecret, &u.TOTPEnabled, &u.RecoveryCodes)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

func (s *UserStore) AddOrgAccess(ctx context.Context, userID, orgID, role string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_orgs (user_id, org_id, role)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, org_id) DO UPDATE SET role = EXCLUDED.role`,
		userID, orgID, role,
	)
	return err
}

func (s *UserStore) GetOrgAccess(ctx context.Context, userID string) ([]fleet.UserOrg, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id, org_id, role
		 FROM user_orgs WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	access := make([]fleet.UserOrg, 0)
	for rows.Next() {
		var uo fleet.UserOrg
		if err := rows.Scan(&uo.UserID, &uo.OrgID, &uo.Role); err != nil {
			return nil, err
		}
		access = append(access, uo)
	}
	return access, rows.Err()
}

func (s *UserStore) HasOrgAccess(ctx context.Context, userID, orgID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM user_orgs WHERE user_id = $1 AND org_id = $2)`,
		userID, orgID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (s *UserStore) ListAll(ctx context.Context) ([]*fleet.User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, name, account_id, role, created_at, totp_enabled,
			COALESCE(org_restrictions, ''), login_attempts, COALESCE(locked_until, '1970-01-01'::timestamptz)
		 FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]*fleet.User, 0)
	for rows.Next() {
		u := &fleet.User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.AccountID, &u.Role, &u.CreatedAt,
			&u.TOTPEnabled, &u.OrgRestrictions, &u.LoginAttempts, &u.LockedUntil); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *UserStore) UpdateProfile(ctx context.Context, userID, name, email string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET name = $2, email = $3, updated_at = NOW() WHERE id = $1`,
		userID, name, email)
	return err
}

func (s *UserStore) UpdatePassword(ctx context.Context, userID, hashedPassword string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET password = $2, updated_at = NOW() WHERE id = $1`,
		userID, hashedPassword)
	return err
}

func (s *UserStore) ListByAccount(ctx context.Context, accountID string) ([]*fleet.User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT u.id, u.email, u.name, u.account_id, u.role, u.created_at, u.totp_enabled,
			COALESCE(u.org_restrictions, ''), u.login_attempts, COALESCE(u.locked_until, '1970-01-01'::timestamptz)
		 FROM users u WHERE u.account_id = $1 ORDER BY u.created_at`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]*fleet.User, 0)
	for rows.Next() {
		u := &fleet.User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.AccountID, &u.Role, &u.CreatedAt,
			&u.TOTPEnabled, &u.OrgRestrictions, &u.LoginAttempts, &u.LockedUntil); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *UserStore) SetOrgRestrictions(ctx context.Context, userID string, orgRestrictions string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET org_restrictions = $2, updated_at = NOW() WHERE id = $1`,
		userID, orgRestrictions)
	return err
}

func (s *UserStore) SetAccount(ctx context.Context, userID, accountID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET account_id = $2, updated_at = NOW() WHERE id = $1`,
		userID, accountID)
	return err
}

func (s *UserStore) IncrementLoginAttempts(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET login_attempts = login_attempts + 1 WHERE id = $1`, userID)
	return err
}

func (s *UserStore) LockAccount(ctx context.Context, userID string, until time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET locked_until = $2 WHERE id = $1`, userID, until)
	return err
}

func (s *UserStore) ResetLoginAttempts(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET login_attempts = 0, locked_until = NULL WHERE id = $1`, userID)
	return err
}

func (s *UserStore) SetTOTP(ctx context.Context, userID, secret string, enabled bool, recoveryCodes string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET totp_secret = $2, totp_enabled = $3, recovery_codes = $4 WHERE id = $1`,
		userID, secret, enabled, recoveryCodes)
	return err
}

func (s *UserStore) ListByOrg(ctx context.Context, orgID string) ([]*fleet.User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT u.id, u.email, u.name, u.account_id, uo.role, u.created_at, u.totp_enabled
		 FROM users u
		 JOIN user_orgs uo ON u.id = uo.user_id
		 WHERE uo.org_id = $1
		 ORDER BY u.created_at`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]*fleet.User, 0)
	for rows.Next() {
		u := &fleet.User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.AccountID, &u.Role, &u.CreatedAt, &u.TOTPEnabled); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *UserStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	return err
}

func (s *UserStore) UpdateRole(ctx context.Context, userID, orgID, role string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE user_orgs SET role = $3 WHERE user_id = $1 AND org_id = $2`,
		userID, orgID, role)
	return err
}

func (s *UserStore) SetRole(ctx context.Context, userID, role string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET role = $2, updated_at = NOW() WHERE id = $1`,
		userID, role)
	return err
}
