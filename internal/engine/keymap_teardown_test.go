// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

type multiMapApp struct {
	ctx    *engine.AppContext
	target string
	hits   int
}

func (a *multiMapApp) Init() error {
	if a.target == "" {
		return nil
	}
	a.ctx.AddKeyMap([]*input.Mapping{
		{
			Conditions: []input.Condition{input.Key(input.KeyEnter)},
			Trigger:    func(input.Event) { _ = a.ctx.Launch(a.target) },
		},
		{
			Conditions: []input.Condition{input.Key(input.KeyJoyA)},
			Trigger:    func(input.Event) { a.hits++ },
		},
	})
	return nil
}
func (a *multiMapApp) HandleInput(clock.FrameTime)                 {}
func (a *multiMapApp) Update(clock.FrameTime)                      {}
func (a *multiMapApp) Render(clock.FrameTime, engine.RenderReason) {}
func (a *multiMapApp) Teardown() error                             { return nil }

func TestMultiMappingKeyMapComesOffAtTeardown(t *testing.T) {
	const slugA = "test/keymap-teardown-a"
	const slugB = "test/keymap-teardown-b"
	var appA *multiMapApp
	engine.Register(engine.Manifest{
		Slug: slugA, Name: "Teardown A", Type: engine.TypeGame, Hidden: true,
		New: func(ctx *engine.AppContext) engine.App {
			appA = &multiMapApp{ctx: ctx, target: slugB}
			return appA
		},
	})
	engine.Register(engine.Manifest{
		Slug: slugB, Name: "Teardown B", Type: engine.TypeGame, Hidden: true,
		New: func(ctx *engine.AppContext) engine.App { return &multiMapApp{ctx: ctx} },
	})
	t.Cleanup(func() { engine.Unregister(slugA); engine.Unregister(slugB) })

	rig := testkit.NewRig(t, testkit.Options{StartApp: slugA})
	rig.Press(input.KeyJoyA)
	rig.Tick(1)
	if appA.hits != 1 {
		t.Fatalf("app A saw %d JoyA presses while loaded, want 1", appA.hits)
	}
	rig.Press(input.KeyEnter)
	rig.Tick(2)
	if got := rig.RT.CurrentSlug(); got != slugB {
		t.Fatalf("current app %q, want %q", got, slugB)
	}
	rig.Press(input.KeyJoyA)
	rig.Tick(1)
	if appA.hits != 1 {
		t.Errorf("unloaded app A's mapping still fired: %d hits", appA.hits)
	}
}
