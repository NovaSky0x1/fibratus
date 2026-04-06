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
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Field represents the type alias for the field.
type Field string

// Fn is the type alias for function definitions.
type Fn uint16

const (
	CIDRContainsFn Fn = iota + 1
	MD5Fn
	ConcatFn
	LtrimFn
	RtrimFn
	LowerFn
	UpperFn
	ReplaceFn
	SplitFn
	LengthFn
	IndexOfFn
	SubstrFn
	EntropyFn
	RegexFn
	IsMinidumpFn
	BaseFn
	DirFn
	SymlinkFn
	ExtFn
	GlobFn
	IsAbsFn
	VolumeFn
	GetRegValueFn
	YaraFn
	ForeachFn
	CountFn
)

// String returns the function name in upper case.
func (f Fn) String() string {
	switch f {
	case CIDRContainsFn:
		return "CIDR_CONTAINS"
	case MD5Fn:
		return "MD5"
	case ConcatFn:
		return "CONCAT"
	case LtrimFn:
		return "LTRIM"
	case RtrimFn:
		return "RTRIM"
	case LowerFn:
		return "LOWER"
	case UpperFn:
		return "UPPER"
	case ReplaceFn:
		return "REPLACE"
	case SplitFn:
		return "SPLIT"
	case LengthFn:
		return "LENGTH"
	case IndexOfFn:
		return "INDEXOF"
	case SubstrFn:
		return "SUBSTR"
	case EntropyFn:
		return "ENTROPY"
	case RegexFn:
		return "REGEX"
	case IsMinidumpFn:
		return "IS_MINIDUMP"
	case BaseFn:
		return "BASE"
	case DirFn:
		return "DIR"
	case SymlinkFn:
		return "SYMLINK"
	case ExtFn:
		return "EXT"
	case GlobFn:
		return "GLOB"
	case IsAbsFn:
		return "IS_ABS"
	case VolumeFn:
		return "VOLUME"
	case GetRegValueFn:
		return "GET_REG_VALUE"
	case YaraFn:
		return "YARA"
	case ForeachFn:
		return "FOREACH"
	case CountFn:
		return "COUNT"
	default:
		return "UNDEFINED"
	}
}

// ArgType is the type alias for the argument value type.
type ArgType uint8

// ArgsValidation is a function for the custom argument validation logic.
type ArgsValidation func(args []string) error

const (
	ArgString ArgType = iota
	ArgNumber
	ArgIP
	ArgField
	ArgFunc
	ArgSlice
	ArgBool
	ArgExpression
	ArgBoundField
	ArgBoundSegment
	ArgBareBoundVariable
	ArgUnknown
)

// String returns the argument type as a string value.
func (typ ArgType) String() string {
	switch typ {
	case ArgString:
		return "string"
	case ArgNumber:
		return "number"
	case ArgIP:
		return "ip"
	case ArgField:
		return "field"
	case ArgFunc:
		return "func"
	case ArgSlice:
		return "slice"
	case ArgBool:
		return "bool"
	case ArgExpression:
		return "expression"
	case ArgBoundField:
		return "boundfield"
	case ArgBoundSegment:
		return "boundsegment"
	case ArgBareBoundVariable:
		return "bareboundvar"
	}
	return "unknown"
}

// FunctionDesc contains the function signature that
// a particular filter function has to satisfy.
type FunctionDesc struct {
	Name               Fn
	Args               []FunctionArgDesc
	ArgsValidationFunc ArgsValidation
}

// RequiredArgs returns the number of required function args.
func (f FunctionDesc) RequiredArgs() int {
	var nargs int
	for _, arg := range f.Args {
		if arg.Required {
			nargs++
		}
	}
	return nargs
}

// FunctionArgDesc describes each function argument.
type FunctionArgDesc struct {
	Keyword  string
	Required bool
	Types    []ArgType
}

// ContainsType returns true if the argument satisfies the given argument type.
func (arg FunctionArgDesc) ContainsType(typ ArgType) bool {
	for _, t := range arg.Types {
		if t == typ {
			return true
		}
	}
	return false
}

// EventType is a stub for event type identifiers (replaces event.Type).
type EventType [18]byte

