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
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	natsPkg "github.com/rabbitstack/fibratus/internal/fleetserver/nats"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	pb "github.com/rabbitstack/fibratus/pkg/fleet/pb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// detectionDedup provides rate limiting for detections. The same rule+agent
// combination is allowed up to maxPerWindow detections within a sliding
// window. After the burst limit, further detections are suppressed until the
// window resets. This prevents a noisy rule from flooding with thousands of
// identical detections while still allowing legitimate repeated triggers.
type detectionDedup struct {
	mu   sync.Mutex
	hits map[string]*dedupEntry
}

type dedupEntry struct {
	count    int
	windowStart time.Time
}

const (
	dedupWindow    = 60 * time.Second  // sliding window duration
	maxPerWindow   = 20                // allow up to 20 detections per rule+agent per window
)

func newDetectionDedup() *detectionDedup {
	d := &detectionDedup{
		hits: make(map[string]*dedupEntry),
	}
	go d.cleanupLoop()
	return d
}

// shouldAllow returns true if this detection should be stored.
// Allows up to maxPerWindow detections per rule+agent per window.
func (d *detectionDedup) shouldAllow(ruleID, agentID string) bool {
	key := ruleID + "|" + agentID
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()

	entry, ok := d.hits[key]
	if !ok || now.Sub(entry.windowStart) > dedupWindow {
		// New window
		d.hits[key] = &dedupEntry{count: 1, windowStart: now}
		return true
	}
	entry.count++
	return entry.count <= maxPerWindow
}

func (d *detectionDedup) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		d.mu.Lock()
		now := time.Now()
		for k, e := range d.hits {
			if now.Sub(e.windowStart) > dedupWindow*2 {
				delete(d.hits, k)
			}
		}
		d.mu.Unlock()
	}
}

// agentService implements the pb.AgentServiceServer interface.
type agentService struct {
	pb.UnimplementedAgentServiceServer

	agents     store.AgentStore
	rules      store.RuleStore
	macros     store.MacroStore
	commands   store.CommandStore
	detections store.DetectionStore
	telemetry  store.TelemetryStore
	captures   store.CaptureStore
	streams    *StreamManager
	natsProd    *natsPkg.Producer // nil if NATS disabled (direct store writes)
	dedup       *detectionDedup
	onHeartbeat func(ctx context.Context, orgID, agentID string) // auto-update check callback
}

// newAgentService creates a new gRPC agent service.
func newAgentService(
	agents store.AgentStore,
	rules store.RuleStore,
	macros store.MacroStore,
	commands store.CommandStore,
	detections store.DetectionStore,
	telemetry store.TelemetryStore,
	captures store.CaptureStore,
	streams *StreamManager,
	natsProducer *natsPkg.Producer,
) *agentService {
	return &agentService{
		agents:     agents,
		rules:      rules,
		macros:     macros,
		commands:   commands,
		detections: detections,
		telemetry:  telemetry,
		captures:   captures,
		streams:  streams,
		natsProd: natsProducer,
		dedup:    newDetectionDedup(),
	}
}

// decommissionChecker is implemented by stores that track decommissioned agents.
type decommissionChecker interface {
	IsDecommissioned(ctx context.Context, agentID string) bool
}

