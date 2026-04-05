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
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	log "github.com/sirupsen/logrus"
)

// TelemetryStore implements store.TelemetryStore backed by PostgreSQL.
type TelemetryStore struct {
	db *sql.DB
}

// NewTelemetryStore creates a new telemetry store.
func NewTelemetryStore(db *sql.DB) *TelemetryStore {
	return &TelemetryStore{db: db}
}

func (s *TelemetryStore) BulkIngest(ctx context.Context, orgID, agentID, hostname string, events []json.RawMessage) error {
	stmt, err := s.db.PrepareContext(ctx,
		`INSERT INTO telemetry_events (org_id, agent_id, agent_hostname, seq, timestamp,
			event_name, event_category, pid, tid, process_name, process_exe,
			process_cmdline, parent_pid, parent_name, params, metadata, raw_event)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`)
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
		var pid, tid, parentPID int
		var processName, processExe, processCmdline, parentName string
		var params, metadata json.RawMessage

		jsonInt64(&seq, evt["seq"])
		jsonTime(&ts, evt["timestamp"])
		jsonString(&eventName, evt["name"])
		jsonString(&eventCategory, evt["category"])
		jsonInt(&pid, evt["pid"])
		jsonInt(&tid, evt["tid"])

		// Extract process fields from the "ps" object
		if psRaw, ok := evt["ps"]; ok {
			var ps map[string]json.RawMessage
			if json.Unmarshal(psRaw, &ps) == nil {
				jsonString(&processName, ps["name"])
				jsonString(&processExe, ps["exe"])
				jsonString(&processCmdline, ps["cmdline"])
				jsonInt(&parentPID, ps["ppid"])
				// parent.name is nested: ps.parent.name
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
		metadata = evt["meta"]
		if metadata == nil {
			metadata = json.RawMessage(`{}`)
		}

		if ts.IsZero() {
			ts = time.Now().UTC()
		}

		_, err := stmt.ExecContext(ctx,
			orgID, agentID, hostname, seq, ts,
			eventName, eventCategory, pid, tid,
			sanitizeUTF8(processName), sanitizeUTF8(processExe),
			sanitizeUTF8(processCmdline), parentPID, sanitizeUTF8(parentName),
			sanitizeUTF8JSON(params), sanitizeUTF8JSON(metadata), sanitizeUTF8JSON(raw),
		)
		if err != nil {
			log.Warnf("fleet: skip event insert: %v", err)
			continue
		}
	}

	return nil
}

func (s *TelemetryStore) Search(ctx context.Context, orgID string, opts store.TelemetrySearchOpts) ([]store.TelemetryEvent, int, error) {
	query := `SELECT id, org_id, agent_id, agent_hostname, seq, timestamp,
				event_name, event_category, pid, tid, process_name, process_exe,
				process_cmdline, parent_pid, parent_name, params, metadata, raw_event
			  FROM telemetry_events WHERE org_id = $1`
	countQuery := `SELECT COUNT(*) FROM telemetry_events WHERE org_id = $1`
	args := []interface{}{orgID}
	argIdx := 2

	if opts.AgentID != "" {
		clause := fmt.Sprintf(" AND agent_id = $%d", argIdx)
		query += clause
		countQuery += clause
		args = append(args, opts.AgentID)
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
		clause := fmt.Sprintf(" AND process_name = $%d", argIdx)
		query += clause
		countQuery += clause
		args = append(args, opts.ProcessName)
		argIdx++
	}
	if opts.PID != 0 {
		clause := fmt.Sprintf(" AND pid = $%d", argIdx)
		query += clause
		countQuery += clause
		args = append(args, opts.PID)
		argIdx++
	}
	if opts.Search != "" {
		like := "%" + opts.Search + "%"
		clause := fmt.Sprintf(" AND (process_name ILIKE $%d OR process_exe ILIKE $%d OR process_cmdline ILIKE $%d OR event_name ILIKE $%d)", argIdx, argIdx+1, argIdx+2, argIdx+3)
		query += clause
		countQuery += clause
		args = append(args, like, like, like, like)
		argIdx += 4
	}
	if !opts.From.IsZero() {
		clause := fmt.Sprintf(" AND timestamp >= $%d", argIdx)
		query += clause
		countQuery += clause
		args = append(args, opts.From)
		argIdx++
	}
	if !opts.To.IsZero() {
		clause := fmt.Sprintf(" AND timestamp <= $%d", argIdx)
		query += clause
		countQuery += clause
		args = append(args, opts.To)
		argIdx++
	}
	// Fibratus QL query
	if opts.Query != "" {
		qlClause, qlArgs, newIdx, err := store.ParseQueryToSQL(opts.Query, "postgres", argIdx)
		if err == nil && qlClause != "" {
			clause := " AND (" + qlClause + ")"
			query += clause
			countQuery += clause
			args = append(args, qlArgs...)
			argIdx = newIdx
		}
	}

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query += " ORDER BY timestamp DESC"

	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	events := make([]store.TelemetryEvent, 0)
	for rows.Next() {
		var evt store.TelemetryEvent
		err := rows.Scan(
			&evt.ID, &evt.OrgID, &evt.AgentID, &evt.AgentHostname, &evt.Seq, &evt.Timestamp,
			&evt.EventName, &evt.EventCategory, &evt.PID, &evt.TID, &evt.ProcessName, &evt.ProcessExe,
			&evt.ProcessCmdline, &evt.ParentPID, &evt.ParentName, &evt.Params, &evt.Metadata, &evt.RawEvent,
		)
		if err != nil {
			return nil, 0, err
		}
		events = append(events, evt)
	}

	return events, total, rows.Err()
}

func (s *TelemetryStore) GetLatestForAgent(ctx context.Context, orgID, agentID string, limit int) ([]store.TelemetryEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, agent_id, agent_hostname, seq, timestamp,
			event_name, event_category, pid, tid, process_name, process_exe,
			process_cmdline, parent_pid, parent_name, params, metadata, raw_event
		 FROM telemetry_events WHERE org_id = $1 AND agent_id = $2
		 ORDER BY timestamp DESC LIMIT $3`,
		orgID, agentID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]store.TelemetryEvent, 0)
	for rows.Next() {
		var evt store.TelemetryEvent
		err := rows.Scan(
			&evt.ID, &evt.OrgID, &evt.AgentID, &evt.AgentHostname, &evt.Seq, &evt.Timestamp,
			&evt.EventName, &evt.EventCategory, &evt.PID, &evt.TID, &evt.ProcessName, &evt.ProcessExe,
			&evt.ProcessCmdline, &evt.ParentPID, &evt.ParentName, &evt.Params, &evt.Metadata, &evt.RawEvent,
		)
		if err != nil {
			return nil, err
		}
		events = append(events, evt)
	}

	return events, rows.Err()
}

func (s *TelemetryStore) Purge(ctx context.Context, retentionDays int) (int64, error) {
	// retentionDays is interpreted as minutes when < 1 (dev mode)
	query := `DELETE FROM telemetry_events WHERE timestamp < NOW() - $1 * interval '1 day'`
	if retentionDays == 0 {
		query = `DELETE FROM telemetry_events WHERE timestamp < NOW() - interval '10 minutes'`
	}
	result, err := s.db.ExecContext(ctx, query, retentionDays)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *TelemetryStore) CountByAgent(ctx context.Context, orgID string) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT agent_id, COUNT(*) FROM telemetry_events WHERE org_id = $1 GROUP BY agent_id`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var agentID string
		var count int64
		if err := rows.Scan(&agentID, &count); err != nil {
			return nil, err
		}
		counts[agentID] = count
	}
	return counts, rows.Err()
}

