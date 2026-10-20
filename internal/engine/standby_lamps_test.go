// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

import (
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// sleepOnDemand calls EnterSleep only on a queued press, like the shell's SLEEP tile, and stays awake once woken so it can stand in for the app a wake resumes.
type sleepOnDemand struct {
	ctx   *engine.AppContext
	queue input.Queue
}

func newSleepOnDemand(ctx *engine.AppContext) engine.App { return &sleepOnDemand{ctx: ctx} }

func (a *sleepOnDemand) Init() error {
	a.ctx.AddKeyMap([]*input.Mapping{{
		Conditions: []input.Condition{input.Key(input.KeyStart)},
		Queue:      &a.queue,
	}})
	return nil
}
func (a *sleepOnDemand) HandleInput(clock.FrameTime) {
	if len(a.queue.Drain()) > 0 && a.ctx.RT.StandbyState() == engine.StateActive {
		a.ctx.EnterSleep()
	}
}
func (a *sleepOnDemand) Update(clock.FrameTime)                      {}
func (a *sleepOnDemand) Render(clock.FrameTime, engine.RenderReason) {}
func (a *sleepOnDemand) Teardown() error                             { return nil }

func init() {
	engine.Register(engine.Manifest{
		Slug:   "test/sleep-on-demand",
		Name:   "Sleep On Demand",
		Type:   engine.TypeGame,
		Hidden: true,
		New:    newSleepOnDemand,
	})
}

// A deliberate sleep (test/sleeper's EnterSleep, the SLEEP tile's route) ramps every lamp and the LCD backlight down without brightening, fully dark by sleepLampFadeMs. test/sleeper sleeps on its first tick, so no press is needed.
func TestDeliberateSleepFadesLampsToDarkOver2500ms(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     "test/sleeper",
		AttractAfter: 5 * time.Minute,
		StandbyAfter: 10 * time.Minute,
	})
	hw, ok := rig.RT.RawHardware().(*hardware.Virtual)
	if !ok {
		t.Fatalf("RawHardware is %T, want *hardware.Virtual", rig.RT.RawHardware())
	}
	rig.RT.Hardware.SetLights(-1, true)

	rig.Tick(1) // sleeper's HandleInput calls EnterSleep this tick
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("StandbyState = %v after EnterSleep, want StateDark", got)
	}

	prev := [2]hardware.LampLevels{hw.LampLevels(0), hw.LampLevels(1)}
	prevBacklight := hw.BacklightLevel()
	if prevBacklight != hardware.LampLevelMax {
		t.Fatalf("backlight at sleep = %d, want full (%d)", prevBacklight, hardware.LampLevelMax)
	}
	// 2500ms at the rig's 30fps is 75 frames (33.3ms each).
	for i := 1; i <= 75; i++ {
		rig.Tick(1)
		for seat := 0; seat < 2; seat++ {
			cur := hw.LampLevels(seat)
			for j, v := range cur {
				if v > prev[seat][j] {
					t.Fatalf("seat %d lamp %d (%s) rose from %d to %d at tick %d",
						seat, j, hardware.LightButtons[j], prev[seat][j], v, i)
				}
			}
			prev[seat] = cur
		}
		if bl := hw.BacklightLevel(); bl > prevBacklight {
			t.Fatalf("backlight rose from %d to %d at tick %d", prevBacklight, bl, i)
		} else {
			prevBacklight = bl
		}
	}

	for seat := 0; seat < 2; seat++ {
		if got := hw.LampLevels(seat); got != (hardware.LampLevels{}) {
			t.Errorf("seat %d lamps at 2.5s = %+v, want all dark", seat, got)
		}
	}
	if got := hw.BacklightLevel(); got != 0 {
		t.Errorf("backlight at 2.5s = %d, want dark", got)
	}
}

// Idle-timeout ATTRACT to DARK has no deliberate EnterSleep, so no goodnight and no lamp fade. test/bounce never sleeps, so only the idle timers lead to DARK.
func TestIdleTimeoutDarkStillCutsLampsInstantly(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     "test/bounce",
		AttractAfter: 50 * time.Millisecond,
		StandbyAfter: 100 * time.Millisecond,
	})
	hw, ok := rig.RT.RawHardware().(*hardware.Virtual)
	if !ok {
		t.Fatalf("RawHardware is %T, want *hardware.Virtual", rig.RT.RawHardware())
	}
	rig.RT.Hardware.SetLights(-1, true)

	rig.TickFor("60ms")
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("StandbyState = %v, want StateAttract", got)
	}
	rig.TickFor("110ms")
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("StandbyState = %v, want StateDark", got)
	}

	for seat := 0; seat < 2; seat++ {
		if got := hw.LampLevels(seat); got != (hardware.LampLevels{}) {
			t.Errorf("seat %d lamps after idle dark = %+v, want instantly dark (no fade)", seat, got)
		}
	}
	if got := hw.BacklightLevel(); got != 0 {
		t.Errorf("backlight after idle dark = %d, want instantly dark (no fade)", got)
	}
}

// leaveStandby resumes the parked app through hwShadow.restore, replaying the snapshot from before enterDark's fade and cancelling it, so a wake mid-goodnight leaves lamps where the app had them. The backlight is not in the snapshot; leaveStandby relights it to full instantly.
func TestWakeMidSleepFadeRestoresPreSleepLampLevels(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     "test/sleep-on-demand",
		AttractAfter: 5 * time.Minute,
		StandbyAfter: 10 * time.Minute,
	})
	hw, ok := rig.RT.RawHardware().(*hardware.Virtual)
	if !ok {
		t.Fatalf("RawHardware is %T, want *hardware.Virtual", rig.RT.RawHardware())
	}

	pre := hardware.LampLevels{hardware.LampLevelMax, 200, 150, 100, 50, 17}
	rig.RT.FadeLamps(-1, pre, hardware.AllLights, 0)

	rig.Press(input.KeyStart) // the SLEEP tile's press, deliberately
	rig.Tick(1)               // EnterSleep runs this tick, fade begins
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("StandbyState = %v after EnterSleep, want StateDark", got)
	}

	rig.Tick(4) // partway down the goodnight ramp
	if got := hw.BacklightLevel(); got == 0 || got >= hardware.LampLevelMax {
		t.Fatalf("backlight partway into the fade = %d, want it mid-ramp", got)
	}

	rig.Tick(30) // ~1s into the 2.5s fade
	if got := hw.LampLevels(0); got == pre {
		t.Fatalf("lamps unchanged 1s into the fade, want them ramping down")
	}

	rig.Press(input.KeyJoyA) // any real press wakes, mid-fade
	rig.Tick(1)
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("StandbyState = %v after a press mid-fade, want StateActive", got)
	}

	for seat := 0; seat < 2; seat++ {
		if got := hw.LampLevels(seat); got != pre {
			t.Errorf("seat %d lamps after waking mid-fade = %+v, want pre-sleep %+v", seat, got, pre)
		}
	}
	if got := hw.BacklightLevel(); got != hardware.LampLevelMax {
		t.Errorf("backlight after waking mid-fade = %d, want instantly full (%d)", got, hardware.LampLevelMax)
	}
}
