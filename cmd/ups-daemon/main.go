package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"upsmart/internal/config"
	"upsmart/internal/hardware"
	"upsmart/internal/monitor"
	"upsmart/internal/service"
	"upsmart/internal/ups"
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
	if handled, err := ups.HandleHelper(args, os.Stdout); handled {
		return err
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

Desktop UI: automatically tries the daemon named pipe, then direct USB.
The daemon can run with "serve"; no installed Windows service is required.
Quitting stops only desktop-owned monitoring, never a daemon.
Configure "desktop" IPC in the daemon configuration for desktop controls.

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
	return monitor.Run(parent, cfg, worker, ready)
}
