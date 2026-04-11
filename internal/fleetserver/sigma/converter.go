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

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// Convert parses a raw SIGMA YAML rule and converts it to a Fibratus detection rule.
func Convert(sigmaYAML []byte) *ConversionResult {
	result := &ConversionResult{Success: false}

	// Parse SIGMA rule
	rule, err := parseSigmaRule(sigmaYAML)
	if err != nil {
		result.Errors = append(result.Errors, "failed to parse SIGMA rule: "+err.Error())
		return result
	}
	result.SigmaID = rule.ID
	result.SigmaTitle = rule.Title

	// Resolve logsource mapping
	cfg, ok := resolveLogsource(rule.Logsource)
	if !ok {
		result.Unconvertible = true
		result.Reason = fmt.Sprintf("unsupported logsource: category=%q product=%q service=%q",
			rule.Logsource.Category, rule.Logsource.Product, rule.Logsource.Service)
		return result
	}

	// Convert detection logic
	condition, warnings, err := convertDetection(rule, cfg)
	if err != nil {
		result.Errors = append(result.Errors, "detection conversion failed: "+err.Error())
		return result
	}
	result.Warnings = warnings

	// Map severity
	severity := mapSeverity(rule.Level)

	// Extract MITRE ATT&CK labels from tags
	labels := extractMITRELabels(rule.Tags)

	// Generate Fibratus rule YAML
	fibratusYAML, err := generateFibratusYAML(rule, condition, severity, labels)
	if err != nil {
		result.Errors = append(result.Errors, "YAML generation failed: "+err.Error())
		return result
	}

	result.Success = true
	result.FibratusYAML = fibratusYAML
	result.RuleName = rule.Title
	result.Condition = condition
	result.Severity = severity
	result.Labels = labels

	return result
}

// ConvertBatch converts multiple SIGMA YAML rules.
func ConvertBatch(rules [][]byte) *BatchConversionResult {
	batch := &BatchConversionResult{
		Total: len(rules),
	}
	for _, raw := range rules {
		result := Convert(raw)
		batch.Results = append(batch.Results, *result)
		if result.Success {
			batch.Converted++
		} else if result.Unconvertible {
			batch.Skipped++
		} else {
			batch.Failed++
		}
	}
	return batch
}

// parseSigmaRule parses a raw SIGMA YAML document into a SigmaRule struct.
// The detection section requires custom handling because its keys are
// dynamic (named selections + a condition key).
func parseSigmaRule(data []byte) (*SigmaRule, error) {
	var rule SigmaRule
	if err := yaml.Unmarshal(data, &rule); err != nil {
		return nil, fmt.Errorf("YAML parse error: %w", err)
	}
	if rule.Title == "" {
		return nil, fmt.Errorf("missing required field: title")
	}

	// Parse detection section manually to capture named selections
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	det, ok := raw["detection"]
	if !ok {
		return nil, fmt.Errorf("missing required field: detection")
	}
	detMap, ok := det.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("detection must be a map")
	}

	rule.Detection.Selections = make(map[string]interface{})
	for k, v := range detMap {
		if k == "condition" {
			rule.Detection.Condition = v
			continue
		}
		rule.Detection.Selections[k] = v
	}

	if rule.Detection.Condition == nil {
		return nil, fmt.Errorf("missing required field: detection.condition")
	}

	return &rule, nil
}

// convertDetection converts the SIGMA detection block into a Fibratus QL condition string.
func convertDetection(rule *SigmaRule, cfg *logsourceConfig) (string, []string, error) {
	var warnings []string

	// Convert each named selection into a Fibratus expression
	selectionExprs := make(map[string]string)
	for name, sel := range rule.Detection.Selections {
		expr, selWarnings, err := convertSelection(name, sel, cfg)
		if err != nil {
			return "", nil, fmt.Errorf("selection %q: %w", name, err)
		}
		warnings = append(warnings, selWarnings...)
		selectionExprs[name] = expr
	}

	// Parse the condition string and substitute selection names with their expressions
	condStr := conditionToString(rule.Detection.Condition)
	condition, err := buildCondition(condStr, selectionExprs)
	if err != nil {
		return "", nil, fmt.Errorf("condition %q: %w", condStr, err)
	}

	// Prepend the logsource event type prefix.
	// Not scoping is handled in buildCondition via (true and not <expr>)
	// wrappers, so no special handling needed here.
	fullCondition := cfg.conditionPrefix + " and\n  " + condition

	// Format for readability: break into multi-line with proper indentation
	fullCondition = formatCondition(fullCondition)

	return fullCondition, warnings, nil
}

