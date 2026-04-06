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
	"strings"

	"github.com/rabbitstack/fibratus/internal/fleetserver/qlparser"
)

// ConditionValidationResult contains the outcome of condition validation.
type ConditionValidationResult struct {
	Valid      bool             `json:"valid"`
	Errors     []ConditionError `json:"errors,omitempty"`
	Warnings   []string         `json:"warnings,omitempty"`
	IsSequence bool             `json:"is_sequence"`
}

// ConditionError describes a specific validation error.
type ConditionError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ValidateCondition validates a filter condition string using
// the real Fibratus QL parser. This catches syntax errors,
// unknown fields, unknown functions, invalid operators, bad
// escape sequences, and malformed sequences.
func ValidateCondition(condition string) *ConditionValidationResult {
	return ValidateConditionWithMacros(condition, nil)
}

// ValidateConditionWithMacros validates a condition with macro support.
func ValidateConditionWithMacros(condition string, macros map[string]*qlparser.Macro) *ConditionValidationResult {
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

	var config *qlparser.MacroConfig
	if macros != nil {
		config = qlparser.NewMacroConfig(macros)
	}

	p := qlparser.NewParserWithConfig(condition, config)

	if p.IsSequence() {
		result.IsSequence = true
		seq, err := p.ParseSequence()
		if err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, ConditionError{
				Type:    "syntax",
				Message: err.Error(),
			})
			return result
		}
		if len(seq.Expressions) < 2 {
			result.Valid = false
			result.Errors = append(result.Errors, ConditionError{
				Type:    "sequence",
				Message: "sequences require at least two expressions",
			})
			return result
		}
		return result
	}

	_, err := p.ParseExpr()
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ConditionError{
			Type:    "syntax",
			Message: err.Error(),
		})
		return result
	}

	return result
}
