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
	configPath := flag.String("config", "", "configuration for web address discovery and direct USB fallback")
	webAddress := flag.String("web-address", "", "optional daemon web address, e.g. 127.0.0.1:18765; otherwise discovered from local config")
	flag.Parse()
	app := app.NewWithID("local.upsmart.monitor")
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
	loading.SetContent(widget.NewLabel("Trying named pipe, then localhost web API...\nIf no monitor owns the UPS, direct USB monitoring will start.\nClose this window to cancel startup."))
	loading.Resize(fyne.NewSize(640, 150))
	loading.Show()
	var session *monitor.Session
	var webClient *control.WebClient
	defer func() {
		if webClient != nil {
			webClient.Close()
		}
	}()
	connected := make(chan struct{})
	go func() {
		defer close(connected)
		var err error
		connections := monitor.Connections{Pipe: client, Web: func() (monitor.Client, error) {
			address := *webAddress
			if address == "" {
				executable, err := os.Executable()
				if err != nil {
					return nil, err
				}
				address, err = monitor.WebAddress(*configPath, executable)
				if err != nil {
					return nil, err
				}
			}
			if address == "" {
				return nil, nil
			}
			var err error
			webClient, err = control.NewWebClient(address)
			if err != nil {
				return nil, err
			}
			return webClient, nil
		}}
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
			ui.SetReadOnly(session.ReadOnly())
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
	label := widget.NewLabel(err.Error() + "\n\nNo Windows service is required. A daemon can provide a named pipe or localhost web API.\nUse --web-address 127.0.0.1:PORT or --config PATH for its address.\nDirect USB cannot take ownership from a running monitor; close legacy UPSmart before direct monitoring.")
	label.Wrapping = fyne.TextWrapWord
	window.SetContent(container.NewBorder(nil, widget.NewButton("Quit", app.Quit), nil, nil, label))
	window.Resize(fyne.NewSize(640, 240))
	window.Show()
}
