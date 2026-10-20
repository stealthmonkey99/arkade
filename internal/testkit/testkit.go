// SPDX-License-Identifier: GPL-2.0-or-later

// Package testkit is the golden-frame test harness. It runs the real engine
// against the data-only Capture renderer with a manual clock and a recording
// audio backend, so a test can script input, step frames deterministically
// and assert on the LED matrix pixel for pixel.
//
// Typical use:
//
//	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/clock"})
//	rig.Tick(3)                    // let the intro fade in
//	rig.Press(input.KeyEnter)      // interact with the app
//	rig.Tick(10)
//	rig.Golden("digital-face")     // compare against testdata golden
//
// Set UPDATE_GOLDENS=1 to (re)write golden files.
package testkit

import (
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/audio"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/controller"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/goldens"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/render"
)

// Options configures a test rig.
type Options struct {
	// StartApp is the slug to boot directly into (bypasses boot/shell).
	StartApp string
	Cols     int
	Rows     int
	// FPS defaults to 30.
	FPS int
	// DataDir defaults to t.TempDir() so tests never touch real saves.
	DataDir string
	// RandSeed defaults to 42 for reproducible random behavior.
	RandSeed int64
	// MakeControllers optionally builds a HID controller manager (usually
	// over a fake enumerator) wired to the engine's input handler, for
	// pairing-flow tests. The rig starts and stops it.
	MakeControllers func(h *input.Handler) *controller.Manager
	// AttractAfter and StandbyAfter configure the standby state machine.
	// Both default to 0 (disabled).
	AttractAfter time.Duration
	StandbyAfter time.Duration
	// HardDecayAfter configures the hard-dark decay window; 0 disables it.
	HardDecayAfter time.Duration
	// RoomOrientation and SeatedOrientation are the default "top" edges the
	// engine switches between as the cabinet moves idle to active. Both
	// default to display.Upright.
	RoomOrientation   display.Orientation
	SeatedOrientation display.Orientation
	// FlightDir enables the input flight recorder and names the directory
	// it writes dumps to. Empty leaves the recorder off.
	FlightDir string
	// Logger receives engine log lines (level, message); nil discards them.
	Logger func(level, msg string)
	// Hardware overrides the peripheral suite (default: hardware.Virtual).
	Hardware hardware.Suite
}

// Rig bundles the engine and its observable test doubles.
type Rig struct {
	T        *testing.T
	RT       *engine.Runtime
	Capture  *render.Capture
	Clock    *clock.ManualClock
	Audio    *audio.Recorder
	tickSize int
}

// NewRig builds and initializes an engine for testing.
func NewRig(t *testing.T, opts Options) *Rig {
	t.Helper()
	cap := render.NewCapture(true)
	clk := clock.NewManualClock()
	rec := audio.NewRecorder()
	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = t.TempDir()
	}
	fps := opts.FPS
	if fps == 0 {
		fps = 30
	}
	seed := opts.RandSeed
	if seed == 0 {
		seed = 42
	}
	rt, err := engine.New(engine.Options{
		RandSeed:          seed,
		Renderer:          cap,
		Cols:              opts.Cols,
		Rows:              opts.Rows,
		TargetFPS:         fps,
		Clock:             clk,
		Audio:             rec,
		DataDir:           dataDir,
		StartApp:          opts.StartApp,
		AttractAfter:      opts.AttractAfter,
		StandbyAfter:      opts.StandbyAfter,
		HardDecayAfter:    opts.HardDecayAfter,
		Logger:            opts.Logger,
		RoomOrientation:   opts.RoomOrientation,
		SeatedOrientation: opts.SeatedOrientation,
		FlightDir:         opts.FlightDir,
		Hardware:          opts.Hardware,
		Controllers:       opts.MakeControllers,
	})
	if err != nil {
		t.Fatalf("testkit: engine.New: %v", err)
	}
	if rt.Controllers != nil {
		rt.Controllers.Start()
		t.Cleanup(rt.Controllers.Stop)
	}
	if err := rt.Init(); err != nil {
		t.Fatalf("testkit: engine.Init: %v", err)
	}
	return &Rig{T: t, RT: rt, Capture: cap, Clock: clk, Audio: rec}
}

// Tick advances the engine n frames, moving the clock one frame budget per
// tick, as a perfectly paced real run would.
func (r *Rig) Tick(n int) {
	r.T.Helper()
	for i := 0; i < n; i++ {
		r.Clock.Advance(r.RT.FrameBudget())
		if err := r.RT.Tick(); err != nil {
			r.T.Fatalf("testkit: tick: %v", err)
		}
	}
}

// TickFor advances enough frames to cover the given duration, such as
// "500ms".
func (r *Rig) TickFor(d string) {
	r.T.Helper()
	dur, err := time.ParseDuration(d)
	if err != nil {
		r.T.Fatalf("testkit: bad duration %q: %v", d, err)
	}
	frames := int(dur/r.RT.FrameBudget()) + 1
	r.Tick(frames)
}

// Press injects a keypress from the player-1 virtual controller.
func (r *Rig) Press(name input.KeyName) {
	r.PressAs(name, 0)
}

// PressAs injects a keypress from the controller in attach slot playerIndex,
// seated at the matching seat (0 is P1, 1 is P2, anything else is seatless).
func (r *Rig) PressAs(name input.KeyName, playerIndex int) {
	r.RT.Input.Push(input.Event{
		Kind: input.KindKeypress,
		Name: name,
		Source: input.PlayerSource{
			Type: input.SourceController,
			Slot: input.SlotOf(playerIndex),
			Seat: input.SeatOf(playerIndex),
		},
	})
}

// PressKeyboard injects a plain keyboard keypress (source type keyboard).
func (r *Rig) PressKeyboard(name input.KeyName, shift bool) {
	r.RT.Input.Push(input.Event{
		Kind:   input.KindKeypress,
		Name:   name,
		Shift:  shift,
		Source: input.PlayerSource{Type: input.SourceKeyboard},
	})
}

// Motion injects a presence-only signal, as the PIR sensor firing with no
// direct input would.
func (r *Rig) Motion() { r.RT.InjectMotion() }

// Frame returns the most recent captured frame, failing if none exists.
func (r *Rig) Frame() display.Matrix {
	r.T.Helper()
	f := r.Capture.LastFrame()
	if f == nil {
		r.T.Fatal("testkit: no frame captured yet")
	}
	return f.Data
}

// PixelAt returns the pixel at x,y of the latest frame.
func (r *Rig) PixelAt(x, y int) display.Pixel {
	r.T.Helper()
	frame := r.Frame()
	if y >= len(frame) || x >= len(frame[y]) {
		r.T.Fatalf("testkit: pixel (%d,%d) out of bounds", x, y)
	}
	return frame[y][x]
}

// ASCII renders a frame as ASCII art plus a legend for non-palette colors.
func ASCII(frame display.Matrix) string { return render.ASCII(frame) }

// Golden compares the latest frame with testdata/golden/<name>.txt.
// UPDATE_GOLDENS=1 rewrites the file instead.
func (r *Rig) Golden(name string) {
	r.T.Helper()
	GoldenFrame(r.T, name, r.Frame())
}

// GoldenFrame compares any frame with testdata/golden/<name>.txt;
// UPDATE_GOLDENS=1 rewrites the file instead.
func GoldenFrame(t *testing.T, name string, frame display.Matrix) {
	t.Helper()
	goldens.Frame(t, name, frame)
}
