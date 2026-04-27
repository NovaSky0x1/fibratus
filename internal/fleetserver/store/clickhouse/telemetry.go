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
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	log "github.com/sirupsen/logrus"
)

// orgTableSchema generates the ClickHouse DDL for a per-org telemetry table.
// Uses compression codecs optimized for EDR telemetry:
//   - ZSTD for large/variable string columns (high compression)
//   - Delta+ZSTD for monotonic numerics (PIDs, sequence numbers)
//   - DoubleDelta+ZSTD for timestamps (extremely compact for time-series)
//   - LowCardinality for enum-like columns (event names, categories)
func orgTableSchema(tableName string, retentionDays int) string {
	if retentionDays <= 0 {
		retentionDays = 7
	}
	return fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
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
ORDER BY (agent_id, timestamp, pid)
TTL toDateTime(timestamp) + INTERVAL %d DAY DELETE
SETTINGS index_granularity = 8192,
         min_bytes_for_wide_part = 10485760,
         merge_with_ttl_timeout = 86400
`, tableName, retentionDays)
}

// Legacy shared table schema for backward compatibility during migration.
const legacySchema = `
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
TTL toDateTime(timestamp) + INTERVAL 7 DAY DELETE
SETTINGS index_granularity = 8192,
         min_bytes_for_wide_part = 10485760,
         merge_with_ttl_timeout = 86400
`

var orgIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var safeTableName = regexp.MustCompile(`[^a-z0-9_]`)

// OrgTableName generates a human-readable ClickHouse table name from org name + ID.
// e.g., "telemetry_production_e5b2a5a0" — sanitized name + 8-char ID suffix.
//
// Exported so the fleet server's retention callback (and other admin paths
// outside this package) can resolve the same table name the ingest pipeline
// uses, instead of guessing.
func OrgTableName(orgID, orgName string) string {
	if orgID == "" || !orgIDPattern.MatchString(orgID) {
		return "telemetry_events"
	}
	if orgName == "" {
		return "telemetry_" + orgID[:8]
	}
	slug := strings.ToLower(strings.TrimSpace(orgName))
	slug = strings.ReplaceAll(slug, " ", "_")
	slug = strings.ReplaceAll(slug, "-", "_")
	slug = safeTableName.ReplaceAllString(slug, "")
	if len(slug) > 30 {
		slug = slug[:30]
	}
	if slug == "" {
		slug = "org"
	}
	return fmt.Sprintf("telemetry_%s_%s", slug, orgID[:8])
}

// OrgNameResolver looks up an org's name by ID. Wired by the server at startup.
type OrgNameResolver func(orgID string) string

// TelemetryStore implements store.TelemetryStore backed by ClickHouse.
// Each organization gets its own table for data isolation and per-org retention.
type TelemetryStore struct {
	db            *sql.DB
	ensuredTables sync.Map // orgID → table name
	orgResolver   OrgNameResolver
}

// NewTelemetryStore creates a ClickHouse-backed telemetry store.
func NewTelemetryStore(db *sql.DB) *TelemetryStore {
	return &TelemetryStore{db: db}
}

// SetOrgNameResolver sets the function to resolve org names for table naming.
func (s *TelemetryStore) SetOrgNameResolver(resolver OrgNameResolver) {
	s.orgResolver = resolver
}

// resolveTable returns the per-org table name, using the cache.
func (s *TelemetryStore) resolveTable(orgID string) string {
	if v, ok := s.ensuredTables.Load(orgID); ok {
		return v.(string)
	}
	orgName := ""
	if s.orgResolver != nil {
		orgName = s.orgResolver(orgID)
	}
	return OrgTableName(orgID, orgName)
}

// Migrate creates the legacy shared table (for backward compat) and ensures
// the database exists. Per-org tables are created on first ingest.
func (s *TelemetryStore) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, legacySchema); err != nil {
		return err
	}
	return nil
}

// ensureOrgTable creates the per-org table if it doesn't exist yet.
func (s *TelemetryStore) ensureOrgTable(ctx context.Context, orgID string) string {
	if v, ok := s.ensuredTables.Load(orgID); ok {
		return v.(string)
	}
	table := s.resolveTable(orgID)
	ddl := orgTableSchema(table, 7)
	if _, err := s.db.ExecContext(ctx, ddl); err != nil {
		log.Warnf("clickhouse: failed to create org table %s: %v", table, err)
		return "telemetry_events" // fallback
	}
	s.ensuredTables.Store(orgID, table)
	log.Infof("clickhouse: created per-org table %s", table)
	return table
}

// SetRetention updates the TTL on a per-org table.
func (s *TelemetryStore) SetRetention(ctx context.Context, orgID string, days int) error {
	table := s.resolveTable(orgID)
	_, err := s.db.ExecContext(ctx, fmt.Sprintf(
		"ALTER TABLE %s MODIFY TTL toDateTime(timestamp) + INTERVAL %d DAY DELETE", table, days))
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

	table := s.ensureOrgTable(ctx, orgID)
	stmt, err := tx.PrepareContext(ctx,
		fmt.Sprintf(`INSERT INTO %s (id, org_id, agent_id, agent_hostname, seq, timestamp,
			event_name, event_category, pid, tid, process_name, process_exe,
			process_cmdline, parent_pid, parent_name, params, metadata, raw_event) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, table))
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
	table := s.resolveTable(orgID)
	query := fmt.Sprintf(`SELECT id, org_id, agent_id, agent_hostname, seq, timestamp,
				event_name, event_category, pid, tid, process_name, process_exe,
				process_cmdline, parent_pid, parent_name, params, metadata, raw_event
			  FROM %s WHERE org_id = ?`, table)
	countQuery := fmt.Sprintf(`SELECT count() FROM %s WHERE org_id = ?`, table)
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
	if opts.ParentPID > 0 {
		query += " AND parent_pid = ?"
		countQuery += " AND parent_pid = ?"
		args = append(args, opts.ParentPID)
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
		} else if err != nil {
			// Fallback: treat as plain text search
			like := "%" + opts.Query + "%"
			query += " AND (process_name ILIKE ? OR process_exe ILIKE ? OR process_cmdline ILIKE ? OR event_name ILIKE ?)"
			countQuery += " AND (process_name ILIKE ? OR process_exe ILIKE ? OR process_cmdline ILIKE ? OR event_name ILIKE ?)"
			args = append(args, like, like, like, like)
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

	table := s.resolveTable(orgID)
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT id, org_id, agent_id, agent_hostname, seq, timestamp,
			event_name, event_category, pid, tid, process_name, process_exe,
			process_cmdline, parent_pid, parent_name, params, metadata, raw_event
		 FROM %s WHERE org_id = ? AND agent_id = ?
		 ORDER BY timestamp DESC LIMIT ?`, table),
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
	table := s.resolveTable(orgID)
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT agent_id, count() FROM %s WHERE org_id = ? GROUP BY agent_id`, table),
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

