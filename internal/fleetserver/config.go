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
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the fleet server configuration.
type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Database      DatabaseConfig      `yaml:"database"`
	ClickHouse    ClickHouseConfig    `yaml:"clickhouse"`
	Elasticsearch ElasticsearchConfig `yaml:"elasticsearch"`
	Auth          AuthConfig          `yaml:"auth"`
	Agent         AgentConfig         `yaml:"agent"`
	Logging       LoggingConfig       `yaml:"logging"`
	Dashboard     DashboardConfig     `yaml:"dashboard"`
}

// ClickHouseConfig configures the ClickHouse connection for telemetry.
type ClickHouseConfig struct {
	Enabled        bool   `yaml:"enabled"`
	Host           string `yaml:"host"`
	Port           int    `yaml:"port"`
	Database       string `yaml:"database"`
	User           string `yaml:"user"`
	Password       string `yaml:"password"`
	MaxOpenConns   int    `yaml:"max-open-conns"`
	MaxIdleConns   int    `yaml:"max-idle-conns"`
	ConnMaxLifetime int   `yaml:"conn-max-lifetime-secs"`
}

// ServerConfig configures the HTTP server.
type ServerConfig struct {
	Listen  string `yaml:"listen"`
	TLSCert string `yaml:"tls-cert"`
	TLSKey  string `yaml:"tls-key"`
}

// DatabaseConfig configures the PostgreSQL connection.
type DatabaseConfig struct {
	Host           string `yaml:"host"`
	Port           int    `yaml:"port"`
	Name           string `yaml:"name"`
	User           string `yaml:"user"`
	Password       string `yaml:"password"`
	SSLMode        string `yaml:"ssl-mode"`
	MaxConnections int    `yaml:"max-connections"`
}

// DSN builds a PostgreSQL connection string.
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		d.Host, d.Port, d.Name, d.User, d.Password, d.SSLMode,
	)
}

// DSN builds a ClickHouse connection string for database/sql.
func (c ClickHouseConfig) DSN() string {
	dsn := fmt.Sprintf("clickhouse://%s:%d/%s", c.Host, c.Port, c.Database)
	if c.User != "" {
		dsn += fmt.Sprintf("?username=%s&password=%s", c.User, c.Password)
	}
	return dsn
}

// ElasticsearchConfig configures the Elasticsearch connection.
type ElasticsearchConfig struct {
	Servers     []string      `yaml:"servers"`
	IndexPrefix string        `yaml:"index-prefix"`
	Username    string        `yaml:"username"`
	Password    string        `yaml:"password"`
	BulkWorkers int           `yaml:"bulk-workers"`
	FlushPeriod time.Duration `yaml:"flush-period"`
}

// AuthConfig configures authentication.
type AuthConfig struct {
	APIKeys   []APIKeyConfig `yaml:"api-keys"`
	JWTSecret string         `yaml:"jwt-secret"`
}

// APIKeyConfig represents a named API key.
// Kept for backward compatibility with Phase 1 agents.
type APIKeyConfig struct {
	Name string `yaml:"name"`
	Key  string `yaml:"key"`
}

// AgentConfig configures agent management behavior.
type AgentConfig struct {
	HeartbeatTimeout time.Duration `yaml:"heartbeat-timeout"`
	OfflineThreshold time.Duration `yaml:"offline-threshold"`
}

// LoggingConfig configures logging.
type LoggingConfig struct {
	Level string `yaml:"level"`
}

// DashboardConfig configures the web dashboard.
type DashboardConfig struct {
	Enabled bool `yaml:"enabled"`
}

// LoadConfig reads and parses the fleet server configuration file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	cfg := &Config{
		Server: ServerConfig{
			Listen: ":8443",
		},
		Database: DatabaseConfig{
			Host:           "localhost",
			Port:           5432,
			Name:           "fibratus_fleet",
			User:           "fibratus",
			SSLMode:        "disable",
			MaxConnections: 50,
		},
		ClickHouse: ClickHouseConfig{
			Enabled:         false,
			Host:            "localhost",
			Port:            9000,
			Database:        "fibratus",
			User:            "default",
			Password:        "",
			MaxOpenConns:    20,
			MaxIdleConns:    10,
			ConnMaxLifetime: 3600,
		},
		Elasticsearch: ElasticsearchConfig{
			Servers:     []string{"http://localhost:9200"},
			IndexPrefix: "fibratus",
			BulkWorkers: 2,
			FlushPeriod: time.Second,
		},
		Agent: AgentConfig{
			HeartbeatTimeout: 90 * time.Second,
			OfflineThreshold: 5 * time.Minute,
		},
		Logging: LoggingConfig{
			Level: "info",
		},
		Dashboard: DashboardConfig{
			Enabled: true,
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return cfg, nil
}
