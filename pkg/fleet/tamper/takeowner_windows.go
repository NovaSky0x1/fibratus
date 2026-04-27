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
	"bytes"
	"encoding/binary"
	"unsafe"

	"golang.org/x/sys/windows"
)

// enableTakeOwnershipPrivilege turns on SeTakeOwnershipPrivilege on the
// current process token so subsequent SetNamedSecurityInfo calls can set
// owner on objects we don't currently own. The privilege is present in the
// admin token by default but starts disabled — Windows requires explicit
// AdjustTokenPrivileges before it kicks in.
func enableTakeOwnershipPrivilege() {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(),
		windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
		return
	}
	defer token.Close()

	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil,
		windows.StringToUTF16Ptr("SeTakeOwnershipPrivilege"), &luid); err != nil {
		return
	}

	// Manually pack TOKEN_PRIVILEGES because Tokenprivileges in x/sys/windows
	// is a fixed-size struct with a single LUID slot — fine for our one
	// privilege but easier to write directly here for clarity.
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, uint32(1)) // PrivilegeCount
	_ = binary.Write(&b, binary.LittleEndian, luid)
	_ = binary.Write(&b, binary.LittleEndian, uint32(windows.SE_PRIVILEGE_ENABLED))

	tp := (*windows.Tokenprivileges)(unsafe.Pointer(&b.Bytes()[0]))
	_ = windows.AdjustTokenPrivileges(token, false, tp, uint32(b.Len()), nil, nil)
}

// setRegistryOwnerToAdministrators sets the owner of the named registry key
// to the local Administrators group (S-1-5-32-544). Requires
// SeTakeOwnershipPrivilege to be enabled on the calling token.
func setRegistryOwnerToAdministrators(path string) error {
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_REGISTRY_KEY,
		windows.OWNER_SECURITY_INFORMATION,
		admins, nil, nil, nil,
	)
}
