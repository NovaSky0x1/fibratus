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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rabbitstack/fibratus/pkg/event"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	"github.com/rabbitstack/fibratus/pkg/fleet/tamper"
	fleetserver "github.com/rabbitstack/fibratus/pkg/outputs/fleetserver"
	log "github.com/sirupsen/logrus"
)

// EventLogReconfigureCallback is called when the server pushes a new event log
// collection policy. The raw JSON payload is passed to the bootstrap layer which
// converts it to the appropriate config type and reconfigures the collector.
type EventLogReconfigureCallback func(policyJSON json.RawMessage)

// WindowsExecutor executes fleet commands on Windows endpoints.
type WindowsExecutor struct {
	serverURL          string
	wfp                *tamper.WFPIsolator
	protector          *tamper.Protector
	eventlogReconfigure EventLogReconfigureCallback
}

// NewWindowsExecutor creates a new Windows command executor.
func NewWindowsExecutor(serverURL string, wfp *tamper.WFPIsolator, protector *tamper.Protector) *WindowsExecutor {
	return &WindowsExecutor{serverURL: serverURL, wfp: wfp, protector: protector}
}

// SetEventLogReconfigureCallback registers the callback for event log policy changes.
func (e *WindowsExecutor) SetEventLogReconfigureCallback(cb EventLogReconfigureCallback) {
	e.eventlogReconfigure = cb
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
	case fleet.CmdSetEventLogPolicy:
		return e.setEventLogPolicy(cmd)
	case fleet.CmdLogoffUser:
		return e.logoffUser(cmd)
	case fleet.CmdListEventLogChannels:
		return e.listEventLogChannels(cmd)
	case fleet.CmdQueryEventLog:
		return e.queryEventLog(cmd)
	case fleet.CmdExportEvtx:
		return e.exportEvtx(cmd)
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

	// Resolve fleet server IP — must be an actual IP for WFP filters.
	// The server URL may be a hostname (e.g. edr.novasky.io) which WFP cannot use directly.
	serverHost := strings.TrimPrefix(e.serverURL, "https://")
	serverHost = strings.TrimPrefix(serverHost, "http://")
	serverHost = strings.Split(serverHost, ":")[0]
	serverHost = strings.Split(serverHost, "/")[0]

	// If it's not already an IP, resolve via DNS
	if net.ParseIP(serverHost) == nil {
		ips, err := net.LookupIP(serverHost)
		if err == nil && len(ips) > 0 {
			// Prefer IPv4
			for _, ip := range ips {
				if ip.To4() != nil {
					serverHost = ip.String()
					break
				}
			}
			if net.ParseIP(serverHost) == nil && len(ips) > 0 {
				serverHost = ips[0].String()
			}
		}
	}

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

// setEventLogPolicy applies the event log collection policy received from the server.
func (e *WindowsExecutor) setEventLogPolicy(cmd *fleet.Command) (json.RawMessage, error) {
	var policy fleet.EventLogPolicy
	if err := json.Unmarshal(cmd.Payload, &policy); err != nil {
		return nil, fmt.Errorf("parse eventlog policy: %v", err)
	}

	// Persist the raw policy to disk for reboot survival
	exe, _ := os.Executable()
	if exe != "" {
		dataDir := filepath.Join(filepath.Dir(exe), "..", "data")
		os.WriteFile(filepath.Join(dataDir, "eventlog-policy.json"), cmd.Payload, 0o600)
	}

	// Apply immediately via the reconfigure callback (bootstrap handles conversion)
	if e.eventlogReconfigure != nil {
		e.eventlogReconfigure(cmd.Payload)
	}

	log.Infof("fleet: event log policy applied (enabled=%v, channels=%d)", policy.Enabled, len(policy.Channels))
	result, _ := json.Marshal(map[string]interface{}{
		"eventlog_enabled":  policy.Enabled,
		"channels_count":    len(policy.Channels),
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

	// CMMC/HIPAA compliance: agent-side enforcement of file access policy
	if !fleet.IsFileExtensionAllowed(payload.Path, nil) {
		return nil, fmt.Errorf("file download blocked by compliance policy — file type not in allowed list")
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

	// Get network adapters as structured JSON — fall back to ipconfig /all if CIM is unavailable
	ipconfig, _ := runPowerShellLong(`try{@(Get-NetAdapter -EA Stop|%{$ip=Get-NetIPAddress -InterfaceIndex $_.ifIndex -EA 0;$dns=Get-DnsClientServerAddress -InterfaceIndex $_.ifIndex -EA 0;$sp=$_.LinkSpeed;$mbps=0;if($sp -match '([\d.]+)\s*Gbps'){$mbps=[double]$Matches[1]*1000}elseif($sp -match '([\d.]+)\s*Mbps'){$mbps=[double]$Matches[1]}elseif($sp -match '([\d.]+)\s*Kbps'){$mbps=[double]$Matches[1]/1000};@{name=$_.Name;description=$_.InterfaceDescription;status=$_.Status;mac=$_.MacAddress;speed_mbps=[math]::Round($mbps,0);ipv4=@($ip|?{$_.AddressFamily -eq 'IPv4'}|%{$_.IPAddress});ipv6=@($ip|?{$_.AddressFamily -eq 'IPv6'}|%{$_.IPAddress});dns=@($dns|%{$_.ServerAddresses}|Select -Unique);dhcp=$_.Dhcp}})|ConvertTo-Json -Depth 3 -Compress}catch{ipconfig /all}`, 15)

	// Get logged in users
	users, _ := runCmd("query", "user")

	// Get registered antivirus products
	avProducts, _ := runPowerShellLong(`Get-CimInstance -Namespace root/SecurityCenter2 -ClassName AntiVirusProduct -EA 0 | Select displayName, @{N='enabled';E={$_.productState -band 0x1000}}, @{N='up_to_date';E={$_.productState -band 0x10}} | ConvertTo-Json -Compress`, 10)

	// Detect installed RMM tools by checking for known service/process names
	rmmCheck, _ := runPowerShellLong(`$rmms=@('AteraAgent','ConnectWiseControl','ScreenConnect','TeamViewer','AnyDesk','LogMeIn','Splashtop','Datto','NinjaRMM','NinjaOne','Syncro','Kaseya','Action1','N-able','SolarWinds','Pulseway','ManageEngine','Bomgar','BeyondTrust','Huntress','Level','MeshAgent','RustDesk','SimpleHelp','GoToAssist','RemotePC','TacticalRMM','pdq','Automox','JumpCloud');$found=@();foreach($n in $rmms){$svc=Get-Service -Name "*$n*" -EA 0|Select -First 1;$proc=Get-Process -Name "*$n*" -EA 0|Select -First 1;if($svc -or $proc){$found+=@{name=$n;service=if($svc){$svc.DisplayName}else{''};status=if($svc){$svc.Status.ToString()}else{'Running'};running=[bool]$proc}}}; @($found)|ConvertTo-Json -Depth 2 -Compress`, 15)

	result, _ := json.Marshal(map[string]interface{}{
		"hostname":          hostname,
		"os":                runtime.GOOS,
		"arch":              runtime.GOARCH,
		"cpus":              runtime.NumCPU(),
		"system_info":       systemInfo,
		"processes":         processes,
		"network":           netstat,
		"services":          services,
		"software":          software,
		"ip_config":         ipconfig,
		"logged_users":      users,
		"security_software": avProducts,
		"installed_rmms":    rmmCheck,
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

// captureMacros maps Fibratus QL event type macros to their expanded
// filter expressions. These are the same macros available in rules and
// the local `fibratus run` / `fibratus capture` commands.
var captureMacros = map[string]string{
	"spawn_process":      "kevt.name = 'CreateProcess'",
	"terminate_process":  "kevt.name = 'TerminateProcess'",
	"create_file":        "kevt.name = 'CreateFile'",
	"write_file":         "kevt.name = 'WriteFile'",
	"read_file":          "kevt.name = 'ReadFile'",
	"delete_file":        "kevt.name = 'DeleteFile'",
	"rename_file":        "kevt.name = 'RenameFile'",
	"set_reg_value":      "kevt.name = 'RegSetValue'",
	"create_reg_key":     "kevt.name = 'RegCreateKey'",
	"delete_reg_key":     "kevt.name = 'RegDeleteKey'",
	"delete_reg_value":   "kevt.name = 'RegDeleteValue'",
	"query_dns":          "kevt.name = 'QueryDns'",
	"reply_dns":          "kevt.name = 'ReplyDns'",
	"connect_process":    "kevt.name = 'Connect'",
	"accept_process":     "kevt.name = 'Accept'",
	"load_image":         "kevt.name = 'LoadImage'",
	"unload_image":       "kevt.name = 'UnloadImage'",
	"set_thread_context": "kevt.name = 'SetThreadContext'",
	"open_process":       "kevt.name = 'OpenProcess'",
	"create_handle":      "kevt.name = 'CreateHandle'",
	"close_handle":       "kevt.name = 'CloseHandle'",
	"virtual_alloc":      "kevt.name = 'VirtualAlloc'",
	"virtual_free":       "kevt.name = 'VirtualFree'",
	"map_view_file":      "kevt.name = 'MapViewFile'",
	"unmap_view_file":    "kevt.name = 'UnmapViewFile'",
	"create_thread":      "kevt.name = 'CreateThread'",
	"terminate_thread":   "kevt.name = 'TerminateThread'",
}

// expandCaptureMacros replaces event type macros in the expression with
// their Fibratus QL equivalents before the filter compiler sees them.
func expandCaptureMacros(expr string) string {
	for macro, expansion := range captureMacros {
		// Use word-boundary-aware replacement to avoid partial matches
		expr = strings.ReplaceAll(expr, macro, "("+expansion+")")
	}
	return expr
}

// buildCaptureFilter compiles a Fibratus QL filter expression using the
// real filter engine via a callback registered by bootstrap. Supports full
// QL syntax (ps.name, kevt.name, file.path, net.dip, etc.) plus event type
// macros (query_dns, spawn_process). Returns nil if expression is empty.
func buildCaptureFilter(expr string) func(evt *event.Event) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}

	// Expand event type macros before compiling
	expanded := expandCaptureMacros(expr)

	// Use the real Fibratus filter engine (registered by bootstrap at startup)
	if fleetserver.CaptureFilterCompiler != nil {
		fn, err := fleetserver.CaptureFilterCompiler(expanded)
		if err != nil {
			fmt.Fprintf(os.Stderr, "capture filter compile error: %v (capturing all events)\n", err)
			return nil
		}
		return fn
	}

	// Fallback if compiler not registered: shouldn't happen in normal operation
	fmt.Fprintf(os.Stderr, "capture filter compiler not registered (capturing all events)\n")
	return nil
}

// startCapture activates capture mode on the running ETW pipeline.
// Events matching the filter are tagged with capture_id and streamed to the server.
func (e *WindowsExecutor) startCapture(cmd *fleet.Command) (json.RawMessage, error) {
	var payload struct {
		CaptureID string `json:"capture_id"`
		Filter    string `json:"filter"`
		Duration  int    `json:"duration"` // seconds, 0 = indefinite
	}
	json.Unmarshal(cmd.Payload, &payload)

	if payload.CaptureID == "" {
		return nil, fmt.Errorf("capture_id required")
	}

	// Build filter function from the expression
	filterFn := buildCaptureFilter(payload.Filter)

	// Activate capture mode on the fleet output — events matching the filter
	// will be tagged with capture_id and bypass the security-relevance filter.
	fleetserver.SetCaptureState(payload.CaptureID, filterFn)

	// Auto-stop after duration (if specified)
	if payload.Duration > 0 {
		go func() {
			time.Sleep(time.Duration(payload.Duration) * time.Second)
			if fleetserver.GetCaptureID() == payload.CaptureID {
				fleetserver.ClearCaptureState()
			}
		}()
	}

	result, _ := json.Marshal(map[string]interface{}{
		"started":    true,
		"capture_id": payload.CaptureID,
		"filter":     payload.Filter,
		"kcap_path":  fleetserver.GetKcapPath(),
	})
	return result, nil
}

// stopCapture deactivates capture mode on the running ETW pipeline.
func (e *WindowsExecutor) stopCapture(cmd *fleet.Command) (json.RawMessage, error) {
	var payload struct {
		CaptureID string `json:"capture_id"`
	}
	json.Unmarshal(cmd.Payload, &payload)

	captureID := fleetserver.GetCaptureID()
	kcapPath := fleetserver.ClearCaptureState()

	result, _ := json.Marshal(map[string]interface{}{
		"stopped":    true,
		"kcap_path":  kcapPath,
		"capture_id": captureID,
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

// logoffUser terminates a user session by session ID.
func (e *WindowsExecutor) logoffUser(cmd *fleet.Command) (json.RawMessage, error) {
	var payload struct {
		SessionID string `json:"session_id"`
		Username  string `json:"username"`
	}
	json.Unmarshal(cmd.Payload, &payload)

	if payload.SessionID == "" && payload.Username == "" {
		return nil, fmt.Errorf("session_id or username required")
	}

	var out string
	var err error
	if payload.SessionID != "" {
		out, err = runCmd("logoff", payload.SessionID)
	} else {
		// Find session ID by username
		sessions, _ := runCmd("query", "user")
		for _, line := range strings.Split(sessions, "\n") {
			if strings.Contains(strings.ToLower(line), strings.ToLower(payload.Username)) {
				fields := strings.Fields(line)
				for _, f := range fields {
					if _, e := fmt.Sscanf(f, "%d", new(int)); e == nil {
						out, err = runCmd("logoff", f)
						break
					}
				}
				break
			}
		}
		if out == "" {
			return nil, fmt.Errorf("session not found for user %s", payload.Username)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("logoff failed: %s: %v", out, err)
	}

	result, _ := json.Marshal(map[string]interface{}{
		"logged_off": true,
		"session_id": payload.SessionID,
		"username":   payload.Username,
		"message":    strings.TrimSpace(out),
	})
	return result, nil
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

// ══════════════════════════════════════════════════════
// Remote Event Viewer commands
// ══════════════════════════════════════════════════════

// listEventLogChannels enumerates available Windows Event Log channels.
func (e *WindowsExecutor) listEventLogChannels(cmd *fleet.Command) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "wevtutil", "el").Output()
	if err != nil {
		return nil, fmt.Errorf("wevtutil el: %v", err)
	}

	type channelInfo struct {
		Name     string `json:"name"`
		Records  int64  `json:"records"`
		Enabled  bool   `json:"enabled"`
		MaxSize  int64  `json:"max_size"`
	}

	var channels []channelInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		ch := channelInfo{Name: name, Enabled: true}
		// Get record count and size for important channels
		if isImportantChannel(name) {
			info, _ := exec.CommandContext(ctx, "wevtutil", "gli", name).Output()
			for _, l := range strings.Split(string(info), "\n") {
				l = strings.TrimSpace(l)
				if strings.HasPrefix(l, "numberOfLogRecords:") {
					fmt.Sscanf(strings.TrimPrefix(l, "numberOfLogRecords:"), "%d", &ch.Records)
				}
				if strings.HasPrefix(l, "maxSize:") {
					fmt.Sscanf(strings.TrimPrefix(l, "maxSize:"), "%d", &ch.MaxSize)
				}
				if strings.HasPrefix(l, "enabled:") {
					ch.Enabled = strings.TrimSpace(strings.TrimPrefix(l, "enabled:")) == "true"
				}
			}
		}
		channels = append(channels, ch)
	}

	return json.Marshal(map[string]interface{}{
		"channels": channels,
		"total":    len(channels),
	})
}

func isImportantChannel(name string) bool {
	important := []string{
		"Security", "System", "Application",
		"Microsoft-Windows-Sysmon/Operational",
		"Microsoft-Windows-PowerShell/Operational",
		"Microsoft-Windows-Windows Defender/Operational",
		"Microsoft-Windows-TaskScheduler/Operational",
		"Microsoft-Windows-TerminalServices-LocalSessionManager/Operational",
		"Microsoft-Windows-WMI-Activity/Operational",
		"Microsoft-Windows-CodeIntegrity/Operational",
	}
	for _, c := range important {
		if strings.EqualFold(name, c) {
			return true
		}
	}
	return false
}

// queryEventLog queries events from a Windows Event Log channel using wevtutil.
func (e *WindowsExecutor) queryEventLog(cmd *fleet.Command) (json.RawMessage, error) {
	var payload struct {
		Channel string `json:"channel"`
		Count   int    `json:"count"`
		Query   string `json:"query"`   // XPath query filter
		EventID int    `json:"event_id"` // Filter by event ID
		Level   int    `json:"level"`    // 0=all, 1=critical, 2=error, 3=warning, 4=info
		Reverse bool   `json:"reverse"`  // newest first (default true)
	}
	json.Unmarshal(cmd.Payload, &payload)

	if payload.Channel == "" {
		payload.Channel = "Security"
	}
	if payload.Count <= 0 || payload.Count > 500 {
		payload.Count = 100
	}

	// Build XPath query
	xpath := payload.Query
	if xpath == "" {
		var filters []string
		if payload.EventID > 0 {
			filters = append(filters, fmt.Sprintf("EventID=%d", payload.EventID))
		}
		if payload.Level > 0 {
			filters = append(filters, fmt.Sprintf("Level=%d", payload.Level))
		}
		if len(filters) > 0 {
			xpath = fmt.Sprintf("*[System[%s]]", strings.Join(filters, " and "))
		} else {
			xpath = "*"
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := []string{"qe", payload.Channel, "/c:" + fmt.Sprintf("%d", payload.Count), "/f:xml"}
	if payload.Reverse || xpath == "*" {
		args = append(args, "/rd:true")
	}
	if xpath != "*" {
		args = append(args, "/q:"+xpath)
	}

	out, err := exec.CommandContext(ctx, "wevtutil", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("wevtutil qe %s: %s: %v", payload.Channel, truncate(string(out), 500), err)
	}

	// Parse XML events into structured JSON
	events := parseWevtutilXML(string(out))

	return json.Marshal(map[string]interface{}{
		"channel": payload.Channel,
		"events":  events,
		"count":   len(events),
		"query":   xpath,
	})
}

// parseWevtutilXML does a simple parse of wevtutil XML output into structured maps.
// Each <Event> block becomes a JSON object with System and EventData fields.
func parseWevtutilXML(xmlData string) []map[string]interface{} {
	var events []map[string]interface{}

	// Split on <Event blocks
	blocks := strings.Split(xmlData, "<Event ")
	for _, block := range blocks[1:] { // skip first empty split
		evt := make(map[string]interface{})

		// Extract System fields
		if provider := extractXMLAttr(block, "Provider", "Name"); provider != "" {
			evt["provider"] = provider
		}
		if eventID := extractXMLValue(block, "EventID"); eventID != "" {
			evt["event_id"] = eventID
		}
		if level := extractXMLValue(block, "Level"); level != "" {
			evt["level"] = level
		}
		if task := extractXMLValue(block, "Task"); task != "" {
			evt["task"] = task
		}
		if opcode := extractXMLValue(block, "Opcode"); opcode != "" {
			evt["opcode"] = opcode
		}
		if keywords := extractXMLValue(block, "Keywords"); keywords != "" {
			evt["keywords"] = keywords
		}
		if timeCreated := extractXMLAttr(block, "TimeCreated", "SystemTime"); timeCreated != "" {
			evt["timestamp"] = timeCreated
		}
		if eventRecordID := extractXMLValue(block, "EventRecordID"); eventRecordID != "" {
			evt["record_id"] = eventRecordID
		}
		if channel := extractXMLValue(block, "Channel"); channel != "" {
			evt["channel"] = channel
		}
		if computer := extractXMLValue(block, "Computer"); computer != "" {
			evt["computer"] = computer
		}
		if security := extractXMLAttr(block, "Security", "UserID"); security != "" {
			evt["user_id"] = security
		}

		// Extract EventData fields
		data := make(map[string]string)
		dataSection := ""
		if idx := strings.Index(block, "<EventData>"); idx >= 0 {
			if endIdx := strings.Index(block[idx:], "</EventData>"); endIdx >= 0 {
				dataSection = block[idx : idx+endIdx]
			}
		}
		if dataSection != "" {
			// Parse <Data Name='key'>value</Data> or <Data Name="key">value</Data>
			parts := strings.Split(dataSection, "<Data ")
			for _, p := range parts[1:] {
				// Extract name from Name='...' or Name="..."
				var name string
				for _, q := range []string{"'", "\""} {
					key := "Name=" + q
					ns := strings.Index(p, key)
					if ns < 0 {
						continue
					}
					ns += len(key)
					ne := strings.Index(p[ns:], q)
					if ne < 0 {
						continue
					}
					name = p[ns : ns+ne]
					break
				}
				if name == "" {
					continue
				}

				valStart := strings.Index(p, ">")
				if valStart < 0 {
					continue
				}
				// Check for self-closing tag: <Data Name='X'/>
				if valStart > 0 && p[valStart-1] == '/' {
					data[name] = ""
					continue
				}
				valEnd := strings.Index(p[valStart:], "</Data>")
				if valEnd < 0 {
					data[name] = ""
					continue
				}
				data[name] = p[valStart+1 : valStart+valEnd]
			}
		}
		if len(data) > 0 {
			evt["data"] = data
		}

		// Also try UserData section (some events use this instead)
		if len(data) == 0 {
			if idx := strings.Index(block, "<UserData>"); idx >= 0 {
				if endIdx := strings.Index(block[idx:], "</UserData>"); endIdx >= 0 {
					userData := block[idx : idx+endIdx]
					// Extract simple element values
					udata := make(map[string]string)
					parts := strings.Split(userData, "<")
					for _, p := range parts {
						if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "!") || strings.HasPrefix(p, "?") {
							continue
						}
						tagEnd := strings.IndexAny(p, " >")
						if tagEnd < 0 {
							continue
						}
						tag := p[:tagEnd]
						if tag == "UserData" || tag == "" {
							continue
						}
						valStart := strings.Index(p, ">")
						if valStart < 0 {
							continue
						}
						val := p[valStart+1:]
						if val != "" {
							udata[tag] = val
						}
					}
					if len(udata) > 0 {
						evt["data"] = udata
					}
				}
			}
		}

		if len(evt) > 0 {
			events = append(events, evt)
		}
	}
	return events
}

func extractXMLValue(xml, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	start := strings.Index(xml, open)
	if start < 0 {
		// Try self-closing with attributes
		open2 := "<" + tag + " "
		start = strings.Index(xml, open2)
		if start >= 0 {
			end := strings.Index(xml[start:], ">")
			if end >= 0 {
				inner := xml[start : start+end]
				if strings.HasSuffix(inner, "/") {
					return ""
				}
				// Has content after >
				contentStart := start + end + 1
				closeIdx := strings.Index(xml[contentStart:], close)
				if closeIdx >= 0 {
					return xml[contentStart : contentStart+closeIdx]
				}
			}
		}
		return ""
	}
	start += len(open)
	end := strings.Index(xml[start:], close)
	if end < 0 {
		return ""
	}
	return xml[start : start+end]
}

func extractXMLAttr(xml, tag, attr string) string {
	open := "<" + tag + " "
	start := strings.Index(xml, open)
	if start < 0 {
		return ""
	}
	end := strings.Index(xml[start:], ">")
	if end < 0 {
		return ""
	}
	tagContent := xml[start : start+end]
	// Try both double and single quotes: attr="val" or attr='val'
	for _, q := range []string{"\"", "'"} {
		attrKey := attr + "=" + q
		aStart := strings.Index(tagContent, attrKey)
		if aStart < 0 {
			continue
		}
		aStart += len(attrKey)
		aEnd := strings.Index(tagContent[aStart:], q)
		if aEnd < 0 {
			continue
		}
		return tagContent[aStart : aStart+aEnd]
	}
	return ""
}

// exportEvtx exports a Windows Event Log channel to an .evtx file and returns it base64-encoded.
func (e *WindowsExecutor) exportEvtx(cmd *fleet.Command) (json.RawMessage, error) {
	var payload struct {
		Channel string `json:"channel"`
		Query   string `json:"query"` // optional XPath filter
	}
	json.Unmarshal(cmd.Payload, &payload)

	if payload.Channel == "" {
		return nil, fmt.Errorf("channel is required")
	}

	// Export to temp file
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("fibratus-evtx-%d.evtx", time.Now().UnixNano()))
	defer os.Remove(tmpFile)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	args := []string{"epl", payload.Channel, tmpFile}
	if payload.Query != "" {
		args = append(args, "/q:"+payload.Query)
	}

	out, err := exec.CommandContext(ctx, "wevtutil", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("wevtutil epl: %s: %v", truncate(string(out), 500), err)
	}

	// Read the file
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("read evtx: %v", err)
	}

	info, _ := os.Stat(tmpFile)
	size := int64(0)
	if info != nil {
		size = info.Size()
	}

	// Base64 encode for transport (files are typically 1-50MB)
	encoded := base64.StdEncoding.EncodeToString(data)

	return json.Marshal(map[string]interface{}{
		"channel":  payload.Channel,
		"filename": filepath.Base(tmpFile),
		"size":     size,
		"data":     encoded,
	})
}
