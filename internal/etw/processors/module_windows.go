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

package processors

import (
	"github.com/rabbitstack/fibratus/pkg/event"
	"github.com/rabbitstack/fibratus/pkg/event/params"
	"github.com/rabbitstack/fibratus/pkg/ps"
	"github.com/rabbitstack/fibratus/pkg/util/hashers"
)

type moduleProcessor struct {
	psnap     ps.Snapshotter
	hashCache *hashers.FileHashCache
}

func newModuleProcessor(psnap ps.Snapshotter) Processor {
	return &moduleProcessor{
		psnap:     psnap,
		hashCache: hashers.NewFileHashCache(50000),
	}
}

func (*moduleProcessor) Name() ProcessorType { return Image }

func (m *moduleProcessor) ProcessEvent(e *event.Event) (*event.Event, bool, error) {
	if e.IsLoadImageInternal() {
		return e, false, m.psnap.AddModule(e)
	}

	if e.IsUnloadImage() {
		pid := e.Params.MustGetPid()
		addr := e.Params.TryGetAddress(params.ImageBase)
		if pid == 0 {
			pid = e.PID
		}
		return e, false, m.psnap.RemoveModule(pid, addr)
	}

	if e.IsLoadImage() || e.IsImageRundown() {
		// Compute file hashes and inject into event params before
		// the event reaches the aggregator/output pipeline.
		filePath := e.GetParamAsString(params.ImagePath)
		if filePath != "" {
			h := m.hashCache.Get(filePath)
			if h.SHA256 != "" {
				e.Params.Append(params.ImageSHA256, params.UnicodeString, h.SHA256)
			}
			if h.MD5 != "" {
				e.Params.Append(params.ImageMD5, params.UnicodeString, h.MD5)
			}
		}
		return e, false, m.psnap.AddModule(e)
	}

	return e, true, nil
}

func (m *moduleProcessor) Close() {}
