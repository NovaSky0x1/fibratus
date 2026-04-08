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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Protector manages all tamper protection mechanisms.
type Protector struct {
	mu          sync.Mutex
	enabled     bool
	installDir  string
	dataDir     string
	serviceName string
	watchdogCmd *exec.Cmd
	stopMonitor chan struct{}
}

// NewProtector creates a tamper protection manager.
func NewProtector(installDir, dataDir, serviceName string) *Protector {
	return &Protector{
		installDir:  installDir,
		dataDir:     dataDir,
		serviceName: serviceName,
	}
}

// EnableProtection activates all tamper protections. Idempotent.
func (p *Protector) EnableProtection() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.enabled {
		return nil
	}

	log.Info("tamper: enabling protection")

	// 1. Service DACL hardening
	if err := p.hardenServiceDACL(); err != nil {
		log.Warnf("tamper: service DACL hardening failed: %v", err)
	} else {
		log.Info("tamper: service DACL hardened — stop/delete denied to non-SYSTEM")
	}

	// 2. Process anti-kill DACL
	if err := p.hardenProcessDACL(); err != nil {
		log.Warnf("tamper: process DACL hardening failed: %v", err)
	} else {
		log.Info("tamper: process DACL hardened — terminate denied to non-SYSTEM")
	}

	// 3. Install directory ACLs
	if err := p.lockdownInstallDir(); err != nil {
		log.Warnf("tamper: install dir lockdown failed: %v", err)
	} else {
		log.Info("tamper: install directory locked down — SYSTEM-only write/delete")
	}

	// 4. Hide from Add/Remove Programs
	if err := p.hideUninstallEntry(); err != nil {
		log.Warnf("tamper: hide uninstall entry failed: %v", err)
	} else {
		log.Info("tamper: hidden from Add/Remove Programs")
	}

	// 5. Registry key protection
	if err := p.protectRegistryKeys(); err != nil {
		log.Warnf("tamper: registry protection failed: %v", err)
	} else {
		log.Info("tamper: service registry keys protected")
	}

	// 6. Watchdog
	if err := p.startWatchdog(); err != nil {
		log.Warnf("tamper: watchdog start failed: %v", err)
	} else {
		log.Info("tamper: watchdog process started")
	}

	// 7. Periodic integrity monitor
	p.startIntegrityMonitor()
	log.Info("tamper: integrity monitor started (5min interval)")

	// 8. Persist state
	p.persistState(true)

	p.enabled = true
	log.Info("tamper: all protections enabled")
	return nil
}

// DisableProtection removes all tamper protections. Idempotent.
func (p *Protector) DisableProtection() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled {
		return nil
	}

	log.Info("tamper: disabling protection")

	// Reverse order
	p.stopIntegrityMonitor()
	p.stopWatchdog()
	p.unprotectRegistryKeys()
	p.restoreUninstallEntry()
	p.unlockInstallDir()
	p.restoreProcessDACL()
	p.restoreServiceDACL()

	p.persistState(false)
	p.enabled = false
	log.Info("tamper: all protections disabled")
	return nil
}

// IsEnabled returns the current protection state.
func (p *Protector) IsEnabled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.enabled
}

// LoadPersistedState reads the tamper state from disk and re-applies if needed.
func (p *Protector) LoadPersistedState() {
	data, err := os.ReadFile(filepath.Join(p.dataDir, "tamper-state"))
	if err != nil {
		return
	}
	if strings.TrimSpace(string(data)) == "enabled" {
		log.Info("tamper: persisted state is enabled — re-applying protections")
		p.EnableProtection()
	}
}

func (p *Protector) persistState(enabled bool) {
	state := "disabled"
	if enabled {
		state = "enabled"
	}
	os.WriteFile(filepath.Join(p.dataDir, "tamper-state"), []byte(state), 0o600)
}

// ═══════════════════════════════════════════════════════════
// Layer 1: Service DACL Hardening
// ═══════════════════════════════════════════════════════════

// hardenServiceDACL sets restrictive permissions on the Fibratus service.
// Denies SERVICE_STOP and DELETE to everyone except SYSTEM.
func (p *Protector) hardenServiceDACL() error {
	// SDDL: SYSTEM gets full control, Admins get read-only, Everyone denied stop/delete
	sddl := `D:(A;;RPWPDTLOCRRC;;;SY)(A;;CCLCSWLOCRRC;;;BA)(D;;WPDT;;;WD)`
	out, err := runCmdSilent("sc", "sdset", p.serviceName, sddl)
	if err != nil {
		return fmt.Errorf("sc sdset: %s: %v", out, err)
	}
	return nil
}

func (p *Protector) restoreServiceDACL() {
	// Restore default service DACL: SYSTEM + Admins full control
	sddl := `D:(A;;CCLCSWRPWPDTLOCRRC;;;SY)(A;;CCDCLCSWRPWPDTLOCRSDRCWDWO;;;BA)(A;;CCLCSWLOCRRC;;;IU)(A;;CCLCSWLOCRRC;;;SU)`
	runCmdSilent("sc", "sdset", p.serviceName, sddl)
}

// ═══════════════════════════════════════════════════════════
// Layer 2: Process Anti-Kill DACL
// ═══════════════════════════════════════════════════════════

func (p *Protector) hardenProcessDACL() error {
	// Get current process handle
	handle := windows.CurrentProcess()

	// Build a DACL that denies PROCESS_TERMINATE (0x0001) to Everyone (WD)
	// and grants full access to SYSTEM
	sddl := `D:(D;;0x1;;;WD)(A;;GA;;;SY)`
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("parse process SDDL: %v", err)
	}

	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("get process DACL: %v", err)
	}

	err = windows.SetSecurityInfo(
		handle,
		windows.SE_KERNEL_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
	if err != nil {
		return fmt.Errorf("SetSecurityInfo on process: %v", err)
	}
	return nil
}

