/*
 * Copyright 2020-2021 by Nedim Sabic Sabic
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
	"errors"
	"fmt"
	"sort"
	"strings"
)

// maxArgs is the maximum number of function arguments.
const maxArgs = 1 << 5

// funcs maps function names to their descriptors.
// Only descriptors are needed for server-side validation —
// no Call implementations required.
var funcs map[string]FunctionDesc

func init() {
	funcs = map[string]FunctionDesc{
		CIDRContainsFn.String(): cidrContainsDesc(),
		MD5Fn.String():          md5Desc(),
		ConcatFn.String():       concatDesc(),
		LtrimFn.String():        ltrimDesc(),
		RtrimFn.String():        rtrimDesc(),
		LowerFn.String():        lowerDesc(),
		UpperFn.String():        upperDesc(),
		ReplaceFn.String():      replaceDesc(),
		SplitFn.String():        splitDesc(),
		LengthFn.String():       lengthDesc(),
		IndexOfFn.String():      indexOfDesc(),
		SubstrFn.String():       substrDesc(),
		EntropyFn.String():      entropyDesc(),
		RegexFn.String():        regexDesc(),
		IsMinidumpFn.String():   isMinidumpDesc(),
		BaseFn.String():         baseDesc(),
		DirFn.String():          dirDesc(),
		SymlinkFn.String():      symlinkDesc(),
		ExtFn.String():          extDesc(),
		GlobFn.String():         globDesc(),
		IsAbsFn.String():        isAbsDesc(),
		VolumeFn.String():       volumeDesc(),
		GetRegValueFn.String():  getRegValueDesc(),
		YaraFn.String():         yaraDesc(),
		ForeachFn.String():      foreachDesc(),
		CountFn.String():        countDesc(),
	}
}

// functionNames returns sorted function names for error messages.
func functionNames() []string {
	names := make([]string, 0, len(funcs))
	for _, f := range funcs {
		names = append(names, f.Name.String())
	}
	sort.Strings(names)
	return names
}

// Common argument type sets used across multiple function descriptors.
var (
	fieldLikeTypes = []ArgType{ArgField, ArgBoundField, ArgBoundSegment, ArgBareBoundVariable, ArgFunc}
	stringOrField  = []ArgType{ArgString, ArgField, ArgBoundField, ArgBoundSegment, ArgBareBoundVariable, ArgFunc}
	pathTypes      = []ArgType{ArgField, ArgBoundField, ArgBoundSegment, ArgBareBoundVariable, ArgFunc, ArgString}
	pathSliceTypes = []ArgType{ArgField, ArgBoundField, ArgBoundSegment, ArgBareBoundVariable, ArgFunc, ArgString, ArgSlice}
)

func cidrContainsDesc() FunctionDesc {
	args := []FunctionArgDesc{
		{Keyword: "ip", Types: []ArgType{ArgIP, ArgField, ArgBoundField, ArgBoundSegment, ArgBareBoundVariable}, Required: true},
		{Keyword: "cidr", Types: []ArgType{ArgString}, Required: true},
	}
	for i := 2; i < maxArgs; i++ {
		args = append(args, FunctionArgDesc{Keyword: fmt.Sprintf("cidr%d", i), Types: []ArgType{ArgString}})
	}
	return FunctionDesc{Name: CIDRContainsFn, Args: args}
}

func md5Desc() FunctionDesc {
	return FunctionDesc{
		Name: MD5Fn,
		Args: []FunctionArgDesc{
			{Keyword: "data", Types: stringOrField, Required: true},
		},
	}
}

func concatDesc() FunctionDesc {
	args := []FunctionArgDesc{
		{Keyword: "string1", Types: []ArgType{ArgString, ArgNumber, ArgField, ArgBoundField, ArgBoundSegment, ArgBareBoundVariable, ArgFunc}, Required: true},
		{Keyword: "string2", Types: []ArgType{ArgString, ArgNumber, ArgField, ArgBoundField, ArgBoundSegment, ArgBareBoundVariable, ArgFunc}, Required: true},
	}
	for i := 2; i < maxArgs; i++ {
		args = append(args, FunctionArgDesc{Keyword: fmt.Sprintf("string%d", i+1), Types: []ArgType{ArgString, ArgNumber, ArgField, ArgBoundField, ArgBoundSegment, ArgBareBoundVariable, ArgFunc}})
	}
	return FunctionDesc{Name: ConcatFn, Args: args}
}

func ltrimDesc() FunctionDesc {
	return FunctionDesc{
		Name: LtrimFn,
		Args: []FunctionArgDesc{
			{Keyword: "string", Types: stringOrField, Required: true},
			{Keyword: "prefix", Types: []ArgType{ArgString, ArgFunc}, Required: true},
		},
	}
}

func rtrimDesc() FunctionDesc {
	return FunctionDesc{
		Name: RtrimFn,
		Args: []FunctionArgDesc{
			{Keyword: "string", Types: stringOrField, Required: true},
			{Keyword: "suffix", Types: []ArgType{ArgString, ArgFunc}, Required: true},
		},
	}
}

func lowerDesc() FunctionDesc {
	return FunctionDesc{
		Name: LowerFn,
		Args: []FunctionArgDesc{
			{Keyword: "string", Types: stringOrField, Required: true},
		},
	}
}

func upperDesc() FunctionDesc {
	return FunctionDesc{
		Name: UpperFn,
		Args: []FunctionArgDesc{
			{Keyword: "string", Types: stringOrField, Required: true},
		},
	}
}

func replaceDesc() FunctionDesc {
	args := []FunctionArgDesc{
		{Keyword: "string", Types: stringOrField, Required: true},
		{Keyword: "old", Types: stringOrField, Required: true},
		{Keyword: "new", Types: stringOrField, Required: true},
	}
	for i := 3; i < maxArgs; i++ {
		args = append(args, FunctionArgDesc{Keyword: fmt.Sprintf("arg%d", i+1), Types: stringOrField})
	}
	return FunctionDesc{
		Name: ReplaceFn,
		Args: args,
		ArgsValidationFunc: func(args []string) error {
			if len(args) == 3 {
				return nil
			}
			if (len(args)-1)%2 != 0 {
				return errors.New("old/new replacements mismatch")
			}
			return nil
		},
	}
}

func splitDesc() FunctionDesc {
	return FunctionDesc{
		Name: SplitFn,
		Args: []FunctionArgDesc{
			{Keyword: "string", Types: stringOrField, Required: true},
			{Keyword: "sep", Types: []ArgType{ArgString}, Required: true},
		},
	}
}

func lengthDesc() FunctionDesc {
	return FunctionDesc{
		Name: LengthFn,
		Args: []FunctionArgDesc{
			{Keyword: "string|slice", Types: []ArgType{ArgField, ArgBoundField, ArgBoundSegment, ArgBareBoundVariable, ArgSlice, ArgFunc}, Required: true},
		},
	}
}

func indexOfDesc() FunctionDesc {
	return FunctionDesc{
		Name: IndexOfFn,
		Args: []FunctionArgDesc{
			{Keyword: "string", Types: fieldLikeTypes, Required: true},
			{Keyword: "substr", Types: []ArgType{ArgString, ArgFunc}, Required: true},
			{Keyword: "index", Types: []ArgType{ArgString}},
		},
		ArgsValidationFunc: func(args []string) error {
			if len(args) == 2 {
				return nil
			}
			if len(args) == 3 {
				valid := map[string]bool{"first": true, "any": true, "last": true, "lastany": true}
				if !valid[strings.ToLower(args[2])] {
					return fmt.Errorf("%s is not a valid index search order. Available options are: first,any,last,lastany", args[2])
				}
			}
			return nil
		},
	}
}

func substrDesc() FunctionDesc {
	return FunctionDesc{
		Name: SubstrFn,
		Args: []FunctionArgDesc{
			{Keyword: "string", Types: []ArgType{ArgFunc, ArgField, ArgBoundField, ArgBoundSegment, ArgBareBoundVariable}, Required: true},
			{Keyword: "start", Types: []ArgType{ArgFunc, ArgNumber}, Required: true},
			{Keyword: "end", Types: []ArgType{ArgFunc, ArgNumber}},
		},
	}
}

func entropyDesc() FunctionDesc {
	return FunctionDesc{
		Name: EntropyFn,
		Args: []FunctionArgDesc{
			{Keyword: "string", Types: fieldLikeTypes, Required: true},
			{Keyword: "algo", Types: []ArgType{ArgString}},
		},
		ArgsValidationFunc: func(args []string) error {
			if len(args) == 1 {
				return nil
			}
			if len(args) > 1 && strings.ToLower(args[1]) != "shannon" {
				return fmt.Errorf("unsupported entropy algorithm: %s. Available algorithms: shannon", args[1])
			}
			return nil
		},
	}
}

func regexDesc() FunctionDesc {
	args := []FunctionArgDesc{
		{Keyword: "string", Types: []ArgType{ArgField, ArgBoundField, ArgString, ArgBoundSegment, ArgBareBoundVariable, ArgFunc}, Required: true},
		{Keyword: "regexp", Types: []ArgType{ArgString}, Required: true},
	}
	for i := 2; i < maxArgs; i++ {
		args = append(args, FunctionArgDesc{Keyword: fmt.Sprintf("regexp%d", i), Types: []ArgType{ArgString}})
	}
	return FunctionDesc{Name: RegexFn, Args: args}
}

func isMinidumpDesc() FunctionDesc {
	return FunctionDesc{
		Name: IsMinidumpFn,
		Args: []FunctionArgDesc{
			{Keyword: "path", Types: stringOrField, Required: true},
		},
	}
}

func baseDesc() FunctionDesc {
	return FunctionDesc{
		Name: BaseFn,
		Args: []FunctionArgDesc{
			{Keyword: "path", Types: pathSliceTypes, Required: true},
			{Keyword: "ext", Types: []ArgType{ArgBool}},
		},
	}
}

func dirDesc() FunctionDesc {
	return FunctionDesc{
		Name: DirFn,
		Args: []FunctionArgDesc{
			{Keyword: "path", Types: pathSliceTypes, Required: true},
		},
	}
}

func symlinkDesc() FunctionDesc {
	return FunctionDesc{
		Name: SymlinkFn,
		Args: []FunctionArgDesc{
			{Keyword: "path", Types: pathTypes, Required: true},
		},
	}
}

func extDesc() FunctionDesc {
	return FunctionDesc{
		Name: ExtFn,
		Args: []FunctionArgDesc{
			{Keyword: "path", Types: []ArgType{ArgField, ArgBoundField, ArgFunc, ArgBoundSegment, ArgBareBoundVariable, ArgString}, Required: true},
			{Keyword: "dot", Types: []ArgType{ArgBool}},
		},
	}
}

func globDesc() FunctionDesc {
	return FunctionDesc{
		Name: GlobFn,
		Args: []FunctionArgDesc{
			{Keyword: "pattern", Types: []ArgType{ArgField, ArgBoundField, ArgFunc, ArgBoundSegment, ArgBareBoundVariable, ArgString}, Required: true},
		},
	}
}

func isAbsDesc() FunctionDesc {
	return FunctionDesc{
		Name: IsAbsFn,
		Args: []FunctionArgDesc{
			{Keyword: "path", Types: []ArgType{ArgField, ArgBoundField, ArgFunc, ArgBoundSegment, ArgBareBoundVariable, ArgString}, Required: true},
		},
	}
}

func volumeDesc() FunctionDesc {
	return FunctionDesc{
		Name: VolumeFn,
		Args: []FunctionArgDesc{
			{Keyword: "path", Types: pathTypes, Required: true},
		},
	}
}

func getRegValueDesc() FunctionDesc {
	return FunctionDesc{
		Name: GetRegValueFn,
		Args: []FunctionArgDesc{
			{Keyword: "path", Types: []ArgType{ArgField, ArgBoundField, ArgString, ArgBoundSegment, ArgBareBoundVariable, ArgFunc}, Required: true},
		},
	}
}

func yaraDesc() FunctionDesc {
	return FunctionDesc{
		Name: YaraFn,
		Args: []FunctionArgDesc{
			{Keyword: "pid|file|bytes", Types: []ArgType{ArgField, ArgBoundField, ArgBoundSegment, ArgBareBoundVariable, ArgFunc, ArgString, ArgNumber}, Required: true},
			{Keyword: "rules", Types: []ArgType{ArgField, ArgBoundField, ArgFunc, ArgString}, Required: true},
			{Keyword: "vars", Types: []ArgType{ArgField, ArgBoundField, ArgFunc, ArgString}},
		},
	}
}

func foreachDesc() FunctionDesc {
	args := []FunctionArgDesc{
		{Keyword: "iterable", Types: []ArgType{ArgField}, Required: true},
		{Keyword: "var", Types: []ArgType{ArgBareBoundVariable}, Required: true},
		{Keyword: "predicate", Types: []ArgType{ArgExpression}, Required: true},
	}
	// append optional capture fields
	for i := 3; i < 13; i++ {
		args = append(args, FunctionArgDesc{Keyword: fmt.Sprintf("capture%d", i-2), Types: []ArgType{ArgField}})
	}
	return FunctionDesc{Name: ForeachFn, Args: args}
}

func countDesc() FunctionDesc {
	return FunctionDesc{
		Name: CountFn,
		Args: []FunctionArgDesc{
			{Keyword: "string|slice", Types: pathSliceTypes, Required: true},
			{Keyword: "pattern", Types: []ArgType{ArgString}, Required: true},
			{Keyword: "case_insensitive", Types: []ArgType{ArgBool}},
		},
	}
}
