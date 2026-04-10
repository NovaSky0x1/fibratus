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

package qlparser

import (
	"sort"
	"strings"
	"unicode"
)

// FieldInfo is the field metadata descriptor.
type FieldInfo struct {
	Field       Field
	Desc        string
	Type        ParamType
	Examples    []string
	Deprecation *Deprecation
	Argument    *Argument
}

// IsDeprecated determines if the field is deprecated.
func (f FieldInfo) IsDeprecated() bool { return f.Deprecation != nil }

// Argument defines field argument information.
type Argument struct {
	// Optional indicates if the argument is optional.
	Optional bool
	// ValidationFunc is the field argument validation function.
	ValidationFunc func(string) bool
	// Pattern contains the regular expression like string that
	// represents the character set allowed for the argument value.
	Pattern string
}

// Validate validates the provided field argument.
func (a *Argument) Validate(v string) bool {
	if a.ValidationFunc == nil {
		return true
	}
	return a.ValidationFunc(v)
}

// Deprecation specifies field deprecation info.
type Deprecation struct {
	// Since denotes from which version the field is flagged as deprecated
	Since string
	// Fields represents the fields by which the deprecated field is superseded
	Fields []Field
}

// ParamType defines event parameter type.
type ParamType uint16

const (
	ParamNull          ParamType = iota
	ParamUnicodeString           // a string of 16-bit characters
	ParamAnsiString              // a string of 8-bit characters
	ParamInt8                    // a signed 8-bit integer
	ParamUint8                   // an unsigned 8-bit integer
	ParamInt16                   // a signed 16-bit integer
	ParamUint16                  // an unsigned 16-bit integer
	ParamInt32                   // a signed 32-bit integer
	ParamUint32                  // an unsigned 32-bit integer
	ParamInt64                   // a signed 64-bit integer
	ParamUint64                  // an unsigned 64-bit integer
	ParamFloat                   // an IEEE 4-byte floating-point number
	ParamDouble                  // an IEEE 8-byte floating-point number
	ParamBool                    // a 32-bit value where 0 is false and 1 is true
	ParamBinary                  // binary data of variable size
	ParamGUID                    // a GUID structure
	ParamPointer                 // an unsigned 32-bit or 64-bit pointer value
	ParamSID                     // a security identifier (SID) structure
	ParamPID                     // the process identifier
	ParamTID                     // the thread identifier
	ParamWbemSID                 // WBEM security identifier
	ParamPort                    // endpoint port number
	ParamIP                      // IP address
	ParamIPv4                    // IPv4 address
	ParamIPv6                    // IPv6 address
	ParamTime                    // timestamp
	ParamSlice                   // a collection of items
	ParamEnum                    // an enumeration
	ParamMap                     // a map
	ParamObject                  // generic object type
	ParamDOSPath                 // file system path in DOS device notation
	ParamPath                    // file system path with normalized drive letter
	ParamStatus                  // system error code message
	ParamKey                     // registry key
	ParamFlags                   // a bitmask of flags
	ParamFlags64                 // an extended (64 bits) bitmask of flags
	ParamAddress                 // memory address reference
	ParamHandleType              // handle type such as Mutex or File
)

// isNumber is the field argument validation function that
// returns true if all characters are digits.
var isNumber = func(s string) bool {
	for _, c := range s {
		if !unicode.IsNumber(c) {
			return false
		}
	}
	return true
}

