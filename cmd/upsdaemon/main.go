package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/thought-machine/go-flags"

	"upsmart/internal/config"
	"upsmart/internal/hardware"
	"upsmart/internal/monitor"
	"upsmart/internal/service"
	"upsmart/internal/ups"
)

var Version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && flagsErr.Type == flags.ErrHelp {
			return
		}
		log.Print(err)
		if !service.Interactive() {
			if logErr := service.LogError(err); logErr != nil {
				log.Printf("Windows Event Log error reporting failed: %v", logErr)
			}
		}
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
	if len(args) > 0 {
		switch args[0] {
		case "version", "-v", "--version":
			if len(args) != 1 {
				return errors.New("version accepts no extra arguments")
			}
			fmt.Printf("upsdaemon_%s\n", Version)
			return nil
		case "help":
			args = append([]string{"--help"}, args[1:]...)
		case "serve":
			args = args[1:]
		case "validate":
			args = append([]string{"--validate"}, args[1:]...)
		case "service":
			return errors.New("use --install, --remove, --start, --stop or --status instead of service subcommands")
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	worker := ups.Worker{Executable: executable}
	if len(args) > 0 {
		switch args[0] {
		case "read-hardware":
			if len(args) != 1 {
				return errors.New("read-hardware accepts no extra arguments")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			sample, collectErr := hardware.Collect(ctx)
			return errors.Join(collectErr, json.NewEncoder(os.Stdout).Encode(sample))
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
		}
	}
	options, err := config.Parse(args, os.Stdout)
	if err != nil {
		return err
	}
	action, err := options.Action()
	if err != nil {
		return err
	}
	if action == "remove" || action == "start" || action == "stop" || action == "status" {
		return service.Manage(action, options.ConfigFile, config.Config{})
	}
	if action == "install" {
		return service.Manage(action, options.ConfigFile, options.Parameters.Config())
	}
	cfg, err := options.Parameters.Config().Effective()
	if err != nil {
		return err
	}
	switch action {
	case "validate":
		fmt.Printf("Configuration valid (%s); PC shutdown: %s; UPS output control unavailable\n", options.ConfigFile, shutdownMode(cfg))
		return nil
	}
	if !service.Interactive() {
		return service.Run(options.ConfigFile, func(ctx context.Context, ready func()) error {
			return monitor(ctx, cfg, worker, ready)
		})
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return monitor(ctx, cfg, worker, nil)
}

func shutdownMode(cfg config.Config) string {
	if cfg.PowerControl == nil || !cfg.PowerControl.Enabled() {
		return "disabled"
	}
	return cfg.PowerControl.Mode
}

func serve(parent context.Context, cfg config.Config, worker ups.Worker, ready func()) error {
	return monitor.Run(parent, cfg, worker, ready)
}
