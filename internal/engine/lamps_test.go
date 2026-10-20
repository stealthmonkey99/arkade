// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// lampRecorder keeps every lamp write so tests can assert how many times a level was written, not only the final level.
type lampRecorder struct {
	levels [2]hardware.LampLevels
	writes []lampWrite
	legacy int // SetLights/SetButtonLights calls, which the shadow must not make
}

type lampWrite struct {
	seat   int
	levels hardware.LampLevels
}

func (r *lampRecorder) SetText(int, string, string)                      {}
func (r *lampRecorder) SetLights(int, bool)                              { r.legacy++ }
func (r *lampRecorder) SetButtonLights(int, hardware.ButtonLights, bool) { r.legacy++ }
func (r *lampRecorder) StatusLines() []string                            { return nil }
func (r *lampRecorder) SetFanConfig(float64, hardware.FanMode)           {}
func (r *lampRecorder) Close() error                                     { return nil }

func (r *lampRecorder) SetLampLevels(index int, lv hardware.LampLevels) {
	for p := range r.levels {
		if index >= 0 && index != p {
			continue
		}
		r.levels[p] = lv
		r.writes = append(r.writes, lampWrite{seat: p, levels: lv})
	}
}

// seatWrites is the recorded writes for one seat, oldest first.
func (r *lampRecorder) seatWrites(seat int) []hardware.LampLevels {
	var out []hardware.LampLevels
	for _, w := range r.writes {
		if w.seat == seat {
			out = append(out, w.levels)
		}
	}
	return out
}

func newLampRig() (*shadowHardware, *lampRecorder) {
	rec := &lampRecorder{}
	return newShadowHardware(rec), rec
}

func lit(levels ...uint8) hardware.LampLevels {
	var lv hardware.LampLevels
	copy(lv[:], levels)
	return lv
}

func TestScaleLampsFloorsALitLamp(t *testing.T) {
	cases := []struct {
		name string
		in   hardware.LampLevels
		pct  int
		want hardware.LampLevels
	}{
		{"full at full", lit(hardware.LampLevelMax, 128, 0), 100, lit(hardware.LampLevelMax, 128, 0)},
		{"half", lit(hardware.LampLevelMax, 128, 0), 50, lit(128, 64, 0)},
		{"a dim panel keeps every lit lamp visible", lit(hardware.LampLevelMax, 1, 0), 10, lit(26, 1, 0)},
		{"off darkens everything", lit(hardware.LampLevelMax, 1, 0), 0, lit(0, 0, 0)},
		{"dark stays dark", lit(0, 0, 0), 100, lit(0, 0, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := scaleLamps(c.in, c.pct); got != c.want {
				t.Errorf("scaleLamps(%v, %d) = %v, want %v", c.in, c.pct, got, c.want)
			}
		})
	}
}

func TestLampPolicyRescalesTheMirror(t *testing.T) {
	s, rec := newLampRig()
	s.setPanelBrightness(40)
	s.SetLampLevels(0, lit(hardware.LampLevelMax, 170, 1))

	if got := rec.levels[0]; got != lit(102, 68, 1) {
		t.Errorf("match: device = %v, want the 40%% scaling %v", got, lit(102, 68, 1))
	}
	s.setLampPolicy(LampsOn)
	if got := rec.levels[0]; got != lit(hardware.LampLevelMax, 170, 1) {
		t.Errorf("on: device = %v, want the app's own levels", got)
	}
	s.setLampPolicy(LampsOff)
	if got := rec.levels[0]; got != (hardware.LampLevels{}) {
		t.Errorf("off: device = %v, want everything dark", got)
	}
	s.setLampPolicy(LampsMatchScreen)
	if got := rec.levels[0]; got != lit(102, 68, 1) {
		t.Errorf("back to match: device = %v, want %v", got, lit(102, 68, 1))
	}
	if got := s.lampLevels()[0]; got != lit(hardware.LampLevelMax, 170, 1) {
		t.Errorf("the mirror = %v, want the app's intent untouched by policy", got)
	}
	if rec.legacy != 0 {
		t.Errorf("the shadow made %d on/off lamp calls, want every write to be a level write", rec.legacy)
	}
}

