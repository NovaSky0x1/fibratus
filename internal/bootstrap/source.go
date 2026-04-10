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

package bootstrap

import (
	"github.com/rabbitstack/fibratus/internal/etw"
	"github.com/rabbitstack/fibratus/pkg/config"
	"github.com/rabbitstack/fibratus/pkg/event"
	"github.com/rabbitstack/fibratus/pkg/eventlog"
	"github.com/rabbitstack/fibratus/pkg/filter"
	"github.com/rabbitstack/fibratus/pkg/handle"
	"github.com/rabbitstack/fibratus/pkg/ps"
	"github.com/rabbitstack/fibratus/pkg/source"
	log "github.com/sirupsen/logrus"
)

// EventSourceControl abstracts away the management of event sources.
// It wraps the ETW source's output channel so that additional event
// producers (like the Windows Event Log collector) can inject events
// into the same stream the aggregator reads from.
type EventSourceControl struct {
	evs       source.EventSource
	collector *eventlog.Collector
	psnap     ps.Snapshotter

	// outEvts is the unified channel returned by Events(). It receives
	// events from the ETW source and, when active, from the event log
	// collector. Set up in Open() before the aggregator wires up.
	outEvts chan *event.Event
	stopFwd chan struct{}
}

func NewEventSourceControl(
	psnap ps.Snapshotter,
	hsnap handle.Snapshotter,
	config *config.Config,
	compiler *config.RulesCompileResult,
) *EventSourceControl {
	return &EventSourceControl{
		evs:   etw.NewEventSource(psnap, hsnap, config, compiler),
		psnap: psnap,
	}
}

func (s *EventSourceControl) Open(config *config.Config) error {
	if err := s.evs.Open(config); err != nil {
		return err
	}
	// Create unified output channel and start forwarding ETW events into it.
	// This must happen before Events() is called by the aggregator.
	s.outEvts = make(chan *event.Event, 1000)
	s.stopFwd = make(chan struct{})
	go func() {
		for {
			select {
			case <-s.stopFwd:
				return
			case evt, ok := <-s.evs.Events():
				if !ok {
					return
				}
				select {
				case s.outEvts <- evt:
				case <-s.stopFwd:
					return
				}
			}
		}
	}()
	return nil
}

func (s *EventSourceControl) Close() error {
	if s.stopFwd != nil {
		close(s.stopFwd)
	}
	if s.collector != nil {
		s.collector.Stop()
	}
	return s.evs.Close()
}

func (s *EventSourceControl) Errors() <-chan error {
	return s.evs.Errors()
}

func (s *EventSourceControl) Events() <-chan *event.Event {
	if s.outEvts != nil {
		return s.outEvts
	}
	return s.evs.Events()
}

func (s *EventSourceControl) SetFilter(f filter.Filter) {
	s.evs.SetFilter(f)
}

func (s *EventSourceControl) RegisterEventListener(lis event.Listener) {
	s.evs.RegisterEventListener(lis)
}

// StartEventLogCollector starts or reconfigures the event log collector.
// Collector events are forwarded into outEvts — the same channel the
// aggregator reads from — so eventlog events appear alongside ETW events.
func (s *EventSourceControl) StartEventLogCollector(cfg eventlog.Config) {
	if s.collector != nil {
		s.collector.Reconfigure(cfg)
		return
	}
	s.collector = eventlog.NewCollector(cfg, s.psnap)
	if err := s.collector.Start(); err != nil {
		log.Warnf("eventlog: collector start failed: %v", err)
		return
	}
	// Forward collector events into the unified output channel.
	go func() {
		for {
			select {
			case <-s.stopFwd:
				return
			case evt, ok := <-s.collector.Events():
				if !ok {
					return
				}
				select {
				case s.outEvts <- evt:
				case <-s.stopFwd:
					return
				}
			}
		}
	}()
	log.Infof("eventlog: collector started, forwarding to main event channel")
}

// convertEventLogConfig converts config.EventLogConfig to eventlog.Config.
func convertEventLogConfig(cfg config.EventLogConfig) eventlog.Config {
	channels := make([]eventlog.ChannelConfig, len(cfg.Channels))
	for i, ch := range cfg.Channels {
		channels[i] = eventlog.ChannelConfig{
			Name:       ch.Name,
			CollectAll: ch.CollectAll,
			EventIDs:   ch.EventIDs,
		}
	}
	return eventlog.Config{
		Enabled:      cfg.Enabled,
		Channels:     channels,
		BookmarkPath: cfg.BookmarkPath,
	}
}
