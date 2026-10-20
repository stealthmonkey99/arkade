// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

// Covers Manifest.RawDirections: remapEvent passes controller directions through untouched for an app whose manifest sets it, and keeps remapping for the default. Driven through testkit because remapEvent is unexported.

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// dirCaptureApp records the last directional KeyName it received; remapEvent runs before Dispatch, so this is what the app sees after any remap.
type dirCaptureApp struct {
	ctx  *engine.AppContext
	last input.KeyName
}

func (a *dirCaptureApp) Init() error {
	a.ctx.AddKeyMap([]*input.Mapping{{
		Conditions: []input.Condition{
			input.Key(input.KeyUp), input.Key(input.KeyDown),
			input.Key(input.KeyLeft), input.Key(input.KeyRight),
		},
		Trigger: func(ev input.Event) { a.last = ev.Name },
	}})
	return nil
}
func (a *dirCaptureApp) HandleInput(clock.FrameTime)                 {}
func (a *dirCaptureApp) Update(clock.FrameTime)                      {}
func (a *dirCaptureApp) Render(clock.FrameTime, engine.RenderReason) {}
func (a *dirCaptureApp) Teardown() error                             { return nil }

// Seat 0 (PositionLeft) under Upright is a non-identity remap (directionSteps = 3, input/keys.go): KeyRight would become KeyUp, so the test passes only if the skip fires.
func TestRemapEventRawDirectionsPassesThroughUnchanged(t *testing.T) {
	const slug = "test/rawdir-on"
	var app *dirCaptureApp
	engine.Register(engine.Manifest{
		Slug: slug, Name: "RawDir On", Type: engine.TypeGame, Hidden: true,
		RawDirections: true,
		New: func(ctx *engine.AppContext) engine.App {
			app = &dirCaptureApp{ctx: ctx}
			return app
		},
	})
	t.Cleanup(func() { engine.Unregister(slug) })

	rig := testkit.NewRig(t, testkit.Options{StartApp: slug})
	rig.PressAs(input.KeyRight, 0)
	rig.Tick(1)

	if app.last != input.KeyRight {
		t.Fatalf("RawDirections=true: app saw %q, want unchanged KeyRight (raw pass-through)", app.last)
	}
}

// Control: the same press without RawDirections still remaps. directionSteps = 3, so KeyRight (index 1 in keyOrder) becomes keyOrder[(1+3)%4] = KeyUp.
func TestRemapEventDefaultStillRemaps(t *testing.T) {
	const slug = "test/rawdir-off"
	var app *dirCaptureApp
	engine.Register(engine.Manifest{
		Slug: slug, Name: "RawDir Off", Type: engine.TypeGame, Hidden: true,
		New: func(ctx *engine.AppContext) engine.App {
			app = &dirCaptureApp{ctx: ctx}
			return app
		},
	})
	t.Cleanup(func() { engine.Unregister(slug) })

	rig := testkit.NewRig(t, testkit.Options{StartApp: slug})
	rig.PressAs(input.KeyRight, 0)
	rig.Tick(1)

	if app.last != input.KeyUp {
		t.Fatalf("default remap: app saw %q, want KeyUp (seat 0/left mount, upright orientation, +3 steps)", app.last)
	}
}
