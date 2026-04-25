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
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rabbitstack/fibratus/internal/fleetserver/clickhousecloud"
	"github.com/rabbitstack/fibratus/internal/fleetserver/handler"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store/postgres"
	log "github.com/sirupsen/logrus"
)

// cloudClient builds a Cloud API client from the credentials stored in the
// secret store. Returns an error if credentials are not configured.
func (s *Server) cloudClient(ctx context.Context) (*clickhousecloud.Client, string, error) {
	if s.secretStore == nil {
		return nil, "", errors.New("secret store not initialised")
	}
	keyID, err := s.secretStore.Get(ctx, SecretClickHouseCloudKeyID)
	if err != nil {
		if errors.Is(err, postgres.ErrSecretNotFound) {
			return nil, "", errors.New("ClickHouse Cloud API credentials not configured")
		}
		return nil, "", err
	}
	keySecret, err := s.secretStore.Get(ctx, SecretClickHouseCloudKeySecret)
	if err != nil {
		if errors.Is(err, postgres.ErrSecretNotFound) {
			return nil, "", errors.New("ClickHouse Cloud API credentials not configured")
		}
		return nil, "", err
	}
	orgID, _ := s.secretStore.GetOr(ctx, SecretClickHouseCloudOrgID, "")
	return clickhousecloud.NewClient(keyID, keySecret, "", nil), orgID, nil
}

// cloudHandlerDeps wires the dashboard-facing CloudHandler to server-side
// state: the secret store, the in-memory ClickHouse config, and the live
// ClickHouse connection. Each closure handles its own context boundaries so
// the handler stays free of Postgres / clickhousecloud imports.
func (s *Server) cloudHandlerDeps() handler.CloudHandlerDeps {
	return handler.CloudHandlerDeps{
		GetStatus: func() handler.CloudCredentialsStatusDTO {
			ctx := context.Background()
			keyID, _ := s.secretStore.GetOr(ctx, SecretClickHouseCloudKeyID, "")
			orgID, _ := s.secretStore.GetOr(ctx, SecretClickHouseCloudOrgID, "")
			has, _ := s.secretStore.Has(ctx, SecretClickHouseCloudKeySecret)
			return handler.CloudCredentialsStatusDTO{
				Configured: has && keyID != "",
				KeyID:      keyID,
				OrgID:      orgID,
			}
		},
		SaveCredentials: func(req handler.CloudCredentialsRequest) error {
			ctx := context.Background()
			if err := s.secretStore.Set(ctx, SecretClickHouseCloudKeyID, req.KeyID, "dashboard"); err != nil {
				return err
			}
			if err := s.secretStore.Set(ctx, SecretClickHouseCloudKeySecret, req.KeySecret, "dashboard"); err != nil {
				return err
			}
			if req.OrgID != "" {
				if err := s.secretStore.Set(ctx, SecretClickHouseCloudOrgID, req.OrgID, "dashboard"); err != nil {
					return err
				}
			}
			// Validate by issuing one cheap call.
			cli, _, err := s.cloudClient(ctx)
			if err != nil {
				return err
			}
			cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			if _, err := cli.ListOrganizations(cctx); err != nil {
				return fmt.Errorf("validation failed (credentials saved but cannot reach API): %w", err)
			}
			return nil
		},
		DeleteCredentials: func() error {
			ctx := context.Background()
			_ = s.secretStore.Delete(ctx, SecretClickHouseCloudKeyID)
			_ = s.secretStore.Delete(ctx, SecretClickHouseCloudKeySecret)
			_ = s.secretStore.Delete(ctx, SecretClickHouseCloudOrgID)
			return nil
		},
		ListOrgs: func(r *http.Request) (interface{}, error) {
			cli, _, err := s.cloudClient(r.Context())
			if err != nil {
				return nil, err
			}
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			return cli.ListOrganizations(ctx)
		},
		ListServices: func(r *http.Request, orgID string) (interface{}, error) {
			cli, _, err := s.cloudClient(r.Context())
			if err != nil {
				return nil, err
			}
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			return cli.ListServices(ctx, orgID)
		},
		CreateService: func(req handler.CloudCreateServiceRequest) (interface{}, error) {
			ctx := context.Background()
			cli, _, err := s.cloudClient(ctx)
			if err != nil {
				return nil, err
			}
			cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			out, err := cli.CreateService(cctx, req.OrgID, clickhousecloud.CreateServiceRequest{
				Name:               req.Name,
				Provider:           req.Provider,
				Region:             req.Region,
				Tier:               req.Tier,
				MinReplicaMemoryGB: req.MinReplicaMemoryGB,
				MaxReplicaMemoryGB: req.MaxReplicaMemoryGB,
			})
			if err != nil {
				return nil, err
			}
			// Persist the new service's connection details + generated password.
			host, port, ok := out.Service.NativeSecureEndpoint()
			if !ok {
				return nil, errors.New("created service has no native secure endpoint yet — wait for provisioning to complete and call Connect manually")
			}
			user := req.User
			if user == "" {
				user = "default"
			}
			db := req.Database
			if db == "" {
				db = "default"
			}
			if err := s.bindCloudService(ctx, host, port, db, user, out.Password, req.OrgID); err != nil {
				return nil, fmt.Errorf("service created but binding to local config failed: %w", err)
			}
			// Don't echo the password back to the client — it's already saved.
			return map[string]interface{}{
				"service_id":       out.Service.ID,
				"service_name":     out.Service.Name,
				"host":             host,
				"port":             port,
				"region":           out.Service.Region,
				"state":            out.Service.State,
				"bound":            true,
				"restart_required": true,
			}, nil
		},
		ConnectService: func(req handler.CloudConnectRequest) error {
			ctx := context.Background()
			cli, _, err := s.cloudClient(ctx)
			if err != nil {
				return err
			}
			cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			svc, err := cli.GetService(cctx, req.OrgID, req.ServiceID)
			if err != nil {
				return err
			}
			host, port, ok := svc.NativeSecureEndpoint()
			if !ok {
				return errors.New("service has no native secure endpoint")
			}
			user := req.User
			if user == "" {
				user = "default"
			}
			db := req.Database
			if db == "" {
				db = "default"
			}
			return s.bindCloudService(ctx, host, port, db, user, req.ServicePassword, req.OrgID)
		},
		ResetPassword: func(orgID, serviceID string) error {
			ctx := context.Background()
			cli, _, err := s.cloudClient(ctx)
			if err != nil {
				return err
			}
			cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			newPw, err := cli.ResetServicePassword(cctx, orgID, serviceID, "")
			if err != nil {
				return err
			}
			if newPw == "" {
				return errors.New("API rotated the password but did not return the new value")
			}
			s.config.ClickHouse.Password = newPw
			if err := s.secretStore.Set(ctx, SecretClickHousePassword, newPw, "dashboard-cloud-reset"); err != nil {
				return err
			}
			return nil
		},
		Restart: func() {
			// Give the in-flight HTTP response time to flush before exit.
			time.Sleep(500 * time.Millisecond)
			log.Warn("fleet: graceful exit for restart")
			os.Exit(0)
		},
	}
}

