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

package config

import (
	"github.com/rabbitstack/fibratus/pkg/fleetclient"
	"github.com/spf13/viper"
)

// FleetConfig stores the fleet client configuration.
type FleetConfig struct {
	fleetclient.Config `mapstructure:",squash"`
}

func (c *FleetConfig) initFromViper(v *viper.Viper) {
	c.Enabled = v.GetBool("fleet.enabled")
	c.ServerURL = v.GetString("fleet.server-url")
	c.APIKey = v.GetString("fleet.api-key")
	c.OrgID = v.GetString("fleet.org-id")
	c.AgentGroup = v.GetString("fleet.agent-group")
	c.HeartbeatInterval = v.GetDuration("fleet.heartbeat-interval")
	c.RuleSyncInterval = v.GetDuration("fleet.rule-sync-interval")
	c.EnableGzip = v.GetBool("fleet.enable-gzip")
	c.TLSCA = v.GetString("fleet.tls-ca")
	c.TLSInsecureSkipVerify = v.GetBool("fleet.tls-insecure-skip-verify")
	c.Timeout = v.GetDuration("fleet.timeout")
}
