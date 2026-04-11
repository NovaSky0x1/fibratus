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

package sigma

import "strings"

// logsourceConfig describes how a SIGMA logsource category maps to Fibratus
// event handling: which macro/condition to prefix, and which field name
// translation table to use.
type logsourceConfig struct {
	// conditionPrefix is prepended to the converted condition
	// (e.g. "spawn_process" for process_creation).
	conditionPrefix string
	// fields maps SIGMA field names to Fibratus field names.
	fields map[string]string
	// eventlogChannel is set when the logsource maps to a Windows Event Log
	// channel rather than a native ETW event.
	eventlogChannel string
	// eventlogEventID is the Sysmon or other provider event ID, if applicable.
	eventlogEventID int
}

// logsourceConfigs maps SIGMA logsource keys to their Fibratus conversion config.
// The key format is "category|product|service" with empty segments allowed.
var logsourceConfigs = map[string]*logsourceConfig{
	// ═══════════════════════════════════════════════════════════════
	// Native ETW event mappings (highest performance)
	// ═══════════════════════════════════════════════════════════════

	"process_creation|windows|": {
		conditionPrefix: "spawn_process",
		fields:          processCreationFields,
	},
	"process_creation||": {
		conditionPrefix: "spawn_process",
		fields:          processCreationFields,
	},
	"file_event|windows|": {
		conditionPrefix: "create_file",
		fields:          fileEventFields,
	},
	"file_event||": {
		conditionPrefix: "create_file",
		fields:          fileEventFields,
	},
	"file_change|windows|": {
		conditionPrefix: "set_file_information",
		fields:          fileEventFields,
	},
	"file_delete|windows|": {
		conditionPrefix: "evt.name = 'DeleteFile'",
		fields:          fileEventFields,
	},
	"file_rename|windows|": {
		conditionPrefix: "evt.name = 'RenameFile'",
		fields:          fileEventFields,
	},
	"registry_event|windows|": {
		conditionPrefix: "modify_registry",
		fields:          registryEventFields,
	},
	"registry_event||": {
		conditionPrefix: "modify_registry",
		fields:          registryEventFields,
	},
	"registry_set|windows|": {
		conditionPrefix: "set_value",
		fields:          registryEventFields,
	},
	"registry_add|windows|": {
		conditionPrefix: "create_key",
		fields:          registryEventFields,
	},
	"registry_delete|windows|": {
		conditionPrefix: "(evt.name = 'RegDeleteKey' or evt.name = 'RegDeleteValue')",
		fields:          registryEventFields,
	},
	"network_connection|windows|": {
		conditionPrefix: "connect_socket",
		fields:          networkConnectionFields,
	},
	"network_connection||": {
		conditionPrefix: "connect_socket",
		fields:          networkConnectionFields,
	},
	"dns_query|windows|": {
		conditionPrefix: "query_dns",
		fields:          dnsQueryFields,
	},
	"dns_query||": {
		conditionPrefix: "query_dns",
		fields:          dnsQueryFields,
	},
	"image_load|windows|": {
		conditionPrefix: "load_module",
		fields:          imageLoadFields,
	},
	"image_load||": {
		conditionPrefix: "load_module",
		fields:          imageLoadFields,
	},
	"driver_load|windows|": {
		conditionPrefix: "load_driver",
		fields:          driverLoadFields,
	},
	"driver_load||": {
		conditionPrefix: "load_driver",
		fields:          driverLoadFields,
	},
	"process_access|windows|": {
		conditionPrefix: "open_process",
		fields:          processAccessFields,
	},
	"create_remote_thread|windows|": {
		conditionPrefix: "create_remote_thread",
		fields:          createRemoteThreadFields,
	},
	"create_stream_hash|windows|": {
		conditionPrefix: "create_file",
		fields:          fileEventFields,
	},

	// ═══════════════════════════════════════════════════════════════
	// Sysmon event log mappings (for Sysmon-only events)
	// ═══════════════════════════════════════════════════════════════

	"|windows|sysmon": {
		conditionPrefix: "eventlog_event and eventlog.channel = 'Microsoft-Windows-Sysmon/Operational'",
		fields:          sysmonGenericFields,
		eventlogChannel: "Microsoft-Windows-Sysmon/Operational",
	},

	// ═══════════════════════════════════════════════════════════════
	// Windows Event Log mappings
	// ═══════════════════════════════════════════════════════════════

	"|windows|powershell": {
		conditionPrefix: "eventlog_event and eventlog.channel = 'Microsoft-Windows-PowerShell/Operational'",
		fields:          powershellFields,
		eventlogChannel: "Microsoft-Windows-PowerShell/Operational",
	},
	"|windows|powershell-classic": {
		conditionPrefix: "eventlog_event and eventlog.channel = 'Windows PowerShell'",
		fields:          powershellFields,
		eventlogChannel: "Windows PowerShell",
	},
	"|windows|security": {
		conditionPrefix: "eventlog_event and eventlog.channel = 'Security'",
		fields:          securityLogFields,
		eventlogChannel: "Security",
	},
	"|windows|system": {
		conditionPrefix: "eventlog_event and eventlog.channel = 'System'",
		fields:          systemLogFields,
		eventlogChannel: "System",
	},
	"|windows|application": {
		conditionPrefix: "eventlog_event and eventlog.channel = 'Application'",
		fields:          applicationLogFields,
		eventlogChannel: "Application",
	},
	"|windows|windefend": {
		conditionPrefix: "eventlog_event and eventlog.channel = 'Microsoft-Windows-Windows Defender/Operational'",
		fields:          eventlogDataFields,
		eventlogChannel: "Microsoft-Windows-Windows Defender/Operational",
	},
	"|windows|taskscheduler": {
		conditionPrefix: "eventlog_event and eventlog.channel = 'Microsoft-Windows-TaskScheduler/Operational'",
		fields:          eventlogDataFields,
		eventlogChannel: "Microsoft-Windows-TaskScheduler/Operational",
	},
	"|windows|wmi": {
		conditionPrefix: "eventlog_event and eventlog.channel = 'Microsoft-Windows-WMI-Activity/Operational'",
		fields:          eventlogDataFields,
		eventlogChannel: "Microsoft-Windows-WMI-Activity/Operational",
	},
	"|windows|bits-client": {
		conditionPrefix: "eventlog_event and eventlog.channel = 'Microsoft-Windows-Bits-Client/Operational'",
		fields:          eventlogDataFields,
		eventlogChannel: "Microsoft-Windows-Bits-Client/Operational",
	},
	"|windows|codeintegrity-operational": {
		conditionPrefix: "eventlog_event and eventlog.channel = 'Microsoft-Windows-CodeIntegrity/Operational'",
		fields:          eventlogDataFields,
		eventlogChannel: "Microsoft-Windows-CodeIntegrity/Operational",
	},
	"|windows|firewall-as": {
		conditionPrefix: "eventlog_event and eventlog.channel = 'Microsoft-Windows-Windows Firewall With Advanced Security/Firewall'",
		fields:          eventlogDataFields,
		eventlogChannel: "Microsoft-Windows-Windows Firewall With Advanced Security/Firewall",
	},
	"|windows|ntlm": {
		conditionPrefix: "eventlog_event and eventlog.channel = 'Microsoft-Windows-NTLM/Operational'",
		fields:          eventlogDataFields,
		eventlogChannel: "Microsoft-Windows-NTLM/Operational",
	},
	"|windows|dns-server": {
		conditionPrefix: "eventlog_event and eventlog.channel = 'DNS Server'",
		fields:          eventlogDataFields,
		eventlogChannel: "DNS Server",
	},
	"|windows|terminalservices-localsessionmanager": {
		conditionPrefix: "eventlog_event and eventlog.channel = 'Microsoft-Windows-TerminalServices-LocalSessionManager/Operational'",
		fields:          eventlogDataFields,
		eventlogChannel: "Microsoft-Windows-TerminalServices-LocalSessionManager/Operational",
	},
}

