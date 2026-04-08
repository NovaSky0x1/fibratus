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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rabbitstack/fibratus/pkg/fleet"
	"github.com/rabbitstack/fibratus/pkg/fleet/tamper"
)

// WindowsExecutor executes fleet commands on Windows endpoints.
type WindowsExecutor struct {
	serverURL string
	wfp       *tamper.WFPIsolator
	protector *tamper.Protector
}

// NewWindowsExecutor creates a new Windows command executor.
func NewWindowsExecutor(serverURL string, wfp *tamper.WFPIsolator, protector *tamper.Protector) *WindowsExecutor {
	return &WindowsExecutor{serverURL: serverURL, wfp: wfp, protector: protector}
}

// Execute dispatches and runs a command based on its type.
func (e *WindowsExecutor) Execute(cmd *fleet.Command) (json.RawMessage, error) {
	switch cmd.Type {
	case fleet.CmdIsolate:
		return e.isolate(cmd)
	case fleet.CmdUnisolate:
		return e.unisolate(cmd)
	case fleet.CmdKillProcess:
		return e.killProcess(cmd)
	case fleet.CmdListDir:
		return e.listDirectory(cmd)
	case fleet.CmdGetFile:
		return e.getFile(cmd)
	case fleet.CmdRunCommand:
		return e.runCommand(cmd)
	case fleet.CmdCollectInfo:
		return e.collectInfo(cmd)
	case fleet.CmdUninstall:
		return e.uninstall(cmd)
	case fleet.CmdGetProcesses:
		return e.getProcesses(cmd)
	case fleet.CmdGetNetwork:
		return e.getNetwork(cmd)
	case fleet.CmdGetServices:
		return e.getServices(cmd)
	case fleet.CmdGetDrivers:
		return e.getDrivers(cmd)
	case fleet.CmdGetAutoruns:
		return e.getAutoruns(cmd)
	case fleet.CmdGetSoftware:
		return e.getSoftware(cmd)
	case fleet.CmdGetUsers:
		return e.getUsers(cmd)
	case fleet.CmdGetRegistry:
		return e.getRegistry(cmd)
	case fleet.CmdStartCapture:
		return e.startCapture(cmd)
	case fleet.CmdStopCapture:
		return e.stopCapture(cmd)
	case fleet.CmdYaraScan:
		return e.yaraScan(cmd)
	case fleet.CmdSetTamperProtection:
		return e.setTamperProtection(cmd)
	default:
		return nil, fmt.Errorf("unknown command type: %s", cmd.Type)
	}
}

// isolate blocks all network traffic using WFP (Windows Filtering Platform)
// kernel-level filters. Much harder to bypass than netsh firewall rules.
func (e *WindowsExecutor) isolate(cmd *fleet.Command) (json.RawMessage, error) {
	var payload struct {
		WhitelistIPs []string `json:"whitelist_ips"`
	}
	json.Unmarshal(cmd.Payload, &payload)

	// Resolve fleet server IP
	serverHost := strings.TrimPrefix(e.serverURL, "https://")
	serverHost = strings.TrimPrefix(serverHost, "http://")
	serverHost = strings.Split(serverHost, ":")[0]
	serverHost = strings.Split(serverHost, "/")[0]

	if err := e.wfp.Isolate(serverHost, payload.WhitelistIPs); err != nil {
		return nil, fmt.Errorf("wfp isolate: %v", err)
	}

	// Persist isolation state
	exe, _ := os.Executable()
	dataDir := filepath.Join(filepath.Dir(exe), "..", "data")
	os.WriteFile(filepath.Join(dataDir, "isolation-state"), []byte("isolated"), 0o600)

	result, _ := json.Marshal(map[string]interface{}{
		"isolated":      true,
		"method":        "wfp",
		"server":        serverHost,
		"whitelist_ips": payload.WhitelistIPs,
	})
	return result, nil
}

// unisolate removes WFP isolation filters, restoring normal network access.
func (e *WindowsExecutor) unisolate(cmd *fleet.Command) (json.RawMessage, error) {
	if err := e.wfp.Unisolate(); err != nil {
		return nil, fmt.Errorf("wfp unisolate: %v", err)
	}

	// Clear persisted isolation state
	exe, _ := os.Executable()
	dataDir := filepath.Join(filepath.Dir(exe), "..", "data")
	os.Remove(filepath.Join(dataDir, "isolation-state"))

	result, _ := json.Marshal(map[string]interface{}{
		"isolated": false,
		"method":   "wfp",
	})
	return result, nil
}