func (p *Protector) restoreProcessDACL() {
	handle := windows.CurrentProcess()
	// Restore default — everyone can query, SYSTEM can do everything
	sddl := `D:(A;;GA;;;SY)(A;;GA;;;BA)(A;;0x1000;;;WD)`
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return
	}
	dacl, _, _ := sd.DACL()
	windows.SetSecurityInfo(handle, windows.SE_KERNEL_OBJECT,
		windows.DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

// ═══════════════════════════════════════════════════════════
// Layer 3: Install Directory ACLs
// ═══════════════════════════════════════════════════════════

func (p *Protector) lockdownInstallDir() error {
	// SYSTEM: full control. Admins: read+execute only, deny delete
	sddl := `D:P(A;OICI;FA;;;SY)(A;OICI;0x1200a9;;;BA)(D;OICI;SD;;;BA)`
	return setDirSecurity(p.installDir, sddl)
}

func (p *Protector) unlockInstallDir() {
	// Restore: SYSTEM + Admins full control
	sddl := `D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)`
	setDirSecurity(p.installDir, sddl)
}

func setDirSecurity(dir, sddl string) error {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	dirPtr, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		dirPtr,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
}

// ═══════════════════════════════════════════════════════════
// Layer 4: Hide from Add/Remove Programs
// ═══════════════════════════════════════════════════════════

func (p *Protector) hideUninstallEntry() error {
	path := findUninstallKey()
	if path == "" {
		return fmt.Errorf("uninstall registry key not found")
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetDWordValue("SystemComponent", 1)
}

func (p *Protector) restoreUninstallEntry() {
	path := findUninstallKey()
	if path == "" {
		return
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	k.DeleteValue("SystemComponent")
}

func findUninstallKey() string {
	basePaths := []string{
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`,
		`SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`,
	}
	for _, base := range basePaths {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, base, registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			continue
		}
		names, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, name := range names {
			subPath := base + `\` + name
			sk, err := registry.OpenKey(registry.LOCAL_MACHINE, subPath, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			displayName, _, _ := sk.GetStringValue("DisplayName")
			sk.Close()
			if strings.Contains(strings.ToLower(displayName), "fibratus") {
				return subPath
			}
		}
	}
	return ""
}

// ═══════════════════════════════════════════════════════════
// Layer 5: Registry Key Protection
// ═══════════════════════════════════════════════════════════

func (p *Protector) protectRegistryKeys() error {
	// Protect service registry key
	sddl := `D:P(A;CI;KA;;;SY)(A;CI;KR;;;BA)(D;CI;KW;;;BA)`
	servicePath := `MACHINE\SYSTEM\CurrentControlSet\Services\` + p.serviceName
	return setRegistrySecurity(servicePath, sddl)
}

func (p *Protector) unprotectRegistryKeys() {
	// Restore default ACL
	sddl := `D:(A;CI;KA;;;SY)(A;CI;KA;;;BA)`
	servicePath := `MACHINE\SYSTEM\CurrentControlSet\Services\` + p.serviceName
	setRegistrySecurity(servicePath, sddl)
}

func setRegistrySecurity(path, sddl string) error {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		pathPtr,
		windows.SE_REGISTRY_KEY,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
}

// ═══════════════════════════════════════════════════════════
// Layer 6: Self-Healing Watchdog
// ═══════════════════════════════════════════════════════════

func (p *Protector) startWatchdog() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	pid := os.Getpid()
	cmd := exec.Command(exe, "--watchdog", fmt.Sprintf("--watchdog-pid=%d", pid))
	cmd.SysProcAttr = &windows.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start watchdog: %v", err)
	}

	p.watchdogCmd = cmd
	log.Infof("tamper: watchdog started (PID %d watching PID %d)", cmd.Process.Pid, pid)
	return nil
}

func (p *Protector) stopWatchdog() {
	if p.watchdogCmd != nil && p.watchdogCmd.Process != nil {
		p.watchdogCmd.Process.Kill()
		p.watchdogCmd = nil
	}
}

// RunWatchdog is the watchdog entry point — monitors the parent process
// and restarts the service if it dies.
func RunWatchdog(parentPID int) {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(parentPID))
	if err != nil {
		return
	}
	defer windows.CloseHandle(handle)

	// Wait for parent to die
	windows.WaitForSingleObject(handle, windows.INFINITE)

	// Parent died — restart the service
	time.Sleep(500 * time.Millisecond)
	exec.Command("sc", "start", "fibratus").Start()
}

// ═══════════════════════════════════════════════════════════
// Layer 7: Periodic Integrity Re-verification
// ═══════════════════════════════════════════════════════════

func (p *Protector) startIntegrityMonitor() {
	p.stopMonitor = make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Re-verify seal
				if err := VerifySeal(p.dataDir); err != nil {
					log.Errorf("tamper: INTEGRITY CHECK FAILED: %v — re-applying protections", err)
				}
				// Re-apply ACLs in case they were weakened
				p.lockdownInstallDir()
				p.protectRegistryKeys()
			case <-p.stopMonitor:
				return
			}
		}
	}()
}

func (p *Protector) stopIntegrityMonitor() {
	if p.stopMonitor != nil {
		close(p.stopMonitor)
	}
}

// ═══════════════════════════════════════════════════════════
// Helpers
// ═══════════════════════════════════════════════════════════

func runCmdSilent(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}

// Ensure unsafe is used (for process handle operations)
var _ = unsafe.Sizeof(0)
