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
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	pb "github.com/rabbitstack/fibratus/pkg/fleet/pb"
	"github.com/rabbitstack/fibratus/pkg/util/version"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	agentIDFile = "agent-id"
)

var userAgent = version.ProductToken()

// Client handles communication with the fleet server over gRPC.
type Client struct {
	conn             *grpc.ClientConn
	agentClient      pb.AgentServiceClient
	enrollClient     pb.EnrollmentServiceClient
	config           Config
	agentID          string
	orgID            string
	hostname         string
	serverAddr       string
	dataDir          string
	mu               sync.RWMutex
	stopCh           chan struct{}
	wg               sync.WaitGroup
	ruleSyncCallback RuleSyncCallback
}

// New creates a new fleet client with gRPC transport.
// The dataDir is used to persist the agent ID and certificates across restarts.
func New(config Config, dataDir string) (*Client, error) {
	// Load persisted enrollment data — overrides config file settings.
	if serverURL := loadFile(filepath.Join(dataDir, "server-url")); serverURL != "" {
		config.ServerURL = serverURL
		log.Infof("fleet: loaded server URL from enrollment: %s", serverURL)
	}
	if orgID := loadFile(filepath.Join(dataDir, "org-id")); orgID != "" {
		config.OrgID = orgID
	}

	// Parse server address for gRPC (strip scheme, use port 8444 for gRPC)
	serverAddr := parseGRPCAddr(config.ServerURL)

	// Build TLS config
	tlsCfg := &tls.Config{
		InsecureSkipVerify: config.TLSInsecureSkipVerify,
	}

	// Load enrollment certificates for mTLS
	certFile := filepath.Join(dataDir, "certs", "agent.crt")
	keyFile := filepath.Join(dataDir, "certs", "agent.key")
	if fileExists(certFile) && fileExists(keyFile) {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			log.Warnf("fleet: failed to load enrollment certs: %v", err)
		} else {
			tlsCfg.Certificates = []tls.Certificate{cert}
			log.Info("fleet: using enrollment certificates for mTLS")
		}
	}

	// Dial options
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(64 * 1024 * 1024),
			grpc.MaxCallSendMsgSize(64 * 1024 * 1024),
		),
	}

	conn, err := grpc.NewClient(serverAddr, opts...)
	if err != nil {
		return nil, fmt.Errorf("fleet: dial %s: %w", serverAddr, err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	c := &Client{
		conn:         conn,
		agentClient:  pb.NewAgentServiceClient(conn),
		enrollClient: pb.NewEnrollmentServiceClient(conn),
		config:       config,
		orgID:        config.OrgID,
		hostname:     hostname,
		serverAddr:   serverAddr,
		dataDir:      dataDir,
		stopCh:       make(chan struct{}),
	}

	// Load persisted agent ID
	c.agentID = c.loadAgentID()

	return c, nil
}

// parseGRPCAddr converts a URL like "https://edr.novasky.io" to "edr.novasky.io:443"
// gRPC traffic goes through Nginx on port 443, which proxies to the gRPC backend.
func parseGRPCAddr(serverURL string) string {
	addr := serverURL
	addr = strings.TrimPrefix(addr, "https://")
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimRight(addr, "/")

	// If no port specified, use HTTPS port (Nginx terminates TLS, proxies gRPC)
	if !strings.Contains(addr, ":") {
		addr += ":443"
	}
	return addr
}

// grpcCtx creates a context with agent identity metadata.
func (c *Client) grpcCtx() context.Context {
	ctx := context.Background()
	md := metadata.Pairs(
		"x-agent-id", c.agentID,
		"x-org-id", c.orgID,
	)
	if c.config.APIKey != "" {
		md.Append("x-api-key", c.config.APIKey)
	}
	return metadata.NewOutgoingContext(ctx, md)
}

// grpcCtxWithTimeout creates a context with timeout and agent identity metadata.
func (c *Client) grpcCtxWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx := c.grpcCtx()
	return context.WithTimeout(ctx, timeout)
}

