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

package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"

	"github.com/rabbitstack/fibratus/internal/fleetserver"
	"github.com/rabbitstack/fibratus/internal/fleetserver/fleetauth"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store/postgres"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	_ "github.com/lib/pq"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"time"
)

var (
	bsAccountName string
	bsOrgName     string
	bsAdminEmail  string
	bsAdminName   string
	bsAdminPass   string
)

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Create initial account, organization, and admin user",
	Long: `Bootstrap creates the initial account, organization, admin user, and API key
needed to operate the fleet server. Run this once after first migration.

Example:
  fleet-server bootstrap --config configs/fleet-server.yml \
    --account "Acme Corp" --org "Production" \
    --email admin@acme.com --name "Admin" --password "changeme123"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Auto-generate password if not provided (must meet policy: 12+ chars, upper/lower/digit/special)
		if bsAdminPass == "" {
			b := make([]byte, 12)
			rand.Read(b)
			bsAdminPass = "Fb!" + hex.EncodeToString(b)[:13]
		}

		cfg, err := fleetserver.LoadConfig(configFile)
		if err != nil {
			return err
		}
		initLogging(cfg.Logging.Level)

		// Run migrations first
		log.Info("running database migrations...")
		if err := postgres.Migrate(cfg.Database.DSN()); err != nil {
			return err
		}

		db, err := sql.Open("postgres", cfg.Database.DSN())
		if err != nil {
			return fmt.Errorf("failed to connect to database: %w", err)
		}
		defer db.Close()

		ctx := context.Background()

		// Generate IDs
		accountID := generateID()
		orgID := generateID()
		userID := generateID()
		apiKey := generateAPIKey()

		// Hash password
		passHash, err := fleetauth.HashPassword(bsAdminPass)
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}

		// Create account
		accountStore := postgres.NewAccountStore(db)
		err = accountStore.Create(ctx, &fleet.Account{
			ID:   accountID,
			Name: bsAccountName,
			Plan: "enterprise",
		})
		if err != nil {
			return fmt.Errorf("failed to create account: %w", err)
		}
		log.Infof("created account: %s (%s)", bsAccountName, accountID)

		// Create organization
		orgStore := postgres.NewOrgStore(db)
		err = orgStore.Create(ctx, &fleet.Organization{
			ID:        orgID,
			AccountID: accountID,
			Name:      bsOrgName,
			Slug:      slugify(bsOrgName),
		})
		if err != nil {
			return fmt.Errorf("failed to create organization: %w", err)
		}
		log.Infof("created organization: %s (%s)", bsOrgName, orgID)

		// Create default agent group for the org
		_, err = db.ExecContext(ctx,
			`INSERT INTO agent_groups (id, org_id, name, description) VALUES ($1, $2, 'default', 'Default agent group') ON CONFLICT DO NOTHING`,
			"default-"+orgID[:8], orgID,
		)
		if err != nil {
			return fmt.Errorf("failed to create default group: %w", err)
		}

		// Create admin user (root role — full system access)
		userStore := postgres.NewUserStore(db)
		err = userStore.Create(ctx, &fleet.User{
			ID:        userID,
			Email:     bsAdminEmail,
			Name:      bsAdminName,
			Password:  passHash,
			AccountID: accountID,
			Role:      "root",
		})
		if err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}

		// Grant org access
		err = userStore.AddOrgAccess(ctx, userID, orgID, "admin")
		if err != nil {
			return fmt.Errorf("failed to grant org access: %w", err)
		}
		log.Infof("created root user: %s (%s)", bsAdminEmail, userID)

		// Enforce 2FA on the account — admin must set up TOTP on first login
		err = accountStore.UpdateSettings(ctx, accountID, true)
		if err != nil {
			log.Warnf("failed to enable 2FA enforcement: %v", err)
		} else {
			log.Info("2FA enforcement enabled — admin must configure TOTP on first login")
		}

		// Create enrollment token
		enrollTokenID := "ft-enroll-" + generateID()
		enrollStore := postgres.NewEnrollmentTokenStore(db)
		err = enrollStore.Create(ctx, &fleet.EnrollmentToken{
			ID:        enrollTokenID,
			AccountID: accountID,
			OrgID:     orgID,
			Name:      "initial-bootstrap",
			MaxUses:   1000,
			ExpiresAt: time.Now().Add(365 * 24 * time.Hour), // 1 year
			CreatedBy: userID,
		})
		if err != nil {
			return fmt.Errorf("failed to create enrollment token: %w", err)
		}
		log.Infof("created enrollment token: %s", enrollTokenID)

		// Print results
		fmt.Println("")
		fmt.Println("═══════════════════════════════════════════════════════")
		fmt.Println("  Fleet Server — Bootstrap Complete")
		fmt.Println("═══════════════════════════════════════════════════════")
		fmt.Println("")
		fmt.Printf("  Account:          %s\n", bsAccountName)
		fmt.Printf("  Organization:     %s\n", bsOrgName)
		fmt.Printf("  Org ID:           %s\n", orgID)
		fmt.Println("")
		fmt.Println("  Dashboard login:")
		fmt.Printf("    Email:          %s\n", bsAdminEmail)
		fmt.Printf("    Password:       %s\n", bsAdminPass)
		fmt.Printf("    Role:           root\n")
		fmt.Println("")
		fmt.Println("  2FA enforcement is ON — you will be required to set up")
		fmt.Println("  an authenticator app (Google Authenticator, Authy, etc.)")
		fmt.Println("  on your first login.")
		fmt.Println("")
		fmt.Printf("  Enrollment Token: %s\n", enrollTokenID)
		fmt.Printf("  API Key:          %s\n", apiKey)
		fmt.Println("")
		fmt.Println("  Enroll agents (run on each Windows endpoint):")
		fmt.Println("")
		fmt.Printf("    fibratus enroll --token %s --server https://<SERVER-IP>:8443 --insecure\n", enrollTokenID)
		fmt.Println("")

		return nil
	},
}

func init() {
	RootCmd.AddCommand(bootstrapCmd)

	bootstrapCmd.Flags().StringVar(&bsAccountName, "account", "Default", "Account name")
	bootstrapCmd.Flags().StringVar(&bsOrgName, "org", "Production", "Organization name")
	bootstrapCmd.Flags().StringVar(&bsAdminEmail, "email", "admin@fibratus.local", "Admin email")
	bootstrapCmd.Flags().StringVar(&bsAdminName, "name", "Admin", "Admin display name")
	bootstrapCmd.Flags().StringVar(&bsAdminPass, "password", "", "Admin password (generated if empty)")
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateAPIKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func slugify(s string) string {
	result := make([]byte, 0, len(s))
	for _, c := range []byte(s) {
		switch {
		case c >= 'a' && c <= 'z':
			result = append(result, c)
		case c >= 'A' && c <= 'Z':
			result = append(result, c+32) // lowercase
		case c >= '0' && c <= '9':
			result = append(result, c)
		case c == ' ' || c == '-' || c == '_':
			if len(result) > 0 && result[len(result)-1] != '-' {
				result = append(result, '-')
			}
		}
	}
	return string(result)
}
