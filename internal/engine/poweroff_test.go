// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

import (
	"math"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// powerOffHeld is the backlight level the sequence settles on and the
// PCA9685 goes on holding once the process is gone.
const powerOffHeld = hardware.LampLevelMax * 30 / 100

// powerOffRun drives a rig by the sequence's own clock: at(ms) advances to
// the first frame at or past ms since the sequence armed.
type powerOffRun struct {
	rig    *testkit.Rig
	hw     *hardware.Virtual
	frames int
}

func (r *powerOffRun) at(ms float64) {
	r.rig.T.Helper()
	want := int(math.Ceil(ms / (1000.0 / 30)))
	if want > r.frames {
		r.rig.Tick(want - r.frames)
		r.frames = want
	}
}

// startPowerOff runs the lamp app up, pins the policy so the assertions read
// raw hardware levels, and arms the sequence.
func startPowerOff(t *testing.T) *powerOffRun {
	t.Helper()
	rig := newPeriphRig(t)
	rig.Tick(2)
	rig.RT.SetLampPolicy(engine.LampsOn)
	rig.RT.SetBacklightLevel(hardware.LampLevelMax)
	hw := virtualHW(t, rig)
	rig.RT.PowerOff()
	rig.Tick(1) // the sequence arms at the top of the next tick
	return &powerOffRun{rig: rig, hw: hw}
}

// TestPowerOffSchedule pins the shutdown choreography: the LCDs say goodbye
// at once, every lamp winks to full, the colour groups then leave in the
// reverse of the boot bloom, the backlight settles on the level the PCA9685
// will go on holding, and only after all of it is a quit requested.
func TestPowerOffSchedule(t *testing.T) {
	r := startPowerOff(t)

	if got := r.rig.RT.CurrentSlug(); got != "" {
		t.Errorf("app %q still loaded, want it unloaded before the sequence paints", got)
	}
	for seat := 0; seat < 2; seat++ {
		if line1, line2 := r.hw.Lines(seat); line1 != hardware.PowerOffLine1 || line2 != hardware.PowerOffLine2 {
			t.Errorf("lcd %d = %q/%q at t=0, want the goodbye straight away", seat, line1, line2)
		}
	}

	r.at(300) // the wink is 250ms to full on every lamp of both seats
	for seat := 0; seat < 2; seat++ {
		for i, v := range r.hw.LampLevels(seat) {
			if v != hardware.LampLevelMax {
				t.Fatalf("seat %d lamp %d = %d after the wink, want full", seat, i, v)
			}
		}
	}

	// Mid-unwind: aux left first and green has not left at all, so the
	// levels read back in bloom order.
	r.at(900)
	for seat := 0; seat < 2; seat++ {
		lv := r.hw.LampLevels(seat)
		if lv[4] != lv[5] {
			t.Errorf("seat %d aux lamps = %d/%d, want the pair together", seat, lv[4], lv[5])
		}
		for i := 4; i > 0; i-- {
			if lv[i] > lv[i-1] {
				t.Errorf("seat %d lamps = %v at 900ms, want them out in reverse bloom order", seat, lv)
				break
			}
		}
		if lv[0] != hardware.LampLevelMax {
			t.Errorf("seat %d green = %d at 900ms, want it still full (it leaves last, at 1000ms)", seat, lv[0])
		}
		if lv[4] >= lv[0] {
			t.Errorf("seat %d aux = %d at 900ms, want it well ahead of green", seat, lv[4])
		}
	}
	if got := r.hw.BacklightLevel(); got >= hardware.LampLevelMax || got <= powerOffHeld {
		t.Errorf("backlight = %d at 900ms, want it part-way down to %d", got, powerOffHeld)
	}

	// The backlight ramp is 1500ms; green's fade ends at 1600.
	r.at(1600)
	if got := r.hw.BacklightLevel(); got != powerOffHeld {
		t.Errorf("backlight = %d after the ramp, want %d", got, powerOffHeld)
	}
	r.at(1700)
	for seat := 0; seat < 2; seat++ {
		if lv := r.hw.LampLevels(seat); lv != (hardware.LampLevels{}) {
			t.Errorf("seat %d lamps = %v at 1700ms, want every one out", seat, lv)
		}
	}

	r.at(2100)
	if r.rig.RT.QuitRequested() {
		t.Fatal("quit requested before the 2200ms deadline")
	}
	r.at(2300)
	if !r.rig.RT.QuitRequested() {
		t.Error("no quit requested once the sequence finished")
	}
	if got := r.hw.BacklightLevel(); got != powerOffHeld {
		t.Errorf("backlight = %d at the end, want it left holding at %d", got, powerOffHeld)
	}
}

func TestPowerOffIgnoresInput(t *testing.T) {
	r := startPowerOff(t)
	r.at(600)
	r.rig.Press(input.KeyJoyA)
	r.at(700)
	if got := r.rig.RT.CurrentSlug(); got != "" {
		t.Errorf("a press during power-off loaded %q", got)
	}
	r.at(2300)
	if !r.rig.RT.QuitRequested() {
		t.Error("a press during power-off stopped the sequence finishing")
	}
}
