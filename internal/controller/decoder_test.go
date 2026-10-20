// SPDX-License-Identifier: GPL-2.0-or-later

package controller

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/input"
)

func summarize(events []input.Event) []string {
	var out []string
	for _, ev := range events {
		switch ev.Kind {
		case input.KindJoystickChange:
			out = append(out, "joy:"+string(ev.Position))
		default:
			out = append(out, string(ev.Kind)+":"+string(ev.Name))
		}
	}
	return out
}

func assertEvents(t *testing.T, got []input.Event, want ...string) {
	t.Helper()
	gs := summarize(got)
	if len(gs) != len(want) {
		t.Fatalf("event mismatch:\n got  %v\n want %v", gs, want)
	}
	for i := range want {
		if gs[i] != want[i] {
			t.Fatalf("event %d: got %q want %q (full: %v)", i, gs[i], want[i], gs)
		}
	}
}

func speedlinkReport(buttons, x, y byte) []byte { return []byte{buttons, x, y} }

func TestSpeedlinkFirstReportPrimesWithoutEvents(t *testing.T) {
	d := NewDecoder(Speedlink.Spec)
	assertEvents(t, d.Feed(speedlinkReport(0b0001, 128, 128)))
}

func TestSpeedlinkButtonPressAndRelease(t *testing.T) {
	d := NewDecoder(Speedlink.Spec)
	d.Feed(speedlinkReport(0, 128, 128))
	assertEvents(t, d.Feed(speedlinkReport(0b0001, 128, 128)),
		"keydown:joya", "keypress:joya")
	assertEvents(t, d.Feed(speedlinkReport(0, 128, 128)),
		"keyup:joya")
}

func TestSpeedlinkMultipleSimultaneousButtons(t *testing.T) {
	d := NewDecoder(Speedlink.Spec)
	d.Feed(speedlinkReport(0, 128, 128))
	assertEvents(t, d.Feed(speedlinkReport(0b1010, 128, 128)),
		"keydown:joyb", "keypress:joyb", "keydown:r1", "keypress:r1")
}

func TestSpeedlinkStickEmitsJoystickAndArrowKeys(t *testing.T) {
	d := NewDecoder(Speedlink.Spec)
	d.Feed(speedlinkReport(0, 128, 128))
	assertEvents(t, d.Feed(speedlinkReport(0, 0, 128)),
		"joy:left", "keydown:left", "keypress:left")
	assertEvents(t, d.Feed(speedlinkReport(0, 0, 0)),
		"joy:up-left", "keydown:up", "keypress:up")
	assertEvents(t, d.Feed(speedlinkReport(0, 128, 128)),
		"joy:center", "keyup:left", "keyup:up")
}

func TestDragonRiseButtonExtraction(t *testing.T) {
	d := NewDecoder(DragonRise.Spec)
	center := []byte{127, 127, 0, 0, 0, 0x0f, 0}
	d.Feed(center)
	// Byte 5 high nibble carries buttons 0-3 over a 0x0f base; JOY_A is bit 4.
	pressed := []byte{127, 127, 0, 0, 0, 0x1f, 0}
	assertEvents(t, d.Feed(pressed), "keydown:joya", "keypress:joya")
	// Byte 6 carries buttons 4+; bit 0 is L1.
	pressed2 := []byte{127, 127, 0, 0, 0, 0x0f, 0b1}
	assertEvents(t, d.Feed(pressed2),
		"keyup:joya", "keydown:l1", "keypress:l1")
}

func TestBuffaloCenterNoiseFiltered(t *testing.T) {
	d := NewDecoder(Buffalo.Spec)
	d.Feed([]byte{128, 128, 0, 0})
	assertEvents(t, d.Feed([]byte{132, 124, 0, 0}))
	assertEvents(t, d.Feed([]byte{0, 128, 0, 0}),
		"joy:left", "keydown:left", "keypress:left")
}

func TestBuffaloButtons(t *testing.T) {
	d := NewDecoder(Buffalo.Spec)
	d.Feed([]byte{128, 128, 0, 0})
	// buttons = data[3]<<4 | data[2]; SELECT is data[3] bit 2.
	assertEvents(t, d.Feed([]byte{128, 128, 0, 0b100}),
		"keydown:select", "keypress:select")
}

