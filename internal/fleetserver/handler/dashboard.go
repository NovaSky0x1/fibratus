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
	"net/http"

	"github.com/rabbitstack/fibratus/internal/fleetserver/ctxutil"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// DashboardHandler handles dashboard aggregate API requests.
type DashboardHandler struct {
	agents     store.AgentStore
	detections store.DetectionStore
	orgs       store.OrgStore
}

// NewDashboardHandler creates a new dashboard handler.
func NewDashboardHandler(agents store.AgentStore, detections store.DetectionStore) *DashboardHandler {
	return &DashboardHandler{agents: agents, detections: detections}
}

// SetOrgStore sets the org store for cross-org aggregation.
func (h *DashboardHandler) SetOrgStore(orgs store.OrgStore) { h.orgs = orgs }

// Overview handles GET /api/v1/orgs/{org_id}/dashboard/overview
func (h *DashboardHandler) Overview(w http.ResponseWriter, r *http.Request) {
	orgID := ctxutil.OrgIDFromContext(r.Context())

	ctx := r.Context()

	// Cross-org aggregation when no org is selected
	if orgID == "" {
		orgIDs := accountOrgIDs(r, h.orgs)
		if len(orgIDs) == 0 {
			writeError(w, http.StatusBadRequest, "org or account context required")
			return
		}
		totalAgents, onlineAgents, offlineAgents, detCount := 0, 0, 0, 0
		severityBreakdown := make(map[string]int)
		for _, oid := range orgIDs {
			sc, err := h.agents.CountByStatus(ctx, oid)
			if err == nil {
				for _, v := range sc {
					totalAgents += v
				}
				onlineAgents += sc[fleet.AgentOnline]
				offlineAgents += sc[fleet.AgentOffline]
			}
			dc, err := h.detections.Count24h(ctx, oid)
			if err == nil {
				detCount += dc
			}
			sb, err := h.detections.CountBySeverity(ctx, oid)
			if err == nil {
				for k, v := range sb {
					severityBreakdown[k] += v
				}
			}
		}
		writeJSON(w, http.StatusOK, fleet.Response{Data: fleet.FleetOverview{
			TotalAgents:        totalAgents,
			OnlineAgents:       onlineAgents,
			OfflineAgents:      offlineAgents,
			TotalDetections24h: detCount,
			SeverityBreakdown:  severityBreakdown,
		}})
		return
	}

	statusCounts, err := h.agents.CountByStatus(ctx, orgID)
	if err != nil {
		log.Errorf("fleet: overview agent count error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	totalAgents := 0
	for _, v := range statusCounts {
		totalAgents += v
	}

	detCount, err := h.detections.Count24h(ctx, orgID)
	if err != nil {
		log.Errorf("fleet: overview detection count error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	severityBreakdown, err := h.detections.CountBySeverity(ctx, orgID)
	if err != nil {
		log.Errorf("fleet: overview severity error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	overview := fleet.FleetOverview{
		TotalAgents:        totalAgents,
		OnlineAgents:       statusCounts[fleet.AgentOnline],
		OfflineAgents:      statusCounts[fleet.AgentOffline],
		TotalDetections24h: detCount,
		SeverityBreakdown:  severityBreakdown,
	}

	writeJSON(w, http.StatusOK, fleet.Response{Data: overview})
}
