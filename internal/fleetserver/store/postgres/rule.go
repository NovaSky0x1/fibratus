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
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/lib/pq"
	"github.com/rabbitstack/fibratus/pkg/fleet"
)

// RuleStore implements store.RuleStore backed by PostgreSQL.
type RuleStore struct {
	db *sql.DB
}

// NewRuleStore creates a new rule store.
func NewRuleStore(db *sql.DB) *RuleStore {
	return &RuleStore{db: db}
}

func (s *RuleStore) Create(ctx context.Context, rule *fleet.Rule) error {
	labels, _ := json.Marshal(rule.Labels)
	validationErrors := rule.ValidationErrors
	if validationErrors == nil {
		validationErrors = json.RawMessage(`[]`)
	}
	if rule.ValidationStatus == "" {
		rule.ValidationStatus = "pending"
	}
	if rule.Source == "" {
		rule.Source = "manual"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO rules (id, org_id, name, version, description, condition, output_template,
			severity, labels, tags, "references", raw_yaml, enabled, source, validation_status, validation_errors, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, NOW(), NOW())`,
		rule.ID, rule.OrgID, rule.Name, rule.Version, rule.Description,
		rule.Condition, rule.Output, rule.Severity, labels,
		pq.Array(rule.Tags), pq.Array(rule.References), rule.RawYAML, rule.Enabled,
		rule.Source, rule.ValidationStatus, validationErrors,
	)
	return err
}

func (s *RuleStore) Get(ctx context.Context, orgID, id string) (*fleet.Rule, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, name, version, description, condition, output_template,
			severity, labels, tags, "references", raw_yaml, enabled,
			COALESCE(validation_status, 'pending'), COALESCE(validation_errors, '[]'),
			COALESCE(source, 'manual'), created_at, updated_at
		 FROM rules WHERE id = $1 AND org_id = $2`, id, orgID)
	return scanRule(row)
}

func (s *RuleStore) List(ctx context.Context, orgID string, opts fleet.ListOptions) ([]*fleet.Rule, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM rules WHERE org_id = $1`, orgID,
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
		`SELECT id, org_id, name, version, description, condition, output_template,
			severity, labels, tags, "references", raw_yaml, enabled,
			COALESCE(validation_status, 'pending'), COALESCE(validation_errors, '[]'),
			COALESCE(source, 'manual'), created_at, updated_at
		 FROM rules WHERE org_id = $1
		 ORDER BY name ASC
		 LIMIT $2 OFFSET $3`,
		orgID, perPage, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	rules := make([]*fleet.Rule, 0)
	for rows.Next() {
		r, err := scanRuleRows(rows)
		if err != nil {
			return nil, 0, err
		}
		rules = append(rules, r)
	}
	return rules, total, rows.Err()
}

func (s *RuleStore) Update(ctx context.Context, rule *fleet.Rule) error {
	labels, _ := json.Marshal(rule.Labels)
	validationErrors := rule.ValidationErrors
	if validationErrors == nil {
		validationErrors = json.RawMessage(`[]`)
	}
	if rule.ValidationStatus == "" {
		rule.ValidationStatus = "pending"
	}
	if rule.Source == "" {
		rule.Source = "manual"
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE rules SET name=$3, version=$4, description=$5, condition=$6,
			output_template=$7, severity=$8, labels=$9, tags=$10, "references"=$11,
			raw_yaml=$12, enabled=$13, source=$14, validation_status=$15, validation_errors=$16, updated_at=NOW()
		 WHERE id=$1 AND org_id=$2`,
		rule.ID, rule.OrgID, rule.Name, rule.Version, rule.Description,
		rule.Condition, rule.Output, rule.Severity, labels,
		pq.Array(rule.Tags), pq.Array(rule.References), rule.RawYAML, rule.Enabled,
		rule.Source, rule.ValidationStatus, validationErrors,
	)
	return err
}