// ═══════════════════════════════════════════════════════════════
// Field mapping tables: SIGMA field name → Fibratus field name
// ═══════════════════════════════════════════════════════════════

var processCreationFields = map[string]string{
	// Core process fields
	"Image":              "ps.exe",
	"image":              "ps.exe",
	"CommandLine":        "ps.cmdline",
	"commandline":        "ps.cmdline",
	"ParentImage":        "ps.parent.exe",
	"parentimage":        "ps.parent.exe",
	"ParentCommandLine":  "ps.parent.cmdline",
	"parentcommandline":  "ps.parent.cmdline",
	"User":               "ps.username",
	"user":               "ps.username",
	"ProcessId":          "ps.pid",
	"processid":          "ps.pid",
	"ParentProcessId":    "ps.ppid",
	"parentprocessid":    "ps.ppid",
	"CurrentDirectory":   "ps.cwd",
	"currentdirectory":   "ps.cwd",
	"IntegrityLevel":     "ps.token.integrity_level",
	"integritylevel":     "ps.token.integrity_level",
	"ProcessName":        "ps.name",

	// PE metadata
	"OriginalFileName": "ps.pe.file.name",
	"originalfilename": "ps.pe.file.name",
	"Product":          "ps.pe.product",
	"product":          "ps.pe.product",
	"Company":          "ps.pe.company",
	"company":          "ps.pe.company",
	"Description":      "ps.pe.description",
	"description":      "ps.pe.description",
	"FileVersion":      "ps.pe.file.version",
	"fileversion":      "ps.pe.file.version",
	"Imphash":          "ps.pe.imphash",
	"imphash":          "ps.pe.imphash",

	// Signature fields
	"Signed":          "ps.signature.exists",
	"SignatureStatus": "ps.signature.trusted",
}

