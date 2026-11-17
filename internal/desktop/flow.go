package desktop

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	flowUnknownColor = color.NRGBA{R: 128, G: 128, B: 128, A: 255}
	flowMainsColor   = color.NRGBA{R: 40, G: 120, B: 220, A: 255}
	flowBatteryColor = color.NRGBA{R: 65, G: 175, B: 75, A: 255}
)

type powerFlow struct {
	widget.BaseWidget
	mode       string
	fresh      bool
	phase      float64
	state      *canvas.Text
	details    *canvas.Text
	mainIn     *canvas.Line
	mainOut    *canvas.Line
	battery    *canvas.Line
	bypass     []*canvas.Line
	marker     *canvas.Circle
	ups        *canvas.Rectangle
	batteryBox *canvas.Rectangle
	labels     []*canvas.Text
}

func newPowerFlow() *powerFlow {
	d := &powerFlow{
		mode:    "unknown",
		state:   canvas.NewText("Power flow unknown", theme.Color(theme.ColorNameForeground)),
		details: canvas.NewText("Charge / bypass / inverter: not reported", theme.Color(theme.ColorNameForeground)),
		mainIn:  canvas.NewLine(flowUnknownColor),
		mainOut: canvas.NewLine(flowUnknownColor),
		battery: canvas.NewLine(flowUnknownColor),
		bypass: []*canvas.Line{
			canvas.NewLine(flowUnknownColor),
			canvas.NewLine(flowUnknownColor),
			canvas.NewLine(flowUnknownColor),
		},
		marker:     canvas.NewCircle(flowUnknownColor),
		ups:        canvas.NewRectangle(color.Transparent),
		batteryBox: canvas.NewRectangle(color.Transparent),
		labels: []*canvas.Text{
			canvas.NewText("AC input", theme.Color(theme.ColorNameForeground)),
			canvas.NewText("Output", theme.Color(theme.ColorNameForeground)),
			canvas.NewText("Bypass", theme.Color(theme.ColorNameForeground)),
			canvas.NewText("UPS", theme.Color(theme.ColorNameForeground)),
			canvas.NewText("Battery", theme.Color(theme.ColorNameForeground)),
		},
	}
	d.ups.StrokeColor = flowUnknownColor
	d.ups.StrokeWidth = 1
	d.batteryBox.StrokeColor = flowUnknownColor
	d.batteryBox.StrokeWidth = 1
	d.marker.Resize(fyne.NewSize(8, 8))
	d.ExtendBaseWidget(d)
	return d
}

func (d *powerFlow) SetState(mode string, fresh bool) {
	d.mode = mode
	d.fresh = fresh
	d.state.Text = "Power flow unknown"
	d.details.Text = "Charge / bypass / inverter: not reported"
	if fresh {
		switch mode {
		case "online":
			d.state.Text = "Mains online"
			d.details.Text = "Charge / bypass: not reported"
		case "on_battery":
			d.state.Text = "On battery"
			d.details.Text = "Inverter / bypass: not reported"
		}
	}
	d.Refresh()
}

func (d *powerFlow) Advance() {
	if !d.fresh || (d.mode != "online" && d.mode != "on_battery") {
		return
	}
	d.phase = math.Mod(d.phase+0.12, 1)
	d.Refresh()
}

func (d *powerFlow) pointOnPath(points []fyne.Position, progress float64) fyne.Position {
	segment := progress * float64(len(points)-1)
	index := int(segment)
	if index >= len(points)-1 {
		return points[len(points)-1]
	}
	fraction := segment - float64(index)
	from, to := points[index], points[index+1]
	return fyne.NewPos(
		from.X+(to.X-from.X)*float32(fraction),
		from.Y+(to.Y-from.Y)*float32(fraction),
	)
}

func (d *powerFlow) CreateRenderer() fyne.WidgetRenderer {
	objects := []fyne.CanvasObject{
		d.mainIn, d.mainOut, d.battery,
		d.bypass[0], d.bypass[1], d.bypass[2],
		d.ups, d.batteryBox,
		d.labels[0], d.labels[1], d.labels[2], d.labels[3], d.labels[4],
		d.state, d.details, d.marker,
	}
	return &powerFlowRenderer{diagram: d, objects: objects}
}

type powerFlowRenderer struct {
	diagram *powerFlow
	objects []fyne.CanvasObject
}

