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

package store

import (
	"context"
	"time"

	"github.com/rabbitstack/fibratus/pkg/fleet"
)

// AgentStore manages agent persistence.
type AgentStore interface {
	Create(ctx context.Context, agent *fleet.Agent) error
	Get(ctx context.Context, id string) (*fleet.Agent, error)
	GetByHostname(ctx context.Context, hostname string) (*fleet.Agent, error)
	List(ctx context.Context, opts fleet.AgentListOptions) ([]*fleet.Agent, int, error)
	Update(ctx context.Context, agent *fleet.Agent) error
	Delete(ctx context.Context, id string) error
	UpdateHeartbeat(ctx context.Context, id string, hb *fleet.Heartbeat) error
	CountByStatus(ctx context.Context) (map[fleet.AgentStatus]int, error)
	MarkOfflineAgents(ctx context.Context, timeout time.Duration) (int, error)
}

// RuleStore manages rule persistence.
type RuleStore interface {
	Create(ctx context.Context, rule *fleet.Rule) error
	Get(ctx context.Context, id string) (*fleet.Rule, error)
	List(ctx context.Context, opts fleet.ListOptions) ([]*fleet.Rule, int, error)
	Update(ctx context.Context, rule *fleet.Rule) error
	Delete(ctx context.Context, id string) error
	GetForAgent(ctx context.Context, agentID string) ([]*fleet.Rule, string, error)
}

// GroupStore manages agent group persistence.
type GroupStore interface {
	Create(ctx context.Context, group *fleet.AgentGroup) error
	Get(ctx context.Context, id string) (*fleet.AgentGroup, error)
	List(ctx context.Context) ([]*fleet.AgentGroup, error)
	Update(ctx context.Context, group *fleet.AgentGroup) error
	Delete(ctx context.Context, id string) error
	AssignRules(ctx context.Context, groupID string, ruleIDs []string) error
}

// DetectionStore manages detection persistence and querying.
type DetectionStore interface {
	Create(ctx context.Context, det *fleet.Detection) error
	Get(ctx context.Context, id string) (*fleet.Detection, error)
	List(ctx context.Context, opts fleet.DetectionListOptions) ([]*fleet.Detection, int, error)
	Count24h(ctx context.Context) (int, error)
	CountBySeverity(ctx context.Context) (map[string]int, error)
	Timeline(ctx context.Context, from, to time.Time, interval string) ([]fleet.TimelineBucket, error)
	MitreHeatmap(ctx context.Context, from, to time.Time) ([]fleet.MitreCell, error)
}

// TelemetryStore manages telemetry event persistence and search.
type TelemetryStore interface {
	BulkIndex(ctx context.Context, agentID string, events []byte) error
	Search(ctx context.Context, query string, from, to time.Time, agentID string, limit int) ([]byte, int, error)
}