var fileEventFields = map[string]string{
	"TargetFilename":   "file.name",
	"targetfilename":   "file.name",
	"FileName":         "file.name",
	"filename":         "file.name",
	"Image":            "ps.exe",
	"image":            "ps.exe",
	"User":             "ps.username",
	"user":             "ps.username",
	"CommandLine":      "ps.cmdline",
	"commandline":      "ps.cmdline",
	"ProcessId":        "ps.pid",
	"SourceFilename":   "file.name",
	"CreationUtcTime":  "evt.time",
}

var registryEventFields = map[string]string{
	"TargetObject":   "registry.path",
	"targetobject":   "registry.path",
	"Details":        "registry.value",
	"details":        "registry.value",
	"Image":          "ps.exe",
	"image":          "ps.exe",
	"User":           "ps.username",
	"user":           "ps.username",
	"CommandLine":    "ps.cmdline",
	"commandline":    "ps.cmdline",
	"ProcessId":      "ps.pid",
	"EventType":      "", // handled by macro selection
	"NewName":        "registry.path",
}

var networkConnectionFields = map[string]string{
	"DestinationIp":       "net.dip",
	"destinationip":       "net.dip",
	"DestinationPort":     "net.dport",
	"destinationport":     "net.dport",
	"SourceIp":            "net.sip",
	"sourceip":            "net.sip",
	"SourcePort":          "net.sport",
	"sourceport":          "net.sport",
	"Protocol":            "net.l4.proto",
	"protocol":            "net.l4.proto",
	"Image":               "ps.exe",
	"image":               "ps.exe",
	"User":                "ps.username",
	"user":                "ps.username",
	"DestinationHostname": "net.dip.names",
	"SourceHostname":      "net.sip.names",
	"Initiated":           "", // direction: true=outbound, handled by macro
	"CommandLine":         "ps.cmdline",
	"commandline":         "ps.cmdline",
	"ProcessId":           "ps.pid",
	"DestinationIsIpv6":   "", // not directly mapped
	"SourceIsIpv6":        "", // not directly mapped
}

var dnsQueryFields = map[string]string{
	"QueryName":   "dns.name",
	"queryname":   "dns.name",
	"QueryType":   "dns.rr",
	"querytype":   "dns.rr",
	"QueryStatus": "dns.rcode",
	"querystatus": "dns.rcode",
	"Image":       "ps.exe",
	"image":       "ps.exe",
	"User":        "ps.username",
	"user":        "ps.username",
	"CommandLine": "ps.cmdline",
	"ProcessId":   "ps.pid",
	"record_type": "dns.rr",
	"answer":      "dns.answers",
}

