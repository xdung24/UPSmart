package policy

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

var epoch = time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)

func automatic(mode string) Options {
	return Options{Mode: mode, Automatic: &AutomaticOptions{
		Enabled: true, OutageAfter: "10s", LowBattery: true, Countdown: "5s",
	}}
}

func withManual(o Options) Options {
	o.Manual = &ManualOptions{Enabled: true, Countdown: "5s", AllowedUserIDs: []int64{123}}
	return o
}

type fixture struct {
	c      *Controller
	input  Input
	calls  int
	err    error
	events []string
	states []Status
}

func setup(t *testing.T, o Options) *fixture {
	t.Helper()
	f := &fixture{input: Input{Fresh: true, OnBattery: true, OutageID: "one"}}
	c, err := New(o, func(time.Time) Input { return f.input }, func(context.Context) error {
		f.calls++
		return f.err
	}, func(s Status, event string) {
		f.events = append(f.events, event)
		f.states = append(f.states, s)
	})
	if err != nil {
		t.Fatal(err)
	}
	f.c = c
	return f
}

func (f *fixture) tick(seconds int) {
	f.c.Tick(context.Background(), epoch.Add(time.Duration(seconds)*time.Second))
}

func requireState(t *testing.T, f *fixture, state string, calls int) Status {
	t.Helper()
	s := f.c.Status()
	if s.State != state || f.calls != calls {
		t.Fatalf("state=%s calls=%d, want %s/%d: %+v", s.State, f.calls, state, calls, s)
	}
	return s
}

func TestOutageDelayAndCountdownExactBoundaries(t *testing.T) {
	f := setup(t, automatic("live"))
	f.tick(0)
	f.c.Tick(context.Background(), epoch.Add(10*time.Second-time.Nanosecond))
	requireState(t, f, "armed", 0)
	f.tick(10)
	s := requireState(t, f, "pending", 0)
	if *s.Trigger != "outage" || !s.Deadline.Equal(epoch.Add(15*time.Second)) {
		t.Fatalf("unexpected pending: %+v", s)
	}
	f.c.Tick(context.Background(), epoch.Add(15*time.Second-time.Nanosecond))
	requireState(t, f, "pending", 0)
	f.tick(15)
	requireState(t, f, "requested", 1)
	for _, seconds := range []int{16, 100, 1000} {
		f.input = Input{Fresh: true}
		f.tick(seconds)
		f.input = Input{Fresh: true, OnBattery: true, LowBattery: true, OutageID: "new"}
		f.tick(seconds + 1)
	}
	requireState(t, f, "requested", 1)
	if err := f.c.Manual(epoch); err == nil {
		t.Fatal("terminal controller accepted manual request")
	}
	if !reflect.DeepEqual(f.events, []string{"shutdown_pending", "shutdown_requesting", "shutdown_requested"}) {
		t.Fatalf("events: %v", f.events)
	}
}

func TestRestorationCancelsBeforeDeadlineAndRequalifies(t *testing.T) {
	f := setup(t, automatic("live"))
	f.tick(0)
	f.tick(10)
	f.input = Input{Fresh: true}
	f.tick(14)
	requireState(t, f, "cancelled", 0)
	f.tick(15)
	requireState(t, f, "armed", 0)
	f.input = Input{Fresh: true, OnBattery: true, OutageID: "two"}
	f.tick(20)
	f.tick(29)
	requireState(t, f, "armed", 0)
	f.tick(30)
	f.tick(35)
	requireState(t, f, "requested", 1)
}

