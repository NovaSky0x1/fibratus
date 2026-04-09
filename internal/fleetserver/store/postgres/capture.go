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

	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// CaptureStore implements store.CaptureStore backed by PostgreSQL.
type CaptureStore struct {
	db *sql.DB
}

// NewCaptureStore creates a new capture store.
func NewCaptureStore(db *sql.DB) *CaptureStore {
	return &CaptureStore{db: db}
}

func (s *CaptureStore) Create(ctx context.Context, cap *fleet.Capture) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO captures (id, org_id, agent_id, agent_hostname, filter, status, duration_sec, created_by, started_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		cap.ID, cap.OrgID, cap.AgentID, cap.AgentHostname, cap.Filter, cap.Status, cap.DurationSec, cap.CreatedBy, cap.StartedAt,
	)
	return err
}

func (s *CaptureStore) Get(ctx context.Context, orgID, id string) (*fleet.Capture, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, agent_id, agent_hostname, filter, status, event_count, duration_sec, created_by, started_at, completed_at
		 FROM captures WHERE org_id = $1 AND id = $2`, orgID, id)

	return scanCapture(row)
}

func (s *CaptureStore) ListByAgent(ctx context.Context, orgID, agentID string) ([]*fleet.Capture, error) {
	query := `SELECT id, org_id, agent_id, agent_hostname, filter, status, event_count, duration_sec, created_by, started_at, completed_at
			  FROM captures WHERE org_id = $1`
	args := []interface{}{orgID}

	if agentID != "" {
		query += ` AND agent_id = $2`
		args = append(args, agentID)
	}
	query += ` ORDER BY started_at DESC LIMIT 50`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	captures := make([]*fleet.Capture, 0)
	for rows.Next() {
		cap, err := scanCaptureRows(rows)
		if err != nil {
			return nil, err
		}
		captures = append(captures, cap)
	}
	return captures, rows.Err()
}

func (s *CaptureStore) Update(ctx context.Context, cap *fleet.Capture) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE captures SET status = $1, event_count = $2, completed_at = $3 WHERE id = $4`,
		cap.Status, cap.EventCount, cap.CompletedAt, cap.ID,
	)
	return err
}

func (s *CaptureStore) Delete(ctx context.Context, orgID, id string) error {
	// capture_events cascade-deleted via FK
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM captures WHERE org_id = $1 AND id = $2`, orgID, id)
	return err
}

func (s *CaptureStore) IncrementEventCount(ctx context.Context, captureID string, count int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE captures SET event_count = event_count + $1 WHERE id = $2`, count, captureID)
	return err
}