var imageLoadFields = map[string]string{
	"ImageLoaded":     "module.name",
	"imageloaded":     "module.name",
	"Image":           "ps.exe",
	"image":           "ps.exe",
	"User":            "ps.username",
	"user":            "ps.username",
	"Signed":          "module.signature.exists",
	"signed":          "module.signature.exists",
	"SignatureStatus": "module.signature.trusted",
	"signaturestatus": "module.signature.trusted",
	"Signature":       "image.cert.subject",
	"signature":       "image.cert.subject",
	"CommandLine":     "ps.cmdline",
	"commandline":     "ps.cmdline",
	"ProcessId":       "ps.pid",
	"OriginalFileName": "pe.file.name",
	"Imphash":         "pe.imphash",
	"imphash":         "pe.imphash",
	"Company":         "pe.company",
	"Description":     "pe.description",
	"Product":         "pe.product",
}

var driverLoadFields = map[string]string{
	"ImageLoaded":     "module.name",
	"imageloaded":     "module.name",
	"Signed":          "module.signature.exists",
	"signed":          "module.signature.exists",
	"SignatureStatus": "module.signature.trusted",
	"signaturestatus": "module.signature.trusted",
	"Signature":       "image.cert.subject",
	"signature":       "image.cert.subject",
	"Imphash":         "pe.imphash",
	"imphash":         "pe.imphash",
}

var processAccessFields = map[string]string{
	"SourceImage":      "ps.exe",
	"sourceimage":      "ps.exe",
	"TargetImage":      "evt.arg[exe]",
	"targetimage":      "evt.arg[exe]",
	"GrantedAccess":    "ps.access.mask",
	"grantedaccess":    "ps.access.mask",
	"CallTrace":        "thread.callstack.detail",
	"calltrace":        "thread.callstack.detail",
	"SourceUser":       "ps.username",
	"sourceuser":       "ps.username",
	"SourceProcessId":  "evt.pid",
	"TargetProcessId":  "ps.pid",
}

var createRemoteThreadFields = map[string]string{
	"SourceImage":     "ps.exe",
	"sourceimage":     "ps.exe",
	"TargetImage":     "evt.arg[exe]",
	"targetimage":     "evt.arg[exe]",
	"StartFunction":   "thread.start_address.symbol",
	"StartModule":     "thread.start_address.module",
	"StartAddress":    "thread.start_address",
	"SourceUser":      "ps.username",
}

// ═══════════════════════════════════════════════════════════════
// Event Log field mappings (for Sysmon, PowerShell, Security, etc.)
// These map to eventlog.data[field] or direct eventlog.* fields.
// ═══════════════════════════════════════════════════════════════

var sysmonGenericFields = map[string]string{
	"EventID":         "eventlog.event.id",
	"eventid":         "eventlog.event.id",
	"Image":           "ps.exe",
	"CommandLine":     "ps.cmdline",
	"ParentImage":     "ps.parent.exe",
	"User":            "ps.username",
	"TargetFilename":  "file.name",
	"TargetObject":    "registry.path",
	"DestinationIp":   "net.dip",
	"DestinationPort": "net.dport",
	"SourceIp":        "net.sip",
	"SourcePort":      "net.sport",
	"QueryName":       "dns.name",
	"ImageLoaded":     "image.name",
}

var powershellFields = map[string]string{
	"EventID":       "eventlog.event.id",
	"eventid":       "eventlog.event.id",
	"ScriptBlockText": "eventlog.data[ScriptBlockText]",
	"HostApplication":  "eventlog.data[HostApplication]",
	"CommandLine":      "eventlog.data[CommandLine]",
	"Payload":          "eventlog.data[Payload]",
	"ContextInfo":      "eventlog.data[ContextInfo]",
	"Path":             "eventlog.data[Path]",
	"ScriptName":       "eventlog.data[ScriptName]",
	"CommandName":      "eventlog.data[CommandName]",
	"CommandType":      "eventlog.data[CommandType]",
}

