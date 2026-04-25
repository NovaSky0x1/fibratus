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
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"database/sql"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ca"
	"github.com/rabbitstack/fibratus/internal/fleetserver/fleetauth"
	natsPkg "github.com/rabbitstack/fibratus/internal/fleetserver/nats"
	pb "github.com/rabbitstack/fibratus/pkg/fleet/pb"
	chstore "github.com/rabbitstack/fibratus/internal/fleetserver/store/clickhouse"
	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/handler"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store/postgres"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

// Server is the fleet management server (HTTP for dashboard, gRPC for agents).
type Server struct {
	config     *Config
	configPath string
	httpServer *http.Server
	grpcServer *GRPCServer
	pgStore    *postgres.Store
	apiKeys    map[string]bool
}

// New creates a new fleet server instance. configPath is the absolute path to
// the YAML config file, retained so admin endpoints can persist config changes
// (e.g. switching ClickHouse to a Cloud-hosted instance).
func New(cfg *Config, configPath string) (*Server, error) {
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
		config:     cfg,
		configPath: configPath,
		pgStore:    pgStore,
		apiKeys:    apiKeys,
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
	yaraRuleStore := postgres.NewYARARuleStore(db)
	commandStore := postgres.NewCommandStore(db)
	enrollStore := postgres.NewEnrollmentTokenStore(db)

	// Telemetry store: use ClickHouse if configured, otherwise PostgreSQL
	var telemetryStore store.TelemetryStore
	var chDB *sql.DB
	if s.config.ClickHouse.Enabled {
		var err error
		chDB, err = sql.Open("clickhouse", s.config.ClickHouse.DSN())
		if err != nil {
			return fmt.Errorf("clickhouse connect: %w", err)
		}
		chDB.SetMaxOpenConns(s.config.ClickHouse.MaxOpenConns)
		chDB.SetMaxIdleConns(s.config.ClickHouse.MaxIdleConns)
		chDB.SetConnMaxLifetime(time.Duration(s.config.ClickHouse.ConnMaxLifetime) * time.Second)
		if err := chDB.Ping(); err != nil {
			return fmt.Errorf("clickhouse ping: %w", err)
		}
		chTelemetry := chstore.NewTelemetryStore(chDB)
		if err := chTelemetry.Migrate(ctx); err != nil {
			return fmt.Errorf("clickhouse migrate: %w", err)
		}
		// Wire org name resolver so ClickHouse tables get human-readable names
		chTelemetry.SetOrgNameResolver(func(orgID string) string {
			org, err := orgStore.Get(ctx, orgID)
			if err != nil || org == nil {
				return ""
			}
			return org.Name
		})
		// Wrap with buffer for high-throughput batch inserts.
		buffered := store.NewBufferedTelemetryStore(chTelemetry, store.BufferConfig{
			FlushInterval: 2 * time.Second,
			FlushSize:     50000,
		})
		buffered.Start()
		defer buffered.Stop()
		telemetryStore = buffered
		log.Infof("fleet: using ClickHouse for telemetry (pool: %d open, %d idle, buffered)",
			s.config.ClickHouse.MaxOpenConns, s.config.ClickHouse.MaxIdleConns)
	} else {
		telemetryStore = postgres.NewTelemetryStore(db)
		log.Info("fleet: using PostgreSQL for telemetry storage")
	}
	caManager := ca.NewManager(db)

	// Create stores for new features
	macroStore := postgres.NewMacroStore(db)
	auditStore := postgres.NewAuditStore(db)
	captureStore := postgres.NewCaptureStore(db)

	// Auto-seed macros and official rules for any org missing them
	seedMacrosFromFile(ctx, macroStore, orgStore, db)
	seedRulesFromFiles(ctx, ruleStore, db)

	// Create handlers
	authHandler := handler.NewAuthHandler(accountStore, orgStore, userStore, agentStore, commandStore, s.config.Auth.JWTSecret)
	totpHandler := handler.NewTOTPHandler(userStore)
	agentHandler := handler.NewAgentHandler(agentStore, accountStore, orgStore)
	commandHandler := handler.NewCommandHandler(commandStore, agentStore, auditStore, userStore)
	commandHandler.SetAccountStore(accountStore)
	commandHandler.SetYARARuleStore(yaraRuleStore)
	detHandler := handler.NewDetectionHandler(detStore, agentStore, telemetryStore)
	ruleHandler := handler.NewRuleHandler(ruleStore, agentStore, macroStore, auditStore, userStore)
	yaraRuleHandler := handler.NewYARARuleHandler(yaraRuleStore)
	enrollHandler := handler.NewEnrollHandler(enrollStore, agentStore, caManager)
	telemetryHandler := handler.NewTelemetryHandler(telemetryStore, agentStore)
	telemetryHandler.SetOrgStore(orgStore)
	enrollTokenHandler := handler.NewEnrollmentTokenHandler(enrollStore)
	dashHandler := handler.NewDashboardHandler(agentStore, detStore)
	macroHandler := handler.NewMacroHandler(macroStore, auditStore, userStore)
	macroHandler.SetOrgStore(orgStore)
	auditHandler := handler.NewAuditHandler(auditStore)
	groupStore := postgres.NewUserGroupStore(db)
	SetGroupStore(groupStore)
	userHandler := handler.NewUserHandler(userStore, groupStore)
	installHandler := handler.NewInstallHandler(enrollStore,
		s.config.Server.ExternalURL, s.config.Deployment.AgentBinaryPath, s.config.Deployment.InstallDir)
	installHandler.SetAccountStore(accountStore)
	installHandler.SetOrgStore(orgStore)
	adminHandler := handler.NewAdminHandler(accountStore, orgStore, userStore)
	adminHandler.SetGroupStore(groupStore)
	adminHandler.SetAuthHandler(authHandler)
	dbAdminHandler := handler.NewDBAdminHandler(db, chDB)
	chConfigHandler := handler.NewCHConfigHandler(
		s.getCHConfigDTO,
		s.saveCHConfig,
		s.testCHConfig,
	)
	groupHandler := handler.NewGroupHandler(groupStore)
	captureHandler := handler.NewCaptureHandler(captureStore, agentStore, commandStore, auditStore, userStore)
	eventLogPolicyStore := postgres.NewEventLogPolicyStore(db)
	eventLogPolicyHandler := handler.NewEventLogPolicyHandler(eventLogPolicyStore, agentStore, commandStore)
	authHandler.SetEventLogPolicyStore(eventLogPolicyStore)
	apiKeyStoreInst := postgres.NewAPIKeyStore(db)
	authHandler.SetAPIKeyStore(apiKeyStoreInst)
	SetAPIKeyStore(apiKeyStoreInst)
	authHandler.SetGroupStore(groupStore)
	authHandler.SetMacroStore(macroStore)
	authHandler.SetRuleStore(ruleStore)
	handler.SetGitHubSyncDB(db)
	githubSyncHandler := handler.NewGitHubSyncHandler(ruleStore, macroStore, auditStore, userStore)
	githubSyncHandler.SetYARARuleStore(yaraRuleStore)
	githubSyncHandler.StartPeriodicSync(ctx)
	releaseChecker := handler.NewReleaseChecker(accountStore, 15*time.Minute)
	releaseChecker.Start(ctx)
	_ = releaseChecker

	// Seed baseline YARA rules into accounts that have none. One-shot at
	// startup; idempotent (skips accounts that already have rules).
	go handler.SeedBaselineYARARules(ctx, accountStore, yaraRuleStore)
	sigmaHandler := handler.NewSigmaHandler(ruleStore, macroStore, auditStore, userStore)
	sigmahqHandler := handler.NewSigmaHQHandler(ruleStore, macroStore, accountStore, orgStore, auditStore, userStore, "/opt/fibratus-fleet/sigmahq")
	sigmahqHandler.StartBackgroundUpdater(ctx)

	// Wire org store for cross-org aggregation (account-scoped views)
	detHandler.SetOrgStore(orgStore)
	ruleHandler.SetOrgStore(orgStore)
	ruleHandler.SetDetectionStore(detStore)
	dashHandler.SetOrgStore(orgStore)
	auditHandler.SetOrgStore(orgStore)
	enrollTokenHandler.SetOrgStore(orgStore)

	// ═══════════════════════════════════════════════════════════
	// gRPC server for agent communication (protobuf/gRPC transport)
	// ═══════════════════════════════════════════════════════════
	clientCAs, caErr := caManager.LoadAllCACerts(context.Background())
	if caErr != nil {
		log.Warnf("fleet: failed to load org CAs for gRPC: %v", caErr)
	}

	grpcCfg := s.config.GRPC
	if grpcCfg.TLSCert == "" {
		grpcCfg.TLSCert = s.config.Server.TLSCert
	}
	if grpcCfg.TLSKey == "" {
		grpcCfg.TLSKey = s.config.Server.TLSKey
	}

	grpcSrv, err := NewGRPCServer(
		grpcCfg,
		s.apiKeys,
		agentStore, ruleStore, macroStore, commandStore, detStore,
		telemetryStore, captureStore, enrollStore, caManager,
		s.config.NATS,
		clientCAs,
	)
	if err != nil {
		return fmt.Errorf("grpc server setup: %w", err)
	}
	s.grpcServer = grpcSrv

	// Wire auto-update heartbeat callback
	grpcSrv.SetHeartbeatCallback(func(ctx context.Context, orgID, agentID string) {
		agentHandler.CheckAutoUpdate(ctx, orgID, agentID)
	})

	// Start NATS consumer if enabled
	if s.config.NATS.Enabled {
		natsConsumer, err := natsPkg.NewConsumer(natsPkg.ConsumerConfig{
			URL:     s.config.NATS.URL,
			Subject: s.config.NATS.Subject,
			Queue:   s.config.NATS.Queue,
		}, telemetryStore)
		if err != nil {
			log.Warnf("fleet: nats consumer setup failed: %v (telemetry will use direct writes)", err)
		} else {
			if err := natsConsumer.Start(ctx, s.config.NATS.Subject, s.config.NATS.Queue); err != nil {
				log.Warnf("fleet: nats consumer start failed: %v", err)
			} else {
				defer natsConsumer.Stop()
				log.Info("fleet: nats telemetry pipeline active")
			}
		}
	}

	// Wire stream callbacks: when dashboard changes rules or creates commands,
	// push them instantly to connected agents via gRPC streams.
	streams := grpcSrv.Streams()

	ruleHandler.SetRuleChangeCallback(func(orgID string) {
		rules, etag, err := ruleStore.GetForAgent(context.Background(), orgID, "")
		if err != nil {
			log.Warnf("fleet: rule push build failed: %v", err)
			return
		}
		update := buildRuleUpdate(rules, etag, orgID, macroStore, context.Background())
		streams.PushRulesToOrg(orgID, update)
	})

	cmdPushCallback := func(agentID, cmdID, cmdType string, payload []byte) bool {
		return streams.PushCommand(agentID, &pb.CommandPush{
			Id:      cmdID,
			Type:    cmdType,
			Payload: payload,
		})
	}
	commandHandler.SetCommandPushCallback(cmdPushCallback)
	captureHandler.SetCommandPushCallback(cmdPushCallback)
	authHandler.SetCommandPushCallback(cmdPushCallback)
	if chDB != nil {
		authHandler.SetRetentionCallback(func(orgID string, days int) error {
			table := "telemetry_" + orgID
			_, err := chDB.Exec(fmt.Sprintf("ALTER TABLE %s MODIFY TTL toDateTime(timestamp) + INTERVAL %d DAY DELETE", table, days))
			return err
		})
	}
	eventLogPolicyHandler.SetCommandPushCallback(cmdPushCallback)
	agentHandler.SetCommandDeps(commandStore, eventLogPolicyStore, cmdPushCallback)

	// Start gRPC server in background
	go func() {
		if err := grpcSrv.Serve(); err != nil {
			log.Errorf("grpc server error: %v", err)
		}
	}()
	defer grpcSrv.Stop()

	// ═══════════════════════════════════════════════════════════
	// Route setup
	// ═══════════════════════════════════════════════════════════

	// Rate limiters
	loginLimiter := newRateLimiter(10, time.Minute)
	signupLimiter := newRateLimiter(3, 5*time.Minute)

	mux := http.NewServeMux()

	// ── Public routes (no auth) ──────────────────────────────

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Auth routes (signup/login — no auth required, rate limited)
	mux.HandleFunc("/api/v1/auth/signup", methodGuard(http.MethodPost, rateLimit(signupLimiter, authHandler.Signup)))
	mux.HandleFunc("/api/v1/auth/login", methodGuard(http.MethodPost, rateLimit(loginLimiter, authHandler.Login)))

	// Enrollment route (token-based auth, no API key or JWT needed)
	mux.HandleFunc("/api/v1/enroll", methodGuard(http.MethodPost, enrollHandler.Enroll))

	// Agent deployment (public — token is the auth)
	mux.HandleFunc("/install/", installHandler.Script)
	mux.HandleFunc("/api/v1/agent/binary", installHandler.Binary)
	mux.HandleFunc("/api/v1/agent/msi", installHandler.MSI)
	mux.HandleFunc("/api/v1/agent/config", installHandler.Config)

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
		case strings.HasPrefix(subpath, "/agents/") && strings.HasSuffix(subpath, "/tamper-protection") && r.Method == http.MethodPut:
			agentHandler.SetTamperProtection(w, r)
		case strings.HasPrefix(subpath, "/agents/") && strings.HasSuffix(subpath, "/eventlog-collection") && r.Method == http.MethodPut:
			agentHandler.SetEventLogCollection(w, r)
		case strings.HasPrefix(subpath, "/agents/") && strings.HasSuffix(subpath, "/heartbeat-history") && r.Method == http.MethodGet:
			agentHandler.HeartbeatHistory(w, r)
		case strings.HasPrefix(subpath, "/agents/") && strings.HasSuffix(subpath, "/events") && r.Method == http.MethodGet:
			telemetryHandler.GetLiveEvents(w, r)
		case strings.HasPrefix(subpath, "/commands/") && r.Method == http.MethodGet:
			commandHandler.GetCommand(w, r)
		case strings.HasPrefix(subpath, "/agents/") && strings.HasSuffix(subpath, "/commands") && r.Method == http.MethodGet:
			commandHandler.ListCommands(w, r)
		case strings.HasPrefix(subpath, "/agents/") && strings.HasSuffix(subpath, "/commands") && r.Method == http.MethodPost:
			requirePermission(fleetauth.PermExecuteCommands, commandHandler.CreateCommand)(w, r)
		case strings.HasPrefix(subpath, "/agents/") && strings.HasSuffix(subpath, "/captures") && r.Method == http.MethodGet:
			captureHandler.ListCaptures(w, r)
		case strings.HasPrefix(subpath, "/agents/") && strings.HasSuffix(subpath, "/captures") && r.Method == http.MethodPost:
			requirePermission(fleetauth.PermExecuteCommands, captureHandler.CreateCapture)(w, r)
		case subpath == "/agents/update-all" && r.Method == http.MethodPost:
			requirePermission(fleetauth.PermManageAgents, agentHandler.UpdateAllAgents)(w, r)
		case strings.HasPrefix(subpath, "/agents/") && strings.HasSuffix(subpath, "/update") && r.Method == http.MethodPost:
			requirePermission(fleetauth.PermManageAgents, agentHandler.UpdateAgent)(w, r)
		case strings.HasPrefix(subpath, "/agents/") && r.Method == http.MethodGet:
			agentHandler.Get(w, r)
		case strings.HasPrefix(subpath, "/agents/") && r.Method == http.MethodDelete:
			requirePermission(fleetauth.PermDeleteAgents, agentHandler.Delete)(w, r)

		// Rules
		case subpath == "/rules" && r.Method == http.MethodGet:
			ruleHandler.List(w, r)
		case subpath == "/rules" && r.Method == http.MethodPost:
			requirePermission(fleetauth.PermManageRules, ruleHandler.Create)(w, r)
		case subpath == "/rules/noisy" && r.Method == http.MethodGet:
			ruleHandler.NoisyRules(w, r)
		case subpath == "/rules/validate-condition" && r.Method == http.MethodPost:
			ruleHandler.ValidateCondition(w, r)
		case subpath == "/rules/validate-all" && r.Method == http.MethodPost:
			requirePermission(fleetauth.PermManageRules, ruleHandler.ValidateAll)(w, r)
		case strings.HasSuffix(subpath, "/validate") && strings.HasPrefix(subpath, "/rules/") && r.Method == http.MethodPost:
			requirePermission(fleetauth.PermManageRules, ruleHandler.Validate)(w, r)
		case strings.HasPrefix(subpath, "/rules/") && r.Method == http.MethodGet:
			ruleHandler.Get(w, r)
		case strings.HasPrefix(subpath, "/rules/") && r.Method == http.MethodPut:
			requirePermission(fleetauth.PermManageRules, ruleHandler.Update)(w, r)
		case strings.HasPrefix(subpath, "/rules/") && r.Method == http.MethodDelete:
			requirePermission(fleetauth.PermManageRules, ruleHandler.Delete)(w, r)

		// Event Log Policy
		case subpath == "/eventlog-policy" && r.Method == http.MethodGet:
			eventLogPolicyHandler.Get(w, r)
		case subpath == "/eventlog-policy" && r.Method == http.MethodPut:
			requirePermission(fleetauth.PermManageSettings, eventLogPolicyHandler.Upsert)(w, r)

		// Enrollment Tokens
		case subpath == "/enrollment-tokens" && r.Method == http.MethodGet:
			requirePermission(fleetauth.PermManageSettings, enrollTokenHandler.List)(w, r)
		case subpath == "/enrollment-tokens" && r.Method == http.MethodPost:
			requirePermission(fleetauth.PermManageSettings, enrollTokenHandler.Create)(w, r)
		case strings.HasPrefix(subpath, "/enrollment-tokens/") && r.Method == http.MethodDelete:
			requirePermission(fleetauth.PermManageSettings, enrollTokenHandler.Delete)(w, r)

		// Detections
		case subpath == "/detections" && r.Method == http.MethodGet:
			detHandler.List(w, r)
		case subpath == "/detections/timeline" && r.Method == http.MethodGet:
			detHandler.Timeline(w, r)
		case subpath == "/detections/mitre" && r.Method == http.MethodGet:
			detHandler.MitreHeatmap(w, r)
		case strings.HasPrefix(subpath, "/detections/") && strings.HasSuffix(subpath, "/process-tree") && r.Method == http.MethodGet:
			detHandler.ProcessTree(w, r)
		case strings.HasPrefix(subpath, "/detections/") && strings.HasSuffix(subpath, "/process-context") && r.Method == http.MethodGet:
			detHandler.ProcessContext(w, r)
		case strings.HasPrefix(subpath, "/detections/") && r.Method == http.MethodGet:
			detHandler.Get(w, r)

		// Telemetry
		case subpath == "/telemetry" && r.Method == http.MethodGet:
			telemetryHandler.Search(w, r)
		case subpath == "/telemetry/fields" && r.Method == http.MethodGet:
			telemetryHandler.GetFieldValues(w, r)
		case subpath == "/telemetry/process-tree" && r.Method == http.MethodGet:
			telemetryHandler.ProcessTree(w, r)

		// Captures (capture-scoped: /captures/{id}/...)
		case strings.HasPrefix(subpath, "/captures/") && strings.HasSuffix(subpath, "/events") && r.Method == http.MethodGet:
			captureHandler.GetCaptureEvents(w, r)
		case strings.HasPrefix(subpath, "/captures/") && strings.HasSuffix(subpath, "/stop") && r.Method == http.MethodPost:
			requirePermission(fleetauth.PermExecuteCommands, captureHandler.StopCapture)(w, r)
		case strings.HasPrefix(subpath, "/captures/") && r.Method == http.MethodGet:
			captureHandler.GetCapture(w, r)
		case strings.HasPrefix(subpath, "/captures/") && r.Method == http.MethodDelete:
			requirePermission(fleetauth.PermExecuteCommands, captureHandler.DeleteCapture)(w, r)

		// Macros
		case subpath == "/macros" && r.Method == http.MethodGet:
			macroHandler.List(w, r)
		case subpath == "/macros/upload" && r.Method == http.MethodPost:
			macroHandler.Upload(w, r)
		case subpath == "/macros" && r.Method == http.MethodPost:
			macroHandler.Create(w, r)
		case strings.HasPrefix(subpath, "/macros/") && r.Method == http.MethodPut:
			macroHandler.Update(w, r)
		case strings.HasPrefix(subpath, "/macros/") && r.Method == http.MethodDelete:
			macroHandler.Delete(w, r)

		// GitHub Sync (multi-repo)
		case subpath == "/github-sync" && r.Method == http.MethodGet:
			githubSyncHandler.ListConfigs(w, r)
		case subpath == "/github-sync" && r.Method == http.MethodPost:
			githubSyncHandler.SaveConfig(w, r)
		case subpath == "/github-sync/trigger" && r.Method == http.MethodPost:
			githubSyncHandler.TriggerSync(w, r)
		case strings.HasSuffix(subpath, "/trigger") && strings.HasPrefix(subpath, "/github-sync/") && r.Method == http.MethodPost:
			githubSyncHandler.TriggerSyncOne(w, r)
		case strings.HasPrefix(subpath, "/github-sync/") && r.Method == http.MethodDelete:
			githubSyncHandler.DeleteConfig(w, r)

		// SIGMA Converter
		case subpath == "/sigma/convert" && r.Method == http.MethodPost:
			sigmaHandler.Convert(w, r)
		case subpath == "/sigma/convert/batch" && r.Method == http.MethodPost:
			sigmaHandler.ConvertBatch(w, r)
		case subpath == "/sigma/import" && r.Method == http.MethodPost:
			requirePermission(fleetauth.PermManageRules, sigmaHandler.ConvertAndImport)(w, r)
		case subpath == "/sigma/import/batch" && r.Method == http.MethodPost:
			requirePermission(fleetauth.PermManageRules, sigmaHandler.ImportBatch)(w, r)
		case subpath == "/sigma/validate" && r.Method == http.MethodPost:
			sigmaHandler.Validate(w, r)
		case subpath == "/sigma/logsources" && r.Method == http.MethodGet:
			sigmaHandler.SupportedLogsources(w, r)
		case subpath == "/sigma/field-mappings" && r.Method == http.MethodGet:
			sigmaHandler.FieldMappings(w, r)

		// Audit Log
		case subpath == "/audit-log" && r.Method == http.MethodGet:
			auditHandler.List(w, r)

		// Users
		case subpath == "/users" && r.Method == http.MethodGet:
			requirePermission(fleetauth.PermManageUsers, userHandler.List)(w, r)
		case subpath == "/users" && r.Method == http.MethodPost:
			requirePermission(fleetauth.PermManageUsers, userHandler.Create)(w, r)
		case strings.HasPrefix(subpath, "/users/") && strings.HasSuffix(subpath, "/groups") && r.Method == http.MethodGet:
			userHandler.GetUserGroups(w, r)
		case strings.HasPrefix(subpath, "/users/") && strings.HasSuffix(subpath, "/groups") && r.Method == http.MethodPut:
			requirePermission(fleetauth.PermManageUsers, userHandler.UpdateUserGroups)(w, r)
		case strings.HasPrefix(subpath, "/users/") && strings.HasSuffix(subpath, "/role") && r.Method == http.MethodPut:
			requirePermission(fleetauth.PermManageUsers, userHandler.UpdateRole)(w, r)
		case strings.HasPrefix(subpath, "/users/") && strings.HasSuffix(subpath, "/password") && r.Method == http.MethodPut:
			requirePermission(fleetauth.PermManageUsers, userHandler.ResetPassword)(w, r)
		case strings.HasPrefix(subpath, "/users/") && strings.HasSuffix(subpath, "/totp") && r.Method == http.MethodDelete:
			requirePermission(fleetauth.PermManageUsers, userHandler.DisableTOTP)(w, r)
		case strings.HasPrefix(subpath, "/users/") && r.Method == http.MethodPut:
			requirePermission(fleetauth.PermManageUsers, userHandler.Update)(w, r)
		case strings.HasPrefix(subpath, "/users/") && r.Method == http.MethodDelete:
			requirePermission(fleetauth.PermManageUsers, userHandler.Delete)(w, r)

		// User Groups
		case subpath == "/groups" && r.Method == http.MethodGet:
			groupHandler.List(w, r)
		case subpath == "/groups" && r.Method == http.MethodPost:
			requirePermission(fleetauth.PermManageUsers, groupHandler.Create)(w, r)
		case strings.HasPrefix(subpath, "/groups/") && strings.HasSuffix(subpath, "/members") && r.Method == http.MethodPost:
			requirePermission(fleetauth.PermManageUsers, groupHandler.AddMember)(w, r)
		case strings.HasPrefix(subpath, "/groups/") && strings.Contains(subpath, "/members/") && r.Method == http.MethodDelete:
			requirePermission(fleetauth.PermManageUsers, groupHandler.RemoveMember)(w, r)
		case strings.HasPrefix(subpath, "/groups/") && r.Method == http.MethodPut:
			requirePermission(fleetauth.PermManageUsers, groupHandler.Update)(w, r)
		case strings.HasPrefix(subpath, "/groups/") && r.Method == http.MethodDelete:
			requirePermission(fleetauth.PermManageUsers, groupHandler.Delete)(w, r)
		case subpath == "/permissions" && r.Method == http.MethodGet:
			groupHandler.GetPermissions(w, r)

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
	dashMux.HandleFunc("/api/v1/account/organizations/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			authHandler.DeleteOrganization(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	// Account-scoped: cross-org views (no org_id = aggregate all orgs)
	dashMux.HandleFunc("/api/v1/account/agents", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			agentHandler.List(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/account/detections/timeline", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet { detHandler.Timeline(w, r) } else { http.Error(w, "method not allowed", 405) }
	})
	dashMux.HandleFunc("/api/v1/account/detections/", func(w http.ResponseWriter, r *http.Request) {
		sub := strings.TrimPrefix(r.URL.Path, "/api/v1/account/detections/")
		if strings.HasSuffix(sub, "/process-tree") {
			detHandler.ProcessTree(w, r)
		} else if strings.HasSuffix(sub, "/process-context") {
			detHandler.ProcessContext(w, r)
		} else {
			detHandler.Get(w, r)
		}
	})
	dashMux.HandleFunc("/api/v1/account/detections", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			detHandler.List(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/account/rules/noisy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			ruleHandler.NoisyRules(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/account/rules", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			ruleHandler.List(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/account/dashboard/overview", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			dashHandler.Overview(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/account/enrollment-tokens", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			enrollTokenHandler.List(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	// YARA rules (account-scoped — shared across every org in the account).
	// Embedded inline in yara_scan command payloads at creation time.
	dashMux.HandleFunc("/api/v1/account/yara-rules", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			yaraRuleHandler.List(w, r)
		case http.MethodPost:
			requirePermission(fleetauth.PermManageRules, yaraRuleHandler.Create)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/account/yara-rules/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/account/yara-rules/validate" && r.Method == http.MethodPost:
			yaraRuleHandler.Validate(w, r)
		case r.Method == http.MethodGet:
			yaraRuleHandler.Get(w, r)
		case r.Method == http.MethodPut:
			requirePermission(fleetauth.PermManageRules, yaraRuleHandler.Update)(w, r)
		case r.Method == http.MethodDelete:
			requirePermission(fleetauth.PermManageRules, yaraRuleHandler.Delete)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/account/groups", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			groupHandler.List(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/account/permissions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			groupHandler.GetPermissions(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	// Account-scoped user management (cross-org)
	dashMux.HandleFunc("/api/v1/account/users/", func(w http.ResponseWriter, r *http.Request) {
		// Extract user ID and sub-resource from path
		sub := strings.TrimPrefix(r.URL.Path, "/api/v1/account/users/")
		sub = strings.TrimSuffix(sub, "/")

		if strings.HasSuffix(sub, "/groups") {
			// /account/users/{id}/groups
			switch r.Method {
			case http.MethodGet:
				userHandler.GetUserGroups(w, r)
			case http.MethodPut:
				requirePermission(fleetauth.PermManageUsers, userHandler.UpdateUserGroups)(w, r)
			default:
				http.Error(w, "method not allowed", 405)
			}
			return
		}

		if strings.HasSuffix(sub, "/password") {
			if r.Method == http.MethodPut {
				requirePermission(fleetauth.PermManageUsers, userHandler.ResetPassword)(w, r)
			} else {
				http.Error(w, "method not allowed", 405)
			}
			return
		}

		if strings.HasSuffix(sub, "/totp") {
			if r.Method == http.MethodDelete {
				requirePermission(fleetauth.PermManageUsers, userHandler.DisableTOTP)(w, r)
			} else {
				http.Error(w, "method not allowed", 405)
			}
			return
		}

		// /account/users/{id} — profile update or delete
		switch r.Method {
		case http.MethodPut:
			requirePermission(fleetauth.PermManageUsers, userHandler.Update)(w, r)
		case http.MethodDelete:
			requirePermission(fleetauth.PermManageUsers, userHandler.Delete)(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})
	dashMux.HandleFunc("/api/v1/account/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			userHandler.List(w, r)
		case http.MethodPost:
			requirePermission(fleetauth.PermManageUsers, userHandler.Create)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/account/audit-log", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			auditHandler.List(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	dashMux.HandleFunc("/api/v1/account/telemetry", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			telemetryHandler.Search(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/account/telemetry/fields", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			telemetryHandler.GetFieldValues(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/account/macros", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			macroHandler.List(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Account-scoped: GitHub Sync (detection as code) — account-wide configs
	dashMux.HandleFunc("/api/v1/account/github-sync/trigger", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			githubSyncHandler.TriggerSync(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/account/github-sync/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/trigger") && r.Method == http.MethodPost:
			githubSyncHandler.TriggerSyncOne(w, r)
		case r.Method == http.MethodDelete:
			githubSyncHandler.DeleteConfig(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/account/github-sync", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			githubSyncHandler.ListConfigs(w, r)
		case http.MethodPost:
			githubSyncHandler.SaveConfig(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	// Account-scoped: SIGMA converter
	dashMux.HandleFunc("/api/v1/account/sigma/convert/batch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost { sigmaHandler.ConvertBatch(w, r) } else { http.Error(w, "method not allowed", 405) }
	})
	dashMux.HandleFunc("/api/v1/account/sigma/convert", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost { sigmaHandler.Convert(w, r) } else { http.Error(w, "method not allowed", 405) }
	})
	dashMux.HandleFunc("/api/v1/account/sigma/import/batch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost { requirePermission(fleetauth.PermManageRules, sigmaHandler.ImportBatch)(w, r) } else { http.Error(w, "method not allowed", 405) }
	})
	dashMux.HandleFunc("/api/v1/account/sigma/import", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost { requirePermission(fleetauth.PermManageRules, sigmaHandler.ConvertAndImport)(w, r) } else { http.Error(w, "method not allowed", 405) }
	})
	dashMux.HandleFunc("/api/v1/account/sigma/validate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost { sigmaHandler.Validate(w, r) } else { http.Error(w, "method not allowed", 405) }
	})
	dashMux.HandleFunc("/api/v1/account/sigma/logsources", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet { sigmaHandler.SupportedLogsources(w, r) } else { http.Error(w, "method not allowed", 405) }
	})
	dashMux.HandleFunc("/api/v1/account/sigma/field-mappings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet { sigmaHandler.FieldMappings(w, r) } else { http.Error(w, "method not allowed", 405) }
	})

	// Account-scoped: SigmaHQ integration
	dashMux.HandleFunc("/api/v1/account/sigmahq/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet { sigmahqHandler.Status(w, r) } else { http.Error(w, "method not allowed", 405) }
	})
	dashMux.HandleFunc("/api/v1/account/sigmahq/enable", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost { requirePermission(fleetauth.PermManageRules, sigmahqHandler.Enable)(w, r) } else { http.Error(w, "method not allowed", 405) }
	})
	dashMux.HandleFunc("/api/v1/account/sigmahq/disable", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost { requirePermission(fleetauth.PermManageRules, sigmahqHandler.Disable)(w, r) } else { http.Error(w, "method not allowed", 405) }
	})
	dashMux.HandleFunc("/api/v1/account/sigmahq/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost { requirePermission(fleetauth.PermManageRules, sigmahqHandler.Refresh)(w, r) } else { http.Error(w, "method not allowed", 405) }
	})

	dashMux.HandleFunc("/api/v1/account/settings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			authHandler.GetAccountSettings(w, r)
		case http.MethodPut:
			requirePermission(fleetauth.PermManageSettings, authHandler.UpdateAccountSettings)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/account/telemetry-retention", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			requirePermission(fleetauth.PermManageSettings, authHandler.UpdateTelemetryRetention)(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/account/orgs/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tamper-protection") && r.Method == http.MethodPut {
			requirePermission(fleetauth.PermManageSettings, authHandler.UpdateOrgTamperProtection)(w, r)
		} else if strings.HasSuffix(r.URL.Path, "/telemetry-retention") && r.Method == http.MethodPut {
			requirePermission(fleetauth.PermManageSettings, authHandler.UpdateOrgRetention)(w, r)
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	// Admin routes (JWT auth, root only — permission checked inside handlers)
	dashMux.HandleFunc("/api/v1/admin/accounts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandler.ListAccounts(w, r)
		case http.MethodPost:
			adminHandler.CreateAccount(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/admin/accounts/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/orgs") && r.Method == http.MethodGet {
			adminHandler.ListAccountOrgs(w, r)
		} else if strings.HasSuffix(r.URL.Path, "/users") && r.Method == http.MethodGet {
			adminHandler.ListAccountUsers(w, r)
		} else if r.Method == http.MethodDelete {
			adminHandler.DeleteAccount(w, r)
		} else if r.Method == http.MethodPut {
			adminHandler.UpdateAccount(w, r)
		} else {
			http.NotFound(w, r)
		}
	})
	dashMux.HandleFunc("/api/v1/admin/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandler.ListAllUsers(w, r)
		case http.MethodPost:
			adminHandler.CreateUser(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/admin/users/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/unlock") && r.Method == http.MethodPost:
			adminHandler.UnlockUser(w, r)
		case strings.HasSuffix(r.URL.Path, "/reset-password") && r.Method == http.MethodPost:
			adminHandler.ResetUserPassword(w, r)
		case strings.HasSuffix(r.URL.Path, "/disable-totp") && r.Method == http.MethodPost:
			adminHandler.DisableUserTOTP(w, r)
		case r.Method == http.MethodPut:
			adminHandler.UpdateUser(w, r)
		case r.Method == http.MethodDelete:
			adminHandler.DeleteUser(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	dashMux.HandleFunc("/api/v1/admin/switch-account", methodGuard(http.MethodPost, adminHandler.SwitchAccount))

	// Signup approval routes (root only): GET lists pending users,
	// POST /:id/approve flips status to approved, POST /:id/reject to rejected.
	dashMux.HandleFunc("/api/v1/admin/pending-users", methodGuard(http.MethodGet, adminHandler.ListPendingUsers))
	dashMux.HandleFunc("/api/v1/admin/pending-users/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/approve") && r.Method == http.MethodPost:
			adminHandler.ApprovePendingUser(w, r)
		case strings.HasSuffix(r.URL.Path, "/reject") && r.Method == http.MethodPost:
			adminHandler.RejectPendingUser(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	// DB Admin routes (root only)
	dashMux.HandleFunc("/api/v1/admin/db/postgres/query", methodGuard(http.MethodPost, dbAdminHandler.QueryPG))
	dashMux.HandleFunc("/api/v1/admin/db/postgres/tables", methodGuard(http.MethodGet, dbAdminHandler.TablesPG))
	dashMux.HandleFunc("/api/v1/admin/db/clickhouse/query", methodGuard(http.MethodPost, dbAdminHandler.QueryCH))
	dashMux.HandleFunc("/api/v1/admin/db/clickhouse/tables", methodGuard(http.MethodGet, dbAdminHandler.TablesCH))
	dashMux.HandleFunc("/api/v1/admin/db/clickhouse/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			chConfigHandler.Get(w, r)
		case http.MethodPut:
			chConfigHandler.Save(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/admin/db/clickhouse/test", methodGuard(http.MethodPost, chConfigHandler.Test))

	// TOTP 2FA routes (JWT auth, user-scoped)
	dashMux.HandleFunc("/api/v1/auth/totp/setup", methodGuard(http.MethodPost, totpHandler.Setup))
	dashMux.HandleFunc("/api/v1/auth/totp/verify", methodGuard(http.MethodPost, totpHandler.Verify))
	dashMux.HandleFunc("/api/v1/auth/totp/disable", methodGuard(http.MethodPost, totpHandler.Disable))
	dashMux.HandleFunc("/api/v1/auth/totp/status", methodGuard(http.MethodGet, totpHandler.Status))
	dashMux.HandleFunc("/api/v1/auth/me/permissions", methodGuard(http.MethodGet, authHandler.GetMyPermissions))
	dashMux.HandleFunc("/api/v1/auth/me/profile", methodGuard(http.MethodPut, authHandler.UpdateMyProfile))
	dashMux.HandleFunc("/api/v1/auth/me/password", methodGuard(http.MethodPut, authHandler.ChangeMyPassword))
	dashMux.HandleFunc("/api/v1/auth/me", methodGuard(http.MethodGet, authHandler.GetCurrentUser))

	// API key management (JWT or API key auth)
	dashMux.HandleFunc("/api/v1/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			authHandler.ListAPIKeys(w, r)
		case http.MethodPost:
			authHandler.CreateAPIKey(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	dashMux.HandleFunc("/api/v1/auth/api-keys/", methodGuard(http.MethodDelete, authHandler.DeleteAPIKey))

	// Rule validation (JWT or API key auth — moved from public for programmatic access)
	dashMux.HandleFunc("/api/v1/validate-rule", methodGuard(http.MethodPost, handler.ValidateRuleAPI))

	// Wrap dashboard routes with JWT auth
	dashAuthenticated := jwtAuth(s.config.Auth.JWTSecret, dashMux)

	// ── Combine all route groups ─────────────────────────────

	rootMux := http.NewServeMux()

	// Public
	rootMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	rootMux.HandleFunc("/install/", mux.ServeHTTP)
	rootMux.HandleFunc("/api/v1/agent/binary", mux.ServeHTTP)
	rootMux.HandleFunc("/api/v1/agent/msi", mux.ServeHTTP)
	rootMux.HandleFunc("/api/v1/agent/config", mux.ServeHTTP)
	rootMux.HandleFunc("/api/v1/auth/totp/", dashAuthenticated.ServeHTTP)
	rootMux.HandleFunc("/api/v1/auth/me/", dashAuthenticated.ServeHTTP)
	rootMux.HandleFunc("/api/v1/auth/me", dashAuthenticated.ServeHTTP)
	rootMux.HandleFunc("/api/v1/auth/api-keys", dashAuthenticated.ServeHTTP)
	rootMux.HandleFunc("/api/v1/auth/api-keys/", dashAuthenticated.ServeHTTP)
	rootMux.HandleFunc("/api/v1/auth/", mux.ServeHTTP)

	// Route dispatcher — determines auth path based on URL prefix
	rootMux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Public routes (no auth required) — TOTP, /me, and /api-keys routes require JWT
		if (strings.HasPrefix(path, "/api/v1/auth/") && !strings.HasPrefix(path, "/api/v1/auth/totp/") && !strings.HasPrefix(path, "/api/v1/auth/me") && !strings.HasPrefix(path, "/api/v1/auth/api-keys")) || path == "/api/v1/enroll" || path == "/api/v1/agents/register" {
			mux.ServeHTTP(w, r)
			return
		}

		// Public agent endpoints (no auth — served by install handler)
		if path == "/api/v1/agent/binary" || path == "/api/v1/agent/msi" || path == "/api/v1/agent/config" {
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

	// Start telemetry retention purge (every minute, keep 10 minutes for live view)
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				deleted, err := telemetryStore.Purge(context.Background(), 0)
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

// seedMacrosFromFile ensures every org has the default Fibratus macros.
// Orgs that already have the full set are skipped.
func seedMacrosFromFile(ctx context.Context, macroStore store.MacroStore, orgStore store.OrgStore, db *sql.DB) {
	// Load macros YAML from disk
	var macrosData []byte
	for _, path := range []string{
		"rules/macros/macros.yml",
		"/opt/fibratus-fleet/src/rules/macros/macros.yml",
	} {
		data, err := os.ReadFile(path)
		if err == nil {
			macrosData = data
			break
		}
	}
	if macrosData == nil {
		return
	}

	pStore, ok := macroStore.(*postgres.MacroStore)
	if !ok {
		return
	}

	// Get all orgs
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

	for _, orgID := range orgIDs {
		// Check how many macros this org has
		var count int
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM macros WHERE org_id = $1", orgID).Scan(&count)
		if count >= 60 {
			continue // already has macros, skip
		}
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

// seedRulesFromFiles ensures every org has the official Fibratus detection rules.
// Orgs that already have official rules are skipped.
func seedRulesFromFiles(ctx context.Context, ruleStore store.RuleStore, db *sql.DB) {
	// Find rules directory
	var rulesDir string
	for _, dir := range []string{
		"rules",
		"/opt/fibratus-fleet/src/rules",
	} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			rulesDir = dir
			break
		}
	}
	if rulesDir == "" {
		return
	}

	// Get all orgs
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

	for _, orgID := range orgIDs {
		// Check if this org already has official rules
		var count int
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM rules WHERE org_id = $1 AND source = 'official'", orgID).Scan(&count)
		if count >= 100 {
			continue // already has official rules
		}

		var seeded int
		filepath.Walk(rulesDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".yaml") {
				return nil
			}
			if strings.Contains(path, "macros/") || strings.Contains(path, "macros\\") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			var ruleData struct {
				Name      string            `yaml:"name"`
				ID        string            `yaml:"id"`
				Version   string            `yaml:"version"`
				Desc      string            `yaml:"description"`
				Condition string            `yaml:"condition"`
				Output    string            `yaml:"output"`
				Severity  string            `yaml:"severity"`
				Labels    map[string]string `yaml:"labels"`
				Tags      []string          `yaml:"tags"`
				Refs      []string          `yaml:"references"`
			}
			if err := yaml.Unmarshal(data, &ruleData); err != nil || ruleData.Name == "" {
				return nil
			}
			rule := &fleet.Rule{
				ID: ruleData.ID, OrgID: orgID, Name: ruleData.Name,
				Version: ruleData.Version, Description: ruleData.Desc,
				Condition: ruleData.Condition, Output: ruleData.Output,
				Severity: ruleData.Severity, Labels: ruleData.Labels,
				Tags: ruleData.Tags, References: ruleData.Refs,
				RawYAML: string(data), Enabled: true, Source: "official",
				ValidationStatus: "valid", ValidationErrors: json.RawMessage(`[]`),
			}
			if rule.ID == "" {
				rule.ID = handler.GenerateID()
			}
			if rule.Version == "" {
				rule.Version = "1.0.0"
			}
			if rule.Severity == "" {
				rule.Severity = "medium"
			}
			if err := ruleStore.Create(ctx, rule); err != nil {
				return nil // duplicate, skip
			}
			seeded++
			return nil
		})
		if seeded > 0 {
			log.Infof("fleet: seeded %d official rules for org %s", seeded, orgID)
		}
	}
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":{"code":%d,"message":"%s"}}`, status, msg)
}
