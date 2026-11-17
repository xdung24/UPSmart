package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	fynedesktop "fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"upsmart/internal/control"
	"upsmart/internal/policy"
	"upsmart/internal/ups"
)

type API interface {
	Request(context.Context, string, string, io.Reader, any) error
}

type UI struct {
	app        fyne.App
	Window     fyne.Window
	api        API
	ctx        context.Context
	cancel     context.CancelFunc
	closed     atomic.Bool
	policy     *widget.Label
	policyNote *widget.Label
	elapsed    *widget.Label
	events     *widget.RichText
	tabs       *container.AppTabs
	shutdown   *widget.Button
	cancelBtn  *widget.Button
	values     map[string]*widget.Label
	lights     map[string]*canvas.Circle
	flow       *powerFlow
	last       control.Snapshot
}

func New(app fyne.App, api API) *UI {
	ctx, cancel := context.WithCancel(context.Background())
	u := &UI{app: app, api: api, ctx: ctx, cancel: cancel, values: make(map[string]*widget.Label),
		lights: make(map[string]*canvas.Circle)}
	u.Window = app.NewWindow("UPSmart Monitor")
	u.Window.SetIcon(theme.ComputerIcon())
	u.Window.Resize(fyne.NewSize(800, 480))
	u.policy = widget.NewLabel("PC shutdown policy: unknown")
	u.policy.Wrapping = fyne.TextWrapWord
	u.policyNote = widget.NewLabel("Shutdown settings are saved by the active monitor and applied immediately.")
	u.policyNote.Wrapping = fyne.TextWrapWord
	u.elapsed = widget.NewLabel("Observed outage: unavailable | Countdown: unavailable")
	u.flow = newPowerFlow()
	input := u.reading("input_voltage", "Input voltage")
	output := u.reading("output_voltage", "Output voltage")
	frequency := u.reading("input_frequency", "Input frequency")
	load := u.reading("load", "Load")
	battery := u.reading("battery_voltage", "Battery voltage")
	temperature := u.reading("ups_temperature", "UPS temperature")
	capacity := u.reading("battery_capacity", "Battery capacity")
	lights := container.NewGridWithColumns(3,
		u.indicator("communication", "Communication verified"),
		u.indicator("normal", "No reported UPS fault"),
		u.indicator("mains", "Mains verified"),
		u.indicator("battery", "Low battery flag"),
		widget.NewLabel("Self-test status: unavailable"))
	u.shutdown = widget.NewButtonWithIcon("Shutdown Windows...", theme.MediaStopIcon(), u.confirmShutdown)
	u.shutdown.Importance = widget.DangerImportance
	u.cancelBtn = widget.NewButton("Cancel pending shutdown", func() { u.command("/cancel", nil) })
	u.shutdown.Disable()
	u.cancelBtn.Disable()
	data := container.NewVBox(
		container.NewGridWithColumns(3,
			container.NewVBox(input, frequency),
			container.NewMax(u.flow),
			container.NewVBox(output, load)),
		container.NewGridWithColumns(3, battery, temperature, capacity),
		widget.NewSeparator(), lights)
	u.events = widget.NewRichText(&widget.TextSegment{
		Style: widget.RichTextStyle{ColorName: theme.ColorNameForeground},
	})
	u.events.Wrapping = fyne.TextWrapWord
	u.events.Scroll = fyne.ScrollVerticalOnly
	settings := container.NewVBox(
		widget.NewCard("General settings", "Not editable in this desktop",
			widget.NewLabel("USB, hardware, Telegram and account settings remain in the active monitor's configuration. Shutdown policy is managed on the Shutdown tab.")),
	)
	shutdown := container.NewVBox(
		widget.NewCard("Shutdown policy", "Configured and applied by the active monitor",
			container.NewVBox(
				u.policy,
				u.elapsed,
				u.policyNote,
				widget.NewButtonWithIcon("Configure shutdown policy...", theme.SettingsIcon(), u.settings),
			)),
		widget.NewCard("Manual shutdown", "Confirmed, non-forced Windows shutdown",
			container.NewVBox(
				container.NewHBox(u.shutdown, u.cancelBtn),
				widget.NewLabel("Cancel works only before Windows accepts the request. UPS output control is unavailable."),
			)),
	)
	u.tabs = container.NewAppTabs(
		container.NewTabItem("Data View", container.NewVScroll(data)),
		container.NewTabItem("Setting", container.NewVScroll(settings)),
		container.NewTabItem("Shutdown", container.NewVScroll(shutdown)),
		container.NewTabItem("Log", u.events))
	u.Window.SetContent(u.tabs)
	if desktop, ok := app.(fynedesktop.App); ok {
		desktop.SetSystemTrayIcon(theme.ComputerIcon())
		desktop.SetSystemTrayMenu(fyne.NewMenu("UPSmart Monitor",
			fyne.NewMenuItem("Show UPSmart", u.Show),
			fyne.NewMenuItem("Settings", func() { u.Show(); u.tabs.SelectIndex(1) }),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Quit desktop", u.Quit)))
		desktop.SetSystemTrayWindow(u.Window)
		u.Window.SetCloseIntercept(u.Window.Hide)
	}
	app.Lifecycle().SetOnStopped(cancel)
	return u
}

func (u *UI) reading(key, label string) fyne.CanvasObject {
	value := widget.NewLabelWithStyle("Unavailable", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	u.values[key] = value
	subtitle := ""
	if key == "ups_temperature" {
		subtitle = "Sensor location unverified"
	}
	return widget.NewCard(label, subtitle, container.NewCenter(value))
}

func (u *UI) indicator(key, label string) fyne.CanvasObject {
	light := canvas.NewCircle(color.NRGBA{R: 128, G: 128, B: 128, A: 255})
	u.lights[key] = light
	return container.NewHBox(container.NewGridWrap(fyne.NewSize(16, 16), light), widget.NewLabel(label))
}

func (u *UI) Show() { u.Window.Show(); u.Window.RequestFocus() }

func (u *UI) SetDirect(direct bool) {
	if direct {
		u.policyNote.SetText("Direct mode: saved in this user's desktop policy and applied immediately.")
	} else {
		u.policyNote.SetText("Daemon mode: saved to the daemon's dedicated shutdown-policy file and applied immediately. The main YAML is unchanged.")
	}
}

func (u *UI) Quit() {
	if !u.closed.CompareAndSwap(false, true) {
		return
	}
	u.cancel()
	finish := func() {
		u.Window.SetCloseIntercept(nil)
		u.Window.Close()
		u.app.Quit()
	}
	if closer, ok := u.api.(io.Closer); ok {
		u.shutdown.Disable()
		u.cancelBtn.Disable()
		go func() {
			err := closer.Close()
			fyne.Do(func() {
				if err != nil {
					u.Show()
					message := dialog.NewError(err, u.Window)
					message.SetOnClosed(finish)
					message.Show()
					return
				}
				finish()
			})
		}()
		return
	}
	finish()
}

func (u *UI) Start() {
	go func() {
		timer := time.NewTicker(180 * time.Millisecond)
		defer timer.Stop()
		for {
			select {
			case <-u.ctx.Done():
				return
			case <-timer.C:
				fyne.Do(func() {
					if !u.closed.Load() {
						u.flow.Advance()
					}
				})
			}
		}
	}()
	go func() {
		for u.ctx.Err() == nil {
			var snapshot control.Snapshot
			err := u.api.Request(u.ctx, http.MethodGet, "/snapshot", nil, &snapshot)
			if u.ctx.Err() != nil {
				return
			}
			fyne.Do(func() {
				if !u.closed.Load() {
					u.Apply(snapshot, err)
				}
			})
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-u.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
}

func (u *UI) Apply(snapshot control.Snapshot, err error) {
	if err == nil && snapshot.UPS.SchemaVersion != 1 {
		err = fmt.Errorf("unsupported backend snapshot schema %d", snapshot.UPS.SchemaVersion)
	}
	if err != nil {
		u.policy.SetText("Policy status unknown. A backend countdown may still be active.")
		u.elapsed.SetText("Observed outage: unavailable | Countdown: unknown")
		u.flow.SetState("unknown", false)
		for _, label := range u.values {
			label.SetText("Unavailable - backend connection lost")
		}
		for key := range u.lights {
			u.light(key, false, false)
		}
		u.shutdown.Disable()
		u.cancelBtn.Disable()
		u.setEventLog("Backend unavailable; event history cannot be refreshed.")
		return
	}
	u.last = snapshot
	s := snapshot.UPS
	fresh := s.Connection.State == "connected" && s.Telemetry != nil && !s.Telemetry.Stale
	for key, label := range u.values {
		text := "Unavailable"
		if s.Telemetry != nil {
			m, ok := s.Telemetry.Measurements[key]
			if key == "battery_capacity" && (!ok || (m.Value == nil && m.Quality == "unavailable")) {
				if voltage, found := s.Telemetry.Measurements["battery_voltage"]; found &&
					voltage.Value != nil && voltage.Quality == "reported" {
					m = ups.EstimateBatteryCapacity(*voltage.Value)
					ok = m.Value != nil
				}
			}
			if ok {
				if m.Value != nil {
					text = fmt.Sprintf("%.1f %s", *m.Value, m.Unit)
					if m.Quality == "estimated" {
						text += " (estimated)"
					}
					if !fresh {
						text += " (last known)"
					}
				} else if m.Reason != "" {
					text += " - " + m.Reason
				}
			}
		}
		label.SetText(text)
	}
	u.light("communication", fresh, fresh)
	u.light("normal", fresh && s.Power.Fault != nil, s.Power.Fault != nil && !*s.Power.Fault)
	u.light("mains", fresh && s.Power.Mode != "unknown", s.Power.Mode == "online")
	u.light("battery", fresh && s.Power.LowBattery != nil, s.Power.LowBattery != nil && *s.Power.LowBattery)
	u.flow.SetState(s.Power.Mode, fresh)
	outage, remaining := "unavailable", "none"
	if fresh && s.Power.Mode == "online" {
		outage = "none"
	}
	if fresh && s.Power.ObservedSince != nil {
		outage = max(0, s.GeneratedAt.Sub(*s.Power.ObservedSince)).Truncate(time.Second).String()
	}
	if s.Policy.Deadline != nil {
		remaining = max(0, s.Policy.Deadline.Sub(s.GeneratedAt)).Truncate(time.Second).String() + " (at last refresh)"
	}
	u.elapsed.SetText("Observed outage: " + outage + " | Shutdown countdown: " + remaining)
	mode := "disabled"
	if s.Policy.Enabled {
		mode = "LIVE"
		if s.Policy.DryRun {
			mode = "DRY RUN - no Windows shutdown"
		}
	}
	text := fmt.Sprintf("PC policy: %s | %s | %s", mode, s.Policy.State, s.Policy.BlockingReason)
	if s.Policy.Outcome != nil {
		text += " | " + *s.Policy.Outcome
	}
	u.policy.SetText(text)
	terminal := s.Policy.State == "requested" || s.Policy.State == "failed" || s.Policy.State == "dry_run"
	u.shutdown.Disable()
	u.cancelBtn.Disable()
	if s.Policy.ManualEnabled && s.Policy.State != "pending" && !terminal {
		u.shutdown.Enable()
	}
	if s.Policy.State == "pending" {
		u.cancelBtn.Enable()
	}
	var events strings.Builder
	for _, event := range snapshot.Events.Items {
		fmt.Fprintf(&events, "%s  [%s] %s\n", event.Time.Local().Format("2006-01-02 15:04:05"), event.Severity, event.Message)
	}
	u.setEventLog(events.String())
}

func (u *UI) setEventLog(text string) {
	if u.events.String() == text {
		return
	}
	u.events.Segments = []widget.RichTextSegment{&widget.TextSegment{
		Text:  text,
		Style: widget.RichTextStyle{ColorName: theme.ColorNameForeground},
	}}
	u.events.Refresh()
}

func (u *UI) light(key string, known, active bool) {
	c := color.NRGBA{R: 128, G: 128, B: 128, A: 255}
	if known {
		c = color.NRGBA{R: 80, G: 95, B: 110, A: 255}
		if active {
			c = color.NRGBA{R: 45, G: 170, B: 80, A: 255}
			if key == "battery" {
				c = color.NRGBA{R: 220, G: 100, B: 30, A: 255}
			}
		}
	}
	u.lights[key].FillColor = c
	u.lights[key].Refresh()
}

func (u *UI) command(path string, expected *policy.Status) {
	go func() {
		var result policy.Status
		var body io.Reader
		var err error
		if expected != nil {
			mode := "live"
			if expected.DryRun {
				mode = "dry_run"
			}
			data, encodeErr := json.Marshal(control.ShutdownRequest{Mode: mode, Countdown: expected.ManualCountdown})
			err = encodeErr
			body = bytes.NewReader(data)
		}
		if err == nil {
			err = u.api.Request(u.ctx, http.MethodPost, path, body, &result)
		}
		if u.ctx.Err() != nil {
			return
		}
		fyne.Do(func() {
			if u.closed.Load() {
				return
			}
			if err != nil {
				dialog.ShowError(err, u.Window)
			} else {
				dialog.ShowInformation("Policy", "Backend accepted: "+result.State+". This does not prove Windows completed shutdown.", u.Window)
			}
		})
	}()
}

func (u *UI) confirmShutdown() {
	p := u.last.UPS.Policy
	mode := "LIVE WINDOWS SHUTDOWN"
	if p.DryRun {
		mode = "DRY RUN (no OS shutdown)"
	}
	dialog.ShowConfirm("Confirm Windows shutdown", fmt.Sprintf("%s\nCountdown: %s\nApplications may block shutdown. Cancel works only before the OS request. UPS output remains on.", mode, p.ManualCountdown), func(yes bool) {
		if yes {
			u.command("/shutdown", &p)
		}
	}, u.Window)
}

func (u *UI) settings() {
	go func() {
		var value control.PolicySettings
		err := u.api.Request(u.ctx, http.MethodGet, "/policy", nil, &value)
		if u.ctx.Err() != nil {
			return
		}
		fyne.Do(func() {
			if u.closed.Load() {
				return
			}
			if err != nil {
				dialog.ShowError(err, u.Window)
				return
			}
			u.policyForm(value)
		})
	}()
}

func (u *UI) policyForm(settings control.PolicySettings) {
	value := settings.Options
	mode := widget.NewSelect([]string{"dry_run", "live"}, nil)
	mode.SetSelected("dry_run")
	if value.Mode != "" {
		mode.SetSelected(value.Mode)
	}
	automatic, low, manual := widget.NewCheck("Enable automatic shutdown", nil), widget.NewCheck("Verified low-battery trigger", nil), widget.NewCheck("Enable desktop manual shutdown", nil)
	outage, autoCountdown, manualCountdown := widget.NewEntry(), widget.NewEntry(), widget.NewEntry()
	outage.SetPlaceHolder("e.g. 5m; empty disables duration trigger")
	autoCountdown.SetPlaceHolder("5s to 10m; required when enabled")
	manualCountdown.SetPlaceHolder("5s to 10m; required when enabled")
	if value.Automatic != nil {
		automatic.SetChecked(value.Automatic.Enabled)
		low.SetChecked(value.Automatic.LowBattery)
		outage.SetText(value.Automatic.OutageAfter)
		autoCountdown.SetText(value.Automatic.Countdown)
	}
	if value.Manual != nil {
		manual.SetChecked(value.Manual.Enabled)
		manualCountdown.SetText(value.Manual.Countdown)
	}
	items := []*widget.FormItem{
		widget.NewFormItem("Execution mode", mode),
		widget.NewFormItem("", automatic), widget.NewFormItem("Outage duration", outage),
		widget.NewFormItem("", low), widget.NewFormItem("Automatic countdown", autoCountdown),
		widget.NewFormItem("", manual), widget.NewFormItem("Desktop countdown", manualCountdown),
		widget.NewFormItem("Other settings", widget.NewLabel("USB, hardware and Telegram setup remain in backend configuration.\nTelegram commands are checking-only. Pending/terminal policy cannot be reset here.")),
	}
	form := dialog.NewForm("PC shutdown policy", "Save policy", "Cancel", items, func(save bool) {
		if !save {
			return
		}
		next := policy.Options{Mode: mode.Selected,
			Automatic: &policy.AutomaticOptions{Enabled: automatic.Checked, OutageAfter: outage.Text, LowBattery: low.Checked, Countdown: autoCountdown.Text},
			Manual:    &policy.ManualOptions{Enabled: manual.Checked, Countdown: manualCountdown.Text}}
		if err := next.Validate(); err != nil {
			dialog.ShowError(err, u.Window)
			return
		}
		savePolicy := func() {
			go func() {
				data, err := json.Marshal(control.PolicySettings{Options: next, Revision: settings.Revision})
				var result policy.Status
				if err == nil {
					err = u.api.Request(u.ctx, http.MethodPut, "/policy", bytes.NewReader(data), &result)
				}
				if u.ctx.Err() != nil {
					return
				}
				fyne.Do(func() {
					if u.closed.Load() {
						return
					}
					if err != nil {
						dialog.ShowError(err, u.Window)
					} else {
						dialog.ShowInformation("Policy saved", "Durable policy saved and activated; automatic qualification restarts from fresh observation.", u.Window)
					}
				})
			}()
		}
		if next.Enabled() && next.Mode == "live" {
			dialog.ShowConfirm("Enable LIVE power controls?", "These settings can shut down Windows. Confirm the trigger and countdown values before continuing.", func(yes bool) {
				if yes {
					savePolicy()
				}
			}, u.Window)
		} else {
			savePolicy()
		}
	}, u.Window)
	form.Resize(fyne.NewSize(700, 500))
	form.Show()
}
