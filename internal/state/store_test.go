package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"upsmart/internal/events"
	"upsmart/internal/hardware"
	"upsmart/internal/policy"
	"upsmart/internal/ups"
)

func TestLiveEventSinkDoesNotReplayHistoryAndReportsInitiallyAssertedFlags(t *testing.T) {
	history := []Event{{SessionID: "previous", Sequence: 1, Type: "startup"}}
	var delivered []Event
	s := NewWithEventSink(time.Second, true, nil, history, nil, func(event Event) {
		delivered = append(delivered, event)
	})
	if len(delivered) != 1 || delivered[0].Type != "startup" || delivered[0].SessionID == "previous" {
		t.Fatalf("startup delivery replayed history: %+v", delivered)
	}
	now := time.Now()
	if err := s.Apply(now, sample("11011000"), nil); err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int)
	for _, event := range delivered {
		counts[event.Type]++
	}
	if counts["low_battery_changed"] != 1 || counts["fault_changed"] != 1 {
		t.Fatalf("initially asserted critical flags were missed: %+v", counts)
	}
	before := len(delivered)
	if err := s.Apply(now.Add(time.Second), sample("11011000"), nil); err != nil {
		t.Fatal(err)
	}
	if len(delivered) != before {
		t.Fatal("unchanged readings created duplicate alerts")
	}
}

func TestHardwareCacheFreshnessAndIsolation(t *testing.T) {
	s := New(time.Second, true)
	s.EnableHardware(15 * time.Second)
	now := time.Now()
	if waiting := s.HardwareSnapshot(now); waiting.Mode != "waiting" || !waiting.Stale || waiting.Data != nil {
		t.Fatalf("waiting cache fabricated data: %+v", waiting)
	}
	s.UpdateHardware(now, hardware.Snapshot{}, nil)
	first := s.HardwareSnapshot(now)
	if first.Mode != "enabled" || first.Stale || !json.Valid(first.Data) {
		t.Fatalf("published hardware sample unavailable: %+v", first)
	}
	first.Data[0] = 'x'
	*first.SampledAt = time.Time{}
	copy := s.HardwareSnapshot(now)
	if !json.Valid(copy.Data) || !copy.SampledAt.Equal(now) {
		t.Fatal("returned hardware snapshot mutated the cache")
	}
	if !s.HardwareSnapshot(now.Add(40 * time.Second)).Stale {
		t.Fatal("hardware cache is not stale at its exact threshold")
	}
	s.UpdateHardware(now.Add(time.Second), hardware.Snapshot{}, errors.New("provider timeout"))
	if got := s.HardwareSnapshot(now.Add(time.Second)); got.Mode != "degraded" || got.Error != "provider timeout" {
		t.Fatalf("provider failure hidden: %+v", got)
	}
	if s.Snapshot(now).Telemetry != nil {
		t.Fatal("PC hardware fabricated UPS readiness")
	}
}

func sample(bits string) ups.Result {
	return ups.Result{
		Devices: []ups.Device{{VendorID: "0001", ProductID: "0000", Product: "MEC0003"}},
		Status:  "(230.0 000.0 230.0 010 50.0 27.0 30.0 " + bits + "\r",
	}
}

func TestPolicyInputCannotCarryOutageContinuityAcrossStaleGap(t *testing.T) {
	s := New(10*time.Second, true)
	start := time.Now()
	if err := s.Apply(start, sample("11001000"), nil); err != nil {
		t.Fatal(err)
	}
	first := s.PolicyInput(start)
	if !first.Fresh || !first.OnBattery || !first.LowBattery || first.OutageID == "" {
		t.Fatalf("verified input missing: %+v", first)
	}
	if s.PolicyInput(start.Add(10 * time.Second)).Fresh {
		t.Fatal("stale input remained eligible")
	}
	if err := s.Apply(start.Add(10*time.Second), sample("11001000"), nil); err != nil {
		t.Fatal(err)
	}
	if next := s.PolicyInput(start.Add(10 * time.Second)); !next.Fresh || next.OutageID == first.OutageID {
		t.Fatal("fresh reading after a stale gap reused outage continuity")
	}
	if err := s.Apply(start.Add(11*time.Second), ups.Result{}, errors.New("lost USB")); err == nil {
		t.Fatal("expected read failure")
	}
	if input := s.PolicyInput(start.Add(11 * time.Second)); input.Fresh || input.OutageID != "" {
		t.Fatalf("communication failure remained eligible: %+v", input)
	}
}