const (
	// PsPid represents the process id field
	PsPid Field = "ps.pid"
	// PsPpid represents the parent process id field
	PsPpid Field = "ps.ppid"
	// PsName represents the process name field
	PsName Field = "ps.name"
	// PsComm represents the process command line field. Deprecated.
	PsComm Field = "ps.comm"
	// PsCmdline represents the process command line field
	PsCmdline Field = "ps.cmdline"
	// PsExe represents the process image path field
	PsExe Field = "ps.exe"
	// PsArgs represents the process command line arguments
	PsArgs Field = "ps.args"
	// PsCwd represents the process current working directory
	PsCwd Field = "ps.cwd"
	// PsSID represents the process security identifier
	PsSID Field = "ps.sid"
	// PsDomain represents the process domain field
	PsDomain Field = "ps.domain"
	// PsUsername represents the process username field
	PsUsername Field = "ps.username"
	// PsSessionID represents the session id bound to the process
	PsSessionID Field = "ps.sessionid"
	// PsEnvs represents the process environment variables
	PsEnvs Field = "ps.envs"
	// PsHandleNames represents the process handles
	PsHandleNames Field = "ps.handles"
	// PsHandleTypes represents the process handle types
	PsHandleTypes Field = "ps.handle.types"
	// PsDTB represents the process directory table base address
	PsDTB Field = "ps.dtb"
	// PsModuleNames represents the process module file names
	PsModuleNames Field = "ps.modules"
	// PsParentPid represents the parent process identifier field
	PsParentPid Field = "ps.parent.pid"
	// PsParentName represents the parent process name field
	PsParentName Field = "ps.parent.name"
	// PsParentComm represents the parent process command line field. Deprecated
	PsParentComm Field = "ps.parent.comm"
	// PsParentCmdline represents the parent process command line field
	PsParentCmdline Field = "ps.parent.cmdline"
	// PsParentExe represents the parent process image path field
	PsParentExe Field = "ps.parent.exe"
	// PsParentArgs represents the parent process command line arguments field
	PsParentArgs Field = "ps.parent.args"
	// PsParentCwd represents the parent process current working directory field
	PsParentCwd Field = "ps.parent.cwd"
	// PsParentSID represents the parent process security identifier field
	PsParentSID Field = "ps.parent.sid"
	// PsParentUsername represents the parent process username field
	PsParentUsername Field = "ps.parent.username"
	// PsParentDomain represents the parent process domain field
	PsParentDomain Field = "ps.parent.domain"
	// PsParentSessionID represents the session id field bound to the parent process
	PsParentSessionID Field = "ps.parent.sessionid"
	// PsParentEnvs represents the parent process environment variables field
	PsParentEnvs Field = "ps.parent.envs"
	// PsParentHandles represents the parent process handles field
	PsParentHandles Field = "ps.parent.handles"
	// PsParentHandleTypes represents the parent process handle types field
	PsParentHandleTypes Field = "ps.parent.handle.types"
	// PsParentDTB represents the parent process directory table base address field
	PsParentDTB Field = "ps.parent.dtb"
	// PsAncestor represents the process ancestor field
	PsAncestor Field = "ps.ancestor"
	// PsAccessMask represents the process access rights field
	PsAccessMask Field = "ps.access.mask"
	// PsAccessMaskNames represents the process access rights list field
	PsAccessMaskNames Field = "ps.access.mask.names"
	// PsAccessStatus represents the process access status field
	PsAccessStatus Field = "ps.access.status"
	// PsIsWOW64Field represents the field that indicates if the 32-bit process is created in 64-bit Windows system
	PsIsWOW64Field Field = "ps.is_wow64"
	// PsIsPackagedField represents the field that indicates if a process is packaged with the MSIX technology
	PsIsPackagedField Field = "ps.is_packaged"
	// PsIsProtectedField represents the field that indicates if the process is to be run as a protected process
	PsIsProtectedField Field = "ps.is_protected"
	// PsParentIsWOW64Field represents the field that indicates if the 32-bit process is created in 64-bit Windows system
	PsParentIsWOW64Field Field = "ps.parent.is_wow64"
	// PsParentIsPackagedField represents the field that indicates if a process is packaged with the MSIX technology
	PsParentIsPackagedField Field = "ps.parent.is_packaged"
	// PsParentIsProtectedField represents the field that indicates if the process is to be run as a protected process
	PsParentIsProtectedField Field = "ps.parent.is_protected"
	// PsUUID represents the unique process identifier
	PsUUID Field = "ps.uuid"
	// PsParentUUID represents the unique parent process identifier
	PsParentUUID Field = "ps.parent.uuid"
	// PsTokenIntegrityLevel represents the field that indicates the current process integrity level
	PsTokenIntegrityLevel = "ps.token.integrity_level"
	// PsTokenIsElevated represents the field that indicates if the current process token is elevated
	PsTokenIsElevated = "ps.token.is_elevated"
	// PsTokenElevationType represents the field that indicates if the current process token elevation type
	PsTokenElevationType = "ps.token.elevation_type"
	// PsParentTokenIntegrityLevel represents the field that indicates the parent process integrity level
	PsParentTokenIntegrityLevel = "ps.parent.token.integrity_level"
	// PsParentTokenIsElevated represents the field that indicates if the parent process token is elevated
	PsParentTokenIsElevated = "ps.parent.token.is_elevated"
	// PsParentTokenElevationType represents the field that indicates if the parent process token elevation type
	PsParentTokenElevationType = "ps.parent.token.elevation_type"
	// PsSignatureExists is the field which indicates if the binary is signed
	PsSignatureExists Field = "ps.signature.exists"
	// PsSignatureTrusted is the field which indicates if the binary signature is trusted
	PsSignatureTrusted Field = "ps.signature.trusted"
	// PsSignatureIssuer is the field which indicates the certificate issuer
	PsSignatureIssuer Field = "ps.signature.issuer"
	// PsSignatureSubject is the field which indicates the certificate subject
	PsSignatureSubject Field = "ps.signature.subject"
	// PsSignatureSerial is the field which indicates the certificate serial
	PsSignatureSerial Field = "ps.signature.serial"
	// PsSignatureAfter is the field which indicates the timestamp after certificate is no longer valid
	PsSignatureAfter Field = "ps.signature.after"
	// PsSignatureBefore is the field which indicates the timestamp of the certificate enrollment date
	PsSignatureBefore Field = "ps.signature.before"

	// PsPeNumSections represents the number of sections
	PsPeNumSections Field = "ps.pe.nsections"
	// PsPeNumSymbols represents the number of exported symbols
	PsPeNumSymbols Field = "ps.pe.nsymbols"
	// PsPeSymbols represents imported symbols
	PsPeSymbols Field = "ps.pe.symbols"
	// PsPeImports represents imported libraries
	PsPeImports Field = "ps.pe.imports"
	// PsPeTimestamp is the PE build timestamp
	PsPeTimestamp Field = "ps.pe.timestamp"
	// PsPeBaseAddress represents the base address when the binary is loaded
	PsPeBaseAddress Field = "ps.pe.address.base"
	// PsPeEntrypoint is the address of the entrypoint function
	PsPeEntrypoint Field = "ps.pe.address.entrypoint"
	// PsPeResources represents PE resources
	PsPeResources Field = "ps.pe.resources"
	// PsPeCompany represents the company name resource
	PsPeCompany Field = "ps.pe.company"
	// PsPeDescription represents the internal description of the file
	PsPeDescription Field = "ps.pe.description"
	// PsPeFileVersion represents the internal file version
	PsPeFileVersion Field = "ps.pe.file.version"
	// PsPeFileName represents the original file name provided at compile-time
	PsPeFileName Field = "ps.pe.file.name"
	// PsPeCopyright represents the copyright notice emitted at compile-time
	PsPeCopyright Field = "ps.pe.copyright"
	// PsPeProduct represents the product name provided at compile-time
	PsPeProduct Field = "ps.pe.product"
	// PsPeProductVersion represents the internal product version provided at compile-time
	PsPeProductVersion Field = "ps.pe.product.version"
	// PsPeAnomalies represents the field that contains PE anomalies detected during parsing
	PsPeAnomalies Field = "ps.pe.anomalies"
	// PsPeImphash is the field that yields the PE import hash
	PsPeImphash Field = "ps.pe.imphash"
	// PsPeIsDotnet is the field which indicates if the binary contains the .NET assembly
	PsPeIsDotnet Field = "ps.pe.is_dotnet"
	// PsPeIsModified is the field that indicates whether disk and in-memory PE headers differ
	PsPeIsModified Field = "ps.pe.is_modified"

	// ThreadBasePrio is the base thread priority
	ThreadBasePrio Field = "thread.prio"
	// ThreadIOPrio is the thread I/O priority
	ThreadIOPrio Field = "thread.io.prio"
	// ThreadPagePrio is the thread page priority
	ThreadPagePrio Field = "thread.page.prio"
	// ThreadKstackBase is the thread kernel stack start address
	ThreadKstackBase Field = "thread.kstack.base"
	// ThreadKstackLimit is the thread kernel stack end address
	ThreadKstackLimit Field = "thread.kstack.limit"
	// ThreadUstackBase is the thread user stack start address
	ThreadUstackBase Field = "thread.ustack.base"
	// ThreadUstackLimit is the thread user stack end address
	ThreadUstackLimit Field = "thread.ustack.limit"
	// ThreadEntrypoint is the thread entrypoint address
	ThreadEntrypoint Field = "thread.entrypoint"
	// ThreadStartAddress is the thread start address
	ThreadStartAddress Field = "thread.start_address"
	// ThreadPID is the process identifier where the thread is created
	ThreadPID Field = "thread.pid"
	// ThreadTEB is the thread environment block base address
	ThreadTEB Field = "thread.teb_address"
	// ThreadAccessMask represents the thread access rights field
	ThreadAccessMask Field = "thread.access.mask"
	// ThreadAccessMaskNames represents the thread access rights list field
	ThreadAccessMaskNames Field = "thread.access.mask.names"
	// ThreadAccessStatus represents the thread access status field
	ThreadAccessStatus Field = "thread.access.status"
	// ThreadCallstackSummary represents the thread callstack summary field
	ThreadCallstackSummary Field = "thread.callstack.summary"
	// ThreadCallstackDetail represents the thread callstack detail field
	ThreadCallstackDetail Field = "thread.callstack.detail"
	// ThreadCallstackModules represents the callstack modules field
	ThreadCallstackModules Field = "thread.callstack.modules"
	// ThreadCallstackSymbols represents the callstack symbols field
	ThreadCallstackSymbols Field = "thread.callstack.symbols"
	// ThreadCallstackProtections represents the callstack region protections field
	ThreadCallstackProtections Field = "thread.callstack.protections"
	// ThreadCallstackAllocationSizes represents the private region page sizes field
	ThreadCallstackAllocationSizes Field = "thread.callstack.allocation_sizes"
	// ThreadCallstackCallsiteLeadingAssembly represents the callsite prelude opcodes field
	ThreadCallstackCallsiteLeadingAssembly Field = "thread.callstack.callsite_leading_assembly"
	// ThreadCallstackCallsiteTrailingAssembly represents the callsite postlude opcodes field
	ThreadCallstackCallsiteTrailingAssembly Field = "thread.callstack.callsite_trailing_assembly"
	// ThreadCallstackIsUnbacked represents the field that indicates if there is an unbacked stack frame
	ThreadCallstackIsUnbacked Field = "thread.callstack.is_unbacked"
	// ThreadStartAddressSymbol represents the symbol corresponding to the thread start address
	ThreadStartAddressSymbol Field = "thread.start_address.symbol"
	// ThreadStartAddressModule represents the module corresponding to the thread start address
	ThreadStartAddressModule Field = "thread.start_address.module"
	// ThreadCallstackAddresses represents all callstack return addresses
	ThreadCallstackAddresses Field = "thread.callstack.addresses"
	// ThreadCallstackFinalUserModuleName represents the final user space stack frame module name
	ThreadCallstackFinalUserModuleName Field = "thread.callstack.final_user_module.name"
	// ThreadCallstackFinalUserModulePath represents the final user space stack frame module path
	ThreadCallstackFinalUserModulePath Field = "thread.callstack.final_user_module.path"
	// ThreadCallstackFinalUserSymbolName represents the final user space stack frame symbol name
	ThreadCallstackFinalUserSymbolName Field = "thread.callstack.final_user_symbol.name"
	// ThreadCallstackFinalKernelModuleName represents the final kernel space stack frame module name
	ThreadCallstackFinalKernelModuleName Field = "thread.callstack.final_kernel_module.name"
	// ThreadCallstackFinalKernelModulePath represents the final kernel space stack frame module path
	ThreadCallstackFinalKernelModulePath Field = "thread.callstack.final_kernel_module.path"
	// ThreadCallstackFinalKernelSymbolName represents the final kernel space stack frame symbol name
	ThreadCallstackFinalKernelSymbolName Field = "thread.callstack.final_kernel_symbol.name"
	// ThreadCallstackFinalUserModuleSignatureExists represents the signature status of the final user space stack frame module
	ThreadCallstackFinalUserModuleSignatureExists Field = "thread.callstack.final_user_module.signature.exists"
	// ThreadCallstackFinalUserModuleSignatureTrusted represents the trust status of the final user space stack frame module signature
	ThreadCallstackFinalUserModuleSignatureTrusted Field = "thread.callstack.final_user_module.signature.trusted"
	// ThreadCallstackFinalUserModuleSignatureIssuer represents the final user space stack frame module certificate issuer
	ThreadCallstackFinalUserModuleSignatureIssuer Field = "thread.callstack.final_user_module.signature.issuer"
	// ThreadCallstackFinalUserModuleSignatureSubject represents the final user space stack frame module certificate subject
	ThreadCallstackFinalUserModuleSignatureSubject Field = "thread.callstack.final_user_module.signature.subject"

	// PeNumSections represents the number of sections
	PeNumSections Field = "pe.nsections"
	// PeNumSymbols represents the number of exported symbols
	PeNumSymbols Field = "pe.nsymbols"
	// PeSymbols represents imported symbols
	PeSymbols Field = "pe.symbols"
	// PeImports represents imported libraries
	PeImports Field = "pe.imports"
	// PeTimestamp is the PE build timestamp
	PeTimestamp Field = "pe.timestamp"
	// PeBaseAddress represents the base address when the binary is loaded
	PeBaseAddress Field = "pe.address.base"
	// PeEntrypoint is the address of the entrypoint function
	PeEntrypoint Field = "pe.address.entrypoint"
	// PeResources represents PE resources
	PeResources Field = "pe.resources"
	// PeCompany represents the company name resource
	PeCompany Field = "pe.company"
	// PeDescription represents the internal description of the file
	PeDescription Field = "pe.description"
	// PeFileVersion represents the internal file version
	PeFileVersion Field = "pe.file.version"
	// PeFileName represents the original file name provided at compile-time
	PeFileName Field = "pe.file.name"
	// PeCopyright represents the copyright notice emitted at compile-time
	PeCopyright Field = "pe.copyright"
	// PeProduct represents the product name provided at compile-time
	PeProduct Field = "pe.product"
	// PeProductVersion represents the internal product version provided at compile-time
	PeProductVersion Field = "pe.product.version"
	// PeIsDLL indicates if the file is a DLL
	PeIsDLL Field = "pe.is_dll"
	// PeIsDriver indicates if the file is a driver
	PeIsDriver Field = "pe.is_driver"
	// PeIsExecutable indicates if the file is an executable
	PeIsExecutable Field = "pe.is_exec"
	// PeAnomalies represents the field that contains PE anomalies detected during parsing
	PeAnomalies Field = "pe.anomalies"
	// PeImphash is the field that yields the PE import hash
	PeImphash Field = "pe.imphash"
	// PeIsDotnet is the field which indicates if the binary contains the .NET assembly
	PeIsDotnet Field = "pe.is_dotnet"
	// PeIsSigned is the field which indicates if the binary is signed
	PeIsSigned Field = "pe.is_signed"
	// PeIsTrusted is the field which indicates if the binary signature is trusted
	PeIsTrusted Field = "pe.is_trusted"
	// PeCertIssuer is the field which indicates the certificate issuer
	PeCertIssuer Field = "pe.cert.issuer"
	// PeCertSubject is the field which indicates the certificate subject
	PeCertSubject Field = "pe.cert.subject"
	// PeCertSerial is the field which indicates the certificate serial
	PeCertSerial Field = "pe.cert.serial"
	// PeCertAfter is the field which indicates the timestamp after certificate is no longer valid
	PeCertAfter Field = "pe.cert.after"
	// PeCertBefore is the field which indicates the timestamp of the certificate enrollment date
	PeCertBefore Field = "pe.cert.before"
	// PeIsModified is the field that indicates whether disk and in-memory PE headers differ
	PeIsModified Field = "pe.is_modified"

	// EvtSeq is the event sequence number
	EvtSeq Field = "evt.seq"
	// EvtPID is the process identifier that generated the event
	EvtPID Field = "evt.pid"
	// EvtTID is the thread identifier that generated the event
	EvtTID Field = "evt.tid"
	// EvtCPU is the CPU core where the event was generated
	EvtCPU Field = "evt.cpu"
	// EvtDesc represents the event description
	EvtDesc Field = "evt.desc"
	// EvtHost represents the host where the event was produced
	EvtHost Field = "evt.host"
	// EvtTime is the event time
	EvtTime Field = "evt.time"
	// EvtTimeHour is the hour part of the event time
	EvtTimeHour Field = "evt.time.h"
	// EvtTimeMin is the minute part of the event time
	EvtTimeMin Field = "evt.time.m"
	// EvtTimeSec is the second part of the event time
	EvtTimeSec Field = "evt.time.s"
	// EvtTimeNs is the nanosecond part of the event time
	EvtTimeNs Field = "evt.time.ns"
	// EvtDate is the event date
	EvtDate Field = "evt.date"
	// EvtDateDay is the day of event date
	EvtDateDay Field = "evt.date.d"
	// EvtDateMonth is the month of event date
	EvtDateMonth Field = "evt.date.m"
	// EvtDateYear is the year of event date
	EvtDateYear Field = "evt.date.y"
	// EvtDateTz is the time zone of event timestamp
	EvtDateTz Field = "evt.date.tz"
	// EvtDateWeek is the event week number
	EvtDateWeek Field = "evt.date.week"
	// EvtDateWeekday is the event week day
	EvtDateWeekday Field = "evt.date.weekday"
	// EvtName is the event name
	EvtName Field = "evt.name"
	// EvtCategory is the event category
	EvtCategory Field = "evt.category"
	// EvtNparams is the number of event parameters
	EvtNparams Field = "evt.nparams"
	// EvtArg represents the field sequence for generic argument access
	EvtArg Field = "evt.arg"
	// EvtIsDirectSyscall represents the field that designates if this event is performing a direct syscall
	EvtIsDirectSyscall Field = "evt.is_direct_syscall"
	// EvtIsIndirectSyscall represents the field that designates if this event is performing an indirect syscall
	EvtIsIndirectSyscall Field = "evt.is_indirect_syscall"

	// KevtSeq is the event sequence number
	KevtSeq Field = "kevt.seq"
	// KevtPID is the process identifier that generated the event
	KevtPID Field = "kevt.pid"
	// KevtTID is the thread identifier that generated the event
	KevtTID Field = "kevt.tid"
	// KevtCPU is the CPU core where the event was generated
	KevtCPU Field = "kevt.cpu"
	// KevtDesc represents the event description
	KevtDesc Field = "kevt.desc"
	// KevtHost represents the host where the event was produced
	KevtHost Field = "kevt.host"
	// KevtTime is the event time
	KevtTime Field = "kevt.time"
	// KevtTimeHour is the hour part of the event time
	KevtTimeHour Field = "kevt.time.h"
	// KevtTimeMin is the minute part of the event time
	KevtTimeMin Field = "kevt.time.m"
	// KevtTimeSec is the second part of the event time
	KevtTimeSec Field = "kevt.time.s"
	// KevtTimeNs is the nanosecond part of the event time
	KevtTimeNs Field = "kevt.time.ns"
	// KevtDate is the event date
	KevtDate Field = "kevt.date"
	// KevtDateDay is the day of event date
	KevtDateDay Field = "kevt.date.d"
	// KevtDateMonth is the month of event date
	KevtDateMonth Field = "kevt.date.m"
	// KevtDateYear is the year of event date
	KevtDateYear Field = "kevt.date.y"
	// KevtDateTz is the time zone of event timestamp
	KevtDateTz Field = "kevt.date.tz"
	// KevtDateWeek is the event week number
	KevtDateWeek Field = "kevt.date.week"
	// KevtDateWeekday is the event week day
	KevtDateWeekday Field = "kevt.date.weekday"
	// KevtName is the event name
	KevtName Field = "kevt.name"
	// KevtCategory is the event category
	KevtCategory Field = "kevt.category"
	// KevtNparams is the number of event parameters
	KevtNparams Field = "kevt.nparams"
	// KevtArg represents the field sequence for generic argument access
	KevtArg Field = "kevt.arg"

	// HandleID represents the handle identifier within the process address space
	HandleID Field = "handle.id"
	// HandleObject represents the handle object address
	HandleObject Field = "handle.object"
	// HandleName represents the handle name
	HandleName Field = "handle.name"
	// HandleType represents the handle type
	HandleType Field = "handle.type"

	// NetDIP represents network destination IP address
	NetDIP Field = "net.dip"
	// NetSIP represents the source IP address
	NetSIP Field = "net.sip"
	// NetDport represents the destination port
	NetDport Field = "net.dport"
	// NetSport represents the source port
	NetSport Field = "net.sport"
	// NetDportName represents the destination port IANA name
	NetDportName Field = "net.dport.name"
	// NetSportName represents the source port IANA name
	NetSportName Field = "net.sport.name"
	// NetL4Proto represents the Layer4 protocol name
	NetL4Proto Field = "net.l4.proto"
	// NetPacketSize represents the packet size
	NetPacketSize Field = "net.size"
	// NetSIPNames represents the source IP names
	NetSIPNames Field = "net.sip.names"
	// NetDIPNames represents the destination IP names
	NetDIPNames Field = "net.dip.names"

	// FileObject represents the address of the file object
	FileObject Field = "file.object"
	// FileName represents the file base name
	FileName Field = "file.name"
	// FilePath represents the file full path
	FilePath Field = "file.path"
	// FilePathStem represents the full file path without extension
	FilePathStem Field = "file.path.stem"
	// FileExtension represents the file extension
	FileExtension Field = "file.extension"
	// FileOperation represents the file operation
	FileOperation Field = "file.operation"
	// FileShareMask represents the file share mask
	FileShareMask Field = "file.share.mask"
	// FileIOSize represents the number of read/written bytes
	FileIOSize Field = "file.io.size"
	// FileOffset represents the read/write offset
	FileOffset Field = "file.offset"
	// FileType represents the file type
	FileType Field = "file.type"
	// FileAttributes represents a slice of file attributes
	FileAttributes Field = "file.attributes"
	// FileStatus represents the status message of the file operation
	FileStatus Field = "file.status"
	// FileViewBase represents the base address of the mapped view
	FileViewBase Field = "file.view.base"
	// FileViewSize represents the size of the mapped view
	FileViewSize Field = "file.view.size"
	// FileViewType represents the type of the mapped view section
	FileViewType Field = "file.view.type"
	// FileViewProtection represents the protection attributes of the section view
	FileViewProtection Field = "file.view.protection"
	// FileIsDriverVulnerable represents the field that denotes whether the created file is a vulnerable driver
	FileIsDriverVulnerable Field = "file.is_driver_vulnerable"
	// FileIsDriverMalicious represents the field that denotes whether the created file is a malicious driver
	FileIsDriverMalicious Field = "file.is_driver_malicious"
	// FileIsDLL indicates if the created file is a DLL
	FileIsDLL Field = "file.is_dll"
	// FileIsDriver indicates if the created file is a driver
	FileIsDriver Field = "file.is_driver"
	// FileIsExecutable indicates if the created file is an executable
	FileIsExecutable Field = "file.is_exec"
	// FilePID represents the field that denotes the process id performing file operations
	FilePID Field = "file.pid"
	// FileKey represents the field that uniquely identifies the file object
	FileKey Field = "file.key"
	// FileInfoClass represents the field that identifies the file information class
	FileInfoClass Field = "file.info_class"
	// FileInfoAllocationSize represents the field that contains the file allocation size
	FileInfoAllocationSize Field = "file.info.allocation_size"
	// FileInfoEOFSize represents the field that contains the EOF size
	FileInfoEOFSize Field = "file.info.eof_size"
	// FileInfoIsDispositionDeleteFile represents the field that indicates if the file is deleted when its handle is closed
	FileInfoIsDispositionDeleteFile Field = "file.info.is_disposition_delete_file"

	// RegistryPath represents the full registry path
	RegistryPath Field = "registry.path"
	// RegistryKeyName represents the registry key name
	RegistryKeyName Field = "registry.key.name"
	// RegistryKeyHandle represents the registry KCB address
	RegistryKeyHandle Field = "registry.key.handle"
	// RegistryValue represents the registry value name field
	RegistryValue Field = "registry.value"
	// RegistryValueType represents the registry value type field
	RegistryValueType Field = "registry.value.type"
	// RegistryData represents the captured registry data field
	RegistryData Field = "registry.data"
	// RegistryStatus represent the registry operation status
	RegistryStatus Field = "registry.status"

	// ImageBase is the module base address
	ImageBase Field = "image.base.address"
	// ImageSize is the module size
	ImageSize Field = "image.size"
	// ImageChecksum represents the module checksum hash
	ImageChecksum Field = "image.checksum"
	// ImageDefaultAddress represents the module address
	ImageDefaultAddress Field = "image.default.address"
	// ImagePath is the module full path
	ImagePath Field = "image.path"
	// ImageName is the module name
	ImageName Field = "image.name"
	// ImagePID is the pid of the process where the image was loaded
	ImagePID Field = "image.pid"
	// ImageSignatureType represents the image signature type
	ImageSignatureType Field = "image.signature.type"
	// ImageSignatureLevel represents the image signature level
	ImageSignatureLevel Field = "image.signature.level"
	// ImageCertSubject is the field that indicates the subject of the certificate
	ImageCertSubject = "image.cert.subject"
	// ImageCertIssuer is the field that represents the certificate authority (CA)
	ImageCertIssuer = "image.cert.issuer"
	// ImageCertSerial is the field that represents the serial number
	ImageCertSerial = "image.cert.serial"
	// ImageCertBefore is the field that specifies the certificate won't be valid before this timestamp
	ImageCertBefore = "image.cert.before"
	// ImageCertAfter is the field that specifies the certificate won't be valid after this timestamp
	ImageCertAfter = "image.cert.after"
	// ImageIsDriverVulnerable represents the field that denotes whether loaded driver is vulnerable
	ImageIsDriverVulnerable Field = "image.is_driver_vulnerable"
	// ImageIsDriverMalicious represents the field that denotes whether the loaded driver is malicious
	ImageIsDriverMalicious Field = "image.is_driver_malicious"
	// ImageIsDLL indicates if the loaded image is a DLL
	ImageIsDLL Field = "image.is_dll"
	// ImageIsDriver indicates if the loaded image is a driver
	ImageIsDriver Field = "image.is_driver"
	// ImageIsExecutable indicates if the loaded image is an executable
	ImageIsExecutable Field = "image.is_exec"
	// ImageIsDotnet indicates if the loaded image is a .NET assembly
	ImageIsDotnet Field = "image.is_dotnet"

	// DllBase is the DLL base address
	DllBase Field = "dll.base"
	// DllSize is the DLL virtual mapped space size
	DllSize Field = "dll.size"
	// DllPath is the DLL full path
	DllPath Field = "dll.path"
	// DllPathStem is the DLL path stem field
	DllPathStem Field = "dll.path.stem"
	// DllName is the DLL name
	DllName Field = "dll.name"
	// DllPID is the pid of the process where the DLL was loaded
	DllPID Field = "dll.pid"
	// DllSignatureType represents the DLL signature type
	DllSignatureType Field = "dll.signature.type"
	// DllSignatureLevel represents the DLL signature level
	DllSignatureLevel Field = "dll.signature.level"
	// DllSignatureExists is the field that determines if the DLL signature exists
	DllSignatureExists Field = "dll.signature.exists"
	// DllSignatureTrusted is the field that determines if the DLL signature is trusted
	DllSignatureTrusted Field = "dll.signature.trusted"
	// DllSignatureSubject is the field that indicates the subject of the certificate
	DllSignatureSubject = "dll.signature.subject"
	// DllSignatureIssuer is the field that represents the certificate authority (CA)
	DllSignatureIssuer = "dll.signature.issuer"
	// DllSignatureSerial is the field that represents the serial number
	DllSignatureSerial = "dll.signature.serial"
	// DllSignatureBefore is the field that specifies the certificate won't be valid before this timestamp
	DllSignatureBefore = "dll.signature.before"
	// DllSignatureAfter is the field that specifies the certificate won't be valid after this timestamp
	DllSignatureAfter = "dll.signature.after"
	// DllIsDotnet indicates if the DLL is a .NET assembly
	DllIsDotnet Field = "dll.pe.is_dotnet"

	// ModuleBase is the module base address
	ModuleBase Field = "module.base"
	// ModuleSize is the module size
	ModuleSize Field = "module.size"
	// ModuleChecksum represents the module checksum hash
	ModuleChecksum Field = "module.checksum"
	// ModuleDefaultAddress represents the module address
	ModuleDefaultAddress Field = "module.default_address"
	// ModulePath is the module full path
	ModulePath Field = "module.path"
	// ModulePathStem is the module path stem field
	ModulePathStem Field = "module.path.stem"
	// ModuleName is the module name
	ModuleName Field = "module.name"
	// ModulePID is the pid of the process where the module was loaded
	ModulePID Field = "module.pid"
	// ModuleSignatureType represents the module signature type
	ModuleSignatureType Field = "module.signature.type"
	// ModuleSignatureLevel represents the module signature level
	ModuleSignatureLevel Field = "module.signature.level"
	// ModuleSignatureExists is the field that determines if the module signature exists
	ModuleSignatureExists Field = "module.signature.exists"
	// ModuleSignatureTrusted is the field that determines if the module signature is trusted
	ModuleSignatureTrusted Field = "module.signature.trusted"
	// ModuleSignatureSubject is the field that indicates the subject of the certificate
	ModuleSignatureSubject = "module.signature.subject"
	// ModuleSignatureIssuer is the field that represents the certificate authority (CA)
	ModuleSignatureIssuer = "module.signature.issuer"
	// ModuleSignatureSerial is the field that represents the serial number
	ModuleSignatureSerial = "module.signature.serial"
	// ModuleSignatureBefore is the field that specifies the certificate won't be valid before this timestamp
	ModuleSignatureBefore = "module.signature.before"
	// ModuleSignatureAfter is the field that specifies the certificate won't be valid after this timestamp
	ModuleSignatureAfter = "module.signature.after"
	// ModuleIsDriverVulnerable represents the field that denotes whether loaded driver is vulnerable
	ModuleIsDriverVulnerable Field = "module.is_driver_vulnerable"
	// ModuleIsDriverMalicious represents the field that denotes whether the loaded driver is malicious
	ModuleIsDriverMalicious Field = "module.is_driver_malicious"
	// ModuleIsDLL indicates if the loaded module is a DLL
	ModuleIsDLL Field = "module.is_dll"
	// ModuleIsDriver indicates if the loaded module is a driver
	ModuleIsDriver Field = "module.is_driver"
	// ModuleIsExecutable indicates if the loaded module is an executable
	ModuleIsExecutable Field = "module.is_exec"
	// ModuleIsDotnet indicates if the loaded module is a .NET assembly
	ModuleIsDotnet Field = "module.pe.is_dotnet"

	// MemBaseAddress identifies the field that denotes the allocation base address
	MemBaseAddress Field = "mem.address"
	// MemRegionSize identifies the field that represents the allocated region size
	MemRegionSize Field = "mem.size"
	// MemAllocType identifies the field that represents region allocation type
	MemAllocType Field = "mem.alloc"
	// MemPageType identifies the parameter that represents the allocated region type
	MemPageType Field = "mem.type"
	// MemProtection identifies the field that represents the memory protection for the range of pages
	MemProtection Field = "mem.protection"
	// MemProtectionMask identifies the field that represents the memory protection in mask notation
	MemProtectionMask Field = "mem.protection.mask"

	// DNSName identifies the field that represents the DNS name
	DNSName Field = "dns.name"
	// DNSRR identifies the field that represents the DNS record type
	DNSRR Field = "dns.rr"
	// DNSOptions identifies the field that represents the DNS options
	DNSOptions Field = "dns.options"
	// DNSAnswers identifies the field that represents the DNS answers
	DNSAnswers Field = "dns.answers"
	// DNSRcode identifies the field that represents the DNS response code
	DNSRcode Field = "dns.rcode"

	// ThreadpoolPoolID identifies the field that represents the thread pool identifier
	ThreadpoolPoolID = "threadpool.id"
	// ThreadpoolTaskID identifies the field that represents the thread pool task identifier
	ThreadpoolTaskID = "threadpool.task.id"
	// ThreadpoolCallbackAddress identifies the field that represents the address of the callback function
	ThreadpoolCallbackAddress = "threadpool.callback.address"
	// ThreadpoolCallbackSymbol identifies the field that represents the callback symbol
	ThreadpoolCallbackSymbol = "threadpool.callback.symbol"
	// ThreadpoolCallbackModule identifies the field that represents the module containing the callback symbol
	ThreadpoolCallbackModule = "threadpool.callback.module"
	// ThreadpoolCallbackContext identifies the field that represents the address of the callback context
	ThreadpoolCallbackContext = "threadpool.callback.context"
	// ThreadpoolCallbackContextRip identifies the field that represents the value of instruction pointer contained in the callback context
	ThreadpoolCallbackContextRip = "threadpool.callback.context.rip"
	// ThreadpoolCallbackContextRipSymbol identifies the field that represents the symbol name associated with the instruction pointer in callback context
	ThreadpoolCallbackContextRipSymbol = "threadpool.callback.context.rip.symbol"
	// ThreadpoolCallbackContextRipModule identifies the field that represents the module name associated with the instruction pointer in callback context
	ThreadpoolCallbackContextRipModule = "threadpool.callback.context.rip.module"
	// ThreadpoolSubprocessTag identifies the field that represents the service identifier associated with the thread pool
	ThreadpoolSubprocessTag = "threadpool.subprocess_tag"
	// ThreadpoolTimerDuetime identifies the field that represents the timer due time
	ThreadpoolTimerDuetime = "threadpool.timer.duetime"
	// ThreadpoolTimerSubqueue identifies the field that represents the memory address of the timer subqueue
	ThreadpoolTimerSubqueue = "threadpool.timer.subqueue"
	// ThreadpoolTimer identifies the field that represents the memory address of the timer object
	ThreadpoolTimer = "threadpool.timer.address"
	// ThreadpoolTimerPeriod identifies the field that represents the period of the timer
	ThreadpoolTimerPeriod = "threadpool.timer.period"
	// ThreadpoolTimerWindow identifies the field that represents the timer tolerate period
	ThreadpoolTimerWindow = "threadpool.timer.window"
	// ThreadpoolTimerAbsolute identifies the field that indicates if the timer is absolute or relative
	ThreadpoolTimerAbsolute = "threadpool.timer.is_absolute"

	// EventLogChannel identifies the Windows Event Log channel field
	EventLogChannel Field = "eventlog.channel"
	// EventLogProvider identifies the Windows Event Log provider field
	EventLogProvider Field = "eventlog.provider"
	// EventLogEventID identifies the Windows Event Log event ID field
	EventLogEventID Field = "eventlog.event.id"
	// EventLogLevel identifies the Windows Event Log level field
	EventLogLevel Field = "eventlog.level"
	// EventLogLevelID identifies the Windows Event Log level ID field
	EventLogLevelID Field = "eventlog.level.id"
	// EventLogRecordID identifies the Windows Event Log record ID field
	EventLogRecordID Field = "eventlog.record.id"
	// EventLogTask identifies the Windows Event Log task field
	EventLogTask Field = "eventlog.task"
	// EventLogOpcode identifies the Windows Event Log opcode field
	EventLogOpcode Field = "eventlog.opcode"
	// EventLogKeywords identifies the Windows Event Log keywords field
	EventLogKeywords Field = "eventlog.keywords"
	// EventLogComputer identifies the Windows Event Log computer name field
	EventLogComputer Field = "eventlog.computer"
	// EventLogUserID identifies the Windows Event Log user SID field
	EventLogUserID Field = "eventlog.user.id"
	// EventLogData identifies the Windows Event Log data field (accessed by name)
	EventLogData Field = "eventlog.data"
)

