package state

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"upsmart/internal/events"
	"upsmart/internal/notifications"
	"upsmart/internal/policy"
	"upsmart/internal/ups"
)

var (
	ErrInvalidCursor = errors.New("invalid event cursor")
	ErrExpiredCursor = errors.New("event cursor is no longer retained; reload history")
)

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Connection struct {
	State       string     `json:"state"`
	LastAttempt *time.Time `json:"last_attempt"`
	LastSuccess *time.Time `json:"last_success"`
	Error       *APIError  `json:"error"`
}

type Telemetry struct {
	Sequence     uint64                     `json:"sequence"`
	AcquiredAt   time.Time                  `json:"acquired_at"`
	Stale        bool                       `json:"stale"`
	Measurements map[string]ups.Measurement `json:"measurements"`
}

type Power struct {
	Mode          string     `json:"mode"`
	LowBattery    *bool      `json:"low_battery"`
	Fault         *bool      `json:"fault"`
	ObservedSince *time.Time `json:"observed_outage_start"`
}

type Policy = policy.Status

type Health struct {
	Warnings         []string                    `json:"warnings"`
	PolicyEligible   bool                        `json:"policy_eligible"`
	EventHistory     events.Status               `json:"event_history"`
	Telegram         notifications.Status        `json:"telegram"`
	TelegramCommands notifications.CommandStatus `json:"telegram_commands"`
}

type Snapshot struct {
	SchemaVersion int         `json:"schema_version"`
	SessionID     string      `json:"service_session_id"`
	GeneratedAt   time.Time   `json:"generated_at"`
	Device        *ups.Device `json:"device"`
	Connection    Connection  `json:"connection"`
	Telemetry     *Telemetry  `json:"telemetry"`
	Power         Power       `json:"power"`
	Policy        Policy      `json:"policy"`
	Health        Health      `json:"health"`
}

type Event = events.Record

