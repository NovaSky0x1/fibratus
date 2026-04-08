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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"time"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ca"
	natsPkg "github.com/rabbitstack/fibratus/internal/fleetserver/nats"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	pb "github.com/rabbitstack/fibratus/pkg/fleet/pb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// GRPCConfig configures the gRPC server for agent communication.
type GRPCConfig struct {
	Listen  string `yaml:"listen"`  // e.g., ":8444"
	TLSCert string `yaml:"tls-cert"`
	TLSKey  string `yaml:"tls-key"`
}

// NATSConfig configures the NATS telemetry message queue.
type NATSConfig struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`     // e.g., "nats://localhost:4222"
	Subject string `yaml:"subject"` // e.g., "fibratus.telemetry"
	Queue   string `yaml:"queue"`   // consumer queue group
}

// GRPCServer wraps the gRPC server for agent communication.
type GRPCServer struct {
	server  *grpc.Server
	config  GRPCConfig
	streams *StreamManager
}

// NewGRPCServer creates and configures the gRPC server with agent and enrollment services.
func NewGRPCServer(
	cfg GRPCConfig,
	apiKeys map[string]bool,
	agents store.AgentStore,
	rules store.RuleStore,
	macros store.MacroStore,
	commands store.CommandStore,
	detections store.DetectionStore,
	telemetry store.TelemetryStore,
	captures store.CaptureStore,
	tokens store.EnrollmentTokenStore,
	caManager *ca.Manager,
	natsCfg NATSConfig,
	clientCAs *x509.CertPool,
) (*GRPCServer, error) {
	streams := NewStreamManager()

	// Set up NATS producer if enabled
	var natsProd *natsPkg.Producer
	if natsCfg.Enabled {
		var err error
		natsProd, err = natsPkg.NewProducer(natsPkg.ProducerConfig{
			URL:     natsCfg.URL,
			Subject: natsCfg.Subject,
		})
		if err != nil {
			return nil, fmt.Errorf("grpc server: nats producer: %w", err)
		}
	}

	// Server options
	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			loggingUnaryInterceptor(),
			agentAuthUnaryInterceptor(apiKeys),
		),
		grpc.ChainStreamInterceptor(
			agentAuthStreamInterceptor(apiKeys),
		),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    30 * time.Second,
			Timeout: 10 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.MaxRecvMsgSize(64 * 1024 * 1024), // 64MB for large telemetry batches
	}

	// TLS with optional mTLS for enrolled agents
	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("grpc server: load TLS cert: %w", err)
		}

		tlsCfg := &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.RequestClientCert, // accept but don't require
		}
		if clientCAs != nil {
			tlsCfg.ClientCAs = clientCAs
		}

		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsCfg)))
	}

	srv := grpc.NewServer(opts...)

	// Register services
	agentSvc := newAgentService(agents, rules, macros, commands, detections, telemetry, captures, streams, natsProd)
	enrollSvc := newEnrollmentService(tokens, agents, caManager)

	pb.RegisterAgentServiceServer(srv, agentSvc)
	pb.RegisterEnrollmentServiceServer(srv, enrollSvc)

	return &GRPCServer{
		server:  srv,
		config:  cfg,
		streams: streams,
	}, nil
}

// Serve starts the gRPC server on the configured address.
func (s *GRPCServer) Serve() error {
	lis, err := net.Listen("tcp", s.config.Listen)
	if err != nil {
		return fmt.Errorf("grpc server: listen %s: %w", s.config.Listen, err)
	}

	log.Infof("grpc server listening on %s", s.config.Listen)
	return s.server.Serve(lis)
}

// Stop gracefully stops the gRPC server.
func (s *GRPCServer) Stop() {
	s.server.GracefulStop()
}

// Streams returns the stream manager for use by HTTP handlers
// (e.g., when a rule is created via dashboard, push to agents).
func (s *GRPCServer) Streams() *StreamManager {
	return s.streams
}
