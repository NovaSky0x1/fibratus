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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rabbitstack/fibratus/internal/evasion"
	"github.com/rabbitstack/fibratus/pkg/aggregator"
	"github.com/rabbitstack/fibratus/pkg/alertsender"
	"github.com/rabbitstack/fibratus/pkg/event"
	"github.com/rabbitstack/fibratus/pkg/eventlog"
	"github.com/rabbitstack/fibratus/pkg/api"
	"github.com/rabbitstack/fibratus/pkg/cap"
	"github.com/rabbitstack/fibratus/pkg/config"
	"github.com/rabbitstack/fibratus/pkg/filament"
	"github.com/rabbitstack/fibratus/pkg/filter"
	"github.com/rabbitstack/fibratus/pkg/fleet"
	"github.com/rabbitstack/fibratus/pkg/fleet/tamper"
	"github.com/rabbitstack/fibratus/pkg/fleetclient"
	"github.com/rabbitstack/fibratus/pkg/handle"
	"github.com/rabbitstack/fibratus/pkg/outputs"
	fleetoutput "github.com/rabbitstack/fibratus/pkg/outputs/fleetserver"
	"github.com/rabbitstack/fibratus/pkg/ps"
	"github.com/rabbitstack/fibratus/pkg/rules"
	"github.com/rabbitstack/fibratus/pkg/symbolize"
	"github.com/rabbitstack/fibratus/pkg/sys"
	"github.com/rabbitstack/fibratus/pkg/util/multierror"
	"github.com/rabbitstack/fibratus/pkg/util/signals"
	"github.com/rabbitstack/fibratus/pkg/util/va"
	"github.com/rabbitstack/fibratus/pkg/util/version"
	"github.com/rabbitstack/fibratus/pkg/yara"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
)

// ErrAlreadyRunning signals a Fibratus process is already running in the system
var ErrAlreadyRunning = errors.New("an instance of Fibratus process is already running in the system")

// App centralizes the core building blocks responsible
// for event acquisition, rule engine initialization,
// captures handling, filament execution and event routing
// to the output sinks.
type App struct {
	config      *config.Config
	evs         *EventSourceControl
	symbolizer  *symbolize.Symbolizer
	engine      *rules.Engine
	hsnap       handle.Snapshotter
	psnap       ps.Snapshotter
	filament    filament.Filament
	agg         *aggregator.BufferedAggregator
	writer      cap.Writer
	reader      cap.Reader
	fleetClient *fleetclient.Client
	yaraScanner yara.Scanner
	signals     chan struct{}
}

// Option enables changing the behaviour of the bootstrap application.
type Option func(*opts)

type opts struct {
	setDebugPrivilege bool
	installSignals    bool
	isCaptureReplay   bool
	handleSnapshotFn  handle.SnapshotBuildCompleted
}

// WithSignals installs signal handlers.
func WithSignals() Option {
	return func(o *opts) {
		o.installSignals = true
	}
}

// WithDebugPrivilege injects the SeDebugPrivilege in the process access token.
func WithDebugPrivilege() Option {
	return func(o *opts) {
		o.setDebugPrivilege = true
	}
}

// WithCaptureReplay denotes the capture file is being replayed.
func WithCaptureReplay() Option {
	return func(o *opts) {
		o.isCaptureReplay = true
	}
}

// WithHandleSnapshotFn sets the handle snapshotter completion function.
func WithHandleSnapshotFn(fn handle.SnapshotBuildCompleted) Option {
	return func(o *opts) {
		o.handleSnapshotFn = fn
	}
}