func TestUnavailableAndContinuityResetQualification(t *testing.T) {
	for _, change := range []string{"stale", "disconnect", "new_id"} {
		for _, pending := range []bool{false, true} {
			t.Run(change+map[bool]string{false: "_qualifying", true: "_pending"}[pending], func(t *testing.T) {
				f := setup(t, automatic("live"))
				f.tick(0)
				if pending {
					f.tick(10)
				}
				if change == "new_id" {
					f.input.OutageID = "two"
				} else {
					f.input.Fresh = false
					if change == "disconnect" {
						f.input.OutageID = ""
					}
				}
				f.tick(11)
				if pending {
					want := "blocked"
					if change == "new_id" {
						want = "cancelled"
					}
					requireState(t, f, want, 0)
				}
				f.input = Input{Fresh: true, OnBattery: true, OutageID: "two"}
				f.tick(12)
				f.tick(20)
				requireState(t, f, "armed", 0)
				qualification := 22
				if change == "new_id" {
					qualification = 21
				}
				f.tick(qualification)
				s := requireState(t, f, "pending", 0)
				if !s.Deadline.Equal(epoch.Add(time.Duration(qualification+5) * time.Second)) {
					t.Fatal("old countdown was reused")
				}
			})
		}
	}
}

func TestLowBatteryRequiresFreshBatteryAndEnabledFlag(t *testing.T) {
	for _, tc := range []struct {
		name        string
		fresh, batt bool
		enabled     bool
		state       string
	}{
		{"verified", true, true, true, "pending"},
		{"stale", false, true, true, "blocked"},
		{"online", true, false, true, "armed"},
		{"not_enabled", true, true, false, "armed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := automatic("live")
			o.Automatic.LowBattery = tc.enabled
			f := setup(t, o)
			f.input = Input{Fresh: tc.fresh, OnBattery: tc.batt, LowBattery: true}
			f.tick(0)
			requireState(t, f, tc.state, 0)
			if tc.state == "pending" {
				f.tick(4)
				requireState(t, f, "pending", 0)
				f.tick(5)
				requireState(t, f, "requested", 1)
			}
		})
	}
}

func TestLowBatteryClearingAndDurationFallback(t *testing.T) {
	t.Run("unqualified_cancelled", func(t *testing.T) {
		f := setup(t, automatic("live"))
		f.input.LowBattery = true
		f.tick(0)
		f.input.LowBattery = false
		f.tick(4)
		requireState(t, f, "cancelled", 0)
		f.tick(5)
		requireState(t, f, "armed", 0)
	})
	t.Run("qualified_gets_full_countdown", func(t *testing.T) {
		f := setup(t, automatic("live"))
		f.tick(0)
		f.input.LowBattery = true
		f.tick(8)
		f.input.LowBattery = false
		f.tick(12)
		s := requireState(t, f, "pending", 0)
		if *s.Trigger != "outage" || !s.Deadline.Equal(epoch.Add(17*time.Second)) {
			t.Fatalf("unsafe fallback countdown: %+v", s)
		}
		f.tick(13)
		requireState(t, f, "pending", 0)
		f.tick(17)
		requireState(t, f, "requested", 1)
	})
	t.Run("new_low_battery_does_not_shorten_outage_countdown", func(t *testing.T) {
		f := setup(t, automatic("live"))
		f.tick(0)
		f.tick(10)
		f.input.LowBattery = true
		f.tick(11)
		s := requireState(t, f, "pending", 0)
		if *s.Trigger != "outage" || !s.Deadline.Equal(epoch.Add(15*time.Second)) {
			t.Fatal("low battery changed existing countdown")
		}
		f.tick(15)
		requireState(t, f, "requested", 1)
	})
}

