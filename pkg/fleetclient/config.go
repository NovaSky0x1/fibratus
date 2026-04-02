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

package fleetclient

import (
	"time"

	"github.com/spf13/pflag"
)

const (
	fleetEnabled              = "fleet.enabled"
	fleetServerURL            = "fleet.server-url"
	fleetAPIKey               = "fleet.api-key"
	fleetOrgID                = "fleet.org-id"
	fleetAgentGroup           = "fleet.agent-group"
	fleetHeartbeatInterval    = "fleet.heartbeat-interval"
	fleetRuleSyncInterval     = "fleet.rule-sync-interval"
	fleetEnableGzip           = "fleet.enable-gzip"
	fleetTLSCA                = "fleet.tls-ca"
	fleetTLSInsecureSkipVerify = "fleet.tls-insecure-skip-verify"
	fleetTimeout              = "fleet.timeout"
)

// Config contains the options for configuring the fleet client.
type Config struct {
	// Enabled determines whether the fleet client is active.
	Enabled bool `mapstructure:"enabled"`
	// ServerURL is the base URL of the fleet server.
	ServerURL string `mapstructure:"server-url"`
	// APIKey is the authentication key for the fleet server.
	APIKey string `mapstructure:"api-key"`
	// OrgID is the organization this agent belongs to.
	OrgID string `mapstructure:"org-id"`
	// AgentGroup is the group this agent belongs to.
	AgentGroup string `mapstructure:"agent-group"`
	// HeartbeatInterval is the interval between heartbeat sends.
	HeartbeatInterval time.Duration `mapstructure:"heartbeat-interval"`
	// RuleSyncInterval is the interval between rule sync checks.
	RuleSyncInterval time.Duration `mapstructure:"rule-sync-interval"`
	// EnableGzip specifies whether to gzip-compress request bodies.
	EnableGzip bool `mapstructure:"enable-gzip"`
	// TLSCA is the path to the CA certificate for TLS verification.
	TLSCA string `mapstructure:"tls-ca"`
	// TLSInsecureSkipVerify skips TLS certificate verification.
	TLSInsecureSkipVerify bool `mapstructure:"tls-insecure-skip-verify"`
	// Timeout is the HTTP request timeout.
	Timeout time.Duration `mapstructure:"timeout"`
}

// AddFlags registers persistent flags for the fleet client.
func AddFlags(flags *pflag.FlagSet) {
	flags.Bool(fleetEnabled, false, "Determines whether the fleet client is enabled")
	flags.String(fleetServerURL, "", "The base URL of the fleet server")
	flags.String(fleetAPIKey, "", "The API key for fleet server authentication")
	flags.String(fleetOrgID, "", "The organization ID this agent belongs to")
	flags.String(fleetAgentGroup, "default", "The agent group this agent belongs to")
	flags.Duration(fleetHeartbeatInterval, 30*time.Second, "The interval between heartbeat messages")
	flags.Duration(fleetRuleSyncInterval, 5*time.Minute, "The interval between rule synchronization checks")
	flags.Bool(fleetEnableGzip, true, "Enable gzip compression for request bodies")
	flags.String(fleetTLSCA, "", "Path to the CA certificate for TLS verification")
	flags.Bool(fleetTLSInsecureSkipVerify, false, "Skip TLS certificate verification")
	flags.Duration(fleetTimeout, 10*time.Second, "HTTP request timeout for fleet server communication")
}
