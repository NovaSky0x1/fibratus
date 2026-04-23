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
	// Resolve account_id from org if the caller didn't set it explicitly
	// (keeps the older org-scoped callers working while new account-scoped
	// management sets AccountID directly).
	accountID := rule.AccountID
	if accountID == "" {
		_ = s.db.QueryRowContext(ctx,
			`SELECT account_id FROM organizations WHERE id = $1`, rule.OrgID,
		).Scan(&accountID)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO rules (id, org_id, account_id, name, version, description, condition, output_template,
			severity, labels, tags, "references", raw_yaml, enabled, source, validation_status, validation_errors,
			user_modified, user_disabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, NOW(), NOW())`,
		rule.ID, rule.OrgID, accountID, rule.Name, rule.Version, rule.Description,
		rule.Condition, rule.Output, rule.Severity, labels,
		pq.Array(rule.Tags), pq.Array(rule.References), rule.RawYAML, rule.Enabled,
		rule.Source, rule.ValidationStatus, validationErrors,
		rule.UserModified, rule.UserDisabled,
	)
	return err
}

func (s *RuleStore) Get(ctx context.Context, orgID, id string) (*fleet.Rule, error) {
	// Account-scoped: return the rule if the caller's org is in the
	// same account as the rule. Keeps the existing (orgID, id) signature
	// so callers that already resolved an org continue to work.
	row := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, name, version, description, condition, output_template,
			severity, labels, tags, "references", raw_yaml, enabled,
			COALESCE(validation_status, 'pending'), COALESCE(validation_errors, '[]'),
			COALESCE(source, 'manual'), created_at, updated_at,
			user_modified, user_disabled
		 FROM rules
		 WHERE id = $1 AND account_id = (SELECT account_id FROM organizations WHERE id = $2)`,
		id, orgID)
	return scanRule(row)
}

func (s *RuleStore) List(ctx context.Context, orgID string, opts fleet.ListOptions) ([]*fleet.Rule, int, error) {
	// Account-scoped listing — every org in an account sees the same
	// rule set. Without this, rules created from one org were invisible
	// to agents and users in sibling orgs under the same account.
	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM rules
		 WHERE account_id = (SELECT account_id FROM organizations WHERE id = $1)`, orgID,
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
			COALESCE(source, 'manual'), created_at, updated_at,
			user_modified, user_disabled
		 FROM rules
		 WHERE account_id = (SELECT account_id FROM organizations WHERE id = $1)
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
	// Match by id alone and verify the caller's org belongs to the rule's
	// account. Rules are account-scoped now — any org in the same account
	// can edit any rule; a cross-account edit is rejected.
	_, err := s.db.ExecContext(ctx,
		`UPDATE rules SET name=$3, version=$4, description=$5, condition=$6,
			output_template=$7, severity=$8, labels=$9, tags=$10, "references"=$11,
			raw_yaml=$12, enabled=$13, source=$14, validation_status=$15, validation_errors=$16,
			user_modified=$17, user_disabled=$18, updated_at=NOW()
		 WHERE id=$1 AND account_id = (SELECT account_id FROM organizations WHERE id=$2)`,
		rule.ID, rule.OrgID, rule.Name, rule.Version, rule.Description,
		rule.Condition, rule.Output, rule.Severity, labels,
		pq.Array(rule.Tags), pq.Array(rule.References), rule.RawYAML, rule.Enabled,
		rule.Source, rule.ValidationStatus, validationErrors,
		rule.UserModified, rule.UserDisabled,
	)
	return err
}

func (s *RuleStore) Delete(ctx context.Context, orgID, id string) error {
	// Account-scoped delete: any org in the account may delete the rule.
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM rules WHERE id=$1 AND account_id = (SELECT account_id FROM organizations WHERE id=$2)`,
		id, orgID)
	return err
}

// UpdateAcrossAccount propagates a rule update to all orgs in the same account.
// Used for account-wide rule management — when a user toggles or edits a rule,
// the change applies to every org, not just the current one.
func (s *RuleStore) UpdateAcrossAccount(ctx context.Context, accountID string, rule *fleet.Rule) (int, error) {
	labels, _ := json.Marshal(rule.Labels)
	validationErrors := rule.ValidationErrors
	if validationErrors == nil {
		validationErrors = json.RawMessage(`[]`)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE rules SET
			name=$3, version=$4, description=$5, condition=$6, output_template=$7,
			severity=$8, labels=$9, tags=$10, "references"=$11,
			raw_yaml=$12, enabled=$13, validation_status=$14, validation_errors=$15,
			user_modified=$16, user_disabled=$17, updated_at=NOW()
		 WHERE id=$1 AND org_id IN (SELECT id FROM organizations WHERE account_id=$2)`,
		rule.ID, accountID, rule.Name, rule.Version, rule.Description,
		rule.Condition, rule.Output, rule.Severity, labels,
		pq.Array(rule.Tags), pq.Array(rule.References), rule.RawYAML, rule.Enabled,
		rule.ValidationStatus, validationErrors,
		rule.UserModified, rule.UserDisabled,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// DeleteAcrossAccount deletes a rule from all orgs in the same account.
func (s *RuleStore) DeleteAcrossAccount(ctx context.Context, accountID, ruleID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM rules WHERE id=$1 AND org_id IN (SELECT id FROM organizations WHERE account_id=$2)`,
		ruleID, accountID)
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
	// Rules are account-scoped: fetch every enabled/valid rule for the
	// account that owns this agent's org so account-wide rule edits reach
	// every agent regardless of which org it's registered in. account_id
	// is resolved from organizations at query time to keep this method's
	// existing (orgID, agentID) signature and avoid churning every caller.
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, name, version, description, condition, output_template,
			severity, labels, tags, "references", raw_yaml, enabled,
			COALESCE(validation_status, 'pending'), COALESCE(validation_errors, '[]'),
			COALESCE(source, 'manual'), created_at, updated_at,
			user_modified, user_disabled
		 FROM rules
		 WHERE account_id = (SELECT account_id FROM organizations WHERE id = $1)
		   AND enabled = true AND COALESCE(validation_status, 'pending') = 'valid'
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
		&r.UserModified, &r.UserDisabled,
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
		&r.UserModified, &r.UserDisabled,
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

// ListUserModifiedIDs returns the IDs of rules that have been modified or disabled by the user.
func (s *RuleStore) ListUserModifiedIDs(ctx context.Context, orgID, source string) (modified, disabled map[string]bool, err error) {
	modified = make(map[string]bool)
	disabled = make(map[string]bool)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_modified, user_disabled FROM rules WHERE org_id = $1 AND source = $2 AND (user_modified = true OR user_disabled = true)`,
		orgID, source)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var mod, dis bool
		rows.Scan(&id, &mod, &dis)
		if mod {
			modified[id] = true
		}
		if dis {
			disabled[id] = true
		}
	}
	return
}

// RecordSyncDeletion records that a user deleted a synced rule so it won't be re-created on next sync.
func (s *RuleStore) RecordSyncDeletion(ctx context.Context, orgID, ruleID, source string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO deleted_sync_rules (org_id, rule_id, source) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		orgID, ruleID, source)
	return err
}

// ListDeletedSyncIDs returns the IDs of synced rules that the user has deleted.
func (s *RuleStore) ListDeletedSyncIDs(ctx context.Context, orgID, source string) (map[string]bool, error) {
	ids := make(map[string]bool)
	rows, err := s.db.QueryContext(ctx,
		`SELECT rule_id FROM deleted_sync_rules WHERE org_id = $1 AND source = $2`,
		orgID, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids[id] = true
	}
	return ids, nil
}
