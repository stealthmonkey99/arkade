// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/render"
)

// orientationPolicy chooses the renderer's base "top" edge from the room and
// seated defaults and the rotation screen's override, remembers what it last
// pushed so the renderer is written only on change, and keeps the rotation
// the controller remap compensates for.
type orientationPolicy struct {
	room, seated display.Orientation
	override     display.Orientation
	overridden   bool

	pushed    display.Orientation
	pushedSet bool
	// input is the base plus per-player virtual rotation, sampled each tick.
	input display.Orientation
}

// desired is the override when pinned, the seated edge during play, and the
// room edge otherwise. Boot counts as the room even though its state is
// ACTIVE. With standby disabled the seated edge applies for good past boot.
func (p orientationPolicy) desired(state StandbyState, bootRunning bool) display.Orientation {
	switch {
	case p.overridden:
		return p.override
	case state == StateActive && !bootRunning:
		return p.seated
	default:
		return p.room
	}
}

// apply reports the base orientation the renderer should carry and whether
// it differs from the last one applied; the first call always reports a
// change, so the renderer is written once whatever it started on.
func (p *orientationPolicy) apply(state StandbyState, bootRunning bool) (display.Orientation, bool) {
	want := p.desired(state, bootRunning)
	if p.pushedSet && want == p.pushed {
		return want, false
	}
	p.pushed, p.pushedSet = want, true
	return want, true
}

// applyBaseOrientation pushes the policy's choice to the renderer on change.
func (rt *Runtime) applyBaseOrientation() {
	if o, changed := rt.orientation.apply(rt.standby.state, rt.bootAppRunning()); changed {
		rt.Renderer.SetBaseOrientation(o)
	}
}

// rendererInputOrientation is the rotation a side-mounted joystick must
// compensate for: base plus per-player virtual, excluding the hotkey layer.
func rendererInputOrientation(r render.Renderer) display.Orientation {
	o := r.VirtualOrientation()
	if bo, ok := r.(interface {
		BaseOrientation() display.Orientation
	}); ok {
		o = display.Orientation((int(o) + int(bo.BaseOrientation())) % 4)
	}
	return o
}

// SetRoomOrientation updates the idle/room default orientation live.
func (rt *Runtime) SetRoomOrientation(o display.Orientation) {
	rt.orientation.room = o
	rt.applyBaseOrientation()
}

// SetSeatedOrientation updates the active/seated default orientation live.
func (rt *Runtime) SetSeatedOrientation(o display.Orientation) {
	rt.orientation.seated = o
	rt.applyBaseOrientation()
}

// SetBaseOrientationOverride pins the renderer's base orientation to o,
// overriding the room/seated logic until ClearBaseOrientationOverride.
func (rt *Runtime) SetBaseOrientationOverride(o display.Orientation) {
	rt.orientation.override, rt.orientation.overridden = o, true
	rt.applyBaseOrientation()
}

// ClearBaseOrientationOverride removes the override and hands the base
// orientation back to the room/seated standby logic.
func (rt *Runtime) ClearBaseOrientationOverride() {
	if !rt.orientation.overridden {
		return
	}
	rt.orientation.overridden = false
	rt.applyBaseOrientation()
}
