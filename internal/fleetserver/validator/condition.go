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

package validator

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// ConditionValidationResult contains the outcome of condition validation.
type ConditionValidationResult struct {
	Valid       bool                `json:"valid"`
	Errors      []ConditionError    `json:"errors,omitempty"`
	Warnings    []string            `json:"warnings,omitempty"`
	IsSequence  bool                `json:"is_sequence"`
}

// ConditionError describes a specific validation error with a suggested fix.
type ConditionError struct {
	Type       string `json:"type"`       // "syntax", "unknown_operator", "unknown_field", "unknown_function", "unknown_event", "escape", "sequence"
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
	Position   int    `json:"position,omitempty"` // character offset if available
}

// ValidateCondition performs comprehensive server-side validation of a filter
// condition string. It checks operators, field names, function names, string
// literals, sequence syntax, event names, and common formatting issues that
// would cause the agent's rule compiler to fail.
func ValidateCondition(condition string) *ConditionValidationResult {
	result := &ConditionValidationResult{Valid: true}
	condition = strings.TrimSpace(condition)

	if condition == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ConditionError{
			Type:    "syntax",
			Message: "condition is empty",
		})
		return result
	}

	// Detect sequence rules
	result.IsSequence = isSequenceCondition(condition)

	// Run all validation checks
	checkStringLiterals(condition, result)
	checkBalancedDelimiters(condition, result)
	checkOperators(condition, result)
	checkFields(condition, result)
	checkFunctions(condition, result)
	checkEventNames(condition, result)
	checkEventCategories(condition, result)
	checkEscaping(condition, result)

	if result.IsSequence {
		checkSequenceSyntax(condition, result)
	}

	return result
}

// ═══════════════════════════════════════════════════════════════
// String literal extraction (respects quoting to avoid false positives)
// ═══════════════════════════════════════════════════════════════

// extractStringLiterals returns all single-quoted string values and their positions.
func extractStringLiterals(s string) (literals []string, positions []int) {
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			start := i
			i++
			var buf strings.Builder
			for i < len(s) {
				if s[i] == '\\' && i+1 < len(s) {
					buf.WriteByte(s[i])
					buf.WriteByte(s[i+1])
					i += 2
					continue
				}
				if s[i] == '\'' {
					break
				}
				buf.WriteByte(s[i])
				i++
			}
			literals = append(literals, buf.String())
			positions = append(positions, start)
		}
	}
	return
}

// stripStringLiterals replaces all single-quoted strings with placeholder tokens
// so that subsequent checks don't match content inside strings.
func stripStringLiterals(s string) string {
	var out strings.Builder
	inString := false
	for i := 0; i < len(s); i++ {
		if inString {
			if s[i] == '\\' && i+1 < len(s) {
				i++ // skip escaped char
				continue
			}
			if s[i] == '\'' {
				out.WriteString("'__STR__'")
				inString = false
			}
			continue
		}
		if s[i] == '\'' {
			inString = true
			continue
		}
		out.WriteByte(s[i])
	}
	return out.String()
}

// ═══════════════════════════════════════════════════════════════
// Check 1: String literal validation
// ═══════════════════════════════════════════════════════════════

func checkStringLiterals(condition string, result *ConditionValidationResult) {
	depth := 0
	inString := false
	for i := 0; i < len(condition); i++ {
		if inString {
			if condition[i] == '\\' && i+1 < len(condition) {
				next := condition[i+1]
				if next != '\\' && next != 'n' && next != '\'' && next != '"' {
					result.Errors = append(result.Errors, ConditionError{
						Type:       "syntax",
						Message:    fmt.Sprintf("invalid escape sequence '\\%c' at position %d", next, i),
						Suggestion: fmt.Sprintf("only \\\\, \\n, \\', and \\\" are valid escape sequences — use \\\\\\\\ for a literal backslash"),
						Position:   i,
					})
					result.Valid = false
				}
				i++
				continue
			}
			if condition[i] == '\'' {
				inString = false
			}
			continue
		}
		switch condition[i] {
		case '\'':
			inString = true
			depth++
		}
	}
	if inString {
		result.Errors = append(result.Errors, ConditionError{
			Type:       "syntax",
			Message:    "unterminated string literal — missing closing single quote",
			Suggestion: "ensure all strings are properly closed with a matching single quote (')",
		})
		result.Valid = false
	}
	_ = depth
}

// ═══════════════════════════════════════════════════════════════
// Check 2: Balanced delimiters
// ═══════════════════════════════════════════════════════════════

