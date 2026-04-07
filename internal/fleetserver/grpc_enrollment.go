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
	"time"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ca"
	"github.com/rabbitstack/fibratus/internal/fleetserver/handler"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	pb "github.com/rabbitstack/fibratus/pkg/fleet/pb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// enrollmentService implements pb.EnrollmentServiceServer.
type enrollmentService struct {
	pb.UnimplementedEnrollmentServiceServer

	tokens store.EnrollmentTokenStore
	agents store.AgentStore
	ca     *ca.Manager
}

// newEnrollmentService creates a new gRPC enrollment service.
func newEnrollmentService(tokens store.EnrollmentTokenStore, agents store.AgentStore, caManager *ca.Manager) *enrollmentService {
	return &enrollmentService{
		tokens: tokens,
		agents: agents,
		ca:     caManager,
	}
}

// Enroll handles agent enrollment via gRPC.
// No authentication required — the enrollment token IS the credential.
func (s *enrollmentService) Enroll(ctx context.Context, req *pb.EnrollRequest) (*pb.EnrollResponse, error) {
	if req.Token == "" {
		return nil, status.Error(codes.InvalidArgument, "enrollment token is required")
	}
	if req.Hostname == "" {
		return nil, status.Error(codes.InvalidArgument, "hostname is required")
	}
	if req.Csr == "" {
		return nil, status.Error(codes.InvalidArgument, "CSR is required")
	}

	// 1. Validate token
	token, err := s.tokens.Get(ctx, req.Token)
	if err != nil {
		log.Errorf("grpc enroll: token lookup error: %v", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	if token == nil {
		return nil, status.Error(codes.Unauthenticated, "invalid enrollment token")
	}
	if !token.IsValid() {
		return nil, status.Error(codes.Unauthenticated, "enrollment token expired or exhausted")
	}

	// 2. Get or create org CA
	orgCA, err := s.ca.GetOrCreateCA(ctx, token.OrgID, token.AccountID)
	if err != nil {
		log.Errorf("grpc enroll: CA error: %v", err)
		return nil, status.Error(codes.Internal, "failed to initialize org CA")
	}

	// 3. Remove existing agent with same hostname+org (re-enrollment)
	if existing, _ := s.agents.GetByHostname(ctx, token.OrgID, req.Hostname); existing != nil {
		log.Infof("grpc enroll: removing previous agent %s for re-enrolling host %s", existing.ID, req.Hostname)
		_ = s.agents.Delete(ctx, token.OrgID, existing.ID)
	}

	// 4. Create agent record
	agentID := handler.GenerateID()
	now := time.Now().UTC()

	agent := &fleet.Agent{
		ID:            agentID,
		OrgID:         token.OrgID,
		Hostname:      req.Hostname,
		OSVersion:     req.OsVersion,
		EngineVersion: req.EngineVersion,
		Status:        fleet.AgentOnline,
		LastHeartbeat: now,
		RegisteredAt:  now,
	}

	if err := s.agents.Create(ctx, agent); err != nil {
		log.Errorf("grpc enroll: agent create error: %v", err)
		return nil, status.Error(codes.Internal, "failed to create agent")
	}

	// 5. Sign CSR
	signedCert, err := s.ca.SignCSR(orgCA, []byte(req.Csr), agentID, token.OrgID, token.AccountID)
	if err != nil {
		log.Errorf("grpc enroll: sign CSR error: %v", err)
		return nil, status.Errorf(codes.InvalidArgument, "failed to sign CSR: %v", err)
	}

	// 6. Increment token usage
	if err := s.tokens.IncrementUses(ctx, token.ID); err != nil {
		log.Warnf("grpc enroll: token increment error: %v", err)
	}

	log.WithFields(log.Fields{
		"agent":    agentID,
		"hostname": req.Hostname,
		"org":      token.OrgID,
		"token":    token.ID[:12] + "...",
	}).Info("grpc: agent enrolled")

	return &pb.EnrollResponse{
		AgentId:    agentID,
		OrgId:      token.OrgID,
		SignedCert: string(signedCert),
		CaCert:     string(orgCA.CertPEM),
	}, nil
}
