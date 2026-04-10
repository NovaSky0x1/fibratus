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

package fleetserver

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rabbitstack/fibratus/pkg/event"
	pb "github.com/rabbitstack/fibratus/pkg/fleet/pb"
	"github.com/rabbitstack/fibratus/pkg/fleet/tamper"
	"github.com/rabbitstack/fibratus/pkg/outputs"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fleetOutput struct {
	client    pb.AgentServiceClient
	conn      *grpc.ClientConn
	stream    pb.AgentService_StreamTelemetryClient
	serverURL string
	agentID   string
	orgID     string
	hostname  string
}

func init() {
	outputs.Register(outputs.FleetServer, initFleetServer)
}

func initFleetServer(config outputs.Config) (outputs.OutputGroup, error) {
	cfg, ok := config.Output.(Config)
	if !ok {
		return outputs.Fail(outputs.ErrInvalidConfig(outputs.FleetServer, config.Output))
	}

	if cfg.ServerURL == "" {
		cfg.ServerURL = loadEnrollmentFile("server-url")
		cfg.OrgID = loadEnrollmentFile("org-id")
		cfg.AgentID = loadEnrollmentFile("agent-id")
	}

	if cfg.ServerURL == "" {
		return outputs.Fail(fmt.Errorf("fleet server URL not configured and no enrollment data found"))
	}

	// Parse gRPC address
	serverAddr := parseGRPCAddr(cfg.ServerURL)

	// Build TLS config
	tlsCfg := &tls.Config{
		InsecureSkipVerify: cfg.TLSInsecureSkipVerify,
	}

	certDir := enrollmentCertDir()
	if certDir != "" {
		certFile := filepath.Join(certDir, "agent.crt")
		keyFile := filepath.Join(certDir, "agent.key")
		if fileExists(certFile) && fileExists(keyFile) {
			cert, err := tls.LoadX509KeyPair(certFile, keyFile)
			if err == nil {
				tlsCfg.Certificates = []tls.Certificate{cert}
			}
		}
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallSendMsgSize(64 * 1024 * 1024),
		),
	}

	conn, err := grpc.NewClient(serverAddr, opts...)
	if err != nil {
		return outputs.Fail(fmt.Errorf("fleet output: dial %s: %v", serverAddr, err))
	}

	hostname, _ := os.Hostname()

	client := &fleetOutput{
		client:    pb.NewAgentServiceClient(conn),
		conn:      conn,
		serverURL: serverAddr,
		agentID:   cfg.AgentID,
		orgID:     cfg.OrgID,
		hostname:  hostname,
	}

	return outputs.Success(client), nil
}

func (f *fleetOutput) Connect() error {
	log.Infof("fleet output: connected to %s (gRPC)", f.serverURL)
	return nil
}

func (f *fleetOutput) Close() error {
	if f.stream != nil {
		f.stream.CloseAndRecv()
	}
	if f.conn != nil {
		return f.conn.Close()
	}
	return nil
}

// telemetryEventNames is the set of events always stored on the fleet server.
var telemetryEventNames = map[string]bool{
	// Process lifecycle — critical for process tree and detection context
	"CreateProcess": true, "TerminateProcess": true,
	// File mutations — creates/writes are too noisy (40K+/min from system services),
	// only deletions and renames are forwarded; creates/writes still sent if they
	// trigger a detection (metadata check below)
	"DeleteFile": true, "RenameFile": true,
	// Registry mutations — all are security-relevant
	"RegSetValue": true, "RegCreateKey": true, "RegDeleteKey": true, "RegDeleteValue": true,
	// Network — connections are security-relevant
	"Connect": true, "Accept": true,
	// DNS — critical for threat hunting
	"QueryDns": true, "ReplyDns": true,
	// Module loads — DLL sideloading, injection detection
	"LoadImage": true, "UnloadImage": true,
	// Thread context manipulation — injection technique indicator
	"SetThreadContext": true,
	// Windows Event Log — all collected events are security-relevant
	"EventLogEvent": true,
}

