package policy

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"time"
)

type Input struct {
	Fresh      bool
	OnBattery  bool
	LowBattery bool
	OutageID   string
}

type Status struct {
	Enabled            bool       `json:"enabled"`
	UPSPowerOff        bool       `json:"ups_power_off_enabled"`
	AutomaticEnabled   bool       `json:"automatic_enabled"`
	ManualEnabled      bool       `json:"manual_enabled"`
	OutageAfter        string     `json:"outage_after,omitempty"`
	LowBatteryEnabled  bool       `json:"low_battery_enabled"`
	AutomaticCountdown string     `json:"automatic_countdown,omitempty"`
	ManualCountdown    string     `json:"manual_countdown,omitempty"`
	DryRun             bool       `json:"dry_run"`
	State              string     `json:"state"`
	BlockingReason     string     `json:"blocking_reason"`
	Trigger            *string    `json:"trigger"`
	Deadline           *time.Time `json:"deadline"`
	Outcome            *string    `json:"outcome"`
}

// Controller serializes commands and dispatch. Callbacks must not reenter it.
// read/publish must be quick in-memory operations; request should honor ctx.
// Authentication and confirmation of manual commands belong to the caller.
type Controller struct {
	mu      sync.Mutex
	options Options
	read    func(time.Time) Input
	request func(context.Context) error
	publish func(Status, string)
	status  Status

	outageAfter, automaticCountdown, manualCountdown  time.Duration
	outageStart                                       time.Time
	outageID                                          string
	tracking, pendingSawBattery, suppressed, terminal bool
}

func New(options Options, read func(time.Time) Input, request func(context.Context) error, publish func(Status, string)) (*Controller, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	if options.Enabled() && read == nil {
		return nil, errors.New("enabled policy requires an input reader")
	}
	if options.Enabled() && options.Mode == "live" && request == nil {
		return nil, errors.New("live policy requires a shutdown requester")
	}
	c := &Controller{options: options, read: read, request: request, publish: publish}
	// Own option values so callers cannot mutate active policy after validation.
	if options.Automatic != nil {
		a := *options.Automatic
		c.options.Automatic = &a
		c.outageAfter, _ = time.ParseDuration(a.OutageAfter)
		c.automaticCountdown, _ = time.ParseDuration(a.Countdown)
	}
	if options.Manual != nil {
		m := *options.Manual
		m.AllowedUserIDs = append([]int64(nil), m.AllowedUserIDs...)
		c.options.Manual = &m
		c.manualCountdown, _ = time.ParseDuration(m.Countdown)
	}
	c.status = Status{
		Enabled: options.Enabled(), AutomaticEnabled: options.AutomaticEnabled(),
		ManualEnabled: options.ManualEnabled(), DryRun: options.Mode == "dry_run", State: "disabled",
	}
	if options.AutomaticEnabled() {
		c.status.OutageAfter = c.options.Automatic.OutageAfter
		c.status.LowBatteryEnabled = c.options.Automatic.LowBattery
		c.status.AutomaticCountdown = c.options.Automatic.Countdown
	}
	if options.ManualEnabled() {
		c.status.ManualCountdown = c.options.Manual.Countdown
	}
	if c.status.Enabled {
		c.status.State = "armed"
	}
	return c, nil
}

func (c *Controller) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return clone(c.status)
}

func (c *Controller) Configuration() Options {
	c.mu.Lock()
	defer c.mu.Unlock()
	return copyOptions(c.options)
}

func copyOptions(value Options) Options {
	if value.Automatic != nil {
		copy := *value.Automatic
		value.Automatic = &copy
	}
	if value.Manual != nil {
		copy := *value.Manual
		copy.AllowedUserIDs = append([]int64(nil), copy.AllowedUserIDs...)
		value.Manual = &copy
	}
	return value
}

func (c *Controller) Reconfigure(options Options, persist func() error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending() || c.terminal {
		return errors.New("cancel a pending countdown first; terminal attempts require process restart")
	}
	next, err := New(options, c.read, c.request, c.publish)
	if err != nil {
		return err
	}
	if persist == nil {
		return errors.New("policy persistence is required")
	}
	if err := persist(); err != nil {
		return err
	}
	c.options, c.status = next.options, next.status
	c.outageAfter, c.automaticCountdown, c.manualCountdown = next.outageAfter, next.automaticCountdown, next.manualCountdown
	c.outageStart, c.outageID = time.Time{}, ""
	c.tracking, c.pendingSawBattery, c.suppressed = false, false, false
	if c.publish != nil {
		c.publish(clone(c.status), "shutdown_state_changed")
	}
	return nil
}

func clone(s Status) Status {
	if s.Trigger != nil {
		v := *s.Trigger
		s.Trigger = &v
	}
	if s.Deadline != nil {
		v := *s.Deadline
		s.Deadline = &v
	}
	if s.Outcome != nil {
		v := *s.Outcome
		s.Outcome = &v
	}
	return s
}

func (c *Controller) change(state, reason string, trigger *string, deadline *time.Time, outcome *string, event string) {
	next := c.status
	next.State, next.BlockingReason = state, reason
	next.Trigger, next.Deadline, next.Outcome = trigger, deadline, outcome
	if reflect.DeepEqual(next, c.status) {
		return
	}
	c.status = clone(next)
	if c.publish != nil {
		c.publish(clone(c.status), event)
	}
}

func text(s string) *string { return &s }

