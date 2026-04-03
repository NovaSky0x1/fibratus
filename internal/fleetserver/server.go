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
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"database/sql"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ca"
	chstore "github.com/rabbitstack/fibratus/internal/fleetserver/store/clickhouse"
	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/handler"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store/postgres"
	log "github.com/sirupsen/logrus"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

// Server is the fleet management HTTP server.
type Server struct {
	config     *Config
	httpServer *http.Server
	pgStore    *postgres.Store
	apiKeys    map[string]bool
}

// New creates a new fleet server instance.
func New(cfg *Config) (*Server, error) {
	pgStore, err := postgres.New(cfg.Database.DSN(), cfg.Database.MaxConnections)
	if err != nil {
		return nil, fmt.Errorf("fleet server: %w", err)
	}

	apiKeys := make(map[string]bool)
	for _, k := range cfg.Auth.APIKeys {
		if k.Key != "" {
			apiKeys[k.Key] = true
		}
	}

	// Generate JWT secret if not set
	if cfg.Auth.JWTSecret == "" {
		cfg.Auth.JWTSecret = "fibratus-fleet-dev-secret-change-me"
		log.Warn("JWT secret not configured — using insecure default. Set auth.jwt-secret in config.")
	}

	return &Server{
		config:  cfg,
		pgStore: pgStore,
		apiKeys: apiKeys,
	}, nil
}

