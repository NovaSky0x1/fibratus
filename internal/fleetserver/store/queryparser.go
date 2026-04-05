package store

import (
	"fmt"
	"regexp"
	"strings"
)

// fieldMapping maps Fibratus QL field names to database columns.
// Fields not in this map are searched in the raw_event JSONB column.
var fieldMapping = map[string]string{
	// Event fields (evt.* is the current naming, kevt.* is deprecated but still supported)
	"evt.name":      "event_name",
	"kevt.name":     "event_name",
	"event_name":    "event_name",
	"evt.category":  "event_category",
	"kevt.category": "event_category",
	"event_category": "event_category",
	"evt.seq":       "seq",
	"kevt.seq":      "seq",

	// Process fields
	"ps.name":         "process_name",
	"process_name":    "process_name",
	"ps.exe":          "process_exe",
	"process_exe":     "process_exe",
	"ps.cmdline":      "process_cmdline",
	"process_cmdline": "process_cmdline",
	"ps.pid":          "pid",
	"pid":             "pid",
	"ps.ppid":         "parent_pid",
	"parent_pid":      "parent_pid",
	"ps.parent.name":  "parent_name",
	"parent_name":     "parent_name",

	// Agent fields
	"agent.hostname":  "agent_hostname",
	"agent_hostname":  "agent_hostname",
	"agent.id":        "agent_id",
	"agent_id":        "agent_id",

	// TID
	"thread.id":       "tid",
	"tid":             "tid",
}

// tokenType for the simple QL parser
type tokenType int

const (
	tokField tokenType = iota
	tokOp
	tokValue
	tokAnd
	tokOr
	tokNot
	tokLParen
	tokRParen
)

type token struct {
	typ tokenType
	val string
}

// ParseQueryToSQL translates a Fibratus filter QL expression into a SQL WHERE clause.
// Returns the clause (without "WHERE") and the parameter values.
// dbType should be "postgres" or "clickhouse".
func ParseQueryToSQL(query string, dbType string, startArgIdx int) (string, []interface{}, int, error) {
	if query == "" {
		return "", nil, startArgIdx, nil
	}

	tokens, err := tokenize(query)
	if err != nil {
		return "", nil, startArgIdx, err
	}

	clause, args, nextIdx, err := buildClause(tokens, dbType, startArgIdx)
	if err != nil {
		return "", nil, startArgIdx, err
	}

	return clause, args, nextIdx, nil
}

