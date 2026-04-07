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

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	log "github.com/sirupsen/logrus"
)

// grpcContextKey is a context key for gRPC-specific values.
type grpcContextKey string

const (
	grpcKeyAgentID   grpcContextKey = "agent_id"
	grpcKeyOrgID     grpcContextKey = "org_id"
	grpcKeyAccountID grpcContextKey = "account_id"
)

// publicMethods are gRPC methods that don't require agent authentication.
var publicMethods = map[string]bool{
	"/fleet.v1.EnrollmentService/Enroll": true,
}

// agentAuthUnaryInterceptor validates agent identity for unary RPCs.
// Agents authenticate via mTLS client certificate or X-Agent-ID/X-Org-ID metadata.
func agentAuthUnaryInterceptor(apiKeys map[string]bool) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if publicMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		ctx, err := authenticateAgent(ctx, apiKeys)
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// agentAuthStreamInterceptor validates agent identity for streaming RPCs.
func agentAuthStreamInterceptor(apiKeys map[string]bool) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if publicMethods[info.FullMethod] {
			return handler(srv, ss)
		}

		ctx, err := authenticateAgent(ss.Context(), apiKeys)
		if err != nil {
			return err
		}
		return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
	}
}

// authenticateAgent extracts agent identity from mTLS cert or metadata headers.
func authenticateAgent(ctx context.Context, apiKeys map[string]bool) (context.Context, error) {
	var agentID, orgID, accountID string

	// Try mTLS first — extract identity from client certificate
	if p, ok := peer.FromContext(ctx); ok {
		if tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo); ok {
			if len(tlsInfo.State.PeerCertificates) > 0 {
				cert := tlsInfo.State.PeerCertificates[0]
				agentID = extractCertField(cert.Subject.CommonName, "agent:")
				if len(cert.Subject.OrganizationalUnit) > 0 {
					orgID = extractCertField(cert.Subject.OrganizationalUnit[0], "org:")
				}
				if len(cert.Subject.Organization) > 0 {
					accountID = extractCertField(cert.Subject.Organization[0], "account:")
				}
			}
		}
	}

	// Fall back to metadata headers
	if agentID == "" || orgID == "" {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if agentID == "" {
				if vals := md.Get("x-agent-id"); len(vals) > 0 {
					agentID = vals[0]
				}
			}
			if orgID == "" {
				if vals := md.Get("x-org-id"); len(vals) > 0 {
					orgID = vals[0]
				}
			}
			// Check API key as fallback auth
			if agentID == "" {
				if vals := md.Get("x-api-key"); len(vals) > 0 && apiKeys[vals[0]] {
					// API key auth — agent ID and org ID must be in metadata
					if vals := md.Get("x-agent-id"); len(vals) > 0 {
						agentID = vals[0]
					}
				}
			}
		}
	}

	if agentID == "" && orgID == "" {
		return ctx, status.Error(codes.Unauthenticated, "agent identity required: provide mTLS cert or x-agent-id/x-org-id metadata")
	}

	ctx = ctxutil.WithAgentContext(ctx, agentID, orgID, accountID)
	return ctx, nil
}

// loggingUnaryInterceptor logs gRPC unary calls.
func loggingUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		code := codes.OK
		if err != nil {
			code = status.Code(err)
		}
		log.WithFields(log.Fields{
			"method":   info.FullMethod,
			"code":     code.String(),
			"duration": time.Since(start).String(),
		}).Debug("grpc request")
		return resp, err
	}
}

// wrappedStream wraps a grpc.ServerStream with a custom context.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context {
	return w.ctx
}

// Note: extractCertField is defined in middleware.go and shared
// between HTTP and gRPC auth paths (same package).