// Register registers or re-registers an agent with the fleet server.
func (s *agentService) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	orgID := ctxutil.OrgIDFromContext(ctx)
	if orgID == "" {
		orgID = "default"
	}

	// Check if this agent ID was decommissioned — tell it to uninstall
	agentID := ctxutil.AgentIDFromContext(ctx)
	if agentID != "" {
		if checker, ok := s.agents.(decommissionChecker); ok && checker.IsDecommissioned(ctx, agentID) {
			log.Infof("grpc: decommissioned agent %s attempted to register — returning uninstall signal", agentID)
			return nil, status.Error(codes.PermissionDenied, "DECOMMISSIONED")
		}
	}

	// Check for existing agent by hostname (upsert)
	existing, err := s.agents.GetByHostname(ctx, orgID, req.Hostname)
	if err == nil && existing != nil {
		existing.OSVersion = req.OsVersion
		existing.EngineVersion = req.EngineVersion
		existing.Status = fleet.AgentOnline
		existing.LastHeartbeat = time.Now().UTC()
		if req.Tags != nil {
			existing.Tags = req.Tags
		}
		if err := s.agents.Update(ctx, existing); err != nil {
			return nil, status.Errorf(codes.Internal, "update agent: %v", err)
		}
		log.Infof("grpc: agent %s re-registered (hostname=%s, org=%s)", existing.ID, req.Hostname, orgID)
		return &pb.RegisterResponse{AgentId: existing.ID}, nil
	}

	// Create new agent
	if agentID == "" {
		agentID = uuid.New().String()
	}

	agent := &fleet.Agent{
		ID:            agentID,
		OrgID:         orgID,
		Hostname:      req.Hostname,
		OSVersion:     req.OsVersion,
		EngineVersion: req.EngineVersion,
		Tags:          req.Tags,
		Status:        fleet.AgentOnline,
		LastHeartbeat: time.Now().UTC(),
		RegisteredAt:  time.Now().UTC(),
	}
	if req.AgentGroup != "" {
		agent.GroupID = req.AgentGroup
	}

	if err := s.agents.Create(ctx, agent); err != nil {
		return nil, status.Errorf(codes.Internal, "create agent: %v", err)
	}

	log.Infof("grpc: new agent %s registered (hostname=%s, org=%s)", agentID, req.Hostname, orgID)
	return &pb.RegisterResponse{AgentId: agentID}, nil
}

// Heartbeat processes a single heartbeat from an agent.
func (s *agentService) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	orgID := ctxutil.OrgIDFromContext(ctx)
	agentID := req.AgentId
	if agentID == "" {
		agentID = ctxutil.AgentIDFromContext(ctx)
	}
	if agentID == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id required")
	}

	ts := time.Now().UTC()
	if req.Timestamp != nil {
		ts = req.Timestamp.AsTime()
	}

	hb := &fleet.Heartbeat{
		Timestamp:    ts,
		RulesVersion: req.RulesVersion,
		CPUPercent:   req.CpuPct,
		MemoryMB:     req.MemMb,
		EventsPerSec: req.EventsPerSec,
		ActiveRules:  int(req.ActiveRules),
	}

	if err := s.agents.UpdateHeartbeat(ctx, orgID, agentID, hb); err != nil {
		return nil, status.Errorf(codes.Internal, "heartbeat: %v", err)
	}

	// Check auto-update in background (non-blocking).
	// Use Background context since the gRPC ctx will cancel when this RPC returns.
	if s.onHeartbeat != nil {
		go s.onHeartbeat(context.Background(), orgID, agentID)
	}

	return &pb.HeartbeatResponse{Status: "ok"}, nil
}

// StreamTelemetry receives a stream of telemetry batches from the agent.
func (s *agentService) StreamTelemetry(stream pb.AgentService_StreamTelemetryServer) error {
	var totalAccepted int64

	for {
		batch, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&pb.TelemetryAck{Accepted: totalAccepted})
		}
		if err != nil {
			return status.Errorf(codes.Internal, "receive telemetry: %v", err)
		}

		// Separate capture events from normal telemetry.
		// capture_id is embedded in the Metadata JSON field (the proto
		// struct field CaptureId isn't in the raw descriptor so we read
		// from metadata instead).
		var captureEventsMap = make(map[string][]json.RawMessage) // captureID → events
		var telEvents []json.RawMessage

		for _, evt := range batch.Events {
			if len(evt.RawEvent) == 0 {
				continue
			}
			capID := extractCaptureIDFromMeta(evt.Metadata)
			if capID != "" {
				captureEventsMap[capID] = append(captureEventsMap[capID], json.RawMessage(evt.RawEvent))
			} else {
				telEvents = append(telEvents, json.RawMessage(evt.RawEvent))
			}
		}

		// Ingest capture events into capture store
		if s.captures != nil {
			for capID, evts := range captureEventsMap {
				if err := s.captures.IngestEvents(stream.Context(), capID, batch.OrgId, evts); err != nil {
					log.Warnf("grpc: capture event ingest error (capture %s): %v", capID, err)
				}
				if err := s.captures.IncrementEventCount(stream.Context(), capID, len(evts)); err != nil {
					log.Warnf("grpc: capture count update error (capture %s): %v", capID, err)
				}
			}
		}

		// Ingest normal telemetry
		if s.natsProd != nil {
			data, err := proto.Marshal(batch)
			if err != nil {
				log.Warnf("grpc: failed to marshal telemetry batch: %v", err)
				continue
			}
			if pubErr := s.natsProd.Publish(data); pubErr != nil {
				log.Warnf("grpc: nats publish error: %v", pubErr)
			}
		} else if len(telEvents) > 0 {
			hostname := batch.Hostname
			if hostname == "" {
				if agent, err := s.agents.Get(stream.Context(), batch.OrgId, batch.AgentId); err == nil {
					hostname = agent.Hostname
				}
			}
			if err := s.telemetry.BulkIngest(stream.Context(), batch.OrgId, batch.AgentId, hostname, telEvents); err != nil {
				log.Warnf("grpc: telemetry ingest error: %v", err)
			}
		}

		totalAccepted += int64(len(batch.Events))
	}
}