// EventCategory is a stub for event categories (replaces event.Category).
type EventCategory string

// EventSource is a stub for event source provenance (replaces event.Source).
type EventSource uint8

// BitSetType designates the type of bitset.
type BitSetType uint8

const (
	BitmaskBitSet  BitSetType = iota + 1
	TypeBitSet
	CategoryBitSet
)

// BitSets is a stub for event bitsets. On the server side, event type
// resolution is not available, so these operations are no-ops.
type BitSets struct{}

func (b *BitSets) SetBit(_ BitSetType, _ EventType)  {}
func (b *BitSets) SetCategoryBit(_ EventCategory)     {}

var (
	// ErrArgumentTypeMismatch signals an invalid argument type
	ErrArgumentTypeMismatch = func(i int, keyword string, fn Fn, types []ArgType) error {
		argTypes := make([]string, len(types))
		for i, typ := range types {
			argTypes[i] = typ.String()
		}
		return fmt.Errorf("argument #%d (%s) in function %s should be one of: %v", i+1, keyword, fn, strings.Join(argTypes, "|"))
	}
	// ErrUndefinedFunction is thrown when an unknown function is supplied
	ErrUndefinedFunction = func(name string) error {
		return fmt.Errorf("%s function is undefined. Did you mean one of %s%s", name, strings.Join(functionNames(), "|"), "?")
	}
	// ErrFunctionSignature is thrown when the function signature is not satisfied
	ErrFunctionSignature = func(desc FunctionDesc, givenArguments int) error {
		return fmt.Errorf("%s function requires %d argument(s) but %d argument(s) given", desc.Name, desc.RequiredArgs(), givenArguments)
	}
)

// StringLiteral represents a string literal.
type StringLiteral struct {
	Value string
}

// FieldLiteral represents a field literal.
type FieldLiteral struct {
	Value string
	Field Field
	Arg   string
}

// IntegerLiteral represents a signed number literal.
type IntegerLiteral struct {
	Value int64
}

// UnsignedLiteral represents an unsigned number literal.
type UnsignedLiteral struct {
	Value uint64
}

// DecimalLiteral represents a floating point number literal.
type DecimalLiteral struct {
	Value float64
}

// BoolLiteral represents the logical true/false literal.
type BoolLiteral struct {
	Value bool
}

// IPLiteral represents an IP literal.
type IPLiteral struct {
	Value net.IP
}

// BoundFieldLiteral represents the bound field literal.
type BoundFieldLiteral struct {
	Value    string
	BoundVar BareBoundVariableLiteral
	Field    *FieldLiteral
}

// BoundSegmentLiteral represents the bound segment literal.
type BoundSegmentLiteral struct {
	Value    string
	BoundVar BareBoundVariableLiteral
	Segment  Segment
}

// BareBoundVariableLiteral represents a bare bound variable reference.
type BareBoundVariableLiteral struct {
	Value string
}

func (i IPLiteral) String() string {
	return i.Value.String()
}

func (i IntegerLiteral) String() string {
	return strconv.Itoa(int(i.Value))
}

func (s StringLiteral) String() string {
	return s.Value
}

func (f *FieldLiteral) String() string {
	if f.Arg != "" {
		var b strings.Builder
		b.Grow(len(f.Value) + len(f.Arg) + 2)
		b.WriteString(f.Value)
		b.WriteByte('[')
		b.WriteString(f.Arg)
		b.WriteByte(']')
		return b.String()
	}
	return f.Value
}

func (u UnsignedLiteral) String() string {
	return strconv.Itoa(int(u.Value))
}

func (d DecimalLiteral) String() string {
	return strconv.FormatFloat(d.Value, 'e', -1, 64)
}

func (b BoolLiteral) String() string {
	return strconv.FormatBool(b.Value)
}

func (b BoundFieldLiteral) String() string {
	return b.Value
}

func (b BoundSegmentLiteral) String() string {
	return b.Value
}

func (b BareBoundVariableLiteral) String() string {
	return b.Value
}

// ListLiteral represents a list of tag key literals.
type ListLiteral struct {
	Values []string
}

