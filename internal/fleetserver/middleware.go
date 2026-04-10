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
	"net/http"
	"strings"
	"time"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/fleetauth"
	log "github.com/sirupsen/logrus"
)

// groupPermissionStore is used by requirePermission to check group-based permissions.
var groupPermissionStore interface {
	GetEffectivePermissions(ctx context.Context, userID string) ([]string, error)
}

// SetGroupStore sets the user group store used for group-based permission checks.
func SetGroupStore(store interface {
	GetEffectivePermissions(ctx context.Context, userID string) ([]string, error)
}) {
	groupPermissionStore = store
}

// jwtAuth validates a JWT Bearer token and injects user context.
func jwtAuth(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, `{"error":{"code":401,"message":"missing or invalid authorization header"}}`, http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(auth, "Bearer ")

		claims, err := fleetauth.ValidateJWT(secret, tokenStr)
		if err != nil {
			http.Error(w, `{"error":{"code":401,"message":"invalid or expired token"}}`, http.StatusUnauthorized)
			return
		}

		ctx := ctxutil.WithUserContext(r.Context(), claims.Sub, claims.AccountID, claims.Role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// agentAuth validates agent requests via API key OR client certificate.
// Enrolled agents present a client cert (mTLS) and don't need an API key.
// Legacy agents use X-API-Key header. At least one must be valid.
func agentAuth(validKeys map[string]bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for valid API key
		key := r.Header.Get("X-API-Key")
		if key != "" && validKeys[key] {
			next.ServeHTTP(w, r)
			return
		}

		// Check for client certificate (enrolled agent via mTLS)
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			next.ServeHTTP(w, r)
			return
		}

		// Check for X-Agent-ID + X-Org-ID headers (enrolled agent without mTLS)
		agentID := r.Header.Get("X-Agent-ID")
		orgID := r.Header.Get("X-Org-ID")
		if agentID != "" && orgID != "" {
			next.ServeHTTP(w, r)
			return
		}

		http.Error(w, `{"error":{"code":401,"message":"unauthorized"}}`, http.StatusUnauthorized)
	})
}

// agentIdentity extracts agent identity from mTLS client certificate
// or falls back to X-Agent-ID / X-Org-ID headers for legacy agents.
func agentIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var agentID, orgID, accountID string

		// Try mTLS first — extract identity from client certificate Subject
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			cert := r.TLS.PeerCertificates[0]
			agentID = extractCertField(cert.Subject.CommonName, "agent:")
			if len(cert.Subject.OrganizationalUnit) > 0 {
				orgID = extractCertField(cert.Subject.OrganizationalUnit[0], "org:")
			}
			if len(cert.Subject.Organization) > 0 {
				accountID = extractCertField(cert.Subject.Organization[0], "account:")
			}
		}

		// Fall back to headers for legacy API-key agents
		if agentID == "" {
			agentID = r.Header.Get("X-Agent-ID")
		}
		if orgID == "" {
			orgID = r.Header.Get("X-Org-ID")
		}

		if agentID != "" || orgID != "" {
			ctx := ctxutil.WithAgentContext(r.Context(), agentID, orgID, accountID)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

// extractCertField extracts the value after a prefix from a cert field.
// e.g., extractCertField("agent:abc123", "agent:") returns "abc123"
func extractCertField(field, prefix string) string {
	if len(field) > len(prefix) && field[:len(prefix)] == prefix {
		return field[len(prefix):]
	}
	return ""
}

// requirePermission returns a middleware that checks if the authenticated user
// has the required permission based on their role or group memberships.
func requirePermission(perm fleetauth.Permission, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := ctxutil.RoleFromContext(r.Context())

		// Root always passes
		if fleetauth.IsRoot(role) {
			next(w, r)
			return
		}

		// Check role-based permissions (fast path)
		if fleetauth.HasPermission(role, perm) {
			next(w, r)
			return
		}

		// Check group-based permissions
		if groupPermissionStore != nil {
			userID := ctxutil.UserIDFromContext(r.Context())
			if userID != "" {
				perms, err := groupPermissionStore.GetEffectivePermissions(r.Context(), userID)
				if err == nil {
					for _, p := range perms {
						if p == string(perm) {
							next(w, r)
							return
						}
					}
				}
			}
		}

		writeJSONError(w, http.StatusForbidden,
			"insufficient permissions: requires "+string(perm))
	}
}

// cors adds CORS headers for dashboard development.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Agent-ID, If-None-Match")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requestLogger logs incoming HTTP requests.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		log.WithFields(log.Fields{
			"method":   r.Method,
			"path":     r.URL.Path,
			"status":   rw.status,
			"duration": time.Since(start).String(),
			"remote":   r.RemoteAddr,
		}).Debug("request")
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
