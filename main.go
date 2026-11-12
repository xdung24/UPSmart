package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	"upsmart/internal/config"
	"upsmart/internal/control"
	"upsmart/internal/events"
	"upsmart/internal/hardware"
	"upsmart/internal/notifications"
	"upsmart/internal/platform"
	"upsmart/internal/policy"
	"upsmart/internal/service"
	"upsmart/internal/state"
	"upsmart/internal/ups"
	"upsmart/internal/web"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		log.Print(err)
		os.Exit(1)
	}
}

func run(args []string) error {
	return runWithMonitor(args, serve)
}

func runWithMonitor(args []string, monitor func(context.Context, config.Config, ups.Worker, func()) error) error {
	if len(args) == 2 && args[0] == "__hid" {
		if args[1] != "discover" && args[1] != "status" {
			return errors.New("unsupported helper operation")
		}
		result, err := ups.NativeOperation(args[1] == "status")
		if err != nil {
			result.Error = err.Error()
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	if len(args) == 0 {
		args = []string{"serve", "--config", "config.local.json"}
	}
	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Print(`UPSmart Monitor - ARES UPS monitoring

Usage:
  upsmart                              Start monitoring with .\config.local.json
  upsmart serve --config PATH           Start with a chosen configuration
  upsmart validate --config PATH        Validate configuration without starting
  upsmart discover                     List matching USB devices
  upsmart read-status                  Read one UPS status sample
  upsmart read-hardware                Read PC metrics without accessing the UPS
  upsmart service install --config PATH Install Windows service (Administrator)
  upsmart service start                Start the installed service
  upsmart service stop                 Stop the installed service
  upsmart service status               Show installed service status
  upsmart service uninstall            Remove stopped service; preserve data
  upsmart --help                       Show this help

Desktop UI: run upsmart-desktop.exe alongside a desktop-enabled monitor/service.
Closing the UI never stops the backend. The optional web dashboard stays read-only.

With no arguments, config.local.json is resolved from the current directory.
Use "serve --config PATH" for another location. Missing or invalid configuration
is an error; no alternative configuration is selected silently.
Explicit serve/validate without --config use:
  %ProgramData%\UPSmartMonitor\config.json

Close legacy UPSmart before live monitoring. The configuration controls whether
USB polling is enabled. PC shutdown is opt-in (dry_run/live); desktop controls use
account-restricted local IPC. Telegram is read-only. UPS output control is not implemented.
"upsmart service run" is reserved for Windows Service Control Manager.
`)
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	worker := ups.Worker{Executable: executable}
	switch args[0] {
	case "read-hardware":
		if len(args) != 1 {
			return errors.New("read-hardware accepts no extra arguments")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		sample, collectErr := hardware.Collect(ctx)
		return errors.Join(collectErr, json.NewEncoder(os.Stdout).Encode(sample))
	case "service":
		if len(args) < 2 {
			return errors.New("service action required: install, start, stop, status, uninstall, run")
		}
		action := args[1]
		if action == "install" || action == "run" {
			path, err := configFlag("service "+action, args[2:])
			if err != nil {
				return err
			}
			if action == "install" {
				return service.Manage(action, path)
			}
			return service.Run(func(ctx context.Context, ready func()) error {
				cfg, err := config.Load(path)
				if err != nil {
					return err
				}
				return monitor(ctx, cfg, worker, ready)
			})
		}
		if len(args) != 2 {
			return errors.New("service management action accepts no extra arguments")
		}
		return service.Manage(action, "")
	case "discover", "read-status":
		if len(args) != 1 {
			return errors.New("discover/read-status accept no extra arguments")
		}
		release, err := ups.AcquireOwner()
		if err != nil {
			return err
		}
		defer release()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result, err := worker.Run(ctx, args[0] == "read-status")
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	case "validate", "serve":
		path, err := configFlag(args[0], args[1:])
		if err != nil {
			return err
		}
		cfg, err := config.Load(path)
		if err != nil {
			return err
		}
		if args[0] == "validate" {
			fmt.Printf("Configuration valid; PC shutdown: %s; UPS output control unavailable\n", shutdownMode(cfg))
			return nil
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		return monitor(ctx, cfg, worker, nil)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func shutdownMode(cfg config.Config) string {
	if cfg.PowerControl == nil || !cfg.PowerControl.Enabled() {
		return "disabled"
	}
	return cfg.PowerControl.Mode
}

func configFlag(command string, args []string) (string, error) {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	defaultPath := filepath.Join(os.Getenv("ProgramData"), "UPSmartMonitor", "config.json")
	path := flags.String("config", defaultPath, "absolute or relative configuration path")
	if err := flags.Parse(args); err != nil {
		return "", err
	}
	if flags.NArg() != 0 {
		return "", errors.New("unexpected positional arguments")
	}
	return filepath.Abs(*path)
}

func serve(parent context.Context, cfg config.Config, worker ups.Worker, ready func()) error {
	var effectiveErr error
	cfg, effectiveErr = cfg.Effective()
	if effectiveErr != nil {
		return fmt.Errorf("configuration: %w", effectiveErr)
	}
	var sender *notifications.Sender
	if cfg.Telegram != nil {
		var err error
		sender, err = notifications.New(*cfg.Telegram)
		if err != nil {
			return fmt.Errorf("initialize Telegram: %w", err)
		}
	}
	var hardwareInterval time.Duration
	if cfg.Hardware != nil {
		var err error
		hardwareInterval, err = cfg.Hardware.Duration()
		if err != nil {
			return err
		}
	}
	release, err := ups.AcquireOwner()
	if err != nil {
		return err
	}
	defer release()
	if cfg.MonitoringEnabled {
		if err := ups.CheckLegacy(); err != nil {
			return err
		}
	}
	var listener net.Listener
	if cfg.WebEnabled() {
		listener, err = net.Listen("tcp4", cfg.Listen)
		if err != nil {
			return fmt.Errorf("bind local dashboard: %w", err)
		}
		defer listener.Close()
	}
	var pipeListener net.Listener
	if cfg.Desktop != nil {
		pipeListener, err = control.Listen(*cfg.Desktop)
		if err != nil {
			return fmt.Errorf("desktop IPC: %w", err)
		}
		defer pipeListener.Close()
	}
	poll, timeout, stale := cfg.Durations()
	var journal *events.Journal
	var history []events.Record
	var historyErr error
	if cfg.EventHistory != nil {
		journal, history, historyErr = events.Open(*cfg.EventHistory)
		if historyErr != nil {
			log.Printf("Event history initialization failed; monitoring continues in degraded mode until restart: %v", historyErr)
		}
	}
	var eventSink func(events.Record)
	if sender != nil {
		eventSink = sender.Submit
	}
	store := state.NewWithEventSink(stale, cfg.MonitoringEnabled, journal, history, historyErr, eventSink)
	closeOnInitError := func(initErr error) error {
		if journal == nil {
			return initErr
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		return errors.Join(initErr, journal.Close(cleanup))
	}
	var powerOptions policy.Options
	if cfg.PowerControl != nil {
		powerOptions = *cfg.PowerControl
	}
	controller, err := policy.New(powerOptions, store.PolicyInput, platform.Shutdown, store.UpdatePolicy)
	if err != nil {
		return closeOnInitError(fmt.Errorf("initialize shutdown policy: %w", err))
	}
	store.UpdatePolicy(controller.Status(), "shutdown_state_changed")
	var commands *notifications.Commands
	if cfg.Telegram != nil && len(cfg.Telegram.AllowedUserIDs) > 0 {
		commands, err = notifications.NewCommands(sender, cfg.Telegram.AllowedUserIDs, func(name string) string {
			if name == "/hardware" {
				data, encodeErr := json.Marshal(store.HardwareSnapshot(time.Now()))
				if encodeErr != nil {
					return "Hardware snapshot could not be encoded"
				}
				return "UPSmart hardware (cached)\n" + string(data)
			}
			snapshot := store.Snapshot(time.Now())
			text := fmt.Sprintf("UPSmart\nConnection: %s\nPower: %s\nPC policy: %s\n", snapshot.Connection.State, snapshot.Power.Mode, snapshot.Policy.State)
			if snapshot.Telemetry == nil {
				return text + "No telemetry available."
			}
			text += fmt.Sprintf("Sample: %s; stale: %t\n", snapshot.Telemetry.AcquiredAt.Format(time.RFC3339), snapshot.Telemetry.Stale)
			for _, key := range []string{"input_voltage", "output_voltage", "input_frequency", "load", "battery_voltage", "ups_temperature"} {
				m := snapshot.Telemetry.Measurements[key]
				if m.Value != nil {
					text += fmt.Sprintf("%s: %.1f %s\n", key, *m.Value, m.Unit)
				}
			}
			return text
		})
		if err != nil {
			return closeOnInitError(fmt.Errorf("initialize Telegram commands: %w", err))
		}
		store.SetCommandStatus(commands.Status)
	}
	if sender != nil {
		store.SetTelegramStatus(sender.Status)
	}
	if cfg.Hardware != nil {
		store.EnableHardware(hardwareInterval)
	}
	server := web.NewServer(cfg.Listen, store)
	var pipeServer *http.Server
	if cfg.Desktop != nil {
		bridge := control.NewBridge(store, controller, func(options policy.Options) error {
			next := cfg
			next.PowerControl = &options
			if err := next.Validate(); err != nil {
				return err
			}
			return control.SavePolicy(cfg.Desktop.SettingsFile, options)
		})
		pipeServer = &http.Server{Handler: bridge, ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second,
			IdleTimeout: 10 * time.Second, MaxHeaderBytes: 8192}
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var workers sync.WaitGroup
	if sender != nil {
		workers.Go(func() { sender.Run(ctx) })
	}
	if commands != nil {
		workers.Go(func() { commands.Run(ctx) })
	}
	if powerOptions.Enabled() || cfg.Desktop != nil {
		workers.Go(func() {
			ticker := time.NewTicker(250 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					controller.Tick(ctx, time.Now())
				}
			}
		})
	}
	if cfg.Hardware != nil {
		workers.Go(func() {
			hardware.Run(ctx, hardwareInterval, func(sample hardware.Snapshot, err error) {
				store.UpdateHardware(time.Now(), sample, err)
			})
		})
	}
	if cfg.MonitoringEnabled {
		workers.Go(func() {
			state.Poll(ctx, store, func(ctx context.Context) (ups.Result, error) {
				return worker.Run(ctx, true)
			}, poll, timeout)
		})
	}
	log.Printf("Monitoring: %t; web enabled: %t (%s); desktop IPC: %t; PC shutdown: %s; UPS output control unavailable", cfg.MonitoringEnabled, cfg.WebEnabled(), cfg.Listen, cfg.Desktop != nil, shutdownMode(cfg))
	httpDone := make(chan error, 2)
	if listener != nil {
		go func() { httpDone <- server.Serve(listener) }()
	}
	if pipeServer != nil {
		go func() { httpDone <- pipeServer.Serve(pipeListener) }()
	}
	if ready != nil {
		ready()
	}
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-httpDone:
	}
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer shutdownCancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	if pipeServer != nil {
		if err := pipeServer.Shutdown(shutdownCtx); err != nil {
			shutdownErr = errors.Join(shutdownErr, err, pipeServer.Close())
		}
	}
	if shutdownErr != nil {
		if err := server.Close(); err != nil {
			log.Printf("close HTTP server: %v", err)
		}
	}
	workers.Wait()
	store.Stop(time.Now())
	var journalErr error
	if journal != nil {
		journalErr = journal.Close(shutdownCtx)
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return errors.Join(fmt.Errorf("HTTP server: %w", serveErr), shutdownErr, journalErr)
	}
	return errors.Join(shutdownErr, journalErr)
}
