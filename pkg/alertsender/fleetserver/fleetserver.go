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
	"os"
	"path/filepath"

	"github.com/rabbitstack/fibratus/pkg/alertsender"
	"github.com/rabbitstack/fibratus/pkg/fleetclient"
	log "github.com/sirupsen/logrus"
)

type sender struct {
	client  *fleetclient.Client
	queue   chan []byte
	stopCh  chan struct{}
}

func init() {
	alertsender.Register(alertsender.FleetServer, makeSender)
}

func makeSender(config alertsender.Config) (alertsender.Sender, error) {
	cfg, ok := config.Sender.(fleetclient.Config)
	if !ok {
		return nil, alertsender.ErrInvalidConfig(alertsender.FleetServer)
	}
	exe, _ := os.Executable()
	if exe == "" {
		exe = "."
	}
	dataDir := filepath.Join(filepath.Dir(exe), "..", "data")
	client, err := fleetclient.New(cfg, dataDir)
	if err != nil {
		return nil, err
	}
	s := &sender{
		client: client,
		queue:  make(chan []byte, 256),
		stopCh: make(chan struct{}),
	}
	go s.run()
	return s, nil
}

// Send queues the alert for async delivery — never blocks the rule engine.
func (s *sender) Send(alert alertsender.Alert) error {
	data, err := alert.MarshalJSON()
	if err != nil {
		return err
	}
	select {
	case s.queue <- data:
	default:
		log.Warn("fleet: detection queue full, dropping alert")
	}
	return nil
}

// run processes queued detections in the background.
func (s *sender) run() {
	for {
		select {
		case data := <-s.queue:
			if err := s.client.SendDetection(data); err != nil {
				log.Warnf("fleet: failed to send detection: %v", err)
			}
		case <-s.stopCh:
			// drain remaining
			for {
				select {
				case data := <-s.queue:
					s.client.SendDetection(data)
				default:
					return
				}
			}
		}
	}
}

func (s *sender) Type() alertsender.Type { return alertsender.FleetServer }

func (s *sender) Shutdown() error {
	close(s.stopCh)
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

func (s *sender) SupportsMarkdown() bool { return false }