// convertSelection converts a single named SIGMA selection into a Fibratus QL expression.
func convertSelection(name string, sel interface{}, cfg *logsourceConfig) (string, []string, error) {
	var warnings []string

	switch v := sel.(type) {
	case map[string]interface{}:
		return convertMapSelection(name, v, cfg)
	case []interface{}:
		// A list at selection level means OR of sub-selections (each item is a map)
		var parts []string
		for i, item := range v {
			m, ok := item.(map[string]interface{})
			if !ok {
				// Could be a plain string list — treat as values for a generic match
				return "", nil, fmt.Errorf("selection %q item %d: expected map, got %T", name, i, item)
			}
			expr, w, err := convertMapSelection(fmt.Sprintf("%s[%d]", name, i), m, cfg)
			if err != nil {
				return "", nil, err
			}
			warnings = append(warnings, w...)
			parts = append(parts, expr)
		}
		if len(parts) == 1 {
			return parts[0], warnings, nil
		}
		return "(" + strings.Join(parts, " or ") + ")", warnings, nil
	default:
		return "", nil, fmt.Errorf("selection %q: unexpected type %T", name, sel)
	}
}

// convertMapSelection converts a map-type SIGMA selection (field→value pairs) to Fibratus QL.
func convertMapSelection(name string, m map[string]interface{}, cfg *logsourceConfig) (string, []string, error) {
	var warnings []string
	var conditions []string

	// Sort keys for deterministic output
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, fieldSpec := range keys {
		value := m[fieldSpec]

		// Parse field name and modifiers: "FieldName|modifier1|modifier2"
		parts := strings.Split(fieldSpec, "|")
		fieldName := parts[0]
		modifiers := parts[1:]

		// Special handling for Sysmon's composite Hashes field.
		// Hashes contains "SHA256=...,MD5=...,IMPHASH=..." — extract the
		// hash type and remap to the correct Fibratus field.
		if strings.EqualFold(fieldName, "Hashes") {
			expr, err := convertHashesField(value, modifiers, cfg)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("field %q in selection %q: %v, skipped", fieldName, name, err))
				continue
			}
			if expr != "" {
				conditions = append(conditions, expr)
			}
			continue
		}

		// Map field to Fibratus
		fibratusField, ok := mapField(fieldName, cfg)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("field %q in selection %q has no Fibratus mapping, skipped", fieldName, name))
			continue
		}

		// Check for unsupported modifiers
		if hasUnsupportedModifier(modifiers) {
			mod := unsupportedModifier(modifiers)
			warnings = append(warnings, fmt.Sprintf("modifier %q on field %q is not directly convertible", mod, fieldName))
			continue
		}

		// Convert the value with modifiers
		expr, err := convertFieldValue(fibratusField, value, modifiers)
		if err != nil {
			return "", nil, fmt.Errorf("field %q: %w", fieldSpec, err)
		}
		if expr != "" {
			conditions = append(conditions, expr)
		}
	}

	if len(conditions) == 0 {
		// All fields in this selection were unmapped — this selection cannot be
		// converted. Return an error rather than "true" which would match everything.
		return "", warnings, fmt.Errorf("selection %q: all fields unmapped, cannot convert", name)
	}
	if len(conditions) == 1 {
		return conditions[0], warnings, nil
	}
	return "(" + strings.Join(conditions, " and ") + ")", warnings, nil
}

