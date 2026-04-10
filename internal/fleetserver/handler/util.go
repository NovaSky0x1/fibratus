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

package handler

import (
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/pkg/fleet"
)

// accountOrgIDs returns all org IDs for the current user's account.
// If orgID is already set in the context (single-org scope), it returns nil.
func accountOrgIDs(r *http.Request, orgs store.OrgStore) []string {
	orgID := ctxutil.OrgIDFromContext(r.Context())
	if orgID != "" {
		return nil
	}
	accountID := ctxutil.AccountIDFromContext(r.Context())
	if accountID == "" || orgs == nil {
		return nil
	}
	orgList, err := orgs.ListByAccount(r.Context(), accountID)
	if err != nil || len(orgList) == 0 {
		return nil
	}
	ids := make([]string, len(orgList))
	for i, o := range orgList {
		ids[i] = o.ID
	}
	return ids
}

// writeJSON encodes the value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, fleet.Response{
		Error: &fleet.APIError{Code: status, Message: message},
	})
}

// GenerateID produces a random hex ID for entities.
func GenerateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// decodeBody decodes JSON from the request body, handling gzip if needed.
func decodeBody(r *http.Request, v interface{}) error {
	var reader io.Reader = r.Body
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(reader)
		if err != nil {
			return err
		}
		defer gz.Close()
		reader = gz
	}
	return json.NewDecoder(reader).Decode(v)
}

// extractPathParam extracts a path segment between a prefix and suffix.
// For example, extractPathParam("/api/v1/agents/abc/heartbeat", "/api/v1/agents/", "/heartbeat")
// returns "abc".
func extractPathParam(path, prefix, suffix string) string {
	s := strings.TrimPrefix(path, prefix)
	if suffix != "" {
		s = strings.TrimSuffix(s, suffix)
	}
	// Remove any trailing/leading slashes
	s = strings.Trim(s, "/")
	// If it contains a slash, it's not a simple param
	if strings.Contains(s, "/") {
		return ""
	}
	return s
}
