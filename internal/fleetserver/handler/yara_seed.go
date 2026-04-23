package handler

import (
	"context"
	"regexp"
	"strings"

	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// baselineYARARules is the conservative starter rule set shipped with the
// fleet agent on disk (rules/yara/baseline.yar). We also seed it into any
// account that has zero yara_rules rows so the server-managed rule set
// isn't empty on first deployment. Users can disable or edit any of
// these from the dashboard.
const baselineYARARules = `rule InMemory_PE_Header
{
    meta:
        description = "PE/MZ header found inside a process memory region"
        threat_name = "Memory-resident PE loader"
        severity = "medium"
        score = 40
        author = "Fibratus"
    strings:
        $mz = { 4D 5A }
        $pe = { 50 45 00 00 }
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
        $b1 = { 66 83 38 4D 5A }
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
        $peb_x86 = { 64 A1 30 00 00 00 }
        $peb_x64 = { 65 48 8B 04 25 60 00 00 00 }
        $hash = { 33 C0 B8 ?? ?? ?? ?? }
    condition:
        any of ($peb_*) or 2 of them
}
`

// ruleStartRe matches the start of a YARA rule declaration.
var ruleStartRe = regexp.MustCompile(`(?m)^(?:private\s+|global\s+)*rule\s+([A-Za-z_][A-Za-z0-9_]*)`)

// splitBaseline splits the baseline blob into individual (name, content) pairs.
// Rules are delimited by their `rule NAME` declarations; we walk start indices
// and slice between consecutive matches.
func splitBaseline(blob string) []struct{ Name, Content string } {
	matches := ruleStartRe.FindAllStringSubmatchIndex(blob, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]struct{ Name, Content string }, 0, len(matches))
	for i, m := range matches {
		name := blob[m[2]:m[3]]
		start := m[0]
		end := len(blob)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		content := strings.TrimSpace(blob[start:end])
		out = append(out, struct{ Name, Content string }{Name: name, Content: content})
	}
	return out
}

// SeedBaselineYARARules ensures each account has at least the baseline rule
// set. Rules are only inserted when the account currently has zero rows —
// existing accounts with their own rule collection are untouched.
func SeedBaselineYARARules(
	ctx context.Context,
	accounts store.AccountStore,
	yaraRules store.YARARuleStore,
) {
	if accounts == nil || yaraRules == nil {
		return
	}
	accs, err := accounts.ListAll(ctx)
	if err != nil {
		log.Warnf("yara seed: list accounts: %v", err)
		return
	}
	baseline := splitBaseline(baselineYARARules)
	for _, acct := range accs {
		existing, err := yaraRules.List(ctx, acct.ID)
		if err != nil {
			log.Warnf("yara seed: list rules for account %s: %v", acct.ID, err)
			continue
		}
		if len(existing) > 0 {
			continue
		}
		for _, b := range baseline {
			r := &fleet.YARARule{
				ID:               GenerateID(),
				AccountID:        acct.ID,
				Name:             b.Name,
				Description:      "Baseline Fibratus rule — shipped on first deployment",
				Content:          b.Content,
				Enabled:          true,
				ValidationStatus: "valid",
			}
			if err := yaraRules.Create(ctx, r); err != nil {
				log.Warnf("yara seed: create %s for account %s: %v", b.Name, acct.ID, err)
			}
		}
		log.Infof("yara seed: inserted %d baseline YARA rules into account %s (%s)", len(baseline), acct.Name, acct.ID)
	}
}