// String returns a string representation of the literal.
func (s *ListLiteral) String() string {
	var n int
	for _, elem := range s.Values {
		n += len(elem) + 2
	}

	var b strings.Builder
	b.Grow(n + 2)
	b.WriteRune('(')

	for idx, elem := range s.Values {
		if idx != 0 {
			b.WriteString(", ")
		}
		b.WriteString(elem)
	}

	b.WriteRune(')')

	return b.String()
}

// Function represents a function call.
type Function struct {
	Name string
	Args []Expr
}

// ArgsSlice returns arguments as a slice of strings.
func (f *Function) ArgsSlice() []string {
	args := make([]string, 0, len(f.Args))
	for _, arg := range f.Args {
		args = append(args, arg.String())
	}
	return args
}

// String returns a string representation of the call.
func (f *Function) String() string {
	args := strings.Join(f.ArgsSlice(), ", ")

	var b strings.Builder
	b.Grow(len(args) + len(f.Name) + 2)

	b.WriteString(f.Name)
	b.WriteRune('(')
	b.WriteString(args)
	b.WriteRune(')')

	// Write function name and args.
	return b.String()
}

func (f *Function) IsForeach() bool {
	return f.Name == "foreach" || f.Name == "FOREACH"
}

func (f *Function) IsBinaryExprArg(i int) bool {
	_, ok := f.Args[i].(*BinaryExpr)
	return ok
}

func (f *Function) IsNotExprArg(i int) bool {
	_, ok := f.Args[i].(*NotExpr)
	return ok
}

func (f *Function) IsBareBoundVariableArg(i int) bool {
	_, ok := f.Args[i].(*BareBoundVariableLiteral)
	return ok
}

func (f *Function) IsFieldArg(i int) bool {
	_, ok := f.Args[i].(*FieldLiteral)
	return ok
}

// validate ensures that the function name obtained
// from the parser exists within the internal functions
// catalog. It also validates the function signature to
// make sure required arguments are supplied. Finally, it
// checks the type of each argument with the expected one.
func (f *Function) validate() error {
	desc, ok := funcs[strings.ToUpper(f.Name)]
	if !ok {
		return ErrUndefinedFunction(f.Name)
	}

	if len(f.Args) < desc.RequiredArgs() ||
		len(f.Args) > len(desc.Args) {
		return ErrFunctionSignature(desc, len(f.Args))
	}

	if desc.ArgsValidationFunc != nil {
		if err := desc.ArgsValidationFunc(f.ArgsSlice()); err != nil {
			return err
		}
	}

	for i, expr := range f.Args {
		arg := desc.Args[i]
		typ := ArgUnknown

		switch reflect.TypeOf(expr) {
		case reflect.TypeOf(&FieldLiteral{}):
			typ = ArgField
		case reflect.TypeOf(&BoundFieldLiteral{}):
			typ = ArgBoundField
		case reflect.TypeOf(&BoundSegmentLiteral{}):
			typ = ArgBoundSegment
		case reflect.TypeOf(&BareBoundVariableLiteral{}):
			typ = ArgBareBoundVariable
		case reflect.TypeOf(&IPLiteral{}):
			typ = ArgIP
		case reflect.TypeOf(&StringLiteral{}):
			typ = ArgString
		case reflect.TypeOf(&IntegerLiteral{}):
			typ = ArgNumber
		case reflect.TypeOf(&Function{}):
			typ = ArgFunc
		case reflect.TypeOf(&ListLiteral{}):
			typ = ArgSlice
		case reflect.TypeOf(&BoolLiteral{}):
			typ = ArgBool
		case reflect.TypeOf(&BinaryExpr{}), reflect.TypeOf(&ParenExpr{}), reflect.TypeOf(&NotExpr{}):
			typ = ArgExpression
		}

		if !arg.ContainsType(typ) {
			return ErrArgumentTypeMismatch(i, arg.Keyword, desc.Name, arg.Types)
		}
	}

	return nil
}

// SequenceExpr represents a single binary expression within the sequence.
type SequenceExpr struct {
	Expr Expr
	// By contains the expression link if the sequence is constrained.
	By *SequenceLink
	// BoundFields is a group of bound fields referenced in the sequence expression.
	BoundFields []*BoundFieldLiteral
	// Alias represents the sequence expression alias when bound fields are used.
	Alias string

	bitsets BitSets
	types   []EventType
}

