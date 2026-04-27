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

package fleetserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	chstore "github.com/rabbitstack/fibratus/internal/fleetserver/store/clickhouse"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store/postgres"
	log "github.com/sirupsen/logrus"
)

// Profile name constants.
const (
	ProfileLocal = "local"
	ProfileCloud = "cloud"
)

// SecretActiveProfile names the secret-store row that points at the live
// profile. Value is the profile name (e.g. "local" or "cloud").
const SecretActiveProfile = "clickhouse.active_profile"

// SecretProfilePassword returns the secret-store key used to encrypt a given
// profile's ClickHouse user password.
func SecretProfilePassword(name string) string {
	return "clickhouse." + name + ".password"
}

// chPipeline owns the live ClickHouse connection and buffered store wrapper.
// It is replaced wholesale on a profile swap so we never mutate a connection
// that's actively serving requests.
type chPipeline struct {
	profile  *postgres.ClickHouseProfile
	db       *sql.DB
	buffered *store.BufferedTelemetryStore
}

// pipelineManager coordinates pipeline construction, profile activation, and
// hot-swap. The mutex serialises swaps so two concurrent activate calls don't
// race; reads of activeName are cheap.
type pipelineManager struct {
	mu           sync.Mutex
	holder       *store.TelemetryHolder
	active       *chPipeline   // nil when telemetry storage is disabled / using PG fallback
	activeName   string        // "local" / "cloud" / "" (PG fallback)
	orgResolver  func(orgID string) string
	makeBuffered func(s store.TelemetryStore) *store.BufferedTelemetryStore
}

// newPipelineManager constructs the manager. orgResolver is wired into each
// chstore so per-org table names resolve to human-readable strings; pass a
// closure over orgStore.Get.
func newPipelineManager(holder *store.TelemetryHolder, orgResolver func(string) string, bufCfg store.BufferConfig) *pipelineManager {
	return &pipelineManager{
		holder:      holder,
		orgResolver: orgResolver,
		makeBuffered: func(s store.TelemetryStore) *store.BufferedTelemetryStore {
			return store.NewBufferedTelemetryStore(s, bufCfg)
		},
	}
}

// openProfile builds (and migrates) a complete pipeline for one profile —
// open chDB, ping, init chstore, run schema migration, wrap in buffered
// store, Start the flush goroutine. Caller is responsible for installing it
// via the holder.
func (m *pipelineManager) openProfile(ctx context.Context, p *postgres.ClickHouseProfile, password string) (*chPipeline, error) {
	cfg := profileToClickHouseConfig(p, password)
	db, err := sql.Open("clickhouse", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("clickhouse open (%s): %w", p.Name, err)
	}
	db.SetMaxOpenConns(p.MaxOpenConns)
	db.SetMaxIdleConns(p.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(p.ConnMaxLifetimeSecs) * time.Second)

	pingCtx, cancel := context.WithTimeout(ctx, time.Duration(p.DialTimeoutSecs+5)*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("clickhouse ping (%s): %w", p.Name, err)
	}

	chTelemetry := chstore.NewTelemetryStore(db)
	if err := chTelemetry.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("clickhouse migrate (%s): %w", p.Name, err)
	}
	if m.orgResolver != nil {
		chTelemetry.SetOrgNameResolver(m.orgResolver)
	}
	buf := m.makeBuffered(chTelemetry)
	buf.Start()
	return &chPipeline{profile: p, db: db, buffered: buf}, nil
}

// activate installs a freshly-built pipeline as the live one and tears down
// the previous one (drains its buffer, closes its connection).
func (m *pipelineManager) activate(next *chPipeline) {
	m.mu.Lock()
	defer m.mu.Unlock()
	prev := m.active
	prevName := m.activeName
	m.active = next
	m.activeName = ""
	if next != nil && next.profile != nil {
		m.activeName = next.profile.Name
		m.holder.Replace(next.buffered)
		log.Infof("fleet: telemetry pipeline active = %s (host=%s:%d secure=%v)", next.profile.Name, next.profile.Host, next.profile.Port, next.profile.Secure)
	}
	if prev != nil {
		// Drain + close the old pipeline last so in-flight ingest finishes
		// against its buffer rather than silently disappearing.
		go func(p *chPipeline, name string) {
			p.buffered.Stop()
			_ = p.db.Close()
			log.Infof("fleet: drained + closed previous telemetry pipeline (%s)", name)
		}(prev, prevName)
	}
}

// shutdown stops the active pipeline (used on server shutdown).
func (m *pipelineManager) shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		return
	}
	m.active.buffered.Stop()
	_ = m.active.db.Close()
	m.active = nil
	m.activeName = ""
}

// activeProfileName returns the name of the live profile, or "" when none.
func (m *pipelineManager) activeProfileName() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeName
}

// activeDB returns the live ClickHouse *sql.DB, or nil when no pipeline is
// active (e.g. PostgreSQL fallback). Callers should not retain the returned
// handle across hot-swaps — fetch it again per request.
func (m *pipelineManager) activeDB() *sql.DB {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		return nil
	}
	return m.active.db
}

// profileToClickHouseConfig is the bridge from the persisted profile row to
// the in-memory ClickHouseConfig the existing DSN() builder consumes.
func profileToClickHouseConfig(p *postgres.ClickHouseProfile, password string) ClickHouseConfig {
	return ClickHouseConfig{
		Enabled:         p.Enabled,
		Host:            p.Host,
		Port:            p.Port,
		Database:        p.Database,
		User:            p.User,
		Password:        password,
		Secure:          p.Secure,
		SkipVerify:      p.SkipVerify,
		DialTimeoutSecs: p.DialTimeoutSecs,
		MaxOpenConns:    p.MaxOpenConns,
		MaxIdleConns:    p.MaxIdleConns,
		ConnMaxLifetime: p.ConnMaxLifetimeSecs,
	}
}

// resolvePassword fetches the encrypted password for a profile, returning ""
// (no error) if no row exists yet — a profile may legitimately be configured
// without auth (e.g. local clickhouse with no password).
func (s *Server) resolvePassword(ctx context.Context, profileName string) (string, error) {
	pw, err := s.secretStore.GetOr(ctx, SecretProfilePassword(profileName), "")
	if err != nil {
		return "", err
	}
	return pw, nil
}

// SetProfilePassword writes (or removes) a profile's password in the secret
// store. Empty value deletes the row so callers can clear credentials.
func (s *Server) SetProfilePassword(ctx context.Context, profileName, password, updatedBy string) error {
	if password == "" {
		return s.secretStore.Delete(ctx, SecretProfilePassword(profileName))
	}
	return s.secretStore.Set(ctx, SecretProfilePassword(profileName), password, updatedBy)
}

// activateProfile is the dashboard-facing entry point: load the named profile,
// fetch its password, build a fresh pipeline, swap it in. Returns ErrProfileNotFound
// when no row matches.
func (s *Server) activateProfile(ctx context.Context, name string) error {
	if s.profileStore == nil || s.pipeline == nil {
		return errors.New("clickhouse pipeline not initialised")
	}
	p, err := s.profileStore.Get(ctx, name)
	if err != nil {
		return err
	}
	pw, err := s.resolvePassword(ctx, name)
	if err != nil {
		return err
	}
	next, err := s.pipeline.openProfile(ctx, p, pw)
	if err != nil {
		return err
	}
	s.pipeline.activate(next)
	if err := s.secretStore.Set(ctx, SecretActiveProfile, name, "dashboard"); err != nil {
		log.Warnf("fleet: profile activated but failed to persist active pointer: %v", err)
	}
	return nil
}
