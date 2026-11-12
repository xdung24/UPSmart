package desktop

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"upsmart/internal/control"
	"upsmart/internal/policy"
	"upsmart/internal/state"
	"upsmart/internal/ups"
)

type unusedAPI struct{ calls int }

func (a *unusedAPI) Request(context.Context, string, string, io.Reader, any) error {
	a.calls++
	return errors.New("not started")
}

func TestNativeViewShowsFreshUnavailableCountdownAndBackendLoss(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	api := new(unusedAPI)
	ui := New(app, api)
	store := state.New(10*time.Second, true)
	now := time.Now()
	result := ups.Result{Devices: []ups.Device{{Product: "MEC0003"}}, Status: "(230.0 000.0 234.0 014 50.1 27.0 29.0 10001000\r"}
	if err := store.Apply(now, result, nil); err != nil {
		t.Fatal(err)
	}
	deadline := now.Add(30 * time.Second)
	s := store.Snapshot(now.Add(5 * time.Second))
	s.Policy = policy.Status{Enabled: true, DryRun: true, ManualEnabled: true, State: "pending", Deadline: &deadline}
	ui.Apply(control.Snapshot{UPS: s}, nil)
	if ui.values["input_voltage"].Text != "230.0 V AC" || ui.values["battery_capacity"].Text == "100.0 %" {
		t.Fatal("native data view fabricated or lost measurement")
	}
	if ui.elapsed.Text != "Observed outage: 5s | Shutdown countdown: 25s (at last refresh)" {
		t.Fatal("native countdown does not follow backend time:", ui.elapsed.Text)
	}
	if !ui.shutdown.Disabled() || ui.cancelBtn.Disabled() {
		t.Fatal("pending action controls are incorrect")
	}
	ui.Apply(control.Snapshot{}, errors.New("disconnected"))
	if !ui.shutdown.Disabled() || !ui.cancelBtn.Disabled() || ui.values["input_voltage"].Text != "Unavailable - backend connection lost" {
		t.Fatal("backend loss left actionable controls or healthy data")
	}
	if api.calls != 0 {
		t.Fatal("view rendering invoked backend control")
	}
}
