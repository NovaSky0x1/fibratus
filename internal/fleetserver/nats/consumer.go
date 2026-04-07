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
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
	pb "github.com/rabbitstack/fibratus/pkg/fleet/pb"
	"github.com/rabbitstack/fibratus/internal/fleetserver/store"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

// ConsumerConfig configures the NATS consumer.
type ConsumerConfig struct {
	URL     string `yaml:"url"`
	Subject string `yaml:"subject"`
	Queue   string `yaml:"queue"` // queue group for load-balanced consumption
}

// Consumer reads telemetry batches from NATS and writes to the telemetry store.
type Consumer struct {
	conn   *nats.Conn
	sub    *nats.Subscription
	store  store.TelemetryStore
	cancel context.CancelFunc
}

// NewConsumer creates a NATS consumer that drains telemetry into the store.
func NewConsumer(cfg ConsumerConfig, telemetryStore store.TelemetryStore) (*Consumer, error) {
	nc, err := nats.Connect(cfg.URL,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*1e9),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				log.Warnf("nats consumer: disconnected: %v", err)
			}
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			log.Info("nats consumer: reconnected")
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("nats consumer: %w", err)
	}

	return &Consumer{
		conn:  nc,
		store: telemetryStore,
	}, nil
}

// Start begins consuming telemetry from NATS.
func (c *Consumer) Start(ctx context.Context, subject, queue string) error {
	ctx, c.cancel = context.WithCancel(ctx)

	sub, err := c.conn.QueueSubscribe(subject, queue, func(msg *nats.Msg) {
		var batch pb.TelemetryBatch
		if err := proto.Unmarshal(msg.Data, &batch); err != nil {
			log.Warnf("nats: failed to unmarshal telemetry batch: %v", err)
			return
		}

		// Convert protobuf events to JSON RawMessage for store
		rawEvents := make([]json.RawMessage, 0, len(batch.Events))
		for _, evt := range batch.Events {
			if evt.RawEvent != nil && len(evt.RawEvent) > 0 {
				rawEvents = append(rawEvents, json.RawMessage(evt.RawEvent))
			} else {
				raw := buildEventJSON(evt)
				rawEvents = append(rawEvents, raw)
			}
		}

		if len(rawEvents) > 0 {
			if err := c.store.BulkIngest(ctx, batch.OrgId, batch.AgentId, batch.Hostname, rawEvents); err != nil {
				log.Warnf("nats: telemetry ingest error (agent %s): %v", batch.AgentId, err)
			}
		}
	})
	if err != nil {
		return fmt.Errorf("nats consumer subscribe: %w", err)
	}

	c.sub = sub
	log.Infof("nats: consumer started (subject: %s, queue: %s)", subject, queue)
	return nil
}

// Stop shuts down the consumer.
func (c *Consumer) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	if c.sub != nil {
		c.sub.Unsubscribe()
	}
	if c.conn != nil {
		c.conn.Drain()
	}
}

// buildEventJSON constructs a JSON object from protobuf TelemetryEvent fields.
func buildEventJSON(evt *pb.TelemetryEvent) json.RawMessage {
	m := map[string]interface{}{
		"seq":      evt.Seq,
		"name":     evt.EventName,
		"category": evt.EventCategory,
		"pid":      evt.Pid,
		"tid":      evt.Tid,
	}
	if evt.Timestamp != nil {
		m["timestamp"] = evt.Timestamp.AsTime().Format("2006-01-02T15:04:05.000Z")
	}
	if evt.ProcessName != "" {
		m["ps"] = map[string]interface{}{
			"name":    evt.ProcessName,
			"exe":     evt.ProcessExe,
			"cmdline": evt.ProcessCmdline,
			"ppid":    evt.ParentPid,
			"parent":  evt.ParentName,
		}
	}
	if len(evt.Params) > 0 {
		m["kparams"] = json.RawMessage(evt.Params)
	}
	if len(evt.Metadata) > 0 {
		m["metadata"] = json.RawMessage(evt.Metadata)
	}
	data, _ := json.Marshal(m)
	return data
}
