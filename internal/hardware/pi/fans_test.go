// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"math"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

func testFan(t *testing.T) (*fan, *fakeBus) {
	t.Helper()
	w, bus := testLampWriter(t)
	return newFan(&expanderFan{w: w, pins: []int{fanPin}}, nil), bus
}

func TestFanStepThreshold(t *testing.T) {
	cases := []struct {
		on          bool
		temp        float64
		wantOn      bool
		wantChanged bool
	}{
		{false, math.Inf(-1), false, false}, // no sample yet
		{false, 119.9, false, false},
		{false, 120, true, true}, // threshold is inclusive
		{true, 130, true, false}, // already running, no rewrite
		{true, 120, true, false},
		{true, 119.9, false, true},
		{false, 80, false, false},
	}
	for i, c := range cases {
		on, changed := fanStep(c.on, c.temp, tempThreshold, hardware.FanAuto)
		if on != c.wantOn || changed != c.wantChanged {
			t.Errorf("case %d (%v, %v°F): got on=%v changed=%v, want on=%v changed=%v",
				i, c.on, c.temp, on, changed, c.wantOn, c.wantChanged)
		}
	}
}

func TestFanStepCustomThreshold(t *testing.T) {
	cases := []struct {
		on          bool
		temp        float64
		threshold   float64
		wantOn      bool
		wantChanged bool
	}{
		{false, 149, 150, false, false},
		{false, 150, 150, true, true},
		{true, 150, 150, true, false},
		{true, 149.9, 150, false, true},
		{false, 79, 80, false, false},
		{false, 80, 80, true, true},
	}
	for i, c := range cases {
		on, changed := fanStep(c.on, c.temp, c.threshold, hardware.FanAuto)
		if on != c.wantOn || changed != c.wantChanged {
			t.Errorf("case %d (%v, %v°F @ %v°F): got on=%v changed=%v, want on=%v changed=%v",
				i, c.on, c.temp, c.threshold, on, changed, c.wantOn, c.wantChanged)
		}
	}
}

func TestFanStepModes(t *testing.T) {
	cases := []struct {
		mode        hardware.FanMode
		on          bool
		temp        float64
		wantOn      bool
		wantChanged bool
	}{
		{hardware.FanOff, true, 200, false, true},
		{hardware.FanOff, false, 200, false, false},
		{hardware.FanOn, false, 60, true, true},
		{hardware.FanOn, true, 60, true, false},
		{hardware.FanAuto, false, 60, false, false},
		{hardware.FanAuto, false, 200, true, true},
	}
	for i, c := range cases {
		on, changed := fanStep(c.on, c.temp, tempThreshold, c.mode)
		if on != c.wantOn || changed != c.wantChanged {
			t.Errorf("case %d (%s, on=%v, %v°F): got on=%v changed=%v, want on=%v changed=%v",
				i, c.mode, c.on, c.temp, on, changed, c.wantOn, c.wantChanged)
		}
	}
}

func TestParseFanMode(t *testing.T) {
	for _, s := range []string{"auto", "on", "off"} {
		if m, ok := hardware.ParseFanMode(s); !ok || string(m) != s {
			t.Errorf("ParseFanMode(%q) = %q, %v", s, m, ok)
		}
	}
	for _, s := range []string{"", "ON", "disabled", "0"} {
		if m, ok := hardware.ParseFanMode(s); ok || m != hardware.FanAuto {
			t.Errorf("ParseFanMode(%q) = %q, %v; want auto, false", s, m, ok)
		}
	}
}

func TestFanCheckDrivesPinSeven(t *testing.T) {
	f, bus := testFan(t)

	f.check(125)
	if len(bus.recs) != 1 || bus.recs[0].w[0] != regOLATA || bus.recs[0].w[1] != 0x80 {
		t.Errorf("hot sample should latch pin 7 high: %v", bus.recs)
	}
	bus.reset()
	f.check(126) // still hot: no rewrite
	if len(bus.recs) != 0 {
		t.Errorf("steady state should not rewrite the latch: %v", bus.recs)
	}
	bus.reset()
	f.check(100)
	if len(bus.recs) != 1 || bus.recs[0].w[1] != 0x00 {
		t.Errorf("cool sample should drop pin 7: %v", bus.recs)
	}
}

