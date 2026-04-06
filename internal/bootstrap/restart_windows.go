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

package bootstrap

import (
	"os/exec"
	"syscall"

	log "github.com/sirupsen/logrus"
)

// restartService spawns a detached process that stops and restarts
// the Fibratus Windows service after a brief delay. This allows the
// current process to exit cleanly before the SCM restarts it.
func restartService() {
	cmd := exec.Command("cmd.exe", "/C",
		"timeout /t 3 /nobreak >nul & sc stop fibratus & timeout /t 3 /nobreak >nul & sc start fibratus")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008, // DETACHED_PROCESS
	}
	if err := cmd.Start(); err != nil {
		log.Errorf("fleet: failed to spawn service restart: %v", err)
		return
	}
	log.Infof("fleet: service restart scheduled (pid %d)", cmd.Process.Pid)
}
