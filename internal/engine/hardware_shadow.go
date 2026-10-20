// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"math"
	"sync"
	"time"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// shadowHardware is the engine's one road to the LCDs and lamps. A write
// passes through, in order:
//
//  1. the mirror: texts and levels record the app's intent per seat, so a
//     suspended app's peripherals can be snapshotted and replayed on resume;
//  2. the holds: textHeld parks text while a lab preview owns the LCDs and
//     lightsHeld parks lamps during a light show, taken independently; the
//     mirror keeps recording under a hold and a release replays it;
//  3. the fades: per-lamp and backlight ramps animate the device value
//     toward the mirrored target, which jumps there at once;
//  4. the scale: the operator's lamp policy, the panel brightness it may
//     follow, and a wake bloom multiply every lamp and backlight level on
//     the way out, so app writes still land during a bloom;
//  5. the dedupe: a device value equal to the last one written is dropped,
//     so apps need no write caches of their own.
//
// Overrides (a preview's text, a show's lamps) enter at step 4, past the
// mirror and the holds, and record what they put on the device so the
// release's replay is deduped against it.
//
// Optional suite capabilities are forwarded with hardware.Virtual's answers
// when the inner suite lacks them, so the engine never asserts on Hardware.
type shadowHardware struct {
	inner hardware.Suite

	mu     sync.Mutex
	texts  [2][2]string
	levels [2]hardware.LampLevels

	textHeld   bool
	lightsHeld bool

	policy     LampPolicy
	brightness int // panel percent, the scale under LampsMatchScreen
	bloom      lampFade

	fades         [2][6]lampFade
	backlight     uint8
	backlightFade lampFade
	// backlightTouched holds off device writes until something sets the
	// backlight, so a dark boot stays dark until the boot screen fades in.
	backlightTouched bool

	pushed             [2]hardware.LampLevels // last unscaled level per seat, for re-scaling
	device             [2]hardware.LampLevels
	deviceSet          [2]bool
	deviceText         [2][2]string
	deviceTextSet      [2]bool
	deviceBacklight    uint8
	deviceBacklightSet bool
}

func newShadowHardware(inner hardware.Suite) *shadowHardware {
	s := &shadowHardware{
		inner:      inner,
		policy:     LampsMatchScreen,
		brightness: 100,
		backlight:  hardware.LampLevelMax, // the cabinet powers up lit
	}
	// The startup card hands the panels over part-lit; the boot fade must
	// start from there rather than snap.
	if r, ok := inner.(hardware.BacklightReader); ok {
		s.backlight = r.BacklightLevel()
	}
	return s
}

type hardwareState struct {
	texts  [2][2]string
	levels [2]hardware.LampLevels
}

func (s *shadowHardware) snapshot() hardwareState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return hardwareState{texts: s.texts, levels: s.levels}
}

// restore replays a snapshot through the normal write path, cancelling any
// fade in flight.
func (s *shadowHardware) restore(st hardwareState) {
	for p := 0; p < 2; p++ {
		s.SetText(p, st.texts[p][0], st.texts[p][1])
		s.SetLampLevels(p, st.levels[p])
	}
}

func lightIndex(name string) int {
	for i, b := range hardware.LightButtons {
		if b == name {
			return i
		}
	}
	return -1
}

// covered reports whether seat p is addressed by index; -1 is both.
func covered(index, p int) bool { return index < 0 || index == p }

// textWrite is one seat's pending LCD write, decided under the lock and
// performed outside it.
type textWrite struct {
	due   bool
	lines [2]string
}

func (s *shadowHardware) SetText(index int, line1, line2 string) {
	s.mu.Lock()
	for p := range s.texts {
		if covered(index, p) {
			s.texts[p] = [2]string{line1, line2}
		}
	}
	writes := s.textWritesLocked(index)
	s.mu.Unlock()
	s.writeText(writes)
}