func TestShutdownPolicyPublishesImmutableStatusAndEvents(t *testing.T) {
	s := New(time.Minute, true)
	now := time.Now()
	deadline, trigger := now.Add(time.Minute), "outage"
	value := policy.Status{Enabled: true, AutomaticEnabled: true, State: "pending", Deadline: &deadline, Trigger: &trigger}
	s.UpdatePolicy(value, "shutdown_pending")
	deadline = time.Time{}
	trigger = "mutated"
	if err := s.Apply(now, sample("10001000"), nil); err != nil {
		t.Fatal(err)
	}
	got := s.Snapshot(now)
	if !got.Health.PolicyEligible || got.Policy.Deadline.IsZero() || *got.Policy.Trigger != "outage" {
		t.Fatal("policy publication lost data or shared pointers")
	}
	*got.Policy.Trigger = "also mutated"
	*got.Policy.Deadline = time.Time{}
	if fresh := s.Snapshot(now); *fresh.Policy.Trigger != "outage" || fresh.Policy.Deadline.IsZero() {
		t.Fatal("API snapshot mutated cached policy")
	}
	page, err := s.Page(20, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range page.Items {
		if event.Type == "shutdown_pending" && strings.Contains(event.Message, "deadline:") {
			return
		}
	}
	t.Fatal("pending policy did not emit an event with its deadline")
}

func TestHistorySurvivesRestartWithoutReusingTelemetry(t *testing.T) {
	options := events.Options{Directory: t.TempDir(), MaxFileBytes: 4096, MaxFiles: 2}
	journal, history, err := events.Open(options)
	if err != nil {
		t.Fatal(err)
	}
	first := NewWithHistory(time.Second, true, journal, history, nil)
	if err := first.Apply(time.Now(), sample("10001001"), nil); err != nil {
		t.Fatal(err)
	}
	first.Stop(time.Now())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := journal.Close(ctx); err != nil {
		t.Fatal(err)
	}
	reopened, history, err := events.Open(options)
	if err != nil {
		t.Fatal(err)
	}
	second := NewWithHistory(time.Second, true, reopened, history, nil)
	defer func() {
		if err := reopened.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	snapshot := second.Snapshot(time.Now())
	if snapshot.Telemetry != nil || snapshot.Power.Mode != "unknown" || snapshot.Power.ObservedSince != nil {
		t.Fatal("historical events became current power/telemetry evidence")
	}
	page, err := second.Page(2, "")
	if err != nil || len(page.Items) != 2 || page.NextCursor == nil || page.Items[0].SessionID == page.Items[1].SessionID {
		t.Fatalf("restart session page: %+v %v", page, err)
	}
	next, err := second.Page(2, *page.NextCursor)
	if err != nil || len(next.Items) != 2 || next.Items[0].SessionID != page.Items[1].SessionID || next.Items[0].Sequence >= page.Items[1].Sequence {
		t.Fatalf("cross-session pagination failed: %+v %v", next, err)
	}
}

func TestHistoryInitializationFailureIsNotReportedAsMemoryOnlySuccess(t *testing.T) {
	s := NewWithHistory(time.Second, false, nil, nil, errors.New("storage access denied"))
	status := s.Snapshot(time.Now()).Health.EventHistory
	if status.Mode != "degraded" || status.Error != "storage access denied" {
		t.Fatalf("storage failure hidden: %+v", status)
	}
}

func TestStalenessFailureAndRecovery(t *testing.T) {
	s := New(10*time.Second, true)
	start := time.Now()
	if got := s.Snapshot(start); got.Telemetry != nil || got.Power.Mode != "unknown" {
		t.Fatal("startup fabricated readings")
	}
	if err := s.Apply(start, sample("10001001"), nil); err != nil {
		t.Fatal(err)
	}
	first := s.Snapshot(start)
	if first.Telemetry.Stale || first.Power.ObservedSince == nil {
		t.Fatal("first battery sample not recorded")
	}
	// Returned maps and numeric pointers cannot mutate the cached measurements.
	*first.Telemetry.Measurements["load"].Value = 99
	delete(first.Telemetry.Measurements, "input_voltage")
	*first.Power.LowBattery = true
	first.Device.Product = "mutated"
	*first.Connection.LastSuccess = time.Time{}
	if got := *s.Snapshot(start).Telemetry.Measurements["load"].Value; got != 10 {
		t.Fatalf("cached measurement mutated: %v", got)
	}
	if got := s.Snapshot(start); *got.Power.LowBattery || got.Device.Product != "MEC0003" || got.Connection.LastSuccess.IsZero() {
		t.Fatal("returned snapshot pointers mutated shared state")
	}
	if got := s.Snapshot(start.Add(10 * time.Second)); !got.Telemetry.Stale {
		t.Fatal("sample must be stale exactly at threshold")
	}
	if err := s.Apply(start.Add(time.Second), ups.Result{}, errors.New("USB disconnected")); err == nil {
		t.Fatal("failure not surfaced")
	}
	failed := s.Snapshot(start.Add(time.Second))
	if failed.Power.Mode != "on_battery" || failed.Power.ObservedSince != nil || failed.Connection.State != "retrying" || failed.Health.PolicyEligible {
		t.Fatalf("failure guessed state or preserved outage continuity: %+v", failed)
	}
	recovered := start.Add(2 * time.Second)
	if err := s.Apply(recovered, sample("10001001"), nil); err != nil {
		t.Fatal(err)
	}
	if got := s.Snapshot(recovered); !got.Power.ObservedSince.Equal(recovered.UTC()) {
		t.Fatal("recovery invented outage continuity")
	}
	if err := s.Apply(recovered.Add(time.Second), sample("00001001"), nil); err != nil {
		t.Fatal(err)
	}
	if got := s.Snapshot(recovered); got.Power.Mode != "online" || got.Power.ObservedSince != nil {
		t.Fatal("restoration did not clear observed outage")
	}
}

func TestEventsBoundedAndPaginated(t *testing.T) {
	s := New(time.Second, true)
	start := time.Now()
	for i := 0; i < 10; i++ {
		s.Apply(start, ups.Result{}, fmt.Errorf("failure %d", i))
	}
	page, err := s.Page(3, "")
	if err != nil || len(page.Items) != 3 || page.NextCursor == nil {
		t.Fatalf("first page: %+v %v", page, err)
	}
	next, err := s.Page(3, *page.NextCursor)
	if err != nil || next.Items[0].Sequence >= page.Items[2].Sequence {
		t.Fatalf("pagination overlaps: %+v %v", next, err)
	}
	for i := 10; i < 220; i++ {
		s.Apply(start, ups.Result{}, fmt.Errorf("failure %d", i))
	}
	if _, err := s.Page(3, *page.NextCursor); !errors.Is(err, ErrExpiredCursor) {
		t.Fatalf("expected expired cursor, got %v", err)
	}
	all, err := s.Page(200, "")
	if err != nil || len(all.Items) != 200 {
		t.Fatalf("retention exceeded or lost entries: %d %v", len(all.Items), err)
	}
	if _, err := s.Page(3, "bad!"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("expected invalid cursor, got %v", err)
	}
}

func TestIdenticalSamplesDoNotDuplicateTransitions(t *testing.T) {
	s := New(time.Second, true)
	for i := 0; i < 5; i++ {
		if err := s.Apply(time.Now(), sample("00001001"), nil); err != nil {
			t.Fatal(err)
		}
	}
	page, _ := s.Page(200, "")
	if len(page.Items) != 3 {
		t.Fatalf("expected startup, connection and initial power state; got %d events", len(page.Items))
	}
}

func TestPollCancellationStopsInFlightRead(t *testing.T) {
	s := New(time.Second, true)
	ctx, cancel := context.WithCancel(context.Background())
	var active atomic.Int32
	entered, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		Poll(ctx, s, func(ctx context.Context) (ups.Result, error) {
			if active.Add(1) != 1 {
				t.Error("overlapping reads")
			}
			defer active.Add(-1)
			close(entered)
			<-ctx.Done()
			return ups.Result{}, ctx.Err()
		}, time.Second, time.Minute)
	}()
	<-entered
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("poller did not stop after cancellation")
	}
	if active.Load() != 0 {
		t.Fatal("in-flight work remains")
	}
}
