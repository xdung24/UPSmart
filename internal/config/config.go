package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"time"

	"upsmart/internal/control"
	"upsmart/internal/events"
	"upsmart/internal/notifications"
	"upsmart/internal/policy"
)

type Config struct {
	SchemaVersion      int                    `json:"schema_version"`
	Listen             string                 `json:"listen"`
	PollInterval       string                 `json:"poll_interval"`
	TransactionTimeout string                 `json:"transaction_timeout"`
	StaleAfter         string                 `json:"stale_after"`
	MonitoringEnabled  bool                   `json:"monitoring_enabled"`
	EventHistory       *events.Options        `json:"event_history,omitempty"`
	Hardware           *HardwareOptions       `json:"hardware,omitempty"`
	Telegram           *notifications.Options `json:"telegram,omitempty"`
	PowerControl       *policy.Options        `json:"power_control,omitempty"`
	Desktop            *control.Options       `json:"desktop,omitempty"`
	Web                *bool                  `json:"web_enabled,omitempty"`
}

type HardwareOptions struct {
	Interval string `json:"interval"`
}

func (o HardwareOptions) Duration() (time.Duration, error) {
	return duration("hardware.interval", o.Interval, 10*time.Second, 5*time.Minute)
}

func Load(path string) (Config, error) {
	var c Config
	f, err := os.Open(path)
	if err != nil {
		return c, fmt.Errorf("open config (copy config.example.json to a local config first): %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return c, fmt.Errorf("inspect config: %w", err)
	}
	if info.Size() > 65536 {
		return c, errors.New("config exceeds 64 KiB")
	}
	d := json.NewDecoder(io.LimitReader(f, 65537))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, fmt.Errorf("decode config: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return c, errors.New("config must contain exactly one JSON object")
	}
	return c.Effective()
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
	host, port, err := net.SplitHostPort(c.Listen)
	if err != nil || host != "127.0.0.1" {
		return errors.New("listen must be 127.0.0.1:<port>; LAN access is not supported")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return errors.New("listen port must be between 1 and 65535")
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
	if !c.WebEnabled() && c.Desktop == nil {
		return errors.New("web_enabled:false requires desktop IPC")
	}
	return nil
}

func (c Config) WebEnabled() bool { return c.Web == nil || *c.Web }

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