func TestFanSetConfigOffForcesOffImmediately(t *testing.T) {
	f, bus := testFan(t)

	f.check(130) // spins the relay up
	if !f.isOn() {
		t.Fatal("relay should be on after a hot sample")
	}
	bus.reset()

	f.setConfig(tempThreshold, hardware.FanOff)
	if f.isOn() {
		t.Error("FanOff should force the relay off immediately")
	}
	if len(bus.recs) != 1 || bus.recs[0].w[1] != 0x00 {
		t.Errorf("FanOff should write the relay low right away: %v", bus.recs)
	}

	bus.reset()
	f.check(200) // still off: a hot sample must not turn it back on
	if f.isOn() || len(bus.recs) != 0 {
		t.Errorf("an Off fan should ignore hot samples: on=%v recs=%v", f.isOn(), bus.recs)
	}

	bus.reset()
	f.setConfig(tempThreshold, hardware.FanAuto)
	f.check(200)
	if !f.isOn() {
		t.Error("back on Auto, a hot sample should turn the relay on again")
	}
}

func TestFanSetConfigOnHoldsThroughCoolSamples(t *testing.T) {
	f, bus := testFan(t)
	f.check(70) // well below the threshold
	bus.reset()

	f.setConfig(tempThreshold, hardware.FanOn)
	if !f.isOn() {
		t.Fatal("FanOn should engage the relay regardless of temperature")
	}
	if len(bus.recs) != 1 || bus.recs[0].w[1] != 1<<fanPin {
		t.Errorf("FanOn should latch the relay high right away: %v", bus.recs)
	}
	if f.mode() != hardware.FanOn {
		t.Errorf("mode() = %q, want on", f.mode())
	}

	bus.reset()
	f.check(60)
	if !f.isOn() || len(bus.recs) != 0 {
		t.Errorf("an On fan should ignore cool samples: on=%v recs=%v", f.isOn(), bus.recs)
	}

	f.setConfig(tempThreshold, hardware.FanAuto)
	if f.isOn() {
		t.Error("back on Auto at 60°F, the relay should drop")
	}
}

func TestFanSetConfigLowersThresholdImmediately(t *testing.T) {
	f, bus := testFan(t)
	f.check(100) // below the 120°F default: stays off
	if f.isOn() {
		t.Fatal("100°F should not trip the default threshold")
	}
	bus.reset()

	f.setConfig(90, hardware.FanAuto) // lower the threshold below the last sample
	if !f.isOn() {
		t.Error("lowering the threshold below the last sample should turn the relay on immediately")
	}
	if len(bus.recs) != 1 || bus.recs[0].w[1] != 0x80 {
		t.Errorf("threshold change should write the relay high right away: %v", bus.recs)
	}
}

func TestFanWithoutSink(t *testing.T) {
	f := newFan(nil, nil)
	f.check(200)
	if !f.isOn() {
		t.Error("a sinkless fan should still track the relay state it wanted")
	}
	f.setConfig(tempThreshold, hardware.FanOff)
	if f.isOn() {
		t.Error("FanOff should still register with no sink")
	}
}

func TestFanStatusGlyphs(t *testing.T) {
	f := newFan(nil, nil)
	if glyph, temp := f.status(); glyph != "✻" || temp != "~°F" {
		t.Errorf("idle fan before first sample: %q %q", glyph, temp)
	}
	f.check(130)
	glyph, temp := f.status()
	if !strings.Contains(strings.Join(fanAnimChars, ""), glyph) {
		t.Errorf("running fan should show a spinner frame, got %q", glyph)
	}
	if temp != "130°F" {
		t.Errorf("temp = %q, want 130°F", temp)
	}
	if next, _ := f.status(); next == glyph {
		t.Error("spinner should advance between reads")
	}
}