var securityLogFields = map[string]string{
	"EventID":         "eventlog.event.id",
	"eventid":         "eventlog.event.id",
	"TargetUserName":  "eventlog.data[TargetUserName]",
	"SubjectUserName": "eventlog.data[SubjectUserName]",
	"LogonType":       "eventlog.data[LogonType]",
	"IpAddress":       "eventlog.data[IpAddress]",
	"IpPort":          "eventlog.data[IpPort]",
	"WorkstationName": "eventlog.data[WorkstationName]",
	"ProcessName":     "eventlog.data[ProcessName]",
	"ObjectName":      "eventlog.data[ObjectName]",
	"ObjectType":      "eventlog.data[ObjectType]",
	"AccessMask":      "eventlog.data[AccessMask]",
	"ServiceName":     "eventlog.data[ServiceName]",
	"TicketEncryptionType": "eventlog.data[TicketEncryptionType]",
	"Status":          "eventlog.data[Status]",
	"SubStatus":       "eventlog.data[SubStatus]",
	"TargetDomainName": "eventlog.data[TargetDomainName]",
	"SubjectDomainName": "eventlog.data[SubjectDomainName]",
	"PrivilegeList":   "eventlog.data[PrivilegeList]",
	"CommandLine":     "eventlog.data[CommandLine]",
	"NewProcessName":  "eventlog.data[NewProcessName]",
	"ParentProcessName": "eventlog.data[ParentProcessName]",
	"TargetServerName": "eventlog.data[TargetServerName]",
	"ShareName":       "eventlog.data[ShareName]",
	"RelativeTargetName": "eventlog.data[RelativeTargetName]",
	"TaskName":        "eventlog.data[TaskName]",
	"TaskContent":     "eventlog.data[TaskContent]",
}

var systemLogFields = map[string]string{
	"EventID":    "eventlog.event.id",
	"eventid":    "eventlog.event.id",
	"Provider":   "eventlog.provider",
	"provider":   "eventlog.provider",
	"DriverName": "eventlog.data[DriverName]",
	"ImagePath":  "eventlog.data[ImagePath]",
	"ServiceName": "eventlog.data[ServiceName]",
	"ServiceType": "eventlog.data[ServiceType]",
	"StartType":   "eventlog.data[StartType]",
}

var applicationLogFields = map[string]string{
	"EventID":    "eventlog.event.id",
	"eventid":    "eventlog.event.id",
	"Provider":   "eventlog.provider",
	"provider":   "eventlog.provider",
}

// eventlogDataFields is the fallback for any Windows Event Log source
// where specific field mappings are not defined. All fields are mapped
// to eventlog.data[field].
var eventlogDataFields = map[string]string{
	"EventID":  "eventlog.event.id",
	"eventid":  "eventlog.event.id",
	"Provider": "eventlog.provider",
	"provider": "eventlog.provider",
}

// resolveLogsource looks up the logsource configuration for a given SIGMA logsource.
// It tries exact match first, then progressively looser matches.
func resolveLogsource(ls Logsource) (*logsourceConfig, bool) {
	keys := []string{
		ls.Category + "|" + ls.Product + "|" + ls.Service,
		ls.Category + "|" + ls.Product + "|",
		ls.Category + "||",
		"|" + ls.Product + "|" + ls.Service,
	}
	for _, key := range keys {
		if cfg, ok := logsourceConfigs[key]; ok {
			return cfg, true
		}
	}
	return nil, false
}

// mapField maps a SIGMA field name to a Fibratus field name given a logsource config.
// If the field uses eventlog.data[...] mapping, unknown fields fall back to
// eventlog.data[FieldName'] for maximum compatibility.
func mapField(sigmaField string, cfg *logsourceConfig) (string, bool) {
	// Try exact match first
	if f, ok := cfg.fields[sigmaField]; ok {
		if f == "" {
			return "", false // explicitly unmappable field
		}
		return f, true
	}
	// Try case-insensitive match
	lower := strings.ToLower(sigmaField)
	if f, ok := cfg.fields[lower]; ok {
		if f == "" {
			return "", false
		}
		return f, true
	}
	// For event log sources, fall back to eventlog.data[field]
	if cfg.eventlogChannel != "" {
		return "eventlog.data[" + sigmaField + "]", true
	}
	return "", false
}

