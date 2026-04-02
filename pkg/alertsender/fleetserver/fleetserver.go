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
	"github.com/rabbitstack/fibratus/pkg/alertsender"
	"github.com/rabbitstack/fibratus/pkg/fleetclient"
	log "github.com/sirupsen/logrus"
)

type sender struct {
	client *fleetclient.Client
}

func init() {
	alertsender.Register(alertsender.FleetServer, makeSender)
}

func makeSender(config alertsender.Config) (alertsender.Sender, error) {
	cfg, ok := config.Sender.(fleetclient.Config)
	if !ok {
		return nil, alertsender.ErrInvalidConfig(alertsender.FleetServer)
	}
	client, err := fleetclient.New(cfg, "")
	if err != nil {
		return nil, err
	}
	return &sender{client: client}, nil
}

func (s *sender) Send(alert alertsender.Alert) error {
	data, err := alert.MarshalJSON()
	if err != nil {
		return err
	}
	if err := s.client.SendDetection(data); err != nil {
		log.Warnf("fleet: failed to send detection: %v", err)
		return err
	}
	return nil
}

func (s *sender) Type() alertsender.Type { return alertsender.FleetServer }

func (s *sender) Shutdown() error {
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

func (s *sender) SupportsMarkdown() bool { return false }
