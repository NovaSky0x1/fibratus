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
	"time"

	"github.com/rabbitstack/fibratus/internal/fleetserver/handler"
	log "github.com/sirupsen/logrus"
)

// getCHConfigDTO returns the current ClickHouse configuration as the DTO the
// dashboard expects. Mode is derived from Secure so the UI can render either
// the local or the Cloud form.
func (s *Server) getCHConfigDTO() handler.ClickHouseConfigDTO {
	c := s.config.ClickHouse
	mode := "local"
	if c.Secure {
		mode = "cloud"
	}
	return handler.ClickHouseConfigDTO{
		Mode:            mode,
		Enabled:         c.Enabled,
		Host:            c.Host,
		Port:            c.Port,
		Database:        c.Database,
		User:            c.User,
		Password:        c.Password,
		Secure:          c.Secure,
		SkipVerify:      c.SkipVerify,
		DialTimeoutSecs: c.DialTimeoutSecs,
		MaxOpenConns:    c.MaxOpenConns,
		MaxIdleConns:    c.MaxIdleConns,
		ConnMaxLifetime: c.ConnMaxLifetime,
	}
}

// saveCHConfig validates the candidate, mutates the in-memory config, persists
// the password to the encrypted secret store, and writes the rest of the
// config back to YAML (with the password field scrubbed so it never sits on
// disk again). Connection pool fields default to sane values if the DTO
// leaves them at zero so the dashboard does not need to duplicate every default.
func (s *Server) saveCHConfig(dto handler.ClickHouseConfigDTO) error {
	if s.configPath == "" {
		return errors.New("config path unknown — cannot persist changes")
	}
	if dto.Host == "" {
		return errors.New("host is required")
	}
	if dto.Port <= 0 {
		return errors.New("port must be positive")
	}
	if dto.Database == "" {
		return errors.New("database is required")
	}
	if dto.MaxOpenConns <= 0 {
		dto.MaxOpenConns = 20
	}
	if dto.MaxIdleConns <= 0 {
		dto.MaxIdleConns = 10
	}
	if dto.ConnMaxLifetime <= 0 {
		dto.ConnMaxLifetime = 3600
	}
	if dto.DialTimeoutSecs <= 0 {
		dto.DialTimeoutSecs = 10
	}

	// Password handling: if the DTO carries a non-empty password, persist it
	// to the encrypted store. Empty means "keep what's already stored".
	if dto.Password != "" && s.secretStore != nil {
		if err := s.secretStore.Set(context.Background(), SecretClickHousePassword, dto.Password, "dashboard"); err != nil {
			return err
		}
	}

	s.config.ClickHouse = ClickHouseConfig{
		Enabled:         dto.Enabled,
		Host:            dto.Host,
		Port:            dto.Port,
		Database:        dto.Database,
		User:            dto.User,
		Password:        dto.Password, // kept in-memory so the live connection can reuse it without a round-trip
		Secure:          dto.Secure,
		SkipVerify:      dto.SkipVerify,
		DialTimeoutSecs: dto.DialTimeoutSecs,
		MaxOpenConns:    dto.MaxOpenConns,
		MaxIdleConns:    dto.MaxIdleConns,
		ConnMaxLifetime: dto.ConnMaxLifetime,
	}
	if dto.Password == "" {
		// Reload from the encrypted store so the in-memory copy stays accurate
		// even when the dashboard sends a redacted DTO.
		if pw, err := s.secretStore.Get(context.Background(), SecretClickHousePassword); err == nil {
			s.config.ClickHouse.Password = pw
		}
	}

	// Persist YAML without the password — secret store is the source of truth.
	yamlSafe := *s.config
	yamlSafe.ClickHouse.Password = ""
	if err := SaveConfig(s.configPath, &yamlSafe); err != nil {
		return err
	}
	log.Info("fleet: ClickHouse config saved (YAML scrubbed; password held in secret store)")
	return nil
}

// testCHConfig opens a one-off connection with the candidate config and runs
// SELECT version() to prove credentials, TLS, and reachability. The DB handle
// is closed before returning so we never leak connections from the test path.
func (s *Server) testCHConfig(dto handler.ClickHouseConfigDTO) handler.CHTestResult {
	cfg := ClickHouseConfig{
		Host:            dto.Host,
		Port:            dto.Port,
		Database:        dto.Database,
		User:            dto.User,
		Password:        dto.Password,
		Secure:          dto.Secure,
		SkipVerify:      dto.SkipVerify,
		DialTimeoutSecs: dto.DialTimeoutSecs,
	}
	if cfg.DialTimeoutSecs <= 0 {
		cfg.DialTimeoutSecs = 10
	}
	db, err := sql.Open("clickhouse", cfg.DSN())
	if err != nil {
		return handler.CHTestResult{OK: false, Error: err.Error()}
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.DialTimeoutSecs+5)*time.Second)
	defer cancel()

	start := time.Now()
	if err := db.PingContext(ctx); err != nil {
		return handler.CHTestResult{OK: false, Error: err.Error()}
	}
	var version string
	if err := db.QueryRowContext(ctx, "SELECT version()").Scan(&version); err != nil {
		return handler.CHTestResult{OK: false, Error: err.Error()}
	}
	return handler.CHTestResult{
		OK:        true,
		Version:   version,
		LatencyMs: time.Since(start).Milliseconds(),
	}
}