// setTamperProtection enables or disables tamper protection.
func (e *WindowsExecutor) setTamperProtection(cmd *fleet.Command) (json.RawMessage, error) {
	var payload struct {
		Enabled bool `json:"enabled"`
	}
	json.Unmarshal(cmd.Payload, &payload)

	if e.protector == nil {
		return nil, fmt.Errorf("tamper protector not initialized")
	}

	var err error
	if payload.Enabled {
		err = e.protector.EnableProtection()
	} else {
		err = e.protector.DisableProtection()
	}
	if err != nil {
		return nil, fmt.Errorf("tamper protection: %v", err)
	}

	result, _ := json.Marshal(map[string]interface{}{
		"tamper_protection": payload.Enabled,
	})
	return result, nil
}

// killProcess terminates a process by PID.
func (e *WindowsExecutor) killProcess(cmd *fleet.Command) (json.RawMessage, error) {
	var payload struct {
		PID  int    `json:"pid"`
		Name string `json:"name"`
	}
	json.Unmarshal(cmd.Payload, &payload)

	if payload.PID == 0 && payload.Name == "" {
		return nil, fmt.Errorf("pid or name required")
	}

	var out string
	var err error

	if payload.PID > 0 {
		out, err = runCmd("taskkill", "/F", "/PID", fmt.Sprintf("%d", payload.PID))
	} else {
		out, err = runCmd("taskkill", "/F", "/IM", payload.Name)
	}

	if err != nil {
		return nil, fmt.Errorf("kill failed: %s: %v", out, err)
	}

	result, _ := json.Marshal(map[string]interface{}{
		"killed":  true,
		"pid":     payload.PID,
		"name":    payload.Name,
		"message": strings.TrimSpace(out),
	})
	return result, nil
}

// listDirectory returns the contents of a directory.
func (e *WindowsExecutor) listDirectory(cmd *fleet.Command) (json.RawMessage, error) {
	var payload struct {
		Path string `json:"path"`
	}
	json.Unmarshal(cmd.Payload, &payload)

	if payload.Path == "" {
		payload.Path = "C:\\"
	}

	entries, err := os.ReadDir(payload.Path)
	if err != nil {
		return nil, fmt.Errorf("readdir %s: %v", payload.Path, err)
	}

	type fileEntry struct {
		Name    string `json:"name"`
		IsDir   bool   `json:"is_dir"`
		Size    int64  `json:"size"`
		ModTime string `json:"mod_time"`
	}

	files := make([]fileEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileEntry{
			Name:    entry.Name(),
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format(time.RFC3339),
		})
	}

	result, _ := json.Marshal(map[string]interface{}{
		"path":  payload.Path,
		"count": len(files),
		"files": files,
	})
	return result, nil
}

// getFile reads a file and returns its contents (base64 for binary, raw for text).
// Limited to 10MB.
func (e *WindowsExecutor) getFile(cmd *fleet.Command) (json.RawMessage, error) {
	var payload struct {
		Path string `json:"path"`
	}
	json.Unmarshal(cmd.Payload, &payload)

	if payload.Path == "" {
		return nil, fmt.Errorf("path required")
	}

	info, err := os.Stat(payload.Path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %v", payload.Path, err)
	}

	if info.Size() > 10<<20 {
		return nil, fmt.Errorf("file too large: %d bytes (max 10MB)", info.Size())
	}

	f, err := os.Open(payload.Path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %v", payload.Path, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %v", payload.Path, err)
	}

	result, _ := json.Marshal(map[string]interface{}{
		"path":     payload.Path,
		"size":     info.Size(),
		"mod_time": info.ModTime().Format(time.RFC3339),
		"content":  data, // JSON will base64-encode []byte
	})
	return result, nil
}

