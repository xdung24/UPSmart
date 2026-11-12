package policy

import (
	"context"
	"testing"
	"time"
)

func TestReconfigureRestartsQualificationAndCannotResetPendingOrTerminal(t *testing.T) {
	options := Options{Mode: "dry_run", Automatic: &AutomaticOptions{Enabled: true, OutageAfter: "1s", Countdown: "5s"}}
	controller, err := New(options, func(time.Time) Input {
		return Input{Fresh: true, OnBattery: true, OutageID: "one"}
	}, func(context.Context) error { t.Fatal("dry run reached OS"); return nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	controller.Tick(context.Background(), now)
	if err := controller.Reconfigure(options, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	controller.Tick(context.Background(), now.Add(time.Second))
	if controller.Status().State == "pending" {
		t.Fatal("reconfiguration inherited earlier outage timing")
	}
	controller.Tick(context.Background(), now.Add(2*time.Second))
	persisted := false
	persist := func() error { persisted = true; return nil }
	if controller.Status().State != "pending" {
		t.Fatal("fresh requalification did not schedule countdown")
	}
	if err := controller.Reconfigure(Options{}, persist); err == nil || persisted {
		t.Fatal("pending policy was reset or saved")
	}
	controller.Tick(context.Background(), now.Add(7*time.Second))
	if controller.Status().State != "dry_run" {
		t.Fatal("test did not reach terminal dry-run outcome")
	}
	if err := controller.Reconfigure(Options{}, persist); err == nil || persisted {
		t.Fatal("terminal attempt was unlatched")
	}
}