func (s *RuleStore) Delete(ctx context.Context, orgID, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM rules WHERE id=$1 AND org_id=$2`, id, orgID)
	return err
}

// DeleteBySource deletes all rules for an org with the given source.
func (s *RuleStore) DeleteBySource(ctx context.Context, orgID, source string) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM rules WHERE org_id = $1 AND source = $2`, orgID, source)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// DeleteBySourceExcept deletes all rules for an org with the given source
// EXCEPT those whose IDs are in the keep set. Used for clean sync.
func (s *RuleStore) DeleteBySourceExcept(ctx context.Context, orgID, source string, keepIDs []string) (int, error) {
	if len(keepIDs) == 0 {
		res, err := s.db.ExecContext(ctx,
			`DELETE FROM rules WHERE org_id = $1 AND source = $2`, orgID, source)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		return int(n), nil
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM rules WHERE org_id = $1 AND source = $2 AND id != ALL($3)`,
		orgID, source, pq.Array(keepIDs))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *RuleStore) CountBySource(ctx context.Context, orgID, source string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM rules WHERE org_id = $1 AND source = $2`, orgID, source).Scan(&count)
	return count, err
}

// GetForAgent returns all enabled rules for an agent's organization and
// computes an ETag based on rule IDs and versions. The ETag allows agents
// to skip downloading rules that haven't changed.
func (s *RuleStore) GetForAgent(ctx context.Context, orgID, agentID string) ([]*fleet.Rule, string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, name, version, description, condition, output_template,
			severity, labels, tags, "references", raw_yaml, enabled,
			COALESCE(validation_status, 'pending'), COALESCE(validation_errors, '[]'),
			COALESCE(source, 'manual'), created_at, updated_at
		 FROM rules
		 WHERE org_id = $1 AND enabled = true AND COALESCE(validation_status, 'pending') = 'valid'
		 ORDER BY name ASC`,
		orgID,
	)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	rules := make([]*fleet.Rule, 0)
	for rows.Next() {
		r, err := scanRuleRows(rows)
		if err != nil {
			return nil, "", err
		}
		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	// Compute ETag from sorted rule ID:version pairs
	etag := computeRulesETag(rules)
	return rules, etag, nil
}

func computeRulesETag(rules []*fleet.Rule) string {
	pairs := make([]string, len(rules))
	for i, r := range rules {
		pairs[i] = r.ID + ":" + r.Version
	}
	sort.Strings(pairs)

	h := sha256.New()
	for _, p := range pairs {
		h.Write([]byte(p))
	}
	return fmt.Sprintf(`"%x"`, h.Sum(nil)[:16])
}

func scanRule(row *sql.Row) (*fleet.Rule, error) {
	r := &fleet.Rule{}
	var labelsJSON []byte
	var validationErrorsJSON []byte
	err := row.Scan(
		&r.ID, &r.OrgID, &r.Name, &r.Version, &r.Description,
		&r.Condition, &r.Output, &r.Severity, &labelsJSON,
		pq.Array(&r.Tags), pq.Array(&r.References), &r.RawYAML, &r.Enabled,
		&r.ValidationStatus, &validationErrorsJSON,
		&r.Source, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	json.Unmarshal(labelsJSON, &r.Labels)
	r.ValidationErrors = validationErrorsJSON
	return r, nil
}

func scanRuleRows(rows *sql.Rows) (*fleet.Rule, error) {
	r := &fleet.Rule{}
	var labelsJSON []byte
	var validationErrorsJSON []byte
	var tagsRaw, refsRaw []sql.NullString
	err := rows.Scan(
		&r.ID, &r.OrgID, &r.Name, &r.Version, &r.Description,
		&r.Condition, &r.Output, &r.Severity, &labelsJSON,
		pq.Array(&tagsRaw), pq.Array(&refsRaw), &r.RawYAML, &r.Enabled,
		&r.ValidationStatus, &validationErrorsJSON,
		&r.Source, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(labelsJSON, &r.Labels)
	r.ValidationErrors = validationErrorsJSON
	for _, t := range tagsRaw {
		if t.Valid {
			r.Tags = append(r.Tags, t.String)
		}
	}
	for _, ref := range refsRaw {
		if ref.Valid {
			r.References = append(r.References, ref.String)
		}
	}
	return r, nil
}
