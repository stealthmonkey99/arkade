// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

import (
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// sleeper calls EnterSleep from inside HandleInput on its first tick, the reentrancy shape of the shell's SLEEP entry, which suspends the app mid-Tick and leaves rt.cur nil for the same tick's Update.
type sleeper struct{ ctx *engine.AppContext }

func newSleeper(ctx *engine.AppContext) engine.App { return &sleeper{ctx: ctx} }

func (s *sleeper) Init() error { return nil }
func (s *sleeper) HandleInput(stats clock.FrameTime) {
	if s.ctx.RT.StandbyState() == engine.StateActive {
		s.ctx.EnterSleep()
	}
}
func (s *sleeper) Update(clock.FrameTime)                      {}
func (s *sleeper) Render(clock.FrameTime, engine.RenderReason) {}
func (s *sleeper) Teardown() error                             { return nil }

// idler stands in for the attract reel: the BACK-in-attract route only
// reaches Tick's input drain with something loaded, and this test binary
// has no real attract app.
type idler struct{}

func newIdler(*engine.AppContext) engine.App                 { return &idler{} }
func (i *idler) Init() error                                 { return nil }
func (i *idler) HandleInput(clock.FrameTime)                 {}
func (i *idler) Update(clock.FrameTime)                      {}
func (i *idler) Render(clock.FrameTime, engine.RenderReason) {}
func (i *idler) Teardown() error                             { return nil }

func init() {
	engine.Register(engine.Manifest{
		Slug:   "test/sleeper",
		Name:   "Sleeper",
		Type:   engine.TypeGame,
		Hidden: true,
		New:    newSleeper,
	})
	engine.Register(engine.Manifest{
		Slug:   "test/idle-attract",
		Name:   "Idle Attract",
		Type:   engine.TypeAttract,
		Hidden: true,
		New:    newIdler,
	})
}

func litFrame(rig *testkit.Rig) bool {
	for _, row := range rig.Frame() {
		for _, p := range row {
			if p != display.Black {
				return true
			}
		}
	}
	return false
}

// attractThenBack idles the cabinet into ATTRACT, then presses BACK there —
// the second deliberate route into dark, and the one the shell can't draw a
// goodnight for because it isn't loaded any more.
func attractThenBack(t *testing.T) *testkit.Rig {
	t.Helper()
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:       "test/bounce",
		AttractAfter:   100 * time.Millisecond,
		StandbyAfter:   time.Minute,
		HardDecayAfter: time.Minute,
	})
	rig.TickFor("150ms")
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("StandbyState = %v, want StateAttract before pressing BACK", got)
	}
	rig.Press(input.KeyJoyB)
	rig.Tick(2) // the press arms the dark, the next tick applies it
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("StandbyState = %v, want StateDark after BACK in attract", got)
	}
	return rig
}

func TestBackWhileAttractingPlaysTheGoodnight(t *testing.T) {
	rig := attractThenBack(t)

	if !litFrame(rig) {
		t.Fatal("BACK in attract cut straight to black; want the goodnight sequence")
	}
	rig.TickFor("1500ms")
	if !litFrame(rig) {
		t.Error("the goodnight went dark early — it should still be painting mid-sequence")
	}
	rig.TickFor("3000ms")
	if litFrame(rig) {
		t.Error("the cabinet should be fully black once the sequence has run its course")
	}
}

func TestKeyDuringTheGoodnightWakesTheCabinet(t *testing.T) {
	rig := attractThenBack(t)
	rig.TickFor("1000ms")

	rig.Press(input.KeyJoyA)
	rig.Tick(1)
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("StandbyState = %v after a press mid-goodnight, want StateActive", got)
	}
	if got := rig.RT.Status().SuspendedSlug; got != "" {
		t.Errorf("SuspendedSlug = %q, want the suspended app resumed by the wake", got)
	}
	// The animation stops the same tick rather than painting over the app
	// that just came back.
	rig.Tick(1)
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("StandbyState = %v a tick after waking, want it to stay active", got)
	}
}

// Nobody is in the room when the presence clock expires, so that dark skips the explanation and just stops painting.
func TestIdleTimeoutDarkStaysInstant(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     "test/bounce",
		AttractAfter: 100 * time.Millisecond,
		StandbyAfter: 200 * time.Millisecond,
	})
	rig.TickFor("400ms")
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("StandbyState = %v, want StateDark after the presence timeout", got)
	}
	if litFrame(rig) {
		t.Error("the idle-timeout dark should cut straight to black, not play the goodnight")
	}
}

// TestMotionDoesNotWakeMidGoodnight pins the interaction the sequence
// depends on: both deliberate routes into dark are hard darks, so the person
// still standing at the cabinet can't bounce it back into attract halfway
// through being told goodnight.
func TestMotionDoesNotWakeMidGoodnight(t *testing.T) {
	rig := attractThenBack(t)
	rig.TickFor("1000ms")

	rig.Motion()
	rig.Tick(1)
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("StandbyState = %v after motion mid-goodnight, want StateDark", got)
	}
	if !litFrame(rig) {
		t.Error("motion interrupted the goodnight; it should have carried on painting")
	}
}

func TestEnterSleepFromHandleInputGoesDarkWithoutPanic(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     "test/sleeper",
		AttractAfter: 5 * time.Minute,
		StandbyAfter: 10 * time.Minute,
	})

	// Tick 1: the app requests sleep mid-HandleInput; Tick must not continue into rt.cur.Update on a nil interface.
	rig.Tick(2)

	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("StandbyState = %v, want StateDark", got)
	}
	if got := rig.RT.Status().SuspendedSlug; got != "test/sleeper" {
		t.Fatalf("SuspendedSlug = %q, want %q (sleep must suspend, not unload)", got, "test/sleeper")
	}
}

func TestEnterSleepWithStandbyDisabledExits(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/sleeper"})

	rig.Tick(2)

	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("StandbyState = %v, want StateActive with standby disabled", got)
	}
	if got := rig.RT.CurrentSlug(); got == "test/sleeper" {
		t.Fatalf("CurrentSlug = %q, want the sleeper exited", got)
	}
	if got := rig.RT.SuspendedSlug(); got != "" {
		t.Fatalf("SuspendedSlug = %q, want nothing parked", got)
	}
}

// The SLEEP tile lands mid-Tick; a frame of stale picture before the sequence starts would be visible.
func TestEnterSleepPaintsTheGoodnightSameTick(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     "test/sleeper",
		AttractAfter: 5 * time.Minute,
		StandbyAfter: 10 * time.Minute,
	})

	rig.Tick(1)
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("StandbyState = %v, want StateDark", got)
	}
	if !litFrame(rig) {
		t.Error("the goodnight's first frame should paint on the tick SLEEP was pressed")
	}
}

func TestEnterSleepIsHardAndMotionDoesNotBounceBack(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:       "test/sleeper",
		AttractAfter:   5 * time.Minute,
		StandbyAfter:   10 * time.Minute,
		HardDecayAfter: 10 * time.Minute,
	})

	rig.Tick(2)
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("StandbyState = %v, want StateDark", got)
	}
	if !rig.RT.StandbyHard() {
		t.Fatal("expected EnterSleep to enter a hard dark, same as the BACK-in-attract path")
	}

	// The person who just pressed SLEEP is still standing there: motion
	// alone must not wake the cabinet back into attract.
	rig.RT.InjectMotion()
	rig.Tick(1)
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("StandbyState = %v after motion, want StateDark (motion alone must not undo a deliberate sleep)", got)
	}
}