// textWritesLocked decides which addressed seats' mirrored text must reach
// the device: none under a hold, otherwise every seat whose device text
// differs or was never written. Deciding marks the device as written.
func (s *shadowHardware) textWritesLocked(index int) (writes [2]textWrite) {
	if s.textHeld {
		return writes
	}
	for p := range s.texts {
		if !covered(index, p) {
			continue
		}
		if s.deviceTextSet[p] && s.deviceText[p] == s.texts[p] {
			continue
		}
		s.deviceText[p], s.deviceTextSet[p] = s.texts[p], true
		writes[p] = textWrite{due: true, lines: s.texts[p]}
	}
	return writes
}

// writeText performs the decided writes, as one both-seats call when the
// seats agree.
func (s *shadowHardware) writeText(writes [2]textWrite) {
	if writes[0].due && writes[1].due && writes[0].lines == writes[1].lines {
		s.inner.SetText(-1, writes[0].lines[0], writes[0].lines[1])
		return
	}
	for p, w := range writes {
		if w.due {
			s.inner.SetText(p, w.lines[0], w.lines[1])
		}
	}
}

// holdText is idempotent. Engine goroutine only.
func (s *shadowHardware) holdText() {
	s.mu.Lock()
	s.textHeld = true
	s.mu.Unlock()
}

// releaseText replays the mirror; a no-op without a hold, so an unbalanced
// release cannot repaint the LCDs.
func (s *shadowHardware) releaseText() {
	s.mu.Lock()
	if !s.textHeld {
		s.mu.Unlock()
		return
	}
	s.textHeld = false
	writes := s.textWritesLocked(-1)
	s.mu.Unlock()
	s.writeText(writes)
}

// overrideText writes past the mirror and the hold, so the preview never
// becomes what releaseText replays.
func (s *shadowHardware) overrideText(index int, line1, line2 string) {
	s.mu.Lock()
	for p := range s.deviceText {
		if covered(index, p) {
			s.deviceText[p], s.deviceTextSet[p] = [2]string{line1, line2}, true
		}
	}
	s.mu.Unlock()
	s.inner.SetText(index, line1, line2)
}

func (s *shadowHardware) SetLights(index int, on bool) {
	var lv hardware.LampLevels
	if on {
		for i := range lv {
			lv[i] = hardware.LampLevelMax
		}
	}
	s.SetLampLevels(index, lv)
}

func (s *shadowHardware) SetButtonLights(index int, lights hardware.ButtonLights, only bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for p := range s.levels {
		if !covered(index, p) {
			continue
		}
		if only {
			s.levels[p] = hardware.LampLevels{}
			s.fades[p] = [6]lampFade{}
		}
		for name, on := range lights {
			if j := lightIndex(name); j >= 0 {
				s.levels[p][j] = onLevel(on)
				s.fades[p][j] = lampFade{}
			}
		}
	}
	s.pushLocked(index)
}

func (s *shadowHardware) SetLampLevels(index int, levels hardware.LampLevels) {
	levels = levels.Clamp()
	s.mu.Lock()
	defer s.mu.Unlock()
	for p := range s.levels {
		if !covered(index, p) {
			continue
		}
		s.levels[p] = levels
		s.fades[p] = [6]lampFade{}
	}
	s.pushLocked(index)
}

func onLevel(on bool) uint8 {
	if on {
		return hardware.LampLevelMax
	}
	return 0
}

// pushLocked writes the addressed seats from the mirror and any fades in
// flight, unless a light show holds the lamps.
func (s *shadowHardware) pushLocked(index int) {
	if s.lightsHeld {
		return
	}
	for p := range s.levels {
		if covered(index, p) {
			s.pushSeatLocked(p, s.shownLocked(p))
		}
	}
}

