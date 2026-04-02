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
	"fmt"

	"github.com/rabbitstack/fibratus/internal/fleetserver"
	_ "github.com/lib/pq"
)

const schema = `
CREATE TABLE IF NOT EXISTS agent_groups (
    id          TEXT PRIMARY KEY,
    name        TEXT UNIQUE NOT NULL,
    description TEXT DEFAULT '',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

INSERT INTO agent_groups (id, name, description)
VALUES ('default', 'default', 'Default agent group')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS agents (
    id              TEXT PRIMARY KEY,
    hostname        TEXT NOT NULL,
    os_version      TEXT DEFAULT '',
    engine_version  TEXT DEFAULT '',
    group_id        TEXT REFERENCES agent_groups(id) DEFAULT 'default',
    tags            JSONB DEFAULT '{}',
    status          TEXT DEFAULT 'online',
    last_heartbeat  TIMESTAMPTZ,
    registered_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
CREATE INDEX IF NOT EXISTS idx_agents_group ON agents(group_id);
CREATE INDEX IF NOT EXISTS idx_agents_hostname ON agents(hostname);

CREATE TABLE IF NOT EXISTS rules (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    version         TEXT NOT NULL DEFAULT '1.0.0',
    description     TEXT DEFAULT '',
    condition       TEXT NOT NULL,
    output_template TEXT DEFAULT '',
    severity        TEXT NOT NULL DEFAULT 'medium',
    labels          JSONB DEFAULT '{}',
    tags            TEXT[] DEFAULT '{}',
    references      TEXT[] DEFAULT '{}',
    raw_yaml        TEXT NOT NULL,
    enabled         BOOLEAN DEFAULT true,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS rule_assignments (
    group_id    TEXT REFERENCES agent_groups(id) ON DELETE CASCADE,
    rule_id     TEXT REFERENCES rules(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, rule_id)
);

CREATE TABLE IF NOT EXISTS detections (
    id              TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL,
    agent_hostname  TEXT NOT NULL,
    rule_id         TEXT DEFAULT '',
    rule_name       TEXT DEFAULT '',
    title           TEXT NOT NULL,
    text            TEXT DEFAULT '',
    description     TEXT DEFAULT '',
    severity        TEXT NOT NULL DEFAULT 'medium',
    labels          JSONB DEFAULT '{}',
    tags            TEXT[] DEFAULT '{}',
    events          JSONB DEFAULT '[]',
    timestamp       TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_detections_agent ON detections(agent_id);
CREATE INDEX IF NOT EXISTS idx_detections_severity ON detections(severity);
CREATE INDEX IF NOT EXISTS idx_detections_timestamp ON detections(timestamp DESC);

CREATE TABLE IF NOT EXISTS api_keys (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    key_hash    TEXT NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    last_used   TIMESTAMPTZ
);
`

// Migrate runs the database schema migrations.
func Migrate(cfg fleetserver.DatabaseConfig) error {
	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		return fmt.Errorf("migrate: failed to connect: %w", err)
	}
	defer db.Close()

	_, err = db.ExecContext(context.Background(), schema)
	if err != nil {
		return fmt.Errorf("migrate: failed to execute schema: %w", err)
	}

	return nil
}
