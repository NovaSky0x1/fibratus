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

// GlobalRuleStore implements global rule management backed by PostgreSQL.
// Global rules apply to all organizations unless explicitly overridden.
type GlobalRuleStore struct {
	db *sql.DB
}

// NewGlobalRuleStore creates a new global rule store.
func NewGlobalRuleStore(db *sql.DB) *GlobalRuleStore {
	return &GlobalRuleStore{db: db}
}

func (s *GlobalRuleStore) Create(ctx context.Context, rule *fleet.Rule) error {
	labels, _ := json.Marshal(rule.Labels)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO global_rules (id, name, version, description, condition, output_template,
			severity, labels, tags, "references", raw_yaml, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW())`,
		rule.ID, rule.Name, rule.Version, rule.Description,
		rule.Condition, rule.Output, rule.Severity, labels,
		rule.Tags, rule.References, rule.RawYAML, rule.Enabled,
	)
	return err
}

func (s *GlobalRuleStore) Get(ctx context.Context, id string) (*fleet.Rule, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, version, description, condition, output_template,
			severity, labels, tags, "references", raw_yaml, enabled, created_at, updated_at
		 FROM global_rules WHERE id = $1`, id)
	return scanGlobalRule(row)
}

func (s *GlobalRuleStore) List(ctx context.Context, opts fleet.ListOptions) ([]*fleet.Rule, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM global_rules`,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = 50
	}
	page := opts.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * perPage

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, version, description, condition, output_template,
			severity, labels, tags, "references", raw_yaml, enabled, created_at, updated_at
		 FROM global_rules
		 ORDER BY name ASC
		 LIMIT $1 OFFSET $2`,
		perPage, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	rules := make([]*fleet.Rule, 0)
	for rows.Next() {
		r, err := scanGlobalRuleRows(rows)
		if err != nil {
			return nil, 0, err
		}
		rules = append(rules, r)
	}
	return rules, total, rows.Err()
}

func (s *GlobalRuleStore) Update(ctx context.Context, rule *fleet.Rule) error {
	labels, _ := json.Marshal(rule.Labels)
	_, err := s.db.ExecContext(ctx,
		`UPDATE global_rules SET name=$2, version=$3, description=$4, condition=$5,
			output_template=$6, severity=$7, labels=$8, tags=$9, "references"=$10,
			raw_yaml=$11, enabled=$12, updated_at=NOW()
		 WHERE id=$1`,
		rule.ID, rule.Name, rule.Version, rule.Description,
		rule.Condition, rule.Output, rule.Severity, labels,
		rule.Tags, rule.References, rule.RawYAML, rule.Enabled,
	)
	return err
}

func (s *GlobalRuleStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM global_rules WHERE id=$1`, id)
	return err
}

// GetForOrg returns all enabled global rules, excluding any that have been
// disabled by an organization-level override in global_rule_overrides.
func (s *GlobalRuleStore) GetForOrg(ctx context.Context, orgID string) ([]*fleet.Rule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT gr.id, gr.name, gr.version, gr.description, gr.condition, gr.output_template,
			gr.severity, gr.labels, gr.tags, gr."references", gr.raw_yaml, gr.enabled,
			gr.created_at, gr.updated_at
		 FROM global_rules gr
		 LEFT JOIN global_rule_overrides o ON gr.id = o.rule_id AND o.org_id = $1
		 WHERE gr.enabled = true AND (o.enabled IS NULL OR o.enabled = true)
		 ORDER BY gr.name ASC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]*fleet.Rule, 0)
	for rows.Next() {
		r, err := scanGlobalRuleRows(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rules, nil
}

// SetOrgOverride upserts a per-organization override for a global rule.
// When enabled is false, the global rule is effectively disabled for that org.
func (s *GlobalRuleStore) SetOrgOverride(ctx context.Context, orgID, ruleID string, enabled bool) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO global_rule_overrides (org_id, rule_id, enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, NOW(), NOW())
		 ON CONFLICT (org_id, rule_id) DO UPDATE SET enabled = $3, updated_at = NOW()`,
		orgID, ruleID, enabled,
	)
	return err
}

// GetOrgOverrides returns a map of rule_id -> enabled for all overrides
// belonging to the given organization.
func (s *GlobalRuleStore) GetOrgOverrides(ctx context.Context, orgID string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT rule_id, enabled FROM global_rule_overrides WHERE org_id = $1`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	overrides := make(map[string]bool)
	for rows.Next() {
		var ruleID string
		var enabled bool
		if err := rows.Scan(&ruleID, &enabled); err != nil {
			return nil, err
		}
		overrides[ruleID] = enabled
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return overrides, nil
}

func scanGlobalRule(row *sql.Row) (*fleet.Rule, error) {
	r := &fleet.Rule{}
	var labelsJSON []byte
	err := row.Scan(
		&r.ID, &r.Name, &r.Version, &r.Description,
		&r.Condition, &r.Output, &r.Severity, &labelsJSON,
		&r.Tags, &r.References, &r.RawYAML, &r.Enabled,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	json.Unmarshal(labelsJSON, &r.Labels)
	return r, nil
}

func scanGlobalRuleRows(rows *sql.Rows) (*fleet.Rule, error) {
	r := &fleet.Rule{}
	var labelsJSON []byte
	err := rows.Scan(
		&r.ID, &r.Name, &r.Version, &r.Description,
		&r.Condition, &r.Output, &r.Severity, &labelsJSON,
		&r.Tags, &r.References, &r.RawYAML, &r.Enabled,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(labelsJSON, &r.Labels)
	return r, nil
}
