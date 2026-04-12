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
	"time"

	"github.com/rabbitstack/fibratus/pkg/fleet"
)

// CommandStore implements store.CommandStore backed by PostgreSQL.
type CommandStore struct {
	db *sql.DB
}

// NewCommandStore creates a new command store.
func NewCommandStore(db *sql.DB) *CommandStore {
	return &CommandStore{db: db}
}

func (s *CommandStore) Create(ctx context.Context, cmd *fleet.Command) error {
	payload := cmd.Payload
	if payload == nil {
		payload = json.RawMessage(`{}`)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO commands (id, org_id, agent_id, type, payload, status, created_by, created_by_email, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		cmd.ID, cmd.OrgID, cmd.AgentID, cmd.Type, payload, cmd.Status, cmd.CreatedBy, cmd.CreatedByEmail, cmd.CreatedAt,
	)
	return err
}

func (s *CommandStore) GetPendingForAgent(ctx context.Context, agentID string) ([]*fleet.Command, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, agent_id, type, payload, status, result, error_message,
				created_by, created_by_email, created_at, started_at, completed_at
		 FROM commands WHERE agent_id = $1 AND status = 'pending'
		 ORDER BY created_at ASC`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	commands := make([]*fleet.Command, 0)
	for rows.Next() {
		cmd, err := scanCommandRows(rows)
		if err != nil {
			return nil, err
		}
		commands = append(commands, cmd)
	}
	return commands, rows.Err()
}

func (s *CommandStore) MarkRunning(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE commands SET status = 'running', started_at = NOW() WHERE id = $1`, id)
	return err
}

func (s *CommandStore) SetResult(ctx context.Context, id string, status string, result json.RawMessage, errMsg string) error {
	if result == nil {
		result = json.RawMessage(`{}`)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE commands SET status = $2, result = $3, error_message = $4, completed_at = NOW()
		 WHERE id = $1`, id, status, result, errMsg)
	return err
}

// HasRecentCommand checks if there's a pending/running command of the given type,
// or one completed within the cooldown period. Uses a single atomic SQL query
// to avoid race conditions between concurrent heartbeat goroutines.
func (s *CommandStore) HasRecentCommand(ctx context.Context, agentID, cmdType string, cooldown time.Duration) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM commands
		 WHERE agent_id = $1 AND type = $2
		   AND (status IN ('pending', 'running')
		        OR (status = 'completed' AND created_at > $3))`,
		agentID, cmdType, time.Now().Add(-cooldown),
	).Scan(&count)
	return count > 0, err
}

func (s *CommandStore) ListByAgent(ctx context.Context, orgID, agentID string, limit int) ([]*fleet.Command, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, agent_id, type, payload, status, result, error_message,
				created_by, created_by_email, created_at, started_at, completed_at
		 FROM commands WHERE org_id = $1 AND agent_id = $2
		 ORDER BY created_at DESC LIMIT $3`, orgID, agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	commands := make([]*fleet.Command, 0)
	for rows.Next() {
		cmd, err := scanCommandRows(rows)
		if err != nil {
			return nil, err
		}
		commands = append(commands, cmd)
	}
	return commands, rows.Err()
}

func (s *CommandStore) Get(ctx context.Context, orgID, id string) (*fleet.Command, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, agent_id, type, payload, status, result, error_message,
				created_by, created_by_email, created_at, started_at, completed_at
		 FROM commands WHERE org_id = $1 AND id = $2`, orgID, id)
	return scanCommandRow(row)
}

func scanCommandRow(row *sql.Row) (*fleet.Command, error) {
	cmd := &fleet.Command{}
	var payload, result []byte
	var startedAt, completedAt sql.NullTime

	err := row.Scan(
		&cmd.ID, &cmd.OrgID, &cmd.AgentID, &cmd.Type, &payload, &cmd.Status, &result,
		&cmd.ErrorMessage, &cmd.CreatedBy, &cmd.CreatedByEmail, &cmd.CreatedAt, &startedAt, &completedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	cmd.Payload = payload
	cmd.Result = result
	if startedAt.Valid {
		cmd.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		cmd.CompletedAt = &completedAt.Time
	}
	return cmd, nil
}

func scanCommandRows(rows *sql.Rows) (*fleet.Command, error) {
	cmd := &fleet.Command{}
	var payload, result []byte
	var startedAt, completedAt sql.NullTime

	err := rows.Scan(
		&cmd.ID, &cmd.OrgID, &cmd.AgentID, &cmd.Type, &payload, &cmd.Status, &result,
		&cmd.ErrorMessage, &cmd.CreatedBy, &cmd.CreatedByEmail, &cmd.CreatedAt, &startedAt, &completedAt,
	)
	if err != nil {
		return nil, err
	}

	cmd.Payload = payload
	cmd.Result = result
	if startedAt.Valid {
		cmd.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		cmd.CompletedAt = &completedAt.Time
	}
	return cmd, nil
}
