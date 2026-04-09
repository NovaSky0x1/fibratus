/*
 * Copyright 2021-2026 by Nedim Sabic Sabic
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

package eventlog

// ChannelConfig represents the configuration for a single event log channel subscription.
type ChannelConfig struct {
	// Name is the Windows Event Log channel path (e.g. "Security", "Microsoft-Windows-Sysmon/Operational").
	Name string `json:"name" yaml:"name"`
	// CollectAll when true collects all event IDs from this channel.
	CollectAll bool `json:"collect_all" yaml:"collect_all"`
	// EventIDs is the list of specific event IDs to collect. Ignored when CollectAll is true.
	EventIDs []uint16 `json:"event_ids,omitempty" yaml:"event_ids,omitempty"`
}

// Config represents the event log collection configuration.
type Config struct {
	// Enabled indicates whether event log collection is active.
	Enabled bool `json:"enabled" yaml:"enabled"`
	// Channels is the list of event log channels to subscribe to.
	Channels []ChannelConfig `json:"channels" yaml:"channels"`
	// BookmarkPath is the directory where bookmark files are stored for reliable delivery.
	BookmarkPath string `json:"bookmark_path,omitempty" yaml:"bookmark_path,omitempty"`
}

// DefaultConfig returns the default event log collection configuration (disabled).
func DefaultConfig() Config {
	return Config{Enabled: false}
}

// RecommendedChannels returns a recommended set of channels for security monitoring.
func RecommendedChannels() []ChannelConfig {
	return []ChannelConfig{
		{Name: "Security", CollectAll: true},
		{Name: "System", CollectAll: true},
		{Name: "Microsoft-Windows-PowerShell/Operational", CollectAll: true},
		{Name: "Microsoft-Windows-Sysmon/Operational", CollectAll: true},
		{Name: "Microsoft-Windows-Windows Defender/Operational", CollectAll: true},
	}
}
