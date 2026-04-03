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
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/rabbitstack/fibratus/pkg/fleet"
)

// DetectionStore implements store.DetectionStore backed by PostgreSQL.
type DetectionStore struct {
	db *sql.DB
}

// NewDetectionStore creates a new detection store.
func NewDetectionStore(db *sql.DB) *DetectionStore {
	return &DetectionStore{db: db}
}

func (s *DetectionStore) Create(ctx context.Context, det *fleet.Detection) error {
	labels, _ := json.Marshal(det.Labels)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO detections (id, org_id, agent_id, agent_hostname, rule_id, rule_name,
			title, text, description, severity, labels, tags, events, timestamp)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		det.ID, det.OrgID, det.AgentID, det.AgentHostname, det.RuleID, det.RuleName,
		det.Title, det.Text, det.Description, det.Severity, labels,
		pq.Array(det.Tags), det.Events, det.Timestamp,
	)
	return err
}

func (s *DetectionStore) Get(ctx context.Context, orgID, id string) (*fleet.Detection, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, agent_id, agent_hostname, rule_id, rule_name,
			title, text, description, severity, labels, tags, events, timestamp
		 FROM detections WHERE id = $1 AND org_id = $2`, id, orgID)

	det := &fleet.Detection{}
	var labelsJSON []byte
	err := row.Scan(
		&det.ID, &det.OrgID, &det.AgentID, &det.AgentHostname, &det.RuleID, &det.RuleName,
		&det.Title, &det.Text, &det.Description, &det.Severity, &labelsJSON,
		pq.Array(&det.Tags), &det.Events, &det.Timestamp,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	json.Unmarshal(labelsJSON, &det.Labels)
	return det, nil
}

func (s *DetectionStore) List(ctx context.Context, orgID string, opts fleet.DetectionListOptions) ([]*fleet.Detection, int, error) {
	query := `SELECT id, org_id, agent_id, agent_hostname, rule_id, rule_name,
				title, text, description, severity, labels, tags, events, timestamp
			  FROM detections WHERE org_id = $1`
	countQuery := `SELECT COUNT(*) FROM detections WHERE org_id = $1`
	args := []interface{}{orgID}
	argIdx := 2

	if opts.AgentID != "" {
		clause := fmt.Sprintf(" AND agent_id = $%d", argIdx)
		query += clause
		countQuery += clause
		args = append(args, opts.AgentID)
		argIdx++
	}
	if opts.Severity != "" {
		clause := fmt.Sprintf(" AND severity = $%d", argIdx)
		query += clause
		countQuery += clause
		args = append(args, opts.Severity)
		argIdx++
	}
	if opts.RuleID != "" {
		clause := fmt.Sprintf(" AND rule_id = $%d", argIdx)
		query += clause
		countQuery += clause
		args = append(args, opts.RuleID)
		argIdx++
	}
	if opts.From != "" {
		clause := fmt.Sprintf(" AND timestamp >= $%d", argIdx)
		query += clause
		countQuery += clause
		args = append(args, opts.From)
		argIdx++
	}
	if opts.To != "" {
		clause := fmt.Sprintf(" AND timestamp <= $%d", argIdx)
		query += clause
		countQuery += clause
		args = append(args, opts.To)
		argIdx++
	}

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query += " ORDER BY timestamp DESC"

	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = 50
	}
	page := opts.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * perPage
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, perPage, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	detections := make([]*fleet.Detection, 0)
	for rows.Next() {
		det := &fleet.Detection{}
		var labelsJSON []byte
		err := rows.Scan(
			&det.ID, &det.OrgID, &det.AgentID, &det.AgentHostname, &det.RuleID, &det.RuleName,
			&det.Title, &det.Text, &det.Description, &det.Severity, &labelsJSON,
			pq.Array(&det.Tags), &det.Events, &det.Timestamp,
		)
		if err != nil {
			return nil, 0, err
		}
		json.Unmarshal(labelsJSON, &det.Labels)
		detections = append(detections, det)
	}

	return detections, total, rows.Err()
}

func (s *DetectionStore) Count24h(ctx context.Context, orgID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM detections WHERE org_id = $1 AND timestamp > $2`,
		orgID, time.Now().UTC().Add(-24*time.Hour),
	).Scan(&count)
	return count, err
}

func (s *DetectionStore) CountBySeverity(ctx context.Context, orgID string) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT severity, COUNT(*) FROM detections
		 WHERE org_id = $1 AND timestamp > $2 GROUP BY severity`,
		orgID, time.Now().UTC().Add(-24*time.Hour),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var severity string
		var count int
		if err := rows.Scan(&severity, &count); err != nil {
			return nil, err
		}
		counts[severity] = count
	}
	return counts, rows.Err()
}

func (s *DetectionStore) Timeline(ctx context.Context, orgID string, from, to time.Time, interval string) ([]fleet.TimelineBucket, error) {
	bucket := "hour"
	switch interval {
	case "day":
		bucket = "day"
	case "minute":
		bucket = "minute"
	}

	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT date_trunc('%s', timestamp) as bucket, severity, COUNT(*)
		 FROM detections
		 WHERE org_id = $1 AND timestamp BETWEEN $2 AND $3
		 GROUP BY bucket, severity
		 ORDER BY bucket`, bucket),
		orgID, from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bucketMap := make(map[time.Time]*fleet.TimelineBucket)
	for rows.Next() {
		var ts time.Time
		var severity string
		var count int
		if err := rows.Scan(&ts, &severity, &count); err != nil {
			return nil, err
		}
		b, ok := bucketMap[ts]
		if !ok {
			b = &fleet.TimelineBucket{
				Timestamp:  ts,
				BySeverity: make(map[string]int),
			}
			bucketMap[ts] = b
		}
		b.Count += count
		b.BySeverity[severity] = count
	}

	result := make([]fleet.TimelineBucket, 0, len(bucketMap))
	for _, b := range bucketMap {
		result = append(result, *b)
	}
	return result, rows.Err()
}

func (s *DetectionStore) MitreHeatmap(ctx context.Context, orgID string, from, to time.Time) ([]fleet.MitreCell, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT
			labels->>'tactic.id' as tactic_id,
			labels->>'tactic.name' as tactic_name,
			labels->>'technique.id' as technique_id,
			labels->>'technique.name' as technique_name,
			COUNT(*) as count
		 FROM detections
		 WHERE org_id = $1 AND timestamp BETWEEN $2 AND $3
			AND labels->>'tactic.id' IS NOT NULL
			AND labels->>'technique.id' IS NOT NULL
		 GROUP BY tactic_id, tactic_name, technique_id, technique_name
		 ORDER BY count DESC`,
		orgID, from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cells := make([]fleet.MitreCell, 0)
	for rows.Next() {
		var cell fleet.MitreCell
		if err := rows.Scan(&cell.TacticID, &cell.TacticName, &cell.TechniqueID, &cell.TechniqueName, &cell.Count); err != nil {
			return nil, err
		}
		cells = append(cells, cell)
	}
	return cells, rows.Err()
}
