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

-- Global rules apply to ALL organizations by default.
-- Admins manage these from the system-level dashboard.
-- Orgs can override (disable) specific global rules via global_rule_overrides.
CREATE TABLE IF NOT EXISTS global_rules (
    id              TEXT PRIMARY KEY,
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

-- Per-org overrides for global rules (e.g., disable a noisy global rule for one org)
CREATE TABLE IF NOT EXISTS global_rule_overrides (
    org_id      TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    rule_id     TEXT NOT NULL REFERENCES global_rules(id) ON DELETE CASCADE,
    enabled     BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (org_id, rule_id)
);

CREATE INDEX IF NOT EXISTS idx_detections_org ON detections(org_id);
CREATE INDEX IF NOT EXISTS idx_detections_agent ON detections(org_id, agent_id);
CREATE INDEX IF NOT EXISTS idx_detections_severity ON detections(org_id, severity);
CREATE INDEX IF NOT EXISTS idx_detections_timestamp ON detections(org_id, timestamp DESC);

-- ═══════════════════════════════════════════════════════════════
-- Command queue: active response commands for agents
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS commands (
    id              TEXT PRIMARY KEY,
    org_id          TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id        TEXT NOT NULL,
    type            TEXT NOT NULL,
    payload         JSONB DEFAULT '{}',
    status          TEXT NOT NULL DEFAULT 'pending',
    result          JSONB DEFAULT '{}',
    error_message   TEXT DEFAULT '',
    created_by      TEXT DEFAULT '',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_commands_agent_status ON commands(agent_id, status);
CREATE INDEX IF NOT EXISTS idx_commands_org ON commands(org_id);

-- ═══════════════════════════════════════════════════════════════
-- Telemetry: kernel events forwarded by agents
-- Uses timestamp-based queries. Old data purged by retention job.
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS telemetry_events (
    id              BIGSERIAL PRIMARY KEY,
    org_id          TEXT NOT NULL,
    agent_id        TEXT NOT NULL,
    agent_hostname  TEXT DEFAULT '',
    seq             BIGINT DEFAULT 0,
    timestamp       TIMESTAMPTZ NOT NULL,
    event_name      TEXT NOT NULL,
    event_category  TEXT DEFAULT '',
    pid             BIGINT DEFAULT 0,
    tid             BIGINT DEFAULT 0,
    process_name    TEXT DEFAULT '',
    process_exe     TEXT DEFAULT '',
    process_cmdline TEXT DEFAULT '',
    parent_pid      BIGINT DEFAULT 0,
    parent_name     TEXT DEFAULT '',
    params          JSONB DEFAULT '{}',
    metadata        JSONB DEFAULT '{}',
    raw_event       JSONB DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_telemetry_org_agent ON telemetry_events(org_id, agent_id);
CREATE INDEX IF NOT EXISTS idx_telemetry_timestamp ON telemetry_events(org_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_telemetry_event_name ON telemetry_events(org_id, event_name);
CREATE INDEX IF NOT EXISTS idx_telemetry_process ON telemetry_events(org_id, process_name);
CREATE INDEX IF NOT EXISTS idx_telemetry_pid ON telemetry_events(org_id, pid);

-- ═══════════════════════════════════════════════════════════════
-- Macros: reusable filter expressions for detection rules
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS macros (
    id          TEXT PRIMARY KEY,
    org_id      TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    expr        TEXT NOT NULL,
    description TEXT DEFAULT '',
    raw_yaml    TEXT NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(org_id, name)
);

CREATE INDEX IF NOT EXISTS idx_macros_org ON macros(org_id);

-- ═══════════════════════════════════════════════════════════════
-- Audit log: tracks all admin actions in the portal
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS audit_log (
    id              TEXT PRIMARY KEY,
    org_id          TEXT NOT NULL,
    user_id         TEXT DEFAULT '',
    user_email      TEXT DEFAULT '',
    action          TEXT NOT NULL,
    resource_type   TEXT NOT NULL,
    resource_id     TEXT DEFAULT '',
    resource_name   TEXT DEFAULT '',
    details         JSONB DEFAULT '{}',
    ip_address      TEXT DEFAULT '',
    timestamp       TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_log_org ON audit_log(org_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_timestamp ON audit_log(org_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_action ON audit_log(org_id, action);

-- Add created_by_email to commands for display
ALTER TABLE commands ADD COLUMN IF NOT EXISTS created_by_email TEXT DEFAULT '';

-- Add list_values to macros for list-type macros (web_browser_binaries, etc.)
ALTER TABLE macros ADD COLUMN IF NOT EXISTS list_values TEXT[] DEFAULT '{}';

-- Authentication security: lockout, TOTP 2FA, recovery codes
ALTER TABLE users ADD COLUMN IF NOT EXISTS login_attempts INT DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret TEXT DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_enabled BOOLEAN DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS recovery_codes TEXT DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS org_restrictions TEXT DEFAULT '';

CREATE TABLE IF NOT EXISTS user_groups (
    id              TEXT PRIMARY KEY,
    account_id      TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT DEFAULT '',
    permissions     JSONB DEFAULT '[]',
    org_restrictions JSONB DEFAULT 'null',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_group_members (
    user_id     TEXT REFERENCES users(id) ON DELETE CASCADE,
    group_id    TEXT REFERENCES user_groups(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, group_id)
);

ALTER TABLE user_groups ADD COLUMN IF NOT EXISTS owner_id TEXT REFERENCES users(id);

ALTER TABLE accounts ADD COLUMN IF NOT EXISTS require_2fa BOOLEAN DEFAULT FALSE;
ALTER TABLE global_rules ADD COLUMN IF NOT EXISTS account_id TEXT DEFAULT '';
ALTER TABLE github_sync_configs DROP CONSTRAINT IF EXISTS github_sync_configs_pkey;
ALTER TABLE github_sync_configs DROP CONSTRAINT IF EXISTS github_sync_configs_org_id_fkey;

CREATE TABLE IF NOT EXISTS github_sync_configs (
    account_id  TEXT NOT NULL DEFAULT '',
    org_id      TEXT NOT NULL DEFAULT '',
    repo_url    TEXT DEFAULT '',
    branch      TEXT DEFAULT 'main',
    path        TEXT DEFAULT '',
    token       TEXT DEFAULT '',
    interval_min INT DEFAULT 30,
    enabled     BOOLEAN DEFAULT FALSE,
    scope       TEXT DEFAULT 'account',
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

ALTER TABLE github_sync_configs ADD COLUMN IF NOT EXISTS account_id TEXT NOT NULL DEFAULT '';
ALTER TABLE github_sync_configs ADD COLUMN IF NOT EXISTS scope TEXT DEFAULT 'account';
ALTER TABLE github_sync_configs ADD COLUMN IF NOT EXISTS id TEXT NOT NULL DEFAULT '';
ALTER TABLE github_sync_configs ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';

ALTER TABLE rules ADD COLUMN IF NOT EXISTS validation_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE rules ADD COLUMN IF NOT EXISTS validation_errors JSONB DEFAULT '[]';
ALTER TABLE rules ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'manual';

-- ═══════════════════════════════════════════════════════════════
-- Heartbeat history for agent health sparklines
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS heartbeat_history (
    id              BIGSERIAL PRIMARY KEY,
    org_id          TEXT NOT NULL,
    agent_id        TEXT NOT NULL,
    cpu_pct         DOUBLE PRECISION DEFAULT 0,
    mem_mb          BIGINT DEFAULT 0,
    events_per_sec  DOUBLE PRECISION DEFAULT 0,
    active_rules    INT DEFAULT 0,
    timestamp       TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_hb_history_agent ON heartbeat_history(agent_id, timestamp DESC);

-- ═══════════════════════════════════════════════════════════════
-- Tamper protection and isolation state
-- ═══════════════════════════════════════════════════════════════
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS tamper_protection_enabled BOOLEAN DEFAULT FALSE;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS isolation_whitelist JSONB DEFAULT '[]';
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS tamper_protection_enabled BOOLEAN DEFAULT FALSE;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS tamper_protection BOOLEAN DEFAULT FALSE;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS isolated BOOLEAN DEFAULT FALSE;

-- Event log collection
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS eventlog_enabled BOOLEAN DEFAULT FALSE;

-- Telemetry retention (days)
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS telemetry_retention_days INTEGER DEFAULT 7;
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS telemetry_retention_days INTEGER DEFAULT 0;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS eventlog_collection BOOLEAN DEFAULT FALSE;

-- ═══════════════════════════════════════════════════════════════
-- Decommissioned agents — deleted agents that should auto-uninstall on reconnect
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS decommissioned_agents (
    id                  TEXT PRIMARY KEY,
    org_id              TEXT NOT NULL,
    hostname            TEXT NOT NULL DEFAULT '',
    decommissioned_at   TIMESTAMPTZ DEFAULT NOW()
);

-- ═══════════════════════════════════════════════════════════════
-- Kernel captures: live event capture sessions from agents
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS captures (
    id              TEXT PRIMARY KEY,
    org_id          TEXT NOT NULL,
    agent_id        TEXT NOT NULL,
    agent_hostname  TEXT DEFAULT '',
    filter          TEXT DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'active',
    event_count     BIGINT DEFAULT 0,
    duration_sec    INT DEFAULT 0,
    created_by      TEXT DEFAULT '',
    started_at      TIMESTAMPTZ DEFAULT NOW(),
    completed_at    TIMESTAMPTZ
);

ALTER TABLE captures ADD COLUMN IF NOT EXISTS kcap_path TEXT DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_captures_org_agent ON captures(org_id, agent_id);
CREATE INDEX IF NOT EXISTS idx_captures_status ON captures(org_id, status);

CREATE TABLE IF NOT EXISTS capture_events (
    id              BIGSERIAL PRIMARY KEY,
    capture_id      TEXT NOT NULL REFERENCES captures(id) ON DELETE CASCADE,
    org_id          TEXT NOT NULL,
    seq             BIGINT DEFAULT 0,
    timestamp       TIMESTAMPTZ NOT NULL,
    event_name      TEXT NOT NULL,
    event_category  TEXT DEFAULT '',
    pid             BIGINT DEFAULT 0,
    process_name    TEXT DEFAULT '',
    process_exe     TEXT DEFAULT '',
    process_cmdline TEXT DEFAULT '',
    parent_pid      BIGINT DEFAULT 0,
    parent_name     TEXT DEFAULT '',
    params          JSONB DEFAULT '{}',
    raw_event       JSONB DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_capture_events_capture ON capture_events(capture_id);
CREATE INDEX IF NOT EXISTS idx_capture_events_ts ON capture_events(capture_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_capture_events_name ON capture_events(capture_id, event_name);

-- ═══════════════════════════════════════════════════════════════
-- Event Log Policies: server-managed Windows Event Log collection config
-- ═══════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS eventlog_policies (
    id         TEXT PRIMARY KEY,
    org_id     TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    enabled    BOOLEAN DEFAULT true,
    channels   JSONB NOT NULL DEFAULT '[]',
    version    INTEGER DEFAULT 1,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(org_id)
);
CREATE INDEX IF NOT EXISTS idx_eventlog_policies_org ON eventlog_policies(org_id);
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

	// Group-only RBAC: set all non-root users to member role.
	// Permissions now come exclusively from group memberships.
	_, err = db.ExecContext(context.Background(),
		`UPDATE users SET role = 'member' WHERE role != 'root' AND role != 'member'`)
	if err != nil {
		return fmt.Errorf("migrate: failed to normalize user roles: %w", err)
	}

	return nil
}