// NewApp constructs a new bootstrap application with the specified configuration
// and a list of options. The configuration is passed from individual command work
// functions.
func NewApp(cfg *config.Config, options ...Option) (*App, error) {
	if err := InitConfigAndLogger(cfg); err != nil {
		return nil, err
	}
	var opts opts
	var sigs chan struct{}
	for _, opt := range options {
		opt(&opts)
	}
	if cfg.DebugPrivilege && opts.setDebugPrivilege {
		sys.SetDebugPrivilege()
	}
	if opts.installSignals {
		sigs = signals.Install()
	}
	if opts.isCaptureReplay {
		reader, err := cap.NewReader(cfg.CapFile, cfg)
		if err != nil {
			return nil, err
		}
		app := &App{
			config:  cfg,
			reader:  reader,
			signals: sigs,
		}
		return app, nil
	}

	hsnap := handle.NewSnapshotter(cfg, opts.handleSnapshotFn)
	psnap := ps.NewSnapshotter(hsnap, cfg)

	// Detect enrollment data EARLY — before rules, output, and event pipeline setup.
	// After `fibratus enroll`, all connection info is stored in DPAPI-encrypted registry.
	// Falls back to plaintext files for backward compatibility with pre-migration agents.
	{
		exe, _ := os.Executable()
		if exe == "" {
			exe = "."
		}
		dataDir := filepath.Join(filepath.Dir(exe), "..", "data")

		// Try DPAPI-encrypted registry first (preferred)
		if enrollData := tamper.LoadEnrollment(); enrollData != nil {
			cfg.Fleet.Enabled = true
			cfg.Fleet.ServerURL = enrollData.ServerURL
			cfg.Fleet.OrgID = enrollData.OrgID
			log.Infof("fleet: enrollment loaded from DPAPI-encrypted registry — server: %s", enrollData.ServerURL)
		} else {
			// Fall back to legacy plaintext files + attempt migration
			if tamper.MigrateFilesToRegistry(dataDir) {
				log.Info("fleet: migrated enrollment from files to DPAPI registry")
				// Re-read from registry after migration
				if enrollData := tamper.LoadEnrollment(); enrollData != nil {
					cfg.Fleet.Enabled = true
					cfg.Fleet.ServerURL = enrollData.ServerURL
					cfg.Fleet.OrgID = enrollData.OrgID
				}
			} else if serverURL := loadFileContent(filepath.Join(dataDir, "server-url")); serverURL != "" {
				// Files exist but migration failed — use files directly
				cfg.Fleet.Enabled = true
				cfg.Fleet.ServerURL = serverURL
				log.Infof("fleet: enrollment detected from files — server: %s", serverURL)
				if orgID := loadFileContent(filepath.Join(dataDir, "org-id")); orgID != "" {
					cfg.Fleet.OrgID = orgID
				}
			}
		}

		// Set defaults if not already configured
		if cfg.Fleet.Enabled {
			if cfg.Fleet.HeartbeatInterval <= 0 {
				cfg.Fleet.HeartbeatInterval = 30 * time.Second
			}
			if cfg.Fleet.RuleSyncInterval <= 0 {
				cfg.Fleet.RuleSyncInterval = 5 * time.Minute
			}
			if cfg.Fleet.Timeout <= 0 {
				cfg.Fleet.Timeout = 10 * time.Second
			}
			// Protect registry keys on every startup
			tamper.ProtectRegistryKeys()
		}
	}

	var engine *rules.Engine
	var rs *config.RulesCompileResult

	if cfg.Filters.Rules.Enabled && !cfg.ForwardMode && !cfg.IsCaptureSet() && !cfg.IsFilamentSet() {
		// When fleet mode is enabled, override rule paths to use server-managed rules
		if cfg.Fleet.Enabled {
			log.Info("fleet mode: rules will be managed by the fleet server")
			exe, err := os.Executable()
			if err != nil {
				exe = "."
			}
			fleetRulesDir := filepath.Join(filepath.Dir(exe), "..", "data", "rules")
			cfg.Filters.Rules.FromPaths = []string{filepath.Join(fleetRulesDir, "*")}
			cfg.Filters.Rules.FromURLs = nil
			cfg.Filters.Macros.FromPaths = []string{filepath.Join(fleetRulesDir, "Macros", "*")}
		}

		engine = rules.NewEngine(psnap, cfg)
		if cfg.Fleet.Enabled {
			engine.SetFleetMode(true)
		}
		var err error
		rs, err = engine.Compile()
		if err != nil {
			if cfg.Fleet.Enabled {
				// Don't fail startup if fleet rules aren't downloaded yet
				log.Warnf("fleet: initial rule compilation failed (rules may not be synced yet): %v", err)
			} else {
				return nil, err
			}
		}
		if rs != nil {
			log.Infof("rules compile summary: %s", rs)
		}
		// In fleet mode, rules arrive asynchronously after ETW trace starts.
		// Ensure ALL event types are collected so rules compiled later can
		// evaluate against any event type. Without this, the ETW trace is
		// configured to only collect events for rules known at startup (0 rules).
		if cfg.Fleet.Enabled {
			// Reduce working set thread pool to 1 worker in fleet mode.
			// The pool creates native threads that sit in WaitForMultipleObjects
			// which Windows counts as CPU time. 8 threads = ~12% phantom CPU.
			va.FleetMode = true
			// Stack enrichment enabled — the StackwalkDecorator flusher
			// releases events without holding the mutex to prevent deadlock
			// when the event channel is full (fleet mode high throughput).
			cfg.EventSource.StackEnrichment = true
			// Disable threadpool events — high volume, low detection value.
			cfg.EventSource.EnableThreadpoolEvents = false
			log.Info("fleet mode: ETW trace uses rule-driven drop masks, stack enrichment enabled, threadpool disabled, ws pool=1")
		}
	} else {
		log.Info("rule engine is disabled")
	}

	evs := NewEventSourceControl(psnap, hsnap, cfg, rs)

	app := &App{
		config:  cfg,
		evs:     evs,
		engine:  engine,
		hsnap:   hsnap,
		psnap:   psnap,
		signals: sigs,
	}

	return app, nil
}

