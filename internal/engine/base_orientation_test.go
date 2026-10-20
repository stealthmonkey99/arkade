// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

import (
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

func TestBaseOrientationFollowsStandbyState(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:          "test/bounce",
		AttractAfter:      time.Second,
		StandbyAfter:      10 * time.Second,
		RoomOrientation:   display.CounterClockwise,
		SeatedOrientation: display.Clockwise,
	})

	rig.Tick(1)
	if got := rig.Capture.BaseOrientation(); got != display.Clockwise {
		t.Fatalf("active play should face the seated player, got %v", got)
	}

	// Idle past attractAfter: the screensaver takes over and the panel faces the room. No attract app is registered here, so cur stays nil and the cabinet cannot be woken; cmd/arkade/standby_test.go covers the full cycle.
	rig.TickFor("1500ms")
	if got := rig.Capture.BaseOrientation(); got != display.CounterClockwise {
		t.Fatalf("idle/attract should face the room, got %v", got)
	}
}

func TestBaseOrientationDefaultsUpright(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/bounce"})
	rig.Tick(1)
	if got := rig.Capture.BaseOrientation(); got != display.Upright {
		t.Fatalf("unconfigured base should be Upright, got %v", got)
	}
	rig.Press(input.KeyJoyA)
	rig.Tick(2)
	if got := rig.Capture.BaseOrientation(); got != display.Upright {
		t.Fatalf("unconfigured base should stay Upright after input, got %v", got)
	}
}

const (
	bootStubSlug  = "test/stub-boot"
	shellStubSlug = "test/stub-shell"
)

// bootStub stands in for apps/boot: it exits on its first Update, having reported whether its pairing flow waited for a human.
type bootStub struct {
	ctx    *engine.AppContext
	waited bool
}

func (a *bootStub) Init() error {
	if a.waited {
		a.ctx.RT.NoteBootPairingWaited()
	}
	return nil
}

func (a *bootStub) HandleInput(clock.FrameTime)                 {}
func (a *bootStub) Update(clock.FrameTime)                      { a.ctx.Exit() }
func (a *bootStub) Render(clock.FrameTime, engine.RenderReason) {}
func (a *bootStub) Teardown() error                             { return nil }

type shellStub struct{}

func (shellStub) Init() error                                 { return nil }
func (shellStub) HandleInput(clock.FrameTime)                 {}
func (shellStub) Update(clock.FrameTime)                      {}
func (shellStub) Render(clock.FrameTime, engine.RenderReason) {}
func (shellStub) Teardown() error                             { return nil }

// newStubRig registers a stand-in boot and shell (the attract reel is sleep_test.go's test/idle-attract) and builds a rig with no StartApp, so the engine starts at boot. Registrations are undone on cleanup because TestEngineQuitKeyStopsLoop needs no shell registered.
func newStubRig(t *testing.T, waited bool, attractAfter time.Duration) *testkit.Rig {
	t.Helper()
	registerStub(t, engine.Manifest{
		Slug: bootStubSlug, Name: "Boot", Type: engine.TypeBoot, Hidden: true,
		New: func(ctx *engine.AppContext) engine.App { return &bootStub{ctx: ctx, waited: waited} },
	})
	registerStub(t, engine.Manifest{
		Slug: shellStubSlug, Name: "Shell", Type: engine.TypeShell, Hidden: true,
		New: func(*engine.AppContext) engine.App { return shellStub{} },
	})
	return testkit.NewRig(t, testkit.Options{
		AttractAfter:      attractAfter,
		StandbyAfter:      10 * time.Minute,
		RoomOrientation:   display.CounterClockwise,
		SeatedOrientation: display.Clockwise,
	})
}

func registerStub(t *testing.T, m engine.Manifest) {
	t.Helper()
	engine.Register(m)
	t.Cleanup(func() { engine.Unregister(m.Slug) })
}