// SendDetection reports a detection to the fleet server.
func (s *agentService) SendDetection(ctx context.Context, req *pb.DetectionReport) (*pb.DetectionAck, error) {
	orgID := req.OrgId
	if orgID == "" {
		orgID = ctxutil.OrgIDFromContext(ctx)
	}

	hostname := req.AgentHostname
	if hostname == "" {
		if agent, err := s.agents.Get(ctx, orgID, req.AgentId); err == nil {
			hostname = agent.Hostname
		}
	}

	detID := uuid.New().String()
	ts := time.Now().UTC()
	if req.Timestamp != nil {
		ts = req.Timestamp.AsTime()
	}

	det := &fleet.Detection{
		ID:            detID,
		OrgID:         orgID,
		AgentID:       req.AgentId,
		AgentHostname: hostname,
		RuleID:        req.RuleId,
		RuleName:      req.RuleName,
		Title:         req.Title,
		Text:          req.Text,
		Description:   req.Description,
		Severity:      req.Severity,
		Labels:        req.Labels,
		Tags:          req.Tags,
		Events:        json.RawMessage(req.Events),
		Timestamp:     ts,
	}

	// Rate limit: allow up to 5 detections per rule+agent per 60s window
	if !s.dedup.shouldAllow(req.RuleId, req.AgentId) {
		return &pb.DetectionAck{DetectionId: detID}, nil // silently drop
	}

	if err := s.detections.Create(ctx, det); err != nil {
		return nil, status.Errorf(codes.Internal, "store detection: %v", err)
	}

	log.Infof("grpc: detection %s from agent %s (rule: %s, severity: %s)", detID, req.AgentId, req.RuleName, req.Severity)
	return &pb.DetectionAck{DetectionId: detID}, nil
}

// SubscribeRules opens a server-streaming channel that pushes rule updates.
func (s *agentService) SubscribeRules(req *pb.RuleSubscription, stream pb.AgentService_SubscribeRulesServer) error {
	agentID := req.AgentId
	orgID := req.OrgId
	if orgID == "" {
		orgID = ctxutil.OrgIDFromContext(stream.Context())
	}
	if agentID == "" {
		agentID = ctxutil.AgentIDFromContext(stream.Context())
	}

	// Send initial rule set immediately
	if err := s.pushCurrentRules(stream, orgID, agentID, req.CurrentVersion); err != nil {
		return err
	}

	// Register for future updates
	ch := s.streams.RegisterRuleStream(agentID, orgID)
	defer s.streams.UnregisterRuleStream(agentID)

	for {
		select {
		case update, ok := <-ch:
			if !ok {
				return nil // stream closed by server
			}
			if err := stream.Send(update); err != nil {
				return status.Errorf(codes.Internal, "send rule update: %v", err)
			}
		case <-stream.Context().Done():
			return nil
		}
	}
}

