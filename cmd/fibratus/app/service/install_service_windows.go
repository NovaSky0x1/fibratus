/*
 * Copyright 2019-2020 by Nedim Sabic Sabic
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

package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
	"os"
)

const svcName = "fibratus"

// ErrServiceAlreadyInstalled indicates the fibratus service is already installed
var ErrServiceAlreadyInstalled = errors.New("fibratus service is already installed")

var installCommand = &cobra.Command{
	Use:   "install",
	Short: "Install fibratus within the Windows service control manager",
	RunE: func(cmd *cobra.Command, args []string) error {
		return InstallService(InstallServiceOpts{})
	},
}

// InstallServiceOpts configures service installation behavior.
type InstallServiceOpts struct {
	// WithRecovery configures the service to restart on failure (3 attempts, 60s delay).
	WithRecovery bool
}

// InstallService creates the fibratus Windows service. If the service is
// already installed, it returns ErrServiceAlreadyInstalled.
func InstallService(opts InstallServiceOpts) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer func() {
		_ = m.Disconnect()
	}()
	s, err := m.OpenService(svcName)
	if err == nil {
		if err := s.Close(); err != nil {
			return err
		}
		return ErrServiceAlreadyInstalled
	}

	svccfg := mgr.Config{
		DisplayName: "Fibratus Service",
		Description: "Adversary tradecraft detection, protection, and hunting",
		StartType:   mgr.StartAutomatic,
	}
	s, err = m.CreateService(svcName, exe, svccfg)
	if err != nil {
		return err
	}
	defer func() {
		_ = s.Close()
	}()

	err = eventlog.InstallAsEventCreate(svcName, eventlog.Error|eventlog.Warning|eventlog.Info)
	if err != nil {
		if err := s.Delete(); err != nil {
			return err
		}
		return fmt.Errorf("couldn't create event log record: %v", err)
	}

	if opts.WithRecovery {
		recoveryActions := []mgr.RecoveryAction{
			{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
			{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
			{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
		}
		if err := s.SetRecoveryActions(recoveryActions, 86400); err != nil {
			return fmt.Errorf("couldn't set recovery actions: %v", err)
		}
	}

	return nil
}
