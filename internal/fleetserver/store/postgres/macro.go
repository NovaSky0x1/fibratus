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
	"fmt"
	"time"

	"github.com/rabbitstack/fibratus/pkg/fleet"
	"gopkg.in/yaml.v3"
)

// MacroStore implements store.MacroStore using PostgreSQL.
type MacroStore struct {
	db *sql.DB
}

// NewMacroStore creates a new PostgreSQL macro store.
func NewMacroStore(db *sql.DB) *MacroStore {
	return &MacroStore{db: db}
}

func (s *MacroStore) Create(ctx context.Context, macro *fleet.Macro) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO macros (id, org_id, name, expr, description, raw_yaml, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`,
		macro.ID, macro.OrgID, macro.Name, macro.Expr, macro.Description, macro.RawYAML,
	)
	return err
}

func (s *MacroStore) Get(ctx context.Context, orgID, id string) (*fleet.Macro, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, name, expr, description, raw_yaml, created_at, updated_at
		 FROM macros WHERE id=$1 AND org_id=$2`, id, orgID)
	return scanMacro(row)
}

func (s *MacroStore) List(ctx context.Context, orgID string) ([]*fleet.Macro, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, name, expr, description, raw_yaml, created_at, updated_at
		 FROM macros WHERE org_id=$1 ORDER BY name ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var macros []*fleet.Macro
	for rows.Next() {
		m := &fleet.Macro{}
		if err := rows.Scan(&m.ID, &m.OrgID, &m.Name, &m.Expr, &m.Description, &m.RawYAML, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		macros = append(macros, m)
	}
	return macros, nil
}

func (s *MacroStore) Update(ctx context.Context, macro *fleet.Macro) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE macros SET name=$3, expr=$4, description=$5, raw_yaml=$6, updated_at=NOW()
		 WHERE id=$1 AND org_id=$2`,
		macro.ID, macro.OrgID, macro.Name, macro.Expr, macro.Description, macro.RawYAML,
	)
	return err
}

func (s *MacroStore) Delete(ctx context.Context, orgID, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM macros WHERE id=$1 AND org_id=$2`, id, orgID)
	return err
}

// GetAllForOrg returns all macros for an org formatted as YAML for agent rule sync.
// Uses proper YAML marshaling to handle special characters in expressions.
func (s *MacroStore) GetAllForOrg(ctx context.Context, orgID string) (string, error) {
	macros, err := s.List(ctx, orgID)
	if err != nil {
		return "", err
	}
	if len(macros) == 0 {
		return "", nil
	}

	type yamlMacro struct {
		Macro       string `yaml:"macro"`
		Expr        string `yaml:"expr"`
		Description string `yaml:"description,omitempty"`
	}

	out := make([]yamlMacro, 0, len(macros))
	for _, m := range macros {
		if m.Expr == "" {
			continue // skip macros with empty expressions
		}
		out = append(out, yamlMacro{
			Macro:       m.Name,
			Expr:        m.Expr,
			Description: m.Description,
		})
	}

	data, err := yaml.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("marshal macros: %w", err)
	}
	return string(data), nil
}

func scanMacro(row *sql.Row) (*fleet.Macro, error) {
	m := &fleet.Macro{}
	err := row.Scan(&m.ID, &m.OrgID, &m.Name, &m.Expr, &m.Description, &m.RawYAML, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

// MacroETag computes a simple ETag for macro change detection.
func (s *MacroStore) MacroETag(ctx context.Context, orgID string) (string, error) {
	var maxUpdated time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(updated_at), '1970-01-01') FROM macros WHERE org_id=$1`, orgID).Scan(&maxUpdated)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("macros-%d", maxUpdated.UnixNano()), nil
}