// runCommand executes a shell command and returns its output.
func (e *WindowsExecutor) runCommand(cmd *fleet.Command) (json.RawMessage, error) {
	var payload struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"` // seconds, default 30
	}
	json.Unmarshal(cmd.Payload, &payload)

	if payload.Command == "" {
		return nil, fmt.Errorf("command required")
	}
	if payload.Timeout <= 0 {
		payload.Timeout = 30
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(payload.Timeout)*time.Second)
	defer cancel()

	c := exec.CommandContext(ctx, "cmd", "/C", payload.Command)
	output, err := c.CombinedOutput()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("exec failed: %v", err)
		}
	}

	result, _ := json.Marshal(map[string]interface{}{
		"command":   payload.Command,
		"output":    string(output),
		"exit_code": exitCode,
	})
	return result, nil
}

// collectInfo gathers system information from the endpoint.
func (e *WindowsExecutor) collectInfo(cmd *fleet.Command) (json.RawMessage, error) {
	hostname, _ := os.Hostname()

	// Get system info
	systemInfo, _ := runCmd("systeminfo", "/FO", "CSV")

	// Get running processes
	processes, _ := runCmd("tasklist", "/FO", "CSV", "/NH")

	// Get network connections
	netstat, _ := runCmd("netstat", "-ano")

	// Get services
	services, _ := runCmd("sc", "query", "type=", "service", "state=", "all")

	// Get installed software
	software, _ := runCmd("wmic", "product", "get", "Name,Version", "/format:csv")

	// Get IP configuration
	ipconfig, _ := runCmd("ipconfig", "/all")

	// Get logged in users
	users, _ := runCmd("query", "user")

	result, _ := json.Marshal(map[string]interface{}{
		"hostname":     hostname,
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
		"cpus":         runtime.NumCPU(),
		"system_info":  truncate(systemInfo, 5000),
		"processes":    truncate(processes, 10000),
		"network":      truncate(netstat, 5000),
		"services":     truncate(services, 10000),
		"software":     truncate(software, 5000),
		"ip_config":    truncate(ipconfig, 3000),
		"logged_users": truncate(users, 1000),
	})
	return result, nil
}

