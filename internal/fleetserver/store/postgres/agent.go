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

	"github.com/rabbitstack/fibratus/pkg/fleet"
)

// AgentStore implements store.AgentStore backed by PostgreSQL.
type AgentStore struct {
	db *sql.DB
}

// NewAgentStore creates a new agent store.
func NewAgentStore(db *sql.DB) *AgentStore {
	return &AgentStore{db: db}
}

func (s *AgentStore) Create(ctx context.Context, agent *fleet.Agent) error {
	tags, _ := json.Marshal(agent.Tags)

	// Pass NULL for empty group_id to satisfy FK constraint
	var groupID interface{}
	if agent.GroupID != "" {
		groupID = agent.GroupID
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agents (id, org_id, hostname, os_version, engine_version, group_id, tags, status, last_heartbeat, registered_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
		 ON CONFLICT (id) DO UPDATE SET
			hostname = EXCLUDED.hostname,
			os_version = EXCLUDED.os_version,
			engine_version = EXCLUDED.engine_version,
			tags = EXCLUDED.tags,
			status = EXCLUDED.status,
			last_heartbeat = EXCLUDED.last_heartbeat,
			updated_at = NOW()`,
		agent.ID, agent.OrgID, agent.Hostname, agent.OSVersion, agent.EngineVersion,
		groupID, tags, string(agent.Status), agent.LastHeartbeat, agent.RegisteredAt,
	)
	return err
}

func (s *AgentStore) Get(ctx context.Context, orgID, id string) (*fleet.Agent, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT a.id, a.org_id, a.hostname, a.os_version, a.engine_version, a.group_id,
				COALESCE(g.name, '') as group_name, a.tags, a.status,
				a.last_heartbeat, a.registered_at, a.updated_at,
				COALESCE(a.tamper_protection, false), COALESCE(a.isolated, false), COALESCE(a.eventlog_collection, false)
		 FROM agents a
		 LEFT JOIN agent_groups g ON a.group_id = g.id
		 WHERE a.id = $1 AND a.org_id = $2`, id, orgID)
	return scanAgent(row)
}

func (s *AgentStore) GetByHostname(ctx context.Context, orgID, hostname string) (*fleet.Agent, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT a.id, a.org_id, a.hostname, a.os_version, a.engine_version, a.group_id,
				COALESCE(g.name, '') as group_name, a.tags, a.status,
				a.last_heartbeat, a.registered_at, a.updated_at,
				COALESCE(a.tamper_protection, false), COALESCE(a.isolated, false), COALESCE(a.eventlog_collection, false)
		 FROM agents a
		 LEFT JOIN agent_groups g ON a.group_id = g.id
		 WHERE a.hostname = $1 AND a.org_id = $2`, hostname, orgID)
	return scanAgent(row)
}

func (s *AgentStore) List(ctx context.Context, orgID string, opts fleet.AgentListOptions) ([]*fleet.Agent, int, error) {
	query := `SELECT a.id, a.org_id, a.hostname, a.os_version, a.engine_version, a.group_id,
				COALESCE(g.name, '') as group_name, a.tags, a.status,
				a.last_heartbeat, a.registered_at, a.updated_at,
				COALESCE(a.tamper_protection, false), COALESCE(a.isolated, false), COALESCE(a.eventlog_collection, false)
			  FROM agents a
			  LEFT JOIN agent_groups g ON a.group_id = g.id
			  WHERE a.org_id = $1`
	countQuery := `SELECT COUNT(*) FROM agents a WHERE a.org_id = $1`
	args := []interface{}{orgID}
	argIdx := 2

	if opts.Status != "" {
		clause := fmt.Sprintf(" AND a.status = $%d", argIdx)
		query += clause
		countQuery += clause
		args = append(args, string(opts.Status))
		argIdx++
	}
	if opts.GroupID != "" {
		clause := fmt.Sprintf(" AND a.group_id = $%d", argIdx)
		query += clause
		countQuery += clause
		args = append(args, opts.GroupID)
		argIdx++
	}
	if opts.Search != "" {
		clause := fmt.Sprintf(" AND a.hostname ILIKE $%d", argIdx)
		query += clause
		countQuery += clause
		args = append(args, "%"+opts.Search+"%")
		argIdx++
	}

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query += " ORDER BY a.last_heartbeat DESC NULLS LAST"

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

	agents := make([]*fleet.Agent, 0)
	for rows.Next() {
		agent, err := scanAgentRows(rows)
		if err != nil {
			return nil, 0, err
		}
		agents = append(agents, agent)
	}

	return agents, total, rows.Err()
}

func (s *AgentStore) Update(ctx context.Context, agent *fleet.Agent) error {
	tags, _ := json.Marshal(agent.Tags)
	// Pass NULL for empty group_id to satisfy FK constraint (same as Create)
	var groupID interface{}
	if agent.GroupID != "" {
		groupID = agent.GroupID
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE agents SET hostname=$3, os_version=$4, engine_version=$5,
			group_id=$6, tags=$7, status=$8, tamper_protection=$9, isolated=$10, eventlog_collection=$11, updated_at=NOW()
		 WHERE id=$1 AND org_id=$2`,
		agent.ID, agent.OrgID, agent.Hostname, agent.OSVersion, agent.EngineVersion,
		groupID, tags, string(agent.Status), agent.TamperProtection, agent.Isolated, agent.EventLogCollection,
	)
	return err
}