// Type returns the data type that this field contains.
func (f Field) Type() ParamType { return fields[f].Type }

func (f Field) IsPsField() bool       { return strings.HasPrefix(string(f), "ps.") }
func (f Field) IsKevtField() bool     { return strings.HasPrefix(string(f), "kevt.") }
func (f Field) IsEvtField() bool      { return strings.HasPrefix(string(f), "evt.") }
func (f Field) IsThreadField() bool   { return strings.HasPrefix(string(f), "thread.") }
func (f Field) IsImageField() bool    { return strings.HasPrefix(string(f), "image.") }
func (f Field) IsFileField() bool     { return strings.HasPrefix(string(f), "file.") }
func (f Field) IsRegistryField() bool { return strings.HasPrefix(string(f), "registry.") }
func (f Field) IsNetworkField() bool  { return strings.HasPrefix(string(f), "net.") }
func (f Field) IsHandleField() bool   { return strings.HasPrefix(string(f), "handle.") }
func (f Field) IsPeField() bool {
	return strings.HasPrefix(string(f), "pe.") || strings.HasPrefix(string(f), "ps.pe.") || strings.HasPrefix(string(f), "ps.signature.")
}
func (f Field) IsModuleField() bool {
	return strings.HasPrefix(string(f), "module.") || strings.HasPrefix(string(f), "dll.")
}
func (f Field) IsMemField() bool        { return strings.HasPrefix(string(f), "mem.") }
func (f Field) IsDNSField() bool        { return strings.HasPrefix(string(f), "dns.") }
func (f Field) IsThreadpoolField() bool { return strings.HasPrefix(string(f), "threadpool.") }
func (f Field) IsEventLogField() bool  { return strings.HasPrefix(string(f), "eventlog.") }

