package desktop

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

//go:embed car-battery.png
var carBatteryPNG []byte

var appIcon = fyne.NewStaticResource("car-battery.png", carBatteryPNG)

func AppIcon() fyne.Resource { return appIcon }
