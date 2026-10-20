// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

import (
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
)

func TestLightShowFadeRisesMonotoneToTheMask(t *testing.T) {
	rig := newPeriphRig(t)
	rig.Tick(2)
	hw := virtualHW(t, rig)

	// The app's own lamps are the fade's starting point: seat 0 begins dark
	// except lampAppSeats[0] (joya), so joyb should climb from 0 to full.
	rig.RT.RequestLightShow(engine.LightShow{
		Steps:   []engine.LightStep{{Seats: seats(hardware.AllLights, 0), Fade: 500 * time.Millisecond, Hold: time.Second}},
		Timeout: 2 * time.Second,
	})
	rig.Tick(1)

	last := hw.LampLevels(0)[1] // joyb: dark at the start of the fade
	if last != 0 {
		t.Fatalf("joyb at fade start = %d, want 0", last)
	}
	for i := 0; i < 9; i++ {
		rig.Tick(1)
		got := hw.LampLevels(0)[1]
		if got < last {
			t.Fatalf("joyb dropped from %d to %d mid-fade, want monotone rise", last, got)
		}
		last = got
	}
	if last == 0 || last == hardware.LampLevelMax {
		t.Fatalf("joyb = %d partway through the fade (~300ms of 500ms), want it strictly between 0 and %d", last, hardware.LampLevelMax)
	}

	rig.Tick(10) // past the 500ms fade
	if got := hw.LampLevels(0)[1]; got != hardware.LampLevelMax {
		t.Errorf("joyb after the fade = %d, want the step's mask level %d", got, hardware.LampLevelMax)
	}
	if got := litMask(hw, 0); got != hardware.AllLights {
		t.Errorf("seat 0 mask after the fade = %v, want %v", got, hardware.AllLights)
	}
}

func TestLightShowFadeZeroIsAHardCut(t *testing.T) {
	rig := newPeriphRig(t)
	rig.Tick(2)
	hw := virtualHW(t, rig)

	show := seats(hardware.AllLights, 0)
	rig.RT.RequestLightShow(engine.LightShow{
		Steps:   []engine.LightStep{{Seats: show}},
		Timeout: time.Second,
	})
	rig.Tick(1)

	if got := litMask(hw, 0); got != show[0] {
		t.Errorf("a zero-fade step should cut instantly: seat 0 = %v, want %v", got, show[0])
	}
	if got := hw.LampLevels(0)[1]; got != hardware.LampLevelMax {
		t.Errorf("a zero-fade step should land at full brightness immediately: joyb = %d, want %d", got, hardware.LampLevelMax)
	}
}

func TestLightShowNextStepCutsALongerFade(t *testing.T) {
	rig := newPeriphRig(t)
	rig.Tick(2)
	hw := virtualHW(t, rig)

	a := hardware.LightMask(1 << 1) // joyb
	b := hardware.LightMask(1 << 2) // joyc
	rig.RT.RequestLightShow(engine.LightShow{
		Steps: []engine.LightStep{
			// A one-second fade with only a 500ms hold: the step advances
			// long before joyb finishes climbing to full.
			{Seats: seats(a, 0), Fade: time.Second, Hold: 500 * time.Millisecond},
			{Seats: seats(b, 0), Fade: 0, Hold: time.Second},
		},
		Timeout: 2 * time.Second,
	})
	rig.Tick(1)
	// 14 frames at 30fps is ~467ms — inside the hold and well into the fade.
	rig.Tick(14)

	partial := hw.LampLevels(0)[1] // joyb, wherever step A's fade got cut off
	if partial == 0 || partial == hardware.LampLevelMax {
		t.Fatalf("joyb at the cut = %d, want it interrupted strictly between 0 and %d", partial, hardware.LampLevelMax)
	}

	// A couple more frames crosses the 500ms hold and cuts to step B.
	rig.Tick(2)
	if got := litMask(hw, 0); got != b {
		t.Fatalf("step B should have cut in hard: seat 0 = %v, want %v", got, b)
	}
	if got := hw.LampLevels(0)[1]; got != 0 {
		t.Errorf("joyb after step B's hard cut = %d, want 0 (not held at the interrupted %d)", got, partial)
	}
}