// Run starts the fleet server and blocks until the context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	log.Info("running database migrations...")
	if err := postgres.Migrate(s.config.Database.DSN()); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	db := s.pgStore.DB()

	// Create stores
	accountStore := postgres.NewAccountStore(db)
	orgStore := postgres.NewOrgStore(db)
	userStore := postgres.NewUserStore(db)
	agentStore := postgres.NewAgentStore(db)
	detStore := postgres.NewDetectionStore(db)
	ruleStore := postgres.NewRuleStore(db)
	commandStore := postgres.NewCommandStore(db)
	enrollStore := postgres.NewEnrollmentTokenStore(db)

	// Telemetry store: use ClickHouse if configured, otherwise PostgreSQL
	var telemetryStore store.TelemetryStore
	if s.config.ClickHouse.Enabled {
		chDB, err := sql.Open("clickhouse", s.config.ClickHouse.DSN())
		if err != nil {
			return fmt.Errorf("clickhouse connect: %w", err)
		}
		if err := chDB.Ping(); err != nil {
			return fmt.Errorf("clickhouse ping: %w", err)
		}
		chTelemetry := chstore.NewTelemetryStore(chDB)
		if err := chTelemetry.Migrate(ctx); err != nil {
			return fmt.Errorf("clickhouse migrate: %w", err)
		}
		telemetryStore = chTelemetry
		log.Info("fleet: using ClickHouse for telemetry storage")
	} else {
		telemetryStore = postgres.NewTelemetryStore(db)
		log.Info("fleet: using PostgreSQL for telemetry storage")
	}
	caManager := ca.NewManager(db)

	// Create stores for new features
	macroStore := postgres.NewMacroStore(db)
	auditStore := postgres.NewAuditStore(db)

	// Auto-seed macros from filesystem for each org that has no macros in DB
	seedMacrosFromFile(ctx, macroStore, orgStore, db)

	// Create handlers
	authHandler := handler.NewAuthHandler(accountStore, orgStore, userStore, s.config.Auth.JWTSecret)
	agentHandler := handler.NewAgentHandler(agentStore)
	commandHandler := handler.NewCommandHandler(commandStore, agentStore, auditStore, userStore)
	detHandler := handler.NewDetectionHandler(detStore, agentStore)
	ruleHandler := handler.NewRuleHandler(ruleStore, agentStore, macroStore, auditStore, userStore)
	enrollHandler := handler.NewEnrollHandler(enrollStore, agentStore, caManager)
	telemetryHandler := handler.NewTelemetryHandler(telemetryStore, agentStore)
	enrollTokenHandler := handler.NewEnrollmentTokenHandler(enrollStore)
	dashHandler := handler.NewDashboardHandler(agentStore, detStore)
	macroHandler := handler.NewMacroHandler(macroStore, auditStore, userStore)
	auditHandler := handler.NewAuditHandler(auditStore)

	// ═══════════════════════════════════════════════════════════
	// Route setup
	// ═══════════════════════════════════════════════════════════

	mux := http.NewServeMux()

	// ── Public routes (no auth) ──────────────────────────────

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Auth routes (signup/login — no auth required)
	mux.HandleFunc("/api/v1/auth/signup", methodGuard(http.MethodPost, authHandler.Signup))
	mux.HandleFunc("/api/v1/auth/login", methodGuard(http.MethodPost, authHandler.Login))

	// Enrollment route (token-based auth, no API key or JWT needed)
	mux.HandleFunc("/api/v1/enroll", methodGuard(http.MethodPost, enrollHandler.Enroll))

	// Agent registration (public — new agents don't have credentials yet)
	mux.HandleFunc("/api/v1/agents/register", methodGuard(http.MethodPost, agentHandler.Register))

	// ── Agent routes (API key auth for Phase 1 compat) ───────

	agentMux := http.NewServeMux()
	agentMux.HandleFunc("/api/v1/agents/register", methodGuard(http.MethodPost, agentHandler.Register))
	agentMux.HandleFunc("/api/v1/agents/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if r.Method == http.MethodPost && strings.HasSuffix(path, "/heartbeat") {
			agentHandler.Heartbeat(w, r)
			return
		}
		http.NotFound(w, r)
	})
	agentMux.HandleFunc("/api/v1/agent/rules", methodGuard(http.MethodGet, ruleHandler.GetForAgent))
	agentMux.HandleFunc("/api/v1/agent/commands", methodGuard(http.MethodGet, commandHandler.PollCommands))
	agentMux.HandleFunc("/api/v1/agent/commands/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/result") {
			commandHandler.ReportResult(w, r)
			return
		}
		http.NotFound(w, r)
	})
	agentMux.HandleFunc("/api/v1/detections", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			detHandler.Ingest(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})
	agentMux.HandleFunc("/api/v1/agent/telemetry", methodGuard(http.MethodPost, telemetryHandler.Ingest))

	// Wrap agent routes with API key auth + agent identity
	agentAuthenticated := agentAuth(s.apiKeys, agentIdentity(agentMux))

	// ── Dashboard routes (JWT auth, org-scoped) ──────────────

	dashMux := http.NewServeMux()

	// Org-scoped: /api/v1/orgs/{org_id}/...
	dashMux.HandleFunc("/api/v1/orgs/", func(w http.ResponseWriter, r *http.Request) {
		// Extract org_id from path: /api/v1/orgs/{org_id}/resource
		orgID, subpath := extractOrgPath(r.URL.Path)
		if orgID == "" {
			writeJSONError(w, http.StatusBadRequest, "org_id required in path")
			return
		}

		// Inject org_id into context
		ctx := ctxutil.WithOrgID(r.Context(), orgID)
		r = r.WithContext(ctx)

		// Route to sub-handlers
		switch {
		// Dashboard overview
		case subpath == "/dashboard/overview" && r.Method == http.MethodGet:
			dashHandler.Overview(w, r)

		// Agents
		case subpath == "/agents" && r.Method == http.MethodGet:
			agentHandler.List(w, r)
		case strings.HasPrefix(subpath, "/agents/") && strings.HasSuffix(subpath, "/events") && r.Method == http.MethodGet:
			telemetryHandler.GetLiveEvents(w, r)
		case strings.HasPrefix(subpath, "/agents/") && strings.HasSuffix(subpath, "/commands") && r.Method == http.MethodGet:
			commandHandler.ListCommands(w, r)
		case strings.HasPrefix(subpath, "/agents/") && strings.HasSuffix(subpath, "/commands") && r.Method == http.MethodPost:
			commandHandler.CreateCommand(w, r)
		case strings.HasPrefix(subpath, "/agents/") && r.Method == http.MethodGet:
			agentHandler.Get(w, r)
		case strings.HasPrefix(subpath, "/agents/") && r.Method == http.MethodDelete:
			agentHandler.Delete(w, r)

		// Rules
		case subpath == "/rules" && r.Method == http.MethodGet:
			ruleHandler.List(w, r)
		case subpath == "/rules" && r.Method == http.MethodPost:
			ruleHandler.Create(w, r)
		case strings.HasPrefix(subpath, "/rules/") && r.Method == http.MethodGet:
			ruleHandler.Get(w, r)
		case strings.HasPrefix(subpath, "/rules/") && r.Method == http.MethodPut:
			ruleHandler.Update(w, r)
		case strings.HasPrefix(subpath, "/rules/") && r.Method == http.MethodDelete:
			ruleHandler.Delete(w, r)

		// Enrollment Tokens
		case subpath == "/enrollment-tokens" && r.Method == http.MethodGet:
			enrollTokenHandler.List(w, r)
		case subpath == "/enrollment-tokens" && r.Method == http.MethodPost:
			enrollTokenHandler.Create(w, r)
		case strings.HasPrefix(subpath, "/enrollment-tokens/") && r.Method == http.MethodDelete:
			enrollTokenHandler.Delete(w, r)

		// Detections
		case subpath == "/detections" && r.Method == http.MethodGet:
			detHandler.List(w, r)
		case subpath == "/detections/timeline" && r.Method == http.MethodGet:
			detHandler.Timeline(w, r)
		case subpath == "/detections/mitre" && r.Method == http.MethodGet:
			detHandler.MitreHeatmap(w, r)
		case strings.HasPrefix(subpath, "/detections/") && r.Method == http.MethodGet:
			detHandler.Get(w, r)

		// Telemetry
		case subpath == "/telemetry" && r.Method == http.MethodGet:
			telemetryHandler.Search(w, r)

		// Macros
		case subpath == "/macros" && r.Method == http.MethodGet:
			macroHandler.List(w, r)
		case subpath == "/macros" && r.Method == http.MethodPost:
			macroHandler.Create(w, r)
		case strings.HasPrefix(subpath, "/macros/") && r.Method == http.MethodPut:
			macroHandler.Update(w, r)
		case strings.HasPrefix(subpath, "/macros/") && r.Method == http.MethodDelete:
			macroHandler.Delete(w, r)

		// Audit Log
		case subpath == "/audit-log" && r.Method == http.MethodGet:
			auditHandler.List(w, r)

		default:
			http.NotFound(w, r)
		}
	})

	// Account-level routes
	dashMux.HandleFunc("/api/v1/account/organizations", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			authHandler.ListOrganizations(w, r)
		case http.MethodPost:
			authHandler.CreateOrganization(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Wrap dashboard routes with JWT auth
	dashAuthenticated := jwtAuth(s.config.Auth.JWTSecret, dashMux)

	// ── Combine all route groups ─────────────────────────────

	rootMux := http.NewServeMux()

	// Public
	rootMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	rootMux.HandleFunc("/api/v1/auth/", mux.ServeHTTP)

	// Route dispatcher — determines auth path based on URL prefix
	rootMux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Public routes (no auth required)
		if strings.HasPrefix(path, "/api/v1/auth/") || path == "/api/v1/enroll" || path == "/api/v1/agents/register" {
			mux.ServeHTTP(w, r)
			return
		}

		// Agent routes (API key auth)
		if strings.HasPrefix(path, "/api/v1/agents/") || strings.HasPrefix(path, "/api/v1/agent/") || (path == "/api/v1/detections" && r.Method == http.MethodPost) {
			agentAuthenticated.ServeHTTP(w, r)
			return
		}

		// Dashboard routes (JWT auth)
		dashAuthenticated.ServeHTTP(w, r)
	})

	// Dashboard SPA
	if s.config.Dashboard.Enabled {
		rootMux.Handle("/", dashboardHandler())
	}

	// Apply global middleware
	var h http.Handler = rootMux
	h = cors(h)
	h = requestLogger(h)

	s.httpServer = &http.Server{
		Addr:         s.config.Server.Listen,
		Handler:      h,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start agent status reaper
	go s.agentReaper(ctx, agentStore)

	// Start telemetry retention purge (every hour, keep 7 days)
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				deleted, err := telemetryStore.Purge(context.Background(), 7)
				if err != nil {
					log.Warnf("fleet: telemetry purge error: %v", err)
				} else if deleted > 0 {
					log.Infof("fleet: purged %d old telemetry events", deleted)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Configure TLS. We use RequestClientCert so browsers can connect
	// without a client cert (dashboard), while enrolled agents can
	// optionally present their cert for mTLS authentication. The
	// agentIdentity middleware extracts identity from presented certs.
	if s.config.Server.TLSCert != "" && s.config.Server.TLSKey != "" {
		tlsConfig := &tls.Config{
			// Request but don't require client certs — browsers won't
			// have one, enrolled agents will present theirs.
			ClientAuth: tls.RequestClientCert,
		}

		// Load org CA certs so we can verify agent certs in middleware
		clientCAs, err := caManager.LoadAllCACerts(context.Background())
		if err != nil {
			log.Warnf("fleet: failed to load org CAs: %v", err)
		}
		if clientCAs != nil {
			tlsConfig.ClientCAs = clientCAs
			log.Info("fleet: agent client certificate verification enabled")
		}

		s.httpServer.TLSConfig = tlsConfig
	}

	// Start HTTP server
	errCh := make(chan error, 1)
	go func() {
		log.Infof("fleet server listening on %s", s.config.Server.Listen)
		if s.config.Server.TLSCert != "" && s.config.Server.TLSKey != "" {
			errCh <- s.httpServer.ListenAndServeTLS(s.config.Server.TLSCert, s.config.Server.TLSKey)
		} else {
			log.Warn("TLS is not configured. Running without encryption.")
			errCh <- s.httpServer.ListenAndServe()
		}
	}()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down fleet server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.httpServer.Shutdown(shutdownCtx)
	}

	s.pgStore.Close()
	log.Info("fleet server stopped")
	return nil
}