// GetSupportedLogsources returns all supported SIGMA logsource mappings.
func GetSupportedLogsources() []LogsourceMapping {
	var mappings []LogsourceMapping

	type entry struct {
		category, product, service, desc string
	}

	entries := []entry{
		{"process_creation", "windows", "", "Process creation events via ETW CreateProcess"},
		{"file_event", "windows", "", "File creation/modification via ETW file events"},
		{"file_change", "windows", "", "File attribute changes via ETW SetFileInformation"},
		{"file_delete", "windows", "", "File deletion via ETW DeleteFile"},
		{"file_rename", "windows", "", "File rename via ETW RenameFile"},
		{"registry_event", "windows", "", "Registry modification via ETW registry events"},
		{"registry_set", "windows", "", "Registry value set via ETW RegSetValue"},
		{"registry_add", "windows", "", "Registry key creation via ETW RegCreateKey"},
		{"registry_delete", "windows", "", "Registry deletion via ETW RegDeleteKey/RegDeleteValue"},
		{"network_connection", "windows", "", "Network connections via ETW TCP/UDP events"},
		{"dns_query", "windows", "", "DNS queries via ETW QueryDNS"},
		{"image_load", "windows", "", "DLL/module loads via ETW LoadImage"},
		{"driver_load", "windows", "", "Driver loads via ETW LoadImage with driver detection"},
		{"process_access", "windows", "", "Process handle opening via ETW OpenProcess"},
		{"create_remote_thread", "windows", "", "Remote thread creation via ETW CreateThread"},
		{"", "windows", "sysmon", "Sysmon event log (generic, all event IDs)"},
		{"", "windows", "powershell", "PowerShell Operational event log"},
		{"", "windows", "powershell-classic", "PowerShell classic event log"},
		{"", "windows", "security", "Windows Security event log"},
		{"", "windows", "system", "Windows System event log"},
		{"", "windows", "application", "Windows Application event log"},
		{"", "windows", "windefend", "Windows Defender event log"},
		{"", "windows", "taskscheduler", "Task Scheduler event log"},
		{"", "windows", "wmi", "WMI Activity event log"},
		{"", "windows", "bits-client", "BITS Client event log"},
		{"", "windows", "codeintegrity-operational", "Code Integrity event log"},
		{"", "windows", "firewall-as", "Windows Firewall event log"},
		{"", "windows", "ntlm", "NTLM Operational event log"},
		{"", "windows", "dns-server", "DNS Server event log"},
		{"", "windows", "terminalservices-localsessionmanager", "Terminal Services session log"},
	}

	for _, e := range entries {
		key := e.category + "|" + e.product + "|" + e.service
		if e.service == "" {
			key = e.category + "|" + e.product + "|"
		}
		cfg, ok := logsourceConfigs[key]
		if !ok {
			continue
		}
		m := LogsourceMapping{
			Category:    e.category,
			Product:     e.product,
			Service:     e.service,
			EventType:   cfg.conditionPrefix,
			Description: e.desc,
			Fields:      cfg.fields,
		}
		mappings = append(mappings, m)
	}
	return mappings
}

// GetFieldMappings returns all field mappings across all logsource categories.
func GetFieldMappings() []FieldMapping {
	seen := make(map[string]bool)
	var mappings []FieldMapping

	type catFields struct {
		category string
		fields   map[string]string
	}

	categories := []catFields{
		{"process_creation", processCreationFields},
		{"file_event", fileEventFields},
		{"registry_event", registryEventFields},
		{"network_connection", networkConnectionFields},
		{"dns_query", dnsQueryFields},
		{"image_load", imageLoadFields},
		{"driver_load", driverLoadFields},
		{"process_access", processAccessFields},
		{"create_remote_thread", createRemoteThreadFields},
		{"powershell", powershellFields},
		{"security", securityLogFields},
	}

	for _, cat := range categories {
		for sigmaField, fibratus := range cat.fields {
			if fibratus == "" {
				continue
			}
			// Skip all-lowercase entries (they are case-insensitive duplicates)
			if sigmaField == strings.ToLower(sigmaField) {
				continue
			}
			key := sigmaField + "|" + fibratus
			if seen[key] {
				continue
			}
			seen[key] = true
			mappings = append(mappings, FieldMapping{
				SigmaField:    sigmaField,
				FibratusField: fibratus,
				Category:      cat.category,
			})
		}
	}
	return mappings
}
