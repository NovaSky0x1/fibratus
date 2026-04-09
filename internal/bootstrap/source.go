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
// It combines the ETW kernel event source with the optional Windows
// Event Log collector. Both sources push events to a unified channel
// that feeds the rules engine and output pipeline.
type EventSourceControl struct {
	evs       source.EventSource
	collector *eventlog.Collector
	psnap     ps.Snapshotter
	mergedEvts chan *event.Event
	mergedErrs chan error
	stopMerge  chan struct{}
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
	// If event log collection is configured, start the collector
	// and merge its events into the unified output channel.
	if config.EventLog.Enabled && len(config.EventLog.Channels) > 0 {
		s.collector = eventlog.NewCollector(convertEventLogConfig(config.EventLog), s.psnap)
		if err := s.collector.Start(); err != nil {
			log.Warnf("eventlog: collector start failed: %v", err)
		} else {
			s.startMerge()
		}
	}
	return nil
}

// startMerge merges ETW events and event log events into unified channels.
func (s *EventSourceControl) startMerge() {
	s.mergedEvts = make(chan *event.Event, 1000)
	s.mergedErrs = make(chan error, 200)
	s.stopMerge = make(chan struct{})

	// Merge ETW events
	go func() {
		for {
			select {
			case <-s.stopMerge:
				return
			case evt, ok := <-s.evs.Events():
				if !ok {
					return
				}
				select {
				case s.mergedEvts <- evt:
				case <-s.stopMerge:
					return
				}
			}
		}
	}()

	// Merge event log events
	go func() {
		for {
			select {
			case <-s.stopMerge:
				return
			case evt, ok := <-s.collector.Events():
				if !ok {
					return
				}
				select {
				case s.mergedEvts <- evt:
				case <-s.stopMerge:
					return
				}
			}
		}
	}()

	// Merge errors from both sources
	go func() {
		for {
			select {
			case <-s.stopMerge:
				return
			case err, ok := <-s.evs.Errors():
				if !ok {
					continue
				}
				select {
				case s.mergedErrs <- err:
				case <-s.stopMerge:
					return
				}
			}
		}
	}()
	go func() {
		for {
			select {
			case <-s.stopMerge:
				return
			case err, ok := <-s.collector.Errors():
				if !ok {
					continue
				}
				select {
				case s.mergedErrs <- err:
				case <-s.stopMerge:
					return
				}
			}
		}
	}()
}

func (s *EventSourceControl) Close() error {
	if s.stopMerge != nil {
		close(s.stopMerge)
	}
	if s.collector != nil {
		s.collector.Stop()
	}
	return s.evs.Close()
}

func (s *EventSourceControl) Errors() <-chan error {
	if s.mergedErrs != nil {
		return s.mergedErrs
	}
	return s.evs.Errors()
}

func (s *EventSourceControl) Events() <-chan *event.Event {
	if s.mergedEvts != nil {
		return s.mergedEvts
	}
	return s.evs.Events()
}

func (s *EventSourceControl) SetFilter(f filter.Filter) {
	s.evs.SetFilter(f)
}

func (s *EventSourceControl) RegisterEventListener(lis event.Listener) {
	s.evs.RegisterEventListener(lis)
}

// EventLogCollector returns the event log collector for dynamic reconfiguration.
func (s *EventSourceControl) EventLogCollector() *eventlog.Collector {
	return s.collector
}

// StartEventLogCollector starts the event log collector with the given config.
// Used for dynamic reconfiguration when the server pushes a new event log policy.
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
	// If we weren't already merging, start the merge goroutines
	if s.mergedEvts == nil {
		s.startMerge()
	}
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
