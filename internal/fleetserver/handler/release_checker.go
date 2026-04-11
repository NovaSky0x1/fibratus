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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	log "github.com/sirupsen/logrus"
)

// ReleaseChecker periodically polls a GitHub repository for new releases
// and updates account settings with the latest version and MSI URL.
type ReleaseChecker struct {
	accounts store.AccountStore
	client   *http.Client
	interval time.Duration
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// NewReleaseChecker creates a release checker that polls every interval.
func NewReleaseChecker(accounts store.AccountStore, interval time.Duration) *ReleaseChecker {
	return &ReleaseChecker{
		accounts: accounts,
		client:   &http.Client{Timeout: 15 * time.Second},
		interval: interval,
	}
}

// Start begins the background polling loop.
func (rc *ReleaseChecker) Start(ctx context.Context) {
	go rc.loop(ctx)
}

func (rc *ReleaseChecker) loop(ctx context.Context) {
	// Check immediately on startup
	rc.checkAll(ctx)

	ticker := time.NewTicker(rc.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rc.checkAll(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (rc *ReleaseChecker) checkAll(ctx context.Context) {
	accounts, err := rc.accounts.ListAll(ctx)
	if err != nil {
		return
	}
	// Group by repo to avoid duplicate API calls
	repoAccounts := make(map[string][]string) // repo -> []accountID
	repoVersions := make(map[string]string)   // repo -> current latest version per any account
	for _, acct := range accounts {
		repo := acct.AgentUpdateRepo
		if repo == "" {
			repo = "NovaSky0x1/fibratus"
		}
		repoAccounts[repo] = append(repoAccounts[repo], acct.ID)
		if acct.LatestAgentVersion != "" {
			repoVersions[repo] = acct.LatestAgentVersion
		}
	}

	for repo, accountIDs := range repoAccounts {
		version, msiURL, err := rc.fetchLatestRelease(repo)
		if err != nil {
			log.Debugf("fleet: release check for %s: %v", repo, err)
			continue
		}
		if version == "" || msiURL == "" {
			continue
		}
		// Only update accounts whose stored version differs
		for _, acctID := range accountIDs {
			acct, _ := rc.accounts.Get(ctx, acctID)
			if acct == nil || acct.LatestAgentVersion == version {
				continue
			}
			if err := rc.accounts.UpdateAgentVersion(ctx, acctID, version, msiURL, acct.AutoUpdateAgents); err != nil {
				log.Warnf("fleet: failed to update agent version for account %s: %v", acctID, err)
				continue
			}
			log.Infof("fleet: release check: updated account %s (%s) to %s", acct.Name, acctID, version)
		}
	}
}

func (rc *ReleaseChecker) fetchLatestRelease(repo string) (version, msiURL string, err error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "fibratus-fleet-server")

	resp, err := rc.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", "", err
	}

	// Extract version from tag (strip leading 'v')
	version = strings.TrimPrefix(release.TagName, "v")

	// Find the MSI asset
	for _, asset := range release.Assets {
		if strings.HasSuffix(asset.Name, ".msi") {
			msiURL = asset.BrowserDownloadURL
			break
		}
	}

	return version, msiURL, nil
}
