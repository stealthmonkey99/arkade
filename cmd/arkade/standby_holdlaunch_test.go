// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

const holdLaunchSlug = "holdlaunch-testgame"

// fakeGame is the hold-launch target; its attract renderer earns it a showcase slot with no scores.
type fakeGame struct{}

func (fakeGame) Init() error                                 { return nil }
func (fakeGame) HandleInput(clock.FrameTime)                 {}
func (fakeGame) Update(clock.FrameTime)                      {}
func (fakeGame) Render(clock.FrameTime, engine.RenderReason) {}
func (fakeGame) Teardown() error                             { return nil }

type fakeAttractRenderer struct{}

func (fakeAttractRenderer) Attract(uint64) {}
func (fakeAttractRenderer) Close()         {}

func registerFakeGame(t *testing.T) {
	t.Helper()
	engine.Register(engine.Manifest{
		Slug: holdLaunchSlug,
		Name: "TESTGAME",
		Type: engine.TypeGame,
		New:  func(*engine.AppContext) engine.App { return fakeGame{} },
		NewAttract: func(*engine.AppContext) (engine.AttractRenderer, bool) {
			return fakeAttractRenderer{}, true
		},
	})
	t.Cleanup(func() { engine.Unregister(holdLaunchSlug) })
}

// attractOnGameSlot idles apps/clock into attract and advances to the fake
// game's slot, so a hold (launch) and a tap (resume apps/clock) end on different slugs.
func attractOnGameSlot(t *testing.T) *testkit.Rig {
	t.Helper()
	registerFakeGame(t)
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     "apps/clock",
		AttractAfter: 2 * time.Second,
		StandbyAfter: 10 * time.Minute, // never go dark mid-test
	})
	rig.TickFor("2200ms") // idle past attractAfter -> attract
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("expected StateAttract, got %v", got)
	}
	if got := rig.RT.CurrentSlug(); got != "attract" {
		t.Fatalf("expected attract app loaded, got %q", got)
	}
	// Two ambient slots of ~8s each, then the game slot.
	rig.TickFor("19s")
	return rig
}

func joyADown(rig *testkit.Rig) {
	rig.RT.Input.Push(input.Event{
		Kind:   input.KindKeydown,
		Name:   input.KeyJoyA,
		Source: input.PlayerSource{Type: input.SourceController},
	})
}

func joyAUp(rig *testkit.Rig) {
	rig.RT.Input.Push(input.Event{
		Kind:   input.KindKeyup,
		Name:   input.KeyJoyA,
		Source: input.PlayerSource{Type: input.SourceController},
	})
}

func TestAttractHoldLaunchesShownGame(t *testing.T) {
	rig := attractOnGameSlot(t)

	// Well short of the ~1.5s threshold: still attracting, hold registering, green lit.
	joyADown(rig)
	rig.Tick(3) // ~100ms
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("a brief hold must not have woken yet, got %v", got)
	}
	if f := rig.RT.AttractHoldFraction(); f <= 0 || f >= 1 {
		t.Fatalf("expected a partial hold fraction in (0,1), got %v", f)
	}
	hw := rig.RT.RawHardware().(*hardware.Virtual)
	for p := 0; p < 2; p++ {
		if !hw.ButtonLit(p, "joya") {
			t.Errorf("expected the green button lit as hold feedback, player %d dark", p)
		}
	}

	// Past the threshold the showcased game launches fresh and apps/clock is discarded.
	rig.TickFor("1700ms")
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("expected StateActive after a full hold, got %v", got)
	}
	if got := rig.RT.CurrentSlug(); got != holdLaunchSlug {
		t.Fatalf("expected hold to launch %q, got %q", holdLaunchSlug, got)
	}
}

func TestAttractGreenTapWakesToParkedApp(t *testing.T) {
	rig := attractOnGameSlot(t)

	// A tap under the threshold wakes and resumes the parked app.
	joyADown(rig)
	rig.Tick(6) // ~200ms, well short of the ~1.5s threshold
	joyAUp(rig)
	rig.Tick(1)

	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("expected a tap to wake to StateActive, got %v", got)
	}
	if got := rig.RT.CurrentSlug(); got != "apps/clock" {
		t.Fatalf("expected a tap to resume the parked apps/clock, got %q", got)
	}
}