// Run configure and opens the event source to start consuming events.
// Depending on whether the filament is provided, this method will either
// spin up a filament or set up the aggregator to start forwarding events
// to the rule engine and output sinks.
func (f *App) Run(args []string) error {
	if f.evs == nil {
		panic("event source is nil")
	}
	cfg := f.config

	if !f.isSingleInstance() {
		return ErrAlreadyRunning
	}

	log.Infof("bootstrapping with pid %d. Version: %s", os.Getpid(), version.Get())
	log.Infof("configuration options: %s", cfg.Print())

	// build the filter from the CLI argument. If we got
	// a valid expression the filter is attached to the
	// event consumer
	fltr, err := filter.NewFromCLI(args, cfg)
	if err != nil {
		return err
	}
	if fltr != nil {
		f.evs.SetFilter(fltr)
	}
	// user can either instruct to bootstrap a filament or
	// start a regular run. We'll set up the corresponding
	// components accordingly to what we got from the CLI options.
	// If a filament was given, we'll assign it the previous filter
	// if it wasn't provided in the filament init function.
	// Finally, we open the event source and run the filament i.e.
	// Python main thread in a new goroutine.
	// In case of a regular run, we additionally set up the aggregator.
	// The aggregator will grab the events from the queue, assemble them
	// into batches and hand over to output sinks.
	if cfg.IsFilamentSet() {
		f.filament, err = filament.New(cfg.Filament.Name, f.psnap, f.hsnap, cfg)
		if err != nil {
			return err
		}
		if f.filament.Filter() != nil {
			f.evs.SetFilter(f.filament.Filter())
		}
		err = f.evs.Open(cfg)
		if err != nil {
			return multierror.Wrap(err, f.evs.Close())
		}
		// load alert senders so emitting alerts is possible from filaments
		err = alertsender.LoadAll(cfg.Alertsenders)
		if err != nil {
			log.Warnf("couldn't load alertsenders: %v", err)
		}
		go func() {
			err = f.filament.Run(f.evs.Events(), f.evs.Errors())
			if err != nil {
				log.Errorf("filament failed: %v", err)
				f.stop()
			}
		}()
	} else {
		// register stack symbolizer
		if cfg.EventSource.StackEnrichment {
			f.symbolizer = symbolize.NewSymbolizer(symbolize.NewDebugHelpResolver(cfg), f.psnap, cfg, false)
			// Wire the rule engine's callstack field map into the symbolizer.
			// This lets the symbolizer skip expensive Debug Help API calls for
			// event types where no rule uses thread.callstack.* fields.
			if f.engine != nil {
				f.symbolizer.NeedsFullSymbolization = f.engine.NeedsCallstack
			}
			f.evs.RegisterEventListener(f.symbolizer)
		}
		// register evasion scanner
		if cfg.Evasion.Enabled {
			f.evs.RegisterEventListener(evasion.NewScanner(cfg.Evasion))
		}
		// register rule engine
		if f.engine != nil {
			f.evs.RegisterEventListener(f.engine)
		}
		// YARA scanner — server-managed, on-demand only. Rules live in the
		// fleet server's yara_rules table; every yara_scan command carries
		// the applicable rule set inline in its payload. The agent does NOT
		// register the scanner as an event listener, so no inline matches
		// on process creation / image load / PE write fire a detection.
		// The scanner struct is still constructed (with zero on-disk rules
		// most of the time) so the executor's ScanTargetInline has a
		// process snapshotter bound for PID-target scans.
		if cfg.Yara.Enabled {
			scanner, err := yara.NewScanner(f.psnap, cfg.Yara)
			if err != nil {
				// Slim builds (no yara build tag) return ErrFeatureUnsupported.
				// Don't take the whole agent down — log and proceed without
				// the YARA scanner. Operators using on-demand scans will see
				// command failures, but baseline agent ingest must keep working.
				log.Warnf("fleet: yara scanner not available — %v. Continuing without YARA support; rebuild with -tags yara,yara_static for inline scanning.", err)
			} else {
				f.yaraScanner = scanner
			}
		}
		err = f.evs.Open(cfg)
		if err != nil {
			return multierror.Wrap(err, f.evs.Close())
		}
		// Register the capture filter compiler so the fleet executor can
		// compile Fibratus QL expressions without importing pkg/filter
		// directly (which would create an import cycle).
		fleetoutput.CaptureFilterCompiler = func(expr string) (func(*event.Event) bool, error) {
			f, err := filter.NewFromCLIWithAllAccessors([]string{expr})
			if err != nil {
				return nil, err
			}
			return f.Run, nil
		}

		// When fleet mode is active, auto-enable fleet server as the output
		// to stream telemetry to the fleet server for remote visibility.
		// Override console or null output — fleet telemetry takes priority.
		if cfg.Fleet.Enabled && cfg.Output.Type != outputs.FleetServer {
			log.Info("fleet: auto-enabling fleet server telemetry output")
			cfg.Output.Type = outputs.FleetServer
			cfg.Output.Output = fleetoutput.Config{
				Enabled:   true,
				ServerURL: cfg.Fleet.ServerURL,
				OrgID:     cfg.Fleet.OrgID,
			}
		}

		// Auto-enable fleet server alert sender so detections reach the server
		if cfg.Fleet.Enabled {
			hasFleetSender := false
			for _, s := range cfg.Alertsenders {
				if s.Type == alertsender.FleetServer {
					hasFleetSender = true
					break
				}
			}
			if !hasFleetSender {
				log.Info("fleet: auto-enabling fleet server alert sender")
				timeout := cfg.Fleet.Timeout
				if timeout == 0 {
					timeout = 10 * time.Second
				}
				cfg.Alertsenders = append(cfg.Alertsenders, alertsender.Config{
					Type: alertsender.FleetServer,
					Sender: fleetclient.Config{
						Enabled:    true,
						ServerURL:  cfg.Fleet.ServerURL,
						OrgID:      cfg.Fleet.OrgID,
						Timeout:    timeout,
						EnableGzip: cfg.Fleet.EnableGzip,
					},
				})
			}
		}

		// set up the aggregator that forwards events to outputs
		f.agg, err = aggregator.NewBuffered(
			f.evs.Events(),
			f.evs.Errors(),
			cfg.Aggregator,
			cfg.Output,
			cfg.Transformers,
			cfg.Alertsenders,
		)
		if err != nil {
			return err
		}
	}
	// Initialize fleet client if fleet mode is enabled (set in NewApp via enrollment detection)
	if cfg.Fleet.Enabled {
		if err := f.initFleetClient(cfg); err != nil {
			log.Errorf("fleet: failed to initialize client: %v", err)
		}
	}

	// start the HTTP server
	return api.StartServer(cfg)
}

