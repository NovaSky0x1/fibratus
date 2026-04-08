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
	"time"

	"github.com/rabbitstack/fibratus/pkg/event"
	pb "github.com/rabbitstack/fibratus/pkg/fleet/pb"
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
	".exe": true, ".dll": true, ".sys": true, ".drv": true,
	".lnk": true, ".scr": true, ".pif": true, ".com": true,
	".ps1": true, ".psm1": true, ".psd1": true,
	".bat": true, ".cmd": true, ".vbs": true, ".vbe": true,
	".js": true, ".jse": true, ".wsh": true, ".wsf": true,
	".hta": true, ".msi": true, ".msp": true, ".mst": true,
	".cpl": true, ".inf": true, ".reg": true,
	".jar": true, ".class": true,
	".doc": true, ".docx": true, ".docm": true,
	".xls": true, ".xlsx": true, ".xlsm": true,
	".ppt": true, ".pptx": true, ".pptm": true,
	".iso": true, ".img": true, ".vhd": true, ".vhdx": true,
	".zip": true, ".7z": true, ".rar": true, ".cab": true,
	".pdf": true, ".rtf": true,
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

// Publish sends a batch of events to the fleet server via gRPC streaming.
func (f *fleetOutput) Publish(batch *event.Batch) error {
	batch = securityRelevant(batch)
	if len(batch.Events) == 0 {
		return nil
	}

	// Refresh agent ID if not set at init time
	agentID := f.agentID
	if agentID == "" {
		agentID = loadEnrollmentFile("agent-id")
	}

	// Convert events to protobuf
	pbEvents := make([]*pb.TelemetryEvent, 0, len(batch.Events))
	for _, evt := range batch.Events {
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
		// Include raw JSON event for the store
		if raw := evt.MarshalJSON(); raw != nil {
			pbEvt.RawEvent = raw
		}
		if evt.Params != nil {
			if paramsJSON, err := json.Marshal(evt.Params); err == nil {
				pbEvt.Params = paramsJSON
			}
		}
		pbEvents = append(pbEvents, pbEvt)
	}

	telBatch := &pb.TelemetryBatch{
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

	if err := stream.Send(telBatch); err != nil {
		return fmt.Errorf("fleet output: send batch: %v", err)
	}

	if _, err := stream.CloseAndRecv(); err != nil {
		return fmt.Errorf("fleet output: close stream: %v", err)
	}

	return nil
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
