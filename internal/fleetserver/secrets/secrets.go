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

// Package secrets implements server-wide secret encryption backed by AES-256-GCM.
//
// All secrets persisted in Postgres (ClickHouse password, ClickHouse Cloud API
// keys, JWT secret, etc.) are encrypted with a 256-bit master key loaded from
// the FLEET_SECRET_KEY environment variable. The wire format is:
//
//	nonce(12) || ciphertext || gcm_tag(16)
//
// concatenated and stored as raw bytes. Each Encrypt() call generates a fresh
// random nonce; the same plaintext encrypted twice produces different bytes.
//
// Operational notes:
//   - The master key MUST be hex-encoded 32 bytes (64 hex chars) — anything
//     else is rejected at startup.
//   - Losing the master key bricks every encrypted secret. Operators should
//     back it up alongside their database backups.
//   - Rotating the master key requires re-encrypting every row. We expose
//     Rotate() for that, but the MVP does not surface it via API yet.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
)

// Encryptor seals and opens secrets using a fixed master key. Implementations
// must be safe for concurrent use.
type Encryptor interface {
	// Encrypt returns the sealed payload (nonce || ciphertext || tag).
	Encrypt(plaintext []byte) ([]byte, error)
	// Decrypt undoes Encrypt. Errors include corrupted ciphertext, wrong key,
	// and short input.
	Decrypt(sealed []byte) ([]byte, error)
}

// EnvKeyName is the environment variable that supplies the master key.
const EnvKeyName = "FLEET_SECRET_KEY"

// keyBytes is the AES-256 key length in bytes.
const keyBytes = 32

// nonceBytes is the GCM nonce length used by this package.
const nonceBytes = 12

// ErrMasterKeyUnset is returned when FLEET_SECRET_KEY is missing.
var ErrMasterKeyUnset = errors.New("secrets: " + EnvKeyName + " is not set — generate with: openssl rand -hex 32")

// ErrMasterKeyMalformed is returned when the master key cannot be decoded as
// 32 raw bytes from hex.
var ErrMasterKeyMalformed = errors.New("secrets: " + EnvKeyName + " must be 64 hex characters (32 bytes)")

// LoadKeyFromEnv reads and validates the master key from the environment.
// Returns ErrMasterKeyUnset if the var is missing, ErrMasterKeyMalformed if it
// is the wrong length or not hex.
func LoadKeyFromEnv() ([]byte, error) {
	raw := os.Getenv(EnvKeyName)
	if raw == "" {
		return nil, ErrMasterKeyUnset
	}
	return parseHexKey(raw)
}

// LoadKey returns the master key, preferring the FLEET_SECRET_KEY env var and
// falling back to a file at filePath. If the file does not exist, a fresh
// key is generated and written there with mode 0600 — this lets the operator
// run the server with zero configuration on first boot while still keeping
// the key off the YAML config and out of the database.
//
// Operators who want to manage the key themselves should set FLEET_SECRET_KEY
// (e.g. via systemd EnvironmentFile) — the env var always wins.
func LoadKey(filePath string) ([]byte, error) {
	if k, err := LoadKeyFromEnv(); err == nil {
		return k, nil
	} else if !errors.Is(err, ErrMasterKeyUnset) {
		return nil, err
	}
	if filePath == "" {
		return nil, ErrMasterKeyUnset
	}
	data, err := os.ReadFile(filePath)
	if err == nil {
		return parseHexKey(strings.TrimSpace(string(data)))
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("secrets: read %s: %w", filePath, err)
	}
	// First boot — generate and persist.
	key := make([]byte, keyBytes)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("secrets: generate key: %w", err)
	}
	if err := os.WriteFile(filePath, []byte(hex.EncodeToString(key)+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("secrets: write %s: %w", filePath, err)
	}
	log.Warnf("secrets: generated master key at %s — back this file up; losing it bricks every encrypted secret", filePath)
	return key, nil
}

func parseHexKey(raw string) ([]byte, error) {
	key, err := hex.DecodeString(raw)
	if err != nil {
		return nil, ErrMasterKeyMalformed
	}
	if len(key) != keyBytes {
		return nil, ErrMasterKeyMalformed
	}
	return key, nil
}

// New constructs an Encryptor from a 32-byte key. Use LoadKeyFromEnv to obtain
// the key from FLEET_SECRET_KEY.
func New(key []byte) (Encryptor, error) {
	if len(key) != keyBytes {
		return nil, ErrMasterKeyMalformed
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secrets: aes init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secrets: gcm init: %w", err)
	}
	return &gcmEncryptor{gcm: gcm}, nil
}

type gcmEncryptor struct {
	gcm cipher.AEAD
}

func (e *gcmEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, nonceBytes)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("secrets: nonce: %w", err)
	}
	// Seal appends ciphertext+tag to nonce so the result is self-contained.
	return e.gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (e *gcmEncryptor) Decrypt(sealed []byte) ([]byte, error) {
	if len(sealed) < nonceBytes+16 { // nonce + at least the GCM tag
		return nil, errors.New("secrets: ciphertext too short")
	}
	nonce, ct := sealed[:nonceBytes], sealed[nonceBytes:]
	out, err := e.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("secrets: decrypt: %w", err)
	}
	return out, nil
}
