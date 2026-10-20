// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// dummySoloSlug names a stand-in game that locks an engine.OnePlayerStrategy
// on a JoyA press, rotating the display toward the pressing seat.
const dummySoloSlug = "test/dummysolo"

type dummySoloGame struct {
	ctx      *engine.AppContext
	strategy *engine.OnePlayerStrategy
	queue    input.Queue
}

func init() {
	engine.Register(engine.Manifest{
		Slug: dummySoloSlug,
		Name: "Dummy Solo",
		Type: engine.TypeGame,
		New: func(ctx *engine.AppContext) engine.App {
			return &dummySoloGame{ctx: ctx}
		},
	})
}

func (g *dummySoloGame) Init() error {
	g.strategy = engine.OnePlayerSolo(g.ctx.RT)
	g.ctx.PushPlayerStrategy(g.strategy)
	g.ctx.AddKeyMap([]*input.Mapping{{
		Conditions: []input.Condition{input.Key(input.KeyJoyA)},
		Queue:      &g.queue,
	}})
	return nil
}

func (g *dummySoloGame) HandleInput(clock.FrameTime) {
	for range g.queue.Drain() {
		g.strategy.Lock()
	}
}

func (g *dummySoloGame) Update(clock.FrameTime)                      {}
func (g *dummySoloGame) Render(clock.FrameTime, engine.RenderReason) {}
func (g *dummySoloGame) Teardown() error                             { return nil }

func TestStandbyCycleResetsGameRotation(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     dummySoloSlug,
		AttractAfter: 5 * time.Minute,
		StandbyAfter: 10 * time.Minute,
	})

	// Player 2's JoyA locks the strategy and rotates the display toward slot 1.
	rig.PressAs(input.KeyJoyA, 1)
	rig.Tick(1)
	if got := rig.RT.Renderer.VirtualOrientation(); got == display.Upright {
		t.Fatalf("setup: expected the locked solo game to rotate the display, still upright (state %v)", rig.RT.CurrentSlug())
	}

	rotated := rig.RT.Renderer.VirtualOrientation()

	// Attract renders upright while the game is suspended behind it.
	rig.TickFor("5m1s")
	// The idle countdown runs from the last press, so suspend lands just past 5m.
	beforeSuspendMs := 5 * 60 * 1000.0
	if got := rig.RT.CurrentSlug(); got != "attract" {
		t.Fatalf("expected attract loaded, got %q", got)
	}
	if got := rig.RT.SuspendedSlug(); got != dummySoloSlug {
		t.Fatalf("expected the dummy solo game suspended behind attract, got %q", got)
	}
	if got := rig.RT.Renderer.VirtualOrientation(); got != display.Upright {
		t.Errorf("attract should render upright, got %v", got)
	}

	rig.TickFor("10m1s")
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("expected StateDark, got %v", got)
	}
	rig.Motion()
	rig.Tick(1)
	if got := rig.RT.CurrentSlug(); got != "attract" {
		t.Fatalf("expected attract reloaded on motion wake, got %q", got)
	}
	if got := rig.RT.Renderer.VirtualOrientation(); got != display.Upright {
		t.Errorf("motion wake should show attract upright, got %v", got)
	}

	// The resumed game keeps its rotation, and attract and dark time stays out of DurationMs.
	rig.PressAs(input.KeyJoyA, 1)
	rig.Tick(2)
	if got := rig.RT.CurrentSlug(); got != dummySoloSlug {
		t.Fatalf("expected the suspended dummy solo game resumed after waking press, got %q", got)
	}
	if got := rig.RT.SuspendedSlug(); got != "" {
		t.Errorf("nothing should remain suspended after resume, got %q", got)
	}
	if got := rig.RT.Renderer.VirtualOrientation(); got != rotated {
		t.Errorf("resume should restore the game's rotation %v, got %v", rotated, got)
	}
	if got := rig.RT.Stats().DurationMs; got < beforeSuspendMs || got > beforeSuspendMs+60*1000 {
		t.Errorf("resumed game clock should continue from ~%.0fms (standby gap excluded), got %.0fms", beforeSuspendMs, got)
	}
}

// TestStandbyResumeRestoresHardware: the LCD text and button lights an app
// last wrote come back on resume, over the screensaver's own.
func TestStandbyResumeRestoresHardware(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     dummySoloSlug,
		AttractAfter: 5 * time.Minute,
		StandbyAfter: 10 * time.Minute,
	})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	rig.RT.Hardware.SetText(0, "P1 SCORE", "4242")
	rig.RT.Hardware.SetText(1, "P2 WAITS", "")
	rig.RT.Hardware.SetButtonLights(0, hardware.ButtonLights{"joyc": true}, true)
	rig.RT.Hardware.SetButtonLights(1, hardware.ButtonLights{"joyb": true, "joyaux1": true}, true)

	rig.TickFor("5m1s")
	if got := rig.RT.CurrentSlug(); got != "attract" {
		t.Fatalf("expected attract loaded, got %q", got)
	}
	status := strings.Join(hw.StatusLines(), "\n")
	if !strings.Contains(status, "Årkade") {
		t.Fatalf("expected attract to own the LCDs during the takeover, got %q", status)
	}

	rig.PressKeyboard(input.KeyEnter, false)
	rig.Tick(2)
	if got := rig.RT.CurrentSlug(); got != dummySoloSlug {
		t.Fatalf("expected the dummy solo game resumed, got %q", got)
	}
	status = strings.Join(hw.StatusLines(), "\n")
	for _, want := range []string{"P1 SCORE", "4242", "P2 WAITS"} {
		if !strings.Contains(status, want) {
			t.Errorf("resumed LCDs should show %q, got %q", want, status)
		}
	}
	wantLights := map[int]map[string]bool{
		0: {"joyc": true},
		1: {"joyb": true, "joyaux1": true},
	}
	for player, want := range wantLights {
		for _, name := range hardware.LightButtons {
			if got := hw.ButtonLit(player, name); got != want[name] {
				t.Errorf("player %d light %s = %v after resume, want %v", player, name, got, want[name])
			}
		}
	}
}