// tokenize splits a QL expression into tokens.
func tokenize(query string) ([]token, error) {
	var tokens []token
	q := strings.TrimSpace(query)
	i := 0

	for i < len(q) {
		// Skip whitespace
		for i < len(q) && (q[i] == ' ' || q[i] == '\t' || q[i] == '\n') {
			i++
		}
		if i >= len(q) {
			break
		}

		// Parentheses
		if q[i] == '(' {
			tokens = append(tokens, token{tokLParen, "("})
			i++
			continue
		}
		if q[i] == ')' {
			tokens = append(tokens, token{tokRParen, ")"})
			i++
			continue
		}

		// String literal (single or double quoted)
		if q[i] == '\'' || q[i] == '"' {
			quote := q[i]
			i++
			start := i
			for i < len(q) && q[i] != quote {
				if q[i] == '\\' {
					i++ // skip escaped char
				}
				i++
			}
			val := q[start:i]
			if i < len(q) {
				i++ // skip closing quote
			}
			tokens = append(tokens, token{tokValue, val})
			continue
		}

		// Number
		if q[i] >= '0' && q[i] <= '9' {
			start := i
			for i < len(q) && ((q[i] >= '0' && q[i] <= '9') || q[i] == '.') {
				i++
			}
			tokens = append(tokens, token{tokValue, q[start:i]})
			continue
		}

		// Operators: !=, >=, <=, ~=, =, >, <
		if q[i] == '!' && i+1 < len(q) && q[i+1] == '=' {
			tokens = append(tokens, token{tokOp, "!="})
			i += 2
			continue
		}
		if q[i] == '>' && i+1 < len(q) && q[i+1] == '=' {
			tokens = append(tokens, token{tokOp, ">="})
			i += 2
			continue
		}
		if q[i] == '<' && i+1 < len(q) && q[i+1] == '=' {
			tokens = append(tokens, token{tokOp, "<="})
			i += 2
			continue
		}
		if q[i] == '~' && i+1 < len(q) && q[i+1] == '=' {
			tokens = append(tokens, token{tokOp, "~="})
			i += 2
			continue
		}
		if q[i] == '=' {
			tokens = append(tokens, token{tokOp, "="})
			i++
			continue
		}
		if q[i] == '>' {
			tokens = append(tokens, token{tokOp, ">"})
			i++
			continue
		}
		if q[i] == '<' {
			tokens = append(tokens, token{tokOp, "<"})
			i++
			continue
		}

		// Word (field name, keyword, or operator word)
		start := i
		for i < len(q) && q[i] != ' ' && q[i] != '\t' && q[i] != '\n' &&
			q[i] != '(' && q[i] != ')' && q[i] != '=' && q[i] != '!' &&
			q[i] != '>' && q[i] != '<' && q[i] != '~' && q[i] != '\'' && q[i] != '"' {
			i++
		}
		word := q[start:i]
		lower := strings.ToLower(word)

		switch lower {
		case "and":
			tokens = append(tokens, token{tokAnd, "AND"})
		case "or":
			tokens = append(tokens, token{tokOr, "OR"})
		case "not":
			tokens = append(tokens, token{tokNot, "NOT"})
		case "in", "iin", "contains", "icontains", "startswith", "istartswith",
			"endswith", "iendswith", "matches", "imatches":
			tokens = append(tokens, token{tokOp, lower})
		case "true":
			tokens = append(tokens, token{tokValue, "true"})
		case "false":
			tokens = append(tokens, token{tokValue, "false"})
		default:
			// Check if it looks like a field name (contains dots or known prefix)
			if strings.Contains(word, ".") || fieldMapping[word] != "" {
				tokens = append(tokens, token{tokField, word})
			} else {
				tokens = append(tokens, token{tokValue, word})
			}
		}
	}

	return tokens, nil
}

// buildClause converts tokens into a SQL WHERE clause.
func buildClause(tokens []token, dbType string, argIdx int) (string, []interface{}, int, error) {
	var parts []string
	var args []interface{}
	i := 0

	for i < len(tokens) {
		t := tokens[i]

		switch t.typ {
		case tokAnd:
			parts = append(parts, "AND")
			i++
		case tokOr:
			parts = append(parts, "OR")
			i++
		case tokNot:
			parts = append(parts, "NOT")
			i++
		case tokLParen:
			parts = append(parts, "(")
			i++
		case tokRParen:
			parts = append(parts, ")")
			i++
		case tokField:
			// Expect: field op value
			if i+2 >= len(tokens) {
				return "", nil, argIdx, fmt.Errorf("incomplete expression at '%s'", t.val)
			}
			opTok := tokens[i+1]
			if opTok.typ != tokOp {
				return "", nil, argIdx, fmt.Errorf("expected operator after '%s', got '%s'", t.val, opTok.val)
			}

			field := t.val
			op := opTok.val

			// Handle IN operator with parenthesized list
			if op == "in" || op == "iin" {
				i += 2 // skip field and op
				// Collect values until closing paren
				var values []string
				if i < len(tokens) && tokens[i].typ == tokLParen {
					i++ // skip (
					for i < len(tokens) && tokens[i].typ != tokRParen {
						if tokens[i].typ == tokValue {
							values = append(values, tokens[i].val)
						}
						i++
					}
					if i < len(tokens) {
						i++ // skip )
					}
				} else if i < len(tokens) && tokens[i].typ == tokValue {
					values = append(values, tokens[i].val)
					i++
				}

				clause, newArgs, newIdx := buildInClause(field, op, values, dbType, argIdx)
				parts = append(parts, clause)
				args = append(args, newArgs...)
				argIdx = newIdx
				continue
			}

			// Simple field op value
			valTok := tokens[i+2]
			clause, newArgs, newIdx := buildComparison(field, op, valTok.val, dbType, argIdx)
			parts = append(parts, clause)
			args = append(args, newArgs...)
			argIdx = newIdx
			i += 3

		default:
			i++
		}
	}

	return strings.Join(parts, " "), args, argIdx, nil
}