func (r *powerFlowRenderer) Layout(size fyne.Size) {
	d := r.diagram
	sx, sy := size.Width/240, size.Height/166
	point := func(x, y float32) fyne.Position { return fyne.NewPos(x*sx, y*sy) }
	setLine := func(line *canvas.Line, x1, y1, x2, y2 float32) {
		line.Position1, line.Position2 = point(x1, y1), point(x2, y2)
	}
	setLine(d.mainIn, 8, 55, 120, 55)
	setLine(d.mainOut, 120, 55, 232, 55)
	setLine(d.battery, 120, 77, 120, 100)
	setLine(d.bypass[0], 45, 55, 45, 20)
	setLine(d.bypass[1], 45, 20, 195, 20)
	setLine(d.bypass[2], 195, 20, 195, 55)
	d.ups.Move(point(65, 34))
	d.ups.Resize(fyne.NewSize(110*sx, 43*sy))
	d.batteryBox.Move(point(105, 100))
	d.batteryBox.Resize(fyne.NewSize(30*sx, 17*sy))
	positions := []fyne.Position{
		point(4, 38), point(197, 38), point(101, 2), point(104, 49), point(91, 120),
	}
	labelSizes := []fyne.Size{
		fyne.NewSize(45*sx, 14*sy), fyne.NewSize(43*sx, 14*sy),
		fyne.NewSize(60*sx, 14*sy), fyne.NewSize(110*sx, 14*sy),
		fyne.NewSize(60*sx, 14*sy),
	}
	for i, label := range d.labels {
		label.Move(positions[i])
		label.Resize(labelSizes[i])
		label.TextSize = 9
		label.Alignment = fyne.TextAlignCenter
	}
	d.state.Move(point(0, 139))
	d.state.Resize(fyne.NewSize(240*sx, 14*sy))
	d.state.TextSize = 11
	d.state.Alignment = fyne.TextAlignCenter
	d.details.Move(point(0, 153))
	d.details.Resize(fyne.NewSize(240*sx, 12*sy))
	d.details.TextSize = 9
	d.details.Alignment = fyne.TextAlignCenter
	d.marker.Resize(fyne.NewSize(8*sx, 8*sy))
	d.positionMarker(size)
	r.refreshColors()
}

func (d *powerFlow) positionMarker(size fyne.Size) {
	sx, sy := size.Width/240, size.Height/166
	points := []fyne.Position{fyne.NewPos(45, 55), fyne.NewPos(120, 55), fyne.NewPos(195, 55)}
	if d.mode == "on_battery" {
		points = []fyne.Position{fyne.NewPos(120, 100), fyne.NewPos(120, 55), fyne.NewPos(195, 55)}
	}
	p := d.pointOnPath(points, d.phase)
	d.marker.Move(fyne.NewPos(p.X*sx, p.Y*sy))
}

func (r *powerFlowRenderer) refreshColors() {
	d := r.diagram
	foreground := theme.Color(theme.ColorNameForeground)
	d.state.Color = foreground
	d.details.Color = foreground
	for _, label := range d.labels {
		label.Color = foreground
	}
	for _, line := range d.bypass {
		line.StrokeColor = flowUnknownColor
	}
	active := flowUnknownColor
	batteryActive := false
	if d.fresh && d.mode == "online" {
		active = flowMainsColor
	} else if d.fresh && d.mode == "on_battery" {
		active = flowBatteryColor
		batteryActive = true
	}
	d.mainIn.StrokeColor = active
	d.mainOut.StrokeColor = active
	d.battery.StrokeColor = flowUnknownColor
	if batteryActive {
		d.battery.StrokeColor = active
		d.mainIn.StrokeColor = flowUnknownColor
	}
	d.marker.FillColor = active
	d.ups.StrokeColor = active
	d.batteryBox.StrokeColor = flowUnknownColor
	if batteryActive {
		d.batteryBox.StrokeColor = active
	}
}

func (r *powerFlowRenderer) MinSize() fyne.Size { return fyne.NewSize(240, 210) }
func (r *powerFlowRenderer) Refresh() {
	r.refreshColors()
	r.diagram.positionMarker(r.diagram.Size())
	for _, object := range r.objects {
		canvas.Refresh(object)
	}
}
func (r *powerFlowRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *powerFlowRenderer) Destroy()                     {}