// sanitizeUTF8 replaces invalid UTF-8 bytes with the Unicode replacement character.
func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "\uFFFD")
}

// sanitizeUTF8JSON replaces invalid UTF-8 bytes in raw JSON.
func sanitizeUTF8JSON(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return raw
	}
	s := string(raw)
	if utf8.ValidString(s) {
		return raw
	}
	return json.RawMessage(strings.ToValidUTF8(s, "\uFFFD"))
}

// jsonString extracts a string from a JSON raw value.
func jsonString(dst *string, raw json.RawMessage) {
	if raw == nil {
		return
	}
	json.Unmarshal(raw, dst)
}

// jsonInt extracts an int from a JSON raw value.
func jsonInt(dst *int, raw json.RawMessage) {
	if raw == nil {
		return
	}
	json.Unmarshal(raw, dst)
}

// jsonInt64 extracts an int64 from a JSON raw value.
func jsonInt64(dst *int64, raw json.RawMessage) {
	if raw == nil {
		return
	}
	json.Unmarshal(raw, dst)
}

// jsonTime extracts a time.Time from a JSON raw value (RFC3339 string or Unix timestamp).
func jsonTime(dst *time.Time, raw json.RawMessage) {
	if raw == nil {
		return
	}
	// Try string first (RFC3339)
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			*dst = t
			return
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			*dst = t
			return
		}
	}
	// Try numeric (Unix epoch seconds)
	var n float64
	if json.Unmarshal(raw, &n) == nil && n > 0 {
		*dst = time.Unix(int64(n), 0).UTC()
	}
}
