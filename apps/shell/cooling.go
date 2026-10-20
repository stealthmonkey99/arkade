// SPDX-License-Identifier: GPL-2.0-or-later

package shell

import (
	"fmt"
	"strings"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/config"
	"github.com/stealthmonkey99/arkade/internal/hardware"
)

const (
	fanThresholdStep = 5

	// cpuTempRefreshMs throttles the readout; the Pi samples the thermal zone every 10s.
	cpuTempRefreshMs = 1000

	// cpuTempNone is shown when the suite has no thermal sensor.
	cpuTempNone = "--"
)

var fanPanel = &levelPanel{
	adjust:  func(a *app, delta int) { a.bumpFanThreshold(delta * fanThresholdStep) },
	toggle:  (*app).cycleFanMode,
	paint:   drawFan,
	readout: fanLCD,
}

// initFan seeds the panel from the stored cooling config without applying it to the hardware.
func (a *app) initFan(cab config.Cabinet) {
	a.st.fanThreshold, a.st.fanMode = cab.FanThresholdF, cab.FanMode
	a.st.cpuTemp = cpuTempNone
}

// bumpFanThreshold steps the trip point, applies it and persists it.
func (a *app) bumpFanThreshold(delta int) {
	next := config.ClampFanThreshold(a.st.fanThreshold + delta)
	if next == a.st.fanThreshold {
		a.ctx.RejectKey()
		return
	}
	a.st.fanThreshold = next
	a.applyFan()
	a.saveFan()
}

// cycleFanMode steps the fan mode: auto, on, off.
func (a *app) cycleFanMode() {
	switch a.st.fanMode {
	case hardware.FanAuto:
		a.st.fanMode = hardware.FanOn
	case hardware.FanOn:
		a.st.fanMode = hardware.FanOff
	default:
		a.st.fanMode = hardware.FanAuto
	}
	a.applyFan()
	a.saveFan()
}

// fansLabel is at most nine columns.
func fansLabel(m hardware.FanMode) string {
	switch m {
	case hardware.FanOn:
		return "FANS ON"
	case hardware.FanOff:
		return "FANS OFF"
	default:
		return "FANS AUTO"
	}
}

func (a *app) applyFan() {
	a.ctx.RT.Hardware.SetFanConfig(float64(a.st.fanThreshold), a.st.fanMode)
}

func (a *app) saveFan() {
	a.saveCabinet(func(c *config.Cabinet) { c.FanThresholdF, c.FanMode = a.st.fanThreshold, a.st.fanMode })
}

// refreshCPUTemp re-reads the SoC temperature every cpuTempRefreshMs.
func (a *app) refreshCPUTemp(stats clock.FrameTime) {
	if a.tempPrimed && stats.DurationMs-a.tempAt < cpuTempRefreshMs {
		return
	}
	a.tempPrimed, a.tempAt = true, stats.DurationMs
	a.st.cpuTemp, a.st.cpuTempOK = cpuTempNone, false
	tr, ok := a.ctx.RT.Hardware.(hardware.ThermalSensor)
	if !ok {
		return
	}
	degF, ok := tr.CPUTempF()
	if !ok {
		return
	}
	a.st.cpuTempF, a.st.cpuTempOK = degF, true
	a.st.cpuTemp = fmt.Sprintf("%.0f°F", degF)
}

// lcdTemp strips the degree sign, which the character LCDs cannot render.
func lcdTemp(s string) string { return strings.ReplaceAll(s, "°", "") }

// fanTileLCD is the readout while the TEMP tile is focused on the shelf.
func fanTileLCD(a *app) (string, string) {
	switch a.st.fanMode {
	case hardware.FanOff:
		return "CPU TEMP", lcdTemp(a.st.cpuTemp) + " fans off"
	case hardware.FanOn:
		return "CPU TEMP", lcdTemp(a.st.cpuTemp) + " fans on"
	default:
		return "CPU TEMP", fmt.Sprintf("%s fan>=%d", lcdTemp(a.st.cpuTemp), a.st.fanThreshold)
	}
}

func fanLCD(st *browseState) (lines [2][2]string) {
	lines[0][0] = "COOLING"
	lines[0][1] = "cpu " + lcdTemp(st.cpuTemp)
	switch st.fanMode {
	case hardware.FanOn:
		lines[1][0] = "- fans on -"
	case hardware.FanOff:
		lines[1][0] = "- fans off -"
	default:
		lines[1][0] = fmt.Sprintf("fan at %dF", st.fanThreshold)
	}
	lines[1][1] = "BLUE: " + fansLabel(st.fanMode)
	return lines
}
