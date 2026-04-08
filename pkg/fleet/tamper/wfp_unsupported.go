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

//go:build !windows

package tamper

// WFPIsolator is a no-op on non-Windows platforms.
type WFPIsolator struct{}

func NewWFPIsolator() *WFPIsolator                          { return &WFPIsolator{} }
func (w *WFPIsolator) Isolate(serverIP string, wl []string) error { return nil }
func (w *WFPIsolator) Unisolate() error                     { return nil }
func (w *WFPIsolator) IsIsolated() bool                     { return false }
func (w *WFPIsolator) Cleanup() error                       { return nil }
