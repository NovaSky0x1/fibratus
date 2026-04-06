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

// Package validator provides server-side rule validation using the
// Fibratus rule JSON schema. This ensures invalid rules are rejected
// before being stored in the database, preventing agent crashes from
// broken rule definitions.
package validator

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

//go:embed rules.schema.json
var rulesSchemaJSON string

// ValidateRuleYAML validates a raw YAML rule against the Fibratus rule schema.
// Returns nil if valid, or a descriptive error explaining what's wrong.
func ValidateRuleYAML(ruleYAML []byte) error {
	// Parse YAML into generic structure
	var out interface{}
	if err := yaml.Unmarshal(ruleYAML, &out); err != nil {
		return fmt.Errorf("invalid YAML syntax: %w", err)
	}

	// Convert YAML map keys from interface{} to string (gojsonschema requirement)
	out = convertMapKeys(out)

	// Validate against JSON schema
	schemaLoader := gojsonschema.NewStringLoader(rulesSchemaJSON)
	docLoader := gojsonschema.NewGoLoader(out)

	result, err := gojsonschema.Validate(schemaLoader, docLoader)
	if err != nil {
		return fmt.Errorf("schema validation error: %w", err)
	}

	if !result.Valid() {
		errs := make([]string, 0, len(result.Errors()))
		for _, e := range result.Errors() {
			errs = append(errs, e.String())
		}
		return fmt.Errorf("rule validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	// Additional checks beyond schema
	var rule struct {
		Name      string `yaml:"name"`
		ID        string `yaml:"id"`
		Condition string `yaml:"condition"`
		Severity  string `yaml:"severity"`
	}
	if err := yaml.Unmarshal(ruleYAML, &rule); err != nil {
		return fmt.Errorf("rule decode error: %w", err)
	}

	if rule.Condition == "" {
		return fmt.Errorf("rule condition is empty")
	}

	// Validate condition with the real QL parser
	condResult := ValidateCondition(rule.Condition)
	if !condResult.Valid && len(condResult.Errors) > 0 {
		return fmt.Errorf("condition syntax error: %s", condResult.Errors[0].Message)
	}

	return nil
}

// ValidateRuleFields validates rule fields from a parsed Rule struct (for JSON API updates).
func ValidateRuleFields(name, condition, severity string) error {
	if name == "" {
		return fmt.Errorf("rule name is required")
	}
	if len(name) < 3 {
		return fmt.Errorf("rule name must be at least 3 characters")
	}
	if condition == "" {
		return fmt.Errorf("rule condition is required")
	}
	if len(condition) < 3 {
		return fmt.Errorf("rule condition must be at least 3 characters")
	}
	validSeverities := map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
	if severity != "" && !validSeverities[severity] {
		return fmt.Errorf("invalid severity %q — must be low, medium, high, or critical", severity)
	}
	condResult := ValidateCondition(condition)
	if !condResult.Valid && len(condResult.Errors) > 0 {
		return fmt.Errorf("condition syntax error: %s", condResult.Errors[0].Message)
	}
	return nil
}

// convertMapKeys recursively converts map[interface{}]interface{} to map[string]interface{}
// which is required by gojsonschema (YAML produces interface{} keys, JSON needs string keys).
func convertMapKeys(v interface{}) interface{} {
	switch v := v.(type) {
	case map[interface{}]interface{}:
		m := make(map[string]interface{})
		for key, val := range v {
			m[fmt.Sprintf("%v", key)] = convertMapKeys(val)
		}
		return m
	case map[string]interface{}:
		m := make(map[string]interface{})
		for key, val := range v {
			m[key] = convertMapKeys(val)
		}
		return m
	case []interface{}:
		for i, val := range v {
			v[i] = convertMapKeys(val)
		}
		return v
	default:
		return v
	}
}