func TestPanelBrightnessRepushesOnlyUnderMatch(t *testing.T) {
	s, rec := newLampRig()
	s.setLampPolicy(LampsOn)
	s.SetLampLevels(0, lit(hardware.LampLevelMax))

	before := len(rec.writes)
	s.setPanelBrightness(20)
	if len(rec.writes) != before {
		t.Errorf("brightness wrote the lamps under policy on: %v", rec.writes[before:])
	}
	s.setLampPolicy(LampsMatchScreen)
	if got := rec.levels[0]; got != lit(51) {
		t.Errorf("device = %v, want the 20%% scaling %v", got, lit(51))
	}
	s.setPanelBrightness(100)
	if got := rec.levels[0]; got != lit(hardware.LampLevelMax) {
		t.Errorf("device = %v after brightening, want %v", got, lit(hardware.LampLevelMax))
	}
}

func TestLampFadeRampsDownAndWritesOnlyOnChange(t *testing.T) {
	s, rec := newLampRig()
	s.setLampPolicy(LampsOn)
	s.SetLampLevels(0, lit(hardware.LampLevelMax))
	start := len(rec.seatWrites(0))

	s.fadeLamps(0, hardware.LampLevels{}, hardware.LightMask(1), 600, Linear)
	if got := s.lampLevels()[0]; got != (hardware.LampLevels{}) {
		t.Errorf("mirror = %v, want the fade's target immediately", got)
	}
	if got := rec.levels[0]; got != lit(hardware.LampLevelMax) {
		t.Errorf("device = %v, want the lamp still lit as the fade begins", got)
	}

	const tickMicros = 33333
	zeroAt, prev := 0, uint8(hardware.LampLevelMax)
	for i := 1; i <= 20; i++ {
		s.stepFades(tickMicros)
		got := rec.levels[0][0]
		if got > prev {
			t.Fatalf("tick %d: level rose from %d to %d", i, prev, got)
		}
		prev = got
		if got == 0 && zeroAt == 0 {
			zeroAt = i
		}
	}
	// 18 ticks is 599.994ms of a 600ms fade: the last quantized step lands
	// there, not a tick later.
	if zeroAt != 18 {
		t.Errorf("the lamp reached 0 on tick %d, want 18", zeroAt)
	}

	// One write per tick of the ramp and not one after it: at 8 bits every
	// tick moves the level, so nothing is written twice and the two idle
	// ticks at the end write nothing at all.
	var got []uint8
	for _, w := range rec.seatWrites(0)[start:] {
		got = append(got, w[0])
	}
	if len(got) != 18 {
		t.Errorf("the fade wrote %d levels %v, want one per tick of the ramp", len(got), got)
	}
	for i, v := range got {
		if i > 0 && v >= got[i-1] {
			t.Errorf("write %d = %d, want it strictly below its predecessor %d", i, v, got[i-1])
		}
	}
	if n := len(got); n == 0 || got[n-1] != 0 {
		t.Errorf("the fade ended at %v, want it to land on 0", got)
	}
}

func TestExplicitWriteCancelsOnlyItsOwnFade(t *testing.T) {
	s, rec := newLampRig()
	s.setLampPolicy(LampsOn)
	s.SetLampLevels(0, lit(hardware.LampLevelMax, hardware.LampLevelMax))
	s.fadeLamps(0, hardware.LampLevels{}, hardware.LightMask(0b11), 600, Linear)

	s.stepFades(300000)
	s.SetButtonLights(0, hardware.ButtonLights{"joya": true}, false)
	if got := rec.levels[0][0]; got != hardware.LampLevelMax {
		t.Errorf("joya = %d after an explicit write, want it pinned at full", got)
	}
	mid := rec.levels[0][1]
	if mid == 0 || mid == hardware.LampLevelMax {
		t.Fatalf("joyb = %d, want it still part-way through its fade", mid)
	}

	s.stepFades(300000)
	if got := rec.levels[0][0]; got != hardware.LampLevelMax {
		t.Errorf("joya = %d, want its fade cancelled for good", got)
	}
	if got := rec.levels[0][1]; got != 0 {
		t.Errorf("joyb = %d, want its own fade to have finished", got)
	}
}