// pushSeatLocked is the one road to the lamps, scaled here by policy.
func (s *shadowHardware) pushSeatLocked(p int, lv hardware.LampLevels) {
	s.pushed[p] = lv
	eff := scaleLamps(lv, s.scalePctLocked())
	if s.deviceSet[p] && eff == s.device[p] {
		return
	}
	s.device[p], s.deviceSet[p] = eff, true
	s.inner.SetLampLevels(p, eff)
}

// pushBacklightLocked scales the backlight by the lamp policy too: a dimmed
// panel beside full-bright LCDs reads as a fault.
func (s *shadowHardware) pushBacklightLocked() {
	if !s.backlightTouched {
		return
	}
	level := scaleLevel(s.shownBacklightLocked(), s.scalePctLocked())
	if s.deviceBacklightSet && level == s.deviceBacklight {
		return
	}
	s.deviceBacklight, s.deviceBacklightSet = level, true
	if d, ok := s.inner.(hardware.BacklightDimmer); ok {
		d.SetBacklightLevel(level)
	}
}

func (s *shadowHardware) repushLocked() {
	for p := range s.pushed {
		s.pushSeatLocked(p, s.pushed[p])
	}
	s.pushBacklightLocked()
}

func (s *shadowHardware) scalePctLocked() int {
	pct := s.brightness
	switch s.policy {
	case LampsOff:
		return 0
	case LampsOn:
		pct = 100
	}
	if b := s.bloomPctLocked(); b < 100 {
		pct = int(math.Round(float64(pct) * float64(b) / 100))
	}
	return pct
}

func scaleLamps(lv hardware.LampLevels, pct int) hardware.LampLevels {
	var out hardware.LampLevels
	for i, v := range lv {
		out[i] = scaleLevel(v, pct)
	}
	return out
}

// scaleLevel never scales a lit lamp to zero: a vanished lamp reads as a dead
// button.
func scaleLevel(v uint8, pct int) uint8 {
	if v == 0 || pct <= 0 {
		return 0
	}
	scaled := int(math.Round(float64(v) * float64(pct) / 100))
	return uint8(min(hardware.LampLevelMax, max(1, scaled)))
}

func (s *shadowHardware) setLampPolicy(p LampPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.policy == p {
		return
	}
	s.policy = p
	s.repushLocked()
}

func (s *shadowHardware) lampPolicy() LampPolicy {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.policy
}

// setPanelBrightness records the panel level LampsMatchScreen follows.
func (s *shadowHardware) setPanelBrightness(pct int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.brightness == pct {
		return
	}
	s.brightness = pct
	if s.policy == LampsMatchScreen {
		s.repushLocked()
	}
}

// lightMasks reports the mirror, the app's intent, rather than the device.
func (s *shadowHardware) lightMasks() [2]hardware.LightMask {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out [2]hardware.LightMask
	for p := range s.levels {
		out[p] = s.levels[p].Mask()
	}
	return out
}

// lampLevels is lightMasks with the brightness kept.
func (s *shadowHardware) lampLevels() [2]hardware.LampLevels {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.levels
}

// holdLights is idempotent. Engine goroutine only.
func (s *shadowHardware) holdLights() {
	s.mu.Lock()
	s.lightsHeld = true
	s.mu.Unlock()
}

// releaseLights replays the mirror; a no-op without a hold.
func (s *shadowHardware) releaseLights() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.lightsHeld {
		return
	}
	s.lightsHeld = false
	s.pushLocked(-1)
}

// overrideLights writes past the mirror and the hold but not the policy: an
// operator's lamps-off outranks a light show.
func (s *shadowHardware) overrideLights(masks [2]hardware.LightMask) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for p, mask := range masks {
		s.pushSeatLocked(p, hardware.LampLevelsOf(mask))
	}
}

// overrideLightLevels is overrideLights for a fading step: per-lamp
// brightness, scaled by the policy like every other write.
func (s *shadowHardware) overrideLightLevels(levels [2]hardware.LampLevels) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for p, lv := range levels {
		s.pushSeatLocked(p, lv)
	}
}