// convertFieldValue converts a SIGMA field+value+modifiers into a Fibratus QL expression.
func convertFieldValue(field string, value interface{}, modifiers []string) (string, error) {
	// Determine the operator based on modifiers
	useAll := hasModifier(modifiers, "all")

	// Check for special modifiers
	if hasModifier(modifiers, "cidr") {
		return convertCIDR(field, value)
	}
	if hasModifier(modifiers, "re") {
		return convertRegex(field, value, useAll)
	}
	if hasModifier(modifiers, "gt") {
		return convertComparison(field, value, ">")
	}
	if hasModifier(modifiers, "gte") {
		return convertComparison(field, value, ">=")
	}
	if hasModifier(modifiers, "lt") {
		return convertComparison(field, value, "<")
	}
	if hasModifier(modifiers, "lte") {
		return convertComparison(field, value, "<=")
	}
	if hasModifier(modifiers, "exists") {
		// exists modifier checks field presence — not directly supported,
		// approximate with != '' for true, = '' for false
		boolVal := toBool(value)
		if boolVal {
			return field + " != ''", nil
		}
		return field + " = ''", nil
	}

	// Determine string matching operator
	op := resolveOperator(modifiers)

	// Handle value types
	switch v := value.(type) {
	case nil:
		return field + " = ''", nil
	case string:
		// Check for boolean-like strings on boolean fields
		if isBooleanField(field) {
			lower := strings.ToLower(v)
			if lower == "false" || lower == "0" || lower == "no" {
				return field + " = false", nil
			}
			if lower == "true" || lower == "1" || lower == "yes" {
				return field + " = true", nil
			}
		}
		return convertSingleValue(field, v, op, modifiers)
	case int, int64, float64:
		return fmt.Sprintf("%s = %v", field, v), nil
	case bool:
		if v {
			return field + " = true", nil
		}
		return field + " = false", nil
	case []interface{}:
		return convertListValues(field, v, op, useAll, modifiers)
	default:
		return fmt.Sprintf("%s = '%v'", field, v), nil
	}
}

// convertSingleValue converts a single string value with the given operator.
func convertSingleValue(field, value, op string, modifiers []string) (string, error) {
	// Handle explicit modifier operators
	if hasModifier(modifiers, "contains") {
		escaped := escapeQL(value)
		return fmt.Sprintf("%s icontains '%s'", field, escaped), nil
	}
	if hasModifier(modifiers, "startswith") {
		escaped := escapeQL(value)
		return fmt.Sprintf("%s istartswith '%s'", field, escaped), nil
	}
	if hasModifier(modifiers, "endswith") {
		escaped := escapeQL(value)
		return fmt.Sprintf("%s iendswith '%s'", field, escaped), nil
	}

	// No explicit modifier — analyze wildcards in value
	return convertWildcardValue(field, value, op)
}

// convertWildcardValue analyzes SIGMA wildcard patterns and converts to optimal Fibratus operator.
func convertWildcardValue(field, value, defaultOp string) (string, error) {
	if value == "*" {
		// Wildcard-only means "any value" — skip this condition
		return "", nil
	}

	hasWildcard := strings.Contains(value, "*") || strings.Contains(value, "?")

	if !hasWildcard {
		// Exact match — use case-insensitive equals
		escaped := escapeQL(value)
		return fmt.Sprintf("%s ~= '%s'", field, escaped), nil
	}

	// Optimize common wildcard patterns
	trimmed := value

	// Pattern: *value* → icontains
	if strings.HasPrefix(trimmed, "*") && strings.HasSuffix(trimmed, "*") {
		inner := trimmed[1 : len(trimmed)-1]
		if !strings.Contains(inner, "*") && !strings.Contains(inner, "?") {
			escaped := escapeQL(inner)
			return fmt.Sprintf("%s icontains '%s'", field, escaped), nil
		}
	}

	// Pattern: *value → iendswith
	if strings.HasPrefix(trimmed, "*") && !strings.Contains(trimmed[1:], "*") && !strings.Contains(trimmed[1:], "?") {
		escaped := escapeQL(trimmed[1:])
		return fmt.Sprintf("%s iendswith '%s'", field, escaped), nil
	}

	// Pattern: value* → istartswith
	if strings.HasSuffix(trimmed, "*") && !strings.Contains(trimmed[:len(trimmed)-1], "*") && !strings.Contains(trimmed[:len(trimmed)-1], "?") {
		escaped := escapeQL(trimmed[:len(trimmed)-1])
		return fmt.Sprintf("%s istartswith '%s'", field, escaped), nil
	}

	// Complex wildcard pattern → imatches with glob
	escaped := escapeQL(value)
	return fmt.Sprintf("%s imatches '%s'", field, escaped), nil
}

