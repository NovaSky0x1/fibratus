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
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agents (id, hostname, os_version, engine_version, group_id, tags, status, last_heartbeat, registered_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
		 ON CONFLICT (id) DO UPDATE SET
			hostname = EXCLUDED.hostname,
			os_version = EXCLUDED.os_version,
			engine_version = EXCLUDED.engine_version,
			group_id = EXCLUDED.group_id,
			tags = EXCLUDED.tags,
			status = EXCLUDED.status,
			last_heartbeat = EXCLUDED.last_heartbeat,
			updated_at = NOW()`,
		agent.ID, agent.Hostname, agent.OSVersion, agent.EngineVersion,
		agent.GroupID, tags, string(agent.Status), agent.LastHeartbeat, agent.RegisteredAt,
	)
	return err
}

func (s *AgentStore) Get(ctx context.Context, id string) (*fleet.Agent, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT a.id, a.hostname, a.os_version, a.engine_version, a.group_id,
				COALESCE(g.name, '') as group_name, a.tags, a.status,
				a.last_heartbeat, a.registered_at, a.updated_at
		 FROM agents a
		 LEFT JOIN agent_groups g ON a.group_id = g.id
		 WHERE a.id = $1`, id)
	return scanAgent(row)
}

func (s *AgentStore) GetByHostname(ctx context.Context, hostname string) (*fleet.Agent, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT a.id, a.hostname, a.os_version, a.engine_version, a.group_id,
				COALESCE(g.name, '') as group_name, a.tags, a.status,
				a.last_heartbeat, a.registered_at, a.updated_at
		 FROM agents a
		 LEFT JOIN agent_groups g ON a.group_id = g.id
		 WHERE a.hostname = $1`, hostname)
	return scanAgent(row)
}

func (s *AgentStore) List(ctx context.Context, opts fleet.AgentListOptions) ([]*fleet.Agent, int, error) {
	query := `SELECT a.id, a.hostname, a.os_version, a.engine_version, a.group_id,
				COALESCE(g.name, '') as group_name, a.tags, a.status,
				a.last_heartbeat, a.registered_at, a.updated_at
			  FROM agents a
			  LEFT JOIN agent_groups g ON a.group_id = g.id
			  WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM agents a WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

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
	_, err := s.db.ExecContext(ctx,
		`UPDATE agents SET hostname=$2, os_version=$3, engine_version=$4,
			group_id=$5, tags=$6, status=$7, updated_at=NOW()
		 WHERE id=$1`,
		agent.ID, agent.Hostname, agent.OSVersion, agent.EngineVersion,
		agent.GroupID, tags, string(agent.Status),
	)
	return err
}

func (s *AgentStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM agents WHERE id=$1`, id)
	return err
}

func (s *AgentStore) UpdateHeartbeat(ctx context.Context, id string, hb *fleet.Heartbeat) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE agents SET last_heartbeat=$2, status='online', updated_at=NOW() WHERE id=$1`,
		id, hb.Timestamp,
	)
	return err
}

func (s *AgentStore) CountByStatus(ctx context.Context) (map[fleet.AgentStatus]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM agents GROUP BY status`)
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

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanAgent(row *sql.Row) (*fleet.Agent, error) {
	a := &fleet.Agent{}
	var tagsJSON []byte
	var status string
	var lastHB sql.NullTime

	err := row.Scan(
		&a.ID, &a.Hostname, &a.OSVersion, &a.EngineVersion,
		&a.GroupID, &a.GroupName, &tagsJSON, &status,
		&lastHB, &a.RegisteredAt, &a.UpdatedAt,
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
	json.Unmarshal(tagsJSON, &a.Tags)
	return a, nil
}

func scanAgentRows(rows *sql.Rows) (*fleet.Agent, error) {
	a := &fleet.Agent{}
	var tagsJSON []byte
	var status string
	var lastHB sql.NullTime

	err := rows.Scan(
		&a.ID, &a.Hostname, &a.OSVersion, &a.EngineVersion,
		&a.GroupID, &a.GroupName, &tagsJSON, &status,
		&lastHB, &a.RegisteredAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	a.Status = fleet.AgentStatus(status)
	if lastHB.Valid {
		a.LastHeartbeat = lastHB.Time
	}
	json.Unmarshal(tagsJSON, &a.Tags)
	return a, nil
}
