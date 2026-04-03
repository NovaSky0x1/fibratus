//go:build !windows

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

package tamper

import "errors"

// GenerateSeal is not supported on non-Windows platforms.
func GenerateSeal(dataDir string) error {
	return errors.New("seal generation is only supported on Windows")
}

// VerifySeal is not supported on non-Windows platforms.
func VerifySeal(dataDir string) error {
	return errors.New("seal verification is only supported on Windows")
}