func (s *AgentStore) Delete(ctx context.Context, orgID, id string) error {
	// Get hostname before deleting for the decommissioned record
	var hostname string
	_ = s.db.QueryRowContext(ctx, `SELECT hostname FROM agents WHERE id=$1 AND org_id=$2`, id, orgID).Scan(&hostname)

	// Add to decommissioned list so reconnecting agents get auto-uninstalled
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO decommissioned_agents (id, org_id, hostname) VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING`,
		id, orgID, hostname,
	)

	_, err := s.db.ExecContext(ctx, `DELETE FROM agents WHERE id=$1 AND org_id=$2`, id, orgID)
	return err
}

// IsDecommissioned checks if an agent ID has been decommissioned.
func (s *AgentStore) IsDecommissioned(ctx context.Context, agentID string) bool {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM decommissioned_agents WHERE id=$1)`, agentID).Scan(&exists)
	return err == nil && exists
}

func (s *AgentStore) UpdateHeartbeat(ctx context.Context, orgID, id string, hb *fleet.Heartbeat) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE agents SET last_heartbeat=$3, status='online', updated_at=NOW() WHERE id=$1 AND org_id=$2`,
		id, orgID, hb.Timestamp,
	)
	if err != nil {
		return err
	}
	// Record heartbeat history for sparkline charts
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO heartbeat_history (org_id, agent_id, cpu_pct, mem_mb, events_per_sec, active_rules, timestamp) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		orgID, id, hb.CPUPercent, hb.MemoryMB, hb.EventsPerSec, hb.ActiveRules, hb.Timestamp,
	)
	return nil
}

// GetHeartbeatHistory returns the last N heartbeat entries for sparkline charts.
func (s *AgentStore) GetHeartbeatHistory(ctx context.Context, orgID, agentID string, limit int) ([]fleet.Heartbeat, error) {
	if limit <= 0 {
		limit = 60
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT cpu_pct, mem_mb, events_per_sec, active_rules, timestamp FROM heartbeat_history WHERE org_id=$1 AND agent_id=$2 ORDER BY timestamp DESC LIMIT $3`,
		orgID, agentID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []fleet.Heartbeat
	for rows.Next() {
		var hb fleet.Heartbeat
		if err := rows.Scan(&hb.CPUPercent, &hb.MemoryMB, &hb.EventsPerSec, &hb.ActiveRules, &hb.Timestamp); err != nil {
			continue
		}
		history = append(history, hb)
	}
	for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
		history[i], history[j] = history[j], history[i]
	}
	return history, nil
}

func (s *AgentStore) CountByStatus(ctx context.Context, orgID string) (map[fleet.AgentStatus]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM agents WHERE org_id=$1 GROUP BY status`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[fleet.AgentStatus]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[fleet.AgentStatus(status)] = count
	}
	return counts, rows.Err()
}

func (s *AgentStore) MarkOfflineAgents(ctx context.Context, timeout time.Duration) (int, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE agents SET status='offline', updated_at=NOW()
		 WHERE status='online' AND last_heartbeat < $1`,
		time.Now().UTC().Add(-timeout),
	)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func scanAgent(row *sql.Row) (*fleet.Agent, error) {
	a := &fleet.Agent{}
	var tagsJSON []byte
	var status string
	var lastHB sql.NullTime
	var groupID sql.NullString

	err := row.Scan(
		&a.ID, &a.OrgID, &a.Hostname, &a.OSVersion, &a.EngineVersion,
		&groupID, &a.GroupName, &tagsJSON, &status,
		&lastHB, &a.RegisteredAt, &a.UpdatedAt,
		&a.TamperProtection, &a.Isolated, &a.EventLogCollection,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	a.Status = fleet.AgentStatus(status)
	if lastHB.Valid {
		a.LastHeartbeat = lastHB.Time
	}
	if groupID.Valid {
		a.GroupID = groupID.String
	}
	json.Unmarshal(tagsJSON, &a.Tags)
	return a, nil
}

func scanAgentRows(rows *sql.Rows) (*fleet.Agent, error) {
	a := &fleet.Agent{}
	var tagsJSON []byte
	var status string
	var lastHB sql.NullTime
	var groupID sql.NullString

	err := rows.Scan(
		&a.ID, &a.OrgID, &a.Hostname, &a.OSVersion, &a.EngineVersion,
		&groupID, &a.GroupName, &tagsJSON, &status,
		&lastHB, &a.RegisteredAt, &a.UpdatedAt,
		&a.TamperProtection, &a.Isolated, &a.EventLogCollection,
	)
	if err != nil {
		return nil, err
	}

	a.Status = fleet.AgentStatus(status)
	if lastHB.Valid {
		a.LastHeartbeat = lastHB.Time
	}
	if groupID.Valid {
		a.GroupID = groupID.String
	}
	json.Unmarshal(tagsJSON, &a.Tags)
	return a, nil
}
