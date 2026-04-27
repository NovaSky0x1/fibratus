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
	"time"

	"github.com/rabbitstack/fibratus/internal/fleetserver/handler"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store/postgres"
)

// chProfileHandlerDeps wires CHProfileHandler closures to server-side stores.
func (s *Server) chProfileHandlerDeps() handler.CHProfileHandlerDeps {
	return handler.CHProfileHandlerDeps{
		List: func() ([]handler.ClickHouseProfileDTO, error) {
			ctx := context.Background()
			rows, err := s.profileStore.List(ctx)
			if err != nil {
				return nil, err
			}
			active, _ := s.secretStore.GetOr(ctx, SecretActiveProfile, "")
			out := make([]handler.ClickHouseProfileDTO, 0, len(rows))
			for _, p := range rows {
				out = append(out, s.profileToDTO(ctx, p, active))
			}
			return out, nil
		},
		Get: func(name string) (*handler.ClickHouseProfileDTO, error) {
			ctx := context.Background()
			p, err := s.profileStore.Get(ctx, name)
			if err != nil {
				return nil, err
			}
			active, _ := s.secretStore.GetOr(ctx, SecretActiveProfile, "")
			dto := s.profileToDTO(ctx, p, active)
			return &dto, nil
		},
		Upsert: func(req handler.ClickHouseProfileDTO) error {
			ctx := context.Background()
			p := dtoToProfile(req)
			if err := s.profileStore.Upsert(ctx, p); err != nil {
				return err
			}
			if req.Password != "" {
				if err := s.SetProfilePassword(ctx, req.Name, req.Password, "dashboard"); err != nil {
					return err
				}
			}
			return nil
		},
		Activate: func(name string) error {
			return s.activateProfile(context.Background(), name)
		},
		Test: func(name string) handler.ProfileTestResult {
			ctx := context.Background()
			p, err := s.profileStore.Get(ctx, name)
			if err != nil {
				return handler.ProfileTestResult{OK: false, Error: err.Error()}
			}
			pw, err := s.resolvePassword(ctx, name)
			if err != nil {
				return handler.ProfileTestResult{OK: false, Error: err.Error()}
			}
			cfg := profileToClickHouseConfig(p, pw)
			if cfg.DialTimeoutSecs <= 0 {
				cfg.DialTimeoutSecs = 10
			}
			db, err := sql.Open("clickhouse", cfg.DSN())
			if err != nil {
				return handler.ProfileTestResult{OK: false, Error: err.Error()}
			}
			defer db.Close()
			cctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.DialTimeoutSecs+5)*time.Second)
			defer cancel()
			start := time.Now()
			if err := db.PingContext(cctx); err != nil {
				return handler.ProfileTestResult{OK: false, Error: err.Error()}
			}
			var version string
			if err := db.QueryRowContext(cctx, "SELECT version()").Scan(&version); err != nil {
				return handler.ProfileTestResult{OK: false, Error: err.Error()}
			}
			return handler.ProfileTestResult{
				OK:        true,
				Version:   version,
				LatencyMs: time.Since(start).Milliseconds(),
			}
		},
	}
}

// profileToDTO maps a store row + the live active-pointer + has-password flag.
func (s *Server) profileToDTO(ctx context.Context, p *postgres.ClickHouseProfile, active string) handler.ClickHouseProfileDTO {
	hasPw, _ := s.secretStore.Has(ctx, SecretProfilePassword(p.Name))
	return handler.ClickHouseProfileDTO{
		Name:                p.Name,
		Active:              p.Name == active,
		Enabled:             p.Enabled,
		Host:                p.Host,
		Port:                p.Port,
		Database:            p.Database,
		User:                p.User,
		HasPassword:         hasPw,
		Secure:              p.Secure,
		SkipVerify:          p.SkipVerify,
		DialTimeoutSecs:     p.DialTimeoutSecs,
		MaxOpenConns:        p.MaxOpenConns,
		MaxIdleConns:        p.MaxIdleConns,
		ConnMaxLifetimeSecs: p.ConnMaxLifetimeSecs,
		CloudOrgID:          p.CloudOrgID,
		CloudServiceID:      p.CloudServiceID,
	}
}

func dtoToProfile(d handler.ClickHouseProfileDTO) *postgres.ClickHouseProfile {
	return &postgres.ClickHouseProfile{
		Name:                d.Name,
		Enabled:             d.Enabled,
		Host:                d.Host,
		Port:                d.Port,
		Database:            d.Database,
		User:                d.User,
		Secure:              d.Secure,
		SkipVerify:          d.SkipVerify,
		DialTimeoutSecs:     d.DialTimeoutSecs,
		MaxOpenConns:        d.MaxOpenConns,
		MaxIdleConns:        d.MaxIdleConns,
		ConnMaxLifetimeSecs: d.ConnMaxLifetimeSecs,
		CloudOrgID:          d.CloudOrgID,
		CloudServiceID:      d.CloudServiceID,
		UpdatedBy:           "dashboard",
	}
}

