// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

import (
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// darkRig idles a plain game to DARK through the timeouts. The idle route cuts lamps instantly, so everything the wake does afterwards is the bloom's. The standby timeout must outlast a whole bloom or the woken cabinet drops back to dark.
func darkRig(t *testing.T) (*testkit.Rig, *hardware.Virtual) {
	t.Helper()
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     "test/bounce",
		AttractAfter: 50 * time.Millisecond,
		StandbyAfter: 3 * time.Second,
	})
	hw, ok := rig.RT.RawHardware().(*hardware.Virtual)
	if !ok {
		t.Fatalf("RawHardware is %T, want *hardware.Virtual", rig.RT.RawHardware())
	}
	rig.TickFor("60ms")
	rig.TickFor("3100ms")
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("StandbyState = %v, want StateDark", got)
	}
	if got := hw.BacklightLevel(); got != 0 {
		t.Fatalf("backlight in dark = %d, want 0", got)
	}
	return rig, hw
}

// bloomTicks is wakeBloomMs at the rig's 30fps (33.3ms a frame), plus the
// wake tick itself — motion is folded in before the newly-requested app
// exists, so that tick paints nothing and steps nothing.
const bloomTicks = 28

func TestMotionWakeBloomsTheBacklightUp(t *testing.T) {
	rig, hw := darkRig(t)

	rig.Motion()
	rig.Tick(1)
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("StandbyState = %v after motion, want StateAttract", got)
	}
	if got := hw.BacklightLevel(); got != 0 {
		t.Fatalf("backlight on the wake tick = %d, want it still dark at the foot of the ramp", got)
	}

	prev := hw.BacklightLevel()
	sawMidRamp := false
	for i := 1; i <= bloomTicks; i++ {
		rig.Tick(1)
		bl := hw.BacklightLevel()
		if bl < prev {
			t.Fatalf("backlight fell from %d to %d at tick %d, want a monotonic ramp", prev, bl, i)
		}
		if bl > 0 && bl < hardware.LampLevelMax {
			sawMidRamp = true
		}
		prev = bl
	}
	if !sawMidRamp {
		t.Error("backlight never held a level between dark and full, want a ramp and not a switch")
	}
	if got := hw.BacklightLevel(); got != hardware.LampLevelMax {
		t.Errorf("backlight after the bloom = %d, want full (%d)", got, hardware.LampLevelMax)
	}
}

// PanelBrightness keeps reporting the operator's setting throughout, so readers mid-bloom (SetPanelDim, the shell slider) don't mistake the ramp for a chosen level.
func TestMotionWakeBloomsTheLedPanel(t *testing.T) {
	rig, _ := darkRig(t)
	rig.RT.SetPanelBrightness(80)

	rig.Motion()
	rig.Tick(1)
	if got := rig.Capture.Brightness(); got != 0 {
		t.Fatalf("panel brightness on the wake tick = %v, want 0", got)
	}

	prev := rig.Capture.Brightness()
	sawMidRamp := false
	for i := 1; i <= bloomTicks; i++ {
		rig.Tick(1)
		b := rig.Capture.Brightness()
		if b < prev {
			t.Fatalf("panel brightness fell from %v to %v at tick %d, want a monotonic ramp", prev, b, i)
		}
		if b > 0 && b < 0.8 {
			sawMidRamp = true
		}
		if got := rig.RT.PanelBrightness(); got != 80 {
			t.Fatalf("PanelBrightness = %d at tick %d, want the operator's 80 throughout the bloom", got, i)
		}
		prev = b
	}
	if !sawMidRamp {
		t.Error("panel brightness never held a level between dark and 80%, want a ramp and not a switch")
	}
	if got := rig.Capture.Brightness(); got != 0.8 {
		t.Errorf("panel brightness after the bloom = %v, want the operator's 0.8", got)
	}
}

// A lamp the loading app writes during the bloom must join the ramp, not jump over it: the attract reel lights buttons in Init instantly, and a fade armed at the wake could only ramp levels that already existed.
func TestALampLitDuringTheBloomComesUpWithIt(t *testing.T) {
	rig, hw := darkRig(t)

	rig.Motion()
	rig.Tick(3) // a few frames up the ramp, still well short of full

	rig.RT.Hardware.SetLights(-1, true)
	lit := hw.LampLevels(0)[0]
	if lit == 0 || lit >= hardware.LampLevelMax {
		t.Fatalf("a lamp switched full on mid-bloom reached the device at %d, want it faint on the ramp", lit)
	}

	prev := lit
	for i := 1; i <= bloomTicks; i++ {
		rig.Tick(1)
		cur := hw.LampLevels(0)[0]
		if cur < prev {
			t.Fatalf("lamp fell from %d to %d at tick %d, want a monotonic ramp", prev, cur, i)
		}
		prev = cur
	}
	if prev != hardware.LampLevelMax {
		t.Errorf("lamp after the bloom = %d, want the full level the app asked for (%d)",
			prev, hardware.LampLevelMax)
	}
}