func checkBalancedDelimiters(condition string, result *ConditionValidationResult) {
	stripped := stripStringLiterals(condition)
	parenDepth := 0
	bracketDepth := 0
	pipeCount := 0

	for i, ch := range stripped {
		switch ch {
		case '(':
			parenDepth++
		case ')':
			parenDepth--
			if parenDepth < 0 {
				result.Errors = append(result.Errors, ConditionError{
					Type:     "syntax",
					Message:  fmt.Sprintf("unexpected ')' at position %d", i),
					Position: i,
				})
				result.Valid = false
				return
			}
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
			if bracketDepth < 0 {
				result.Errors = append(result.Errors, ConditionError{
					Type:     "syntax",
					Message:  fmt.Sprintf("unexpected ']' at position %d", i),
					Position: i,
				})
				result.Valid = false
				return
			}
		case '|':
			pipeCount++
		}
	}
	if parenDepth != 0 {
		result.Errors = append(result.Errors, ConditionError{
			Type:       "syntax",
			Message:    fmt.Sprintf("unbalanced parentheses (%d unclosed)", parenDepth),
			Suggestion: "check for missing closing ')' in the condition",
		})
		result.Valid = false
	}
	if bracketDepth != 0 {
		result.Errors = append(result.Errors, ConditionError{
			Type:       "syntax",
			Message:    fmt.Sprintf("unbalanced brackets (%d unclosed)", bracketDepth),
			Suggestion: "check for missing closing ']' in field arguments like ps.ancestor[0]",
		})
		result.Valid = false
	}
	if isSequenceCondition(condition) && pipeCount%2 != 0 {
		result.Errors = append(result.Errors, ConditionError{
			Type:       "sequence",
			Message:    "odd number of pipe '|' delimiters in sequence — each expression must be enclosed in |...|",
			Suggestion: "ensure each sequence expression is wrapped in pipes: |expr1| |expr2|",
		})
		result.Valid = false
	}
}

// ═══════════════════════════════════════════════════════════════
// Check 3: Operator validation
// ═══════════════════════════════════════════════════════════════

// validKeywordOperators are all keyword-based operators recognized by the QL lexer.
var validKeywordOperators = map[string]bool{
	"and": true, "or": true, "not": true,
	"contains": true, "icontains": true,
	"in": true, "iin": true,
	"startswith": true, "istartswith": true,
	"endswith": true, "iendswith": true,
	"matches": true, "imatches": true,
	"fuzzy": true, "ifuzzy": true,
	"fuzzynorm": true, "ifuzzynorm": true,
	"intersects": true, "iintersects": true,
}

// validSequenceKeywords are sequence-related keywords.
var validSequenceKeywords = map[string]bool{
	"sequence": true, "maxspan": true, "by": true, "as": true,
}

// symbolOperatorPattern matches symbol-based operators.
// Valid: =, ~=, !=, <>, <, <=, >, >=
var symbolOperatorPattern = regexp.MustCompile(`~=|!=|<>|<=|>=|[=<>]`)

func checkOperators(condition string, result *ConditionValidationResult) {
	stripped := stripStringLiterals(condition)

	// Tokenize by whitespace and delimiters, check each word token
	tokens := tokenizeCondition(stripped)
	for _, tok := range tokens {
		lower := strings.ToLower(tok.value)
		// Skip if it's a known keyword, field, function, boolean, number, or bound variable
		if validKeywordOperators[lower] || validSequenceKeywords[lower] ||
			lower == "true" || lower == "false" ||
			isKnownFieldOrPrefix(tok.value) || isKnownFunction(lower) ||
			strings.HasPrefix(tok.value, "$") || isNumber(tok.value) ||
			isIPAddress(tok.value) || tok.value == "__STR__" {
			continue
		}
		// Check for common invalid operators from other rule languages
		invalidOps := map[string]string{
			"eq":         "use '=' for equality",
			"ne":         "use '!=' for not-equal",
			"like":       "use 'matches' or 'imatches' for pattern matching",
			"regex":      "'regex' is a function, not an operator — use regex(field, 'pattern')",
			"between":    "use field > X and field < Y instead",
			"is":         "use '=' for equality or 'in' for set membership",
			"has":        "use 'contains' or 'icontains' for substring matching",
			"any":        "use 'in' or 'iin' for set membership",
			"all":        "use 'intersects' for set intersection",
			"cidr":       "'cidr_contains' is a function — use cidr_contains(field, 'cidr')",
			"notin":      "use 'not ... in (...)' instead",
			"notcontains": "use 'not ... contains ...' instead",
		}
		if suggestion, ok := invalidOps[lower]; ok {
			result.Errors = append(result.Errors, ConditionError{
				Type:       "unknown_operator",
				Message:    fmt.Sprintf("'%s' is not a valid operator", tok.value),
				Suggestion: suggestion,
			})
			result.Valid = false
		}
	}
}

// ═══════════════════════════════════════════════════════════════
// Check 4: Field name validation
// ═══════════════════════════════════════════════════════════════

func checkFields(condition string, result *ConditionValidationResult) {
	stripped := stripStringLiterals(condition)
	tokens := tokenizeCondition(stripped)

	for _, tok := range tokens {
		// Skip non-field tokens
		if !strings.Contains(tok.value, ".") || strings.HasPrefix(tok.value, "$") {
			continue
		}
		lower := strings.ToLower(tok.value)
		if validKeywordOperators[lower] || validSequenceKeywords[lower] ||
			isKnownFunction(lower) || isNumber(tok.value) || isIPAddress(tok.value) ||
			tok.value == "__STR__" {
			continue
		}

		// Strip bracket argument if present (e.g., "evt.arg[cmdline]" → "evt.arg")
		fieldBase := tok.value
		if idx := strings.Index(fieldBase, "["); idx >= 0 {
			fieldBase = fieldBase[:idx]
		}

		if !isKnownFieldOrPrefix(fieldBase) {
			suggestion := suggestField(fieldBase)
			result.Errors = append(result.Errors, ConditionError{
				Type:       "unknown_field",
				Message:    fmt.Sprintf("'%s' is not a recognized filter field", fieldBase),
				Suggestion: suggestion,
			})
			result.Valid = false
		} else {
			// Check for deprecated fields
			if dep, ok := deprecatedFields[fieldBase]; ok {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("'%s' is deprecated since %s — use %s instead", fieldBase, dep.since, dep.replacement))
			}
		}
	}
}

