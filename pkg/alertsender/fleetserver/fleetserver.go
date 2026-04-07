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
	pb "github.com/rabbitstack/fibratus/pkg/fleet/pb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type sender struct {
	client  *fleetclient.Client
	queue   chan *pb.DetectionReport
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
		queue:  make(chan *pb.DetectionReport, 256),
		stopCh: make(chan struct{}),
	}
	go s.run()
	return s, nil
}

// Send queues the alert for async delivery — never blocks the rule engine.
func (s *sender) Send(alert alertsender.Alert) error {
	// Convert alertsender.Alert to protobuf DetectionReport
	det := &pb.DetectionReport{
		RuleId:      alert.ID,
		RuleName:    alert.Title,
		Title:       alert.Title,
		Text:        alert.Text,
		Description: alert.Description,
		Severity:    string(alert.Severity),
		Tags:        alert.Tags,
		Labels:      alert.Labels,
		Timestamp:   timestamppb.Now(),
	}

	// Serialize the alert (including events) as JSON bytes
	if data, err := alert.MarshalJSON(); err == nil {
		det.Events = data
	}

	select {
	case s.queue <- det:
	default:
		log.Warn("fleet: detection queue full, dropping alert")
	}
	return nil
}

// run processes queued detections in the background.
func (s *sender) run() {
	for {
		select {
		case det := <-s.queue:
			if err := s.client.SendDetection(det); err != nil {
				log.Warnf("fleet: failed to send detection: %v", err)
			}
		case <-s.stopCh:
			// drain remaining
			for {
				select {
				case det := <-s.queue:
					s.client.SendDetection(det)
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