func TestDisabledAndDryRunNeverRequest(t *testing.T) {
	c, err := New(Options{}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.Tick(context.Background(), epoch)
	if c.Status().State != "disabled" || c.Manual(epoch) == nil {
		t.Fatal("disabled policy was actionable")
	}
	o := automatic("dry_run")
	o.Automatic.OutageAfter = ""
	f := setup(t, o)
	f.c.request = nil
	f.input.LowBattery = true
	f.tick(0)
	f.tick(5)
	s := requireState(t, f, "dry_run", 0)
	if *s.Outcome != "would_request" {
		t.Fatalf("outcome: %+v", s)
	}
	for i := 6; i < 10; i++ {
		f.tick(i)
	}
	requireState(t, f, "dry_run", 0)
	if !reflect.DeepEqual(f.events, []string{"shutdown_pending", "shutdown_dry_run"}) {
		t.Fatalf("dry run events: %v", f.events)
	}
}

func TestFailureIsVisibleAndNeverRetried(t *testing.T) {
	f := setup(t, automatic("live"))
	f.err = errors.New("permission denied")
	f.input.LowBattery = true
	f.tick(0)
	f.tick(5)
	for i := 6; i < 20; i++ {
		f.tick(i)
	}
	s := requireState(t, f, "failed", 1)
	if *s.Outcome != "permission denied" || f.events[len(f.events)-1] != "shutdown_failed" {
		t.Fatal("failure evidence missing")
	}
}

func TestManualIndependentOfFreshnessAndRestoration(t *testing.T) {
	for _, initial := range []Input{{Fresh: true}, {}, {Fresh: true, OnBattery: true, OutageID: "one"}} {
		t.Run("initial_"+map[bool]string{true: "battery", false: "other"}[initial.OnBattery]+map[bool]string{true: "_fresh", false: "_stale"}[initial.Fresh], func(t *testing.T) {
			f := setup(t, withManual(Options{Mode: "live"}))
			f.input = initial
			if err := f.c.Manual(epoch); err != nil {
				t.Fatal(err)
			}
			if err := f.c.Manual(epoch.Add(time.Second)); err == nil {
				t.Fatal("duplicate manual command extended countdown")
			}
			f.tick(4)
			requireState(t, f, "pending", 0)
			f.tick(5)
			requireState(t, f, "requested", 1)
			f.tick(100)
			requireState(t, f, "requested", 1)
		})
	}
	t.Run("battery_seen_then_restored", func(t *testing.T) {
		f := setup(t, withManual(Options{Mode: "live"}))
		f.input = Input{Fresh: true}
		if err := f.c.Manual(epoch); err != nil {
			t.Fatal(err)
		}
		f.tick(1)
		requireState(t, f, "pending", 0)
		f.input = Input{Fresh: true, OnBattery: true}
		f.tick(2)
		f.input = Input{}
		f.tick(3)
		requireState(t, f, "pending", 0)
		f.input = Input{Fresh: true}
		f.tick(5)
		requireState(t, f, "cancelled", 0)
	})
}

func TestCancelSuppressesUntilVerifiedMains(t *testing.T) {
	f := setup(t, withManual(automatic("live")))
	f.input.LowBattery = true
	f.tick(0)
	if err := f.c.Cancel(epoch.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	f.tick(5)
	requireState(t, f, "cancelled", 0)
	f.input.Fresh = false
	f.tick(6)
	f.input = Input{Fresh: true, OnBattery: true, LowBattery: true, OutageID: "different"}
	f.tick(7)
	f.tick(100)
	requireState(t, f, "cancelled", 0)
	f.input = Input{Fresh: true}
	f.tick(101)
	f.input = Input{Fresh: true, OnBattery: true, OutageID: "next"}
	f.tick(102)
	f.tick(111)
	requireState(t, f, "armed", 0)
	f.tick(112)
	requireState(t, f, "pending", 0)
	f.tick(117)
	requireState(t, f, "requested", 1)
	if err := f.c.Cancel(epoch.Add(118 * time.Second)); err == nil {
		t.Fatal("cancel accepted after request")
	}
}

func TestManualCancelAndExplicitReschedule(t *testing.T) {
	f := setup(t, withManual(Options{Mode: "live"}))
	f.input = Input{}
	if err := f.c.Manual(epoch); err != nil {
		t.Fatal(err)
	}
	if err := f.c.Cancel(epoch.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	f.tick(5)
	requireState(t, f, "cancelled", 0)
	if err := f.c.Manual(epoch.Add(6 * time.Second)); err != nil {
		t.Fatal("explicit manual request was suppressed:", err)
	}
	f.tick(10)
	requireState(t, f, "pending", 0)
	f.tick(11)
	requireState(t, f, "requested", 1)
}

func TestManualCanOverrideAutomaticCancellationSuppression(t *testing.T) {
	f := setup(t, withManual(automatic("live")))
	f.input.LowBattery = true
	f.tick(0)
	if err := f.c.Cancel(epoch.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := f.c.Manual(epoch.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	f.tick(7)
	s := requireState(t, f, "requested", 1)
	if *s.Trigger != "manual" {
		t.Fatal("manual request lost its independent trigger")
	}
}

func TestLatestInputRecheckedAtDispatch(t *testing.T) {
	for _, latest := range []Input{
		{Fresh: true}, {},
		{Fresh: true, OnBattery: true, OutageID: "new"},
		{Fresh: true, OnBattery: true, OutageID: "one"},
	} {
		t.Run("latest_"+latest.OutageID+map[bool]string{true: "_fresh", false: "_stale"}[latest.Fresh], func(t *testing.T) {
			f := setup(t, automatic("live"))
			f.input.LowBattery = true
			f.tick(0)
			reads := 0
			f.c.read = func(time.Time) Input {
				reads++
				if reads == 1 {
					return f.input
				}
				return latest
			}
			f.tick(5)
			if reads != 2 || f.calls != 0 || f.c.Status().State == "requested" {
				t.Fatalf("latest input ignored: reads=%d calls=%d status=%+v", reads, f.calls, f.c.Status())
			}
		})
	}
}

func TestCancelledContextCannotDispatch(t *testing.T) {
	f := setup(t, automatic("live"))
	f.input.LowBattery = true
	f.tick(0)
	ctx, cancel := context.WithCancel(context.Background())
	f.c.read = func(time.Time) Input {
		cancel()
		return f.input
	}
	f.c.Tick(ctx, epoch.Add(5*time.Second))
	requireState(t, f, "pending", 0)
}

func TestContextCancelledDuringRequestingPublication(t *testing.T) {
	f := setup(t, automatic("live"))
	f.input.LowBattery = true
	f.tick(0)
	ctx, cancel := context.WithCancel(context.Background())
	f.c.publish = func(_ Status, event string) {
		if event == "shutdown_requesting" {
			cancel()
		}
	}
	f.c.Tick(ctx, epoch.Add(5*time.Second))
	requireState(t, f, "failed", 0)
	if *f.c.Status().Outcome != context.Canceled.Error() {
		t.Fatal("cancellation outcome missing")
	}
}

func TestConcurrentTicksOnlyDispatchOnce(t *testing.T) {
	f := setup(t, automatic("live"))
	f.input.LowBattery = true
	f.tick(0)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.tick(5)
		}()
	}
	wg.Wait()
	requireState(t, f, "requested", 1)
}

func TestAcceptedCancelBeforeConcurrentTicksPreventsDispatch(t *testing.T) {
	f := setup(t, automatic("live"))
	f.input.LowBattery = true
	f.tick(0)
	var wg sync.WaitGroup
	cancelAccepted := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := f.c.Cancel(epoch.Add(5 * time.Second)); err != nil {
			t.Error(err)
		}
		close(cancelAccepted)
	}()
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-cancelAccepted
			f.tick(5)
			_ = f.c.Status()
		}()
	}
	wg.Wait()
	requireState(t, f, "cancelled", 0)
}

func TestStatusAndPublishedPointersAreIndependent(t *testing.T) {
	f := setup(t, automatic("live"))
	f.input.LowBattery = true
	f.tick(0)
	s := f.c.Status()
	*s.Trigger = "manual"
	*s.Deadline = epoch
	*f.states[0].Trigger = "corrupted"
	*f.states[0].Deadline = epoch
	current := f.c.Status()
	if *current.Trigger != "low_battery" || !current.Deadline.Equal(epoch.Add(5*time.Second)) {
		t.Fatal("status aliased external pointers")
	}
	f.tick(5)
	s = f.c.Status()
	*s.Outcome = "corrupted"
	if *f.c.Status().Outcome != "accepted" {
		t.Fatal("outcome alias")
	}
}

func TestRestartAndOptionsOwnership(t *testing.T) {
	o := automatic("live")
	f := setup(t, o)
	o.Automatic.Enabled = false
	o.Automatic.Countdown = "1h"
	f.tick(0)
	f.tick(10)
	requireState(t, f, "pending", 0)
	restarted := setup(t, automatic("live"))
	restarted.tick(15)
	restarted.tick(24)
	requireState(t, restarted, "armed", 0)
	restarted.tick(25)
	requireState(t, restarted, "pending", 0)
}