// convertListValues converts a list of SIGMA values into OR/AND combined Fibratus expressions.
func convertListValues(field string, values []interface{}, op string, useAll bool, modifiers []string) (string, error) {
	var parts []string
	for _, val := range values {
		var expr string
		var err error
		switch v := val.(type) {
		case string:
			expr, err = convertSingleValue(field, v, op, modifiers)
		case int, int64, float64:
			expr = fmt.Sprintf("%s = %v", field, v)
		case nil:
			expr = fmt.Sprintf("%s = ''", field)
		default:
			expr = fmt.Sprintf("%s = '%v'", field, v)
		}
		if err != nil {
			return "", err
		}
		if expr != "" {
			parts = append(parts, expr)
		}
	}
	if len(parts) == 0 {
		return "", nil
	}
	if len(parts) == 1 {
		return parts[0], nil
	}

	joiner := " or "
	if useAll {
		joiner = " and "
	}
	return "(" + strings.Join(parts, joiner) + ")", nil
}

// convertCIDR converts a CIDR value to a cidr_contains() function call.
// Fibratus signature: cidr_contains(ip_field, 'cidr_notation')
func convertCIDR(field string, value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("cidr_contains(%s, '%s')", field, v), nil
	case []interface{}:
		var parts []string
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				continue
			}
			parts = append(parts, fmt.Sprintf("cidr_contains(%s, '%s')", field, s))
		}
		if len(parts) == 0 {
			return "", nil
		}
		return "(" + strings.Join(parts, " or ") + ")", nil
	default:
		return fmt.Sprintf("cidr_contains(%s, '%v')", field, v), nil
	}
}

// convertRegex converts a regex value to an imatches operator.
func convertRegex(field string, value interface{}, useAll bool) (string, error) {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("%s imatches '%s'", field, escapeQL(v)), nil
	case []interface{}:
		var parts []string
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s imatches '%s'", field, escapeQL(s)))
		}
		joiner := " or "
		if useAll {
			joiner = " and "
		}
		if len(parts) == 1 {
			return parts[0], nil
		}
		return "(" + strings.Join(parts, joiner) + ")", nil
	default:
		return fmt.Sprintf("%s imatches '%v'", field, v), nil
	}
}

// convertComparison converts a numeric comparison.
func convertComparison(field string, value interface{}, op string) (string, error) {
	return fmt.Sprintf("%s %s %v", field, op, value), nil
}

// buildCondition parses the SIGMA condition string and replaces selection
// names with their converted Fibratus expressions.
func buildCondition(condStr string, selectionExprs map[string]string) (string, error) {
	condStr = strings.TrimSpace(condStr)

	// Handle "1 of them" / "all of them"
	if condStr == "1 of them" || condStr == "any of them" {
		return joinAllSelections(selectionExprs, " or "), nil
	}
	if condStr == "all of them" {
		return joinAllSelections(selectionExprs, " and "), nil
	}

	// Handle "1 of <pattern>*" / "all of <pattern>*"
	oneOfPattern := regexp.MustCompile(`^1\s+of\s+(\w+)\*$`)
	if m := oneOfPattern.FindStringSubmatch(condStr); m != nil {
		return joinMatchingSelections(selectionExprs, m[1], " or "), nil
	}
	allOfPattern := regexp.MustCompile(`^all\s+of\s+(\w+)\*$`)
	if m := allOfPattern.FindStringSubmatch(condStr); m != nil {
		return joinMatchingSelections(selectionExprs, m[1], " and "), nil
	}

	// For complex conditions, do token-based substitution
	result := condStr

	// Replace "not" with Fibratus "not"
	// Replace "and" / "or" — they're the same in both languages

	// Sort selection names by length (longest first) to avoid partial replacement
	names := make([]string, 0, len(selectionExprs))
	for name := range selectionExprs {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return len(names[i]) > len(names[j])
	})

	// Handle "1 of <pattern>*" or "all of <pattern>*" within complex conditions
	oneOfInline := regexp.MustCompile(`1\s+of\s+(\w+)\*`)
	result = oneOfInline.ReplaceAllStringFunc(result, func(match string) string {
		m := oneOfInline.FindStringSubmatch(match)
		if m != nil {
			return "(" + joinMatchingSelections(selectionExprs, m[1], " or ") + ")"
		}
		return match
	})
	allOfInline := regexp.MustCompile(`all\s+of\s+(\w+)\*`)
	result = allOfInline.ReplaceAllStringFunc(result, func(match string) string {
		m := allOfInline.FindStringSubmatch(match)
		if m != nil {
			return "(" + joinMatchingSelections(selectionExprs, m[1], " and ") + ")"
		}
		return match
	})

	// Replace selection names with their expressions.
	// Handle "not <selection>" specially: wrap as "(not <expr>)" to limit
	// the scope of negation, since Fibratus QL's "not" after "and" consumes
	// the entire remaining expression via ParseExpr().
	for _, name := range names {
		expr := selectionExprs[name]
		quotedName := regexp.QuoteMeta(name)

		// First pass: replace "not <selection>" with "(true and not <expr>)".
		// We need "true and" because the QL parser doesn't support (not ...) — not
		// cannot be the first token inside parens. And we need the outer parens
		// because the parser's not handler calls ParseExpr() which consumes
		// everything until EOF or closing paren, so without the outer ), not would
		// negate the entire remaining expression instead of just this selection.
		notRe := regexp.MustCompile(`\bnot\s+` + quotedName + `\b`)
		result = notRe.ReplaceAllString(result, "(true and not "+expr+")")

		// Second pass: replace remaining (non-negated) occurrences
		re := regexp.MustCompile(`\b` + quotedName + `\b`)
		result = re.ReplaceAllString(result, expr)
	}

	return result, nil
}

