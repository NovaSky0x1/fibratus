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

import "context"

type contextKey string

const (
	ctxKeyUserID    contextKey = "user_id"
	ctxKeyAccountID contextKey = "account_id"
	ctxKeyRole      contextKey = "role"
	ctxKeyAgentID   contextKey = "agent_id"
	ctxKeyOrgID     contextKey = "org_id"
)

// WithUserContext stores user identity in the context (from JWT).
func WithUserContext(ctx context.Context, userID, accountID, role string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyUserID, userID)
	ctx = context.WithValue(ctx, ctxKeyAccountID, accountID)
	ctx = context.WithValue(ctx, ctxKeyRole, role)
	return ctx
}

// WithAgentContext stores agent identity in the context (from mTLS cert or API key).
func WithAgentContext(ctx context.Context, agentID, orgID, accountID string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyAgentID, agentID)
	ctx = context.WithValue(ctx, ctxKeyOrgID, orgID)
	ctx = context.WithValue(ctx, ctxKeyAccountID, accountID)
	return ctx
}

// UserIDFromContext extracts the user ID from the context.
func UserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyUserID).(string)
	return v
}

// AccountIDFromContext extracts the account ID from the context.
func AccountIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyAccountID).(string)
	return v
}

// RoleFromContext extracts the user role from the context.
func RoleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyRole).(string)
	return v
}

// AgentIDFromContext extracts the agent ID from the context.
func AgentIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyAgentID).(string)
	return v
}

// OrgIDFromContext extracts the org ID from the context.
func OrgIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyOrgID).(string)
	return v
}