// pressBloomTicks is pressBloomMs at the rig's 30fps, plus the tick the
// press lands on: the wake runs in that tick's input drain, downstream of
// where the fades already stepped.
const pressBloomTicks = 17

// The press bloom uses a shorter, front-loaded curve than motion, because somebody is waiting on it.
func TestPressWakeBloomsAtItsOwnSpeed(t *testing.T) {
	rig, hw := darkRig(t)

	rig.Press(input.KeyJoyA)
	rig.Tick(1)
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("StandbyState = %v after a press, want StateActive", got)
	}
	if got := hw.BacklightLevel(); got != 0 {
		t.Fatalf("backlight on the press tick = %d, want it at the foot of the ramp", got)
	}

	prev := hw.BacklightLevel()
	sawMidRamp := false
	for i := 1; i <= pressBloomTicks; i++ {
		rig.Tick(1)
		bl := hw.BacklightLevel()
		if bl < prev {
			t.Fatalf("backlight fell from %d to %d at tick %d, want a monotonic ramp", prev, bl, i)
		}
		if bl > 0 && bl < hardware.LampLevelMax {
			sawMidRamp = true
		}
		prev = bl
	}
	if !sawMidRamp {
		t.Error("backlight never held a level between dark and full, want a ramp and not a switch")
	}
	if got := hw.BacklightLevel(); got != hardware.LampLevelMax {
		t.Errorf("backlight after the press bloom = %d, want full (%d)", got, hardware.LampLevelMax)
	}
}

// EaseOut over pressBloomMs puts the press well ahead of EaseInOut over wakeBloomMs at the same point.
func TestPressBloomOutrunsTheMotionBloom(t *testing.T) {
	const at = 6 // ~200ms in

	byMotion, hw := darkRig(t)
	byMotion.Motion()
	byMotion.Tick(1 + at)
	motion := hw.BacklightLevel()

	byPress, hw2 := darkRig(t)
	byPress.Press(input.KeyJoyA)
	byPress.Tick(1 + at)
	press := hw2.BacklightLevel()

	if press <= motion {
		t.Errorf("backlight %d ticks in: press %d, motion %d — want the press well ahead",
			at, press, motion)
	}
	if motion == 0 || press >= hardware.LampLevelMax {
		t.Errorf("backlight %d ticks in: press %d, motion %d — want both still climbing",
			at, press, motion)
	}
}

// A press partway up the motion ramp must finish on the press's curve instead of jumping to full.
func TestPressMidBloomHurriesItRatherThanSnapping(t *testing.T) {
	rig, hw := darkRig(t)

	rig.Motion()
	rig.Tick(5) // partway up the slow ramp
	caught := hw.BacklightLevel()
	if caught == 0 || caught >= hardware.LampLevelMax {
		t.Fatalf("backlight partway into the motion bloom = %d, want it mid-ramp", caught)
	}

	rig.Press(input.KeyJoyA)
	rig.Tick(1)
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("StandbyState = %v after a press mid-bloom, want StateActive", got)
	}
	if got := hw.BacklightLevel(); got == hardware.LampLevelMax {
		t.Fatal("backlight snapped to full on a press mid-bloom, want the ramp hurried along")
	}

	prev := hw.BacklightLevel()
	if prev < caught {
		t.Fatalf("backlight dipped from %d to %d on the press, want it to carry on up", caught, prev)
	}
	for i := 1; i <= pressBloomTicks; i++ {
		rig.Tick(1)
		bl := hw.BacklightLevel()
		if bl < prev {
			t.Fatalf("backlight fell from %d to %d at tick %d, want a monotonic ramp", prev, bl, i)
		}
		prev = bl
	}
	if prev != hardware.LampLevelMax {
		t.Errorf("backlight after the hurried bloom = %d, want full (%d)", prev, hardware.LampLevelMax)
	}
}

// ACTIVE to ATTRACT was never dark, so there is nothing to bring up and no dip.
func TestIdleAttractDoesNotBloom(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     "test/bounce",
		AttractAfter: 50 * time.Millisecond,
		StandbyAfter: 10 * time.Minute,
	})
	hw, ok := rig.RT.RawHardware().(*hardware.Virtual)
	if !ok {
		t.Fatalf("RawHardware is %T, want *hardware.Virtual", rig.RT.RawHardware())
	}
	rig.RT.Hardware.SetLights(-1, true)

	for i := 1; i <= 30; i++ {
		rig.Tick(1)
		if got := hw.BacklightLevel(); got != hardware.LampLevelMax {
			t.Fatalf("backlight = %d at tick %d, want it never to leave full across an idle attract", got, i)
		}
		if got := rig.Capture.Brightness(); got != 1 {
			t.Fatalf("panel brightness = %v at tick %d, want it never to dip across an idle attract", got, i)
		}
	}
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("StandbyState = %v, want StateAttract", got)
	}
}
