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

package fleetclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rabbitstack/fibratus/pkg/fleet"
	log "github.com/sirupsen/logrus"
)

// CommandExecutor executes a command and returns the result.
type CommandExecutor interface {
	Execute(cmd *fleet.Command) (json.RawMessage, error)
}

// PollCommands fetches pending commands from the fleet server.
func (c *Client) PollCommands() ([]*fleet.Command, error) {
	resp, err := c.doRequestWithHeaders(http.MethodGet, "/agent/commands", nil, map[string]string{
		"X-Agent-ID": c.AgentID(),
	})
	if err != nil {
		return nil, fmt.Errorf("fleet commands: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.readError(resp)
	}

	var apiResp struct {
		Data []*fleet.Command `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("fleet commands: decode: %w", err)
	}

	return apiResp.Data, nil
}

// ReportCommandResult sends the result of a command back to the server.
func (c *Client) ReportCommandResult(cmdID, status string, result json.RawMessage, errMsg string) error {
	body, _ := json.Marshal(fleet.CommandResultRequest{
		Status:       status,
		Result:       result,
		ErrorMessage: errMsg,
	})

	path := fmt.Sprintf("/agent/commands/%s/result", cmdID)
	resp, err := c.doRequestWithHeaders(http.MethodPost, path, body, map[string]string{
		"X-Agent-ID": c.AgentID(),
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.readError(resp)
	}
	return nil
}

// StartCommandLoop polls for commands and executes them in the background.
func (c *Client) StartCommandLoop(executor CommandExecutor) {
	c.wg.Add(1)
	go c.commandLoop(executor)
}

func (c *Client) commandLoop(executor CommandExecutor) {
	defer c.wg.Done()

	// Poll every 5 seconds for commands
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.processCommands(executor)
		case <-c.stopCh:
			log.Info("fleet: command loop stopped")
			return
		}
	}
}

func (c *Client) processCommands(executor CommandExecutor) {
	commands, err := c.PollCommands()
	if err != nil {
		log.Warnf("fleet: failed to poll commands: %v", err)
		return
	}

	for _, cmd := range commands {
		log.Infof("fleet: executing command %s (type: %s)", cmd.ID, cmd.Type)

		result, err := executor.Execute(cmd)
		if err != nil {
			log.Errorf("fleet: command %s failed: %v", cmd.ID, err)
			c.ReportCommandResult(cmd.ID, fleet.CmdStatusFailed, nil, err.Error())
			continue
		}

		log.Infof("fleet: command %s completed", cmd.ID)
		c.ReportCommandResult(cmd.ID, fleet.CmdStatusCompleted, result, "")
	}
}
