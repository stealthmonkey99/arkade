// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"math"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// MinPanelBrightness is the brightness floor: a panel dimmed to nothing looks
// like a crashed cabinet and cannot be read to recover.
const MinPanelBrightness = 10

// panelLevel is the LED panel's brightness as the engine owns it: the
// operator's percent, a screensaver's dim over it with the level to give
// back, and the wake bloom's ramp under both. Outside a bloom the renderer
// owns the live value, so the console's dim hotkeys keep working; the engine
// re-reads it whenever it takes over.
type panelLevel struct {
	pct     int
	dimmed  bool
	restore int
	bloom   lampFade
}

// set clamps pct into the operator's range, MinPanelBrightness..100.
func (p *panelLevel) set(pct int) {
	p.pct = min(100, max(MinPanelBrightness, pct))
}

// dim takes the panel to pct, 1..100, remembering current to restore on the
// first dim only.
func (p *panelLevel) dim(pct, current int) {
	if !p.dimmed {
		p.restore, p.dimmed = current, true
	}
	p.pct = min(100, max(1, pct))
}

// clearDim ends a dim and reports the level to restore, once.
func (p *panelLevel) clearDim() (restore int, ok bool) {
	if !p.dimmed {
		return 0, false
	}
	p.dimmed = false
	return p.restore, true
}

// value is the renderer brightness, 0..1, with any bloom folded in.
func (p panelLevel) value() float64 {
	v := float64(p.pct) / 100
	if p.bloom.active {
		v *= p.bloom.value() / 100
	}
	return v
}

// reported is the level a reader should see: the intended one during a
// bloom, so nothing captures the ramp as a setting, else the live one.
func (p panelLevel) reported(live int) int {
	if p.bloom.active {
		return p.pct
	}
	return live
}

// startBloom ramps from dark, or from where a bloom in flight has reached,
// to current over fadeMs along ease.
func (p *panelLevel) startBloom(current, fadeMs int, ease Ease) {
	p.pct = current
	from := 0.0
	if p.bloom.active {
		from = p.bloom.value()
	}
	p.bloom = lampFade{from: from, to: 100, total: uint64(fadeMs) * 1000, ease: ease, active: true}
}

// endBloom lands a bloom in flight at full and reports whether one was.
func (p *panelLevel) endBloom() bool {
	if !p.bloom.active {
		return false
	}
	p.bloom = lampFade{}
	return true
}

// stepBloom advances a bloom by dtMicros and reports whether one was running.
func (p *panelLevel) stepBloom(dtMicros uint64) bool {
	if !p.bloom.active {
		return false
	}
	p.bloom.elapsed += dtMicros
	if p.bloom.elapsed >= p.bloom.total {
		p.bloom = lampFade{}
	}
	return true
}

// SetPanelBrightness sets the LED panel brightness to pct percent, clamped
// to MinPanelBrightness..100.
func (rt *Runtime) SetPanelBrightness(pct int) {
	rt.panel.set(pct)
	rt.applyPanelBrightness()
	rt.hwShadow.setPanelBrightness(rt.panel.pct)
}

// PanelBrightness reports the LED panel brightness as a percent. During a
// wake bloom it reports the intended level, not the ramp, so nothing captures
// the ramp as the operator's setting.
func (rt *Runtime) PanelBrightness() int {
	return rt.panel.reported(int(math.Round(rt.Renderer.Brightness() * 100)))
}

func (rt *Runtime) applyPanelBrightness() { rt.Renderer.SetBrightness(rt.panel.value()) }

// LampPolicy is the operator's standing answer to how bright the button
// lamps run, whatever the app asks for.
type LampPolicy string

const (
	// LampsOff keeps the lamps and the character-LCD backlight dark.
	LampsOff LampPolicy = "off"
	// LampsOn runs every lamp at the level the app wrote.
	LampsOn LampPolicy = "on"
	// LampsMatchScreen dims them with the LED panel.
	LampsMatchScreen LampPolicy = "match"
)

// ParseLampPolicy reads a persisted policy name, reporting whether it is
// one; unknown names (including "") leave the caller to pick a default.
func ParseLampPolicy(s string) (LampPolicy, bool) {
	switch LampPolicy(s) {
	case LampsOff, LampsOn, LampsMatchScreen:
		return LampPolicy(s), true
	}
	return LampsMatchScreen, false
}

// SetLampPolicy applies a lamp policy live, re-scaling the lamps. LampsOff
// also darkens the LCD backlight; leaving it restores the backlight only as
// far as the standby state allows.
func (rt *Runtime) SetLampPolicy(p LampPolicy) {
	rt.hwShadow.setLampPolicy(p)
}

// LampPolicy reports the policy in effect.
func (rt *Runtime) LampPolicy() LampPolicy { return rt.hwShadow.lampPolicy() }

// FadeLamps ramps the lamps named by touch (bit i = hardware.LightButtons[i])
// on seat index (-1 = both) to levels over fadeMs milliseconds; fadeMs <= 0
// lands instantly. The app's intent becomes levels at once, so a resume replay
// shows the end state, and any later write to a lamp cancels its fade.
func (rt *Runtime) FadeLamps(index int, levels hardware.LampLevels, touch hardware.LightMask, fadeMs int) {
	rt.FadeLampsEased(index, levels, touch, fadeMs, Linear)
}

// FadeLampsEased is FadeLamps along a named curve.
func (rt *Runtime) FadeLampsEased(index int, levels hardware.LampLevels, touch hardware.LightMask, fadeMs int, ease Ease) {
	rt.hwShadow.fadeLamps(index, levels, touch, fadeMs, ease)
}

// SetBacklightLevel sets the character-LCD backlight to level (0 dark to
// hardware.LampLevelMax full), cancelling any fade in flight. The operator's
// lamp policy scales it.
func (rt *Runtime) SetBacklightLevel(level uint8) {
	if level == 0 && rt.backlightHeld() {
		return
	}
	rt.hwShadow.SetBacklightLevel(level)
}

// FadeBacklight ramps the character-LCD backlight to level over fadeMs
// milliseconds; fadeMs <= 0 lands instantly. A later SetBacklightLevel cancels
// it.
func (rt *Runtime) FadeBacklight(level uint8, fadeMs int) {
	rt.FadeBacklightEased(level, fadeMs, Linear)
}

// FadeBacklightEased is FadeBacklight along a named curve.
func (rt *Runtime) FadeBacklightEased(level uint8, fadeMs int, ease Ease) {
	if level == 0 && rt.backlightHeld() {
		return
	}
	rt.hwShadow.fadeBacklight(level, fadeMs, ease)
}

func (rt *Runtime) stepLampFades(dtMicros uint64) { rt.hwShadow.stepFades(dtMicros) }
