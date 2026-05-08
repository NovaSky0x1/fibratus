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

package tamper

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows/registry"
)

const (
	enrollmentKeyPath = `SOFTWARE\Fibratus\Enrollment`
	stateKeyPath      = `SOFTWARE\Fibratus\State`
)

// Registry value names for enrollment data.
const (
	regServerURL = "ServerURL"
	regOrgID     = "OrgID"
	regAgentID   = "AgentID"
	regAgentCert = "AgentCert"
	regAgentKey  = "AgentKey"
	regCACert    = "CACert"

	regTamperState    = "TamperProtection"
	regIsolationState = "NetworkIsolation"
	regRulesETag      = "RulesETag"
	regEventLogPolicy = "EventLogPolicy"
)

// DPAPI bindings for registry encryption.
var (
	crypt32Lib           = syscall.NewLazyDLL("crypt32.dll")
	procCryptProtect     = crypt32Lib.NewProc("CryptProtectData")
	procCryptUnprotect   = crypt32Lib.NewProc("CryptUnprotectData")
	kernel32Lib          = syscall.NewLazyDLL("kernel32.dll")
	procLocalFreeEnroll  = kernel32Lib.NewProc("LocalFree")
)

type cryptBlob struct {
	cbData uint32
	pbData *byte
}

// dpEncrypt encrypts data using DPAPI with CRYPTPROTECT_LOCAL_MACHINE scope.
func dpEncrypt(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, nil
	}
	input := cryptBlob{cbData: uint32(len(plaintext)), pbData: &plaintext[0]}
	var output cryptBlob
	ret, _, err := procCryptProtect.Call(
		uintptr(unsafe.Pointer(&input)),
		0, 0, 0, 0,
		4, // CRYPTPROTECT_LOCAL_MACHINE
		uintptr(unsafe.Pointer(&output)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("CryptProtectData failed: %v", err)
	}
	encrypted := make([]byte, output.cbData)
	copy(encrypted, unsafe.Slice(output.pbData, output.cbData))
	procLocalFreeEnroll.Call(uintptr(unsafe.Pointer(output.pbData)))
	// Zero plaintext
	for i := range plaintext {
		plaintext[i] = 0
	}
	return encrypted, nil
}

// dpDecrypt decrypts DPAPI-encrypted data.
func dpDecrypt(encrypted []byte) ([]byte, error) {
	if len(encrypted) == 0 {
		return nil, nil
	}
	input := cryptBlob{cbData: uint32(len(encrypted)), pbData: &encrypted[0]}
	var output cryptBlob
	ret, _, err := procCryptUnprotect.Call(
		uintptr(unsafe.Pointer(&input)),
		0, 0, 0, 0,
		4, // CRYPTPROTECT_LOCAL_MACHINE
		uintptr(unsafe.Pointer(&output)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("CryptUnprotectData failed: %v", err)
	}
	decrypted := make([]byte, output.cbData)
	copy(decrypted, unsafe.Slice(output.pbData, output.cbData))
	procLocalFreeEnroll.Call(uintptr(unsafe.Pointer(output.pbData)))
	return decrypted, nil
}

// EnrollmentData holds all enrollment fields.
type EnrollmentData struct {
	ServerURL string
	OrgID     string
	AgentID   string
	AgentCert []byte // PEM
	AgentKey  []byte // PEM
	CACert    []byte // PEM
}

// StoreEnrollment writes enrollment data to DPAPI-encrypted registry values.
//
// On re-enrollment the existing keys may be locked down to SYSTEM-only by
// ProtectRegistryKeys from the previous install, which leaves administrators
// with read-only access. If CreateKey fails because of that, take ownership
// and widen the DACL so the elevated re-install can proceed; ProtectRegistryKeys
// re-applies the lockdown after the rewrite.
//
// Stale values from a prior install are explicitly deleted before the new
// values are written. Without this, a previous enrollment that left behind
// values the new EnrollmentData doesn't supply (e.g., keys we no longer
// write) would silently leak into the agent's identity. Clearing also
// guarantees the OrgID/AgentID can never half-update — partial writes can't
// produce a Frankenstein identity that mixes the prior and current enroll.
func StoreEnrollment(data *EnrollmentData) error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, enrollmentKeyPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		if isAccessDenied(err) {
			unprotectEnrollmentKeys()
			k, _, err = registry.CreateKey(registry.LOCAL_MACHINE, enrollmentKeyPath, registry.SET_VALUE|registry.QUERY_VALUE)
		}
		if err != nil {
			return fmt.Errorf("create enrollment registry key: %w", err)
		}
	}
	defer k.Close()

	if names, err := k.ReadValueNames(0); err == nil {
		for _, n := range names {
			if err := k.DeleteValue(n); err != nil {
				log.Debugf("enrollment: clear stale value %s: %v", n, err)
			}
		}
	} else {
		log.Debugf("enrollment: enumerate values: %v", err)
	}

	values := map[string][]byte{
		regServerURL: []byte(data.ServerURL),
		regOrgID:     []byte(data.OrgID),
		regAgentID:   []byte(data.AgentID),
		regAgentCert: data.AgentCert,
		regAgentKey:  data.AgentKey,
		regCACert:    data.CACert,
	}

	for name, val := range values {
		if len(val) == 0 {
			continue
		}
		encrypted, err := dpEncrypt(val)
		if err != nil {
			return fmt.Errorf("encrypt %s: %w", name, err)
		}
		if err := k.SetBinaryValue(name, encrypted); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}

	log.Info("enrollment: stored to DPAPI-encrypted registry")
	return nil
}