// joinAllSelections joins all selection expressions with the given operator.
func joinAllSelections(exprs map[string]string, op string) string {
	keys := sortedKeys(exprs)
	var parts []string
	for _, k := range keys {
		parts = append(parts, exprs[k])
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, op) + ")"
}

// joinMatchingSelections joins selections whose names start with the given prefix.
func joinMatchingSelections(exprs map[string]string, prefix, op string) string {
	var parts []string
	for _, k := range sortedKeys(exprs) {
		if strings.HasPrefix(k, prefix) {
			parts = append(parts, exprs[k])
		}
	}
	if len(parts) == 0 {
		return "true"
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, op) + ")"
}

// ═══════════════════════════════════════════════════════════════
// Severity & Label Mapping
// ═══════════════════════════════════════════════════════════════

func mapSeverity(level string) string {
	switch strings.ToLower(level) {
	case "informational":
		return "low"
	case "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	case "critical":
		return "critical"
	default:
		return "medium"
	}
}

// MITRE ATT&CK tactic tag → (tactic ID, tactic name)
var mitreTactics = map[string][2]string{
	"attack.initial_access":     {"TA0001", "Initial Access"},
	"attack.execution":          {"TA0002", "Execution"},
	"attack.persistence":        {"TA0003", "Persistence"},
	"attack.privilege_escalation": {"TA0004", "Privilege Escalation"},
	"attack.defense_evasion":    {"TA0005", "Defense Evasion"},
	"attack.credential_access":  {"TA0006", "Credential Access"},
	"attack.discovery":          {"TA0007", "Discovery"},
	"attack.lateral_movement":   {"TA0008", "Lateral Movement"},
	"attack.collection":         {"TA0009", "Collection"},
	"attack.exfiltration":       {"TA0010", "Exfiltration"},
	"attack.command_and_control": {"TA0011", "Command and Control"},
	"attack.impact":             {"TA0040", "Impact"},
	"attack.resource_development": {"TA0042", "Resource Development"},
	"attack.reconnaissance":     {"TA0043", "Reconnaissance"},
}

// extractMITRELabels parses SIGMA tags to extract MITRE ATT&CK tactic and technique info.
func extractMITRELabels(tags []string) map[string]string {
	labels := make(map[string]string)

	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))

		// Check for tactic tags
		if info, ok := mitreTactics[tag]; ok {
			labels["tactic.id"] = info[0]
			labels["tactic.name"] = info[1]
			continue
		}

		// Check for technique tags: attack.t1234 or attack.t1234.567
		if strings.HasPrefix(tag, "attack.t") {
			techID := strings.TrimPrefix(tag, "attack.")
			techID = strings.ToUpper(techID)

			if strings.Contains(techID, ".") {
				parts := strings.SplitN(techID, ".", 2)
				labels["technique.id"] = parts[0]
				labels["subtechnique.id"] = techID
			} else {
				labels["technique.id"] = techID
			}
		}
	}

	return labels
}

// ═══════════════════════════════════════════════════════════════
// Fibratus YAML Generation
// ═══════════════════════════════════════════════════════════════

