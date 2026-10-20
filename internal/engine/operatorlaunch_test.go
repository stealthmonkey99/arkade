// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/render"
)

func newPowerDownRig(t *testing.T, reasons *[]string) *saverRig {
	t.Helper()
	registerApp(t, Manifest{Slug: "test/foreground", Name: "Foreground", Type: TypeApp, Hidden: true, New: newStub})
	clk := clock.NewManualClock()
	rt, err := New(Options{
		Renderer:     render.NewCapture(false),
		Clock:        clk,
		DataDir:      t.TempDir(),
		StartApp:     "test/foreground",
		RandSeed:     42,
		AttractAfter: time.Minute,
		StandbyAfter: time.Minute,
		OnPowerDown:  func(reason string) { *reasons = append(*reasons, reason) },
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	if err := rt.Init(); err != nil {
		t.Fatalf("engine.Init: %v", err)
	}
	return &saverRig{t: t, rt: rt, clk: clk}
}

func (r *saverRig) idleToDark() {
	r.t.Helper()
	r.jump(time.Minute)
	r.jump(time.Minute)
	r.tick(1)
	if got := r.rt.StandbyState(); got != StateDark {
		r.t.Fatalf("setup: expected StateDark after idling, got %v", got)
	}
}

// A dashboard LAUNCH out of DARK powers the cabinet up as surely as a press
// does, so the next DARK must tell the host to power down again — including
// a SLEEP straight from the game, which never passes through attract.
func TestOperatorLaunchFromDarkReArmsPowerDown(t *testing.T) {
	var reasons []string
	rig := newPowerDownRig(t, &reasons)

	rig.idleToDark()
	if len(reasons) != 1 {
		t.Fatalf("OnPowerDown fired %d times on the first DARK, want 1", len(reasons))
	}

	rig.rt.RequestLaunchNow("test/foreground")
	rig.tick(2)
	if got := rig.rt.StandbyState(); got != StateActive {
		t.Fatalf("operator launch left the cabinet %v, want active", got)
	}

	rig.rt.enterSleepNow(rig.clk.Now())
	rig.tick(1)
	if len(reasons) != 2 {
		t.Errorf("OnPowerDown fired %d times across launch-from-dark and a sleep, want 2", len(reasons))
	}
}

// Launching out of a finished DARK brings the panel up the way a press does,
// rather than slamming it on at full.
func TestOperatorLaunchFromDarkBlooms(t *testing.T) {
	var reasons []string
	rig := newPowerDownRig(t, &reasons)
	rig.idleToDark()

	rig.rt.RequestLaunchNow("test/foreground")
	rig.tick(1)
	if !rig.rt.panel.bloom.active {
		t.Error("operator launch out of DARK started no wake bloom")
	}
}
