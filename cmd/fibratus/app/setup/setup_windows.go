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

package setup

import (
	"fmt"

	"github.com/rabbitstack/fibratus/cmd/fibratus/app/enroll"
	"github.com/rabbitstack/fibratus/cmd/fibratus/app/service"
	"github.com/rabbitstack/fibratus/pkg/fleet/tamper"
	"github.com/rabbitstack/fibratus/pkg/sys"
	"github.com/spf13/cobra"
)

var (
	token     string
	serverURL string
	dataDir   string
	insecure  bool
)

// Command is the setup Cobra command.
var Command = &cobra.Command{
	Use:   "setup",
	Short: "Enroll, secure, and start the agent in one step",
	Long: `Setup performs the complete agent enrollment and installation workflow:

  1. Verify running as Administrator
  2. Enroll with the fleet server (generate certs, register agent)
  3. Lock down the data directory with NTFS ACLs (SYSTEM + Admins only)
  4. Generate an integrity seal over enrollment data
  5. Install the Windows service with recovery options
  6. Start the service

Example:
  fibratus setup --token ft-enroll-abc123... --server https://fleet.acme.com`,
	RunE: runSetup,
}

func init() {
	Command.Flags().StringVar(&token, "token", "", "Enrollment token (required)")
	Command.Flags().StringVar(&serverURL, "server", "", "Fleet server URL (required)")
	Command.Flags().StringVar(&dataDir, "data-dir", "", "Data directory override")
	Command.Flags().BoolVar(&insecure, "insecure", false, "Skip TLS certificate verification")
	Command.MarkFlagRequired("token")
	Command.MarkFlagRequired("server")
}

func runSetup(cmd *cobra.Command, args []string) error {
	// Step 1: Check elevation
	if !sys.IsElevated() {
		return fmt.Errorf("fibratus setup must be run as Administrator")
	}
	fmt.Println("[1/6] Administrator privileges verified")

	// Step 2: Enroll
	fmt.Println("[2/6] Enrolling agent...")
	resp, err := enroll.Enroll(enroll.EnrollOpts{
		Token:       token,
		ServerURL:   serverURL,
		DataDir:     dataDir,
		InsecureTLS: insecure,
	})
	if err != nil {
		return fmt.Errorf("enrollment failed: %w", err)
	}
	resolvedDir := enroll.ResolveDataDir(dataDir)
	fmt.Printf("       Agent enrolled: %s\n", resp.AgentID)

	// Step 3: ACL lockdown
	fmt.Println("[3/6] Securing data directory...")
	if err := tamper.LockdownDir(resolvedDir); err != nil {
		return fmt.Errorf("ACL lockdown failed: %w", err)
	}

	// Step 4: Generate integrity seal
	fmt.Println("[4/6] Generating integrity seal...")
	if err := tamper.GenerateSeal(resolvedDir); err != nil {
		return fmt.Errorf("seal generation failed: %w", err)
	}

	// Step 5: Install service with recovery options
	fmt.Println("[5/6] Installing Windows service...")
	err = service.InstallService(service.InstallServiceOpts{WithRecovery: true})
	if err != nil && err != service.ErrServiceAlreadyInstalled {
		return fmt.Errorf("service installation failed: %w", err)
	}
	if err == service.ErrServiceAlreadyInstalled {
		fmt.Println("       Service already installed, skipping")
	}

	// Step 6: Start service
	fmt.Println("[6/6] Starting service...")
	if err := service.StartService(); err != nil {
		return fmt.Errorf("service start failed: %w", err)
	}

	fmt.Println("")
	fmt.Println("Setup complete!")
	fmt.Printf("  Agent ID:     %s\n", resp.AgentID)
	fmt.Printf("  Organization: %s\n", resp.OrgID)
	fmt.Printf("  Server:       %s\n", serverURL)
	fmt.Printf("  Data:         %s (locked + sealed)\n", resolvedDir)
	fmt.Println("")

	return nil
}