// WriteCapture writes the event stream to the capture file.
func (f *App) WriteCapture(args []string) error {
	if f.evs == nil {
		panic("event source is nil")
	}

	if !f.isSingleInstance() {
		return ErrAlreadyRunning
	}

	fltr, err := filter.NewFromCLI(args, f.config)
	if err != nil {
		return err
	}
	if fltr != nil {
		f.evs.SetFilter(fltr)
	}
	err = f.evs.Open(f.config)
	if err != nil {
		return err
	}
	f.writer, err = cap.NewWriter(f.config.CapFile, f.psnap, f.hsnap)
	if err != nil {
		return err
	}
	errsChan := f.writer.Write(f.evs.Events(), f.evs.Errors())
	go func() {
		for err := range errsChan {
			log.Warnf("fail to write event to capture: %v", err)
		}
	}()
	return api.StartServer(f.config)
}

// ReadCapture reconstructs the event stream from the capture file.
func (f *App) ReadCapture(ctx context.Context, args []string) error {
	if f.reader == nil {
		panic("reader is nil")
	}
	fltr, err := filter.NewFromCLIWithAllAccessors(args)
	if err != nil {
		return err
	}
	f.hsnap, f.psnap, err = f.reader.RecoverSnapshotters()
	if err != nil {
		return err
	}

	if f.config.IsFilamentSet() {
		f.filament, err = filament.New(f.config.Filament.Name, f.psnap, f.hsnap, f.config)
		if err != nil {
			return err
		}
		if f.filament.Filter() != nil {
			// filament filter overrides CLI filter
			f.reader.SetFilter(f.filament.Filter())
		} else if fltr != nil {
			f.reader.SetFilter(fltr)
		}
		// returns the channel where events are read from the cap
		evts, errs := f.reader.Read(ctx)
		go func() {
			defer f.filament.Close()
			err = f.filament.Run(evts, errs)
			if err != nil {
				log.Errorf("filament failed: %v", err)
				f.stop()
			}
		}()
	} else {
		if fltr != nil {
			f.reader.SetFilter(fltr)
		}
		// use the channels where events are read
		// from the capture as aggregator source
		evts, errs := f.reader.Read(ctx)
		f.agg, err = aggregator.NewBuffered(
			evts,
			errs,
			f.config.Aggregator,
			f.config.Output,
			f.config.Transformers,
			f.config.Alertsenders,
		)
		if err != nil {
			return err
		}
	}

	return api.StartServer(f.config)
}