var telemetryDropNames = map[string]bool{
	"OpenProcess": true, // extremely noisy — 200K+/5min
	// CreateFile/WriteFile handled specially in securityRelevant() — only security extensions
	"SubmitThreadpoolWork": true, "SubmitThreadpoolCallback": true, "SetThreadpoolTimer": true,
	"VirtualAlloc": true, "VirtualFree": true,
	"ReadFile": true, "CloseFile": true, "ReleaseFile": true, "EnumDirectory": true,
	"FileOpEnd": true, "FileRundown": true, "SetFileInformation": true,
	"MapViewFile": true, "UnmapViewFile": true, "MapFileRundown": true,
	"RegOpenKey": true, "RegCloseKey": true, "RegQueryKey": true, "RegQueryValue": true,
	"RegKCBRundown": true, "RegCreateKCB": true,
	"CreateHandle": true, "CloseHandle": true, "DuplicateHandle": true,
	"CreateThread": true, "TerminateThread": true, "OpenThread": true, "ThreadRundown": true,
	"ProcessRundown": true, "ImageRundown": true, "StackWalk": true,
	"CreateSymbolicLinkObject": true,
}

// securityFileExtensions are file extensions worth storing for investigations.
var securityFileExtensions = map[string]bool{
	// Windows executables and libraries
	".exe": true, ".dll": true, ".sys": true, ".drv": true,
	".ocx": true, ".cpl": true, ".scr": true, ".com": true,
	".pif": true, ".lnk": true, ".url": true, ".appx": true,
	".msix": true, ".appxbundle": true,

	// Installers and packages
	".msi": true, ".msp": true, ".mst": true, ".msm": true,
	".cab": true, ".deb": true, ".rpm": true, ".pkg": true,
	".dmg": true, ".snap": true, ".flatpak": true,

	// Scripts — Windows
	".bat": true, ".cmd": true, ".ps1": true, ".psm1": true,
	".psd1": true, ".vbs": true, ".vbe": true, ".js": true,
	".jse": true, ".wsh": true, ".wsf": true, ".wsc": true,
	".hta": true, ".inf": true, ".reg": true, ".sct": true,

	// Scripts — Python
	".py": true, ".pyw": true, ".pyc": true, ".pyo": true,
	".pyd": true, ".pyz": true, ".pyzw": true, ".egg": true,
	".whl": true,

	// Scripts — Other languages
	".rb": true, ".pl": true, ".pm": true, ".sh": true,
	".bash": true, ".zsh": true, ".fish": true, ".csh": true,
	".lua": true, ".tcl": true, ".r": true, ".go": true,
	".rs": true, ".swift": true, ".kt": true, ".scala": true,
	".groovy": true, ".gradle": true,

	// Notebooks and data science
	".ipynb": true, ".rmd": true, ".qmd": true,

	// Java / JVM
	".jar": true, ".class": true, ".war": true, ".ear": true,
	".jnlp": true, ".jsp": true,

	// .NET
	".cs": true, ".csx": true, ".cshtml": true,
	".nupkg": true, ".csproj": true,

	// Web / markup that can execute
	".html": true, ".htm": true, ".xhtml": true, ".svg": true,
	".xml": true, ".xsl": true, ".xslt": true, ".asp": true,
	".aspx": true, ".php": true, ".ejs": true, ".ts": true,
	".tsx": true, ".jsx": true, ".mjs": true, ".cjs": true,
	".json": true, ".yaml": true, ".yml": true, ".toml": true,

	// Office documents (macro-capable)
	".doc": true, ".docx": true, ".docm": true, ".dotm": true,
	".xls": true, ".xlsx": true, ".xlsm": true, ".xltm": true,
	".xlam": true, ".xlsb": true,
	".ppt": true, ".pptx": true, ".pptm": true, ".potm": true,
	".ppam": true, ".ppsm": true, ".sldm": true,
	".pub": true, ".one": true, ".onenote": true,
	".accdb": true, ".accde": true, ".mdb": true,

	// OpenDocument (macro-capable)
	".odt": true, ".ods": true, ".odp": true, ".odg": true,

	// PDF and rich text
	".pdf": true, ".rtf": true, ".xps": true,

	// Disk images and virtual disks
	".iso": true, ".img": true, ".vhd": true, ".vhdx": true,
	".vmdk": true, ".vdi": true, ".qcow2": true, ".wim": true,

	// Archives
	".zip": true, ".7z": true, ".rar": true, ".tar": true,
	".gz": true, ".bz2": true, ".xz": true, ".zst": true,
	".tgz": true, ".tbz2": true, ".lzma": true, ".lz": true,
	".arj": true, ".ace": true, ".zoo": true,

	// Crypto and credentials
	".pfx": true, ".p12": true, ".cer": true, ".crt": true,
	".pem": true, ".key": true, ".der": true, ".jks": true,
	".keystore": true, ".kdb": true, ".kdbx": true,
	".rdp": true, ".rdg": true, ".ppk": true,

	// Database
	".sql": true, ".sqlite": true, ".db": true, ".mdf": true,
	".ldf": true, ".bak": true,

	// Configuration
	".conf": true, ".cfg": true, ".ini": true, ".env": true,
	".htaccess": true, ".htpasswd": true, ".gitconfig": true,

	// Containers and IaC
	".dockerfile": true, ".tf": true, ".tfvars": true,
	".vagrantfile": true, ".ansible": true,

	// Memory and forensic artifacts
	".dmp": true, ".mdmp": true, ".hdmp": true, ".etl": true,
	".evtx": true, ".evt": true, ".pcap": true, ".pcapng": true,

	// Firmware and UEFI
	".efi": true, ".rom": true, ".bin": true, ".fw": true,

	// Shortcut and autorun
	".desktop": true, ".service": true, ".timer": true,
	".automount": true, ".plist": true, ".job": true,
	".task": true, ".ics": true,

	// C/C++ source and build artifacts
	".c": true, ".cpp": true, ".cc": true, ".cxx": true,
	".h": true, ".hpp": true, ".hxx": true,
	".o": true, ".obj": true, ".lib": true, ".a": true,
	".so": true, ".dylib": true,
	".makefile": true, ".cmake": true, ".sln": true, ".vcxproj": true,

	// C2 frameworks / red team
	".cna": true, ".profile": true, ".beacon": true,
	".malleable": true, ".cobaltstrike": true,

	// AutoIt / AutoHotkey (common in malware)
	".au3": true, ".ahk": true, ".a3x": true,

	// WMI persistence
	".mof": true,

	// Compiled HTML Help (malware vector)
	".chm": true,

	// PowerShell constrained language bypass
	".psrc": true, ".pssc": true,

	// OneNote
	".onepkg": true,

	// Electron / Node
	".asar": true,

	// Windows settings / ClickOnce execution vectors
	".settingcontent-ms": true, ".diagcab": true,
	".appref-ms": true, ".idt": true,
}

