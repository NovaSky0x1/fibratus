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

// Package ca provides per-organization Certificate Authority management
// for mTLS agent enrollment. Each organization gets its own CA which
// signs agent client certificates during enrollment.
package ca

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// Manager handles certificate authority operations for organizations.
type Manager struct {
	db *sql.DB
}

// NewManager creates a new CA manager.
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// OrgCA holds a parsed CA certificate and private key for an organization.
type OrgCA struct {
	Cert    *x509.Certificate
	Key     *rsa.PrivateKey
	CertPEM []byte
	KeyPEM  []byte
}

// GetOrCreateCA returns the CA for an organization, creating one if it
// doesn't exist yet. The CA cert/key are stored in the org_cas table.
func (m *Manager) GetOrCreateCA(ctx context.Context, orgID, accountID string) (*OrgCA, error) {
	// Try to load existing CA
	var certPEM, keyPEM []byte
	err := m.db.QueryRowContext(ctx,
		`SELECT ca_cert, ca_key FROM org_cas WHERE org_id = $1`, orgID,
	).Scan(&certPEM, &keyPEM)

	if err == nil {
		return parseCA(certPEM, keyPEM)
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("ca: failed to query org CA: %w", err)
	}

	// Generate new CA
	ca, err := generateCA(orgID, accountID)
	if err != nil {
		return nil, err
	}

	// Persist with a detached context: if the agent's HTTP client times out
	// mid-enrollment, the request ctx is cancelled — but the CA generation
	// has already completed and the row MUST land so the next retry hits
	// the cached CA path instead of regenerating. A 10s budget is plenty
	// for the local Postgres write.
	insertCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = m.db.ExecContext(insertCtx,
		`INSERT INTO org_cas (org_id, ca_cert, ca_key) VALUES ($1, $2, $3)
		 ON CONFLICT (org_id) DO NOTHING`,
		orgID, ca.CertPEM, ca.KeyPEM,
	)
	if err != nil {
		return nil, fmt.Errorf("ca: failed to store org CA: %w", err)
	}

	return ca, nil
}

// SignCSR signs an agent's certificate signing request with the org's CA.
// The resulting certificate embeds the agent identity in the Subject:
//   - CN = agent:<agentID>
//   - OU = org:<orgID>
//   - O  = account:<accountID>
func (m *Manager) SignCSR(ca *OrgCA, csrPEM []byte, agentID, orgID, accountID string) ([]byte, error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, fmt.Errorf("ca: invalid CSR PEM")
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("ca: failed to parse CSR: %w", err)
	}

	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("ca: CSR signature invalid: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("ca: failed to generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:         "agent:" + agentID,
			OrganizationalUnit: []string{"org:" + orgID},
			Organization:       []string{"account:" + accountID},
		},
		NotBefore:             time.Now().Add(-5 * time.Minute),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour), // 1 year
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	// Copy SANs from CSR if present
	template.DNSNames = csr.DNSNames
	template.IPAddresses = csr.IPAddresses

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.Cert, csr.PublicKey, ca.Key)
	if err != nil {
		return nil, fmt.Errorf("ca: failed to sign certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	return certPEM, nil
}

// GetCACert returns the PEM-encoded CA certificate for an organization.
func (m *Manager) GetCACert(ctx context.Context, orgID string) ([]byte, error) {
	var certPEM []byte
	err := m.db.QueryRowContext(ctx,
		`SELECT ca_cert FROM org_cas WHERE org_id = $1`, orgID,
	).Scan(&certPEM)
	if err != nil {
		return nil, fmt.Errorf("ca: no CA found for org %s: %w", orgID, err)
	}
	return certPEM, nil
}

// LoadAllCACerts returns all org CA certificates for configuring the
// server's TLS client CA pool.
func (m *Manager) LoadAllCACerts(ctx context.Context) (*x509.CertPool, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT ca_cert FROM org_cas`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pool := x509.NewCertPool()
	count := 0
	for rows.Next() {
		var certPEM []byte
		if err := rows.Scan(&certPEM); err != nil {
			return nil, err
		}
		if pool.AppendCertsFromPEM(certPEM) {
			count++
		}
	}

	if count == 0 {
		return nil, nil // No CAs yet — mTLS not available
	}
	return pool, rows.Err()
}

// generateCA creates a new self-signed CA certificate and private key.
//
// 2048-bit RSA matches the industry default for internal CAs and generates
// in ~50–200ms — fast enough that the first agent enrollment for a fresh
// org doesn't risk timing out the agent's HTTP client. 4096 bits is overkill
// for a CA that only signs short-lived agent certs and used to take 5–15s on
// small VMs, which was bumping the request past the agent's enroll timeout
// and leaving the CA half-saved (context cancelled).
func generateCA(orgID, accountID string) (*OrgCA, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("ca: keygen failed: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("ca: serial gen failed: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:         fmt.Sprintf("Fibratus Fleet CA - %s", orgID[:8]),
			Organization:       []string{"account:" + accountID},
			OrganizationalUnit: []string{"org:" + orgID},
		},
		NotBefore:             time.Now().Add(-5 * time.Minute),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // 10 years
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("ca: cert creation failed: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("ca: parse cert failed: %w", err)
	}

	return &OrgCA{
		Cert:    cert,
		Key:     key,
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	}, nil
}

// parseCA decodes PEM-encoded CA cert and key from the database.
func parseCA(certPEM, keyPEM []byte) (*OrgCA, error) {
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, fmt.Errorf("ca: invalid cert PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("ca: parse cert: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("ca: invalid key PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("ca: parse key: %w", err)
	}

	return &OrgCA{
		Cert:    cert,
		Key:     key,
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	}, nil
}
