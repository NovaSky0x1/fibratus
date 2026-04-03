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
	"io/fs"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// sddl grants Full Control to SYSTEM and Administrators only.
// D:P  = DACL present, Protected (blocks inheritance from parent).
// A    = Allow ACE.
// OICI = Object Inherit + Container Inherit (propagate to children).
// FA   = FILE_ALL_ACCESS (full control).
// SY   = NT AUTHORITY\SYSTEM (S-1-5-18).
// BA   = BUILTIN\Administrators (S-1-5-32-544).
const sddl = "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

// LockdownDir sets restrictive NTFS ACLs on dir and all children.
// Only SYSTEM and Administrators retain access; inherited permissions
// are broken so parent directory ACLs do not weaken the lockdown.
func LockdownDir(dir string) error {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("parse SDDL: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("extract DACL: %w", err)
	}

	// Apply to the root directory itself.
	if err := applyDACL(dir, dacl); err != nil {
		return fmt.Errorf("lockdown %s: %w", dir, err)
	}

	// Walk children and apply the same DACL explicitly.
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		if err := applyDACL(path, dacl); err != nil {
			return fmt.Errorf("lockdown %s: %w", path, err)
		}
		return nil
	})
}

// applyDACL sets the given DACL on path using SetNamedSecurityInfo.
// PROTECTED_DACL_SECURITY_INFORMATION prevents inheritance from parents.
func applyDACL(path string, dacl *windows.ACL) error {
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	)
}
