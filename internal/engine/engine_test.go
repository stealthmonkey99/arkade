// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// bounceApp is a minimal engine.App: a red square bouncing off the walls.
type bounceApp struct {
	ctx    *engine.AppContext
	rect   *element.Rect
	x, y   int
	dx, dy int
}

func newBounce(ctx *engine.AppContext) engine.App {
	return &bounceApp{ctx: ctx, x: 4, y: 4, dx: 1, dy: 1}
}

func (a *bounceApp) Init() error {
	fill := display.Red
	a.rect = element.NewRect(element.RectOptions{W: 6, H: 6, Fill: &fill, Border: &element.NoBorder})
	a.rect.SetOffsetX(a.x)
	a.rect.SetOffsetY(a.y)
	a.ctx.AddLayer("bounce", []display.Element{a.rect}, 1)
	return nil
}

func (a *bounceApp) HandleInput(clock.FrameTime) {}

func (a *bounceApp) Update(clock.FrameTime) {
	w, h := a.ctx.RT.Display.Size()
	a.x += a.dx
	a.y += a.dy
	if a.x <= 0 || a.x+6 >= w {
		a.dx = -a.dx
	}
	if a.y <= 0 || a.y+6 >= h {
		a.dy = -a.dy
	}
	a.rect.SetOffsetX(a.x)
	a.rect.SetOffsetY(a.y)
}

func (a *bounceApp) Render(clock.FrameTime, engine.RenderReason) {}
func (a *bounceApp) Teardown() error                             { return nil }

func init() {
	engine.Register(engine.Manifest{
		Slug:   "test/bounce",
		Name:   "Bounce",
		Type:   engine.TypeGame,
		Hidden: true,
		New:    newBounce,
	})
}

func TestEngineRunsBounceApp(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/bounce"})

	if got := rig.PixelAt(5, 5); got != display.Red {
		t.Errorf("expected red square at start, got %+v", got)
	}
	rig.Tick(10)
	if got := rig.PixelAt(15, 15); got != display.Red {
		t.Errorf("expected red square after 10 ticks, got %+v", got)
	}
	if got := rig.PixelAt(5, 5); got != display.Black {
		t.Errorf("old position should be black base layer, got %+v", got)
	}
}

func TestEngineGoldenBounce(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/bounce"})
	rig.Tick(10)
	rig.Golden("bounce-tick10")
}

func TestEngineRotationTransformsFrame(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/bounce"})
	rig.Tick(1)
	before := rig.PixelAt(5, 5)
	if before != display.Red {
		t.Fatalf("expected red at (5,5), got %+v", before)
	}
	rig.PressKeyboard(input.KeyRotate, false)
	rig.Tick(1)
	w, _ := rig.RT.Display.Size()
	// Rotated clockwise the square appears mirrored at the right edge: dest(x,y) = src(w-1-x, y).
	if got := rig.PixelAt(w-1-6, 6); got != display.Red {
		t.Errorf("rotation should move the square, got %+v", got)
	}
}

func TestEngineBrightnessDims(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/bounce"})
	rig.Tick(1)
	rig.PressKeyboard(input.KeyBrightness, true)
	rig.PressKeyboard(input.KeyBrightness, true)
	rig.Tick(1)
	px := rig.PixelAt(6, 6)
	if px.R != 204 { // round(255 * 0.8)
		t.Errorf("expected dimmed red 204, got %d", px.R)
	}
}

func TestEngineQuitKeyStopsLoop(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/bounce"})
	rig.Tick(1)
	rig.PressKeyboard("q", false)
	rig.Tick(2)
	// bounce is Hidden but not shell/boot; with no shell registered, unload quits the engine.
	n := rig.Capture.FrameCount()
	rig.Tick(3)
	if rig.Capture.FrameCount() != n {
		t.Error("engine should stop painting after quit")
	}
}

func TestFrameStatsAdvance(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/bounce"})
	rig.Tick(5)
	f := rig.Capture.LastFrame()
	if f.Stats.Ticks != 5 {
		t.Errorf("expected 5 ticks, got %d", f.Stats.Ticks)
	}
	if f.Stats.ElapsedMs < 33 || f.Stats.ElapsedMs > 34 {
		t.Errorf("expected ~33.3ms elapsed at 30fps, got %f", f.Stats.ElapsedMs)
	}
}