func (s *CaptureStore) IngestEvents(ctx context.Context, captureID, orgID string, events []json.RawMessage) error {
	stmt, err := s.db.PrepareContext(ctx,
		`INSERT INTO capture_events (capture_id, org_id, seq, timestamp,
			event_name, event_category, pid, process_name, process_exe,
			process_cmdline, parent_pid, parent_name, params, raw_event)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, raw := range events {
		var evt map[string]json.RawMessage
		if err := json.Unmarshal(raw, &evt); err != nil {
			continue
		}

		var seq int64
		var ts time.Time
		var eventName, eventCategory string
		var pid, parentPID int
		var processName, processExe, processCmdline, parentName string
		var params json.RawMessage

		jsonInt64(&seq, evt["seq"])
		jsonTime(&ts, evt["timestamp"])
		jsonString(&eventName, evt["name"])
		jsonString(&eventCategory, evt["category"])
		jsonInt(&pid, evt["pid"])

		if psRaw, ok := evt["ps"]; ok {
			var ps map[string]json.RawMessage
			if json.Unmarshal(psRaw, &ps) == nil {
				jsonString(&processName, ps["name"])
				jsonString(&processExe, ps["exe"])
				jsonString(&processCmdline, ps["cmdline"])
				jsonInt(&parentPID, ps["ppid"])
				if parentRaw, ok := ps["parent"]; ok {
					var parent map[string]json.RawMessage
					if json.Unmarshal(parentRaw, &parent) == nil {
						jsonString(&parentName, parent["name"])
					}
				}
			}
		}

		params = evt["params"]
		if params == nil {
			params = json.RawMessage(`{}`)
		}

		if ts.IsZero() {
			ts = time.Now().UTC()
		}

		_, err := stmt.ExecContext(ctx,
			captureID, orgID, seq, ts,
			eventName, eventCategory, pid,
			sanitizeUTF8(processName), sanitizeUTF8(processExe),
			sanitizeUTF8(processCmdline), parentPID, sanitizeUTF8(parentName),
			sanitizeUTF8JSON(params), sanitizeUTF8JSON(raw),
		)
		if err != nil {
			log.Warnf("fleet: skip capture event insert: %v", err)
			continue
		}
	}

	return nil
}

func (s *CaptureStore) GetEvents(ctx context.Context, captureID string, opts store.CaptureEventSearchOpts) ([]store.CaptureEvent, int, error) {
	query := `SELECT id, capture_id, org_id, seq, timestamp,
				event_name, event_category, pid, process_name, process_exe,
				process_cmdline, parent_pid, parent_name, params, raw_event
			  FROM capture_events WHERE capture_id = $1`
	countQuery := `SELECT COUNT(*) FROM capture_events WHERE capture_id = $1`
	args := []interface{}{captureID}
	argIdx := 2

	if opts.AfterId > 0 {
		clause := fmt.Sprintf(" AND id > $%d", argIdx)
		query += clause
		// Don't add to count query — AfterId is for cursor pagination
		args = append(args, opts.AfterId)
		argIdx++
	}
	if opts.EventName != "" {
		clause := fmt.Sprintf(" AND event_name = $%d", argIdx)
		query += clause
		countQuery += clause
		args = append(args, opts.EventName)
		argIdx++
	}
	if opts.ProcessName != "" {
		clause := fmt.Sprintf(" AND process_name ILIKE $%d", argIdx)
		query += clause
		countQuery += clause
		args = append(args, "%"+opts.ProcessName+"%")
		argIdx++
	}
	if opts.PID > 0 {
		clause := fmt.Sprintf(" AND pid = $%d", argIdx)
		query += clause
		countQuery += clause
		args = append(args, opts.PID)
		argIdx++
	}
	if opts.Search != "" {
		clause := fmt.Sprintf(" AND (process_name ILIKE $%d OR process_exe ILIKE $%d OR process_cmdline ILIKE $%d OR event_name ILIKE $%d)", argIdx, argIdx, argIdx, argIdx)
		query += clause
		countQuery += clause
		args = append(args, "%"+opts.Search+"%")
		argIdx++
	}

	// Get total count (using only non-cursor args)
	countArgs := make([]interface{}, 0)
	countArgs = append(countArgs, captureID)
	for i, a := range args {
		if i == 0 {
			continue // skip captureID (already added)
		}
		// Skip the AfterId arg in count query
		if opts.AfterId > 0 && i == 1 {
			continue
		}
		countArgs = append(countArgs, a)
	}

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		total = 0
	}

	query += " ORDER BY id ASC"
	limit := opts.Limit
	if limit <= 0 || limit > 10000 {
		limit = 5000
	}
	query += fmt.Sprintf(" LIMIT %d", limit)
	if opts.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", opts.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	events := make([]store.CaptureEvent, 0)
	for rows.Next() {
		var e store.CaptureEvent
		if err := rows.Scan(
			&e.ID, &e.CaptureID, &e.OrgID, &e.Seq, &e.Timestamp,
			&e.EventName, &e.EventCategory, &e.PID, &e.ProcessName, &e.ProcessExe,
			&e.ProcessCmdline, &e.ParentPID, &e.ParentName, &e.Params, &e.RawEvent,
		); err != nil {
			return nil, 0, err
		}
		events = append(events, e)
	}

	return events, total, rows.Err()
}

func scanCapture(row *sql.Row) (*fleet.Capture, error) {
	c := &fleet.Capture{}
	var completedAt sql.NullTime
	err := row.Scan(
		&c.ID, &c.OrgID, &c.AgentID, &c.AgentHostname, &c.Filter,
		&c.Status, &c.EventCount, &c.DurationSec, &c.CreatedBy,
		&c.StartedAt, &completedAt,
	)
	if err != nil {
		return nil, err
	}
	if completedAt.Valid {
		c.CompletedAt = &completedAt.Time
	}
	return c, nil
}

func scanCaptureRows(rows *sql.Rows) (*fleet.Capture, error) {
	c := &fleet.Capture{}
	var completedAt sql.NullTime
	err := rows.Scan(
		&c.ID, &c.OrgID, &c.AgentID, &c.AgentHostname, &c.Filter,
		&c.Status, &c.EventCount, &c.DurationSec, &c.CreatedBy,
		&c.StartedAt, &completedAt,
	)
	if err != nil {
		return nil, err
	}
	if completedAt.Valid {
		c.CompletedAt = &completedAt.Time
	}
	return c, nil
}