// bindCloudService updates the in-memory ClickHouse config with cloud
// connection details, persists the password to the secret store, and rewrites
// the YAML on disk (without the password). Restart is required to pick up the
// new connection.
func (s *Server) bindCloudService(ctx context.Context, host string, port int, db, user, password, orgID string) error {
	if s.configPath == "" {
		return errors.New("config path unknown — cannot persist changes")
	}
	host = strings.TrimSpace(host)
	if host == "" || port == 0 {
		return errors.New("invalid cloud service endpoint")
	}
	if password == "" {
		return errors.New("service password is required")
	}
	s.config.ClickHouse = ClickHouseConfig{
		Enabled:         true,
		Host:            host,
		Port:            port,
		Database:        db,
		User:            user,
		Password:        password,
		Secure:          true,
		SkipVerify:      false,
		DialTimeoutSecs: 10,
		MaxOpenConns:    s.config.ClickHouse.MaxOpenConns,
		MaxIdleConns:    s.config.ClickHouse.MaxIdleConns,
		ConnMaxLifetime: s.config.ClickHouse.ConnMaxLifetime,
	}
	if s.config.ClickHouse.MaxOpenConns == 0 {
		s.config.ClickHouse.MaxOpenConns = 20
	}
	if s.config.ClickHouse.MaxIdleConns == 0 {
		s.config.ClickHouse.MaxIdleConns = 10
	}
	if s.config.ClickHouse.ConnMaxLifetime == 0 {
		s.config.ClickHouse.ConnMaxLifetime = 3600
	}
	if err := s.secretStore.Set(ctx, SecretClickHousePassword, password, "dashboard-cloud-connect"); err != nil {
		return err
	}
	if orgID != "" {
		_ = s.secretStore.Set(ctx, SecretClickHouseCloudOrgID, orgID, "dashboard-cloud-connect")
	}
	yamlSafe := *s.config
	yamlSafe.ClickHouse.Password = ""
	if err := SaveConfig(s.configPath, &yamlSafe); err != nil {
		return err
	}
	log.Infof("fleet: bound to ClickHouse Cloud service at %s:%d (db=%s user=%s) — restart required",
		host, port, db, user)
	return nil
}
