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
	"time"

	"github.com/rabbitstack/fibratus/pkg/fleet"
	pb "github.com/rabbitstack/fibratus/pkg/fleet/pb"
	log "github.com/sirupsen/logrus"
)

// CommandExecutor executes a command and returns the result.
type CommandExecutor interface {
	Execute(cmd *fleet.Command) (json.RawMessage, error)
}

// StartCommandLoop opens a persistent gRPC bidirectional stream for commands.
// Server pushes commands, agent sends back results.
func (c *Client) StartCommandLoop(executor CommandExecutor) {
	c.wg.Add(1)
	go c.commandStreamLoop(executor)
}

// commandStreamLoop maintains a persistent CommandChannel stream.
func (c *Client) commandStreamLoop(executor CommandExecutor) {
	defer c.wg.Done()

	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		err := c.runCommandChannel(executor)
		if err != nil {
			log.Warnf("fleet: command stream error: %v (reconnecting in %s)", err, backoff)
		}

		select {
		case <-time.After(backoff):
			backoff = backoff * 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		case <-c.stopCh:
			return
		}
	}
}

// runCommandChannel opens one CommandChannel stream and processes commands until error.
func (c *Client) runCommandChannel(executor CommandExecutor) error {
	c.mu.RLock()
	agentID := c.agentID
	c.mu.RUnlock()

	stream, err := c.agentClient.CommandChannel(c.grpcCtx())
	if err != nil {
		return fmt.Errorf("open command channel: %w", err)
	}

	// Send initial identification message
	if err := stream.Send(&pb.CommandResult{
		AgentId: agentID,
	}); err != nil {
		return fmt.Errorf("send command channel init: %w", err)
	}

	log.Info("fleet: command channel stream opened")

	// Reset backoff on successful connection
	for {
		cmd, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("receive command: %w", err)
		}

		log.Infof("fleet: received command %s (type: %s)", cmd.Id, cmd.Type)

		// Convert protobuf command to fleet.Command for executor
		fleetCmd := &fleet.Command{
			ID:      cmd.Id,
			Type:    cmd.Type,
			Payload: json.RawMessage(cmd.Payload),
		}

		result, execErr := executor.Execute(fleetCmd)

		// Send result back to server
		resultMsg := &pb.CommandResult{
			AgentId:   agentID,
			CommandId: cmd.Id,
		}

		if execErr != nil {
			log.Errorf("fleet: command %s failed: %v", cmd.Id, execErr)
			resultMsg.Status = fleet.CmdStatusFailed
			resultMsg.ErrorMessage = execErr.Error()
		} else {
			log.Infof("fleet: command %s completed", cmd.Id)
			resultMsg.Status = fleet.CmdStatusCompleted
			if result != nil {
				resultMsg.Result = []byte(result)
			}
		}

		if err := stream.Send(resultMsg); err != nil {
			return fmt.Errorf("send command result: %w", err)
		}
	}
}