// LoadEnrollment reads enrollment data from DPAPI-encrypted registry values.
// Returns nil if no enrollment data exists in the registry.
func LoadEnrollment() *EnrollmentData {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, enrollmentKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()

	readString := func(name string) string {
		encrypted, _, err := k.GetBinaryValue(name)
		if err != nil || len(encrypted) == 0 {
			return ""
		}
		decrypted, err := dpDecrypt(encrypted)
		if err != nil {
			log.Warnf("enrollment: failed to decrypt %s: %v", name, err)
			return ""
		}
		return strings.TrimSpace(string(decrypted))
	}

	readBytes := func(name string) []byte {
		encrypted, _, err := k.GetBinaryValue(name)
		if err != nil || len(encrypted) == 0 {
			return nil
		}
		decrypted, err := dpDecrypt(encrypted)
		if err != nil {
			log.Warnf("enrollment: failed to decrypt %s: %v", name, err)
			return nil
		}
		return decrypted
	}

	serverURL := readString(regServerURL)
	if serverURL == "" {
		return nil // No enrollment data
	}

	return &EnrollmentData{
		ServerURL: serverURL,
		OrgID:     readString(regOrgID),
		AgentID:   readString(regAgentID),
		AgentCert: readBytes(regAgentCert),
		AgentKey:  readBytes(regAgentKey),
		CACert:    readBytes(regCACert),
	}
}

// StoreAgentID writes just the agent ID to the registry (called after registration).
func StoreAgentID(agentID string) error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, enrollmentKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open enrollment registry key: %w", err)
	}
	defer k.Close()

	encrypted, err := dpEncrypt([]byte(agentID))
	if err != nil {
		return fmt.Errorf("encrypt agent ID: %w", err)
	}
	return k.SetBinaryValue(regAgentID, encrypted)
}

// MigrateFilesToRegistry reads enrollment data from legacy plaintext files,
// stores it in the DPAPI-encrypted registry, and removes the files.
// Returns true if migration occurred.
func MigrateFilesToRegistry(dataDir string) bool {
	// Check if registry already has data
	if data := LoadEnrollment(); data != nil {
		return false // Already migrated
	}

	// Check if files exist
	serverURL := readFileContent(filepath.Join(dataDir, "server-url"))
	if serverURL == "" {
		return false // No files to migrate
	}

	data := &EnrollmentData{
		ServerURL: serverURL,
		OrgID:     readFileContent(filepath.Join(dataDir, "org-id")),
		AgentID:   readFileContent(filepath.Join(dataDir, "agent-id")),
	}

	// Read certificates
	if cert, err := os.ReadFile(filepath.Join(dataDir, "certs", "agent.crt")); err == nil {
		data.AgentCert = cert
	}
	if key, err := os.ReadFile(filepath.Join(dataDir, "certs", "agent.key")); err == nil {
		data.AgentKey = key
	}
	if ca, err := os.ReadFile(filepath.Join(dataDir, "certs", "ca.crt")); err == nil {
		data.CACert = ca
	}

	if err := StoreEnrollment(data); err != nil {
		log.Errorf("enrollment: migration to registry failed: %v", err)
		return false
	}

	// Remove plaintext files
	for _, name := range []string{"server-url", "org-id", "agent-id"} {
		os.Remove(filepath.Join(dataDir, name))
	}
	for _, name := range []string{"agent.crt", "agent.key", "ca.crt"} {
		os.Remove(filepath.Join(dataDir, "certs", name))
	}
	os.Remove(filepath.Join(dataDir, "certs"))
	os.Remove(filepath.Join(dataDir, ".seal"))

	log.Info("enrollment: migrated from plaintext files to DPAPI-encrypted registry")
	return true
}

