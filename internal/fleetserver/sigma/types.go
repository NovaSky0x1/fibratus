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

// Package sigma provides SIGMA rule parsing and conversion to Fibratus
// detection rule format. It maps SIGMA logsources, field names, modifiers,
// and detection logic into Fibratus filter query language expressions.
package sigma

// SigmaRule represents a parsed SIGMA detection rule.
type SigmaRule struct {
	Title       string            `yaml:"title"`
	ID          string            `yaml:"id"`
	Status      string            `yaml:"status"`
	Description string            `yaml:"description"`
	Author      string            `yaml:"author"`
	Date        string            `yaml:"date"`
	Modified    string            `yaml:"modified"`
	References  []string          `yaml:"references"`
	Tags        []string          `yaml:"tags"`
	Logsource   Logsource         `yaml:"logsource"`
	Detection   Detection         `yaml:"detection"`
	Level       string            `yaml:"level"`
	Fields      []string          `yaml:"fields"`
	FalsePos    []string          `yaml:"falsepositives"`
	Related     []RelatedRule     `yaml:"related"`
}

// Logsource identifies what type of log data a SIGMA rule targets.
type Logsource struct {
	Category   string `yaml:"category"`
	Product    string `yaml:"product"`
	Service    string `yaml:"service"`
	Definition string `yaml:"definition"`
}

// Detection holds the detection logic of a SIGMA rule,
// consisting of named selections/filters and a condition
// that combines them with boolean logic.
type Detection struct {
	Condition  interface{}            `yaml:"condition"`
	Selections map[string]interface{} `yaml:"-"`
}

// RelatedRule links this rule to other SIGMA rules.
type RelatedRule struct {
	ID   string `yaml:"id"`
	Type string `yaml:"type"`
}

// ConversionResult holds the output of converting a SIGMA rule to Fibratus format.
type ConversionResult struct {
	Success       bool              `json:"success"`
	FibratusYAML  string            `json:"fibratus_yaml,omitempty"`
	RuleName      string            `json:"rule_name,omitempty"`
	Condition     string            `json:"condition,omitempty"`
	Severity      string            `json:"severity,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Errors        []string          `json:"errors,omitempty"`
	Warnings      []string          `json:"warnings,omitempty"`
	Unconvertible bool              `json:"unconvertible,omitempty"`
	Reason        string            `json:"reason,omitempty"`
	SigmaID       string            `json:"sigma_id,omitempty"`
	SigmaTitle    string            `json:"sigma_title,omitempty"`
}

// BatchConversionResult holds results for a batch of SIGMA rule conversions.
type BatchConversionResult struct {
	Total       int                `json:"total"`
	Converted   int                `json:"converted"`
	Failed      int                `json:"failed"`
	Skipped     int                `json:"skipped"`
	Results     []ConversionResult `json:"results"`
}

// LogsourceMapping describes how a SIGMA logsource maps to Fibratus.
type LogsourceMapping struct {
	Category    string            `json:"category"`
	Product     string            `json:"product,omitempty"`
	Service     string            `json:"service,omitempty"`
	EventType   string            `json:"event_type"`
	Macro       string            `json:"macro,omitempty"`
	Description string            `json:"description"`
	Fields      map[string]string `json:"fields"`
}

// FieldMapping describes a single field mapping from SIGMA to Fibratus.
type FieldMapping struct {
	SigmaField    string `json:"sigma_field"`
	FibratusField string `json:"fibratus_field"`
	Category      string `json:"category"`
	Notes         string `json:"notes,omitempty"`
}
