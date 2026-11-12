// Package policy implements opt-in PC shutdown decisions without accessing hardware.
package policy

import (
	"fmt"
	"time"
)

type Options struct {
	Mode      string            `json:"mode"`
	Automatic *AutomaticOptions `json:"automatic,omitempty"`
	Manual    *ManualOptions    `json:"manual,omitempty"`
}

type AutomaticOptions struct {
	Enabled     bool   `json:"enabled"`
	OutageAfter string `json:"outage_after,omitempty"`
	LowBattery  bool   `json:"low_battery"`
	Countdown   string `json:"countdown"`
}

type ManualOptions struct {
	Enabled   bool   `json:"enabled"`
	Countdown string `json:"countdown"`
	// Retained for old configuration files; desktop access uses Windows SID ACLs.
	AllowedUserIDs []int64 `json:"allowed_user_ids,omitempty"`
}

func (o Options) AutomaticEnabled() bool { return o.Automatic != nil && o.Automatic.Enabled }
func (o Options) ManualEnabled() bool    { return o.Manual != nil && o.Manual.Enabled }
func (o Options) Enabled() bool          { return o.AutomaticEnabled() || o.ManualEnabled() }

func (o Options) Validate() error {
	if o.Mode != "" && o.Mode != "dry_run" && o.Mode != "live" {
		return fmt.Errorf("policy mode must be dry_run or live")
	}
	if o.Enabled() && o.Mode == "" {
		return fmt.Errorf("enabled policy requires explicit dry_run or live mode")
	}
	if o.Automatic != nil {
		if err := o.Automatic.Validate(); err != nil {
			return err
		}
	}
	if o.Manual != nil {
		return o.Manual.Validate()
	}
	return nil
}

func (o AutomaticOptions) Validate() error {
	if !o.Enabled {
		return nil
	}
	if o.OutageAfter == "" && !o.LowBattery {
		return fmt.Errorf("automatic policy requires outage_after or low_battery")
	}
	if o.OutageAfter != "" {
		if _, err := duration(o.OutageAfter, time.Second, 24*time.Hour); err != nil {
			return fmt.Errorf("automatic outage_after: %w", err)
		}
	}
	if _, err := duration(o.Countdown, 5*time.Second, 10*time.Minute); err != nil {
		return fmt.Errorf("automatic countdown: %w", err)
	}
	return nil
}

func (o ManualOptions) Validate() error {
	if !o.Enabled {
		return nil
	}
	if _, err := duration(o.Countdown, 5*time.Second, 10*time.Minute); err != nil {
		return fmt.Errorf("manual countdown: %w", err)
	}
	if len(o.AllowedUserIDs) > 16 {
		return fmt.Errorf("legacy manual user IDs cannot exceed 16; desktop authorization uses Windows IPC")
	}
	seen := make(map[int64]bool, len(o.AllowedUserIDs))
	for _, id := range o.AllowedUserIDs {
		if id <= 0 || seen[id] {
			return fmt.Errorf("manual allowed user IDs must be positive and unique")
		}
		seen[id] = true
	}
	return nil
}

func duration(value string, min, max time.Duration) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil || d < min || d > max {
		return 0, fmt.Errorf("duration must be explicit and between %s and %s", min, max)
	}
	return d, nil
}
