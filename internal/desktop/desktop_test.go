package desktop

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
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

type quitApp struct {
	fyne.App
	quit chan struct{}
}

func (a *quitApp) Quit() { close(a.quit) }

type closingAPI struct {
	unusedAPI
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (a *closingAPI) Close() error {
	if a.calls.Add(1) == 1 {
		close(a.started)
	}
	<-a.release
	return nil
}

func TestQuitJoinsOwnedMonitorBeforeExitingAndIsIdempotent(t *testing.T) {
	base := test.NewApp()
	defer base.Quit()
	app := &quitApp{App: base, quit: make(chan struct{})}
	api := &closingAPI{started: make(chan struct{}), release: make(chan struct{})}
	ui := New(app, api)
	ui.Quit()
	select {
	case <-api.started:
	case <-time.After(3 * time.Second):
		t.Fatal("Quit did not stop the owned monitor")
	}
	select {
	case <-app.quit:
		t.Fatal("desktop exited before owned monitoring stopped")
	default:
	}
	ui.Quit()
	close(api.release)
	select {
	case <-app.quit:
	case <-time.After(3 * time.Second):
		t.Fatal("desktop did not finish quitting after monitoring stopped")
	}
	if api.calls.Load() != 1 {
		t.Fatal("repeated Quit stopped the owner more than once")
	}
}

func TestNativeViewShowsFreshUnavailableCountdownAndBackendLoss(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	api := new(unusedAPI)
	ui := New(app, api)
	ui.SetDirect(true)
	if !strings.Contains(ui.source.Text, "Direct USB") || !strings.Contains(ui.footer.Text, "Quit stops direct USB") {
		t.Fatal("direct ownership/quit semantics not disclosed")
	}
	ui.SetDirect(false)
	if !strings.Contains(ui.source.Text, "Daemon") || !strings.Contains(ui.footer.Text, "daemon monitoring") {
		t.Fatal("daemon ownership/quit semantics not disclosed")
	}
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
