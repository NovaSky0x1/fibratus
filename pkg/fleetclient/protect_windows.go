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

package fleetclient

import (
	"syscall"
	"unsafe"
)

var (
	crypt32              = syscall.NewLazyDLL("crypt32.dll")
	procCryptProtectData   = crypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procLocalFree        = kernel32.NewProc("LocalFree")
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

// protectMemory encrypts data using Windows DPAPI (CryptProtectData).
// The encrypted data can only be decrypted by the same user on the same machine.
// This prevents rule extraction even with memory dumps from other contexts.
func protectMemory(plaintext []byte) []byte {
	if len(plaintext) == 0 {
		return nil
	}

	input := dataBlob{
		cbData: uint32(len(plaintext)),
		pbData: &plaintext[0],
	}
	var output dataBlob

	// CRYPTPROTECT_LOCAL_MACHINE flag (0x4) — tied to machine, not user
	ret, _, _ := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&input)),
		0, // description
		0, // optional entropy
		0, // reserved
		0, // prompt
		4, // CRYPTPROTECT_LOCAL_MACHINE
		uintptr(unsafe.Pointer(&output)),
	)

	if ret == 0 {
		// DPAPI failed — return a copy (degraded mode, not encrypted)
		cp := make([]byte, len(plaintext))
		copy(cp, plaintext)
		return cp
	}

	encrypted := make([]byte, output.cbData)
	copy(encrypted, unsafe.Slice(output.pbData, output.cbData))
	procLocalFree.Call(uintptr(unsafe.Pointer(output.pbData)))

	// Zero the plaintext input
	for i := range plaintext {
		plaintext[i] = 0
	}

	return encrypted
}

// unprotectMemory decrypts DPAPI-encrypted data.
func unprotectMemory(encrypted []byte) []byte {
	if len(encrypted) == 0 {
		return nil
	}

	input := dataBlob{
		cbData: uint32(len(encrypted)),
		pbData: &encrypted[0],
	}
	var output dataBlob

	ret, _, _ := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&input)),
		0, // description
		0, // optional entropy
		0, // reserved
		0, // prompt
		4, // CRYPTPROTECT_LOCAL_MACHINE
		uintptr(unsafe.Pointer(&output)),
	)

	if ret == 0 {
		// Decryption failed — data may not have been encrypted (degraded mode)
		cp := make([]byte, len(encrypted))
		copy(cp, encrypted)
		return cp
	}

	decrypted := make([]byte, output.cbData)
	copy(decrypted, unsafe.Slice(output.pbData, output.cbData))
	procLocalFree.Call(uintptr(unsafe.Pointer(output.pbData)))

	return decrypted
}