func generateFibratusYAML(rule *SigmaRule, condition, severity string, labels map[string]string) (string, error) {
	// Generate a deterministic UUID based on SIGMA rule ID if available
	ruleID := rule.ID
	if ruleID == "" {
		ruleID = uuid.New().String()
	}

	var b strings.Builder

	b.WriteString(fmt.Sprintf("name: %s\n", yamlSafe(rule.Title)))
	b.WriteString(fmt.Sprintf("id: %s\n", ruleID))
	b.WriteString("version: 1.0.0\n")

	if rule.Description != "" {
		b.WriteString(fmt.Sprintf("description: |\n  %s\n", strings.ReplaceAll(rule.Description, "\n", "\n  ")))
	}

	if len(labels) > 0 {
		b.WriteString("labels:\n")
		for _, key := range sortedKeys(labels) {
			b.WriteString(fmt.Sprintf("  %s: %s\n", key, labels[key]))
		}
	}

	if len(rule.References) > 0 {
		b.WriteString("references:\n")
		for _, ref := range rule.References {
			b.WriteString(fmt.Sprintf("  - %s\n", ref))
		}
	}

	if rule.Author != "" {
		b.WriteString(fmt.Sprintf("authors:\n  - %s\n", rule.Author))
	}

	b.WriteString(fmt.Sprintf("severity: %s\n", severity))

	// Convert SIGMA tags to Fibratus tags (excluding MITRE attack.* tags)
	var fibTags []string
	for _, tag := range rule.Tags {
		if !strings.HasPrefix(strings.ToLower(tag), "attack.") {
			fibTags = append(fibTags, tag)
		}
	}
	// Add a sigma-converted tag for tracking
	fibTags = append(fibTags, "sigma-converted")
	if len(fibTags) > 0 {
		b.WriteString("tags:\n")
		for _, tag := range fibTags {
			b.WriteString(fmt.Sprintf("  - %s\n", tag))
		}
	}

	b.WriteString(fmt.Sprintf("condition: >\n  %s\n", strings.ReplaceAll(condition, "\n", "\n  ")))

	// Generate output template
	output := generateOutputTemplate(rule, labels)
	b.WriteString(fmt.Sprintf("output: |\n  %s\n", output))

	b.WriteString("min-engine-version: 3.0.0\n")

	if len(rule.FalsePos) > 0 {
		falsePositiveNotes := strings.Join(rule.FalsePos, "; ")
		b.WriteString(fmt.Sprintf("notes: |\n  SIGMA false positives: %s\n", falsePositiveNotes))
	}

	return b.String(), nil
}

func generateOutputTemplate(rule *SigmaRule, labels map[string]string) string {
	techID := labels["technique.id"]
	if techID != "" {
		return fmt.Sprintf("[%s] %s detected on %%ps.exe (%%ps.cmdline)", techID, rule.Title)
	}
	return fmt.Sprintf("%s detected on %%ps.exe (%%ps.cmdline)", rule.Title)
}

// ═══════════════════════════════════════════════════════════════
// Helper Functions
// ═══════════════════════════════════════════════════════════════

func resolveOperator(modifiers []string) string {
	for _, mod := range modifiers {
		switch strings.ToLower(mod) {
		case "contains":
			return "icontains"
		case "startswith":
			return "istartswith"
		case "endswith":
			return "iendswith"
		case "re":
			return "imatches"
		}
	}
	return "~="
}

func hasModifier(modifiers []string, target string) bool {
	for _, m := range modifiers {
		if strings.ToLower(m) == target {
			return true
		}
	}
	return false
}

var unsupportedModifiers = map[string]bool{
	"base64":       true,
	"base64offset": true,
	"utf16":        true,
	"utf16le":      true,
	"utf16be":      true,
	"wide":         true,
	"windash":      true,
	"fieldref":     true,
	"expand":       true,
}

func hasUnsupportedModifier(modifiers []string) bool {
	for _, m := range modifiers {
		if unsupportedModifiers[strings.ToLower(m)] {
			return true
		}
	}
	return false
}

func unsupportedModifier(modifiers []string) string {
	for _, m := range modifiers {
		if unsupportedModifiers[strings.ToLower(m)] {
			return m
		}
	}
	return ""
}

