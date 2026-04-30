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

package handler

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
	log "github.com/sirupsen/logrus"
)

// NoisyDefaultDisabledRules is the canonical list of detection rules that are
// known-noisy on real Windows endpoints — they fire on legitimate behaviour
// (Windows Update, Edge, Defender, package managers, AV products, IT tooling)
// at a rate that drowns out real signal in a brand-new deployment.
//
// Rules in this list are still imported / synced like any other so an operator
// can enable them after they've audited their fleet. They just start
// disabled with user_disabled=true so the noisy-rule report is empty at
// first sign-in. The operator's own enable/disable choices (tracked via
// user_modified=true) are never overridden.
//
// Sourced from the top-firing-rule report on the production fleet —
// see docs/fleet/rules.md for the methodology.
var NoisyDefaultDisabledRules = []string{
	// Upstream Fibratus rules (rules/*.yml):
	"Suspicious object symbolic link creation",            // Edge, Windows Update, every installer
	"Suspicious access to the hosts file",                 // every AV / DNS-redirect product
	"Suspicious access to Windows Credential Manager files", // password managers, browsers
	".NET assembly loaded by unmanaged process",           // every PowerShell / .NET host (wix.exe, vstest, etc)
	"Suspicious Vault client DLL load",                    // Brave/Chrome/Edge/Outlook/Teams all load vaultcli.dll
	"Suspicious child process integrity level",            // any installer / elevated PS spawning DllHost.exe
	"Unusual access to SSH keys",                          // ssh.exe / git running as the user — devs hit constantly
	"Process execution from hollowed memory section",      // wevtutil.exe LoadImage pattern, also Defender
	"File access to SAM database",                         // svchost.exe SysMain (Superfetch) — normal Windows behavior
	"Renamed Schtasks Execution",                          // ngentask.exe (.NET native image gen) and wuaucltcore.exe (Windows Update)
	"Common Autorun Keys Modification",                    // every browser / Edge / Chrome installer
	"Local Accounts Discovery",                            // quser.exe — common admin tooling
	"Compressed File Creation Via Tar.EXE",                // tar.exe ships with modern Windows; devs use routinely
	"Suspicious Execution of Hostname",                    // every Cygwin/MSYS2/build script invokes hostname
	"Weak or Abused Passwords In CLI",                     // pattern matches autoconf alphabet strings (sed/expr)
	"Potential Hidden Directory Creation Via NTFS INDEX_ALLOCATION Stream - CLI", // pattern over-matches autoconf m4 macros
	"Curl Download And Execute Combination",               // matches every dev curl-then-tar/extract sequence
	"Usage Of Web Request Commands And Cmdlets",           // matches every dev/admin curl/wget/Invoke-WebRequest

	// SIGMA-converted rules (synced via sigmahq):
	"Rare Remote Thread Creation By Uncommon Source Image", // legitimate AV / EDR / debuggers
	"Potential Defense Evasion Via Rename Of Highly Relevant Binaries", // patch installers, msiexec
	"Potential Defense Evasion Via Binary Rename",         // wslhost.exe → cmd.exe /C echo.%UserProfile%
	"Portable Gpg.EXE Execution",                          // gpg.exe in non-system paths
	"Potential CobaltStrike Service Installations - Registry", // generic service-install pattern
	"Suspicious Process Created Via Wmic.EXE",             // sysadmin tooling
	"Potential WinAPI Calls Via CommandLine",              // PowerShell scripting
	"Scheduled TaskCache Change by Uncommon Program",      // installer-driven scheduled-task creation
	"Potential Persistence Via New AMSI Providers - Registry", // Atera/Action1/AV legitimately add AMSI providers
	"Windows Shell/Scripting Processes Spawning Suspicious Programs", // any admin script touching net/whoami/reg
}

// SuppressNoisyDefaultRules walks every account/org in the database and flips
// the noisy-by-default rules to enabled=false, user_disabled=true — but only
// where the operator has not explicitly modified the rule themselves
// (user_modified=false). Safe to call repeatedly on every boot.
func SuppressNoisyDefaultRules(ctx context.Context, db *sql.DB) {
	if db == nil || len(NoisyDefaultDisabledRules) == 0 {
		return
	}
	res, err := db.ExecContext(ctx, `
		UPDATE rules
		SET enabled = false,
		    user_disabled = true,
		    updated_at = NOW()
		WHERE name = ANY($1)
		  AND COALESCE(user_modified, false) = false
		  AND (enabled = true OR COALESCE(user_disabled, false) = false)
	`, pq.Array(NoisyDefaultDisabledRules))
	if err != nil {
		log.Warnf("fleet: suppress noisy-default rules: %v", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Infof("fleet: suppressed %d noisy-by-default rule rows (user_modified preserved)", n)
	}
}

// IsNoisyDefault reports whether a rule name is in the suppression list.
// Used by the seed flow to mark freshly imported rules as user_disabled
// from row creation, so they never spend a single second enabled and
// generating noise before the boot-time suppression sweep catches them.
func IsNoisyDefault(name string) bool {
	for _, n := range NoisyDefaultDisabledRules {
		if n == name {
			return true
		}
	}
	return false
}