// agentReaper periodically marks agents as offline if they miss heartbeats.
func (s *Server) agentReaper(ctx context.Context, agents *postgres.AgentStore) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n, err := agents.MarkOfflineAgents(ctx, s.config.Agent.HeartbeatTimeout)
			if err != nil {
				log.Warnf("fleet: reaper error: %v", err)
				continue
			}
			if n > 0 {
				log.Infof("fleet: marked %d agent(s) as offline", n)
			}
		case <-ctx.Done():
			return
		}
	}
}

// extractOrgPath splits /api/v1/orgs/{org_id}/... into org_id and the rest.
func extractOrgPath(path string) (orgID, subpath string) {
	const prefix = "/api/v1/orgs/"
	if !strings.HasPrefix(path, prefix) {
		return "", ""
	}
	rest := path[len(prefix):]
	idx := strings.Index(rest, "/")
	if idx < 0 {
		return rest, ""
	}
	return rest[:idx], rest[idx:]
}

// methodGuard restricts a handler to a specific HTTP method.
func methodGuard(method string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h(w, r)
	}
}

// seedMacrosFromFile imports macros from the rules/macros/ filesystem into the
// database for each org that has no macros yet. This ensures a smooth transition
// from file-based macros to DB-managed macros.
func seedMacrosFromFile(ctx context.Context, macroStore store.MacroStore, orgStore store.OrgStore, db *sql.DB) {
	// Check if there are any macros in the DB already
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM macros").Scan(&count); err != nil {
		return
	}
	if count > 0 {
		return // macros already seeded
	}

	// Try to load macros from well-known filesystem locations
	var macrosData []byte
	for _, path := range []string{
		"rules/macros/macros.yml",
		"/opt/fibratus-fleet/src/rules/macros/macros.yml",
	} {
		data, err := os.ReadFile(path)
		if err == nil {
			macrosData = data
			log.Infof("fleet: seeding macros from %s (%d bytes)", path, len(data))
			break
		}
	}
	if macrosData == nil {
		return
	}

	// Get all orgs and seed macros for each
	rows, err := db.QueryContext(ctx, "SELECT id FROM organizations")
	if err != nil {
		return
	}
	defer rows.Close()

	var orgIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			orgIDs = append(orgIDs, id)
		}
	}

	pStore, ok := macroStore.(*postgres.MacroStore)
	if !ok {
		return
	}
	for _, orgID := range orgIDs {
		n, err := pStore.ImportFromYAML(ctx, orgID, macrosData)
		if err != nil {
			log.Warnf("fleet: failed to seed macros for org %s: %v", orgID, err)
			continue
		}
		if n > 0 {
			log.Infof("fleet: seeded %d macros for org %s", n, orgID)
		}
	}
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":{"code":%d,"message":"%s"}}`, status, msg)
}