func hasSecurityExtension(name string) bool {
	name = strings.ToLower(name)
	for ext := range securityFileExtensions {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

func securityRelevant(batch *event.Batch) *event.Batch {
	filtered := make([]*event.Event, 0, len(batch.Events)/4)
	for _, evt := range batch.Events {
		if telemetryDropNames[evt.Name] {
			continue
		}
		// Always forward events with rule matches or evasion flags
		if len(evt.Metadata) > 0 || evt.Evasions > 0 {
			filtered = append(filtered, evt)
			continue
		}
		// CreateFile/WriteFile: only for security-relevant file extensions
		if evt.Name == "CreateFile" || evt.Name == "WriteFile" {
			// Only forward file mutations (create/overwrite/supersede), not opens
			disp := evt.GetParamAsString("create_disposition")
			if disp == "OPEN" || disp == "" {
				continue // skip file opens — too noisy (23K+/min)
			}
			fp := evt.GetParamAsString("file_path")
			if fp == "" {
				fp = evt.GetParamAsString("file_name")
			}
			if fp != "" && hasSecurityExtension(fp) {
				filtered = append(filtered, evt)
			}
			continue
		}
		if telemetryEventNames[evt.Name] {
			filtered = append(filtered, evt)
		}
	}
	return &event.Batch{Events: filtered}
}

// CaptureFilterCompiler compiles a Fibratus QL expression into a filter
// function. Registered by bootstrap at startup to avoid import cycles.
var CaptureFilterCompiler func(expr string) (func(*event.Event) bool, error)

// captureState tracks the active capture session for event forking.
var captureState struct {
	sync.RWMutex
	active   bool
	id       string
	filterFn func(*event.Event) bool // nil = capture all events
	writer   *kcapWriter             // local .kcap file writer
	kcapPath string                  // path to local .kcap file
}

// SetCaptureState activates capture mode. Matching events will be tagged
// with the capture ID and bypass the security-relevance filter.
// Also starts a local .kcap writer (pure-Go, no CGO needed).
func SetCaptureState(captureID string, filterFn func(*event.Event) bool) {
	captureState.Lock()
	defer captureState.Unlock()
	captureState.active = true
	captureState.id = captureID
	captureState.filterFn = filterFn

	// Start local .kcap writer
	exe, _ := os.Executable()
	captureDir := filepath.Join(filepath.Dir(exe), "..", "captures")
	os.MkdirAll(captureDir, 0o755)
	capPath := filepath.Join(captureDir, fmt.Sprintf("capture-%s.kcap", captureID[:8]))

	w, err := newKcapWriter(capPath)
	if err != nil {
		log.Warnf("fleet output: failed to create .kcap writer: %v", err)
	} else {
		captureState.writer = w
		captureState.kcapPath = capPath
		log.Infof("fleet output: capture %s writing .kcap to %s", captureID, capPath)
	}

	log.Infof("fleet output: capture %s activated", captureID)
}

// ClearCaptureState deactivates capture mode and closes the .kcap writer.
func ClearCaptureState() string {
	captureState.Lock()
	defer captureState.Unlock()
	kcapPath := captureState.kcapPath
	if captureState.active {
		log.Infof("fleet output: capture %s deactivated", captureState.id)
		if captureState.writer != nil {
			captureState.writer.close()
		}
	}
	captureState.active = false
	captureState.id = ""
	captureState.filterFn = nil
	captureState.writer = nil
	captureState.kcapPath = ""
	return kcapPath
}

// GetCaptureID returns the active capture ID (empty if no capture active).
func GetCaptureID() string {
	captureState.RLock()
	defer captureState.RUnlock()
	if captureState.active {
		return captureState.id
	}
	return ""
}

// GetKcapPath returns the .kcap file path for the active capture.
func GetKcapPath() string {
	captureState.RLock()
	defer captureState.RUnlock()
	return captureState.kcapPath
}

// Publish sends a batch of events to the fleet server via gRPC streaming.
func (f *fleetOutput) Publish(batch *event.Batch) error {
	// Snapshot capture state once per batch
	captureState.RLock()
	capActive := captureState.active
	capID := captureState.id
	capFilter := captureState.filterFn
	capWriter := captureState.writer
	captureState.RUnlock()

	// Build capture events from the unfiltered batch
	var captureEvents []*event.Event
	if capActive {
		for _, evt := range batch.Events {
			if capFilter == nil || capFilter(evt) {
				captureEvents = append(captureEvents, evt)
				// Also write to local .kcap file
				if capWriter != nil {
					capWriter.writeEvent(evt)
				}
			}
		}
	}

	// Apply security-relevance filter for normal telemetry
	telBatch := securityRelevant(batch)

	if len(telBatch.Events) == 0 && len(captureEvents) == 0 {
		return nil
	}

	// Refresh agent ID if not set at init time
	agentID := f.agentID
	if agentID == "" {
		// Try DPAPI registry first, then legacy file fallback
		if enrollData := tamper.LoadEnrollment(); enrollData != nil && enrollData.AgentID != "" {
			agentID = enrollData.AgentID
		} else {
			agentID = loadEnrollmentFile("agent-id")
		}
	}

	// Convert telemetry events to protobuf
	seenSeq := make(map[uint64]int) // seq → index in pbEvents
	pbEvents := make([]*pb.TelemetryEvent, 0, len(telBatch.Events)+len(captureEvents))
	for _, evt := range telBatch.Events {
		pbEvt := convertEventToProto(evt)
		seenSeq[evt.Seq] = len(pbEvents)
		pbEvents = append(pbEvents, pbEvt)
	}

	// Add capture events — embed capture_id in the Metadata JSON field
	// (the CaptureId proto struct field isn't in the raw descriptor, so we
	// use metadata which is an existing bytes field that serializes correctly)
	capMeta, _ := json.Marshal(map[string]string{"capture_id": capID})
	for _, evt := range captureEvents {
		if idx, exists := seenSeq[evt.Seq]; exists {
			// Event already in telemetry batch — tag it via metadata
			pbEvents[idx].Metadata = capMeta
		} else {
			pbEvt := convertEventToProto(evt)
			pbEvt.Metadata = capMeta
			pbEvents = append(pbEvents, pbEvt)
		}
	}

	pbBatch := &pb.TelemetryBatch{
		AgentId:  agentID,
		OrgId:    f.orgID,
		Hostname: f.hostname,
		Events:   pbEvents,
	}

	// Open a new stream per publish batch (simple, reliable).
	md := metadata.Pairs("x-agent-id", agentID, "x-org-id", f.orgID)
	streamCtx := metadata.NewOutgoingContext(context.Background(), md)

	stream, err := f.client.StreamTelemetry(streamCtx)
	if err != nil {
		return fmt.Errorf("fleet output: open stream: %v", err)
	}

	if err := stream.Send(pbBatch); err != nil {
		return fmt.Errorf("fleet output: send batch: %v", err)
	}

	if _, err := stream.CloseAndRecv(); err != nil {
		return fmt.Errorf("fleet output: close stream: %v", err)
	}

	return nil
}

// convertEventToProto converts a kernel event to a protobuf TelemetryEvent.
func convertEventToProto(evt *event.Event) *pb.TelemetryEvent {
	pbEvt := &pb.TelemetryEvent{
		Seq:       int64(evt.Seq),
		Timestamp: timestamppb.New(evt.Timestamp),
		EventName: evt.Name,
	}
	if evt.PS != nil {
		pbEvt.ProcessName = evt.PS.Name
		pbEvt.ProcessExe = evt.PS.Exe
		pbEvt.ProcessCmdline = evt.PS.Cmdline
		pbEvt.Pid = uint32(evt.PS.PID)
		if evt.PS.Parent != nil {
			pbEvt.ParentPid = uint32(evt.PS.Parent.PID)
			pbEvt.ParentName = evt.PS.Parent.Name
		}
	}
	if raw := evt.MarshalJSON(); raw != nil {
		pbEvt.RawEvent = raw
	}
	if evt.Params != nil {
		if paramsJSON, err := json.Marshal(evt.Params); err == nil {
			pbEvt.Params = paramsJSON
		}
	}
	return pbEvt
}

// Config for the fleet server output.
type Config struct {
	Enabled              bool   `mapstructure:"enabled"`
	ServerURL            string `mapstructure:"server-url"`
	APIKey               string `mapstructure:"api-key"`
	OrgID                string `mapstructure:"org-id"`
	AgentID              string `mapstructure:"agent-id"`
	TLSCA                string `mapstructure:"tls-ca"`
	TLSInsecureSkipVerify bool  `mapstructure:"tls-insecure-skip-verify"`
}

func parseGRPCAddr(serverURL string) string {
	addr := serverURL
	addr = strings.TrimPrefix(addr, "https://")
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimRight(addr, "/")
	if !strings.Contains(addr, ":") {
		addr += ":443"
	}
	return addr
}

func loadEnrollmentFile(name string) string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dataDir := filepath.Join(filepath.Dir(exe), "..", "data")
	data, err := os.ReadFile(filepath.Join(dataDir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func enrollmentCertDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "..", "data", "certs")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