func buildComparison(field, op, value, dbType string, argIdx int) (string, []interface{}, int) {
	col, isColumn := fieldMapping[field]
	placeholder := func() string {
		if dbType == "clickhouse" {
			return "?"
		}
		p := fmt.Sprintf("$%d", argIdx)
		argIdx++
		return p
	}

	if isColumn {
		switch op {
		case "=":
			ph := placeholder()
			return fmt.Sprintf("%s = %s", col, ph), []interface{}{value}, argIdx
		case "!=":
			ph := placeholder()
			return fmt.Sprintf("%s != %s", col, ph), []interface{}{value}, argIdx
		case ">", "<", ">=", "<=":
			ph := placeholder()
			return fmt.Sprintf("%s %s %s", col, op, ph), []interface{}{value}, argIdx
		case "~=", "icontains", "contains":
			ph := placeholder()
			return fmt.Sprintf("%s ILIKE %s", col, ph), []interface{}{"%" + value + "%"}, argIdx
		case "startswith", "istartswith":
			ph := placeholder()
			return fmt.Sprintf("%s ILIKE %s", col, ph), []interface{}{value + "%"}, argIdx
		case "endswith", "iendswith":
			ph := placeholder()
			return fmt.Sprintf("%s ILIKE %s", col, ph), []interface{}{"%" + value}, argIdx
		case "matches", "imatches":
			pattern := strings.ReplaceAll(value, "*", "%")
			pattern = strings.ReplaceAll(pattern, "?", "_")
			ph := placeholder()
			return fmt.Sprintf("%s ILIKE %s", col, ph), []interface{}{pattern}, argIdx
		}
	}

	// JSON field query — search in raw_event or params
	jsonPath := fieldToJSONPath(field)
	ph := placeholder()
	if dbType == "clickhouse" {
		return fmt.Sprintf("JSONExtractString(raw_event, %s) = %s", jsonPath, ph), []interface{}{value}, argIdx
	}
	// PostgreSQL JSONB
	return fmt.Sprintf("raw_event::text ILIKE %s", ph), []interface{}{"%" + value + "%"}, argIdx
}

func buildInClause(field, op string, values []string, dbType string, argIdx int) (string, []interface{}, int) {
	col, isColumn := fieldMapping[field]
	if !isColumn {
		// Fallback to raw_event search
		var orParts []string
		var args []interface{}
		for _, v := range values {
			if dbType == "clickhouse" {
				orParts = append(orParts, "raw_event LIKE ?")
			} else {
				orParts = append(orParts, fmt.Sprintf("raw_event::text ILIKE $%d", argIdx))
				argIdx++
			}
			args = append(args, "%"+v+"%")
		}
		return "(" + strings.Join(orParts, " OR ") + ")", args, argIdx
	}

	var placeholders []string
	var args []interface{}
	for _, v := range values {
		if dbType == "clickhouse" {
			placeholders = append(placeholders, "?")
		} else {
			placeholders = append(placeholders, fmt.Sprintf("$%d", argIdx))
			argIdx++
		}
		args = append(args, v)
	}

	caseFunc := ""
	if op == "iin" {
		caseFunc = "LOWER"
	}
	if caseFunc != "" {
		return fmt.Sprintf("%s(%s) IN (%s)", caseFunc, col, strings.Join(placeholders, ", ")), args, argIdx
	}
	return fmt.Sprintf("%s IN (%s)", col, strings.Join(placeholders, ", ")), args, argIdx
}

func fieldToJSONPath(field string) string {
	// Convert dotted field to JSON path segments
	parts := strings.Split(field, ".")
	var jsonParts []string
	for _, p := range parts {
		jsonParts = append(jsonParts, "'"+p+"'")
	}
	return strings.Join(jsonParts, ", ")
}

// fieldNamePattern matches valid field names for autocomplete validation
var fieldNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_.]*$`)

// ValidateFieldName checks if a field name is syntactically valid.
func ValidateFieldName(field string) bool {
	return fieldNamePattern.MatchString(field)
}
