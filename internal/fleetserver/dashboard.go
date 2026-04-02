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

package fleetserver

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:../../web/dashboard/dist
var dashboardFS embed.FS

// dashboardHandler serves the embedded React SPA.
// It serves static files from the dist directory and falls back
// to index.html for client-side routing.
func dashboardHandler() http.Handler {
	distFS, err := fs.Sub(dashboardFS, "web/dashboard/dist")
	if err != nil {
		// Fallback: serve empty handler
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "dashboard not available", http.StatusNotFound)
		})
	}

	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't serve dashboard for API routes
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/health" {
			http.NotFound(w, r)
			return
		}

		// Try to serve the static file
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// Check if the file exists
		f, err := distFS.Open(strings.TrimPrefix(path, "/"))
		if err != nil {
			// Fall back to index.html for SPA routing
			r.URL.Path = "/"
		} else {
			f.Close()
		}

		fileServer.ServeHTTP(w, r)
	})
}
