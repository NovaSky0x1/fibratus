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

// Store wraps a PostgreSQL database connection.
type Store struct {
	db *sql.DB
}

// New creates a new PostgreSQL store.
func New(cfg fleetserver.DatabaseConfig) (*Store, error) {
	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to open: %w", err)
	}
	db.SetMaxOpenConns(cfg.MaxConnections)

	if err := db.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("postgres: failed to ping: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection for use by sub-stores.
func (s *Store) DB() *sql.DB {
	return s.db
}
