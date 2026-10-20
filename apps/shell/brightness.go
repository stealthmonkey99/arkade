// SPDX-License-Identifier: GPL-2.0-or-later

package shell

import (
	"fmt"

	"github.com/stealthmonkey99/arkade/internal/config"
	"github.com/stealthmonkey99/arkade/internal/engine"
)

const brightnessStep = 5

var brightnessPanel = &levelPanel{
	adjust:  func(a *app, delta int) { a.bumpBrightness(delta * brightnessStep) },
	toggle:  (*app).cycleLamps,
	paint:   drawBrightness,
	readout: brightnessLCD,
}

// initBrightness reads the runtime, which the --brightness flag and hotkeys change behind the shell.
func (a *app) initBrightness() {
	a.st.brightness = a.ctx.RT.PanelBrightness()
	a.st.lamps = a.ctx.RT.LampPolicy()
}

// bumpBrightness steps the panel level, applies it and persists it. The floor
// is engine.MinPanelBrightness, not 0: a blank panel reads as a crashed cabinet.
func (a *app) bumpBrightness(delta int) {
	next := min(100, max(engine.MinPanelBrightness, a.st.brightness+delta))
	if next == a.st.brightness {
		a.ctx.RejectKey()
		return
	}
	a.st.brightness = next
	a.ctx.RT.SetPanelBrightness(a.st.brightness)
	a.saveBrightness()
}

// cycleLamps steps the lamp policy: auto, on, off.
func (a *app) cycleLamps() {
	switch a.st.lamps {
	case engine.LampsMatchScreen:
		a.st.lamps = engine.LampsOn
	case engine.LampsOn:
		a.st.lamps = engine.LampsOff
	default:
		a.st.lamps = engine.LampsMatchScreen
	}
	a.ctx.RT.SetLampPolicy(a.st.lamps)
	a.saveBrightness()
	a.applyLights()
}

// lampsLabel is at most ten columns, leaving the LCD legend room for the button colour.
func lampsLabel(p engine.LampPolicy) string {
	switch p {
	case engine.LampsOff:
		return "LAMPS OFF"
	case engine.LampsOn:
		return "LAMPS ON"
	default:
		return "LAMPS AUTO"
	}
}

func (a *app) saveBrightness() {
	a.saveCabinet(func(c *config.Cabinet) { c.Brightness, c.Lamps = a.st.brightness, a.st.lamps })
}

// brightnessTileLCD is the readout while the BRIGHT tile is focused on the shelf.
func brightnessTileLCD(a *app) (string, string) {
	return "BRIGHTNESS", fmt.Sprintf("level: %d%%", a.st.brightness)
}

func brightnessLCD(st *browseState) (lines [2][2]string) {
	lines[0][0] = "BRIGHTNESS"
	lines[0][1] = lcdBar(st.brightness)
	lines[1][0] = "LEFT/RIGHT ADJ"
	lines[1][1] = "BLUE: " + lampsLabel(st.lamps)
	return lines
}
