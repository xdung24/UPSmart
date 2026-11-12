//go:build windows

package main

import (
	"flag"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"upsmart/internal/control"
	"upsmart/internal/desktop"
)

func main() {
	smoke := flag.Bool("smoke-test", false, "show the desktop briefly, then quit; no backend/control actions")
	flag.Parse()
	client := control.NewClient()
	defer client.Close()
	app := app.NewWithID("local.upsmart.monitor")
	ui := desktop.New(app, client)
	ui.Show()
	if *smoke {
		go func() {
			time.Sleep(3 * time.Second)
			fyne.Do(ui.Quit)
		}()
	} else {
		ui.Start()
	}
	app.Run()
}
