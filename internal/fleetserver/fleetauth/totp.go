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

package fleetauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	totpDigits = 6
	totpPeriod = 30
	totpWindow = 1 // allow +/-1 time step
)

// GenerateTOTPSecret creates a random 20-byte secret, base32-encoded.
func GenerateTOTPSecret() (string, error) {
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret), nil
}

// TOTPProvisioningURI builds an otpauth:// URI for authenticator apps.
func TOTPProvisioningURI(secret, email, issuer string) string {
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=%d&period=%d",
		issuer, email, secret, issuer, totpDigits, totpPeriod)
}

// ValidateTOTP checks if the code matches the secret for the current time
// (with a +/-1 period window to account for clock drift).
func ValidateTOTP(secret, code string) bool {
	secretBytes, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return false
	}
	now := time.Now().Unix()
	for i := -totpWindow; i <= totpWindow; i++ {
		counter := uint64((now / totpPeriod) + int64(i))
		if generateTOTPCode(secretBytes, counter) == code {
			return true
		}
	}
	return false
}

func generateTOTPCode(secret []byte, counter uint64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)
	mac := hmac.New(sha1.New, secret)
	mac.Write(buf)
	hash := mac.Sum(nil)
	offset := hash[len(hash)-1] & 0x0f
	truncated := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff
	code := truncated % uint32(math.Pow10(totpDigits))
	return fmt.Sprintf("%0*d", totpDigits, code)
}

// GenerateRecoveryCodes creates n random 8-character alphanumeric recovery codes.
func GenerateRecoveryCodes(n int) ([]string, error) {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no ambiguous chars (0/O, 1/I)
	codes := make([]string, n)
	for i := range codes {
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		code := make([]byte, 8)
		for j := range code {
			code[j] = chars[int(b[j])%len(chars)]
		}
		codes[i] = string(code[:4]) + "-" + string(code[4:])
	}
	return codes, nil
}
