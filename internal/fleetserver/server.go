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
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rabbitstack/fibratus/internal/fleetserver/handler"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store/postgres"
	log "github.com/sirupsen/logrus"
)

// Server is the fleet management HTTP server.
type Server struct {
	config      *Config
	httpServer  *http.Server
	pgStore     *postgres.Store
	agentStore  *postgres.AgentStore
	detStore    *postgres.DetectionStore
	apiKeys     map[string]bool
}

// New creates a new fleet server instance.
func New(cfg *Config) (*Server, error) {
	// Connect to PostgreSQL
	pgStore, err := postgres.New(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("fleet server: %w", err)
	}

	agentStore := postgres.NewAgentStore(pgStore.DB())
	detStore := postgres.NewDetectionStore(pgStore.DB())

	// Build API key lookup map
	apiKeys := make(map[string]bool)
	for _, k := range cfg.Auth.APIKeys {
		if k.Key != "" {
			apiKeys[k.Key] = true
		}
	}

	return &Server{
		config:     cfg,
		pgStore:    pgStore,
		agentStore: agentStore,
		detStore:   detStore,
		apiKeys:    apiKeys,
	}, nil
}

// Run starts the fleet server and blocks until the context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	// Run migrations
	log.Info("running database migrations...")
	if err := postgres.Migrate(s.config.Database); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	// Create handlers
	agentHandler := handler.NewAgentHandler(s.agentStore)
	detHandler := handler.NewDetectionHandler(s.detStore, s.agentStore)
	dashHandler := handler.NewDashboardHandler(s.agentStore, s.detStore)

	// Setup routes
	mux := http.NewServeMux()

	// Agent API endpoints
	mux.HandleFunc("/api/v1/agents/register", methodGuard(http.MethodPost, agentHandler.Register))
	mux.HandleFunc("/api/v1/agents/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// POST /api/v1/agents/{id}/heartbeat
		if r.Method == http.MethodPost && strings.HasSuffix(path, "/heartbeat") {
			agentHandler.Heartbeat(w, r)
			return
		}

		// GET /api/v1/agents (list)
		if path == "/api/v1/agents/" || path == "/api/v1/agents" {
			if r.Method == http.MethodGet {
				agentHandler.List(w, r)
				return
			}
		}

		// GET/DELETE /api/v1/agents/{id}
		switch r.Method {
		case http.MethodGet:
			agentHandler.Get(w, r)
		case http.MethodDelete:
			agentHandler.Delete(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Detection API endpoints
	mux.HandleFunc("/api/v1/detections/timeline", methodGuard(http.MethodGet, detHandler.Timeline))
	mux.HandleFunc("/api/v1/detections/mitre", methodGuard(http.MethodGet, detHandler.MitreHeatmap))
	mux.HandleFunc("/api/v1/detections/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/api/v1/detections/" || path == "/api/v1/detections" {
			switch r.Method {
			case http.MethodGet:
				detHandler.List(w, r)
			case http.MethodPost:
				detHandler.Ingest(w, r)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
		// GET /api/v1/detections/{id}
		if r.Method == http.MethodGet {
			detHandler.Get(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/api/v1/detections", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			detHandler.List(w, r)
		case http.MethodPost:
			detHandler.Ingest(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Dashboard API endpoints
	mux.HandleFunc("/api/v1/dashboard/overview", methodGuard(http.MethodGet, dashHandler.Overview))

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Serve embedded dashboard for all non-API routes
	if s.config.Dashboard.Enabled {
		mux.Handle("/", dashboardHandler())
	}

	// Apply middleware chain: logging -> CORS -> auth
	var h http.Handler = mux
	h = apiKeyAuth(s.apiKeys, h)
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
	go s.agentReaper(ctx)

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
func (s *Server) agentReaper(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n, err := s.agentStore.MarkOfflineAgents(ctx, s.config.Agent.HeartbeatTimeout)
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
