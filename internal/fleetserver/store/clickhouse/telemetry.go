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

package clickhouse

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

// Schema is the ClickHouse DDL for the telemetry table.
// Uses compression codecs optimized for EDR telemetry:
//   - ZSTD for large/variable string columns (high compression)
//   - Delta+ZSTD for monotonic numerics (PIDs, sequence numbers)
//   - DoubleDelta+ZSTD for timestamps (extremely compact for time-series)
//   - LowCardinality for enum-like columns (event names, categories)
const Schema = `
CREATE TABLE IF NOT EXISTS telemetry_events (
    id             UInt64,
    org_id         LowCardinality(String),
    agent_id       LowCardinality(String),
    agent_hostname LowCardinality(String),
    seq            UInt64      CODEC(Delta, ZSTD(1)),
    timestamp      DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(1)),
    event_name     LowCardinality(String),
    event_category LowCardinality(String),
    pid            UInt32      CODEC(Delta, ZSTD(1)),
    tid            UInt32      CODEC(Delta, ZSTD(1)),
    process_name   LowCardinality(String),
    process_exe    String      CODEC(ZSTD(3)),
    process_cmdline String     CODEC(ZSTD(3)),
    parent_pid     UInt32      CODEC(Delta, ZSTD(1)),
    parent_name    LowCardinality(String),
    params         String      CODEC(ZSTD(3)),
    metadata       String      CODEC(ZSTD(3)),
    raw_event      String      CODEC(ZSTD(1))
) ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (org_id, agent_id, timestamp, pid)
TTL toDateTime(timestamp) + INTERVAL 10 MINUTE DELETE
SETTINGS index_granularity = 8192,
         min_bytes_for_wide_part = 10485760,
         merge_with_ttl_timeout = 86400
`

// TelemetryStore implements store.TelemetryStore backed by ClickHouse.
type TelemetryStore struct {
	db *sql.DB
}

// NewTelemetryStore creates a ClickHouse-backed telemetry store.
func NewTelemetryStore(db *sql.DB) *TelemetryStore {
	return &TelemetryStore{db: db}
}

// Migrate creates the telemetry table if it doesn't exist.
func (s *TelemetryStore) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, Schema)
	return err
}

// BulkIngest inserts a batch of events using a single prepared statement
// within a transaction. clickhouse-go v2 accumulates all rows in the
// transaction and sends them as one columnar block on Commit().
func (s *TelemetryStore) BulkIngest(ctx context.Context, orgID, agentID, hostname string, events []json.RawMessage) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO telemetry_events (id, org_id, agent_id, agent_hostname, seq, timestamp,
			event_name, event_category, pid, tid, process_name, process_exe,
			process_cmdline, parent_pid, parent_name, params, metadata, raw_event) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UnixNano()
	inserted := 0

	for i, raw := range events {
		var evt map[string]json.RawMessage
		if err := json.Unmarshal(raw, &evt); err != nil {
			continue
		}

		var seq int64
		var ts time.Time
		var eventName, eventCategory string
		var pid, tid, parentPID int
		var processName, processExe, processCmdline, parentName string

		jsonInt64(&seq, evt["seq"])
		jsonTime(&ts, evt["timestamp"])
		jsonString(&eventName, evt["name"])
		jsonString(&eventCategory, evt["category"])
		jsonInt(&pid, evt["pid"])
		jsonInt(&tid, evt["tid"])

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

		params := sanitizeJSON(evt["params"])
		metadata := sanitizeJSON(evt["meta"])
		rawEvent := sanitizeJSON(raw)

		if ts.IsZero() {
			ts = time.Now().UTC()
		}

		id := uint64(now) + uint64(i)

		if _, err := stmt.ExecContext(ctx,
			id, orgID, agentID, hostname, seq, ts,
			eventName, eventCategory, pid, tid,
			sanitizeUTF8(processName), sanitizeUTF8(processExe),
			sanitizeUTF8(processCmdline), parentPID, sanitizeUTF8(parentName),
			params, metadata, rawEvent,
		); err != nil {
			log.Warnf("fleet: skip clickhouse row: %v", err)
			continue
		}
		inserted++
	}

	if inserted == 0 {
		tx.Rollback()
		return nil
	}

	return tx.Commit()
}