func TestBuffaloFaceButtonColors(t *testing.T) {
	d := NewDecoder(Buffalo.Spec)
	d.Feed([]byte{128, 128, 0, 0})
	assertEvents(t, d.Feed([]byte{128, 128, 0b0001, 0}), // physical A
		"keydown:joyd", "keypress:joyd")
	assertEvents(t, d.Feed([]byte{128, 128, 0b0011, 0}), // + physical B
		"keydown:joyc", "keypress:joyc")
	assertEvents(t, d.Feed([]byte{128, 128, 0b0111, 0}), // + physical X
		"keydown:joyb", "keypress:joyb")
	assertEvents(t, d.Feed([]byte{128, 128, 0b1111, 0}), // + physical Y
		"keydown:joya", "keypress:joya")
}

func TestSoniconHatDecoding(t *testing.T) {
	d := NewDecoder(Sonicon.Spec)
	// Hat 8 = center (x: 8%4==0 → 0; y: 8 → 0).
	d.Feed([]byte{0, 0, 8})
	assertEvents(t, d.Feed([]byte{0, 0, 0}),
		"joy:up", "keydown:up", "keypress:up")
	// Hat 2 = right: x = -(2-4) = 2, y = 0. X edges emit before Y edges.
	assertEvents(t, d.Feed([]byte{0, 0, 2}),
		"joy:right", "keydown:right", "keypress:right", "keyup:up")
}

func TestSoniconSixteenBitButtons(t *testing.T) {
	d := NewDecoder(Sonicon.Spec)
	d.Feed([]byte{0, 0, 8})
	// HOME is data[1] bit 4.
	assertEvents(t, d.Feed([]byte{0, 0b10000, 8}),
		"keydown:home", "keypress:home")
}

func TestDualShockDpad(t *testing.T) {
	d := NewDecoder(DualShock.Spec)
	// data[5] low nibble = hat: 8 = center; axes centered at 128.
	center := []byte{0, 128, 128, 0, 0, 0x08, 0, 0}
	d.Feed(center)
	up := []byte{0, 128, 128, 0, 0, 0x00, 0, 0}
	assertEvents(t, d.Feed(up),
		"joy:up", "keydown:up", "keypress:up")
	down := []byte{0, 128, 128, 0, 0, 0x04, 0, 0}
	assertEvents(t, d.Feed(down),
		"joy:down", "keyup:up", "keydown:down", "keypress:down")
}

func TestDualShockButtonsAndStickTogether(t *testing.T) {
	d := NewDecoder(DualShock.Spec)
	center := []byte{0, 128, 128, 0, 0, 0x08, 0, 0}
	d.Feed(center)
	// JOY_A is data[6] bit 7.
	joyA := []byte{0, 128, 128, 0, 0, 0x08, 0b10000000, 0}
	assertEvents(t, d.Feed(joyA), "keydown:joya", "keypress:joya")
	stick := []byte{0, 255, 128, 0, 0, 0x08, 0b10000000, 0}
	assertEvents(t, d.Feed(stick),
		"joy:right", "keydown:right", "keypress:right")
}

func TestJSRoundMatchesJavaScript(t *testing.T) {
	// Math.round(-0.5) is 0 in JS; the Buffalo/DualShock centre filter relies on it.
	if jsRound(-0.5) != 0 {
		t.Errorf("jsRound(-0.5) = %d, want 0", jsRound(-0.5))
	}
	if jsRound(0.5) != 1 {
		t.Errorf("jsRound(0.5) = %d, want 1", jsRound(0.5))
	}
	if jsRound(-0.6) != -1 {
		t.Errorf("jsRound(-0.6) = %d, want -1", jsRound(-0.6))
	}
}

func TestShortReportIsSafe(t *testing.T) {
	for _, p := range Profiles {
		d := NewDecoder(p.Spec)
		d.Feed([]byte{}) // must not panic
		d.Feed([]byte{0})
	}
}
