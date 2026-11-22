//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"upsmart/internal/control"
	"upsmart/internal/desktop"
	"upsmart/internal/monitor"
	"upsmart/internal/ups"
)

func main() {
	if handled, err := ups.HandleHelper(os.Args[1:], os.Stdout); handled {
		if err != nil {
			log.Print(err)
			os.Exit(1)
		}
		return
	}
	smoke := flag.Bool("smoke-test", false, "show the desktop briefly, then quit; no backend/control actions")
	configPath := flag.String("config", "", "YAML configuration for direct USB monitoring")
	flag.Parse()
	app := app.NewWithID("local.upsmart.monitor")
	app.SetIcon(desktop.AppIcon())
	if flag.NArg() != 0 {
		showError(app.NewWindow("UPSmart Monitor - connection error"), app, fmt.Errorf("unexpected positional arguments"))
		app.Run()
		return
	}
	client := control.NewClient()
	defer client.Close()
	if *smoke {
		ui := desktop.New(app, client)
		ui.Show()
		go func() {
			time.Sleep(3 * time.Second)
			fyne.Do(ui.Quit)
		}()
		app.Run()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.Lifecycle().SetOnStopped(cancel)
	loading := app.NewWindow("UPSmart Monitor")
	loading.SetContent(widget.NewLabel("Trying daemon named pipe...\nIf no monitor owns the UPS, direct USB monitoring will start.\nClose this window to cancel startup."))
	loading.Resize(fyne.NewSize(640, 150))
	loading.Show()
	var session *monitor.Session
	connected := make(chan struct{})
	go func() {
		defer close(connected)
		var err error
		connections := monitor.Connections{Pipe: client}
		session, err = monitor.Connect(ctx, connections, func(ctx context.Context, ready func()) error {
			cfg, err := monitor.DirectConfig(*configPath)
			if err != nil {
				return err
			}
			executable, err := os.Executable()
			if err != nil {
				return err
			}
			return monitor.Run(ctx, cfg, ups.Worker{Executable: executable}, ready)
		})
		fyne.Do(func() {
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				showError(loading, app, err)
				return
			}
			ui := desktop.New(app, session)
			ui.SetDirect(session.Direct())
			ui.Show()
			ui.Start()
			loading.Close()
		})
	}()
	app.Run()
	cancel()
	<-connected
	if session != nil {
		if err := session.Close(); err != nil {
			log.Printf("stop direct USB monitor: %v", err)
		}
	}
}

func showError(window fyne.Window, app fyne.App, err error) {
	log.Print(err)
	window.SetTitle("UPSmart Monitor - connection error")
	label := widget.NewLabel(err.Error() + "\n\nNo Windows service is required. Start a daemon with desktop IPC configured, or allow this desktop to own direct USB monitoring.\nDirect USB cannot take ownership from a running monitor; close legacy UPSmart before direct monitoring.")
	label.Wrapping = fyne.TextWrapWord
	window.SetContent(container.NewBorder(nil, widget.NewButton("Quit", app.Quit), nil, nil, label))
	window.Resize(fyne.NewSize(640, 240))
	window.Show()
}
