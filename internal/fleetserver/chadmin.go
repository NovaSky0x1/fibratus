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

// saveCHConfig validates the candidate, mutates the in-memory config, and
// persists the YAML file to disk. Connection pool fields default to sane
// values if the DTO leaves them at zero so the dashboard does not need to
// duplicate every default.
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

	s.config.ClickHouse = ClickHouseConfig{
		Enabled:         dto.Enabled,
		Host:            dto.Host,
		Port:            dto.Port,
		Database:        dto.Database,
		User:            dto.User,
		Password:        dto.Password,
		Secure:          dto.Secure,
		SkipVerify:      dto.SkipVerify,
		DialTimeoutSecs: dto.DialTimeoutSecs,
		MaxOpenConns:    dto.MaxOpenConns,
		MaxIdleConns:    dto.MaxIdleConns,
		ConnMaxLifetime: dto.ConnMaxLifetime,
	}
	return SaveConfig(s.configPath, s.config)
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