func (s *TelemetryStore) GetFieldValues(ctx context.Context, orgID string) (map[string][]string, error) {
	result := make(map[string][]string)
	table := s.resolveTable(orgID)

	queries := map[string]string{
		"event_types":      fmt.Sprintf("SELECT DISTINCT event_name FROM %s WHERE org_id = ? AND event_name != '' ORDER BY event_name LIMIT 100", table),
		"event_categories": fmt.Sprintf("SELECT DISTINCT event_category FROM %s WHERE org_id = ? AND event_category != '' ORDER BY event_category LIMIT 50", table),
		"process_names":    fmt.Sprintf("SELECT process_name FROM %s WHERE org_id = ? AND process_name != '' GROUP BY process_name ORDER BY count() DESC LIMIT 50", table),
		"agents":           fmt.Sprintf("SELECT DISTINCT agent_hostname FROM %s WHERE org_id = ? AND agent_hostname != '' ORDER BY agent_hostname LIMIT 50", table),
	}

	for key, query := range queries {
		rows, err := s.db.QueryContext(ctx, query, orgID)
		if err != nil {
			continue
		}
		var values []string
		for rows.Next() {
			var val string
			rows.Scan(&val)
			if val != "" {
				values = append(values, val)
			}
		}
		rows.Close()
		result[key] = values
	}

	return result, nil
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