// isBooleanField returns true if the Fibratus field expects a boolean value.
func isBooleanField(field string) bool {
	boolFields := map[string]bool{
		"module.signature.exists":  true,
		"module.signature.trusted": true,
		"ps.signature.exists":      true,
		"ps.signature.trusted":     true,
		"ps.is_wow64":              true,
		"ps.is_packaged":           true,
		"ps.is_protected":          true,
		"ps.token.is_elevated":     true,
		"pe.is_signed":             true,
		"pe.is_trusted":            true,
		"pe.is_dotnet":             true,
		"pe.is_dll":                true,
		"pe.is_driver":             true,
		"pe.is_exec":               true,
		"file.is_driver":           true,
		"file.is_dll":              true,
		"file.is_exec":             true,
	}
	return boolFields[field]
}

// convertHashesField handles the Sysmon composite "Hashes" field.
// SIGMA uses Hashes|contains: 'IMPHASH=xxx' to match imphashes via the
// composite field. We extract the hash type and value, mapping to the
// appropriate Fibratus field (e.g. pe.imphash for IMPHASH=).
func convertHashesField(value interface{}, modifiers []string, cfg *logsourceConfig) (string, error) {
	// Determine the imphash field for this logsource
	imphashField := ""
	if f, ok := cfg.fields["Imphash"]; ok && f != "" {
		imphashField = f
	} else if f, ok := cfg.fields["imphash"]; ok && f != "" {
		imphashField = f
	}

	// Extract hash values from the value
	values := toStringSlice(value)
	if len(values) == 0 {
		return "", fmt.Errorf("empty Hashes value")
	}

	var parts []string
	for _, v := range values {
		v = strings.TrimSpace(v)
		upper := strings.ToUpper(v)
		if strings.HasPrefix(upper, "IMPHASH=") && imphashField != "" {
			hash := v[len("IMPHASH="):]
			parts = append(parts, imphashField+" ~= '"+escapeQL(hash)+"'")
		}
		// SHA256, MD5, SHA1 — skip (no Fibratus field for loaded module hashes)
	}

	if len(parts) == 0 {
		return "", fmt.Errorf("no convertible hash types in Hashes field")
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	// Use OR if |contains| modifier (any match) or AND if |all|
	op := " or "
	if hasModifier(modifiers, "all") {
		op = " and "
	}
	return "(" + strings.Join(parts, op) + ")", nil
}

// toStringSlice converts a SIGMA value (string, []interface{}, etc.) to a []string.
func toStringSlice(value interface{}) []string {
	switch v := value.(type) {
	case string:
		return []string{v}
	case []interface{}:
		var result []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			} else if item != nil {
				result = append(result, fmt.Sprintf("%v", item))
			}
		}
		return result
	default:
		if value != nil {
			return []string{fmt.Sprintf("%v", value)}
		}
		return nil
	}
}

