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
)

// WindowsExecutor executes fleet commands on Windows endpoints.
type WindowsExecutor struct {
	serverURL string
}

// NewWindowsExecutor creates a new Windows command executor.
func NewWindowsExecutor(serverURL string) *WindowsExecutor {
	return &WindowsExecutor{serverURL: serverURL}
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
	default:
		return nil, fmt.Errorf("unknown command type: %s", cmd.Type)
	}
}

// isolate blocks all network traffic except communication with the fleet server.
// Uses Windows Firewall (netsh advfirewall) to create blocking rules with an
// exception for the fleet server IP.
func (e *WindowsExecutor) isolate(cmd *fleet.Command) (json.RawMessage, error) {
	// Extract fleet server hostname for the allow rule
	serverHost := strings.TrimPrefix(e.serverURL, "https://")
	serverHost = strings.TrimPrefix(serverHost, "http://")
	serverHost = strings.Split(serverHost, ":")[0]
	serverHost = strings.Split(serverHost, "/")[0]

	commands := []string{
		// Block all outbound
		`netsh advfirewall firewall add rule name="Fibratus Isolation - Block Outbound" dir=out action=block enable=yes profile=any`,
		// Block all inbound
		`netsh advfirewall firewall add rule name="Fibratus Isolation - Block Inbound" dir=in action=block enable=yes profile=any`,
		// Allow fleet server outbound
		fmt.Sprintf(`netsh advfirewall firewall add rule name="Fibratus Isolation - Allow Fleet" dir=out action=allow remoteip=%s enable=yes profile=any`, serverHost),
		// Allow fleet server inbound
		fmt.Sprintf(`netsh advfirewall firewall add rule name="Fibratus Isolation - Allow Fleet In" dir=in action=allow remoteip=%s enable=yes profile=any`, serverHost),
		// Allow DNS (needed to resolve fleet server hostname)
		`netsh advfirewall firewall add rule name="Fibratus Isolation - Allow DNS" dir=out action=allow protocol=udp remoteport=53 enable=yes profile=any`,
		// Allow loopback
		`netsh advfirewall firewall add rule name="Fibratus Isolation - Allow Loopback" dir=out action=allow remoteip=127.0.0.1 enable=yes profile=any`,
	}

	var results []string
	for _, c := range commands {
		out, err := runCmd("cmd", "/C", c)
		if err != nil {
			results = append(results, fmt.Sprintf("FAILED: %s: %v", c, err))
		} else {
			results = append(results, fmt.Sprintf("OK: %s", strings.TrimSpace(out)))
		}
	}

	result, _ := json.Marshal(map[string]interface{}{
		"isolated": true,
		"server":   serverHost,
		"details":  results,
	})
	return result, nil
}

// unisolate removes all Fibratus isolation firewall rules.
func (e *WindowsExecutor) unisolate(cmd *fleet.Command) (json.RawMessage, error) {
	rules := []string{
		"Fibratus Isolation - Block Outbound",
		"Fibratus Isolation - Block Inbound",
		"Fibratus Isolation - Allow Fleet",
		"Fibratus Isolation - Allow Fleet In",
		"Fibratus Isolation - Allow DNS",
		"Fibratus Isolation - Allow Loopback",
	}

	var removed int
	for _, name := range rules {
		_, err := runCmd("netsh", "advfirewall", "firewall", "delete", "rule", fmt.Sprintf("name=%s", name))
		if err == nil {
			removed++
		}
	}

	result, _ := json.Marshal(map[string]interface{}{
		"isolated":      false,
		"rules_removed": removed,
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

// uninstall removes the Fibratus service and cleans up.
func (e *WindowsExecutor) uninstall(cmd *fleet.Command) (json.RawMessage, error) {
	// Stop the service
	runCmd("sc", "stop", "fibratus")
	time.Sleep(2 * time.Second)

	// Remove the service
	out, err := runCmd("sc", "delete", "fibratus")
	if err != nil {
		return nil, fmt.Errorf("failed to remove service: %s: %v", out, err)
	}

	// Clean up data directory
	exe, _ := os.Executable()
	dataDir := filepath.Join(filepath.Dir(exe), "..", "data")
	os.RemoveAll(dataDir)

	result, _ := json.Marshal(map[string]interface{}{
		"uninstalled": true,
		"message":     "Fibratus service removed. Binary remains on disk for manual cleanup.",
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
