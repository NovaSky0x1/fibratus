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

package yara

import (
	"github.com/rabbitstack/fibratus/pkg/event"
)

// Scanner watches for certain events such as process creation or image loading and
// triggers the scanning either on the process memory or on-disk file. If matches occur,
// an alert is emitted via registered alert senders.
type Scanner interface {
	event.Listener
	// Scan runs a scan when the specified signal is observed. The signal
	// can be the creation of a new process, image loading, writing the PE
	// file or ADS to the file system, or a suspicious memory allocation.
	Scan(*event.Event) (bool, error)
	// ScanTarget runs an on-demand scan without going through the ETW event
	// pipeline. target must be a uint32 (process PID), string (file path on
	// disk), or []byte (raw memory buffer). The concrete return type is
	// go-yara v4 MatchRules under the `yara` build tag; callers should
	// JSON-marshal the result directly (go-yara MatchRules is JSON-friendly).
	// Intended for the fleet active-response "YARA Scan" command.
	ScanTarget(target any) (any, error)
	// Close disposes any resources allocated by the scanner.
	Close()
}
