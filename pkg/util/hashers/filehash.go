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

package hashers

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"sync"
)

// FileHash holds the SHA256 and MD5 hex-encoded digests for a file.
type FileHash struct {
	SHA256 string
	MD5    string
}

// maxFileSize is the maximum file size we'll hash (256 MB).
// Files larger than this are skipped to avoid blocking event processing.
const maxFileSize = 256 << 20

// ComputeFileHash reads a file once and computes both SHA256 and MD5
// simultaneously using an io.MultiWriter.
func ComputeFileHash(path string) (FileHash, error) {
	f, err := os.Open(path)
	if err != nil {
		return FileHash{}, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return FileHash{}, err
	}
	if stat.Size() > maxFileSize || stat.IsDir() {
		return FileHash{}, nil
	}

	sha := sha256.New()
	m := md5.New()
	if _, err := io.Copy(io.MultiWriter(sha, m), f); err != nil {
		return FileHash{}, err
	}

	return FileHash{
		SHA256: hex.EncodeToString(sha.Sum(nil)),
		MD5:    hex.EncodeToString(m.Sum(nil)),
	}, nil
}

// FileHashCache is a thread-safe cache for file hashes, keyed by
// normalized file path. Common system DLLs are loaded by every process,
// so caching avoids redundant disk I/O.
type FileHashCache struct {
	mu      sync.RWMutex
	entries map[string]FileHash
	maxSize int
}

// NewFileHashCache creates a hash cache with the given maximum entry count.
// When full, the oldest entries are not evicted (simple bounded map) —
// this is acceptable because the working set of files on a system is
// relatively stable during a session.
func NewFileHashCache(maxSize int) *FileHashCache {
	if maxSize <= 0 {
		maxSize = 50000
	}
	return &FileHashCache{
		entries: make(map[string]FileHash, 4096),
		maxSize: maxSize,
	}
}

// Get returns the cached hash for a file path, or computes and caches it.
func (c *FileHashCache) Get(path string) FileHash {
	key := normalizeKey(path)

	c.mu.RLock()
	h, ok := c.entries[key]
	c.mu.RUnlock()
	if ok {
		return h
	}

	h, err := ComputeFileHash(path)
	if err != nil {
		return FileHash{}
	}

	c.mu.Lock()
	if len(c.entries) < c.maxSize {
		c.entries[key] = h
	}
	c.mu.Unlock()

	return h
}

func normalizeKey(path string) string {
	return strings.ToLower(path)
}
