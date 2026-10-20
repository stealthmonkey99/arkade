// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"time"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// Peripheral timings for the power-off sequence; powerOffQuitMs is the
// deadline for the whole thing.
const (
	powerOffBacklightMs = 1500
	powerOffWinkMs      = 250
	powerOffLampOutMs   = 600
	powerOffQuitMs      = 2200
)

// powerOffBacklightLevel is where the backlight settles: the PCA9685 holds its
// PWM after the process exits, so the goodbye stays readable.
const powerOffBacklightLevel = hardware.LampLevelMax * 30 / 100

// powerOffLampOut is the boot bloom in reverse, one colour group every 150ms,
// both seats together.
var powerOffLampOut = []struct {
	at    float64
	touch hardware.LightMask
}{
	{400, 1<<4 | 1<<5}, // aux
	{550, 1 << 3},      // blue
	{700, 1 << 2},      // yellow
	{850, 1 << 1},      // red
	{1000, 1 << 0},     // green
}

type powerOffSeq struct {
	start time.Time
	last  display.Matrix
	group int
}

// PowerOff plays the shutdown sequence (LCD goodbye, lamps out, panel
// collapse) and then requests a quit; Quit is the abrupt route, for SIGTERM.
// The sequence starts at the top of the next tick, since an app cannot be
// unloaded from inside its own input dispatch.
//
// Safe from any goroutine. Input during the sequence is discarded, it cannot
// be cancelled, and a second call while one plays is ignored.
func (rt *Runtime) PowerOff() { rt.requests.powerOff.Set(struct{}{}) }

func (rt *Runtime) applyPowerOff(time.Time, struct{}) {
	if !rt.poweringOff() {
		rt.setTransition(transition{kind: transitionPowerOff})
	}
}

func (rt *Runtime) startPowerOff() {
	// Composed before the unload, while the app's layers still exist.
	rt.powerOff = powerOffSeq{start: rt.frames.tickStart, last: rt.Display.Compose(rt.Stats())}
	rt.Log("info", "power off: sequence started")
	rt.endWakeBloom() // the sequence lights its own lamps; nothing scales them
	rt.unloadCurrent()
	rt.powerDown.notify("shutdown")
	rt.Hardware.SetText(-1, hardware.PowerOffLine1, hardware.PowerOffLine2)
	rt.FadeBacklightEased(powerOffBacklightLevel, powerOffBacklightMs, EaseInOut)
	rt.FadeLampsEased(-1, hardware.LampLevelsOf(hardware.AllLights), hardware.AllLights, powerOffWinkMs, EaseOut)
}

func (rt *Runtime) poweringOff() bool { return !rt.powerOff.start.IsZero() }

// stepPowerOff releases due lamp groups, paints one frame, and requests the
// quit at the end. It reports whether a sequence is in flight.
func (rt *Runtime) stepPowerOff(now time.Time) bool {
	if !rt.poweringOff() {
		return false
	}
	elapsed := float64(now.Sub(rt.powerOff.start).Microseconds()) / 1000
	for rt.powerOff.group < len(powerOffLampOut) && elapsed >= powerOffLampOut[rt.powerOff.group].at {
		g := powerOffLampOut[rt.powerOff.group]
		rt.powerOff.group++
		rt.FadeLampsEased(-1, hardware.LampLevels{}, g.touch, powerOffLampOutMs, EaseInOut)
	}
	rt.Renderer.Paint(powerOffFrame(rt.powerOff.last, elapsed), rt.Stats())
	if elapsed >= powerOffQuitMs {
		rt.halt.request()
	}
	return true
}