// pushCurrentRules sends the current ruleset to the agent on stream open.
// Always sends the full rule set — the agent compiles and caches locally.
// This ensures agents always have rules after reconnecting from server restarts.
func (s *agentService) pushCurrentRules(stream pb.AgentService_SubscribeRulesServer, orgID, agentID, currentVersion string) error {
	rules, etag, err := s.rules.GetForAgent(stream.Context(), orgID, agentID)
	if err != nil {
		return status.Errorf(codes.Internal, "get rules: %v", err)
	}

	log.Infof("streams: pushing %d rules to agent %s (org %s, etag %s)", len(rules), agentID, orgID, etag)
	update := buildRuleUpdate(rules, etag, orgID, s.macros, stream.Context())
	return stream.Send(update)
}

// buildRuleUpdate constructs a RuleUpdate proto from a list of rules.
func buildRuleUpdate(rules []*fleet.Rule, version, orgID string, macroStore store.MacroStore, ctx context.Context) *pb.RuleUpdate {
	var rulesYAML []byte
	for i, r := range rules {
		if !r.Enabled {
			continue
		}
		if i > 0 {
			rulesYAML = append(rulesYAML, []byte("\n---\n")...)
		}
		rulesYAML = append(rulesYAML, []byte(r.RawYAML)...)
	}

	var macrosYAML []byte
	if macroStore != nil {
		if combined, err := macroStore.GetAllForOrg(ctx, orgID); err == nil && combined != "" {
			macrosYAML = []byte(combined)
		}
	}

	return &pb.RuleUpdate{
		RulesYaml:  rulesYAML,
		MacrosYaml: macrosYAML,
		Version:    version,
		RuleCount:  int32(len(rules)),
	}
}

// CommandChannel handles the bidirectional command stream.
// Server pushes commands, agent sends back results.
func (s *agentService) CommandChannel(stream pb.AgentService_CommandChannelServer) error {
	// First message from agent identifies it
	firstMsg, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.Internal, "receive initial command msg: %v", err)
	}

	agentID := firstMsg.AgentId
	if agentID == "" {
		agentID = ctxutil.AgentIDFromContext(stream.Context())
	}
	orgID := ctxutil.OrgIDFromContext(stream.Context())

	// Register command stream
	cmdCh := s.streams.RegisterCommandStream(agentID, orgID)
	defer s.streams.UnregisterCommandStream(agentID)

	// Send any pending commands immediately
	pending, err := s.commands.GetPendingForAgent(stream.Context(), agentID)
	if err == nil {
		for _, cmd := range pending {
			if err := stream.Send(&pb.CommandPush{
				Id:      cmd.ID,
				Type:    cmd.Type,
				Payload: cmd.Payload,
			}); err != nil {
				return status.Errorf(codes.Internal, "send pending command: %v", err)
			}
			s.commands.MarkRunning(stream.Context(), cmd.ID)
		}
	}

	// Process results from agent and push new commands concurrently
	errCh := make(chan error, 1)

	// Goroutine: receive command results from agent
	go func() {
		for {
			result, err := stream.Recv()
			if err == io.EOF {
				errCh <- nil
				return
			}
			if err != nil {
				errCh <- err
				return
			}
			if result.CommandId != "" {
				var resultJSON json.RawMessage
				if len(result.Result) > 0 {
					resultJSON = json.RawMessage(result.Result)
				}
				if setErr := s.commands.SetResult(stream.Context(), result.CommandId, result.Status, resultJSON, result.ErrorMessage); setErr != nil {
					log.Warnf("grpc: failed to set command result %s: %v", result.CommandId, setErr)
				}
				log.Infof("grpc: command %s result: %s", result.CommandId, result.Status)
			}
		}
	}()

	// Main loop: push new commands to agent
	for {
		select {
		case cmd, ok := <-cmdCh:
			if !ok {
				return nil
			}
			if err := stream.Send(cmd); err != nil {
				return status.Errorf(codes.Internal, "send command: %v", err)
			}
			s.commands.MarkRunning(stream.Context(), cmd.Id)
		case err := <-errCh:
			return err
		case <-stream.Context().Done():
			return nil
		}
	}
}

// extractCaptureIDFromMeta parses the metadata JSON bytes and returns
// the capture_id value if present, or empty string otherwise.
func extractCaptureIDFromMeta(meta []byte) string {
	if len(meta) == 0 {
		return ""
	}
	var m map[string]string
	if json.Unmarshal(meta, &m) == nil {
		return m["capture_id"]
	}
	return ""
}
