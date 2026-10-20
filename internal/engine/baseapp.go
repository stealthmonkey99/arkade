// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/fsm"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
)

// BaseApp is the common app skeleton: a scene manager and input queue with
// no-op lifecycle defaults. Embed it and override what you need.
type BaseApp struct {
	Ctx      *AppContext
	SceneMgr *scene.Manager
	Queue    input.Queue
}

// NewBaseApp builds the skeleton; pass the FSM that names your scenes.
func NewBaseApp(ctx *AppContext, machine *fsm.FSM) BaseApp {
	return BaseApp{Ctx: ctx, SceneMgr: scene.NewManager(ctx, machine)}
}

// Scenes lets the engine drive scene update/render automatically.
func (b *BaseApp) Scenes() *scene.Manager { return b.SceneMgr }

// QueueKeys registers a key map routing the given keys into b.Queue.
func (b *BaseApp) QueueKeys(names ...input.KeyName) {
	conds := make([]input.Condition, len(names))
	for i, n := range names {
		conds[i] = input.Key(n)
	}
	b.Ctx.AddKeyMap([]*input.Mapping{{Conditions: conds, Queue: &b.Queue}})
}

// HandleInput drains queued events into the scenes.
func (b *BaseApp) HandleInput(stats clock.FrameTime) {
	for _, ev := range b.Queue.Drain() {
		if b.SceneMgr != nil {
			b.SceneMgr.HandleInput(stats, ev)
		}
	}
}

// Update is a no-op; the engine updates scenes.
func (b *BaseApp) Update(stats clock.FrameTime) {}

// Render is a no-op; the engine renders scenes.
func (b *BaseApp) Render(stats clock.FrameTime, reason RenderReason) {}

// Teardown is a no-op by default.
func (b *BaseApp) Teardown() error { return nil }