// uninstall removes Fibratus completely from the endpoint.
// Strategy: force-kill the process first (ETW sessions prevent clean stop),
// then remove the service, run MSI uninstall, and clean up remaining files.
func (e *WindowsExecutor) uninstall(cmd *fleet.Command) (json.RawMessage, error) {
	exe, _ := os.Executable()
	installDir := filepath.Dir(filepath.Dir(exe)) // C:\Program Files\Fibratus
	dataDir := filepath.Join(installDir, "data")
	method := "manual"

	// 1. Force-kill the fibratus process (ourselves) in a detached process
	//    so the uninstall can proceed. Use a PowerShell script that:
	//    - Kills fibratus
	//    - Deletes the service
	//    - Runs MSI uninstall if available
	//    - Cleans up remaining files
	cleanupScript := fmt.Sprintf(`
Start-Sleep 2
Stop-Process -Name fibratus -Force -ErrorAction SilentlyContinue
Start-Sleep 1
sc.exe delete fibratus 2>$null
$pc = (Get-ChildItem "HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall" -EA 0 | Where-Object { $_.GetValue("DisplayName") -like "*Fibratus*" }).PSChildName
if ($pc) { msiexec /x $pc /quiet /norestart 2>$null; Start-Sleep 3 }
Remove-Item -Recurse -Force "%s" -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force "%s" -ErrorAction SilentlyContinue
# Remove PATH entry
$path = [Environment]::GetEnvironmentVariable("PATH", "Machine")
$newPath = ($path -split ";" | Where-Object { $_ -notlike "*Fibratus*" }) -join ";"
[Environment]::SetEnvironmentVariable("PATH", $newPath, "Machine")
`, strings.ReplaceAll(installDir, `\`, `\\`), strings.ReplaceAll(dataDir, `\`, `\\`))

	// Write cleanup script to temp
	scriptPath := filepath.Join(os.TempDir(), "fibratus-uninstall.ps1")
	os.WriteFile(scriptPath, []byte(cleanupScript), 0o644)

	// Launch cleanup script detached — it will kill us and clean up
	c := exec.Command("powershell", "-NoProfile", "-NonInteractive",
		"-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass",
		"-File", scriptPath)
	c.Start() // fire and forget — don't wait

	result, _ := json.Marshal(map[string]interface{}{
		"uninstalled": true,
		"method":      method,
		"message":     "Uninstall initiated. Cleanup script will force-kill, remove service, run MSI uninstall, and delete files.",
	})
	return result, nil
}

// getProcesses returns structured process information using a single WMI query.
func (e *WindowsExecutor) getProcesses(cmd *fleet.Command) (json.RawMessage, error) {
	// Single WMI call gets PID, name, cmdline, path, memory in one pass — no per-process lookups
	psCmd := `Get-CimInstance Win32_Process | Select-Object ProcessId,Name,CommandLine,ExecutablePath,@{N='mem_mb';E={[math]::Round($_.WorkingSet64/1MB,1)}},@{N='username';E={try{$o=$_.GetOwner();if($o.User){"$($o.Domain)\$($o.User)"}else{''}}catch{''}}} | ConvertTo-Json -Depth 2 -Compress`
	out, err := runPowerShellLong(psCmd, 30)
	if err != nil {
		return nil, fmt.Errorf("get_processes: %v: %s", err, out)
	}
	result, _ := json.Marshal(map[string]interface{}{
		"processes": safeJSON(out),
	})
	return result, nil
}

// getNetwork returns structured network connection information.
func (e *WindowsExecutor) getNetwork(cmd *fleet.Command) (json.RawMessage, error) {
	// Build process lookup hash once, then select with it — no per-connection lookups
	// State enum must be cast to string — PowerShell 5.1 serializes it as integer
	psCmd := `$p=@{};Get-Process|%{$p[$_.Id]=$_.ProcessName};@(Get-NetTCPConnection -EA 0|Select LocalAddress,LocalPort,RemoteAddress,RemotePort,@{N='State';E={$_.State.ToString()}},OwningProcess,@{N='process_name';E={$p[[int]$_.OwningProcess]}})|ConvertTo-Json -Depth 2 -Compress`
	out, err := runPowerShellLong(psCmd, 15)
	if err != nil {
		return nil, fmt.Errorf("get_network: %v: %s", err, out)
	}
	result, _ := json.Marshal(map[string]interface{}{
		"connections": safeJSON(out),
	})
	return result, nil
}

// getServices returns structured Windows service information.
func (e *WindowsExecutor) getServices(cmd *fleet.Command) (json.RawMessage, error) {
	psCmd := `Get-CimInstance Win32_Service|Select Name,DisplayName,State,StartMode,@{N='account';E={$_.StartName}},PathName,ProcessId|ConvertTo-Json -Depth 2 -Compress`
	out, err := runPowerShellLong(psCmd, 20)
	if err != nil {
		return nil, fmt.Errorf("get_services: %v: %s", err, out)
	}
	result, _ := json.Marshal(map[string]interface{}{
		"services": safeJSON(out),
	})
	return result, nil
}

// getDrivers returns loaded kernel driver information.
func (e *WindowsExecutor) getDrivers(cmd *fleet.Command) (json.RawMessage, error) {
	psCmd := `Get-CimInstance Win32_SystemDriver|Select Name,DisplayName,State,StartMode,PathName,ServiceType|ConvertTo-Json -Depth 2 -Compress`
	out, err := runPowerShellLong(psCmd, 20)
	if err != nil {
		return nil, fmt.Errorf("get_drivers: %v: %s", err, out)
	}
	result, _ := json.Marshal(map[string]interface{}{
		"drivers": safeJSON(out),
	})
	return result, nil
}

// getAutoruns returns persistence mechanisms (Run keys, scheduled tasks, startup folder, auto-start services).
func (e *WindowsExecutor) getAutoruns(cmd *fleet.Command) (json.RawMessage, error) {
	// Registry Run keys
	runKeysCmd := `$keys = @(); foreach ($path in @('HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Run','HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\RunOnce','HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Run','HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\RunOnce')) { try { $props = Get-ItemProperty $path -ErrorAction SilentlyContinue; if ($props) { $props.PSObject.Properties | Where-Object { $_.Name -notlike 'PS*' } | ForEach-Object { $keys += @{name=$_.Name;value=$_.Value;location=$path} } } } catch {} }; $keys | ConvertTo-Json -Depth 3 -Compress`
	runKeys, _ := runPowerShellLong(runKeysCmd, 15)

	// Scheduled tasks
	tasksCmd := `@(Get-ScheduledTask -EA 0|?{$_.State -eq 'Ready' -and $_.TaskPath -notlike '\Microsoft\*'}|Select TaskName,TaskPath,State,@{N='action';E={($_.Actions|Select -First 1).Execute}})|ConvertTo-Json -Depth 2 -Compress`
	tasks, _ := runPowerShellLong(tasksCmd, 15)

	// Startup folder
	startupCmd := `$items = @(); foreach ($dir in @("$env:ProgramData\Microsoft\Windows\Start Menu\Programs\StartUp","$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Startup")) { Get-ChildItem $dir -ErrorAction SilentlyContinue | ForEach-Object { $items += @{name=$_.Name;path=$_.FullName;location=$dir} } }; $items | ConvertTo-Json -Depth 3 -Compress`
	startup, _ := runPowerShellLong(startupCmd, 10)

	result, _ := json.Marshal(map[string]interface{}{
		"run_keys":       safeJSON(runKeys),
		"scheduled_tasks": safeJSON(tasks),
		"startup_folder": safeJSON(startup),
	})
	return result, nil
}

// getSoftware returns installed software from the registry.
func (e *WindowsExecutor) getSoftware(cmd *fleet.Command) (json.RawMessage, error) {
	psCmd := `$apps = @(); foreach ($path in @('HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*','HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*')) { Get-ItemProperty $path -ErrorAction SilentlyContinue | Where-Object { $_.DisplayName } | ForEach-Object { $apps += @{name=$_.DisplayName;version=$_.DisplayVersion;publisher=$_.Publisher;install_date=$_.InstallDate;size_mb=[math]::Round($_.EstimatedSize/1024,1)} } }; $apps | Sort-Object { $_.name } | ConvertTo-Json -Depth 3 -Compress`
	out, err := runPowerShellLong(psCmd, 30)
	if err != nil {
		return nil, fmt.Errorf("get_software: %v: %s", err, out)
	}
	result, _ := json.Marshal(map[string]interface{}{
		"software": safeJSON(out),
	})
	return result, nil
}

// getUsers returns local user accounts, active sessions, and admin group members.
func (e *WindowsExecutor) getUsers(cmd *fleet.Command) (json.RawMessage, error) {
	usersCmd := `Get-LocalUser | Select-Object Name,Enabled,LastLogon,Description | ConvertTo-Json -Depth 3 -Compress`
	users, _ := runPowerShellLong(usersCmd, 10)

	adminsCmd := `Get-LocalGroupMember -Group Administrators -ErrorAction SilentlyContinue | Select-Object Name,ObjectClass,PrincipalSource | ConvertTo-Json -Depth 3 -Compress`
	admins, _ := runPowerShellLong(adminsCmd, 10)

	sessions, _ := runCmd("query", "user")

	result, _ := json.Marshal(map[string]interface{}{
		"local_users": safeJSON(users),
		"admin_group": safeJSON(admins),
		"sessions":    strings.TrimSpace(sessions),
	})
	return result, nil
}

// getRegistry returns registry keys and values at a given path.
func (e *WindowsExecutor) getRegistry(cmd *fleet.Command) (json.RawMessage, error) {
	var payload struct {
		Path string `json:"path"`
	}
	json.Unmarshal(cmd.Payload, &payload)
	if payload.Path == "" {
		payload.Path = "HKLM:\\SOFTWARE"
	}

	// Get subkeys
	// @() forces array output even for single results
	keysCmd := fmt.Sprintf(`@(Get-ChildItem '%s' -EA 0|Select PSChildName,@{N='subkey_count';E={@(Get-ChildItem $_.PSPath -EA 0).Count}})|ConvertTo-Json -Depth 2 -Compress`, payload.Path)
	keys, _ := runPowerShellLong(keysCmd, 15)

	// Get values at this path
	valsCmd := fmt.Sprintf(`$p=Get-ItemProperty '%s' -EA 0;if($p){@($p.PSObject.Properties|?{$_.Name -notlike 'PS*'}|%%{@{name=$_.Name;value=($_.Value|Out-String).Trim();type=$_.TypeNameOfValue}})|ConvertTo-Json -Depth 2 -Compress}else{'[]'}`, payload.Path)
	vals, _ := runPowerShellLong(valsCmd, 15)

	result, _ := json.Marshal(map[string]interface{}{
		"path":   payload.Path,
		"keys":   safeJSON(keys),
		"values": safeJSON(vals),
	})
	return result, nil
}

// startCapture starts a kernel event capture in the background.
func (e *WindowsExecutor) startCapture(cmd *fleet.Command) (json.RawMessage, error) {
	var payload struct {
		Filter   string `json:"filter"`
		Duration int    `json:"duration"` // seconds, 0 = indefinite
	}
	json.Unmarshal(cmd.Payload, &payload)

	exe, _ := os.Executable()
	captureDir := filepath.Join(filepath.Dir(exe), "..", "captures")
	os.MkdirAll(captureDir, 0o755)
	capturePath := filepath.Join(captureDir, fmt.Sprintf("capture-%d.kcap", time.Now().Unix()))

	args := []string{"capture", "-o", capturePath}
	if payload.Filter != "" {
		args = append(args, payload.Filter)
	}

	c := exec.Command(exe, args...)
	if err := c.Start(); err != nil {
		return nil, fmt.Errorf("start capture: %v", err)
	}

	result, _ := json.Marshal(map[string]interface{}{
		"started":      true,
		"capture_path": capturePath,
		"pid":          c.Process.Pid,
		"filter":       payload.Filter,
	})
	return result, nil
}

// stopCapture stops a running capture by PID.
func (e *WindowsExecutor) stopCapture(cmd *fleet.Command) (json.RawMessage, error) {
	var payload struct {
		PID int `json:"pid"`
	}
	json.Unmarshal(cmd.Payload, &payload)

	if payload.PID <= 0 {
		return nil, fmt.Errorf("capture pid required")
	}

	out, err := runCmd("taskkill", "/F", "/PID", fmt.Sprintf("%d", payload.PID))
	if err != nil {
		return nil, fmt.Errorf("stop capture: %s: %v", out, err)
	}

	result, _ := json.Marshal(map[string]interface{}{
		"stopped": true,
		"pid":     payload.PID,
		"message": strings.TrimSpace(out),
	})
	return result, nil
}

// yaraScan runs a YARA scan on a process or file.
func (e *WindowsExecutor) yaraScan(cmd *fleet.Command) (json.RawMessage, error) {
	var payload struct {
		PID  int    `json:"pid"`
		Path string `json:"path"`
	}
	json.Unmarshal(cmd.Payload, &payload)

	if payload.PID == 0 && payload.Path == "" {
		return nil, fmt.Errorf("pid or path required for yara scan")
	}

	exe, _ := os.Executable()
	var args []string
	if payload.PID > 0 {
		args = []string{"yara", "--pid", fmt.Sprintf("%d", payload.PID)}
	} else {
		args = []string{"yara", "--path", payload.Path}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	c := exec.CommandContext(ctx, exe, args...)
	out, err := c.CombinedOutput()

	result, _ := json.Marshal(map[string]interface{}{
		"pid":     payload.PID,
		"path":    payload.Path,
		"output":  string(out),
		"error":   errStr(err),
		"success": err == nil,
	})
	return result, nil
}

// runPowerShell runs a PowerShell command with default timeout.
func runPowerShell(psCmd string) (string, error) {
	return runPowerShellLong(psCmd, 30)
}

// runPowerShellLong runs a PowerShell command with a custom timeout.
func runPowerShellLong(psCmd string, timeoutSec int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd)
	out, err := c.CombinedOutput()
	return string(out), err
}

// safeJSON wraps a raw JSON string, returning an empty array if invalid.
// safeJSON wraps raw JSON, ensuring it's always a valid JSON array.
// PowerShell's ConvertTo-Json returns a bare object (not array) when
// there's only one result — this normalizes that to an array.
func safeJSON(s string) json.RawMessage {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		return json.RawMessage("[]")
	}
	if !json.Valid([]byte(s)) {
		return json.RawMessage("[]")
	}
	// If it's a single object (starts with {), wrap in array
	if s[0] == '{' {
		return json.RawMessage("[" + s + "]")
	}
	return json.RawMessage(s)
}

// errStr returns an error string or empty string if nil.
func errStr(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

func runCmd(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, name, args...)
	out, err := c.CombinedOutput()
	return string(out), err
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "\n... (truncated)"
	}
	return s
}
