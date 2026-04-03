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

const sealFile = ".seal"

// sealedFiles defines the fixed order of enrollment files included
// in the integrity seal. Changing this order invalidates existing seals.
// sealedFiles excludes agent-id because it is written by the fleet
// client Register() call after enrollment, not during enrollment itself.
var sealedFiles = []string{
	"org-id",
	"server-url",
	"certs/agent.crt",
	"certs/agent.key",
	"certs/ca.crt",
}
