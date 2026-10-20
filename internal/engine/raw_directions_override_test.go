// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

// Covers Runtime.SetRawDirectionsOverride, the engine side of host_set_raw_directions: the override beats the manifest in both directions and resets on app switch, even to a fresh instance of the same app.

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// switchableDirCaptureApp is raw_directions_test.go's dirCaptureApp plus an optional Launch target: Enter switches to it, driving a genuine app switch.
type switchableDirCaptureApp struct {
	ctx    *engine.AppContext
	last   input.KeyName
	target string
}

func (a *switchableDirCaptureApp) Init() error {
	a.ctx.AddKeyMap([]*input.Mapping{
		{
			Conditions: []input.Condition{
				input.Key(input.KeyUp), input.Key(input.KeyDown),
				input.Key(input.KeyLeft), input.Key(input.KeyRight),
			},
			Trigger: func(ev input.Event) { a.last = ev.Name },
		},
		{
			Conditions: []input.Condition{input.Key(input.KeyEnter)},
			Trigger: func(input.Event) {
				if a.target != "" {
					_ = a.ctx.Launch(a.target)
				}
			},
		},
	})
	return nil
}
func (a *switchableDirCaptureApp) HandleInput(clock.FrameTime)                 {}
func (a *switchableDirCaptureApp) Update(clock.FrameTime)                      {}
func (a *switchableDirCaptureApp) Render(clock.FrameTime, engine.RenderReason) {}
func (a *switchableDirCaptureApp) Teardown() error                             { return nil }

func TestSetRawDirectionsOverrideTrueWinsOverManifestFalse(t *testing.T) {
	const slug = "test/rawdir-override-on"
	var app *switchableDirCaptureApp
	engine.Register(engine.Manifest{
		Slug: slug, Name: "Override On", Type: engine.TypeGame, Hidden: true,
		New: func(ctx *engine.AppContext) engine.App {
			app = &switchableDirCaptureApp{ctx: ctx}
			return app
		},
	})
	t.Cleanup(func() { engine.Unregister(slug) })

	rig := testkit.NewRig(t, testkit.Options{StartApp: slug})
	rig.RT.SetRawDirectionsOverride(true)
	rig.PressAs(input.KeyRight, 0)
	rig.Tick(1)

	if app.last != input.KeyRight {
		t.Fatalf("override=true over manifest RawDirections=false: app saw %q, want unchanged KeyRight (raw pass-through)", app.last)
	}
}

func TestSetRawDirectionsOverrideFalseWinsOverManifestTrue(t *testing.T) {
	const slug = "test/rawdir-override-off"
	var app *switchableDirCaptureApp
	engine.Register(engine.Manifest{
		Slug: slug, Name: "Override Off", Type: engine.TypeGame, Hidden: true,
		RawDirections: true,
		New: func(ctx *engine.AppContext) engine.App {
			app = &switchableDirCaptureApp{ctx: ctx}
			return app
		},
	})
	t.Cleanup(func() { engine.Unregister(slug) })

	rig := testkit.NewRig(t, testkit.Options{StartApp: slug})
	rig.RT.SetRawDirectionsOverride(false)
	rig.PressAs(input.KeyRight, 0)
	rig.Tick(1)

	if app.last != input.KeyUp {
		t.Fatalf("override=false over manifest RawDirections=true: app saw %q, want remapped KeyUp (seat 0/left mount, upright orientation, +3 steps)", app.last)
	}
}

func TestSetRawDirectionsOverrideResetsOnAppSwitch(t *testing.T) {
	const slugA = "test/rawdir-override-reset-a"
	const slugB = "test/rawdir-override-reset-b"
	var appA, appB *switchableDirCaptureApp
	engine.Register(engine.Manifest{
		Slug: slugA, Name: "Reset A", Type: engine.TypeGame, Hidden: true,
		New: func(ctx *engine.AppContext) engine.App {
			appA = &switchableDirCaptureApp{ctx: ctx, target: slugB}
			return appA
		},
	})
	engine.Register(engine.Manifest{
		Slug: slugB, Name: "Reset B", Type: engine.TypeGame, Hidden: true,
		New: func(ctx *engine.AppContext) engine.App {
			appB = &switchableDirCaptureApp{ctx: ctx}
			return appB
		},
	})
	t.Cleanup(func() { engine.Unregister(slugA); engine.Unregister(slugB) })

	rig := testkit.NewRig(t, testkit.Options{StartApp: slugA})
	rig.RT.SetRawDirectionsOverride(true)
	rig.PressAs(input.KeyRight, 0)
	rig.Tick(1)
	if appA.last != input.KeyRight {
		t.Fatalf("app A: override=true not in effect before the switch: saw %q, want KeyRight", appA.last)
	}

	// Enter triggers ctx.Launch during this tick's HandleInput; the switch applies at the top of the next tick.
	rig.PressAs(input.KeyEnter, 0)
	rig.Tick(1) // dispatches HandleInput: Launch(slugB) queues the switch
	rig.Tick(1) // applies the pending switch: loadApp(slugB)

	rig.PressAs(input.KeyRight, 0)
	rig.Tick(1)
	if appB.last != input.KeyUp {
		t.Fatalf("app B (fresh instance after switch): saw %q, want remapped KeyUp — a leaked override would show KeyRight here", appB.last)
	}
}