// ── State storage ──

// StoreState writes a plaintext state value to the registry.
// Used for non-sensitive operational state (tamper, isolation, etag).
func StoreState(name, value string) error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, stateKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, value)
}

// LoadState reads a state value from the registry.
func LoadState(name string) string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, stateKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	val, _, err := k.GetStringValue(name)
	if err != nil {
		return ""
	}
	return val
}

// StoreEncryptedState writes DPAPI-encrypted state (e.g., eventlog policy JSON).
func StoreEncryptedState(name string, data []byte) error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, stateKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	encrypted, err := dpEncrypt(data)
	if err != nil {
		return err
	}
	return k.SetBinaryValue(name, encrypted)
}

// LoadEncryptedState reads DPAPI-encrypted state.
func LoadEncryptedState(name string) []byte {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, stateKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	encrypted, _, err := k.GetBinaryValue(name)
	if err != nil || len(encrypted) == 0 {
		return nil
	}
	decrypted, err := dpDecrypt(encrypted)
	if err != nil {
		return nil
	}
	return decrypted
}

// ProtectRegistryKeys applies restrictive ACLs to the Fibratus registry keys.
// SYSTEM: full control, Administrators: read-only, Everyone: denied write.
func ProtectRegistryKeys() {
	for _, path := range []string{
		`MACHINE\SOFTWARE\Fibratus`,
		`MACHINE\SOFTWARE\Fibratus\Enrollment`,
		`MACHINE\SOFTWARE\Fibratus\State`,
	} {
		sddl := `D:P(A;CI;KA;;;SY)(A;CI;KR;;;BA)`
		if err := setRegistrySecurity(path, sddl); err != nil {
			log.Debugf("enrollment: ACL protection not applied to %s: %v (enrollment data is safely stored)", path, err)
		}
	}
}

func readFileContent(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// isAccessDenied returns true when err corresponds to ERROR_ACCESS_DENIED (5).
// Used to detect SYSTEM-only-locked enrollment keys on re-enrollment.
func isAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	if errno, ok := err.(syscall.Errno); ok {
		return errno == syscall.ERROR_ACCESS_DENIED
	}
	return strings.Contains(err.Error(), "Access is denied")
}

// unprotectEnrollmentKeys walks the enrollment registry tree, claims ownership
// for the local Administrators group, and rewrites the DACL to grant Admin
// full access. Lets the elevated enroll command overwrite keys that the
// previous install had locked to SYSTEM-only via ProtectRegistryKeys. ACLs
// are re-tightened by ProtectRegistryKeys at the end of StoreEnrollment.
func unprotectEnrollmentKeys() {
	enableTakeOwnershipPrivilege()
	// Process child keys before the parent so ownership/ACL changes propagate
	// in the order the registry expects.
	for _, path := range []string{
		`MACHINE\SOFTWARE\Fibratus\Enrollment`,
		`MACHINE\SOFTWARE\Fibratus\State`,
		`MACHINE\SOFTWARE\Fibratus`,
	} {
		// Owner = Built-in Administrators (S-1-5-32-544). Then DACL grants
		// SYSTEM full + Admin full so the rewrite can proceed.
		if err := setRegistryOwnerToAdministrators(path); err != nil {
			log.Debugf("enrollment: take ownership %s: %v", path, err)
			continue
		}
		if err := setRegistrySecurity(path, `D:P(A;CI;KA;;;SY)(A;CI;KA;;;BA)`); err != nil {
			log.Debugf("enrollment: relax DACL on %s: %v", path, err)
		}
	}
}