// escapeQL escapes a string for use in Fibratus QL single-quoted strings.
func escapeQL(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

// yamlSafe ensures a string is safe for YAML output.
func yamlSafe(s string) string {
	if strings.ContainsAny(s, ":{}[]|>&*!%#`@,") || strings.HasPrefix(s, "'") || strings.HasPrefix(s, "\"") {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	return s
}

func conditionToString(v interface{}) string {
	switch c := v.(type) {
	case string:
		return c
	case []interface{}:
		// Multiple conditions are OR'd together
		var parts []string
		for _, item := range c {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " or ")
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toBool(v interface{}) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return strings.ToLower(b) == "true" || b == "1"
	case int:
		return b != 0
	default:
		return false
	}
}

// formatCondition takes a flat condition string and formats it into a
// readable multi-line layout matching native Fibratus rule style:
//
//	spawn_process and
//	  (ps.exe iendswith '\\cmd.exe' or
//	   ps.exe iendswith '\\powershell.exe') and
//	  ps.cmdline icontains 'whoami' and
//	  not (ps.parent.exe iendswith '\\explorer.exe')
func formatCondition(cond string) string {
	// Split on top-level " and " and " or " boundaries, respecting parens
	// and string literals. Never break inside quoted strings.
	var lines []string
	depth := 0
	inString := false
	current := strings.Builder{}
	i := 0

	for i < len(cond) {
		ch := cond[i]

		// Track string literals — don't interpret anything inside quotes
		if ch == '\'' && !inString {
			inString = true
			current.WriteByte(ch)
			i++
			continue
		}
		if ch == '\'' && inString {
			// Check for escaped quote \'
			if i+1 < len(cond) && cond[i+1] == '\'' {
				current.WriteByte(ch)
				current.WriteByte(cond[i+1])
				i += 2
				continue
			}
			// Check if preceded by backslash (escaped)
			if i > 0 && cond[i-1] == '\\' {
				current.WriteByte(ch)
				i++
				continue
			}
			inString = false
			current.WriteByte(ch)
			i++
			continue
		}
		if inString {
			current.WriteByte(ch)
			i++
			continue
		}

		if ch == '(' {
			depth++
			current.WriteByte(ch)
			i++
			continue
		}
		if ch == ')' {
			depth--
			current.WriteByte(ch)
			i++
			continue
		}

		// Check for " and not " at depth <= 1 — always break here
		if depth <= 1 && i+9 <= len(cond) && cond[i:i+9] == " and not " {
			lines = append(lines, strings.TrimSpace(current.String())+" and")
			current.Reset()
			current.WriteString("not ")
			i += 9
			continue
		}

		// Check for " and " at depth <= 1
		if depth <= 1 && i+5 <= len(cond) && cond[i:i+5] == " and " {
			lines = append(lines, strings.TrimSpace(current.String())+" and")
			current.Reset()
			i += 5
			continue
		}

		current.WriteByte(ch)
		i++
	}
	if current.Len() > 0 {
		lines = append(lines, strings.TrimSpace(current.String()))
	}

	if len(lines) <= 1 {
		return cond
	}

	// Now format OR lists within each line: if a line contains " or " inside
	// parens and is long, break the OR items onto separate lines
	var formatted []string
	for _, line := range lines {
		if len(line) > 100 && strings.Contains(line, " or ") {
			formatted = append(formatted, formatOrList(line))
		} else {
			formatted = append(formatted, line)
		}
	}

	// Join with newline + indent
	return strings.Join(formatted, "\n  ")
}

// formatOrList breaks a long OR-list into multiple lines with alignment.
func formatOrList(line string) string {
	// Find the first ( that contains " or "
	depth := 0
	orStart := -1
	for i, ch := range line {
		if ch == '(' {
			depth++
			if orStart == -1 {
				// Check if this paren group contains " or "
				sub := line[i:]
				d := 0
				for j, c := range sub {
					if c == '(' {
						d++
					}
					if c == ')' {
						d--
						if d == 0 {
							inner := sub[1:j]
							if strings.Contains(inner, " or ") {
								orStart = i
							}
							break
						}
					}
				}
			}
		}
		if ch == ')' {
			depth--
		}
	}

	if orStart == -1 || len(line) < 120 {
		return line
	}

	// Find the matching closing paren for the OR group
	prefix := line[:orStart+1]
	rest := line[orStart+1:]
	depth = 1
	closeIdx := -1
	for i, ch := range rest {
		if ch == '(' {
			depth++
		}
		if ch == ')' {
			depth--
			if depth == 0 {
				closeIdx = i
				break
			}
		}
	}
	if closeIdx == -1 {
		return line
	}

	inner := rest[:closeIdx]
	suffix := rest[closeIdx:]

	// Split inner on " or " at depth 0
	var orParts []string
	d := 0
	cur := strings.Builder{}
	for j := 0; j < len(inner); j++ {
		if inner[j] == '(' {
			d++
		}
		if inner[j] == ')' {
			d--
		}
		if d == 0 && j+4 <= len(inner) && inner[j:j+4] == " or " {
			orParts = append(orParts, strings.TrimSpace(cur.String()))
			cur.Reset()
			j += 3
			continue
		}
		cur.WriteByte(inner[j])
	}
	if cur.Len() > 0 {
		orParts = append(orParts, strings.TrimSpace(cur.String()))
	}

	if len(orParts) <= 2 {
		return line // Not worth breaking
	}

	// Calculate indent: align with the content after the opening paren
	indent := strings.Repeat(" ", len(prefix))

	var b strings.Builder
	b.WriteString(prefix)
	for i, part := range orParts {
		if i == 0 {
			b.WriteString(part)
		} else {
			b.WriteString(" or\n")
			b.WriteString(indent)
			b.WriteString(part)
		}
	}
	b.WriteString(suffix)
	return b.String()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