// Wait waits for the app to receive the termination signal.
func (f *App) Wait() {
	if f.signals != nil {
		<-f.signals
	}
}

// Shutdown is responsible for tearing down everything gracefully.
func (f *App) Shutdown() error {
	errs := make([]error, 0)
	if f.symbolizer != nil {
		f.symbolizer.Close()
	}
	if f.evs != nil {
		if err := f.evs.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if f.hsnap != nil {
		if err := f.hsnap.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if f.psnap != nil {
		if err := f.psnap.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if f.filament != nil {
		if err := f.filament.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if f.writer != nil {
		if err := f.writer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if f.reader != nil {
		if err := f.reader.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if f.agg != nil {
		if err := f.agg.Stop(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := handle.CloseTimeout(); err != nil {
		errs = append(errs, err)
	}
	if f.fleetClient != nil {
		if err := f.fleetClient.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := api.CloseServer(); err != nil {
		errs = append(errs, err)
	}
	if err := alertsender.ShutdownAll(); err != nil {
		errs = append(errs, err)
	}
	return multierror.Wrap(errs...)
}

// isEnrolled checks if enrollment data exists on disk from a prior
// `fibratus enroll` command. If it does, fleet mode can be auto-enabled.
// initFleetClient initializes the fleet client, registers with
// the fleet server, and starts the heartbeat goroutine.
func (f *App) initFleetClient(cfg *config.Config) error {
	log.Infof("fleet: initializing client (server=%s, org=%s)", cfg.Fleet.ServerURL, cfg.Fleet.OrgID)

	exe, err := os.Executable()
	if err != nil {
		exe = "."
	}
	dataDir := filepath.Join(filepath.Dir(exe), "..", "data")
	log.Infof("fleet: data directory: %s", dataDir)

	client, err := fleetclient.New(cfg.Fleet.Config, dataDir)
	if err != nil {
		return err
	}
	f.fleetClient = client

	if err := client.Register(); err != nil {
		if errors.Is(err, fleetclient.ErrDecommissioned) {
			log.Warn("fleet: agent decommissioned — executing self-uninstall")
			executor := fleetclient.NewWindowsExecutor(cfg.Fleet.ServerURL, nil, nil)
			executor.Execute(&fleet.Command{Type: fleet.CmdUninstall})
			return fmt.Errorf("agent decommissioned by server")
		}
		return err
	}

	// Start heartbeat with engine stats if available
	var collector fleetclient.HeartbeatCollector
	if f.engine != nil {
		collector = f.engine
	}
	client.StartHeartbeat(collector)

	// Start rule sync — rules are loaded into DPAPI-encrypted memory,
	// never written to disk. On update, rules are fed directly to the
	// rule engine compiler from memory.
	client.StartRuleSync(func(ruleDocs [][]byte, macrosYAML []byte) error {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("fleet: panic during rule compilation (recovered): %v", r)
			}
		}()

		log.Infof("fleet: %d rules received in encrypted memory, compiling...", len(ruleDocs))

		// Load macros from memory first (rules may reference them)
		if len(macrosYAML) > 0 {
			if err := cfg.Filters.LoadMacrosFromMemory(macrosYAML); err != nil {
				log.Warnf("fleet: failed to load macros from memory: %v", err)
			}
		}

		// Load rules from memory
		if err := cfg.Filters.LoadFiltersFromMemory(ruleDocs); err != nil {
			log.Errorf("fleet: load rules from memory failed: %v", err)
			return fmt.Errorf("fleet: load rules from memory: %w", err)
		}

		// Recompile the rule engine with new in-memory rules.
		// The engine may not be initialized yet on the first rule delivery
		// (rules arrive via async gRPC stream before engine.New() completes).
		// In that case, the engine will compile these rules on its normal init path.
		if f.engine == nil {
			log.Info("fleet: rules loaded to memory — engine will compile on init")
			return nil
		}

		result, err := f.engine.Compile()
		if err != nil {
			log.Errorf("fleet: rule compile error: %v", err)
			return err
		}
		// Update drop masks so the ETW consumer only processes
		// event types referenced by the compiled rules.
		f.evs.UpdateDropMasks(result, cfg)
		log.Infof("fleet: rules compiled from encrypted memory — %d rules active", result.NumberRules)

		return nil
	})

	// Start command polling — agent checks for pending commands every 5 seconds
	// Create WFP isolator and tamper protector
	wfpIsolator := tamper.NewWFPIsolator()
	protector := tamper.NewProtector(
		filepath.Dir(filepath.Dir(exe)), // install dir
		dataDir,
		"fibratus",
	)

	// Re-apply persisted tamper protection state from before reboot
	protector.LoadPersistedState()

	// Re-apply isolation if persisted
	if isoState, err := os.ReadFile(filepath.Join(dataDir, "isolation-state")); err == nil {
		if strings.TrimSpace(string(isoState)) == "isolated" {
			serverHost := strings.TrimPrefix(cfg.Fleet.ServerURL, "https://")
			serverHost = strings.TrimPrefix(serverHost, "http://")
			serverHost = strings.Split(serverHost, ":")[0]
			if err := wfpIsolator.Isolate(serverHost, nil); err != nil {
				log.Warnf("fleet: failed to re-apply WFP isolation: %v", err)
			} else {
				log.Info("fleet: WFP network isolation re-applied from persisted state")
			}
		}
	}

	executor := fleetclient.NewWindowsExecutor(cfg.Fleet.ServerURL, wfpIsolator, protector)
	// Wire the YARA scanner created during Run() so the yara_scan
	// active-response command can execute in-process. Adapt the Scanner
	// interface to the executor's function-typed hook to avoid importing
	// pkg/yara from pkg/fleetclient (which would close an import cycle
	// through pkg/config).
	if f.yaraScanner != nil {
		scanner := f.yaraScanner
		executor.SetYaraScanner(func(target any, inlineRules string) (any, error) {
			if inlineRules != "" {
				return scanner.ScanTargetInline(target, inlineRules)
			}
			return scanner.ScanTarget(target)
		})
		log.Info("fleet: on-demand YARA scanner wired to active-response executor")
	} else {
		log.Warn("fleet: no YARA scanner available for active-response yara_scan command (yara.enabled=false or build without yara tag)")
	}
	// Register event log reconfigure callback so server policy changes
	// dynamically update the collector without agent restart
	executor.SetEventLogReconfigureCallback(func(policyJSON json.RawMessage) {
		if f.evs == nil {
			return
		}
		elCfg, err := parseEventLogPolicyJSON(policyJSON)
		if err != nil {
			log.Errorf("eventlog: failed to parse policy JSON: %v", err)
			return
		}
		f.evs.StartEventLogCollector(elCfg)
		log.Infof("eventlog: reconfigured via fleet policy (%d channels, enabled=%v)", len(elCfg.Channels), elCfg.Enabled)
	})
	client.StartCommandLoop(executor)

	// Load persisted event log policy from disk (survives reboot)
	if policyData, err := os.ReadFile(filepath.Join(dataDir, "eventlog-policy.json")); err == nil {
		if elCfg, err := parseEventLogPolicyJSON(policyData); err == nil && elCfg.Enabled {
			f.evs.StartEventLogCollector(elCfg)
			log.Infof("eventlog: loaded persisted policy (%d channels)", len(elCfg.Channels))
		}
	}

	log.Infof("fleet: connected to %s", cfg.Fleet.ServerURL)
	return nil
}

func (f *App) stop() {
	if f.signals != nil {
		f.signals <- struct{}{}
	}
}

// isSingleInstance checks if there is a single instance
// of the Fibratus process running in the system. This is
// accomplished by creating a global event object. If such
// an object already exists, we can conclude Fibratus process
// is already running.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func loadFileContent(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (f *App) isSingleInstance() bool {
	name, err := windows.UTF16PtrFromString("Global\\Fibratus")
	if err != nil {
		return false
	}
	event, err := windows.CreateEvent(nil, 0, 0, name)
	return event != 0 && !errors.Is(err, windows.ERROR_ALREADY_EXISTS)
}

// parseEventLogPolicyJSON converts the raw fleet policy JSON into an eventlog.Config.
func parseEventLogPolicyJSON(data []byte) (eventlog.Config, error) {
	var policy fleet.EventLogPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return eventlog.Config{}, err
	}
	channels := make([]eventlog.ChannelConfig, len(policy.Channels))
	for i, ch := range policy.Channels {
		channels[i] = eventlog.ChannelConfig{
			Name:       ch.Name,
			CollectAll: ch.CollectAll,
			EventIDs:   ch.EventIDs,
		}
	}
	return eventlog.Config{
		Enabled:  policy.Enabled,
		Channels: channels,
	}, nil
}
