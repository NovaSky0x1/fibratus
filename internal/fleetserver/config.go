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
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the fleet server configuration.
type Config struct {
	Server        ServerConfig        `yaml:"server"`
	GRPC          GRPCConfig          `yaml:"grpc"`
	Database      DatabaseConfig      `yaml:"database"`
	ClickHouse    ClickHouseConfig    `yaml:"clickhouse"`
	NATS          NATSConfig          `yaml:"nats"`
	Elasticsearch ElasticsearchConfig `yaml:"elasticsearch"`
	Auth          AuthConfig          `yaml:"auth"`
	Agent         AgentConfig         `yaml:"agent"`
	Deployment    DeploymentConfig    `yaml:"deployment"`
	Logging       LoggingConfig       `yaml:"logging"`
	Dashboard     DashboardConfig     `yaml:"dashboard"`
}

// ClickHouseConfig configures the ClickHouse connection for telemetry.
//
// Two deployment modes are supported:
//   - Local/self-hosted: TLS off, native protocol on :9000
//   - ClickHouse Cloud: TLS on (Secure=true), native protocol on :9440
//
// For ClickHouse Cloud, set Host to the service hostname
// (e.g. "abc123.us-east-1.aws.clickhouse.cloud"), Port to 9440, Secure to true,
// and supply the service username/password issued in the Cloud console.
type ClickHouseConfig struct {
	Enabled         bool   `yaml:"enabled"`
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	Database        string `yaml:"database"`
	User            string `yaml:"user"`
	Password        string `yaml:"password"`
	Secure          bool   `yaml:"secure"`            // TLS — required for ClickHouse Cloud
	SkipVerify      bool   `yaml:"skip-verify"`       // skip TLS cert verification (dev only)
	DialTimeoutSecs int    `yaml:"dial-timeout-secs"` // 0 = driver default
	MaxOpenConns    int    `yaml:"max-open-conns"`
	MaxIdleConns    int    `yaml:"max-idle-conns"`
	ConnMaxLifetime int    `yaml:"conn-max-lifetime-secs"`
}

// ServerConfig configures the HTTP server.
type ServerConfig struct {
	Listen      string `yaml:"listen"`
	TLSCert     string `yaml:"tls-cert"`
	TLSKey      string `yaml:"tls-key"`
	ExternalURL string `yaml:"external-url"` // Public URL for agent enrollment (e.g., https://edr.novasky.io)
}

// DeploymentConfig configures agent deployment.
type DeploymentConfig struct {
	AgentBinaryPath string `yaml:"agent-binary-path"` // Path to the agent EXE on disk
	AgentConfigDir  string `yaml:"agent-config-dir"`  // Path to fibratus config files
	InstallDir      string `yaml:"install-dir"`       // Target install dir on endpoints (default: C:\Program Files\Fibratus)
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

// DSN builds a ClickHouse connection string compatible with the
// clickhouse-go v2 database/sql driver. It URL-encodes credentials and emits
// TLS / dial-timeout options as query parameters so ClickHouse Cloud and
// self-hosted deployments share the same code path.
func (c ClickHouseConfig) DSN() string {
	u := url.URL{
		Scheme: "clickhouse",
		Host:   fmt.Sprintf("%s:%d", c.Host, c.Port),
		Path:   "/" + c.Database,
	}
	if c.User != "" {
		u.User = url.UserPassword(c.User, c.Password)
	}
	q := url.Values{}
	if c.Secure {
		q.Set("secure", "true")
		if c.SkipVerify {
			q.Set("skip_verify", "true")
		}
	}
	if c.DialTimeoutSecs > 0 {
		q.Set("dial_timeout", strconv.Itoa(c.DialTimeoutSecs)+"s")
	}
	if len(q) > 0 {
		u.RawQuery = q.Encode()
	}
	return u.String()
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
		GRPC: GRPCConfig{
			Listen: ":8444",
		},
		NATS: NATSConfig{
			Enabled: false,
			URL:     "nats://localhost:4222",
			Subject: "fibratus.telemetry",
			Queue:   "fleet-consumers",
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
			Secure:          false,
			SkipVerify:      false,
			DialTimeoutSecs: 10,
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
		Deployment: DeploymentConfig{
			AgentBinaryPath: "",
			AgentConfigDir:  "",
			InstallDir:      `C:\Program Files\Fibratus`,
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

// SaveConfig writes cfg back to path as YAML. The write is performed via a
// temp file + rename so a partially written config can never appear on disk.
// File mode is preserved if the original file exists.
func SaveConfig(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".fleet-config-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("chmod temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp config: %w", err)
	}
	return nil
}
