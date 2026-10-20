// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/display"
)

func TestOrientationPolicyDesired(t *testing.T) {
	room, seated := display.CounterClockwise, display.Clockwise
	p := orientationPolicy{room: room, seated: seated}
	cases := []struct {
		state StandbyState
		boot  bool
		want  display.Orientation
	}{
		{StateActive, false, seated},
		{StateActive, true, room},
		{StateAttract, false, room},
		{StateDark, false, room},
	}
	for _, c := range cases {
		if got := p.desired(c.state, c.boot); got != c.want {
			t.Errorf("desired(%v, boot=%v) = %v, want %v", c.state, c.boot, got, c.want)
		}
	}
	p.override, p.overridden = display.Upright, true
	for _, c := range cases {
		if got := p.desired(c.state, c.boot); got != display.Upright {
			t.Errorf("override: desired(%v, boot=%v) = %v, want the override", c.state, c.boot, got)
		}
	}
}
