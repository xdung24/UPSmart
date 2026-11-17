package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/thought-machine/go-flags"

	"upsmart/internal/control"
	"upsmart/internal/events"
	"upsmart/internal/notifications"
	"upsmart/internal/policy"
)

type Config struct {
	SchemaVersion      int                    `json:"schema_version" yaml:"schema_version"`
	PollInterval       string                 `json:"poll_interval" yaml:"poll_interval"`
	TransactionTimeout string                 `json:"transaction_timeout" yaml:"transaction_timeout"`
	StaleAfter         string                 `json:"stale_after" yaml:"stale_after"`
	MonitoringEnabled  bool                   `json:"monitoring_enabled" yaml:"monitoring_enabled"`
	EventHistory       *events.Options        `json:"event_history,omitempty" yaml:"event_history,omitempty"`
	Hardware           *HardwareOptions       `json:"hardware,omitempty" yaml:"hardware,omitempty"`
	Telegram           *notifications.Options `json:"telegram,omitempty" yaml:"telegram,omitempty"`
	PowerControl       *policy.Options        `json:"power_control,omitempty" yaml:"power_control,omitempty"`
	Desktop            *control.Options       `json:"desktop,omitempty" yaml:"desktop,omitempty"`
}

type HardwareOptions struct {
	Interval string `json:"interval" yaml:"interval"`
}

func (o HardwareOptions) Duration() (time.Duration, error) {
	return duration("hardware.interval", o.Interval, 10*time.Second, 5*time.Minute)
}

func Load(path string) (Config, error) {
	args, err := argumentsFromFile(path)
	if err != nil {
		return Config{}, err
	}
	var parameters Parameters
	if _, err := flags.NewParser(&parameters, flags.None).ParseArgs(args); err != nil {
		return Config{}, err
	}
	return parameters.Config().Effective()
}

func (c Config) Effective() (Config, error) {
	if err := c.Validate(); err != nil {
		return c, err
	}
	if c.Desktop != nil {
		saved, err := control.LoadPolicy(c.Desktop.SettingsFile)
		if err != nil {
			return c, err
		}
		if saved != nil {
			c.PowerControl = saved
			if err := c.Validate(); err != nil {
				return c, fmt.Errorf("saved desktop policy: %w", err)
			}
		}
	}
	return c, nil
}

func (c Config) Validate() error {
	if c.SchemaVersion != 1 {
		return errors.New("schema_version must be 1")
	}
	poll, err := duration("poll_interval", c.PollInterval, time.Second, time.Minute)
	if err != nil {
		return err
	}
	timeout, err := duration("transaction_timeout", c.TransactionTimeout, 100*time.Millisecond, 30*time.Second)
	if err != nil {
		return err
	}
	stale, err := duration("stale_after", c.StaleAfter, time.Second, 5*time.Minute)
	if err != nil {
		return err
	}
	if timeout >= poll {
		return errors.New("transaction_timeout must be less than poll_interval")
	}
	if stale <= poll+timeout {
		return errors.New("stale_after must exceed poll_interval + transaction_timeout")
	}
	if c.EventHistory != nil {
		if err := c.EventHistory.Validate(); err != nil {
			return err
		}
	}
	if c.Hardware != nil {
		if _, err := c.Hardware.Duration(); err != nil {
			return err
		}
	}
	if c.Telegram != nil {
		if err := c.Telegram.Validate(); err != nil {
			return fmt.Errorf("telegram: %w", err)
		}
	}
	if c.PowerControl != nil {
		if err := c.PowerControl.Validate(); err != nil {
			return fmt.Errorf("power_control: %w", err)
		}
		if c.PowerControl.AutomaticEnabled() && !c.MonitoringEnabled {
			return errors.New("automatic shutdown requires monitoring_enabled")
		}
		if c.PowerControl.ManualEnabled() && c.Desktop == nil {
			return errors.New("manual shutdown requires desktop IPC configuration; Telegram is read-only")
		}
	}
	if c.Desktop != nil {
		if err := c.Desktop.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func duration(name, value string, min, max time.Duration) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil || d < min || d > max {
		return 0, fmt.Errorf("%s must be a duration between %s and %s", name, min, max)
	}
	return d, nil
}

func (c Config) Durations() (poll, timeout, stale time.Duration) {
	// Callers validate the configuration before starting workers.
	poll, _ = time.ParseDuration(c.PollInterval)
	timeout, _ = time.ParseDuration(c.TransactionTimeout)
	stale, _ = time.ParseDuration(c.StaleAfter)
	return
}
