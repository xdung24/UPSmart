package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"upsmart/internal/config"
	"upsmart/internal/control"
	"upsmart/internal/events"
	"upsmart/internal/hardware"
	"upsmart/internal/notifications"
	"upsmart/internal/platform"
	"upsmart/internal/policy"
	"upsmart/internal/state"
	"upsmart/internal/ups"
	"upsmart/internal/web"
)

func Run(parent context.Context, cfg config.Config, worker ups.Worker, ready func()) error {
	return run(parent, cfg, worker, ready, ups.AcquireOwner)
}

func run(parent context.Context, cfg config.Config, worker ups.Worker, ready func(), acquire func() (func(), error)) error {
	if err := parent.Err(); err != nil {
		return err
	}
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
	release, err := acquire()
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

func shutdownMode(cfg config.Config) string {
	if cfg.PowerControl == nil || !cfg.PowerControl.Enabled() {
		return "disabled"
	}
	return cfg.PowerControl.Mode
}
