// SPDX-License-Identifier: GPL-2.0-or-later

package engine

// startWakeBloom brings the cabinet up from dark over fadeMs, or restarts a
// bloom in flight from its current level along a new curve. The bloom is two
// ramps: shadowHardware.bloom scales lamps and backlight, panelLevel.bloom
// scales the renderer. Wakes that were already lit (ATTRACT to ACTIVE) start
// none.
func (rt *Runtime) startWakeBloom(fadeMs int, ease Ease) {
	rt.panel.startBloom(rt.PanelBrightness(), fadeMs, ease)
	rt.applyPanelBrightness()
	rt.hwShadow.startBloom(fadeMs, ease)
}

// hurryWakeBloom finishes a bloom already rising at the press speed rather
// than snapping it to full.
func (rt *Runtime) hurryWakeBloom() {
	if rt.panel.bloom.active {
		rt.startWakeBloom(pressBloomMs, EaseOut)
	}
}

// endWakeBloom lands a bloom in flight at full immediately. enterDark does
// not call it: cancelling there would snap the lamps to full for a frame
// before the goodnight fade took hold.
func (rt *Runtime) endWakeBloom() {
	if rt.panel.endBloom() {
		rt.applyPanelBrightness()
	}
	rt.hwShadow.endBloom()
}

func (rt *Runtime) stepWakeBloom(dtMicros uint64) {
	if rt.panel.stepBloom(dtMicros) {
		rt.applyPanelBrightness()
	}
}
