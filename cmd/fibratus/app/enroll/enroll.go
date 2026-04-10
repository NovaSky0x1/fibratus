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

package enroll

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/rabbitstack/fibratus/pkg/fleet"
	"github.com/rabbitstack/fibratus/pkg/fleet/tamper"
	"github.com/rabbitstack/fibratus/pkg/util/version"
	"github.com/spf13/cobra"
)

var (
	enrollToken string
	serverURL   string
	dataDir     string
	insecureTLS bool
)

// Command is the enroll Cobra command.
var Command = &cobra.Command{
	Use:   "enroll",
	Short: "Enroll this agent with a fleet server",
	Long: `Enroll registers this agent with a fleet management server using a
one-time enrollment token. The server issues a client certificate that
the agent uses for all subsequent communication (mTLS).

The enrollment token is created by an admin in the fleet server dashboard
and is scoped to a specific organization.

Example:
  fibratus enroll --token ft-enroll-abc123... --server https://fleet.acme.com:8443`,
	RunE: runEnroll,
}

func init() {
	Command.Flags().StringVar(&enrollToken, "token", "", "Enrollment token (required)")
	Command.Flags().StringVar(&serverURL, "server", "", "Fleet server URL (required)")
	Command.Flags().StringVar(&dataDir, "data-dir", "", "Data directory for storing certificates")
	Command.Flags().BoolVar(&insecureTLS, "insecure", false, "Skip TLS certificate verification")
	Command.MarkFlagRequired("token")
	Command.MarkFlagRequired("server")
}

// EnrollOpts contains the options for agent enrollment.
type EnrollOpts struct {
	Token       string
	ServerURL   string
	DataDir     string
	InsecureTLS bool
}

// ResolveDataDir returns the data directory path, applying the same
// default logic used during enrollment.
func ResolveDataDir(dataDir string) string {
	if dataDir != "" {
		return dataDir
	}
	exe, err := os.Executable()
	if err != nil {
		exe = "."
	}
	return filepath.Join(filepath.Dir(exe), "..", "data")
}

// Enroll performs the full enrollment workflow: generates a key pair,
// creates a CSR, sends it to the fleet server, and stores the resulting
// certificates and identity data to the data directory.
func Enroll(opts EnrollOpts) (*fleet.EnrollResponse, error) {
	dir := ResolveDataDir(opts.DataDir)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	fmt.Printf("Enrolling agent %s with fleet server %s...\n", hostname, opts.ServerURL)

	// 1. Generate RSA key pair
	fmt.Println("  Generating key pair...")
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	// 2. Create CSR
	csrTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: hostname,
		},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, key)
	if err != nil {
		return nil, fmt.Errorf("failed to create CSR: %w", err)
	}
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	// 3. Send enrollment request
	fmt.Println("  Sending enrollment request...")
	enrollReq := fleet.EnrollRequest{
		Token:         opts.Token,
		Hostname:      hostname,
		OSVersion:     osVersion(),
		EngineVersion: version.Get(),
		CSR:           string(csrPEM),
	}

	reqBody, err := json.Marshal(enrollReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpClient := &http.Client{}
	if opts.InsecureTLS {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: nil,
		}
	}

	url := opts.ServerURL + "/api/v1/enroll"
	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("enrollment request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		var apiResp fleet.Response
		json.Unmarshal(body, &apiResp)
		if apiResp.Error != nil {
			return nil, fmt.Errorf("enrollment failed: %s", apiResp.Error.Message)
		}
		return nil, fmt.Errorf("enrollment failed with status %d: %s", resp.StatusCode, string(body))
	}

	// 4. Parse response
	var apiResp struct {
		Data fleet.EnrollResponse `json:"data"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	enrollResp := apiResp.Data
	if enrollResp.AgentID == "" || enrollResp.SignedCert == "" {
		return nil, fmt.Errorf("server returned incomplete enrollment response")
	}

	// 5. Store enrollment data in DPAPI-encrypted registry
	fmt.Println("  Storing enrollment data (DPAPI-encrypted registry)...")
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	enrollData := &tamper.EnrollmentData{
		ServerURL: opts.ServerURL,
		OrgID:     enrollResp.OrgID,
		AgentID:   enrollResp.AgentID,
		AgentCert: []byte(enrollResp.SignedCert),
		AgentKey:  keyPEM,
		CACert:    []byte(enrollResp.CACert),
	}
	if err := tamper.StoreEnrollment(enrollData); err != nil {
		return nil, fmt.Errorf("failed to store enrollment in registry: %w", err)
	}

	// Protect the registry keys with SYSTEM-only write ACLs
	tamper.ProtectRegistryKeys()

	// Create data directory for operational state (bookmarks, etc.)
	os.MkdirAll(dir, 0o700)

	return &enrollResp, nil
}

func runEnroll(cmd *cobra.Command, args []string) error {
	resp, err := Enroll(EnrollOpts{
		Token:       enrollToken,
		ServerURL:   serverURL,
		DataDir:     dataDir,
		InsecureTLS: insecureTLS,
	})
	if err != nil {
		return err
	}

	resolvedDir := ResolveDataDir(dataDir)
	certDir := filepath.Join(resolvedDir, "certs")

	fmt.Println("")
	fmt.Println("  Enrollment successful!")
	fmt.Println("")
	fmt.Printf("  Agent ID:      %s\n", resp.AgentID)
	fmt.Printf("  Organization:  %s\n", resp.OrgID)
	fmt.Printf("  Server:        %s\n", serverURL)
	fmt.Printf("  Certificates:  %s\n", certDir)
	fmt.Println("")
	fmt.Println("  The agent is now enrolled. Start it with:")
	fmt.Println("    fibratus service start")
	fmt.Println("")
	fmt.Println("  No configuration file changes needed.")

	return nil
}