func (s *TelemetryStore) Search(ctx context.Context, orgID string, opts store.TelemetrySearchOpts) ([]store.TelemetryEvent, int, error) {
	query := `SELECT id, org_id, agent_id, agent_hostname, seq, timestamp,
				event_name, event_category, pid, tid, process_name, process_exe,
				process_cmdline, parent_pid, parent_name, params, metadata, raw_event
			  FROM telemetry_events WHERE org_id = ?`
	countQuery := `SELECT count() FROM telemetry_events WHERE org_id = ?`
	args := []interface{}{orgID}

	if opts.AgentID != "" {
		query += " AND agent_id = ?"
		countQuery += " AND agent_id = ?"
		args = append(args, opts.AgentID)
	}
	if opts.EventName != "" {
		query += " AND event_name = ?"
		countQuery += " AND event_name = ?"
		args = append(args, opts.EventName)
	}
	if opts.ProcessName != "" {
		query += " AND process_name = ?"
		countQuery += " AND process_name = ?"
		args = append(args, opts.ProcessName)
	}
	if opts.PID > 0 {
		query += " AND pid = ?"
		countQuery += " AND pid = ?"
		args = append(args, opts.PID)
	}
	if opts.Search != "" {
		like := "%" + opts.Search + "%"
		query += " AND (process_name ILIKE ? OR process_exe ILIKE ? OR process_cmdline ILIKE ? OR event_name ILIKE ?)"
		countQuery += " AND (process_name ILIKE ? OR process_exe ILIKE ? OR process_cmdline ILIKE ? OR event_name ILIKE ?)"
		args = append(args, like, like, like, like)
	}
	if !opts.From.IsZero() {
		query += " AND timestamp >= ?"
		countQuery += " AND timestamp >= ?"
		args = append(args, opts.From)
	}
	if !opts.To.IsZero() {
		query += " AND timestamp <= ?"
		countQuery += " AND timestamp <= ?"
		args = append(args, opts.To)
	}
	if opts.Query != "" {
		qlClause, qlArgs, _, err := store.ParseQueryToSQL(opts.Query, "clickhouse", 0)
		if err == nil && qlClause != "" {
			clause := " AND (" + qlClause + ")"
			query += clause
			countQuery += clause
			args = append(args, qlArgs...)
		}
	}

	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total)

	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	query += " ORDER BY timestamp DESC LIMIT ? OFFSET ?"
	args = append(args, opts.Limit, opts.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	events := make([]store.TelemetryEvent, 0)
	for rows.Next() {
		var evt store.TelemetryEvent
		var params, metadata, rawEvent string
		err := rows.Scan(
			&evt.ID, &evt.OrgID, &evt.AgentID, &evt.AgentHostname, &evt.Seq, &evt.Timestamp,
			&evt.EventName, &evt.EventCategory, &evt.PID, &evt.TID, &evt.ProcessName, &evt.ProcessExe,
			&evt.ProcessCmdline, &evt.ParentPID, &evt.ParentName, &params, &metadata, &rawEvent,
		)
		if err != nil {
			return nil, 0, err
		}
		evt.Params = json.RawMessage(params)
		evt.Metadata = json.RawMessage(metadata)
		evt.RawEvent = json.RawMessage(rawEvent)
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
		 FROM telemetry_events WHERE org_id = ? AND agent_id = ?
		 ORDER BY timestamp DESC LIMIT ?`,
		orgID, agentID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]store.TelemetryEvent, 0)
	for rows.Next() {
		var evt store.TelemetryEvent
		var params, metadata, rawEvent string
		err := rows.Scan(
			&evt.ID, &evt.OrgID, &evt.AgentID, &evt.AgentHostname, &evt.Seq, &evt.Timestamp,
			&evt.EventName, &evt.EventCategory, &evt.PID, &evt.TID, &evt.ProcessName, &evt.ProcessExe,
			&evt.ProcessCmdline, &evt.ParentPID, &evt.ParentName, &params, &metadata, &rawEvent,
		)
		if err != nil {
			return nil, err
		}
		evt.Params = json.RawMessage(params)
		evt.Metadata = json.RawMessage(metadata)
		evt.RawEvent = json.RawMessage(rawEvent)
		events = append(events, evt)
	}

	return events, rows.Err()
}

func (s *TelemetryStore) Purge(ctx context.Context, retentionDays int) (int64, error) {
	// ClickHouse TTL handles retention automatically.
	return 0, nil
}

func (s *TelemetryStore) CountByAgent(ctx context.Context, orgID string) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT agent_id, count() FROM telemetry_events WHERE org_id = ? GROUP BY agent_id`,
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

// Helper functions

func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "\uFFFD")
}

func sanitizeJSON(raw json.RawMessage) string {
	if raw == nil {
		return "{}"
	}
	s := string(raw)
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "\uFFFD")
}

func jsonString(dst *string, raw json.RawMessage) {
	if raw == nil {
		return
	}
	json.Unmarshal(raw, dst)
}

func jsonInt(dst *int, raw json.RawMessage) {
	if raw == nil {
		return
	}
	json.Unmarshal(raw, dst)
}

func jsonInt64(dst *int64, raw json.RawMessage) {
	if raw == nil {
		return
	}
	json.Unmarshal(raw, dst)
}

func jsonTime(dst *time.Time, raw json.RawMessage) {
	if raw == nil {
		return
	}
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
	var n float64
	if json.Unmarshal(raw, &n) == nil && n > 0 {
		*dst = time.Unix(int64(n), 0).UTC()
	}
}
