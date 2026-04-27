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
		ResetPassword: func(req handler.CloudResetPasswordRequest) error {
			ctx := context.Background()
			cli, _, err := s.cloudClient(ctx)
			if err != nil {
				return err
			}
			cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			// Rotate the password.
			newPw, err := cli.ResetServicePassword(cctx, req.OrgID, req.ServiceID, "")
			if err != nil {
				return err
			}
			if newPw == "" {
				return errors.New("API rotated the password but did not return the new value")
			}
			// Then resolve the service's connection endpoint and bind the local
			// config so YAML host/port + secret-store password move together.
			// Without this, the encrypted store would diverge from the YAML and
			// the next restart would crash-loop on auth failure against the old
			// host with the new password.
			svc, err := cli.GetService(cctx, req.OrgID, req.ServiceID)
			if err != nil {
				return fmt.Errorf("password rotated but could not fetch service endpoint to bind: %w", err)
			}
			host, port, ok := svc.NativeSecureEndpoint()
			if !ok {
				return errors.New("password rotated but service has no native secure endpoint")
			}
			db := req.Database
			if db == "" {
				db = "default" // Cloud services ship with `default` only — never inherit a local DB name
			}
			user := req.User
			if user == "" {
				user = "default"
			}
			return s.bindCloudService(ctx, host, port, db, user, newPw, req.OrgID)
		},
		Restart: func() {
			// Give the in-flight HTTP response time to flush before exit.
			time.Sleep(500 * time.Millisecond)
			log.Warn("fleet: graceful exit for restart")
			os.Exit(0)
		},
	}
}

// bindCloudService writes the cloud connection details into the "cloud"
// profile, stores the supplied password under the cloud profile's secret key,
// and — if the cloud profile is currently active — hot-swaps the running
// pipeline so the new connection takes effect immediately, no restart.
func (s *Server) bindCloudService(ctx context.Context, host string, port int, db, user, password, orgID string) error {
	host = strings.TrimSpace(host)
	if host == "" || port == 0 {
		return errors.New("invalid cloud service endpoint")
	}
	if password == "" {
		return errors.New("service password is required")
	}
	if db == "" {
		db = "default"
	}
	if user == "" {
		user = "default"
	}
	if s.profileStore == nil {
		return errors.New("clickhouse profile store not initialised")
	}

	// Read the existing cloud profile (created by boot migration) so we
	// preserve operator-tuned fields (pool sizes, timeouts) on rebind.
	existing, err := s.profileStore.Get(ctx, ProfileCloud)
	if err != nil {
		return fmt.Errorf("load cloud profile: %w", err)
	}
	existing.Enabled = true
	existing.Host = host
	existing.Port = port
	existing.Database = db
	existing.User = user
	existing.Secure = true
	existing.SkipVerify = false
	if existing.DialTimeoutSecs <= 0 {
		existing.DialTimeoutSecs = 10
	}
	if existing.MaxOpenConns <= 0 {
		existing.MaxOpenConns = 20
	}
	if existing.MaxIdleConns <= 0 {
		existing.MaxIdleConns = 10
	}
	if existing.ConnMaxLifetimeSecs <= 0 {
		existing.ConnMaxLifetimeSecs = 3600
	}
	existing.CloudOrgID = orgID
	existing.UpdatedBy = "dashboard-cloud-bind"

	if err := s.profileStore.Upsert(ctx, existing); err != nil {
		return err
	}
	if err := s.SetProfilePassword(ctx, ProfileCloud, password, "dashboard-cloud-bind"); err != nil {
		return err
	}
	if orgID != "" {
		_ = s.secretStore.Set(ctx, SecretClickHouseCloudOrgID, orgID, "dashboard-cloud-bind")
	}

	// Hot-swap if this is the active profile, otherwise just leave the
	// updated row for the next activation.
	active, _ := s.secretStore.GetOr(ctx, SecretActiveProfile, "")
	if active == ProfileCloud {
		if err := s.activateProfile(ctx, ProfileCloud); err != nil {
			return fmt.Errorf("bound cloud profile but hot-swap failed: %w", err)
		}
	}
	log.Infof("fleet: bound clickhouse cloud profile to %s:%d (db=%s user=%s)", host, port, db, user)
	return nil
}
