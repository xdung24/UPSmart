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
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

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
	if len(ui.tabs.Items) != 4 ||
		ui.tabs.Items[0].Text != "Data View" ||
		ui.tabs.Items[1].Text != "Setting" ||
		ui.tabs.Items[2].Text != "Shutdown" ||
		ui.tabs.Items[3].Text != "Log" {
		t.Fatal("desktop tabs should be Data View, Setting, Shutdown, and Log")
	}
	if ui.tabs.SelectedIndex() != 0 {
		t.Fatal("desktop should open on the Data View tab")
	}
	if ui.tabs.Items[3].Content != ui.events {
		t.Fatal("Log tab should contain only the event log")
	}
	ui.setEventLog("Readable event")
	if ui.events.String() != "Readable event" || len(ui.events.Segments) != 1 {
		t.Fatal("event log text was not rendered")
	}
	segment, ok := ui.events.Segments[0].(*widget.TextSegment)
	if !ok || segment.Style.ColorName != theme.ColorNameForeground {
		t.Fatal("event log should use the current theme's foreground color")
	}
	_, isContainer := ui.Window.Canvas().Content().(*fyne.Container)
	if isContainer || ui.Window.Canvas().Content() != ui.tabs {
		t.Fatal("desktop content should consist of the tabs without a footer")
	}
	if size := ui.Window.Canvas().Size(); size.Height != 480 {
		t.Fatalf("desktop window height = %v; want 480", size.Height)
	}
	dataView := ui.tabs.Items[0].Content.(*container.Scroll)
	dataContent := dataView.Content.(*fyne.Container)
	if len(dataContent.Objects) != 5 {
		t.Fatal("Data View should not include a bottom text row")
	}
	ui.SetDirect(true)
	if !strings.Contains(ui.policyNote.Text, "Direct mode") {
		t.Fatal("direct-mode ownership/policy behavior not disclosed")
	}
	ui.SetDirect(false)
	if !strings.Contains(ui.policyNote.Text, "daemon's dedicated shutdown-policy file") {
		t.Fatal("daemon ownership/policy behavior not disclosed")
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
	s.Telemetry.Measurements["battery_capacity"] = ups.Measurement{
		Unit: "%", Quality: "unavailable", Reason: "Not reported by the Q1 status query",
	}
	s.Health.Warnings = []string{"test warning"}
	ui.Apply(control.Snapshot{UPS: s}, nil)
	if ui.values["input_voltage"].Text != "230.0 V AC" || ui.values["battery_capacity"].Text != "100.0 % (estimated)" {
		t.Fatalf("native data view lost or mislabeled the estimated capacity: voltage=%q capacity=%q",
			ui.values["input_voltage"].Text, ui.values["battery_capacity"].Text)
	}
	s.Telemetry.Measurements["battery_capacity"] = ups.Measurement{
		Unit: "%", Quality: "invalid", Reason: "Invalid reported capacity",
	}
	ui.Apply(control.Snapshot{UPS: s}, nil)
	if ui.values["battery_capacity"].Text != "Unavailable - Invalid reported capacity" {
		t.Fatal("legacy fallback must not override invalid capacity data:", ui.values["battery_capacity"].Text)
	}
	s.Telemetry.Measurements["battery_capacity"] = ups.Measurement{
		Unit: "%", Quality: "unavailable", Reason: "Not reported by the Q1 status query",
	}
	ui.Apply(control.Snapshot{UPS: s}, nil)
	if ui.elapsed.Text != "Observed outage: 5s | Shutdown countdown: 25s (at last refresh)" {
		t.Fatal("native countdown does not follow backend time:", ui.elapsed.Text)
	}
	if !ui.shutdown.Disabled() || ui.cancelBtn.Disabled() {
		t.Fatal("pending action controls are incorrect")
	}
	s.Policy.State = "armed"
	ui.Apply(control.Snapshot{UPS: s}, nil)
	if ui.shutdown.Disabled() || !ui.cancelBtn.Disabled() {
		t.Fatal("daemon desktop controls do not match armed policy")
	}
	ui.Apply(control.Snapshot{}, errors.New("disconnected"))
	if !ui.shutdown.Disabled() || !ui.cancelBtn.Disabled() || ui.values["input_voltage"].Text != "Unavailable - backend connection lost" {
		t.Fatal("backend loss left actionable controls or healthy data")
	}
	if api.calls != 0 {
		t.Fatal("view rendering invoked backend control")
	}
}