func (s *shadowHardware) StatusLines() []string { return s.inner.StatusLines() }
func (s *shadowHardware) Close() error          { return s.inner.Close() }

func (s *shadowHardware) SetFanConfig(thresholdF float64, mode hardware.FanMode) {
	s.inner.SetFanConfig(thresholdF, mode)
}

func (s *shadowHardware) MotionActive() bool {
	if m, ok := s.inner.(hardware.MotionSensor); ok {
		return m.MotionActive()
	}
	return false
}

func (s *shadowHardware) SetBacklight(on bool) { s.SetBacklightLevel(onLevel(on)) }

// SetBacklightLevel sets the mirrored backlight and writes it through,
// cancelling any fade.
func (s *shadowHardware) SetBacklightLevel(level uint8) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backlightTouched = true
	s.backlight = min(level, hardware.LampLevelMax)
	s.backlightFade = lampFade{}
	s.pushBacklightLocked()
}

func (s *shadowHardware) CPUTempF() (degF float64, ok bool) {
	if t, ok := s.inner.(hardware.ThermalSensor); ok {
		return t.CPUTempF()
	}
	return 0, false
}

func (s *shadowHardware) FanOn() bool {
	if f, ok := s.inner.(hardware.CoolingFan); ok {
		return f.FanOn()
	}
	return false
}

func (s *shadowHardware) FanMode() hardware.FanMode {
	if f, ok := s.inner.(hardware.CoolingFan); ok {
		return f.FanMode()
	}
	return hardware.FanAuto
}

func (s *shadowHardware) LastMotion() (time.Time, bool) {
	if m, ok := s.inner.(hardware.MotionHistory); ok {
		return m.LastMotion()
	}
	return time.Time{}, false
}

// SetPreviewGlyphs forwards a CGRAM overlay to a hardware.GlyphPager. Glyphs
// are not mirrored: each overlay's installer owns it start to finish.
func (s *shadowHardware) SetPreviewGlyphs(glyphs map[rune]hardware.GlyphRows) {
	if g, ok := s.inner.(hardware.GlyphPager); ok {
		g.SetPreviewGlyphs(glyphs)
	}
}

func (s *shadowHardware) SetCartGlyphs(glyphs map[rune]hardware.GlyphRows) {
	if g, ok := s.inner.(hardware.GlyphPager); ok {
		g.SetCartGlyphs(glyphs)
	}
}

func (s *shadowHardware) MicInfo() hardware.MicInfo {
	if m, ok := s.inner.(hardware.MicSource); ok {
		return m.MicInfo()
	}
	return hardware.MicInfo{}
}

func (s *shadowHardware) MicLevel() (hardware.MicLevel, bool) {
	if m, ok := s.inner.(hardware.MicSource); ok {
		return m.MicLevel()
	}
	return hardware.MicLevel{}, false
}

func (s *shadowHardware) MicBands() ([hardware.MicBandCount]float64, bool) {
	if m, ok := s.inner.(hardware.MicBander); ok {
		return m.MicBands()
	}
	return [hardware.MicBandCount]float64{}, false
}

func (s *shadowHardware) InputSignal() {
	if i, ok := s.inner.(hardware.InputSpy); ok {
		i.InputSignal()
	}
}

var (
	_ hardware.Suite           = (*shadowHardware)(nil)
	_ hardware.BacklightDimmer = (*shadowHardware)(nil)
	_ hardware.GlyphPager      = (*shadowHardware)(nil)
	_ hardware.MotionSensor    = (*shadowHardware)(nil)
	_ hardware.MotionHistory   = (*shadowHardware)(nil)
	_ hardware.InputSpy        = (*shadowHardware)(nil)
	_ hardware.ThermalSensor   = (*shadowHardware)(nil)
	_ hardware.CoolingFan      = (*shadowHardware)(nil)
	_ hardware.MicSource       = (*shadowHardware)(nil)
	_ hardware.MicBander       = (*shadowHardware)(nil)
)