// ErrDecommissioned is returned when the server indicates this agent was deleted
// and should self-uninstall.
var ErrDecommissioned = fmt.Errorf("agent decommissioned by server")

// Register registers this agent with the fleet server.
func (c *Client) Register() error {
	ctx, cancel := c.grpcCtxWithTimeout(c.config.Timeout)
	defer cancel()

	resp, err := c.agentClient.Register(ctx, &pb.RegisterRequest{
		Hostname:      c.hostname,
		OsVersion:     osVersion(),
		EngineVersion: version.Get(),
		AgentGroup:    c.config.AgentGroup,
	})
	if err != nil {
		// Check if server says we're decommissioned
		if strings.Contains(err.Error(), "DECOMMISSIONED") {
			log.Warn("fleet: this agent has been decommissioned by the server — initiating self-uninstall")
			return ErrDecommissioned
		}
		return fmt.Errorf("fleet register: %w", err)
	}

	agentID := resp.AgentId
	if agentID == "" {
		return fmt.Errorf("fleet register: server returned empty agent ID")
	}

	c.mu.Lock()
	c.agentID = agentID
	c.mu.Unlock()

	c.persistAgentID(agentID)

	log.Infof("fleet: registered with server as agent %s (gRPC)", agentID)
	return nil
}

// SendHeartbeat sends a single heartbeat to the fleet server.
func (c *Client) SendHeartbeat(hb *pb.HeartbeatRequest) error {
	c.mu.RLock()
	agentID := c.agentID
	c.mu.RUnlock()

	if agentID == "" {
		return fmt.Errorf("fleet heartbeat: agent not registered")
	}

	hb.AgentId = agentID
	if hb.Timestamp == nil {
		hb.Timestamp = timestamppb.Now()
	}

	ctx, cancel := c.grpcCtxWithTimeout(c.config.Timeout)
	defer cancel()

	_, err := c.agentClient.Heartbeat(ctx, hb)
	if err != nil {
		return fmt.Errorf("fleet heartbeat: %w", err)
	}
	return nil
}

// SendDetection reports a detection to the fleet server.
func (c *Client) SendDetection(det *pb.DetectionReport) error {
	c.mu.RLock()
	agentID := c.agentID
	c.mu.RUnlock()

	det.AgentId = agentID
	det.OrgId = c.orgID
	det.AgentHostname = c.hostname

	ctx, cancel := c.grpcCtxWithTimeout(c.config.Timeout)
	defer cancel()

	_, err := c.agentClient.SendDetection(ctx, det)
	if err != nil {
		return fmt.Errorf("fleet detection: %w", err)
	}
	return nil
}

// AgentID returns the current agent ID.
func (c *Client) AgentID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.agentID
}

// Hostname returns the agent hostname.
func (c *Client) Hostname() string {
	return c.hostname
}

// AgentServiceClient returns the raw gRPC agent client for direct stream use.
func (c *Client) AgentServiceClient() pb.AgentServiceClient {
	return c.agentClient
}

// GRPCCtx returns a context with agent identity metadata (for stream setup).
func (c *Client) GRPCCtx() context.Context {
	return c.grpcCtx()
}

// Close shuts down the fleet client and stops background goroutines.
func (c *Client) Close() error {
	close(c.stopCh)
	c.wg.Wait()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// loadAgentID loads a previously persisted agent ID from disk.
func (c *Client) loadAgentID() string {
	return loadFile(filepath.Join(c.dataDir, agentIDFile))
}

// loadFile reads a single-value text file, returning empty string on error.
func loadFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// persistAgentID saves the agent ID to disk for persistence across restarts.
func (c *Client) persistAgentID(id string) {
	if c.dataDir == "" {
		return
	}
	path := filepath.Join(c.dataDir, agentIDFile)
	if err := os.MkdirAll(c.dataDir, 0o755); err != nil {
		log.Warnf("fleet: failed to create data dir: %v", err)
		return
	}
	if err := os.WriteFile(path, []byte(id), 0o600); err != nil {
		log.Warnf("fleet: failed to persist agent ID: %v", err)
	}
}
