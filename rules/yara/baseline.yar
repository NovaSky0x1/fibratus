// Baseline YARA rules shipped with Fibratus Fleet Agent.
// These are threat-specific signatures designed for inline process and
// image-load scanning. They deliberately DO NOT include broad patterns
// like "matches any PE header" or "reads the PEB" — those fire on every
// legitimate Windows process and flood the detection pipeline.
// Operators should layer their own rule sources on top via
// yara.rule.paths in fibratus.yml (or via the dashboard YARA Rules page
// on fleet-managed agents).

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
