// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import "testing"

func TestTransitionPriority(t *testing.T) {
	a, b := Manifest{Slug: "a"}, Manifest{Slug: "b"}
	cases := []struct {
		name string
		seq  []transition
		want transition
	}{
		{"load outranks unload", []transition{{kind: transitionLoad, manifest: a}, {kind: transitionUnload}}, transition{kind: transitionLoad, manifest: a}},
		{"load replaces unload", []transition{{kind: transitionUnload}, {kind: transitionLoad, manifest: a}}, transition{kind: transitionLoad, manifest: a}},
		{"latest load wins", []transition{{kind: transitionLoad, manifest: a}, {kind: transitionLoad, manifest: b}}, transition{kind: transitionLoad, manifest: b}},
		{"unload outranks dark", []transition{{kind: transitionUnload}, {kind: transitionDark, fade: true}}, transition{kind: transitionUnload}},
		{"power-off outranks load", []transition{{kind: transitionPowerOff}, {kind: transitionLoad, manifest: a}}, transition{kind: transitionPowerOff}},
		{"a deliberate dark keeps its goodnight", []transition{{kind: transitionDark, fade: true}, {kind: transitionDark}}, transition{kind: transitionDark, fade: true}},
	}
	for _, c := range cases {
		rt := &Runtime{}
		for _, tr := range c.seq {
			rt.setTransition(tr)
		}
		got := rt.transition
		if got.kind != c.want.kind || got.fade != c.want.fade || got.manifest.Slug != c.want.manifest.Slug {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}