// A cabinet that paired both pads from remembered settings has nobody in front of it, so it lands in the screensaver facing the room, not the shell.
func TestBootUntouchedEntersAttract(t *testing.T) {
	rig := newStubRig(t, false, time.Minute)
	rig.Tick(4) // boot Update exits, the unload lands, attract loads

	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("boot should hand off to attract, state is %v (app %q)", got, rig.RT.CurrentSlug())
	}
	if got := rig.RT.CurrentSlug(); got != "test/idle-attract" {
		t.Fatalf("expected the attract reel, got %q", got)
	}
	if got := rig.Capture.BaseOrientation(); got != display.CounterClockwise {
		t.Fatalf("an untouched cabinet should face the room, got %v", got)
	}

	// The waking press is swallowed; the shell comes up already seated.
	rig.Press(input.KeyJoyA)
	rig.Tick(3)
	if got := rig.RT.CurrentSlug(); got != shellStubSlug {
		t.Fatalf("a press should wake to the shell, got %q", got)
	}
	if got := rig.Capture.BaseOrientation(); got != display.Clockwise {
		t.Fatalf("the woken shell should already face the seated player, got %v", got)
	}
}

// The pairing flow waited on a prompt, so somebody is pressing buttons; the screensaver would make them press again.
func TestBootPairedByHandGoesToShell(t *testing.T) {
	rig := newStubRig(t, true, time.Minute)
	rig.Tick(4)

	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("a hand-paired boot should stay active, state is %v", got)
	}
	if got := rig.RT.CurrentSlug(); got != shellStubSlug {
		t.Fatalf("expected the shell, got %q", got)
	}
	if got := rig.Capture.BaseOrientation(); got != display.Clockwise {
		t.Fatalf("somebody is at the cabinet, so it should face the seat, got %v", got)
	}
}

func TestBootWithStandbyDisabledGoesToShell(t *testing.T) {
	rig := newStubRig(t, false, 0)
	rig.Tick(4)

	if got := rig.RT.CurrentSlug(); got != shellStubSlug {
		t.Fatalf("with standby disabled boot must load the shell, got %q", got)
	}
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("standby disabled should never leave active, state is %v", got)
	}
}

// lingeringBoot never exits, so orientation can be observed while boot is the loaded app.
type lingeringBoot struct{}

func (lingeringBoot) Init() error                                 { return nil }
func (lingeringBoot) HandleInput(clock.FrameTime)                 {}
func (lingeringBoot) Update(clock.FrameTime)                      {}
func (lingeringBoot) Render(clock.FrameTime, engine.RenderReason) {}
func (lingeringBoot) Teardown() error                             { return nil }

// Boot is ACTIVE but nobody's game: it announces the cabinet to the room, so it must not take the seated orientation. A pairing press must not flip it either; the prompt turns with VirtualRotate on top of this base.
func TestBootAppFacesTheRoom(t *testing.T) {
	for _, tc := range []struct {
		name         string
		attractAfter time.Duration
	}{
		{"standby on", time.Minute},
		{"standby disabled", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registerStub(t, engine.Manifest{
				Slug: bootStubSlug, Name: "Boot", Type: engine.TypeBoot, Hidden: true,
				New: func(*engine.AppContext) engine.App { return lingeringBoot{} },
			})
			registerStub(t, engine.Manifest{
				Slug: shellStubSlug, Name: "Shell", Type: engine.TypeShell, Hidden: true,
				New: func(*engine.AppContext) engine.App { return shellStub{} },
			})
			rig := testkit.NewRig(t, testkit.Options{
				AttractAfter:      tc.attractAfter,
				StandbyAfter:      10 * time.Minute,
				RoomOrientation:   display.CounterClockwise,
				SeatedOrientation: display.Clockwise,
			})
			rig.Tick(2)

			if got := rig.RT.CurrentSlug(); got != bootStubSlug {
				t.Fatalf("expected boot still loaded, got %q", got)
			}
			if got := rig.Capture.BaseOrientation(); got != display.CounterClockwise {
				t.Fatalf("the boot splash should face the room, got %v", got)
			}

			rig.Press(input.KeyJoyA)
			rig.Tick(2)
			if got := rig.RT.CurrentSlug(); got != bootStubSlug {
				t.Fatalf("expected boot still loaded after a press, got %q", got)
			}
			if got := rig.Capture.BaseOrientation(); got != display.CounterClockwise {
				t.Fatalf("a pairing press must not turn the splash to the seats, got %v", got)
			}
		})
	}
}
