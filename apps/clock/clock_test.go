// SPDX-License-Identifier: GPL-2.0-or-later

package clock_test

import (
	"testing"
	"time"

	_ "github.com/stealthmonkey99/arkade/apps/clock"
	_ "github.com/stealthmonkey99/arkade/apps/shell"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// newClockRig boots the clock and moves the manual clock from
// 2024-01-01T00:00:00Z to 10:08:42 so the three hands sit at distinct angles.
func newClockRig(t *testing.T) *testkit.Rig {
	t.Helper()
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/clock"})
	rig.Clock.Advance(10*time.Hour + 8*time.Minute + 42*time.Second)
	rig.Tick(2) // fade completes instantly after the jump
	return rig
}

func tickPlays(rig *testkit.Rig) int {
	n := 0
	for _, ev := range rig.Audio.Events() {
		if ev.Action == "play" && ev.File == "./assets/tick.wav" {
			n++
		}
	}
	return n
}

func TestClockGoldenFaces(t *testing.T) {
	rig := newClockRig(t)

	rig.Golden("analog-face")

	rig.PressKeyboard(input.KeyRight, false)
	rig.TickFor("600ms")
	rig.Golden("digital-face")

	rig.PressKeyboard(input.KeyRight, false)
	rig.TickFor("600ms")
	rig.Golden("calendar-face")
}

func TestClockFaceSwitchingWraps(t *testing.T) {
	rig := newClockRig(t)

	// Jan 1 2024 is a Monday, so cell 1 holds day 1; (13,18) is inside it.
	rig.PressKeyboard(input.KeyRight, false) // → digital
	rig.TickFor("600ms")
	rig.PressKeyboard(input.KeyRight, false) // → calendar
	rig.TickFor("600ms")
	if got := rig.PixelAt(13, 18); got != display.Focus {
		t.Errorf("calendar should focus today's cell, got %+v", got)
	}
	// When today is the 1st the leading blank cell is focused too.
	if got := rig.PixelAt(4, 18); got != display.Focus {
		t.Errorf("calendar quirk: leading blank cell should be focused, got %+v", got)
	}

	// December 2023 has no current day.
	rig.PressKeyboard(input.KeyUp, false)
	rig.Tick(1)
	if got := rig.PixelAt(13, 18); got != display.White {
		t.Errorf("previous month should clear the focus fill, got %+v", got)
	}

	rig.PressKeyboard(input.KeyRight, false)
	rig.TickFor("600ms")
	if got := rig.PixelAt(13, 18); got != display.Black {
		t.Errorf("next from calendar should wrap to analog, got %+v", got)
	}

	rig.PressKeyboard(input.KeyLeft, false)
	rig.TickFor("600ms")
	if got := rig.PixelAt(13, 18); got != display.White {
		t.Errorf("prev from analog should wrap to calendar, got %+v", got)
	}
}

func TestClockFacesLauncher(t *testing.T) {
	// The clock's Init turns toward the seat of the last controller press.
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})
	rig.PressAs(input.KeyUp, 1)
	rig.Tick(1)
	rig.RT.RequestLaunch("apps/clock")
	rig.TickFor("600ms")
	if got := rig.RT.CurrentSlug(); got != "apps/clock" {
		t.Fatalf("expected clock to be running, got %q", got)
	}
	if got := rig.RT.Renderer.VirtualOrientation(); got != display.Clockwise {
		t.Errorf("clock should face the launching player 2 (clockwise), got %v", got)
	}
}

func TestClockTurnsTowardActivePlayer(t *testing.T) {
	rig := newClockRig(t)

	if got := rig.RT.Renderer.VirtualOrientation(); got != display.Upright {
		t.Fatalf("clock should start upright without a launch press, got %v", got)
	}

	rig.PressAs(input.KeyEnter, 1)
	rig.Tick(1)
	if got := rig.RT.Renderer.VirtualOrientation(); got != display.Clockwise {
		t.Errorf("player-2 press should turn the clock clockwise, got %v", got)
	}

	rig.PressAs(input.KeyEnter, 0)
	rig.Tick(1)
	if got := rig.RT.Renderer.VirtualOrientation(); got != display.CounterClockwise {
		t.Errorf("player-1 press should turn the clock counter-clockwise, got %v", got)
	}

	rig.PressKeyboard(input.KeyRight, false)
	rig.TickFor("600ms")
	if got := rig.RT.Renderer.VirtualOrientation(); got != display.CounterClockwise {
		t.Errorf("keyboard press should not re-turn the clock, got %v", got)
	}
}

func TestClockTickSoundTogglesWithEnter(t *testing.T) {
	rig := newClockRig(t)

	rig.TickFor("1s")
	if tickPlays(rig) == 0 {
		t.Fatal("analog face should play ./assets/tick.wav on second change")
	}

	rig.Press(input.KeyEnter)
	rig.Tick(1)
	before := tickPlays(rig)
	rig.TickFor("2s")
	if got := tickPlays(rig); got != before {
		t.Errorf("tick sound should be muted after enter, got %d extra plays", got-before)
	}

	rig.Press(input.KeyEnter)
	rig.Tick(1)
	before = tickPlays(rig)
	rig.TickFor("2s")
	if got := tickPlays(rig); got <= before {
		t.Error("tick sound should resume after second enter press")
	}
}
