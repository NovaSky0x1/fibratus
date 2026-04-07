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

package nats

import (
	"fmt"

	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

// ProducerConfig configures the NATS producer.
type ProducerConfig struct {
	URL     string `yaml:"url"`
	Subject string `yaml:"subject"`
}

// Producer publishes telemetry event batches to a NATS subject.
// Each message is a protobuf-serialized TelemetryBatch.
type Producer struct {
	conn    *nats.Conn
	subject string
}

// NewProducer creates a NATS producer connection.
func NewProducer(cfg ProducerConfig) (*Producer, error) {
	nc, err := nats.Connect(cfg.URL,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*1e9), // 2s
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				log.Warnf("nats producer: disconnected: %v", err)
			}
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			log.Info("nats producer: reconnected")
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("nats producer: %w", err)
	}

	log.Infof("nats: producer connected to %s (subject: %s)", cfg.URL, cfg.Subject)
	return &Producer{conn: nc, subject: cfg.Subject}, nil
}

// Publish sends a serialized telemetry batch to NATS.
func (p *Producer) Publish(data []byte) error {
	return p.conn.Publish(p.subject, data)
}

// Close shuts down the producer connection.
func (p *Producer) Close() {
	p.conn.Drain()
}