func (c *Controller) pending() bool { return c.status.State == "pending" }
func (c *Controller) automaticPending() bool {
	return c.pending() && c.status.Trigger != nil && *c.status.Trigger != "manual"
}

func (c *Controller) cancel(reason, state string) {
	c.pendingSawBattery = false
	c.change(state, reason, c.status.Trigger, nil, text(reason), "shutdown_cancelled")
}

// observe uses only process-local now, never persisted or incoming sample times.
// A manual countdown survives unavailable telemetry, but not verified restoration
// after it has actually observed battery operation.
func (c *Controller) observe(in Input, now time.Time) bool {
	if !in.Fresh {
		c.tracking = false
		c.outageStart = time.Time{}
		if c.automaticPending() {
			c.cancel("input_unavailable", "blocked")
			return true
		}
		return false
	}
	if !in.OnBattery {
		c.tracking = false
		c.outageStart = time.Time{}
		c.suppressed = false
		if c.pending() && c.pendingSawBattery {
			c.cancel("mains_restored", "cancelled")
			return true
		}
		return false
	}
	changed := c.tracking && c.outageID != in.OutageID
	if !c.tracking || changed {
		c.outageID, c.outageStart, c.tracking = in.OutageID, now, true
	}
	if c.pending() {
		c.pendingSawBattery = true
	}
	if changed && c.automaticPending() {
		c.cancel("outage_changed", "cancelled")
		return true
	}
	return false
}

func (c *Controller) qualifies(in Input, now time.Time) string {
	if !in.Fresh || !in.OnBattery || !c.tracking || c.suppressed {
		return ""
	}
	if c.options.Automatic.LowBattery && in.LowBattery {
		return "low_battery"
	}
	if c.outageAfter > 0 && now.Sub(c.outageStart) >= c.outageAfter {
		return "outage"
	}
	return ""
}

func (c *Controller) schedule(trigger string, now time.Time, countdown time.Duration, in Input) {
	deadline := now.Add(countdown)
	c.pendingSawBattery = in.Fresh && in.OnBattery
	c.change("pending", "", text(trigger), &deadline, nil, "shutdown_pending")
}

// Tick dispatches at most once in this controller's lifetime. Terminal outcomes
// are latched; a rejected OS request is never automatically retried.
func (c *Controller) Tick(ctx context.Context, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.status.Enabled || c.terminal || ctx.Err() != nil {
		return
	}
	in := c.read(now)
	if c.observe(in, now) {
		return
	}
	if c.automaticPending() {
		if !c.checkPending(in, now) {
			return
		}
	}
	if !c.pending() {
		if !c.options.AutomaticEnabled() {
			return
		}
		if !in.Fresh {
			c.change("blocked", "input_unavailable", nil, nil, nil, "shutdown_state_changed")
			return
		}
		if c.suppressed {
			c.change("cancelled", "explicit_cancel", nil, nil, text("explicit_cancel"), "shutdown_state_changed")
			return
		}
		if trigger := c.qualifies(in, now); trigger != "" {
			c.schedule(trigger, now, c.automaticCountdown, in)
		} else {
			c.change("armed", "", nil, nil, nil, "shutdown_state_changed")
		}
		return
	}
	if now.Before(*c.status.Deadline) {
		return
	}
	if c.automaticPending() {
		// Read again at the dispatch boundary: the snapshot may have changed
		// since evaluation, even though policy commands are serialized.
		in = c.read(now)
		if c.observe(in, now) || !c.checkPending(in, now) {
			return
		}
		if now.Before(*c.status.Deadline) {
			return
		}
	}
	if ctx.Err() != nil {
		return
	}
	c.terminal = true
	if c.status.DryRun {
		c.change("dry_run", "", c.status.Trigger, nil, text("would_request"), "shutdown_dry_run")
		return
	}
	c.change("requesting", "", c.status.Trigger, nil, nil, "shutdown_requesting")
	if err := ctx.Err(); err != nil {
		c.change("failed", "", c.status.Trigger, nil, text(err.Error()), "shutdown_failed")
		return
	}
	if err := c.request(ctx); err != nil {
		c.change("failed", "", c.status.Trigger, nil, text(err.Error()), "shutdown_failed")
	} else {
		c.change("requested", "", c.status.Trigger, nil, text("accepted"), "shutdown_requested")
	}
}

func (c *Controller) checkPending(in Input, now time.Time) bool {
	trigger := c.qualifies(in, now)
	if trigger == "" {
		c.cancel("trigger_cleared", "cancelled")
		return false
	}
	if *c.status.Trigger == "low_battery" && !in.LowBattery {
		// Falling back to the duration trigger gets its own full countdown.
		c.schedule("outage", now, c.automaticCountdown, in)
		return false
	}
	return true
}

func (c *Controller) Manual(now time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.options.ManualEnabled() {
		return errors.New("manual shutdown is disabled")
	}
	if c.terminal {
		return errors.New("shutdown outcome is already latched")
	}
	if c.pending() {
		return errors.New("shutdown is already pending")
	}
	in := c.read(now)
	c.observe(in, now)
	c.schedule("manual", now, c.manualCountdown, in)
	return nil
}

func (c *Controller) Cancel(now time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.pending() {
		return errors.New("no cancellable shutdown is pending")
	}
	// Suppression lasts until verified mains, including across disconnects
	// and continuity-ID changes. A cancel button must not immediately rearm.
	c.suppressed = true
	c.cancel("explicit_cancel", "cancelled")
	return nil
}
