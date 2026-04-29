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

	// SIGMA-converted rules (synced via sigmahq):
	"Rare Remote Thread Creation By Uncommon Source Image", // legitimate AV / EDR / debuggers
	"Potential Defense Evasion Via Rename Of Highly Relevant Binaries", // patch installers, msiexec
	"Portable Gpg.EXE Execution",                          // gpg.exe in non-system paths
	"Potential CobaltStrike Service Installations - Registry", // generic service-install pattern
	"Suspicious Process Created Via Wmic.EXE",             // sysadmin tooling
	"Potential WinAPI Calls Via CommandLine",              // PowerShell scripting
	"Scheduled TaskCache Change by Uncommon Program",      // installer-driven scheduled-task creation
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
	`, NoisyDefaultDisabledRules)
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