func (f Field) IsPeSection() bool { return f == PeNumSections || f == PsPeNumSections }
func (f Field) IsPeSymbol() bool {
	return f == PeSymbols || f == PeNumSymbols || f == PeImports || f == PsPeSymbols || f == PsPeNumSymbols || f == PsPeImports
}
func (f Field) IsPeVersionResource() bool {
	return f == PeCompany || f == PeCopyright || f == PeDescription || f == PeFileName || f == PeFileVersion || f == PeProduct || f == PeProductVersion ||
		f == PsPeCompany || f == PsPeCopyright || f == PsPeDescription || f == PsPeFileName || f == PsPeFileVersion || f == PsPeProduct || f == PsPeProductVersion
}
func (f Field) IsPeVersionResources() bool { return f == PeResources || f == PsPeResources }
func (f Field) IsPeImphash() bool          { return f == PeImphash || f == PsPeImphash }
func (f Field) IsPeDotnet() bool           { return f == PeIsDotnet || f == PsPeIsDotnet }
func (f Field) IsPeAnomalies() bool        { return f == PeAnomalies || f == PsPeAnomalies }
func (f Field) IsPeSignature() bool {
	return f == PeIsTrusted || f == PeIsSigned || f == PeCertIssuer || f == PeCertSerial || f == PeCertSubject || f == PeCertBefore || f == PeCertAfter || strings.HasPrefix(string(f), "ps.signature.")
}
func (f Field) IsPeIsTrusted() bool { return f == PeIsTrusted || f == PsSignatureTrusted }
func (f Field) IsPeIsSigned() bool  { return f == PeIsSigned || f == PsSignatureExists }

func (f Field) IsPeCert() bool {
	return strings.HasPrefix(string(f), "pe.cert.") || f == PsSignatureIssuer || f == PsSignatureSubject || f == PsSignatureSerial || f == PsSignatureAfter || f == PsSignatureBefore
}
func (f Field) IsImageCert() bool { return strings.HasPrefix(string(f), "image.cert.") }
func (f Field) IsModuleCert() bool {
	return f == ModuleSignatureSubject || f == ModuleSignatureIssuer || f == ModuleSignatureSerial || f == ModuleSignatureAfter || f == ModuleSignatureBefore ||
		f == DllSignatureSubject || f == DllSignatureIssuer || f == DllSignatureSerial || f == DllSignatureAfter || f == DllSignatureBefore
}
func (f Field) IsModuleSignature() bool {
	return strings.HasPrefix(string(f), "module.signature.") || strings.HasPrefix(string(f), "dll.signature.")
}

func (f Field) IsPeModified() bool { return f == PeIsModified || f == PsPeIsModified }

// Segment represents the type alias for the segment.
type Segment string

const (
	PathSegment     Segment = "path"
	NameSegment     Segment = "name"
	TypeSegment     Segment = "type"
	SizeSegment     Segment = "size"
	ChecksumSegment Segment = "checksum"
	AddressSegment  Segment = "address"
	OffsetSegment   Segment = "offset"
	EntropySegment  Segment = "entropy"
	MD5Segment      Segment = "md5"

	PIDSegment                 Segment = "pid"
	CmdlineSegment             Segment = "cmdline"
	ExeSegment                 Segment = "exe"
	ArgsSegment                Segment = "args"
	CwdSegment                 Segment = "cwd"
	SIDSegment                 Segment = "sid"
	SessionIDSegment           Segment = "sessionid"
	UsernameSegment            Segment = "username"
	DomainSegment              Segment = "domain"
	TokenIntegrityLevelSegment Segment = "token.integrity_level"
	TokenIsElevatedSegment     Segment = "token.is_elevated"
	TokenElevationTypeSegment  Segment = "token.elevation_type"

	TidSegment              Segment = "tid"
	StartAddressSegment     Segment = "start_address"
	UserStackBaseSegment    Segment = "user_stack_base"
	UserStackLimitSegment   Segment = "user_stack_limit"
	KernelStackBaseSegment  Segment = "kernel_stack_base"
	KernelStackLimitSegment Segment = "kernel_stack_limit"

	SymbolSegment                   Segment = "symbol"
	ModuleSegment                   Segment = "module"
	AllocationSizeSegment           Segment = "allocation_size"
	ProtectionSegment               Segment = "protection"
	IsUnbackedSegment               Segment = "is_unbacked"
	CallsiteLeadingAssemblySegment  Segment = "callsite_leading_assembly"
	CallsiteTrailingAssemblySegment Segment = "callsite_trailing_assembly"

	ModuleSignatureExistsSegment  Segment = "module.signature.exists"
	ModuleSignatureTrustedSegment Segment = "module.signature.trusted"
	ModuleSignatureIssuerSegment  Segment = "module.signature.issuer"
	ModuleSignatureSubjectSegment Segment = "module.signature.subject"
)

var segments = map[Segment]bool{
	NameSegment:                     true,
	PathSegment:                     true,
	TypeSegment:                     true,
	EntropySegment:                  true,
	SizeSegment:                     true,
	MD5Segment:                      true,
	AddressSegment:                  true,
	ChecksumSegment:                 true,
	PIDSegment:                      true,
	CmdlineSegment:                  true,
	ExeSegment:                      true,
	ArgsSegment:                     true,
	CwdSegment:                      true,
	SIDSegment:                      true,
	SessionIDSegment:                true,
	UsernameSegment:                 true,
	DomainSegment:                   true,
	TokenIntegrityLevelSegment:      true,
	TokenIsElevatedSegment:          true,
	TokenElevationTypeSegment:       true,
	TidSegment:                      true,
	StartAddressSegment:             true,
	UserStackBaseSegment:            true,
	UserStackLimitSegment:           true,
	KernelStackBaseSegment:          true,
	KernelStackLimitSegment:         true,
	OffsetSegment:                   true,
	SymbolSegment:                   true,
	ModuleSegment:                   true,
	AllocationSizeSegment:           true,
	ProtectionSegment:               true,
	IsUnbackedSegment:               true,
	CallsiteLeadingAssemblySegment:  true,
	CallsiteTrailingAssemblySegment: true,
	ModuleSignatureExistsSegment:    true,
	ModuleSignatureTrustedSegment:   true,
	ModuleSignatureIssuerSegment:    true,
	ModuleSignatureSubjectSegment:   true,
}

// Pseudo fields provide access to the process/event internal state.
var (
	PsModules       Field = "ps._modules"
	PsThreads       Field = "ps._threads"
	PsMmaps         Field = "ps._mmaps"
	PsAncestors     Field = "ps._ancestors"
	PsPeSections    Field = "ps.pe._sections"
	ThreadCallstack Field = "thread._callstack"
	PeSections      Field = "pe._sections"
)

var allowedSegments = map[Field][]Segment{
	PsAncestors:     {NameSegment, PIDSegment, CmdlineSegment, ExeSegment, ArgsSegment, CwdSegment, SIDSegment, SessionIDSegment, UsernameSegment, DomainSegment, TokenIntegrityLevelSegment, TokenIsElevatedSegment, TokenElevationTypeSegment},
	PsThreads:       {TidSegment, StartAddressSegment, UserStackBaseSegment, UserStackLimitSegment, KernelStackBaseSegment, KernelStackLimitSegment},
	PsModules:       {PathSegment, NameSegment, AddressSegment, SizeSegment, ChecksumSegment},
	PsMmaps:         {AddressSegment, TypeSegment, SizeSegment, ProtectionSegment, PathSegment},
	PeSections:      {NameSegment, SizeSegment, EntropySegment, MD5Segment},
	PsPeSections:    {NameSegment, SizeSegment, EntropySegment, MD5Segment},
	ThreadCallstack: {AddressSegment, OffsetSegment, SymbolSegment, ModuleSegment, AllocationSizeSegment, ProtectionSegment, IsUnbackedSegment, CallsiteLeadingAssemblySegment, CallsiteTrailingAssemblySegment, ModuleSignatureExistsSegment, ModuleSignatureTrustedSegment, ModuleSignatureIssuerSegment, ModuleSignatureSubjectSegment},
}

func (s Segment) IsEntropy() bool { return s == EntropySegment }

func IsPseudoField(f Field) bool {
	return f == PsAncestors || f == PsModules || f == PsThreads || f == PsMmaps || f == ThreadCallstack || f == PeSections || f == PsPeSections
}

func (f Field) IsPeSectionsPseudo() bool { return f == PeSections || f == PsPeSections }

// IsSegmentAllowed determines if the segment is valid for the pseudo field.
func IsSegmentAllowed(f Field, s Segment) bool {
	segs := allowedSegments[f]
	if len(segs) == 0 {
		return false
	}

	for _, seg := range segs {
		if seg == s {
			return true
		}
	}

	return false
}

// SegmentsHint returns the sequence of available segments for the pseudo field.
func SegmentsHint(f Field) string {
	segs := allowedSegments[f]
	if len(segs) == 0 {
		return ""
	}

	s := make([]string, len(segs))
	for i, seg := range segs {
		s[i] = string(seg)
	}

	return strings.Join(s, ", ")
}

// IsSegment indicates if the given string is recognized as a known segment.
func IsSegment(s string) bool {
	return segments[Segment(s)]
}

