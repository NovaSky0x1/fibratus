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

// Package ctxutil provides request context helpers for the fleet server.
// It is a separate package to avoid import cycles between the server
// core, middleware, and handler packages.
package ctxutil

import "context"

type contextKey string

const (
	keyUserID    contextKey = "user_id"
	keyAccountID contextKey = "account_id"
	keyRole      contextKey = "role"
	keyAgentID   contextKey = "agent_id"
	keyOrgID     contextKey = "org_id"
)

// WithUserContext stores user identity in the context (from JWT).
func WithUserContext(ctx context.Context, userID, accountID, role string) context.Context {
	ctx = context.WithValue(ctx, keyUserID, userID)
	ctx = context.WithValue(ctx, keyAccountID, accountID)
	ctx = context.WithValue(ctx, keyRole, role)
	return ctx
}

// WithAgentContext stores agent identity in the context (from mTLS cert or API key).
func WithAgentContext(ctx context.Context, agentID, orgID, accountID string) context.Context {
	ctx = context.WithValue(ctx, keyAgentID, agentID)
	ctx = context.WithValue(ctx, keyOrgID, orgID)
	ctx = context.WithValue(ctx, keyAccountID, accountID)
	return ctx
}

// WithOrgID stores just the org ID in the context (from URL path).
func WithOrgID(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, keyOrgID, orgID)
}

// UserIDFromContext extracts the user ID.
func UserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(keyUserID).(string)
	return v
}

// AccountIDFromContext extracts the account ID.
func AccountIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(keyAccountID).(string)
	return v
}

// RoleFromContext extracts the user role.
func RoleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(keyRole).(string)
	return v
}

// AgentIDFromContext extracts the agent ID.
func AgentIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(keyAgentID).(string)
	return v
}

// OrgIDFromContext extracts the org ID.
func OrgIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(keyOrgID).(string)
	return v
}