// ═══════════════════════════════════════════════════════════════
// Check 5: Function validation
// ═══════════════════════════════════════════════════════════════

func checkFunctions(condition string, result *ConditionValidationResult) {
	stripped := stripStringLiterals(condition)
	// Match function calls: word followed by (
	funcCallPattern := regexp.MustCompile(`\b([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)
	matches := funcCallPattern.FindAllStringSubmatch(stripped, -1)

	for _, m := range matches {
		name := strings.ToLower(m[1])
		// Skip if it's a keyword operator (they don't use parens, but just in case)
		if validKeywordOperators[name] || validSequenceKeywords[name] {
			continue
		}
		if !isKnownFunction(name) {
			suggestion := suggestFunction(name)
			result.Errors = append(result.Errors, ConditionError{
				Type:       "unknown_function",
				Message:    fmt.Sprintf("'%s' is not a recognized function", m[1]),
				Suggestion: suggestion,
			})
			result.Valid = false
		}
	}
}

// ═══════════════════════════════════════════════════════════════
// Check 6: Event name validation
// ═══════════════════════════════════════════════════════════════

func checkEventNames(condition string, result *ConditionValidationResult) {
	stripped := stripStringLiterals(condition)
	// Look for evt.name = 'Value' or kevt.name = 'Value' patterns
	pattern := regexp.MustCompile(`(?i)(?:evt|kevt)\.name\s*(?:=|!=|~=|<>)\s*`)
	locs := pattern.FindAllStringIndex(stripped, -1)

	// Now find the string literal that follows each match
	literals, positions := extractStringLiterals(condition)
	for _, loc := range locs {
		matchEnd := loc[1]
		// Find the closest string literal after the match
		for i, pos := range positions {
			if pos >= matchEnd-2 && pos <= matchEnd+5 {
				evtName := literals[i]
				if !isKnownEventName(evtName) {
					suggestion := suggestEventName(evtName)
					result.Errors = append(result.Errors, ConditionError{
						Type:       "unknown_event",
						Message:    fmt.Sprintf("'%s' is not a recognized event name", evtName),
						Suggestion: suggestion,
					})
					result.Valid = false
				}
				break
			}
		}
	}

	// Also check IN lists: evt.name in ('CreateProcess', 'OpenProcess')
	inPattern := regexp.MustCompile(`(?i)(?:evt|kevt)\.name\s+(?:i?in)\s*\(`)
	inLocs := inPattern.FindAllStringIndex(stripped, -1)
	for _, loc := range inLocs {
		matchEnd := loc[1]
		// Find closing paren in stripped
		parenEnd := strings.Index(stripped[matchEnd:], ")")
		if parenEnd < 0 {
			continue
		}
		// Find all string literals in this range of the original condition
		for i, pos := range positions {
			if pos >= loc[0] && pos <= matchEnd+parenEnd+loc[0] {
				evtName := literals[i]
				if !isKnownEventName(evtName) {
					suggestion := suggestEventName(evtName)
					result.Errors = append(result.Errors, ConditionError{
						Type:       "unknown_event",
						Message:    fmt.Sprintf("'%s' is not a recognized event name", evtName),
						Suggestion: suggestion,
					})
					result.Valid = false
				}
			}
		}
	}
}

// ═══════════════════════════════════════════════════════════════
// Check 7: Event category validation
// ═══════════════════════════════════════════════════════════════

func checkEventCategories(condition string, result *ConditionValidationResult) {
	literals, positions := extractStringLiterals(condition)
	stripped := stripStringLiterals(condition)

	pattern := regexp.MustCompile(`(?i)(?:evt|kevt)\.category\s*(?:=|!=|~=|<>)\s*`)
	locs := pattern.FindAllStringIndex(stripped, -1)

	for _, loc := range locs {
		matchEnd := loc[1]
		for i, pos := range positions {
			if pos >= matchEnd-2 && pos <= matchEnd+5 {
				cat := literals[i]
				if !isKnownEventCategory(cat) {
					result.Errors = append(result.Errors, ConditionError{
						Type:       "unknown_event",
						Message:    fmt.Sprintf("'%s' is not a recognized event category", cat),
						Suggestion: fmt.Sprintf("valid categories: %s", strings.Join(validEventCategories(), ", ")),
					})
					result.Valid = false
				}
				break
			}
		}
	}
	_ = positions
}

// ═══════════════════════════════════════════════════════════════
// Check 8: Over-escaped backslashes (DB round-trip corruption)
// ═══════════════════════════════════════════════════════════════

func checkEscaping(condition string, result *ConditionValidationResult) {
	// Detect quadruple+ backslash patterns that indicate DB round-trip corruption
	if strings.Contains(condition, `\\\\`) {
		result.Errors = append(result.Errors, ConditionError{
			Type:       "escape",
			Message:    "condition contains over-escaped backslashes (\\\\\\\\) — likely corrupted during database storage",
			Suggestion: "replace \\\\\\\\\\\\\\\\ with \\\\ for Windows paths",
		})
		result.Valid = false
	}
}

// ═══════════════════════════════════════════════════════════════
// Check 9: Sequence syntax validation
// ═══════════════════════════════════════════════════════════════

func checkSequenceSyntax(condition string, result *ConditionValidationResult) {
	stripped := stripStringLiterals(condition)
	lower := strings.ToLower(stripped)

	// Must start with "sequence"
	trimmed := strings.TrimSpace(lower)
	if !strings.HasPrefix(trimmed, "sequence") {
		return
	}

	// Count pipe-delimited expressions
	pipeCount := strings.Count(stripped, "|")
	exprCount := pipeCount / 2

	if exprCount < 2 {
		result.Errors = append(result.Errors, ConditionError{
			Type:       "sequence",
			Message:    fmt.Sprintf("sequences require at least 2 expressions but found %d", exprCount),
			Suggestion: "add at least 2 pipe-delimited expressions: sequence |expr1| |expr2|",
		})
		result.Valid = false
	}
	if exprCount > 5 {
		result.Errors = append(result.Errors, ConditionError{
			Type:       "sequence",
			Message:    fmt.Sprintf("maximum 5 sequence expressions allowed but found %d", exprCount),
			Suggestion: "split into multiple rules or reduce sequence expressions",
		})
		result.Valid = false
	}

	// Check for maxspan if present
	if strings.Contains(lower, "maxspan") {
		maxspanPattern := regexp.MustCompile(`(?i)maxspan\s+(\d+[a-z]+)`)
		m := maxspanPattern.FindStringSubmatch(lower)
		if m != nil {
			dur := m[1]
			// Check for durations > 4h (the engine limit)
			if isOverFourHours(dur) {
				result.Errors = append(result.Errors, ConditionError{
					Type:       "sequence",
					Message:    fmt.Sprintf("maxspan %s exceeds 4 hour limit", dur),
					Suggestion: "reduce maxspan to 4h or less",
				})
				result.Valid = false
			}
		}
	}
}

// ═══════════════════════════════════════════════════════════════
// Token helpers
// ═══════════════════════════════════════════════════════════════

type condToken struct {
	value    string
	position int
}

// tokenizeCondition splits a condition (with strings already stripped) into word tokens.
func tokenizeCondition(stripped string) []condToken {
	var tokens []condToken
	i := 0
	for i < len(stripped) {
		ch := stripped[i]
		// Skip whitespace
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			i++
			continue
		}
		// Skip symbol operators and punctuation
		if ch == '(' || ch == ')' || ch == '[' || ch == ']' || ch == ',' || ch == '|' {
			i++
			continue
		}
		// Symbol operators
		if ch == '=' || ch == '!' || ch == '<' || ch == '>' || ch == '~' {
			i++
			if i < len(stripped) && stripped[i] == '=' {
				i++
			} else if ch == '<' && i < len(stripped) && stripped[i] == '>' {
				i++
			}
			continue
		}
		// Identifier or number
		if isIdentStart(ch) || ch == '$' || isDigitByte(ch) {
			start := i
			for i < len(stripped) && isIdentChar(stripped[i]) {
				i++
			}
			// Include bracket arguments as part of the token
			if i < len(stripped) && stripped[i] == '[' {
				for i < len(stripped) && stripped[i] != ']' {
					i++
				}
				if i < len(stripped) {
					i++ // skip ]
				}
			}
			tokens = append(tokens, condToken{value: stripped[start:i], position: start})
			continue
		}
		// Skip single quote placeholders
		if ch == '\'' {
			for i < len(stripped) && stripped[i] != '\'' {
				i++
			}
			if i < len(stripped) {
				i++
			}
			continue
		}
		i++
	}
	return tokens
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdentChar(ch byte) bool {
	return isIdentStart(ch) || isDigitByte(ch) || ch == '.' || ch == '$'
}

func isDigitByte(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isNumber(s string) bool {
	if len(s) == 0 {
		return false
	}
	hasDot := false
	for i, r := range s {
		if r == '-' && i == 0 {
			continue
		}
		if r == '.' && !hasDot {
			hasDot = true
			continue
		}
		if !unicode.IsDigit(r) {
			// Duration suffix
			if i > 0 && (r == 'h' || r == 'm' || r == 's' || r == 'd' || r == 'w') {
				continue
			}
			return false
		}
	}
	return true
}

func isIPAddress(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if len(p) == 0 || len(p) > 3 {
			return false
		}
		for _, ch := range p {
			if !unicode.IsDigit(ch) {
				return false
			}
		}
	}
	return true
}

func isSequenceCondition(condition string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(condition))
	return strings.HasPrefix(trimmed, "sequence")
}

func isOverFourHours(dur string) bool {
	// Parse simple duration: Nh, Nm, Ns, Nd, Nw
	if len(dur) < 2 {
		return false
	}
	unit := dur[len(dur)-1]
	numStr := dur[:len(dur)-1]
	var num int
	for _, ch := range numStr {
		if ch >= '0' && ch <= '9' {
			num = num*10 + int(ch-'0')
		}
	}
	switch unit {
	case 'h':
		return num > 4
	case 'd':
		return true // any day is > 4h
	case 'w':
		return true
	case 'm':
		return num > 240
	case 's':
		return num > 14400
	}
	return false
}

// ═══════════════════════════════════════════════════════════════
// Known fields registry — ALL fields from fields_windows.go
// ═══════════════════════════════════════════════════════════════

// knownFields contains every single field string recognized by the Fibratus
// filter engine. Extracted from pkg/filter/fields/fields_windows.go.
var knownFields = map[string]bool{
	// Process fields
	"ps.pid": true, "ps.ppid": true, "ps.name": true, "ps.comm": true,
	"ps.cmdline": true, "ps.exe": true, "ps.args": true, "ps.cwd": true,
	"ps.sid": true, "ps.domain": true, "ps.username": true, "ps.sessionid": true,
	"ps.envs": true, "ps.handles": true, "ps.handle.types": true, "ps.dtb": true,
	"ps.modules": true,
	"ps.parent.pid": true, "ps.parent.name": true, "ps.parent.comm": true,
	"ps.parent.cmdline": true, "ps.parent.exe": true, "ps.parent.args": true,
	"ps.parent.cwd": true, "ps.parent.sid": true, "ps.parent.username": true,
	"ps.parent.domain": true, "ps.parent.sessionid": true, "ps.parent.envs": true,
	"ps.parent.handles": true, "ps.parent.handle.types": true, "ps.parent.dtb": true,
	"ps.ancestor": true,
	"ps.access.mask": true, "ps.access.mask.names": true, "ps.access.status": true,
	"ps.is_wow64": true, "ps.is_packaged": true, "ps.is_protected": true,
	"ps.parent.is_wow64": true, "ps.parent.is_packaged": true, "ps.parent.is_protected": true,
	"ps.uuid": true, "ps.parent.uuid": true,
	"ps.token.integrity_level": true, "ps.token.is_elevated": true, "ps.token.elevation_type": true,
	"ps.parent.token.integrity_level": true, "ps.parent.token.is_elevated": true, "ps.parent.token.elevation_type": true,
	"ps.signature.exists": true, "ps.signature.trusted": true,
	"ps.signature.issuer": true, "ps.signature.subject": true, "ps.signature.serial": true,
	"ps.signature.after": true, "ps.signature.before": true,

	// Process PE fields
	"ps.pe.nsections": true, "ps.pe.nsymbols": true, "ps.pe.symbols": true,
	"ps.pe.imports": true, "ps.pe.timestamp": true,
	"ps.pe.address.base": true, "ps.pe.address.entrypoint": true,
	"ps.pe.resources": true, "ps.pe.company": true, "ps.pe.description": true,
	"ps.pe.file.version": true, "ps.pe.file.name": true, "ps.pe.copyright": true,
	"ps.pe.product": true, "ps.pe.product.version": true,
	"ps.pe.anomalies": true, "ps.pe.imphash": true,
	"ps.pe.is_dotnet": true, "ps.pe.is_modified": true,

	// Thread fields
	"thread.prio": true, "thread.io.prio": true, "thread.page.prio": true,
	"thread.kstack.base": true, "thread.kstack.limit": true,
	"thread.ustack.base": true, "thread.ustack.limit": true,
	"thread.entrypoint": true, "thread.start_address": true,
	"thread.pid": true, "thread.teb_address": true,
	"thread.access.mask": true, "thread.access.mask.names": true, "thread.access.status": true,
	"thread.callstack.summary": true, "thread.callstack.detail": true,
	"thread.callstack.modules": true, "thread.callstack.symbols": true,
	"thread.callstack.protections": true, "thread.callstack.allocation_sizes": true,
	"thread.callstack.callsite_leading_assembly": true, "thread.callstack.callsite_trailing_assembly": true,
	"thread.callstack.is_unbacked": true,
	"thread.start_address.symbol": true, "thread.start_address.module": true,
	"thread.callstack.addresses": true,
	"thread.callstack.final_user_module.name": true, "thread.callstack.final_user_module.path": true,
	"thread.callstack.final_user_symbol.name": true,
	"thread.callstack.final_kernel_module.name": true, "thread.callstack.final_kernel_module.path": true,
	"thread.callstack.final_kernel_symbol.name": true,
	"thread.callstack.final_user_module.signature.exists": true,
	"thread.callstack.final_user_module.signature.trusted": true,
	"thread.callstack.final_user_module.signature.issuer": true,
	"thread.callstack.final_user_module.signature.subject": true,

	// PE fields (standalone, used for image events)
	"pe.nsections": true, "pe.nsymbols": true, "pe.symbols": true,
	"pe.imports": true, "pe.timestamp": true,
	"pe.address.base": true, "pe.address.entrypoint": true,
	"pe.resources": true, "pe.company": true, "pe.description": true,
	"pe.file.version": true, "pe.file.name": true, "pe.copyright": true,
	"pe.product": true, "pe.product.version": true,
	"pe.is_dll": true, "pe.is_driver": true, "pe.is_exec": true,
	"pe.anomalies": true, "pe.imphash": true, "pe.is_dotnet": true,
	"pe.is_signed": true, "pe.is_trusted": true,
	"pe.cert.issuer": true, "pe.cert.subject": true, "pe.cert.serial": true,
	"pe.cert.after": true, "pe.cert.before": true, "pe.is_modified": true,

	// Event fields
	"evt.seq": true, "evt.pid": true, "evt.tid": true, "evt.cpu": true,
	"evt.desc": true, "evt.host": true,
	"evt.time": true, "evt.time.h": true, "evt.time.m": true, "evt.time.s": true, "evt.time.ns": true,
	"evt.date": true, "evt.date.d": true, "evt.date.m": true, "evt.date.y": true,
	"evt.date.tz": true, "evt.date.week": true, "evt.date.weekday": true,
	"evt.name": true, "evt.category": true, "evt.nparams": true, "evt.arg": true,
	"evt.is_direct_syscall": true, "evt.is_indirect_syscall": true,

	// Deprecated kevt.* fields (still recognized)
	"kevt.seq": true, "kevt.pid": true, "kevt.tid": true, "kevt.cpu": true,
	"kevt.desc": true, "kevt.host": true,
	"kevt.time": true, "kevt.time.h": true, "kevt.time.m": true, "kevt.time.s": true, "kevt.time.ns": true,
	"kevt.date": true, "kevt.date.d": true, "kevt.date.m": true, "kevt.date.y": true,
	"kevt.date.tz": true, "kevt.date.week": true, "kevt.date.weekday": true,
	"kevt.name": true, "kevt.category": true, "kevt.nparams": true, "kevt.arg": true,

	// Handle fields
	"handle.id": true, "handle.object": true, "handle.name": true, "handle.type": true,

	// Network fields
	"net.dip": true, "net.sip": true, "net.dport": true, "net.sport": true,
	"net.dport.name": true, "net.sport.name": true, "net.l4.proto": true,
	"net.size": true, "net.sip.names": true, "net.dip.names": true,

	// File fields
	"file.object": true, "file.name": true, "file.path": true, "file.path.stem": true,
	"file.extension": true, "file.operation": true, "file.share.mask": true,
	"file.io.size": true, "file.offset": true, "file.type": true,
	"file.attributes": true, "file.status": true,
	"file.view.base": true, "file.view.size": true, "file.view.type": true, "file.view.protection": true,
	"file.is_driver_vulnerable": true, "file.is_driver_malicious": true,
	"file.is_dll": true, "file.is_driver": true, "file.is_exec": true,
	"file.pid": true, "file.key": true, "file.info_class": true,
	"file.info.allocation_size": true, "file.info.eof_size": true,
	"file.info.is_disposition_delete_file": true,

	// Registry fields
	"registry.path": true, "registry.key.name": true, "registry.key.handle": true,
	"registry.value": true, "registry.value.type": true, "registry.data": true,
	"registry.status": true,

	// Image fields
	"image.base.address": true, "image.size": true, "image.checksum": true,
	"image.default.address": true, "image.path": true, "image.name": true,
	"image.pid": true, "image.signature.type": true, "image.signature.level": true,
	"image.cert.subject": true, "image.cert.issuer": true, "image.cert.serial": true,
	"image.cert.before": true, "image.cert.after": true,
	"image.is_driver_vulnerable": true, "image.is_driver_malicious": true,
	"image.is_dll": true, "image.is_driver": true, "image.is_exec": true,
	"image.is_dotnet": true,

	// DLL fields
	"dll.base": true, "dll.size": true, "dll.path": true, "dll.path.stem": true,
	"dll.name": true, "dll.pid": true,
	"dll.signature.type": true, "dll.signature.level": true,
	"dll.signature.exists": true, "dll.signature.trusted": true,
	"dll.signature.subject": true, "dll.signature.issuer": true, "dll.signature.serial": true,
	"dll.signature.before": true, "dll.signature.after": true,
	"dll.pe.is_dotnet": true,

	// Module fields
	"module.base": true, "module.size": true, "module.checksum": true,
	"module.default_address": true, "module.path": true, "module.path.stem": true,
	"module.name": true, "module.pid": true,
	"module.signature.type": true, "module.signature.level": true,
	"module.signature.exists": true, "module.signature.trusted": true,
	"module.signature.subject": true, "module.signature.issuer": true, "module.signature.serial": true,
	"module.signature.before": true, "module.signature.after": true,
	"module.is_driver_vulnerable": true, "module.is_driver_malicious": true,
	"module.is_dll": true, "module.is_driver": true, "module.is_exec": true,
	"module.pe.is_dotnet": true,

	// Memory fields
	"mem.address": true, "mem.size": true, "mem.alloc": true,
	"mem.type": true, "mem.protection": true, "mem.protection.mask": true,

	// DNS fields
	"dns.name": true, "dns.rr": true, "dns.options": true,
	"dns.answers": true, "dns.rcode": true,

	// Threadpool fields
	"threadpool.id": true, "threadpool.task.id": true,
	"threadpool.callback.address": true, "threadpool.callback.symbol": true,
	"threadpool.callback.module": true, "threadpool.callback.context": true,
	"threadpool.callback.context.rip": true, "threadpool.callback.context.rip.symbol": true,
	"threadpool.callback.context.rip.module": true,
	"threadpool.subprocess_tag": true,
	"threadpool.timer.duetime": true, "threadpool.timer.subqueue": true,
	"threadpool.timer.address": true, "threadpool.timer.period": true,
	"threadpool.timer.window": true, "threadpool.timer.is_absolute": true,
}

// validFieldPrefixes are the top-level field category prefixes.
var validFieldPrefixes = []string{
	"ps.", "thread.", "evt.", "kevt.", "pe.", "file.", "registry.",
	"net.", "image.", "handle.", "mem.", "dns.", "dll.", "module.",
	"threadpool.",
}

func isKnownFieldOrPrefix(name string) bool {
	if knownFields[name] {
		return true
	}
	// Check if it's a pseudo field
	pseudoFields := map[string]bool{
		"ps._modules": true, "ps._threads": true, "ps._mmaps": true,
		"ps._ancestors": true, "ps.pe._sections": true,
		"thread._callstack": true, "pe._sections": true,
	}
	if pseudoFields[name] {
		return true
	}
	return false
}

func suggestField(name string) string {
	// Check if it has a valid prefix
	hasValidPrefix := false
	for _, prefix := range validFieldPrefixes {
		if strings.HasPrefix(name, prefix) {
			hasValidPrefix = true
			break
		}
	}
	if !hasValidPrefix {
		return fmt.Sprintf("field must start with a known prefix: %s", strings.Join(validFieldPrefixes, ", "))
	}
	// Try to find a close match
	best := ""
	bestDist := 999
	for f := range knownFields {
		if strings.HasPrefix(f, name[:strings.Index(name, ".")+1]) {
			d := levenshtein(name, f)
			if d < bestDist && d <= 3 {
				bestDist = d
				best = f
			}
		}
	}
	if best != "" {
		return fmt.Sprintf("did you mean '%s'?", best)
	}
	return "check the Fibratus documentation for valid field names"
}

// ═══════════════════════════════════════════════════════════════
// Known functions registry — ALL functions from QL function.go
// ═══════════════════════════════════════════════════════════════

var knownFunctions = map[string]bool{
	"cidr_contains": true, "md5": true, "concat": true,
	"ltrim": true, "rtrim": true, "lower": true, "upper": true,
	"replace": true, "split": true, "length": true,
	"indexof": true, "substr": true, "entropy": true,
	"regex": true, "is_minidump": true,
	"base": true, "dir": true, "symlink": true, "ext": true,
	"glob": true, "is_abs": true, "volume": true,
	"get_reg_value": true, "yara": true, "foreach": true, "count": true,
}

func isKnownFunction(name string) bool {
	return knownFunctions[strings.ToLower(name)]
}

func suggestFunction(name string) string {
	lower := strings.ToLower(name)
	suggestions := map[string]string{
		"contains":    "'contains' is an operator, not a function — use: field contains 'value'",
		"startswith":  "'startswith' is an operator, not a function — use: field startswith 'value'",
		"endswith":    "'endswith' is an operator, not a function — use: field endswith 'value'",
		"match":       "did you mean 'matches' (operator) or 'regex' (function)?",
		"regexp":      "did you mean 'regex'?",
		"len":         "did you mean 'length'?",
		"trim":        "did you mean 'ltrim' or 'rtrim'?",
		"tolower":     "did you mean 'lower'?",
		"toupper":     "did you mean 'upper'?",
		"basename":    "did you mean 'base'?",
		"dirname":     "did you mean 'dir'?",
		"extension":   "did you mean 'ext'?",
		"hash":        "did you mean 'md5'?",
		"sha256":      "only 'md5' hash function is available",
		"cidr":        "did you mean 'cidr_contains'?",
		"ip_in_cidr":  "did you mean 'cidr_contains'?",
	}
	if s, ok := suggestions[lower]; ok {
		return s
	}
	// Levenshtein closest match
	best := ""
	bestDist := 999
	for f := range knownFunctions {
		d := levenshtein(lower, f)
		if d < bestDist && d <= 3 {
			bestDist = d
			best = f
		}
	}
	if best != "" {
		return fmt.Sprintf("did you mean '%s'?", best)
	}
	return "check the Fibratus documentation for available functions"
}

// ═══════════════════════════════════════════════════════════════
// Known event names — ALL from event/metainfo_windows.go
// ═══════════════════════════════════════════════════════════════

var knownEventNames = map[string]bool{
	// Process
	"CreateProcess": true, "TerminateProcess": true, "OpenProcess": true,
	// Thread
	"CreateThread": true, "TerminateThread": true, "OpenThread": true, "SetThreadContext": true,
	// File
	"ReadFile": true, "WriteFile": true, "CreateFile": true, "CloseFile": true,
	"DeleteFile": true, "RenameFile": true, "SetFileInformation": true,
	"EnumDirectory": true, "MapViewFile": true, "UnmapViewFile": true,
	// Registry
	"RegCreateKey": true, "RegOpenKey": true, "RegCloseKey": true,
	"RegSetValue": true, "RegQueryValue": true, "RegQueryKey": true,
	"RegDeleteKey": true, "RegDeleteValue": true,
	// Network
	"Accept": true, "Send": true, "Recv": true, "Connect": true,
	"Reconnect": true, "Disconnect": true, "Retransmit": true,
	"QueryDns": true, "ReplyDns": true,
	// Image/Module
	"LoadImage": true, "UnloadImage": true,
	// Handle
	"CreateHandle": true, "CloseHandle": true, "DuplicateHandle": true,
	// Memory
	"VirtualAlloc": true, "VirtualFree": true,
	// Object
	"CreateSymbolicLinkObject": true,
	// Threadpool
	"SubmitThreadpoolWork": true, "SubmitThreadpoolCallback": true, "SetThreadpoolTimer": true,
}

func isKnownEventName(name string) bool {
	return knownEventNames[name]
}

func suggestEventName(name string) string {
	best := ""
	bestDist := 999
	for n := range knownEventNames {
		d := levenshtein(strings.ToLower(name), strings.ToLower(n))
		if d < bestDist && d <= 4 {
			bestDist = d
			best = n
		}
	}
	if best != "" {
		return fmt.Sprintf("did you mean '%s'?", best)
	}
	return "check Fibratus documentation for valid event names (e.g., CreateProcess, LoadImage, Send, RegSetValue)"
}

// ═══════════════════════════════════════════════════════════════
// Known event categories — ALL from event/category.go
// ═══════════════════════════════════════════════════════════════

var knownEventCategoriesMap = map[string]bool{
	"registry": true, "file": true, "net": true,
	"process": true, "thread": true, "image": true,
	"handle": true, "driver": true, "mem": true,
	"object": true, "threadpool": true, "other": true,
	"unknown": true,
}

func isKnownEventCategory(name string) bool {
	return knownEventCategoriesMap[name]
}

func validEventCategories() []string {
	return []string{"registry", "file", "net", "process", "thread", "image",
		"handle", "driver", "mem", "object", "threadpool", "other"}
}

// ═══════════════════════════════════════════════════════════════
// Deprecated fields
// ═══════════════════════════════════════════════════════════════

type deprecation struct {
	since       string
	replacement string
}

var deprecatedFields = map[string]deprecation{
	"ps.comm":            {since: "1.10.0", replacement: "ps.cmdline"},
	"ps.parent.comm":     {since: "1.10.0", replacement: "ps.parent.cmdline"},
	"kevt.seq":           {since: "3.0.0", replacement: "evt.seq"},
	"kevt.pid":           {since: "3.0.0", replacement: "evt.pid"},
	"kevt.tid":           {since: "3.0.0", replacement: "evt.tid"},
	"kevt.cpu":           {since: "3.0.0", replacement: "evt.cpu"},
	"kevt.desc":          {since: "3.0.0", replacement: "evt.desc"},
	"kevt.host":          {since: "3.0.0", replacement: "evt.host"},
	"kevt.time":          {since: "3.0.0", replacement: "evt.time"},
	"kevt.time.h":        {since: "3.0.0", replacement: "evt.time.h"},
	"kevt.time.m":        {since: "3.0.0", replacement: "evt.time.m"},
	"kevt.time.s":        {since: "3.0.0", replacement: "evt.time.s"},
	"kevt.time.ns":       {since: "3.0.0", replacement: "evt.time.ns"},
	"kevt.date":          {since: "3.0.0", replacement: "evt.date"},
	"kevt.date.d":        {since: "3.0.0", replacement: "evt.date.d"},
	"kevt.date.m":        {since: "3.0.0", replacement: "evt.date.m"},
	"kevt.date.y":        {since: "3.0.0", replacement: "evt.date.y"},
	"kevt.date.tz":       {since: "3.0.0", replacement: "evt.date.tz"},
	"kevt.date.week":     {since: "3.0.0", replacement: "evt.date.week"},
	"kevt.date.weekday":  {since: "3.0.0", replacement: "evt.date.weekday"},
	"kevt.name":          {since: "3.0.0", replacement: "evt.name"},
	"kevt.category":      {since: "3.0.0", replacement: "evt.category"},
	"kevt.nparams":       {since: "3.0.0", replacement: "evt.nparams"},
	"kevt.arg":           {since: "3.0.0", replacement: "evt.arg"},
	"pe.is_dll":          {since: "2.0.0", replacement: "image.is_dll or file.is_dll"},
	"pe.is_driver":       {since: "2.0.0", replacement: "image.is_driver or file.is_driver"},
	"pe.is_exec":         {since: "2.0.0", replacement: "image.is_exec or file.is_exec"},
	"handle.id":          {since: "3.0.0", replacement: "handle fields are deprecated"},
	"handle.object":      {since: "3.0.0", replacement: "handle fields are deprecated"},
	"handle.name":        {since: "3.0.0", replacement: "handle fields are deprecated"},
	"handle.type":        {since: "3.0.0", replacement: "handle fields are deprecated"},
}

// ═══════════════════════════════════════════════════════════════
// Levenshtein distance for suggestions
// ═══════════════════════════════════════════════════════════════

func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