var fields = map[Field]FieldInfo{
	EvtSeq:         {EvtSeq, "event sequence number", ParamUint64, []string{"evt.seq > 666"}, nil, nil},
	EvtPID:         {EvtPID, "process identifier generating the event", ParamUint32, []string{"evt.pid = 6"}, nil, nil},
	EvtTID:         {EvtTID, "thread identifier generating the event", ParamUint32, []string{"evt.tid = 1024"}, nil, nil},
	EvtCPU:         {EvtCPU, "logical processor core where the event was generated", ParamUint8, []string{"evt.cpu = 2"}, nil, nil},
	EvtName:        {EvtName, "symbolical event name", ParamAnsiString, []string{"evt.name = 'CreateThread'"}, nil, nil},
	EvtCategory:    {EvtCategory, "event category", ParamAnsiString, []string{"evt.category = 'registry'"}, nil, nil},
	EvtDesc:        {EvtDesc, "event description", ParamAnsiString, []string{"evt.desc contains 'Creates a new process'"}, nil, nil},
	EvtHost:        {EvtHost, "host name on which the event was produced", ParamUnicodeString, []string{"evt.host contains 'kitty'"}, nil, nil},
	EvtTime:        {EvtTime, "event timestamp as a time string", ParamTime, []string{"evt.time = '17:05:32'"}, nil, nil},
	EvtTimeHour:    {EvtTimeHour, "hour within the day on which the event occurred", ParamTime, []string{"evt.time.h = 23"}, nil, nil},
	EvtTimeMin:     {EvtTimeMin, "minute offset within the hour on which the event occurred", ParamTime, []string{"evt.time.m = 54"}, nil, nil},
	EvtTimeSec:     {EvtTimeSec, "second offset within the minute  on which the event occurred", ParamTime, []string{"evt.time.s = 0"}, nil, nil},
	EvtTimeNs:      {EvtTimeNs, "nanoseconds specified by event timestamp", ParamInt64, []string{"evt.time.ns > 1591191629102337000"}, nil, nil},
	EvtDate:        {EvtDate, "event timestamp as a date string", ParamTime, []string{"evt.date = '2018-03-03'"}, nil, nil},
	EvtDateDay:     {EvtDateDay, "day of the month on which the event occurred", ParamTime, []string{"evt.date.d = 12"}, nil, nil},
	EvtDateMonth:   {EvtDateMonth, "month of the year on which the event occurred", ParamTime, []string{"evt.date.m = 11"}, nil, nil},
	EvtDateYear:    {EvtDateYear, "year on which the event occurred", ParamUint32, []string{"evt.date.y = 2020"}, nil, nil},
	EvtDateTz:      {EvtDateTz, "time zone associated with the event timestamp", ParamAnsiString, []string{"evt.date.tz = 'UTC'"}, nil, nil},
	EvtDateWeek:    {EvtDateWeek, "week number within the year on which the event occurred", ParamUint8, []string{"evt.date.week = 2"}, nil, nil},
	EvtDateWeekday: {EvtDateWeekday, "week day on which the event occurred", ParamAnsiString, []string{"evt.date.weekday = 'Monday'"}, nil, nil},
	EvtNparams:     {EvtNparams, "number of parameters", ParamInt8, []string{"evt.nparams > 2"}, nil, nil},
	EvtArg: {EvtArg, "event parameter", ParamObject, []string{"evt.arg[cmdline] istartswith 'C:\\Windows'"}, nil, &Argument{Optional: false, Pattern: "[a-z0-9_]+", ValidationFunc: func(s string) bool {
		for _, c := range s {
			switch {
			case unicode.IsLower(c):
			case unicode.IsNumber(c):
			case c == '_':
			default:
				return false
			}
		}
		return true
	}}},
	EvtIsDirectSyscall:   {EvtIsDirectSyscall, "indicates if the event is performing a direct syscall", ParamBool, []string{"evt.is_direct_syscall = true"}, nil, nil},
	EvtIsIndirectSyscall: {EvtIsIndirectSyscall, "indicates if the event is performing an indirect syscall", ParamBool, []string{"evt.is_indirect_syscall = true"}, nil, nil},

	KevtSeq:         {KevtSeq, "event sequence number", ParamUint64, []string{"kevt.seq > 666"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtSeq}}, nil},
	KevtPID:         {KevtPID, "process identifier generating the event", ParamUint32, []string{"kevt.pid = 6"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtPID}}, nil},
	KevtTID:         {KevtTID, "thread identifier generating the event", ParamUint32, []string{"kevt.tid = 1024"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtTID}}, nil},
	KevtCPU:         {KevtCPU, "logical processor core where the event was generated", ParamUint8, []string{"kevt.cpu = 2"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtCPU}}, nil},
	KevtName:        {KevtName, "symbolical event name", ParamAnsiString, []string{"kevt.name = 'CreateThread'"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtName}}, nil},
	KevtCategory:    {KevtCategory, "event category", ParamAnsiString, []string{"kevt.category = 'registry'"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtCategory}}, nil},
	KevtDesc:        {KevtDesc, "event description", ParamAnsiString, []string{"kevt.desc contains 'Creates a new process'"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtDesc}}, nil},
	KevtHost:        {KevtHost, "host name on which the event was produced", ParamUnicodeString, []string{"kevt.host contains 'kitty'"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtHost}}, nil},
	KevtTime:        {KevtTime, "event timestamp as a time string", ParamTime, []string{"kevt.time = '17:05:32'"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtTime}}, nil},
	KevtTimeHour:    {KevtTimeHour, "hour within the day on which the event occurred", ParamTime, []string{"kevt.time.h = 23"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtTimeHour}}, nil},
	KevtTimeMin:     {KevtTimeMin, "minute offset within the hour on which the event occurred", ParamTime, []string{"kevt.time.m = 54"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtTimeMin}}, nil},
	KevtTimeSec:     {KevtTimeSec, "second offset within the minute  on which the event occurred", ParamTime, []string{"kevt.time.s = 0"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtTimeSec}}, nil},
	KevtTimeNs:      {KevtTimeNs, "nanoseconds specified by event timestamp", ParamInt64, []string{"kevt.time.ns > 1591191629102337000"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtTimeNs}}, nil},
	KevtDate:        {KevtDate, "event timestamp as a date string", ParamTime, []string{"kevt.date = '2018-03-03'"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtDate}}, nil},
	KevtDateDay:     {KevtDateDay, "day of the month on which the event occurred", ParamTime, []string{"kevt.date.d = 12"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtDateDay}}, nil},
	KevtDateMonth:   {KevtDateMonth, "month of the year on which the event occurred", ParamTime, []string{"kevt.date.m = 11"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtDateMonth}}, nil},
	KevtDateYear:    {KevtDateYear, "year on which the event occurred", ParamUint32, []string{"kevt.date.y = 2020"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtDateYear}}, nil},
	KevtDateTz:      {KevtDateTz, "time zone associated with the event timestamp", ParamAnsiString, []string{"kevt.date.tz = 'UTC'"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtDateTz}}, nil},
	KevtDateWeek:    {KevtDateWeek, "week number within the year on which the event occurred", ParamUint8, []string{"kevt.date.week = 2"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtDateWeek}}, nil},
	KevtDateWeekday: {KevtDateWeekday, "week day on which the event occurred", ParamAnsiString, []string{"kevt.date.weekday = 'Monday'"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtDateWeekday}}, nil},
	KevtNparams:     {KevtNparams, "number of parameters", ParamInt8, []string{"kevt.nparams > 2"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtNparams}}, nil},
	KevtArg: {KevtArg, "event parameter", ParamObject, []string{"kevt.arg[cmdline] istartswith 'C:\\Windows'"}, &Deprecation{Since: "3.0.0", Fields: []Field{EvtArg}}, &Argument{Optional: false, Pattern: "[a-z0-9_]+", ValidationFunc: func(s string) bool {
		for _, c := range s {
			switch {
			case unicode.IsLower(c):
			case unicode.IsNumber(c):
			case c == '_':
			default:
				return false
			}
		}
		return true
	}}},

	PsPid:                       {PsPid, "process identifier", ParamPID, []string{"ps.pid = 1024"}, nil, nil},
	PsPpid:                      {PsPpid, "parent process identifier", ParamPID, []string{"ps.ppid = 45"}, nil, nil},
	PsName:                      {PsName, "process image name including the file extension", ParamUnicodeString, []string{"ps.name contains 'firefox'"}, nil, nil},
	PsComm:                      {PsComm, "process command line", ParamUnicodeString, []string{"ps.comm contains 'java'"}, &Deprecation{Since: "1.10.0", Fields: []Field{PsCmdline}}, nil},
	PsCmdline:                   {PsCmdline, "process command line", ParamUnicodeString, []string{"ps.cmdline contains 'java'"}, nil, nil},
	PsExe:                       {PsExe, "full name of the process' executable", ParamUnicodeString, []string{"ps.exe = 'C:\\Windows\\system32\\cmd.exe'"}, nil, nil},
	PsArgs:                      {PsArgs, "process command line arguments", ParamSlice, []string{"ps.args in ('/cdir', '/-C')"}, nil, nil},
	PsCwd:                       {PsCwd, "process current working directory", ParamUnicodeString, []string{"ps.cwd = 'C:\\Users\\Default'"}, nil, nil},
	PsSID:                       {PsSID, "security identifier under which this process is run", ParamUnicodeString, []string{"ps.sid contains 'SYSTEM'"}, nil, nil},
	PsSessionID:                 {PsSessionID, "unique identifier for the current session", ParamInt16, []string{"ps.sessionid = 1"}, nil, nil},
	PsDomain:                    {PsDomain, "process domain", ParamUnicodeString, []string{"ps.domain contains 'SERVICE'"}, nil, nil},
	PsUsername:                   {PsUsername, "process username", ParamUnicodeString, []string{"ps.username contains 'system'"}, nil, nil},
	PsEnvs:                      {PsEnvs, "process environment variables", ParamSlice, []string{"ps.envs in ('SystemRoot:C:\\WINDOWS')", "ps.envs[windir] = 'C:\\WINDOWS'"}, nil, &Argument{Optional: true, ValidationFunc: func(arg string) bool { return true }}},
	PsHandleNames:               {PsHandleNames, "allocated process handle names", ParamSlice, []string{"ps.handles in ('\\BaseNamedObjects\\__ComCatalogCache__')"}, nil, nil},
	PsHandleTypes:               {PsHandleTypes, "allocated process handle types", ParamSlice, []string{"ps.handle.types in ('Key', 'Mutant', 'Section')"}, nil, nil},
	PsDTB:                       {PsDTB, "process directory table base address", ParamAddress, []string{"ps.dtb = '7ffe0000'"}, nil, nil},
	PsModuleNames:               {PsModuleNames, "modules loaded by the process", ParamSlice, []string{"ps.modules in ('crypt32.dll', 'xul.dll')"}, nil, nil},
	PsParentName:                {PsParentName, "parent process image name including the file extension", ParamUnicodeString, []string{"ps.parent.name contains 'cmd.exe'"}, nil, nil},
	PsParentPid:                 {PsParentPid, "parent process id", ParamUint32, []string{"ps.parent.pid = 4"}, nil, nil},
	PsParentComm:                {PsParentComm, "parent process command line", ParamUnicodeString, []string{"ps.parent.comm contains 'java'"}, &Deprecation{Since: "1.10.0", Fields: []Field{PsParentCmdline}}, nil},
	PsParentCmdline:             {PsParentCmdline, "parent process command line", ParamUnicodeString, []string{"ps.parent.cmdline contains 'java'"}, nil, nil},
	PsParentExe:                 {PsParentExe, "full name of the parent process' executable", ParamUnicodeString, []string{"ps.parent.exe = 'C:\\Windows\\system32\\explorer.exe'"}, nil, nil},
	PsParentArgs:                {PsParentArgs, "parent process command line arguments", ParamSlice, []string{"ps.parent.args in ('/cdir', '/-C')"}, nil, nil},
	PsParentCwd:                 {PsParentCwd, "parent process current working directory", ParamUnicodeString, []string{"ps.parent.cwd = 'C:\\Temp'"}, nil, nil},
	PsParentSID:                 {PsParentSID, "security identifier under which the parent process is run", ParamUnicodeString, []string{"ps.parent.sid contains 'SYSTEM'"}, nil, nil},
	PsParentDomain:              {PsParentDomain, "parent process domain", ParamUnicodeString, []string{"ps.parent.domain contains 'SERVICE'"}, nil, nil},
	PsParentUsername:             {PsParentUsername, "parent process username", ParamUnicodeString, []string{"ps.parent.username contains 'system'"}, nil, nil},
	PsParentSessionID:           {PsParentSessionID, "unique identifier for the current session of parent process", ParamInt16, []string{"ps.parent.sessionid = 1"}, nil, nil},
	PsParentEnvs:                {PsParentEnvs, "parent process environment variables", ParamSlice, []string{"ps.parent.envs in ('MOZ_CRASHREPORTER_DATA_DIRECTORY')"}, nil, nil},
	PsParentHandles:             {PsParentHandles, "allocated parent process handle names", ParamSlice, []string{"ps.parent.handles in ('\\BaseNamedObjects\\__ComCatalogCache__')"}, nil, nil},
	PsParentHandleTypes:         {PsParentHandleTypes, "allocated parent process handle types", ParamSlice, []string{"ps.parent.handle.types in ('File', 'SymbolicLink')"}, nil, nil},
	PsParentDTB:                 {PsParentDTB, "parent process directory table base address", ParamAddress, []string{"ps.parent.dtb = '7ffe0000'"}, nil, nil},
	PsAccessMask:                {PsAccessMask, "process desired access rights", ParamAnsiString, []string{"ps.access.mask = '0x1400'"}, nil, nil},
	PsAccessMaskNames:           {PsAccessMaskNames, "process desired access rights as a string list", ParamSlice, []string{"ps.access.mask.names in ('SUSPEND_RESUME')"}, nil, nil},
	PsAccessStatus:              {PsAccessStatus, "process access status", ParamUnicodeString, []string{"ps.access.status = 'access is denied.'"}, nil, nil},
	PsUUID:                      {PsUUID, "unique process identifier", ParamUint64, []string{"ps.uuid > 6000054355"}, nil, nil},
	PsParentUUID:                {PsParentUUID, "unique parent process identifier", ParamUint64, []string{"ps.parent.uuid > 6000054355"}, nil, nil},
	PsIsWOW64Field:              {PsIsWOW64Field, "indicates if the process generating the event is a 32-bit process created in 64-bit Windows system", ParamBool, []string{"ps.is_wow64"}, nil, nil},
	PsIsPackagedField:           {PsIsPackagedField, "indicates if the process generating the event is packaged with the MSIX technology", ParamBool, []string{"ps.is_packaged"}, nil, nil},
	PsIsProtectedField:          {PsIsProtectedField, "indicates if the process generating the event is a protected process", ParamBool, []string{"ps.is_protected"}, nil, nil},
	PsParentIsWOW64Field:        {PsParentIsWOW64Field, "indicates if the parent process generating the event is a 32-bit process created in 64-bit Windows system", ParamBool, []string{"ps.parent.is_wow64"}, nil, nil},
	PsParentIsPackagedField:     {PsParentIsPackagedField, "indicates if the parent process generating the event is packaged with the MSIX technology", ParamBool, []string{"ps.parent.is_packaged"}, nil, nil},
	PsParentIsProtectedField:    {PsParentIsProtectedField, "indicates if the the parent process generating the event is a protected process", ParamBool, []string{"ps.parent.is_protected"}, nil, nil},
	PsAncestor:                  {PsAncestor, "the process ancestor name", ParamUnicodeString, []string{"ps.ancestor[1] = 'svchost.exe'", "ps.ancestor in ('winword.exe')"}, nil, &Argument{Optional: true, Pattern: "[0-9]+", ValidationFunc: isNumber}},
	PsTokenIntegrityLevel:       {PsTokenIntegrityLevel, "process token integrity level", ParamUnicodeString, []string{"ps.token.integrity_level = 'SYSTEM'"}, nil, nil},
	PsTokenIsElevated:           {PsTokenIsElevated, "indicates if the process token is elevated", ParamBool, []string{"ps.token.is_elevated = true"}, nil, nil},
	PsTokenElevationType:        {PsTokenElevationType, "process token elevation type", ParamAnsiString, []string{"ps.token.elevation_type = 'LIMITED'"}, nil, nil},
	PsParentTokenIntegrityLevel: {PsParentTokenIntegrityLevel, "parent process token integrity level", ParamUnicodeString, []string{"ps.parent.token.integrity_level = 'HIGH'"}, nil, nil},
	PsParentTokenIsElevated:     {PsParentTokenIsElevated, "indicates if the parent process token is elevated", ParamBool, []string{"ps.parent.token.is_elevated = true"}, nil, nil},
	PsParentTokenElevationType:  {PsParentTokenElevationType, "parent process token elevation type", ParamAnsiString, []string{"ps.parent.token.elevation_type = 'LIMITED'"}, nil, nil},
	PsSignatureExists:           {PsSignatureExists, "indicates if the process executable has a valid signature", ParamBool, []string{"ps.signature.exists"}, nil, nil},
	PsSignatureTrusted:          {PsSignatureTrusted, "indicates if the process executable signature certificate chain is trusted", ParamBool, []string{"ps.signature.trusted"}, nil, nil},
	PsSignatureSerial:           {PsSignatureSerial, "represents signature serial number", ParamUnicodeString, []string{"ps.signature.serial = '330000023241fb59996dcc4dff000000000232'"}, nil, nil},
	PsSignatureSubject:          {PsSignatureSubject, "represents signature subject", ParamUnicodeString, []string{"ps.signature.subject contains 'Washington, Redmond, Microsoft Corporation'"}, nil, nil},
	PsSignatureIssuer:           {PsSignatureIssuer, "represents signature CA", ParamUnicodeString, []string{"ps.signature.issuer contains 'Washington, Redmond, Microsoft Corporation'"}, nil, nil},
	PsSignatureAfter:            {PsSignatureAfter, "represents certificate expiration date", ParamTime, []string{"ps.signature.after contains '2024-02-01 00:05:42 +0000 UTC'"}, nil, nil},
	PsSignatureBefore:           {PsSignatureBefore, "represents certificate enrollment date", ParamTime, []string{"ps.signature.before contains '2024-02-01 00:05:42 +0000 UTC'"}, nil, nil},

	PsPeNumSections: {PsPeNumSections, "number of PE sections", ParamUint16, []string{"ps.pe.nsections < 5"}, nil, nil},
	PsPeNumSymbols:  {PsPeNumSymbols, "number of entries in the symbol table", ParamUint32, []string{"ps.pe.nsymbols > 230"}, nil, nil},
	PsPeBaseAddress: {PsPeBaseAddress, "executable base address", ParamAddress, []string{"ps.pe.address.base = '140000000'"}, nil, nil},
	PsPeEntrypoint:  {PsPeEntrypoint, "address of the entrypoint function", ParamAddress, []string{"ps.pe.address.entrypoint = '20110'"}, nil, nil},
	PsPeSymbols:     {PsPeSymbols, "imported symbols", ParamSlice, []string{"ps.pe.symbols in ('GetTextFaceW', 'GetProcessHeap')"}, nil, nil},
	PsPeImports:     {PsPeImports, "imported dynamic linked libraries", ParamSlice, []string{"ps.pe.imports in ('msvcrt.dll', 'GDI32.dll'"}, nil, nil},
	PsPeResources: {PsPeResources, "version resources", ParamMap, []string{"ps.pe.resources[FileDescription] = 'Notepad'"}, nil, &Argument{Optional: true, Pattern: "[a-zA-Z0-9_]+", ValidationFunc: func(s string) bool {
		for _, c := range s {
			switch {
			case unicode.IsLower(c):
			case unicode.IsUpper(c):
			case unicode.IsNumber(c):
			case c == '_':
			default:
				return false
			}
		}
		return true
	}}},
	PsPeCompany:        {PsPeCompany, "internal company name of the file provided at compile-time", ParamUnicodeString, []string{"ps.pe.company = 'Microsoft Corporation'"}, nil, nil},
	PsPeCopyright:      {PsPeCopyright, "copyright notice for the file emitted at compile-time", ParamUnicodeString, []string{"ps.pe.copyright = '\u00a9 Microsoft Corporation'"}, nil, nil},
	PsPeDescription:    {PsPeDescription, "internal description of the file provided at compile-time", ParamUnicodeString, []string{"ps.pe.description = 'Notepad'"}, nil, nil},
	PsPeFileName:       {PsPeFileName, "original file name supplied at compile-time", ParamUnicodeString, []string{"ps.pe.file.name = 'NOTEPAD.EXE'"}, nil, nil},
	PsPeFileVersion:    {PsPeFileVersion, "file version supplied at compile-time", ParamUnicodeString, []string{"ps.pe.file.version = '10.0.18362.693 (WinBuild.160101.0800)'"}, nil, nil},
	PsPeProduct:        {PsPeProduct, "internal product name of the file provided at compile-time", ParamUnicodeString, []string{"ps.pe.product = 'Microsoft\u00ae Windows\u00ae Operating System'"}, nil, nil},
	PsPeProductVersion: {PsPeProductVersion, "internal product version of the file provided at compile-time", ParamUnicodeString, []string{"ps.pe.product.version = '10.0.18362.693'"}, nil, nil},
	PsPeImphash:        {PsPeImphash, "import hash", ParamAnsiString, []string{"pe.impash = '5d3861c5c547f8a34e471ba273a732b2'"}, nil, nil},
	PsPeIsDotnet:       {PsPeIsDotnet, "indicates if PE contains CLR data", ParamBool, []string{"ps.pe.is_dotnet"}, nil, nil},
	PsPeAnomalies:      {PsPeAnomalies, "contains PE anomalies detected during parsing", ParamSlice, []string{"ps.pe.anomalies in ('number of sections is 0')"}, nil, nil},
	PsPeIsModified:     {PsPeIsModified, "indicates if disk and in-memory PE headers differ", ParamBool, []string{"ps.pe.is_modified"}, nil, nil},

	ThreadBasePrio:                                 {ThreadBasePrio, "scheduler priority of the thread", ParamInt8, []string{"thread.prio = 5"}, nil, nil},
	ThreadIOPrio:                                   {ThreadIOPrio, "I/O priority hint for scheduling I/O operations", ParamInt8, []string{"thread.io.prio = 4"}, nil, nil},
	ThreadPagePrio:                                 {ThreadPagePrio, "memory page priority hint for memory pages accessed by the thread", ParamInt8, []string{"thread.page.prio = 12"}, nil, nil},
	ThreadKstackBase:                               {ThreadKstackBase, "base address of the thread's kernel space stack", ParamAddress, []string{"thread.kstack.base = 'a65d800000'"}, nil, nil},
	ThreadKstackLimit:                              {ThreadKstackLimit, "limit of the thread's kernel space stack", ParamAddress, []string{"thread.kstack.limit = 'a85d800000'"}, nil, nil},
	ThreadUstackBase:                               {ThreadUstackBase, "base address of the thread's user space stack", ParamAddress, []string{"thread.ustack.base = '7ffe0000'"}, nil, nil},
	ThreadUstackLimit:                              {ThreadUstackLimit, "limit of the thread's user space stack", ParamAddress, []string{"thread.ustack.limit = '8ffe0000'"}, nil, nil},
	ThreadEntrypoint:                               {ThreadEntrypoint, "starting address of the function to be executed by the thread", ParamAddress, []string{"thread.entrypoint = '7efe0000'"}, &Deprecation{Since: "2.3.0", Fields: []Field{ThreadStartAddress}}, nil},
	ThreadStartAddress:                             {ThreadStartAddress, "thread start address", ParamAddress, []string{"thread.start_address = '7efe0000'"}, nil, nil},
	ThreadStartAddressSymbol:                       {ThreadStartAddressSymbol, "thread start address symbol", ParamUnicodeString, []string{"thread.start_address.symbol = 'LoadImage'"}, nil, nil},
	ThreadStartAddressModule:                       {ThreadStartAddressModule, "thread start address module", ParamUnicodeString, []string{"thread.start_address.module endswith 'kernel32.dll'"}, nil, nil},
	ThreadPID:                                      {ThreadPID, "the process identifier where the thread is created", ParamUint32, []string{"evt.pid != thread.pid"}, nil, nil},
	ThreadTEB:                                      {ThreadTEB, "the base address of the thread environment block", ParamAddress, []string{"thread.teb_address = '8f30893000'"}, nil, nil},
	ThreadAccessMask:                               {ThreadAccessMask, "thread desired access rights", ParamAnsiString, []string{"thread.access.mask = '0x1fffff'"}, nil, nil},
	ThreadAccessMaskNames:                          {ThreadAccessMaskNames, "thread desired access rights as a string list", ParamSlice, []string{"thread.access.mask.names in ('IMPERSONATE')"}, nil, nil},
	ThreadAccessStatus:                             {ThreadAccessStatus, "thread access status", ParamUnicodeString, []string{"thread.access.status = 'success'"}, nil, nil},
	ThreadCallstackSummary:                         {ThreadCallstackSummary, "callstack summary", ParamUnicodeString, []string{"thread.callstack.summary contains 'ntdll.dll|KERNELBASE.dll'"}, nil, nil},
	ThreadCallstackDetail:                          {ThreadCallstackDetail, "detailed information of each stack frame", ParamUnicodeString, []string{"thread.callstack.detail contains 'KERNELBASE.dll!CreateProcessW'"}, nil, nil},
	ThreadCallstackModules:                         {ThreadCallstackModules, "list of modules comprising the callstack", ParamSlice, []string{"thread.callstack.modules in ('C:\\WINDOWS\\System32\\KERNELBASE.dll')", "base(thread.callstack.modules[7]) = 'ntdll.dll'"}, nil, &Argument{Optional: true, Pattern: "[0-9]+", ValidationFunc: isNumber}},
	ThreadCallstackSymbols:                         {ThreadCallstackSymbols, "list of symbols comprising the callstack", ParamSlice, []string{"thread.callstack.symbols in ('ntdll.dll!NtCreateProcess')", "thread.callstack.symbols[3] = 'ntdll!NtCreateProcess'"}, nil, &Argument{Optional: true, Pattern: "[0-9]+", ValidationFunc: isNumber}},
	ThreadCallstackAllocationSizes:                 {ThreadCallstackAllocationSizes, "allocation sizes of private pages", ParamSlice, []string{"thread.callstack.allocation_sizes > 10000"}, nil, nil},
	ThreadCallstackProtections:                     {ThreadCallstackProtections, "page protections masks of each frame", ParamSlice, []string{"thread.callstack.protections in ('RWX', 'WX')"}, nil, nil},
	ThreadCallstackCallsiteLeadingAssembly:         {ThreadCallstackCallsiteLeadingAssembly, "callsite leading assembly instructions", ParamSlice, []string{"thread.callstack.callsite_leading_assembly in ('mov r10,rcx', 'syscall')"}, nil, nil},
	ThreadCallstackCallsiteTrailingAssembly:        {ThreadCallstackCallsiteTrailingAssembly, "callsite trailing assembly instructions", ParamSlice, []string{"thread.callstack.callsite_trailing_assembly in ('add esp, 0xab')"}, nil, nil},
	ThreadCallstackIsUnbacked:                      {ThreadCallstackIsUnbacked, "indicates if the callstack contains unbacked regions", ParamBool, []string{"thread.callstack.is_unbacked"}, nil, nil},
	ThreadCallstackAddresses:                       {ThreadCallstackAddresses, "list of all stack return addresses", ParamSlice, []string{"thread.callstack.addresses in ('7ffb5c1d0396')"}, nil, nil},
	ThreadCallstackFinalUserModuleName:             {ThreadCallstackFinalUserModuleName, "final user space stack frame module name", ParamUnicodeString, []string{"thread.callstack.final_user_module.name != 'ntdll.dll'"}, nil, nil},
	ThreadCallstackFinalUserModulePath:             {ThreadCallstackFinalUserModulePath, "final user space stack frame module path", ParamUnicodeString, []string{"thread.callstack.final_user_module.path imatches '?:\\Windows\\System32\\ntdll.dll'"}, nil, nil},
	ThreadCallstackFinalUserSymbolName:             {ThreadCallstackFinalUserSymbolName, "final user space stack symbol name", ParamUnicodeString, []string{"thread.callstack.final_user_symbol.name imatches 'CreateProcess*'"}, nil, nil},
	ThreadCallstackFinalKernelModuleName:           {ThreadCallstackFinalKernelModuleName, "final kernel space stack frame module name", ParamUnicodeString, []string{"thread.callstack.final_kernel_module.name = 'FLTMGR.SYS'"}, nil, nil},
	ThreadCallstackFinalKernelModulePath:           {ThreadCallstackFinalKernelModulePath, "final kernel space stack frame module path", ParamUnicodeString, []string{"thread.callstack.final_kernel_module.path imatches '?:\\WINDOWS\\System32\\drivers\\FLTMGR.SYS'"}, nil, nil},
	ThreadCallstackFinalKernelSymbolName:           {ThreadCallstackFinalKernelSymbolName, "final kernel space stack symbol name", ParamUnicodeString, []string{"thread.callstack.final_kernel_symbol.name = 'FltGetStreamContext'"}, nil, nil},
	ThreadCallstackFinalUserModuleSignatureExists:  {ThreadCallstackFinalUserModuleSignatureExists, "signature status of the final user space stack frame module", ParamBool, []string{"thread.callstack.final_user_module.signature.exists = true"}, nil, nil},
	ThreadCallstackFinalUserModuleSignatureTrusted: {ThreadCallstackFinalUserModuleSignatureTrusted, "signature trust status of the final user space stack frame module", ParamBool, []string{"thread.callstack.final_user_module.signature.trusted = true"}, nil, nil},
	ThreadCallstackFinalUserModuleSignatureIssuer:  {ThreadCallstackFinalUserModuleSignatureIssuer, "final user space stack frame module signature certificate issuer", ParamUnicodeString, []string{"thread.callstack.final_user_module.signature.issuer imatches '*Microsoft Corporation*'"}, nil, nil},
	ThreadCallstackFinalUserModuleSignatureSubject: {ThreadCallstackFinalUserModuleSignatureSubject, "final user space stack frame module signature certificate subject", ParamUnicodeString, []string{"thread.callstack.final_user_module.signature.subject imatches '*Microsoft Windows*'"}, nil, nil},

	ImagePath:                {ImagePath, "full image path", ParamUnicodeString, []string{"image.path = 'C:\\Windows\\System32\\advapi32.dll'"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModulePath}}, nil},
	ImageName:                {ImageName, "image name", ParamUnicodeString, []string{"image.name = 'advapi32.dll'"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModuleName}}, nil},
	ImageBase:                {ImageBase, "the base address of process in which the image is loaded", ParamAddress, []string{"image.base.address = 'a65d800000'"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModuleBase}}, nil},
	ImageChecksum:            {ImageChecksum, "image checksum", ParamUint32, []string{"image.checksum = 746424"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModuleChecksum}}, nil},
	ImageSize:                {ImageSize, "image size", ParamUint32, []string{"image.size > 1024"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModuleSize}}, nil},
	ImageDefaultAddress:      {ImageDefaultAddress, "default image address", ParamAddress, []string{"image.default.address = '7efe0000'"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModuleDefaultAddress}}, nil},
	ImagePID:                 {ImagePID, "target process identifier", ParamUint32, []string{"image.pid = 80"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModulePID}}, nil},
	ImageSignatureType:       {ImageSignatureType, "image signature type", ParamAnsiString, []string{"image.signature.type != 'NONE'"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModuleSignatureType}}, nil},
	ImageSignatureLevel:      {ImageSignatureLevel, "image signature level", ParamAnsiString, []string{"image.signature.level = 'AUTHENTICODE'"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModuleSignatureLevel}}, nil},
	ImageCertSerial:          {ImageCertSerial, "image certificate serial number", ParamUnicodeString, []string{"image.cert.serial = '330000023241fb59996dcc4dff000000000232'"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModuleSignatureSerial}}, nil},
	ImageCertSubject:         {ImageCertSubject, "image certificate subject", ParamUnicodeString, []string{"image.cert.subject contains 'Washington, Redmond, Microsoft Corporation'"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModuleSignatureSubject}}, nil},
	ImageCertIssuer:          {ImageCertIssuer, "image certificate CA", ParamUnicodeString, []string{"image.cert.issuer contains 'Washington, Redmond, Microsoft Corporation'"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModuleSignatureIssuer}}, nil},
	ImageCertAfter:           {ImageCertAfter, "image certificate expiration date", ParamTime, []string{"image.cert.after contains '2024-02-01 00:05:42 +0000 UTC'"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModuleSignatureAfter}}, nil},
	ImageCertBefore:          {ImageCertBefore, "image certificate enrollment date", ParamTime, []string{"image.cert.before contains '2024-02-01 00:05:42 +0000 UTC'"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModuleSignatureBefore}}, nil},
	ImageIsDriverMalicious:   {ImageIsDriverMalicious, "indicates if the loaded driver is malicious", ParamBool, []string{"image.is_driver_malicious"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModuleIsDriverMalicious}}, nil},
	ImageIsDriverVulnerable:  {ImageIsDriverVulnerable, "indicates if the loaded driver is vulnerable", ParamBool, []string{"image.is_driver_vulnerable"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModuleIsDriverVulnerable}}, nil},
	ImageIsDLL:               {ImageIsDLL, "indicates if the loaded image is a DLL", ParamBool, []string{"image.is_dll'"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModuleIsDLL}}, nil},
	ImageIsDriver:            {ImageIsDriver, "indicates if the loaded image is a driver", ParamBool, []string{"image.is_driver'"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModuleIsDriver}}, nil},
	ImageIsExecutable:        {ImageIsExecutable, "indicates if the loaded image is an executable", ParamBool, []string{"image.is_exec'"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModuleIsExecutable}}, nil},
	ImageIsDotnet:            {ImageIsDotnet, "indicates if the loaded image is a .NET assembly", ParamBool, []string{"image.is_dotnet'"}, &Deprecation{Since: "3.0.0", Fields: []Field{ModuleIsDotnet}}, nil},
	ModulePath:               {ModulePath, "full module path", ParamUnicodeString, []string{"module.path = 'C:\\Windows\\System32\\advapi32.dll'"}, nil, nil},
	ModulePathStem:           {ModulePathStem, "module path stem", ParamUnicodeString, []string{"module.path.stem = 'C:\\Windows\\System32\\advapi32'"}, nil, nil},
	ModuleName:               {ModuleName, "module name", ParamUnicodeString, []string{"module.name = 'advapi32.dll'"}, nil, nil},
	ModuleBase:               {ModuleBase, "the base address of process in which the module is loaded", ParamAddress, []string{"module.base.address = 'a65d800000'"}, nil, nil},
	ModuleChecksum:           {ModuleChecksum, "module checksum", ParamUint32, []string{"module.checksum = 746424"}, nil, nil},
	ModuleSize:               {ModuleSize, "module size", ParamUint32, []string{"module.size > 1024"}, nil, nil},
	ModuleDefaultAddress:     {ModuleDefaultAddress, "default module address", ParamAddress, []string{"module.default_address = '7efe0000'"}, nil, nil},
	ModulePID:                {ModulePID, "target process identifier", ParamUint32, []string{"module.pid = 80"}, nil, nil},
	ModuleSignatureType:      {ModuleSignatureType, "module signature type", ParamAnsiString, []string{"module.signature.type != 'NONE'"}, nil, nil},
	ModuleSignatureLevel:     {ModuleSignatureLevel, "module signature level", ParamAnsiString, []string{"module.signature.level = 'AUTHENTICODE'"}, nil, nil},
	ModuleSignatureExists:    {ModuleSignatureExists, "indicates if the module is signed", ParamBool, []string{"module.signature.exists = true"}, nil, nil},
	ModuleSignatureTrusted:   {ModuleSignatureTrusted, "indicates if the module signature is trusted", ParamBool, []string{"module.signature.trusted = false"}, nil, nil},
	ModuleSignatureSerial:    {ModuleSignatureSerial, "module certificate serial number", ParamUnicodeString, []string{"module.signature.serial = '330000023241fb59996dcc4dff000000000232'"}, nil, nil},
	ModuleSignatureSubject:   {ModuleSignatureSubject, "module certificate subject", ParamUnicodeString, []string{"module.signature.subject contains 'Washington, Redmond, Microsoft Corporation'"}, nil, nil},
	ModuleSignatureIssuer:    {ModuleSignatureIssuer, "module certificate CA", ParamUnicodeString, []string{"module.signature.issuer contains 'Washington, Redmond, Microsoft Corporation'"}, nil, nil},
	ModuleSignatureAfter:     {ModuleSignatureAfter, "module certificate expiration date", ParamTime, []string{"module.signature.after contains '2024-02-01 00:05:42 +0000 UTC'"}, nil, nil},
	ModuleSignatureBefore:    {ModuleSignatureBefore, "module certificate enrollment date", ParamTime, []string{"module.signature.before contains '2024-02-01 00:05:42 +0000 UTC'"}, nil, nil},
	ModuleIsDriverMalicious:  {ModuleIsDriverMalicious, "indicates if the loaded driver is malicious", ParamBool, []string{"module.is_driver_malicious"}, nil, nil},
	ModuleIsDriverVulnerable: {ModuleIsDriverVulnerable, "indicates if the loaded driver is vulnerable", ParamBool, []string{"module.is_driver_vulnerable"}, nil, nil},
	ModuleIsDLL:              {ModuleIsDLL, "indicates if the loaded module is a DLL", ParamBool, []string{"module.is_dll'"}, nil, nil},
	ModuleIsDriver:           {ModuleIsDriver, "indicates if the loaded module is a driver", ParamBool, []string{"module.is_driver'"}, nil, nil},
	ModuleIsExecutable:       {ModuleIsExecutable, "indicates if the loaded module is an executable", ParamBool, []string{"module.is_exec'"}, nil, nil},
	ModuleIsDotnet:           {ModuleIsDotnet, "indicates if the loaded module is a .NET assembly", ParamBool, []string{"module.pe.is_dotnet'"}, nil, nil},
	DllPath:                  {DllPath, "full dll path", ParamUnicodeString, []string{"dll.path = 'C:\\Windows\\System32\\advapi32.dll'"}, nil, nil},
	DllPathStem:              {DllPathStem, "dll path stem", ParamUnicodeString, []string{"dll.path.stem = 'C:\\Windows\\System32\\advapi32'"}, nil, nil},
	DllName:                  {DllName, "module name", ParamUnicodeString, []string{"dll.name = 'advapi32.dll'"}, nil, nil},
	DllBase:                  {DllBase, "the base address of process in which the DLL is loaded", ParamAddress, []string{"dll.base = 'a65d800000'"}, nil, nil},
	DllSize:                  {DllSize, "dll virtual mapped size", ParamUint32, []string{"dll.size > 1024"}, nil, nil},
	DllPID:                   {DllPID, "target process identifier", ParamUint32, []string{"dll.pid = 80"}, nil, nil},
	DllSignatureType:         {DllSignatureType, "dll signature type", ParamAnsiString, []string{"dll.signature.type != 'NONE'"}, nil, nil},
	DllSignatureLevel:        {DllSignatureLevel, "dll signature level", ParamAnsiString, []string{"dll.signature.level = 'AUTHENTICODE'"}, nil, nil},
	DllSignatureExists:       {DllSignatureExists, "indicates if the dll is signed", ParamBool, []string{"dll.signature.exists = true"}, nil, nil},
	DllSignatureTrusted:      {DllSignatureTrusted, "indicates if the dll signature is trusted", ParamBool, []string{"dll.signature.trusted = false"}, nil, nil},
	DllSignatureSerial:       {DllSignatureSerial, "dll certificate serial number", ParamUnicodeString, []string{"dll.signature.serial = '330000023241fb59996dcc4dff000000000232'"}, nil, nil},
	DllSignatureSubject:      {DllSignatureSubject, "dll certificate subject", ParamUnicodeString, []string{"dll.signature.subject contains 'Washington, Redmond, Microsoft Corporation'"}, nil, nil},
	DllSignatureIssuer:       {DllSignatureIssuer, "dll certificate CA", ParamUnicodeString, []string{"dll.signature.issuer contains 'Washington, Redmond, Microsoft Corporation'"}, nil, nil},
	DllSignatureAfter:        {DllSignatureAfter, "moddllule certificate expiration date", ParamTime, []string{"dll.signature.after contains '2024-02-01 00:05:42 +0000 UTC'"}, nil, nil},
	DllSignatureBefore:       {DllSignatureBefore, "dll certificate enrollment date", ParamTime, []string{"dll.signature.before contains '2024-02-01 00:05:42 +0000 UTC'"}, nil, nil},
	DllIsDotnet:              {DllIsDotnet, "indicates if the loaded dll is a .NET assembly", ParamBool, []string{"dll.pe.is_dotnet'"}, nil, nil},

	FileObject:                      {FileObject, "file object address", ParamUint64, []string{"file.object = 18446738026482168384"}, nil, nil},
	FilePath:                        {FilePath, "full file path", ParamUnicodeString, []string{"file.path = 'C:\\Windows\\System32'"}, nil, nil},
	FilePathStem:                    {FilePathStem, "full file path without extension", ParamUnicodeString, []string{"file.path.stem = 'C:\\Windows\\System32\\cmd'"}, nil, nil},
	FileName:                        {FileName, "full file name", ParamUnicodeString, []string{"file.name contains 'mimikatz'"}, nil, nil},
	FileOperation:                   {FileOperation, "file operation", ParamAnsiString, []string{"file.operation = 'open'"}, nil, nil},
	FileShareMask:                   {FileShareMask, "file share mask", ParamAnsiString, []string{"file.share.mask = 'rw-'"}, nil, nil},
	FileIOSize:                      {FileIOSize, "file I/O size", ParamUint32, []string{"file.io.size > 512"}, nil, nil},
	FileOffset:                      {FileOffset, "file offset", ParamUint64, []string{"file.offset = 1024"}, nil, nil},
	FileType:                        {FileType, "file type", ParamAnsiString, []string{"file.type = 'directory'"}, nil, nil},
	FileExtension:                   {FileExtension, "file extension", ParamAnsiString, []string{"file.extension = '.dll'"}, nil, nil},
	FileAttributes:                  {FileAttributes, "file attributes", ParamSlice, []string{"file.attributes in ('archive', 'hidden')"}, nil, nil},
	FileStatus:                      {FileStatus, "file operation status message", ParamUnicodeString, []string{"file.status != 'success'"}, nil, nil},
	FileViewBase:                    {FileViewBase, "view base address", ParamAddress, []string{"file.view.base = '25d42170000'"}, nil, nil},
	FileViewSize:                    {FileViewSize, "size of the mapped view", ParamUint64, []string{"file.view.size > 1024"}, nil, nil},
	FileViewType:                    {FileViewType, "type of the mapped view section", ParamEnum, []string{"file.view.type = 'IMAGE'"}, nil, nil},
	FileViewProtection:              {FileViewProtection, "protection rights of the section view", ParamAnsiString, []string{"file.view.protection = 'READONLY'"}, nil, nil},
	FileIsDriverMalicious:           {FileIsDriverMalicious, "indicates if the dropped driver is malicious", ParamBool, []string{"file.is_driver_malicious"}, nil, nil},
	FileIsDriverVulnerable:          {FileIsDriverVulnerable, "indicates if the dropped driver is vulnerable", ParamBool, []string{"file.is_driver_vulnerable"}, nil, nil},
	FileIsDLL:                       {FileIsDLL, "indicates if the created file is a DLL", ParamBool, []string{"file.is_dll'"}, nil, nil},
	FileIsDriver:                    {FileIsDriver, "indicates if the created file is a driver", ParamBool, []string{"file.is_driver'"}, nil, nil},
	FileIsExecutable:                {FileIsExecutable, "indicates if the created file is an executable", ParamBool, []string{"file.is_exec'"}, nil, nil},
	FilePID:                         {FilePID, "denotes the process id performing file operation", ParamPID, []string{"file.pid = 4"}, nil, nil},
	FileKey:                         {FileKey, "uniquely identifies the file object", ParamUint64, []string{"file.key = 12446738026482168384"}, nil, nil},
	FileInfoClass:                   {FileInfoClass, "identifies the file information class", ParamEnum, []string{"file.info_class = 'Allocation'"}, nil, nil},
	FileInfoAllocationSize:          {FileInfoAllocationSize, "file allocation size", ParamUint64, []string{"file.info.allocation_size > 645400"}, nil, nil},
	FileInfoEOFSize:                 {FileInfoEOFSize, "file EOF size", ParamUint64, []string{"file.info.eof_size > 1000"}, nil, nil},
	FileInfoIsDispositionDeleteFile: {FileInfoIsDispositionDeleteFile, "indicates if the file is deleted when its handle is closed", ParamBool, []string{"file.info.is_disposition_file_delete = true"}, nil, nil},

	RegistryPath:      {RegistryPath, "fully qualified registry path", ParamUnicodeString, []string{"registry.path = 'HKEY_LOCAL_MACHINE\\SYSTEM'"}, nil, nil},
	RegistryKeyName:   {RegistryKeyName, "registry key name", ParamUnicodeString, []string{"registry.key.name = 'CurrentControlSet'"}, nil, nil},
	RegistryKeyHandle: {RegistryKeyHandle, "registry key object address", ParamAddress, []string{"registry.key.handle = 'FFFFB905D60C2268'"}, nil, nil},
	RegistryValue:     {RegistryValue, "registry value name", ParamUnicodeString, []string{"registry.value = 'Epoch'"}, nil, nil},
	RegistryValueType: {RegistryValueType, "type of registry value", ParamUnicodeString, []string{"registry.value.type = 'REG_SZ'"}, nil, nil},
	RegistryData:      {RegistryData, "registry value captured data", ParamObject, []string{"registry.data = '%SystemRoot%'"}, nil, nil},
	RegistryStatus:    {RegistryStatus, "status of registry operation", ParamUnicodeString, []string{"registry.status != 'success'"}, nil, nil},

	NetDIP:        {NetDIP, "destination IP address", ParamIP, []string{"net.dip = 172.17.0.3"}, nil, nil},
	NetSIP:        {NetSIP, "source IP address", ParamIP, []string{"net.sip = 127.0.0.1"}, nil, nil},
	NetDport:      {NetDport, "destination port", ParamUint16, []string{"net.dport in (80, 443, 8080)"}, nil, nil},
	NetSport:      {NetSport, "source port", ParamUint16, []string{"net.sport != 3306"}, nil, nil},
	NetDportName:  {NetDportName, "destination port name", ParamAnsiString, []string{"net.dport.name = 'dns'"}, nil, nil},
	NetSportName:  {NetSportName, "source port name", ParamAnsiString, []string{"net.sport.name = 'http'"}, nil, nil},
	NetL4Proto:    {NetL4Proto, "layer 4 protocol name", ParamAnsiString, []string{"net.l4.proto = 'TCP"}, nil, nil},
	NetPacketSize: {NetPacketSize, "packet size", ParamUint32, []string{"net.size > 512"}, nil, nil},
	NetSIPNames:   {NetSIPNames, "source IP names", ParamSlice, []string{"net.sip.names in ('github.com.')"}, nil, nil},
	NetDIPNames:   {NetDIPNames, "destination IP names", ParamSlice, []string{"net.dip.names in ('github.com.')"}, nil, nil},

	HandleID:     {HandleID, "handle identifier", ParamUint16, []string{"handle.id = 24"}, nil, nil},
	HandleObject: {HandleObject, "handle object address", ParamAddress, []string{"handle.object = 'FFFFB905DBF61988'"}, nil, nil},
	HandleName:   {HandleName, "handle name", ParamUnicodeString, []string{"handle.name = '\\Device\\NamedPipe\\chrome.12644.28.105826381'"}, nil, nil},
	HandleType:   {HandleType, "handle type", ParamAnsiString, []string{"handle.type = 'Mutant'"}, nil, nil},

	PeNumSections: {PeNumSections, "number of sections", ParamUint16, []string{"pe.nsections < 5"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsPeNumSections}}, nil},
	PeNumSymbols:  {PeNumSymbols, "number of entries in the symbol table", ParamUint32, []string{"pe.nsymbols > 230"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsPeNumSymbols}}, nil},
	PeBaseAddress: {PeBaseAddress, "image base address", ParamAddress, []string{"pe.address.base = '140000000'"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsPeBaseAddress}}, nil},
	PeEntrypoint:  {PeEntrypoint, "address of the entrypoint function", ParamAddress, []string{"pe.address.entrypoint = '20110'"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsPeEntrypoint}}, nil},
	PeSymbols:     {PeSymbols, "imported symbols", ParamSlice, []string{"pe.symbols in ('GetTextFaceW', 'GetProcessHeap')"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsPeSymbols}}, nil},
	PeImports:     {PeImports, "imported dynamic linked libraries", ParamSlice, []string{"pe.imports in ('msvcrt.dll', 'GDI32.dll'"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsPeImports}}, nil},

	PeResources: {PeResources, "version resources", ParamMap, []string{"pe.resources[FileDescription] = 'Notepad'"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsPeResources}}, &Argument{Optional: true, Pattern: "[a-zA-Z0-9_]+", ValidationFunc: func(s string) bool {
		for _, c := range s {
			switch {
			case unicode.IsLower(c):
			case unicode.IsUpper(c):
			case unicode.IsNumber(c):
			case c == '_':
			default:
				return false
			}
		}
		return true
	}}},

	PeCompany:        {PeCompany, "internal company name of the file provided at compile-time", ParamUnicodeString, []string{"pe.company = 'Microsoft Corporation'"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsPeCompany}}, nil},
	PeCopyright:      {PeCopyright, "copyright notice for the file emitted at compile-time", ParamUnicodeString, []string{"pe.copyright = '\u00a9 Microsoft Corporation'"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsPeCopyright}}, nil},
	PeDescription:    {PeDescription, "internal description of the file provided at compile-time", ParamUnicodeString, []string{"pe.description = 'Notepad'"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsPeDescription}}, nil},
	PeFileName:       {PeFileName, "original file name supplied at compile-time", ParamUnicodeString, []string{"pe.file.name = 'NOTEPAD.EXE'"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsPeFileName}}, nil},
	PeFileVersion:    {PeFileVersion, "file version supplied at compile-time", ParamUnicodeString, []string{"pe.file.version = '10.0.18362.693 (WinBuild.160101.0800)'"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsPeFileVersion}}, nil},
	PeProduct:        {PeProduct, "internal product name of the file provided at compile-time", ParamUnicodeString, []string{"pe.product = 'Microsoft\u00ae Windows\u00ae Operating System'"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsPeProduct}}, nil},
	PeProductVersion: {PeProductVersion, "internal product version of the file provided at compile-time", ParamUnicodeString, []string{"pe.product.version = '10.0.18362.693'"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsPeProductVersion}}, nil},
	PeIsDLL:          {PeIsDLL, "indicates if the loaded image or created file is a DLL", ParamBool, []string{"pe.is_dll'"}, &Deprecation{Since: "2.0.0", Fields: []Field{FileIsDLL, ImageIsDLL}}, nil},
	PeIsDriver:       {PeIsDriver, "indicates if the loaded image or created file is a driver", ParamBool, []string{"pe.is_driver'"}, &Deprecation{Since: "2.0.0", Fields: []Field{FileIsDriver, ImageIsDriver}}, nil},
	PeIsExecutable:   {PeIsExecutable, "indicates if the loaded image or created file is an executable", ParamBool, []string{"pe.is_exec'"}, &Deprecation{Since: "2.0.0", Fields: []Field{FileIsExecutable, ImageIsExecutable}}, nil},
	PeImphash:        {PeImphash, "import hash", ParamAnsiString, []string{"pe.impash = '5d3861c5c547f8a34e471ba273a732b2'"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsPeImphash}}, nil},
	PeIsDotnet:       {PeIsDotnet, "indicates if PE contains CLR data", ParamBool, []string{"pe.is_dotnet"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsPeIsDotnet}}, nil},
	PeAnomalies:      {PeAnomalies, "contains PE anomalies detected during parsing", ParamSlice, []string{"pe.anomalies in ('number of sections is 0')"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsPeAnomalies}}, nil},
	PeIsSigned:       {PeIsSigned, "indicates if the PE has embedded or catalog signature", ParamBool, []string{"pe.is_signed"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsSignatureExists}}, nil},
	PeIsTrusted:      {PeIsTrusted, "indicates if the PE certificate chain is trusted", ParamBool, []string{"pe.is_trusted"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsSignatureTrusted}}, nil},
	PeCertSerial:     {PeCertSerial, "PE certificate serial number", ParamUnicodeString, []string{"pe.cert.serial = '330000023241fb59996dcc4dff000000000232'"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsSignatureSerial}}, nil},
	PeCertSubject:    {PeCertSubject, "PE certificate subject", ParamUnicodeString, []string{"pe.cert.subject contains 'Washington, Redmond, Microsoft Corporation'"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsSignatureSubject}}, nil},
	PeCertIssuer:     {PeCertIssuer, "PE certificate CA", ParamUnicodeString, []string{"pe.cert.issuer contains 'Washington, Redmond, Microsoft Corporation'"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsSignatureIssuer}}, nil},
	PeCertAfter:      {PeCertAfter, "PE certificate expiration date", ParamTime, []string{"pe.cert.after contains '2024-02-01 00:05:42 +0000 UTC'"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsSignatureAfter}}, nil},
	PeCertBefore:     {PeCertBefore, "PE certificate enrollment date", ParamTime, []string{"pe.cert.before contains '2024-02-01 00:05:42 +0000 UTC'"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsSignatureBefore}}, nil},
	PeIsModified:     {PeIsModified, "indicates if disk and in-memory PE headers differ", ParamBool, []string{"pe.is_modified"}, &Deprecation{Since: "3.0.0", Fields: []Field{PsPeIsModified}}, nil},

	MemBaseAddress:    {MemBaseAddress, "region base address", ParamAddress, []string{"mem.address = '211d13f2000'"}, nil, nil},
	MemRegionSize:     {MemRegionSize, "region size", ParamUint64, []string{"mem.size > 438272"}, nil, nil},
	MemAllocType:      {MemAllocType, "region allocation or release type", ParamFlags, []string{"mem.alloc = 'COMMIT'"}, nil, nil},
	MemPageType:       {MemPageType, "page type of the allocated region", ParamEnum, []string{"mem.type = 'PRIVATE'"}, nil, nil},
	MemProtection:     {MemProtection, "allocated region protection type", ParamEnum, []string{"mem.protection = 'READWRITE'"}, nil, nil},
	MemProtectionMask: {MemProtectionMask, "allocated region protection in mask notation", ParamEnum, []string{"mem.protection.mask = 'RWX'"}, nil, nil},

	DNSName:    {DNSName, "dns query name", ParamUnicodeString, []string{"dns.name = 'example.org'"}, nil, nil},
	DNSRR:      {DNSRR, "dns resource record type", ParamAnsiString, []string{"dns.rr = 'AA'"}, nil, nil},
	DNSOptions: {DNSOptions, "dns query options", ParamFlags64, []string{"dns.options in ('ADDRCONFIG', 'DUAL_ADDR')"}, nil, nil},
	DNSRcode:   {DNSRR, "dns response status", ParamAnsiString, []string{"dns.rcode = 'NXDOMAIN'"}, nil, nil},
	DNSAnswers: {DNSAnswers, "dns response answers", ParamSlice, []string{"dns.answers in ('o.lencr.edgesuite.net', 'a1887.dscq.akamai.net')"}, nil, nil},

	ThreadpoolPoolID:                   {ThreadpoolPoolID, "thread pool identifier", ParamAddress, []string{"threadpool.id = '20f5fc02440'"}, nil, nil},
	ThreadpoolTaskID:                   {ThreadpoolTaskID, "thread pool task identifier", ParamAddress, []string{"threadpool.task.id = '20f7ecd21f8'"}, nil, nil},
	ThreadpoolCallbackAddress:          {ThreadpoolCallbackAddress, "thread pool callback address", ParamAddress, []string{"threadpool.callback.address = '7ff868739ed0'"}, nil, nil},
	ThreadpoolCallbackSymbol:           {ThreadpoolCallbackSymbol, "thread pool callback symbol", ParamUnicodeString, []string{"threadpool.callback.symbol = 'RtlDestroyQueryDebugBuffer'"}, nil, nil},
	ThreadpoolCallbackModule:           {ThreadpoolCallbackModule, "thread pool module containing the callback symbol", ParamUnicodeString, []string{"threadpool.callback.module contains 'ntdll.dll'"}, nil, nil},
	ThreadpoolCallbackContext:          {ThreadpoolCallbackContext, "thread pool callback context address", ParamAddress, []string{"threadpool.callback.context = '1df41e07bd0'"}, nil, nil},
	ThreadpoolCallbackContextRip:       {ThreadpoolCallbackContextRip, "thread pool callback thread context instruction pointer", ParamAddress, []string{"threadpool.callback.context.rip = '1df42ffc1f8'"}, nil, nil},
	ThreadpoolCallbackContextRipSymbol: {ThreadpoolCallbackContextRipSymbol, "thread pool callback thread context instruction pointer symbol", ParamUnicodeString, []string{"threadpool.callback.context.rip.symbol = 'VirtualProtect'"}, nil, nil},
	ThreadpoolCallbackContextRipModule: {ThreadpoolCallbackContextRipModule, "thread pool callback thread context instruction pointer symbol module", ParamUnicodeString, []string{"threadpool.callback.context.rip.module contains 'ntdll.dll'"}, nil, nil},
	ThreadpoolSubprocessTag:            {ThreadpoolSubprocessTag, "thread pool service identifier", ParamAddress, []string{"threadpool.subprocess_tag = '10d'"}, nil, nil},
	ThreadpoolTimerDuetime:             {ThreadpoolTimerDuetime, "thread pool timer due time", ParamUint64, []string{"threadpool.timer.duetime > 10"}, nil, nil},
	ThreadpoolTimerSubqueue:            {ThreadpoolTimerSubqueue, "thread pool timer subqueue address", ParamAddress, []string{"threadpool.timer.subqueue = '1db401703e8'"}, nil, nil},
	ThreadpoolTimer:                    {ThreadpoolTimer, "thread pool timer address", ParamAddress, []string{"threadpool.timer.address = '3e8'"}, nil, nil},
	ThreadpoolTimerPeriod:              {ThreadpoolTimerPeriod, "thread pool timer period", ParamUint32, []string{"threadpool.timer.period = 0'"}, nil, nil},
	ThreadpoolTimerWindow:              {ThreadpoolTimerWindow, "thread pool timer tolerate period", ParamUint32, []string{"threadpool.timer.window = 0'"}, nil, nil},
	ThreadpoolTimerAbsolute:            {ThreadpoolTimerAbsolute, "indicates if the thread pool timer is absolute or relative", ParamBool, []string{"threadpool.timer.is_absolute = true'"}, nil, nil},

	// Event Log fields
	EventLogChannel:   {EventLogChannel, "event log channel name", ParamUnicodeString, []string{"eventlog.channel = 'Security'"}, nil, nil},
	EventLogProvider:  {EventLogProvider, "event log provider name", ParamUnicodeString, []string{"eventlog.provider = 'Microsoft-Windows-Sysmon'"}, nil, nil},
	EventLogEventID:   {EventLogEventID, "event log event ID", ParamUint32, []string{"eventlog.event.id = 4688"}, nil, nil},
	EventLogLevel:     {EventLogLevel, "event log level name", ParamUnicodeString, []string{"eventlog.level = 'Information'"}, nil, nil},
	EventLogLevelID:   {EventLogLevelID, "event log level ID", ParamUint32, []string{"eventlog.level.id = 4"}, nil, nil},
	EventLogRecordID:  {EventLogRecordID, "event log record ID", ParamUint64, []string{"eventlog.record.id > 100"}, nil, nil},
	EventLogTask:      {EventLogTask, "event log task name", ParamUnicodeString, []string{"eventlog.task = 'Process Create'"}, nil, nil},
	EventLogOpcode:    {EventLogOpcode, "event log opcode", ParamUnicodeString, []string{"eventlog.opcode = 'Info'"}, nil, nil},
	EventLogKeywords:  {EventLogKeywords, "event log keywords", ParamUnicodeString, []string{"eventlog.keywords contains 'Audit'"}, nil, nil},
	EventLogComputer:  {EventLogComputer, "event log computer name", ParamUnicodeString, []string{"eventlog.computer = 'DC01'"}, nil, nil},
	EventLogUserID:    {EventLogUserID, "event log user SID", ParamUnicodeString, []string{"eventlog.user.id = 'S-1-5-18'"}, nil, nil},
	EventLogData:      {EventLogData, "event log data field accessed by name", ParamObject, []string{"eventlog.data[TargetUserName] = 'admin'"}, nil, &Argument{Optional: false, Pattern: "[a-zA-Z0-9_]+"}},
}

// ArgumentOf returns argument data for the specified field.
func ArgumentOf(name string) *Argument {
	f, ok := fields[Field(name)]
	if !ok {
		return nil
	}
	return f.Argument
}

// IsField returns true if the provided string is a
// recognized field or pseudo field.
func IsField(name string) bool {
	if _, ok := fields[Field(name)]; ok || IsPseudoField(Field(name)) {
		return true
	}
	return false
}

// Get returns a slice of field information.
func Get() []FieldInfo {
	fi := make([]FieldInfo, 0, len(fields))
	for _, field := range fields {
		fi = append(fi, field)
	}
	sort.Slice(fi, func(i, j int) bool { return fi[i].Field < fi[j].Field })
	return fi
}

// IsDeprecated determines if the given field is deprecated.
func IsDeprecated(f Field) (bool, *Deprecation) {
	for _, field := range fields {
		if field.Field == f && field.IsDeprecated() {
			return true, field.Deprecation
		}
	}
	return false, nil
}

// IsBoolean determines if the given field has the bool type.
func IsBoolean(f Field) bool {
	return fields[f].Type == ParamBool
}
