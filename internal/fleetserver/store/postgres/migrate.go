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

	_ "github.com/lib/pq"
)

const schema = `
-- ═══════════════════════════════════════════════════════════════
-- Multi-tenancy: Account > Organization hierarchy
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS accounts (
    id          TEXT PRIMARY KEY,
    name        TEXT UNIQUE NOT NULL,
    plan        TEXT DEFAULT 'free',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS organizations (
    id          TEXT PRIMARY KEY,
    account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(account_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_organizations_account ON organizations(account_id);

-- ═══════════════════════════════════════════════════════════════
-- Users and access control
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS users (
    id          TEXT PRIMARY KEY,
    email       TEXT UNIQUE NOT NULL,
    name        TEXT NOT NULL,
    password    TEXT NOT NULL,
    account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role        TEXT DEFAULT 'admin',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_orgs (
    user_id     TEXT REFERENCES users(id) ON DELETE CASCADE,
    org_id      TEXT REFERENCES organizations(id) ON DELETE CASCADE,
    role        TEXT DEFAULT 'viewer',
    PRIMARY KEY (user_id, org_id)
);

-- ═══════════════════════════════════════════════════════════════
-- Agent enrollment and certificate authority
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS enrollment_tokens (
    id          TEXT PRIMARY KEY,
    account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    org_id      TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name        TEXT DEFAULT '',
    max_uses    INT NOT NULL DEFAULT 50,
    uses_count  INT NOT NULL DEFAULT 0,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_by  TEXT NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_org ON enrollment_tokens(org_id);

CREATE TABLE IF NOT EXISTS org_cas (
    org_id      TEXT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    ca_cert     BYTEA NOT NULL,
    ca_key      BYTEA NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- ═══════════════════════════════════════════════════════════════
-- Fleet: agent groups, agents, rules, detections (all org-scoped)
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS agent_groups (
    id          TEXT PRIMARY KEY,
    org_id      TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT DEFAULT '',
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(org_id, name)
);

CREATE INDEX IF NOT EXISTS idx_agent_groups_org ON agent_groups(org_id);

CREATE TABLE IF NOT EXISTS agents (
    id              TEXT PRIMARY KEY,
    org_id          TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    hostname        TEXT NOT NULL,
    os_version      TEXT DEFAULT '',
    engine_version  TEXT DEFAULT '',
    group_id        TEXT REFERENCES agent_groups(id),
    tags            JSONB DEFAULT '{}',
    status          TEXT DEFAULT 'online',
    last_heartbeat  TIMESTAMPTZ,
    registered_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agents_org ON agents(org_id);
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(org_id, status);
CREATE INDEX IF NOT EXISTS idx_agents_group ON agents(group_id);
CREATE INDEX IF NOT EXISTS idx_agents_hostname ON agents(org_id, hostname);

CREATE TABLE IF NOT EXISTS rules (
    id              TEXT PRIMARY KEY,
    org_id          TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    version         TEXT NOT NULL DEFAULT '1.0.0',
    description     TEXT DEFAULT '',
    condition       TEXT NOT NULL,
    output_template TEXT DEFAULT '',
    severity        TEXT NOT NULL DEFAULT 'medium',
    labels          JSONB DEFAULT '{}',
    tags            TEXT[] DEFAULT '{}',
    "references"    TEXT[] DEFAULT '{}',
    raw_yaml        TEXT NOT NULL,
    enabled         BOOLEAN DEFAULT true,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rules_org ON rules(org_id);

CREATE TABLE IF NOT EXISTS rule_assignments (
    group_id    TEXT REFERENCES agent_groups(id) ON DELETE CASCADE,
    rule_id     TEXT REFERENCES rules(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, rule_id)
);

CREATE TABLE IF NOT EXISTS detections (
    id              TEXT PRIMARY KEY,
    org_id          TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
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

CREATE INDEX IF NOT EXISTS idx_detections_org ON detections(org_id);
CREATE INDEX IF NOT EXISTS idx_detections_agent ON detections(org_id, agent_id);
CREATE INDEX IF NOT EXISTS idx_detections_severity ON detections(org_id, severity);
CREATE INDEX IF NOT EXISTS idx_detections_timestamp ON detections(org_id, timestamp DESC);
`

// Migrate runs the database schema migrations.
// Migrate runs the database schema migrations.
func Migrate(dsn string) error {
	db, err := sql.Open("postgres", dsn)
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
