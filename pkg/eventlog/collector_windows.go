/*
 * Copyright 2021-2026 by Nedim Sabic Sabic
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

package eventlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rabbitstack/fibratus/pkg/event"
	"github.com/rabbitstack/fibratus/pkg/event/params"
	"github.com/rabbitstack/fibratus/pkg/ps"
	"github.com/rabbitstack/fibratus/pkg/sys/wevtapi"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

const (
	// batchSize is the number of events fetched per EvtNext call.
	batchSize = 64
	// nextTimeout is the timeout in milliseconds for EvtNext.
	nextTimeout = 1000
	// bookmarkFlushInterval is how often bookmarks are persisted to disk.
	bookmarkFlushInterval = 30 * time.Second
)

// Collector subscribes to Windows Event Log channels and emits
// event.Event instances for consumption by the Fibratus pipeline.
type Collector struct {
	config       Config
	psnap        ps.Snapshotter
	evts         chan *event.Event
	errs         chan error
	subscriptions []subscription
	seq          atomic.Uint64
	bookmarkDir  string
	mu           sync.Mutex
	closed       atomic.Bool
	stopCh       chan struct{}
}

type subscription struct {
	channel   string
	handle    wevtapi.EvtHandle
	bookmark  wevtapi.EvtHandle
	signal    windows.Handle
	eventIDs  map[uint16]struct{}
	collectAll bool
}

// NewCollector creates a new event log collector.
func NewCollector(config Config, psnap ps.Snapshotter) *Collector {
	bookmarkDir := config.BookmarkPath
	if bookmarkDir == "" {
		exe, _ := os.Executable()
		if exe != "" {
			bookmarkDir = filepath.Join(filepath.Dir(exe), "..", "data", "eventlog-bookmarks")
		} else {
			bookmarkDir = "data/eventlog-bookmarks"
		}
	}
	return &Collector{
		config:      config,
		psnap:       psnap,
		evts:        make(chan *event.Event, 500),
		errs:        make(chan error, 100),
		bookmarkDir: bookmarkDir,
		stopCh:      make(chan struct{}),
	}
}

// Events returns the channel where parsed events are pushed.
func (c *Collector) Events() <-chan *event.Event {
	return c.evts
}

// Errors returns the channel where collection errors are pushed.
func (c *Collector) Errors() <-chan error {
	return c.errs
}

// Start begins collecting events from all configured channels.
func (c *Collector) Start() error {
	log.Infof("eventlog: Start() called — enabled=%v, channels=%d", c.config.Enabled, len(c.config.Channels))
	if !c.config.Enabled || len(c.config.Channels) == 0 {
		log.Info("eventlog: collection disabled or no channels configured")
		return nil
	}
	os.MkdirAll(c.bookmarkDir, 0o700)

	for _, ch := range c.config.Channels {
		if err := c.subscribe(ch); err != nil {
			log.Errorf("eventlog: failed to subscribe to %s: %v", ch.Name, err)
			c.errs <- fmt.Errorf("eventlog subscribe %s: %w", ch.Name, err)
			continue
		}
		log.Infof("eventlog: subscribed to channel %s (collect_all=%v, event_ids=%v)", ch.Name, ch.CollectAll, ch.EventIDs)
	}

	// Start reader goroutines
	for i := range c.subscriptions {
		go c.readLoop(&c.subscriptions[i])
	}

	// Start bookmark flush goroutine
	go c.bookmarkFlushLoop()

	log.Infof("eventlog: collector started with %d channel(s)", len(c.subscriptions))
	return nil
}

// Stop shuts down all subscriptions and flushes bookmarks.
func (c *Collector) Stop() error {
	if c.closed.Swap(true) {
		return nil
	}
	close(c.stopCh)

	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.subscriptions {
		sub := &c.subscriptions[i]
		c.flushBookmark(sub)
		wevtapi.Close(sub.handle)
		wevtapi.Close(sub.bookmark)
		if sub.signal != 0 {
			windows.CloseHandle(sub.signal)
		}
	}
	c.subscriptions = nil
	return nil
}

// Reconfigure dynamically reconfigures the collector with a new set of channels.
// Existing subscriptions not in the new config are closed; new channels are subscribed.
func (c *Collector) Reconfigure(config Config) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed.Load() {
		return
	}

	// Close all existing subscriptions
	for i := range c.subscriptions {
		sub := &c.subscriptions[i]
		c.flushBookmark(sub)
		wevtapi.Close(sub.handle)
		wevtapi.Close(sub.bookmark)
		if sub.signal != 0 {
			windows.CloseHandle(sub.signal)
		}
	}
	c.subscriptions = nil

	c.config = config
	if !config.Enabled {
		log.Info("eventlog: collection disabled via reconfigure")
		return
	}

	for _, ch := range config.Channels {
		if err := c.subscribe(ch); err != nil {
			log.Errorf("eventlog: reconfigure failed to subscribe to %s: %v", ch.Name, err)
			continue
		}
		log.Infof("eventlog: reconfigured subscription to %s", ch.Name)
	}

	// Start reader goroutines for new subscriptions
	for i := range c.subscriptions {
		go c.readLoop(&c.subscriptions[i])
	}
}

func (c *Collector) subscribe(ch ChannelConfig) error {
	// Create auto-reset signal event for EvtSubscribe notification.
	// Per MSDN: use auto-reset event with EvtSubscribe signal mode.
	signal, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return fmt.Errorf("create signal event: %w", err)
	}

	// Build XPath query for event ID filtering at the source level
	// Use empty query (nil) for collect-all — "*" can cause issues on some Windows versions
	var query string
	if !ch.CollectAll && len(ch.EventIDs) > 0 {
		query = BuildXPathQuery(ch.EventIDs)
	}

	// Load existing bookmark for this channel
	bookmark, err := c.loadBookmark(ch.Name)
	if err != nil {
		log.Warnf("eventlog: couldn't load bookmark for %s, starting from future events: %v", ch.Name, err)
	}

	// Subscribe starting at oldest record to catch existing events immediately.
	// This validates the subscription is working, then bookmarks track position.
	var flags wevtapi.EvtSubscribeFlags = wevtapi.EvtSubscribeStartAtOldestRecord
	// Ignore any loaded bookmark for now
	if bookmark != 0 {
		wevtapi.Close(bookmark)
		bookmark = 0
	}

	handle, err := wevtapi.Subscribe(ch.Name, query, bookmark, signal, flags)
	if err != nil {
		windows.CloseHandle(signal)
		wevtapi.Close(bookmark)
		return fmt.Errorf("subscribe to %s: %w", ch.Name, err)
	}

	// If we couldn't use a bookmark, create a fresh one
	if bookmark == 0 {
		bookmark, _ = wevtapi.CreateBookmark("")
	}

	eventIDs := make(map[uint16]struct{}, len(ch.EventIDs))
	for _, id := range ch.EventIDs {
		eventIDs[id] = struct{}{}
	}

	c.subscriptions = append(c.subscriptions, subscription{
		channel:    ch.Name,
		handle:     handle,
		bookmark:   bookmark,
		signal:     signal,
		eventIDs:   eventIDs,
		collectAll: ch.CollectAll || len(ch.EventIDs) == 0,
	})
	return nil
}

func (c *Collector) readLoop(sub *subscription) {
	events := make([]wevtapi.EvtHandle, batchSize)
	log.Infof("eventlog: readLoop started for channel %s (handle=%v, signal=%v, closed=%v)", sub.channel, sub.handle, sub.signal, c.closed.Load())
	var totalEvents uint64
	var waitCount uint64

	for {
		if c.closed.Load() {
			log.Warnf("eventlog: %s readLoop exiting — closed=true", sub.channel)
			return
		}

		// EvtNext with INFINITE timeout blocks until events arrive.
		// For pull-based subscriptions (signal mode), this is valid per MSDN.
		returned, err := wevtapi.Next(sub.handle, events, 0xFFFFFFFF)
		waitCount++
		if returned == 0 {
			if err != nil {
				errno, ok := err.(syscall.Errno)
				if !ok || errno != 259 { // ERROR_NO_MORE_ITEMS
					if waitCount <= 3 || waitCount%30 == 0 {
						log.Warnf("eventlog: EvtNext on %s: %v (wait #%d)", sub.channel, err, waitCount)
					}
				}
			}
			continue
		}

		totalEvents += uint64(returned)
		if totalEvents <= 10 || totalEvents%1000 == 0 {
			log.Infof("eventlog: channel %s — %d events received (batch=%d)", sub.channel, totalEvents, returned)
		}

		for i := uint32(0); i < returned; i++ {
			c.processEvent(sub, events[i])
			if sub.bookmark != 0 {
				wevtapi.UpdateBookmark(sub.bookmark, events[i])
			}
			wevtapi.Close(events[i])
		}
	}
}

func (c *Collector) processEvent(sub *subscription, evtHandle wevtapi.EvtHandle) {
	xmlStr, err := wevtapi.RenderXML(evtHandle)
	if err != nil {
		return
	}

	parsed, err := ParseEventXML(xmlStr)
	if err != nil {
		return
	}

	// Event ID filtering (double-check in case XPath didn't filter)
	if !sub.collectAll && len(sub.eventIDs) > 0 {
		if _, ok := sub.eventIDs[parsed.System.EventID]; !ok {
			return
		}
	}

	evt := c.toEvent(parsed, sub.channel)
	if evt == nil {
		return
	}

	select {
	case c.evts <- evt:
	case <-c.stopCh:
	}
}

func (c *Collector) toEvent(parsed *EventXML, channel string) *event.Event {
	seq := c.seq.Add(1)

	evt := &event.Event{
		Seq:       seq,
		Timestamp: parsed.Timestamp(),
		PID:       parsed.System.Execution.ProcessID,
		Tid:       parsed.System.Execution.ThreadID,
		Type:      event.EventLogEvent,
		Name:      "EventLogEvent",
		Category:  event.EventLog,
		Description: fmt.Sprintf("Windows Event Log record from %s (EventID: %d)",
			channel, parsed.System.EventID),
		Host:     parsed.System.Computer,
		Params:   make(event.Params),
		Metadata: make(map[event.MetadataKey]any),
	}

	// Populate event parameters
	evt.AppendParam("eventlog.channel", params.UnicodeString, channel)
	evt.AppendParam("eventlog.provider", params.UnicodeString, parsed.System.Provider.Name)
	evt.AppendParam("eventlog.event.id", params.Uint16, parsed.System.EventID)
	evt.AppendParam("eventlog.level", params.UnicodeString, parsed.LevelName())
	evt.AppendParam("eventlog.level.id", params.Uint8, parsed.System.Level)
	evt.AppendParam("eventlog.record.id", params.Uint64, parsed.System.EventRecordID)
	evt.AppendParam("eventlog.task", params.Uint16, parsed.System.Task)
	evt.AppendParam("eventlog.opcode", params.Uint8, parsed.System.Opcode)
	evt.AppendParam("eventlog.keywords", params.UnicodeString, parsed.System.Keywords)
	evt.AppendParam("eventlog.computer", params.UnicodeString, parsed.System.Computer)
	evt.AppendParam("eventlog.user.id", params.UnicodeString, parsed.System.Security.UserID)

	// Populate EventData fields as eventlog.data.* parameters
	for _, d := range parsed.Data {
		if d.Name != "" {
			evt.AppendParam("eventlog.data."+d.Name, params.UnicodeString, d.Value)
		}
	}

	// Attach process state if PID is available and psnap is set
	if c.psnap != nil && parsed.System.Execution.ProcessID > 0 {
		if ok, proc := c.psnap.Find(parsed.System.Execution.ProcessID); ok {
			evt.PS = proc
		}
	}

	return evt
}

func (c *Collector) bookmarkFlushLoop() {
	ticker := time.NewTicker(bookmarkFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.mu.Lock()
			for i := range c.subscriptions {
				c.flushBookmark(&c.subscriptions[i])
			}
			c.mu.Unlock()
		}
	}
}

func (c *Collector) flushBookmark(sub *subscription) {
	if sub.bookmark == 0 {
		return
	}
	xml, err := wevtapi.RenderBookmark(sub.bookmark)
	if err != nil {
		return
	}
	// Sanitize channel name for filesystem
	safeName := sanitizeChannelName(sub.channel)
	path := filepath.Join(c.bookmarkDir, safeName+".json")
	data, _ := json.Marshal(map[string]string{"channel": sub.channel, "bookmark": xml})
	os.WriteFile(path, data, 0o600)
}

func (c *Collector) loadBookmark(channel string) (wevtapi.EvtHandle, error) {
	safeName := sanitizeChannelName(channel)
	path := filepath.Join(c.bookmarkDir, safeName+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var saved map[string]string
	if err := json.Unmarshal(data, &saved); err != nil {
		return 0, err
	}
	xml, ok := saved["bookmark"]
	if !ok || xml == "" {
		return 0, fmt.Errorf("no bookmark data")
	}
	return wevtapi.CreateBookmark(xml)
}

func sanitizeChannelName(channel string) string {
	r := make([]byte, 0, len(channel))
	for _, c := range channel {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
			r = append(r, byte(c))
		default:
			r = append(r, '_')
		}
	}
	return string(r)
}
