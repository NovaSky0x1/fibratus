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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// GenerateSeal computes an HMAC-SHA256 seal over the enrollment data
// files in dataDir and writes the result to dataDir/.seal. The HMAC
// key is derived from the Windows machine GUID and the agent private key.
func GenerateSeal(dataDir string) error {
	key, err := deriveKey(dataDir)
	if err != nil {
		return fmt.Errorf("derive seal key: %w", err)
	}
	seal, err := computeSeal(dataDir, key)
	if err != nil {
		return fmt.Errorf("compute seal: %w", err)
	}
	return os.WriteFile(filepath.Join(dataDir, sealFile), []byte(seal), 0o644)
}

// VerifySeal recomputes the HMAC-SHA256 seal and compares it against
// the stored seal in dataDir/.seal. Returns nil if valid.
func VerifySeal(dataDir string) error {
	stored, err := os.ReadFile(filepath.Join(dataDir, sealFile))
	if err != nil {
		return fmt.Errorf("read seal file: %w", err)
	}
	key, err := deriveKey(dataDir)
	if err != nil {
		return fmt.Errorf("derive seal key: %w", err)
	}
	computed, err := computeSeal(dataDir, key)
	if err != nil {
		return fmt.Errorf("compute seal: %w", err)
	}
	storedBytes, err := hex.DecodeString(string(stored))
	if err != nil {
		return fmt.Errorf("decode stored seal: %w", err)
	}
	computedBytes, err := hex.DecodeString(computed)
	if err != nil {
		return fmt.Errorf("decode computed seal: %w", err)
	}
	if !hmac.Equal(storedBytes, computedBytes) {
		return fmt.Errorf("enrollment data integrity check failed: seal mismatch")
	}
	return nil
}

// deriveKey produces the HMAC key: SHA256(MachineGuid + SHA256(agent.key)).
func deriveKey(dataDir string) ([]byte, error) {
	guid, err := machineGUID()
	if err != nil {
		return nil, fmt.Errorf("read machine GUID: %w", err)
	}
	keyData, err := os.ReadFile(filepath.Join(dataDir, "certs", "agent.key"))
	if err != nil {
		return nil, fmt.Errorf("read agent key: %w", err)
	}
	keyHash := sha256.Sum256(keyData)
	combined := sha256.Sum256(append([]byte(guid), keyHash[:]...))
	return combined[:], nil
}

// computeSeal reads the sealed files in fixed order, feeds their
// contents into an HMAC-SHA256 hasher, and returns the hex digest.
func computeSeal(dataDir string, key []byte) (string, error) {
	mac := hmac.New(sha256.New, key)
	for _, name := range sealedFiles {
		data, err := os.ReadFile(filepath.Join(dataDir, name))
		if err != nil {
			return "", fmt.Errorf("read %s: %w", name, err)
		}
		mac.Write(data)
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// machineGUID reads the Windows machine GUID from the registry.
func machineGUID() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()
	val, _, err := k.GetStringValue("MachineGuid")
	return val, err
}
