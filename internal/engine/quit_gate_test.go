// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

// The idle seat can always leave a game. A locked one-player strategy drops every other controller's records, which without an exemption would include B. gateInput exempts quit keys for everyone but must still block the idle seat from playing; these tests pin both halves.
//
// They run through the real engine via testkit because gateInput is unexported.

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// keyCaptureApp records every key that reaches it, quit and directional
// alike, so one app can answer both halves of the contract.
type keyCaptureApp struct {
	ctx  *engine.AppContext
	seen []input.KeyName
}

func (a *keyCaptureApp) Init() error {
	a.ctx.AddKeyMap([]*input.Mapping{{
		Conditions: []input.Condition{
			input.Key(input.KeyJoyB), input.Key(input.KeyJoyA),
			input.Key(input.KeyUp), input.Key(input.KeyDown),
			input.Key(input.KeyLeft), input.Key(input.KeyRight),
		},
		Trigger: func(ev input.Event) { a.seen = append(a.seen, ev.Name) },
	}})
	return nil
}
func (a *keyCaptureApp) HandleInput(clock.FrameTime)                 {}
func (a *keyCaptureApp) Update(clock.FrameTime)                      {}
func (a *keyCaptureApp) Render(clock.FrameTime, engine.RenderReason) {}
func (a *keyCaptureApp) Teardown() error                             { return nil }

func (a *keyCaptureApp) sawAny(k input.KeyName) bool {
	for _, s := range a.seen {
		if s == k {
			return true
		}
	}
	return false
}

// lockedSoloRig starts an app, locks a solo strategy to physical seat 0,
// and hands back the app so a test can press from the OTHER seat.
func lockedSoloRig(t *testing.T, slug string) (*testkit.Rig, *keyCaptureApp) {
	t.Helper()
	var app *keyCaptureApp
	engine.Register(engine.Manifest{
		Slug: slug, Name: "Quit Gate", Type: engine.TypeGame, Hidden: true,
		New: func(ctx *engine.AppContext) engine.App {
			app = &keyCaptureApp{ctx: ctx}
			return app
		},
	})
	t.Cleanup(func() { engine.Unregister(slug) })

	rig := testkit.NewRig(t, testkit.Options{StartApp: slug})
	s := engine.OnePlayerSolo(rig.RT)
	rig.RT.PushPlayerStrategy(s)
	s.LockToSeat(input.SeatP1) // the human is P1; P2 is the bystander
	return rig, app
}

// Seat 1's B reaches the cartridge even though seat 0 owns the game; the cartridge still decides what to do with it.
func TestLockedSoloStillLetsTheIdleSeatQuit(t *testing.T) {
	rig, app := lockedSoloRig(t, "test/quitgate-b")

	rig.PressAs(input.KeyJoyB, 1)
	rig.Tick(1)

	if !app.sawAny(input.KeyJoyB) {
		t.Fatalf("idle seat's B never reached the app (saw %v) — the person not playing cannot leave the game", app.seen)
	}
}

func TestLockedSoloStillBlocksTheIdleSeatFromPlaying(t *testing.T) {
	rig, app := lockedSoloRig(t, "test/quitgate-dir")

	rig.PressAs(input.KeyRight, 1)
	rig.PressAs(input.KeyJoyA, 1)
	rig.Tick(1)

	if len(app.seen) != 0 {
		t.Fatalf("idle seat's non-quit keys reached the app (%v) — the exemption must cover quit ONLY", app.seen)
	}
}

func TestLockedSoloOwnerStillPlays(t *testing.T) {
	rig, app := lockedSoloRig(t, "test/quitgate-owner")

	rig.PressAs(input.KeyJoyA, 0)
	rig.Tick(1)

	if !app.sawAny(input.KeyJoyA) {
		t.Fatalf("the playing seat's own A did not reach the app (saw %v)", app.seen)
	}
}
