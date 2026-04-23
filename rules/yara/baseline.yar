// Baseline YARA rules shipped with Fibratus Fleet Agent.
// These are intentionally conservative signatures that trigger on hallmarks
// of memory-resident tradecraft (in-memory PE loading, known offensive
// framework artifacts, common shellcode patterns). Operators should layer
// their own rule sources on top via yara.rule.paths in fibratus.yml.

rule InMemory_PE_Header
{
    meta:
        description = "PE/MZ header found inside a process memory region"
        threat_name = "Memory-resident PE loader"
        severity = "medium"
        score = 40
        author = "Fibratus"
    strings:
        $mz = { 4D 5A }                 // "MZ"
        $pe = { 50 45 00 00 }           // "PE\x00\x00"
        $dos = "This program cannot be run in DOS mode"
    condition:
        $mz at 0 and $pe and $dos
}

rule Meterpreter_Stage_Marker
{
    meta:
        description = "Meterpreter reflective loader stage markers"
        threat_name = "Meterpreter"
        severity = "high"
        score = 75
        author = "Fibratus"
    strings:
        $rflat = "ReflectiveLoader" ascii
        $stdapi = "stdapi_" ascii
        $meterp = "metsrv" ascii nocase
    condition:
        any of them
}

rule Cobalt_Strike_Beacon_Strings
{
    meta:
        description = "Strings observed in Cobalt Strike beacons and profiles"
        threat_name = "Cobalt Strike"
        severity = "high"
        score = 85
        author = "Fibratus"
    strings:
        $a1 = "beacon.dll" ascii nocase
        $a2 = "beacon.x64.dll" ascii nocase
        $a3 = "%s as %s\\%s: %d" ascii
        $a4 = "ReflectiveLoader" ascii
        $b1 = { 66 83 38 4D 5A }        // cmp word ptr [rax], 'MZ'
    condition:
        2 of ($a*) or $b1
}

rule Mimikatz_Markers
{
    meta:
        description = "Strings observed in Mimikatz credential-dumping tool"
        threat_name = "Mimikatz"
        severity = "critical"
        score = 95
        author = "Fibratus"
    strings:
        $m1 = "mimikatz" ascii nocase
        $m2 = "sekurlsa::logonpasswords" ascii
        $m3 = "privilege::debug" ascii
        $m4 = "lsadump::" ascii
        $m5 = "kerberos::list" ascii
        $m6 = "!+ MIMIKATZ" ascii
    condition:
        2 of them
}

rule Reflective_DLL_Loader
{
    meta:
        description = "Common reflective DLL injection loader strings"
        threat_name = "Reflective DLL injection"
        severity = "medium"
        score = 60
        author = "Fibratus"
    strings:
        $a1 = "ReflectiveLoader" ascii
        $a2 = "_ReflectiveLoader@4" ascii
        $a3 = "LoadRemoteLibraryR" ascii
    condition:
        any of them
}

rule Common_Shellcode_Patterns
{
    meta:
        description = "Shellcode prologues and API-hashing constructs common to injected stages"
        threat_name = "Shellcode"
        severity = "medium"
        score = 55
        author = "Fibratus"
    strings:
        // fs:[0x30] -> PEB (x86), then InLoadOrderModuleList
        $peb_x86 = { 64 A1 30 00 00 00 }
        // gs:[0x60] -> PEB (x64)
        $peb_x64 = { 65 48 8B 04 25 60 00 00 00 }
        // Common API hash prologue: xor eax, eax ; mov eax, <hash>
        $hash = { 33 C0 B8 ?? ?? ?? ?? }
    condition:
        any of ($peb_*) or 2 of them
}