func (e *SequenceExpr) init() {
	e.types = make([]EventType, 0)
	e.BoundFields = make([]*BoundFieldLiteral, 0)
}

func (e *SequenceExpr) walk() {
	stringFields := make(map[Field][]string)
	walk := func(n Node) {
		if expr, ok := n.(*BinaryExpr); ok {
			switch lhs := expr.LHS.(type) {
			case *BoundFieldLiteral:
				e.BoundFields = append(e.BoundFields, lhs)
			case *FieldLiteral:
				field := Field(lhs.Value)
				switch v := expr.RHS.(type) {
				case *StringLiteral:
					stringFields[field] = append(stringFields[field], v.Value)
				case *ListLiteral:
					stringFields[field] = append(stringFields[field], v.Values...)
				}
			}

			switch rhs := expr.RHS.(type) {
			case *BoundFieldLiteral:
				e.BoundFields = append(e.BoundFields, rhs)
			case *FieldLiteral:
				field := Field(rhs.Value)
				switch v := expr.LHS.(type) {
				case *StringLiteral:
					stringFields[field] = append(stringFields[field], v.Value)
				case *ListLiteral:
					stringFields[field] = append(stringFields[field], v.Values...)
				}
			}
		}

		if expr, ok := n.(*Function); ok {
			for _, arg := range expr.Args {
				switch v := arg.(type) {
				case *FieldLiteral:
					field := Field(v.Value)
					stringFields[field] = append(stringFields[field], v.Value)
				case *BoundFieldLiteral:
					e.BoundFields = append(e.BoundFields, v)
				}
			}
		}
	}

	WalkFunc(e.Expr, walk)

	// On the server side, event type resolution (NameToTypes, TypeToEventInfo)
	// is not available since those are Windows-specific constructs. We still
	// walk string fields to collect bound fields but skip the bitset
	// population that depends on the event type registry.
	for name, values := range stringFields {
		for _, v := range values {
			switch name {
			case EvtName:
				// Server-side: skip event type resolution.
				// On Windows, this would call event.NameToTypes(v)
				// and populate e.types and category bitsets.
				_ = v
			case EvtCategory:
				e.bitsets.SetCategoryBit(EventCategory(v))
			}
		}
	}
}

// HasBoundFields determines if this sequence expression references any bound field.
func (e *SequenceExpr) HasBoundFields() bool {
	return len(e.BoundFields) > 0
}

// SequenceLink represents a single or
// a collection of fields that are used to
// build the sequence join link.
type SequenceLink struct {
	Fields []*FieldLiteral
}

// IsCompound indicates if the sequence expression
// uses multiple fields for the join link.
func (l *SequenceLink) IsCompound() bool {
	return len(l.Fields) > 1
}

// First returns the first field if the link is not compound.
func (l *SequenceLink) First() string {
	if len(l.Fields) == 1 {
		return l.Fields[0].Value
	}
	return ""
}

// Sequence is a collection of two or more sequence expressions.
type Sequence struct {
	MaxSpan     time.Duration
	By          *SequenceLink
	Expressions []SequenceExpr
	IsUnordered bool
}

// IsConstrained determines if the sequence has the global or per-expression `BY` statement.
func (s Sequence) IsConstrained() bool {
	return s.By != nil || s.Expressions[0].By != nil
}

func (s *Sequence) init() {
	// On the server side, event source resolution is not available.
	// The IsUnordered flag defaults to false since we cannot determine
	// event source provenance without the Windows event type registry.
}

func (s Sequence) impairBy() bool {
	b := make(map[bool]int, len(s.Expressions))
	for _, expr := range s.Expressions {
		b[expr.By != nil]++
	}
	if s.By != nil && (b[true] == len(s.Expressions) || b[false] == len(s.Expressions)) {
		return false
	}
	return b[true] > 0 && b[false] > 0
}

// incompatibleConstraints checks if the sequence has
// both global and per-expression `BY` statements and
// returns true if such condition is satisfied.
func (s Sequence) incompatibleConstraints() bool {
	for _, expr := range s.Expressions {
		if expr.By != nil && s.By != nil {
			return true
		}
	}
	return false
}