func TestFadeLampsWithoutDurationIsInstant(t *testing.T) {
	s, rec := newLampRig()
	s.setLampPolicy(LampsOn)
	s.SetLampLevels(-1, lit(hardware.LampLevelMax, hardware.LampLevelMax))

	s.fadeLamps(-1, lit(0, 64), hardware.LightMask(0b11), 0, Linear)
	for p := range rec.levels {
		if got := rec.levels[p]; got != lit(0, 64) {
			t.Errorf("seat %d device = %v, want the target straight away", p, got)
		}
	}
	before := len(rec.writes)
	s.stepFades(33333)
	if len(rec.writes) != before {
		t.Errorf("an instant fade left something to step: %v", rec.writes[before:])
	}
}

func TestRestoreReplaysLevelsAndDropsFades(t *testing.T) {
	s, rec := newLampRig()
	s.setLampPolicy(LampsOn)
	s.SetLampLevels(0, lit(200, 50))
	parked := s.snapshot()

	s.SetLights(-1, false)
	s.fadeLamps(0, lit(hardware.LampLevelMax, hardware.LampLevelMax), hardware.LightMask(0b11), 600, Linear)
	s.stepFades(100000)

	s.restore(parked)
	if got := rec.levels[0]; got != lit(200, 50) {
		t.Errorf("device = %v after a resume, want the parked levels %v", got, lit(200, 50))
	}
	before := len(rec.writes)
	s.stepFades(500000)
	if len(rec.writes) != before {
		t.Errorf("a fade survived the resume: %v", rec.writes[before:])
	}
}

// The character LCD backlight is scaled by the lamp policy like the buttons; one dim panel beside two full backlights reads as a fault.
func TestBacklightFollowsTheLampPolicy(t *testing.T) {
	hw := hardware.NewVirtual()
	s := newShadowHardware(hw)
	s.SetBacklightLevel(hardware.LampLevelMax) // as the boot screen does; untouched, it is left alone

	s.setPanelBrightness(50)
	if got, want := hw.BacklightLevel(), uint8(128); got != want {
		t.Errorf("backlight at 50%% = %d, want %d", got, want)
	}
	s.setPanelBrightness(20)
	if got, want := hw.BacklightLevel(), uint8(51); got != want {
		t.Errorf("backlight after dimming the panel to 20%% = %d, want %d", got, want)
	}
	s.setLampPolicy(LampsOn)
	if got := hw.BacklightLevel(); got != hardware.LampLevelMax {
		t.Errorf("backlight under lamps-on = %d, want full (%d)", got, hardware.LampLevelMax)
	}
	s.setLampPolicy(LampsOff)
	if got := hw.BacklightLevel(); got != 0 {
		t.Errorf("backlight under lamps-off = %d, want dark", got)
	}
	s.setLampPolicy(LampsMatchScreen)
	if got, want := hw.BacklightLevel(), uint8(51); got != want {
		t.Errorf("backlight back under match = %d, want the panel's %d", got, want)
	}
}

func TestBacklightFadeScalesEveryStepUnderMatch(t *testing.T) {
	hw := hardware.NewVirtual()
	s := newShadowHardware(hw)
	s.setPanelBrightness(50)

	s.fadeBacklight(0, 600, Linear)
	const tickMicros = 33333
	prev, steps := hw.BacklightLevel(), 0
	for i := 1; i <= 18; i++ {
		s.stepFades(tickMicros)
		got := hw.BacklightLevel()
		if got > prev {
			t.Fatalf("tick %d: backlight rose from %d to %d", i, prev, got)
		}
		if got > 128 {
			t.Fatalf("tick %d: backlight = %d, want every step of the ramp scaled to the panel's 50%%", i, got)
		}
		if got > 0 && got < 128 {
			steps++
		}
		prev = got
	}
	if steps < 10 {
		t.Errorf("the ramp showed %d scaled intermediate levels, want it stepping smoothly", steps)
	}
	if prev != 0 {
		t.Errorf("backlight after the full 600ms = %d, want dark", prev)
	}
}
