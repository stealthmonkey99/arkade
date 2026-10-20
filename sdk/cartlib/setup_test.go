// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

import "testing"

type setupKeeper struct {
	gameStub
	kept *Setup
}

func (g *setupKeeper) Init(s *Setup) { g.kept = s }

func mustPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s did not panic on a dead Setup", what)
		}
	}()
	fn()
}

func TestSetupIsLiveOnlyInsideInit(t *testing.T) {
	g := &setupKeeper{}
	Run(g)
	dispatchInit()
	if g.kept != &setup {
		t.Fatal("Init did not receive cartlib's own Setup")
	}
	if setup.live {
		t.Fatal("the Setup stayed live after Init returned")
	}
	mustPanic(t, "LoadSound", func() { g.kept.LoadSound("assets/x.wav") })
	mustPanic(t, "LoadSheet", func() { g.kept.LoadSheet("assets/x.arkadesprite.png") })
	mustPanic(t, "BindButton", func() { g.kept.BindButton(P1 + ChanA) })
	mustPanic(t, "BindAxis", func() { g.kept.BindAxis(P1 + ChanJoyX) })
	mustPanic(t, "BindDelta", func() { g.kept.BindDelta(P1 + "spinner") })
	mustPanic(t, "BindBlob", func() { g.kept.BindBlob(P1 + "pad") })
	mustPanic(t, "BindOutput", func() { g.kept.BindOutput(P1 + "rumble") })
	mustPanic(t, "DefineLCDGlyph", func() { g.kept.DefineLCDGlyph('♥', GlyphRows{}) })
}

func TestTheZeroSetupIsDead(t *testing.T) {
	var s Setup
	mustPanic(t, "zero Setup", func() { s.LoadSound("assets/x.wav") })
	mustPanic(t, "nil Setup", func() { (*Setup)(nil).BindButton(P1 + ChanA) })
	if (Sheet{}).Cel("idle") != nil {
		t.Error("the zero Sheet lifted a cel")
	}
}