type EventPage struct {
	Items      []Event `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

type Store struct {
	mu                 sync.RWMutex
	snapshot           Snapshot
	lastGood           time.Time
	staleAfter         time.Duration
	enabled            bool
	events             []Event
	eventSeq           uint64
	sampleSeq          uint64
	journal            *events.Journal
	historyError       string
	eventSink          func(Event)
	hardware           HardwareStatus
	hardwareStaleAfter time.Duration
	hardwareLastSample time.Time
	telegramStatus     func() notifications.Status
	commandStatus      func() notifications.CommandStatus
}

func New(staleAfter time.Duration, enabled bool) *Store {
	return NewWithHistory(staleAfter, enabled, nil, nil, nil)
}

func NewWithHistory(staleAfter time.Duration, enabled bool, journal *events.Journal, history []Event, historyErr error) *Store {
	return NewWithEventSink(staleAfter, enabled, journal, history, historyErr, nil)
}

// The sink must enqueue without blocking; it is called while state is locked.
func NewWithEventSink(staleAfter time.Duration, enabled bool, journal *events.Journal, history []Event, historyErr error, sink func(Event)) *Store {
	s := &Store{staleAfter: staleAfter, enabled: enabled, journal: journal, eventSink: sink}
	if len(history) > events.MemoryLimit {
		history = history[len(history)-events.MemoryLimit:]
	}
	s.events = append([]Event(nil), history...)
	if historyErr != nil {
		s.historyError = historyErr.Error()
	}
	s.snapshot.SchemaVersion = 1
	s.snapshot.SessionID = rand.Text()
	s.snapshot.Connection.State = "disabled"
	if enabled {
		s.snapshot.Connection.State = "discovering"
	}
	s.snapshot.Power.Mode = "unknown"
	s.snapshot.Policy = Policy{State: "disabled", BlockingReason: "PC shutdown is disabled"}
	s.addEvent(time.Now(), "startup", "info", fmt.Sprintf("Application started; live USB polling enabled: %t; PC policy initializes separately; UPS output control unavailable", enabled))
	return s
}

func (s *Store) addEvent(now time.Time, kind, severity, message string) {
	s.eventSeq++
	s.events = append(s.events, Event{
		SchemaVersion: 1, SessionID: s.snapshot.SessionID, Sequence: s.eventSeq,
		Time: now.UTC(), Type: kind, Severity: severity, Message: message,
	})
	if s.journal != nil {
		s.journal.Enqueue(s.events[len(s.events)-1])
	}
	if s.eventSink != nil {
		s.eventSink(s.events[len(s.events)-1])
	}
	if len(s.events) > events.MemoryLimit {
		copy(s.events, s.events[len(s.events)-events.MemoryLimit:])
		s.events = s.events[:events.MemoryLimit]
	}
}

func (s *Store) Stop(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addEvent(now, "stop", "info", "Monitoring stopped normally")
}

func (s *Store) Apply(now time.Time, result ups.Result, readErr error) error {
	var reading ups.Reading
	if readErr == nil {
		if len(result.Devices) != 1 {
			readErr = fmt.Errorf("expected exactly one UPS; found %d", len(result.Devices))
		} else {
			reading, readErr = ups.ParseQ1(result.Status)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	timestamp := now.UTC()
	s.snapshot.Connection.LastAttempt = &timestamp
	if readErr != nil {
		message := readErr.Error()
		if s.snapshot.Connection.Error == nil || s.snapshot.Connection.Error.Message != message {
			s.addEvent(now, "connection_error", "warning", message)
		}
		s.snapshot.Connection.State = "retrying"
		s.snapshot.Connection.Error = &APIError{"device_read_failed", message}
		s.snapshot.Power.ObservedSince = nil
		return readErr
	}
	if s.snapshot.Connection.State != "connected" {
		s.addEvent(now, "connected", "info", "Verified Q1 response received")
	}
	old := s.snapshot.Power
	if old.Mode != reading.Mode {
		s.addEvent(now, "power_changed", "info", "Power mode: "+reading.Mode)
	}
	if (old.LowBattery == nil && reading.LowBattery) || (old.LowBattery != nil && *old.LowBattery != reading.LowBattery) {
		s.addEvent(now, "low_battery_changed", "warning", fmt.Sprintf("Low battery: %t", reading.LowBattery))
	}
	if (old.Fault == nil && reading.Fault) || (old.Fault != nil && *old.Fault != reading.Fault) {
		s.addEvent(now, "fault_changed", "warning", fmt.Sprintf("UPS fault: %t", reading.Fault))
	}
	var since *time.Time
	if reading.Mode == "on_battery" {
		since = old.ObservedSince
		if now.Sub(s.lastGood) >= s.staleAfter {
			since = nil
		}
		if since == nil {
			since = &timestamp
		}
	}
	s.snapshot.Power = Power{reading.Mode, &reading.LowBattery, &reading.Fault, since}
	device := result.Devices[0]
	s.snapshot.Device = &device
	s.snapshot.Connection = Connection{"connected", &timestamp, &timestamp, nil}
	s.lastGood = now
	s.sampleSeq++
	s.snapshot.Telemetry = &Telemetry{s.sampleSeq, timestamp, false, reading.Measurements}
	return nil
}

func (s *Store) Snapshot(now time.Time) Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.snapshot
	out.Device = copyPointer(out.Device)
	if out.Device != nil {
		out.Device.DescriptorWarnings = append([]string(nil), out.Device.DescriptorWarnings...)
		out.Device.Capabilities = append([]string{}, out.Device.Capabilities...)
	}
	out.Connection.LastAttempt = copyPointer(out.Connection.LastAttempt)
	out.Connection.LastSuccess = copyPointer(out.Connection.LastSuccess)
	out.Connection.Error = copyPointer(out.Connection.Error)
	out.Power.LowBattery = copyPointer(out.Power.LowBattery)
	out.Power.Fault = copyPointer(out.Power.Fault)
	out.Power.ObservedSince = copyPointer(out.Power.ObservedSince)
	out.Policy.Trigger = copyPointer(out.Policy.Trigger)
	out.Policy.Deadline = copyPointer(out.Policy.Deadline)
	out.Policy.Outcome = copyPointer(out.Policy.Outcome)
	out.GeneratedAt = now.UTC()
	out.Health.Warnings = []string{"UPS output control is unavailable"}
	if !out.Policy.Enabled {
		out.Health.Warnings = append(out.Health.Warnings, "PC shutdown is disabled")
	} else if out.Policy.DryRun {
		out.Health.Warnings = append(out.Health.Warnings, "PC shutdown is in DRY RUN; no Windows shutdown requests")
	} else {
		out.Health.Warnings = append(out.Health.Warnings, "LIVE PC shutdown enabled; cancellation is only possible before the OS request")
	}
	out.Health.EventHistory = events.Status{Mode: "memory_only"}
	out.Health.Telegram = notifications.Status{Mode: "disabled"}
	out.Health.TelegramCommands = notifications.CommandStatus{Mode: "disabled"}
	if s.commandStatus != nil {
		out.Health.TelegramCommands = s.commandStatus()
		if out.Health.TelegramCommands.Error != "" {
			out.Health.Warnings = append(out.Health.Warnings, "Telegram commands: "+out.Health.TelegramCommands.Error)
		}
	}
	if s.telegramStatus != nil {
		out.Health.Telegram = s.telegramStatus()
		if out.Health.Telegram.Error != "" {
			out.Health.Warnings = append(out.Health.Warnings, "Telegram delivery: "+out.Health.Telegram.Error)
		}
	}
	if s.journal != nil {
		out.Health.EventHistory = s.journal.Status()
	} else if s.historyError != "" {
		out.Health.EventHistory = events.Status{Mode: "degraded", Error: s.historyError}
	}
	history := out.Health.EventHistory
	if history.Mode == "memory_only" {
		out.Health.Warnings = append(out.Health.Warnings, "Event history is memory-only")
	}
	if history.Error != "" {
		out.Health.Warnings = append(out.Health.Warnings, "Event persistence: "+history.Error)
	}
	if history.Warning != "" {
		out.Health.Warnings = append(out.Health.Warnings, history.Warning)
	}
	if history.Unpersisted > 0 {
		out.Health.Warnings = append(out.Health.Warnings, fmt.Sprintf("%d event(s) were not persisted", history.Unpersisted))
	}
	if !s.enabled {
		out.Health.Warnings = append(out.Health.Warnings, "Live USB polling is disabled in configuration")
	}
	if out.Connection.Error != nil {
		out.Health.Warnings = append(out.Health.Warnings, out.Connection.Error.Message)
	}
	if out.Device != nil {
		out.Health.Warnings = append(out.Health.Warnings, out.Device.DescriptorWarnings...)
	}
	if out.Telemetry != nil {
		telemetry := *out.Telemetry
		telemetry.Stale = now.Sub(s.lastGood) >= s.staleAfter
		telemetry.Measurements = make(map[string]ups.Measurement, len(out.Telemetry.Measurements))
		for name, measurement := range out.Telemetry.Measurements {
			if measurement.Value != nil {
				v := *measurement.Value
				measurement.Value = &v
			}
			telemetry.Measurements[name] = measurement
		}
		out.Telemetry = &telemetry
	}
	if out.Telemetry == nil || out.Telemetry.Stale {
		out.Health.Warnings = append(out.Health.Warnings, "No fresh telemetry is available")
	}
	out.Health.PolicyEligible = out.Policy.AutomaticEnabled &&
		(out.Policy.State == "armed" || out.Policy.State == "pending") &&
		out.Connection.State == "connected" && out.Telemetry != nil &&
		!out.Telemetry.Stale && out.Power.Mode == "on_battery"
	return out
}

func (s *Store) PolicyInput(now time.Time) policy.Input {
	s.mu.RLock()
	defer s.mu.RUnlock()
	power := s.snapshot.Power
	input := policy.Input{
		Fresh: s.enabled && s.snapshot.Connection.State == "connected" &&
			!s.lastGood.IsZero() && now.Sub(s.lastGood) < s.staleAfter &&
			(power.Mode == "online" || power.Mode == "on_battery"),
		OnBattery:  power.Mode == "on_battery",
		LowBattery: power.LowBattery != nil && *power.LowBattery,
	}
	if power.ObservedSince != nil {
		input.OutageID = power.ObservedSince.Format(time.RFC3339Nano)
	}
	return input
}

func (s *Store) UpdatePolicy(value policy.Status, kind string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value.Trigger = copyPointer(value.Trigger)
	value.Deadline = copyPointer(value.Deadline)
	value.Outcome = copyPointer(value.Outcome)
	s.snapshot.Policy = value
	message := "PC shutdown policy: " + value.State
	if value.DryRun {
		message += " (DRY RUN)"
	}
	if value.Trigger != nil {
		message += "; trigger: " + *value.Trigger
	}
	if value.Deadline != nil {
		message += "; deadline: " + value.Deadline.UTC().Format(time.RFC3339)
	}
	if value.BlockingReason != "" {
		message += "; " + value.BlockingReason
	}
	if value.Outcome != nil {
		message += "; " + *value.Outcome
	}
	severity := "info"
	if value.State == "failed" {
		severity = "error"
	} else if value.State == "pending" && !value.DryRun {
		severity = "warning"
	}
	s.addEvent(time.Now(), kind, severity, message)
}

func (s *Store) SetCommandStatus(provider func() notifications.CommandStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commandStatus = provider
}

func (s *Store) SetTelegramStatus(provider func() notifications.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.telegramStatus = provider
}

func copyPointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (s *Store) Page(limit int, cursor string) (EventPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	page := EventPage{Items: make([]Event, 0, limit)}
	before := len(s.events)
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return page, ErrInvalidCursor
		}
		session, sequence, ok := strings.Cut(string(raw), ":")
		n, err := strconv.ParseUint(sequence, 10, 64)
		if !ok || err != nil || n == 0 {
			return page, ErrInvalidCursor
		}
		before = -1
		for i := len(s.events) - 1; i >= 0; i-- {
			if s.events[i].SessionID == session && s.events[i].Sequence == n {
				before = i
				break
			}
		}
		if before < 0 {
			return page, ErrExpiredCursor
		}
	}
	for i := before - 1; i >= 0; i-- {
		event := s.events[i]
		if len(page.Items) == limit {
			last := page.Items[len(page.Items)-1]
			cursor := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%d", last.SessionID, last.Sequence)))
			page.NextCursor = &cursor
			break
		}
		page.Items = append(page.Items, event)
	}
	return page, nil
}
