// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// lcdApp paints one solid colour and rewrites its LCD text every tick, standing in for a running game so tests can show a preview neither disturbs it nor is disturbed by it.
type lcdApp struct {
	ctx   *engine.AppContext
	ticks int
}

const (
	lcdAppSlug  = "games/lcdapp"
	lcdAppLine1 = "GAME LINE ONE"
	lcdAppLine2 = "GAME LINE TWO"
)

func (a *lcdApp) Init() error {
	a.ctx.RT.Display.AddLayer(lcdAppSlug, "bg", display.LayerOptions{Fill: &display.Blue})
	a.ctx.RT.Hardware.SetText(-1, lcdAppLine1, lcdAppLine2)
	return nil
}
func (a *lcdApp) HandleInput(clock.FrameTime) {}
func (a *lcdApp) Update(clock.FrameTime) {
	a.ticks++
	// Rewrite every tick, as a real game's HUD does: the preview must hold
	// these back while it owns the panels, not merely win a one-time race.
	a.ctx.RT.Hardware.SetText(-1, lcdAppLine1, lcdAppLine2)
}
func (a *lcdApp) Render(clock.FrameTime, engine.RenderReason) {}
func (a *lcdApp) Teardown() error                             { return nil }

// liveLCDApp is the most recently constructed lcdApp; tests reach its AppContext through it to call app-facing entry points (EnterSleep). Safe because the rig ticks on the test's goroutine.
var liveLCDApp *lcdApp

func init() {
	engine.Register(engine.Manifest{
		Slug:   lcdAppSlug,
		Name:   "LCD App",
		Type:   engine.TypeGame,
		Hidden: true,
		New: func(ctx *engine.AppContext) engine.App {
			liveLCDApp = &lcdApp{ctx: ctx}
			return liveLCDApp
		},
	})
}

func solidFrame(t *testing.T, rt *engine.Runtime, p display.Pixel) display.Matrix {
	t.Helper()
	w, h := rt.Display.Size()
	return display.NewMatrix(w, h, p)
}

func newPreviewRig(t *testing.T, logs *[]string) *testkit.Rig {
	t.Helper()
	return testkit.NewRig(t, testkit.Options{
		StartApp: lcdAppSlug,
		Logger: func(level, msg string) {
			if logs != nil {
				*logs = append(*logs, level+": "+msg)
			}
		},
	})
}

func TestLabPreviewFrameOverlaysAndRestores(t *testing.T) {
	rig := newPreviewRig(t, nil)
	rig.Tick(2)

	if got := rig.PixelAt(0, 0); got != display.Blue {
		t.Fatalf("before preview: pixel = %+v, want the app's blue", got)
	}

	rig.RT.RequestLabPreview(engine.LabPreview{
		Frames:  []engine.LabPreviewFrame{{Data: solidFrame(t, rig.RT, display.Red)}},
		Timeout: time.Second,
	})
	rig.Tick(1)

	if !rig.RT.LabPreviewActive() {
		t.Fatal("preview should be active after the request is drained")
	}
	if got := rig.PixelAt(0, 0); got != display.Red {
		t.Fatalf("during preview: pixel = %+v, want the preview's red", got)
	}
	if !rig.RT.Status().LabPreviewActive {
		t.Error("Status().LabPreviewActive should be true during a preview")
	}

	if slug := rig.RT.CurrentSlug(); slug != lcdAppSlug {
		t.Errorf("current app = %q during preview, want it untouched (%q)", slug, lcdAppSlug)
	}
	if rig.RT.SuspendedSlug() != "" {
		t.Errorf("preview suspended %q; it must not suspend anything", rig.RT.SuspendedSlug())
	}

	rig.TickFor("1100ms")
	if rig.RT.LabPreviewActive() {
		t.Fatal("preview should have expired")
	}
	if got := rig.PixelAt(0, 0); got != display.Blue {
		t.Fatalf("after preview: pixel = %+v, want the app's blue back", got)
	}
}

func TestLabPreviewDoesNotStallTheRunningApp(t *testing.T) {
	rig := newPreviewRig(t, nil)
	rig.Tick(2)

	rig.RT.RequestLabPreview(engine.LabPreview{
		Frames:  []engine.LabPreviewFrame{{Data: solidFrame(t, rig.RT, display.Red)}},
		Timeout: time.Second,
	})
	rig.Tick(1)

	before := rig.RT.Status().Ticks
	rig.Tick(10)
	after := rig.RT.Status().Ticks
	if after-before != 10 {
		t.Errorf("app advanced %d ticks under a preview, want 10 — a preview must not gate the loop", after-before)
	}
}

func TestLabPreviewAnimationAdvancesAndLoops(t *testing.T) {
	rig := newPreviewRig(t, nil)
	rig.Tick(2)

	frames := []engine.LabPreviewFrame{
		{Data: solidFrame(t, rig.RT, display.Red), Delay: 100 * time.Millisecond},
		{Data: solidFrame(t, rig.RT, display.Green), Delay: 100 * time.Millisecond},
	}
	rig.RT.RequestLabPreview(engine.LabPreview{
		Frames: frames, Loop: true, Timeout: 5 * time.Second,
	})
	rig.Tick(1)
	if got := rig.PixelAt(0, 0); got != display.Red {
		t.Fatalf("frame 0: pixel = %+v, want red", got)
	}
	rig.TickFor("120ms")
	if got := rig.PixelAt(0, 0); got != display.Green {
		t.Fatalf("frame 1: pixel = %+v, want green", got)
	}
	rig.TickFor("120ms")
	if got := rig.PixelAt(0, 0); got != display.Red {
		t.Fatalf("wrapped frame: pixel = %+v, want red again (Loop: true)", got)
	}
}

func TestLabPreviewNonLoopingHoldsLastFrame(t *testing.T) {
	rig := newPreviewRig(t, nil)
	rig.Tick(2)

	rig.RT.RequestLabPreview(engine.LabPreview{
		Frames: []engine.LabPreviewFrame{
			{Data: solidFrame(t, rig.RT, display.Red), Delay: 50 * time.Millisecond},
			{Data: solidFrame(t, rig.RT, display.Green), Delay: 50 * time.Millisecond},
		},
		Timeout: 2 * time.Second,
	})
	rig.Tick(1)
	rig.TickFor("500ms")
	if got := rig.PixelAt(0, 0); got != display.Green {
		t.Fatalf("pixel = %+v, want the last frame held (green) with Loop false", got)
	}
}

func TestLabPreviewLCDHoldsAndReplaysAppText(t *testing.T) {
	rig := newPreviewRig(t, nil)
	rig.Tick(2)

	hw, ok := rig.RT.RawHardware().(*hardware.Virtual)
	if !ok {
		t.Fatalf("RawHardware is %T, want *hardware.Virtual", rig.RT.RawHardware())
	}
	if l1, _ := hw.Lines(0); l1 != lcdAppLine1 {
		t.Fatalf("before preview: line1 = %q, want %q", l1, lcdAppLine1)
	}

	rig.RT.RequestLabPreview(engine.LabPreview{
		LCD: &engine.LabPreviewLCD{
			Lines: [2][2]string{{"P1 TOP", "P1 BOTTOM"}, {"P2 TOP", "P2 BOTTOM"}},
		},
		Timeout: time.Second,
	})
	rig.Tick(5) // the app rewrites its own text on every one of these

	for seat, want := range [2][2]string{{"P1 TOP", "P1 BOTTOM"}, {"P2 TOP", "P2 BOTTOM"}} {
		l1, l2 := hw.Lines(seat)
		if l1 != want[0] || l2 != want[1] {
			t.Errorf("seat %d during preview: %q/%q, want %q/%q — the app's writes must be held", seat, l1, l2, want[0], want[1])
		}
	}

	rig.TickFor("1100ms")
	for seat := 0; seat < 2; seat++ {
		l1, l2 := hw.Lines(seat)
		if l1 != lcdAppLine1 || l2 != lcdAppLine2 {
			t.Errorf("seat %d after preview: %q/%q, want the app's %q/%q replayed", seat, l1, l2, lcdAppLine1, lcdAppLine2)
		}
	}
}

func TestLabPreviewGlyphsPagedInAndCleared(t *testing.T) {
	rig := newPreviewRig(t, nil)
	rig.Tick(2)
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	want := hardware.GlyphRows{0x04, 0x0e, 0x1f, 0x0e, 0x04, 0, 0, 0}
	rig.RT.RequestLabPreview(engine.LabPreview{
		LCD: &engine.LabPreviewLCD{
			Lines:  [2][2]string{{"◆", ""}, {"◆", ""}},
			Glyphs: map[rune]hardware.GlyphRows{'◆': want},
		},
		Timeout: time.Second,
	})
	rig.Tick(1)

	got := hw.PreviewGlyphs()
	if len(got) != 1 || got['◆'] != want {
		t.Fatalf("preview glyphs = %v, want one entry for ◆ = %v", got, want)
	}

	rig.TickFor("1100ms")
	if got := hw.PreviewGlyphs(); got != nil {
		t.Errorf("preview glyphs after expiry = %v, want them cleared", got)
	}
}

func TestLabPreviewCancelRestoresImmediately(t *testing.T) {
	rig := newPreviewRig(t, nil)
	rig.Tick(2)

	rig.RT.RequestLabPreview(engine.LabPreview{
		Frames:  []engine.LabPreviewFrame{{Data: solidFrame(t, rig.RT, display.Red)}},
		LCD:     &engine.LabPreviewLCD{Lines: [2][2]string{{"HELD", ""}, {"HELD", ""}}},
		Timeout: time.Minute,
	})
	rig.Tick(1)
	if !rig.RT.LabPreviewActive() {
		t.Fatal("preview should be active")
	}

	rig.RT.CancelLabPreview()
	rig.Tick(1)
	if rig.RT.LabPreviewActive() {
		t.Fatal("cancel should have ended the preview")
	}
	if got := rig.PixelAt(0, 0); got != display.Blue {
		t.Errorf("after cancel: pixel = %+v, want the app's blue back", got)
	}
	hw := rig.RT.RawHardware().(*hardware.Virtual)
	if l1, _ := hw.Lines(0); l1 != lcdAppLine1 {
		t.Errorf("after cancel: line1 = %q, want the app's %q replayed", l1, lcdAppLine1)
	}
}

func TestLabPreviewLatestRequestWins(t *testing.T) {
	rig := newPreviewRig(t, nil)
	rig.Tick(2)

	rig.RT.RequestLabPreview(engine.LabPreview{
		Frames:  []engine.LabPreviewFrame{{Data: solidFrame(t, rig.RT, display.Red)}},
		Timeout: time.Second,
	})
	rig.Tick(1)
	// A second preview that takes only the LCD must hand the PANEL back,
	// not leave the first preview's frame stranded on it.
	rig.RT.RequestLabPreview(engine.LabPreview{
		LCD:     &engine.LabPreviewLCD{Lines: [2][2]string{{"SECOND", ""}, {"SECOND", ""}}},
		Timeout: time.Second,
	})
	rig.Tick(1)

	if got := rig.PixelAt(0, 0); got != display.Blue {
		t.Errorf("pixel = %+v, want the app's blue: the replacing preview owns no frames", got)
	}
	hw := rig.RT.RawHardware().(*hardware.Virtual)
	if l1, _ := hw.Lines(0); l1 != "SECOND" {
		t.Errorf("line1 = %q, want %q", l1, "SECOND")
	}
}

func TestLabPreviewTimeoutClamped(t *testing.T) {
	rig := newPreviewRig(t, nil)
	rig.Tick(2)

	// An hour is well past MaxLabPreviewTimeout: the preview must still end.
	rig.RT.RequestLabPreview(engine.LabPreview{
		Frames:  []engine.LabPreviewFrame{{Data: solidFrame(t, rig.RT, display.Red)}},
		Timeout: time.Hour,
	})
	rig.Tick(1)
	if !rig.RT.LabPreviewActive() {
		t.Fatal("preview should be active")
	}
	rig.Clock.Advance(engine.MaxLabPreviewTimeout + time.Second)
	rig.Tick(1)
	if rig.RT.LabPreviewActive() {
		t.Fatalf("preview outlived MaxLabPreviewTimeout (%s)", engine.MaxLabPreviewTimeout)
	}
}

func TestLabPreviewSoundPlaysAndStops(t *testing.T) {
	rig := newPreviewRig(t, nil)
	rig.Tick(2)

	wav := []byte("RIFF....WAVEfmt ")
	rig.RT.RequestLabPreview(engine.LabPreview{
		Sound:   &engine.LabPreviewSound{WAV: wav, Gain: 0.5},
		Timeout: time.Second,
	})
	rig.Tick(1)

	var played bool
	for _, ev := range rig.Audio.Events() {
		if ev.Action == "play" && ev.Owner == engine.LabPreviewOwner {
			played = true
			if ev.Bytes != len(wav) {
				t.Errorf("played %d bytes, want %d", ev.Bytes, len(wav))
			}
			if ev.Gain != 0.5 {
				t.Errorf("gain = %v, want 0.5", ev.Gain)
			}
		}
	}
	if !played {
		t.Fatalf("no play event for %q: %+v", engine.LabPreviewOwner, rig.Audio.Events())
	}

	rig.TickFor("1100ms")
	var stopped bool
	for _, ev := range rig.Audio.Events() {
		if ev.Action == "stop" && ev.Owner == engine.LabPreviewOwner {
			stopped = true
		}
	}
	if !stopped {
		t.Errorf("preview sound was never stopped at the timeout: %+v", rig.Audio.Events())
	}
}

func TestLabPreviewEmptyRequestIgnored(t *testing.T) {
	var logs []string
	rig := newPreviewRig(t, &logs)
	rig.Tick(2)

	rig.RT.RequestLabPreview(engine.LabPreview{Timeout: time.Second})
	rig.Tick(1)

	if rig.RT.LabPreviewActive() {
		t.Fatal("an empty preview request must not become an active preview")
	}
	if !hasLog(logs, "nothing to show") {
		t.Errorf("expected a warning about an empty request, got: %v", logs)
	}
}

func TestLabPreviewIgnoresStandbyState(t *testing.T) {
	// A preview is not presence: it must not reset the idle clock, and it
	// must not stop the cabinet reaching attract on its own schedule.
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     lcdAppSlug,
		AttractAfter: time.Second,
	})
	rig.Tick(2)

	rig.RT.RequestLabPreview(engine.LabPreview{
		Frames:  []engine.LabPreviewFrame{{Data: solidFrame(t, rig.RT, display.Red)}},
		Timeout: 5 * time.Second,
	})
	rig.Tick(1)

	rig.TickFor("1100ms")
	if state := rig.RT.StandbyState(); state != engine.StateAttract {
		t.Errorf("standby state = %v under a preview, want attract — a preview must not count as presence", state)
	}
	if !rig.RT.LabPreviewActive() {
		t.Error("the preview should have survived the standby transition underneath it")
	}
}

// DARK stops painting (enterDark), so a preview there must drive the panel itself, restore black afterwards, and not wake the cabinet, which would resume the suspended app.
func TestLabPreviewOnDarkCabinet(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     lcdAppSlug,
		AttractAfter: 500 * time.Millisecond,
		StandbyAfter: 500 * time.Millisecond,
	})
	rig.Tick(2)
	// Straight to DARK via the shell's SLEEP path: with no attract app registered, idle decay would stall in ATTRACT.
	liveLCDApp.ctx.EnterSleep()
	rig.Tick(1)
	if state := rig.RT.StandbyState(); state != engine.StateDark {
		t.Fatalf("standby state = %v, want dark", state)
	}
	if got := rig.PixelAt(0, 0); got != display.Black {
		t.Fatalf("dark cabinet: pixel = %+v, want black", got)
	}

	rig.RT.RequestLabPreview(engine.LabPreview{
		Frames:  []engine.LabPreviewFrame{{Data: solidFrame(t, rig.RT, display.Red)}},
		Timeout: time.Second,
	})
	rig.Tick(1)
	if got := rig.PixelAt(0, 0); got != display.Red {
		t.Fatalf("preview on a dark cabinet: pixel = %+v, want red", got)
	}
	if state := rig.RT.StandbyState(); state != engine.StateDark {
		t.Errorf("standby state = %v, want it left dark — a preview is not a wake", state)
	}

	rig.TickFor("1100ms")
	if rig.RT.LabPreviewActive() {
		t.Fatal("preview should have expired")
	}
	if got := rig.PixelAt(0, 0); got != display.Black {
		t.Errorf("after a dark-cabinet preview: pixel = %+v, want black again", got)
	}
}

// The preview is already up when standby drops to DARK: the panel must not blink black (enterDark skips its black frame while a preview owns the panel), and the black frame must land when the preview ends.
func TestLabPreviewSurvivesGoingDarkUnderneath(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     lcdAppSlug,
		AttractAfter: time.Minute,
	})
	rig.Tick(2)

	rig.RT.RequestLabPreview(engine.LabPreview{
		Frames:  []engine.LabPreviewFrame{{Data: solidFrame(t, rig.RT, display.Red)}},
		LCD:     &engine.LabPreviewLCD{Lines: [2][2]string{{"STILL HERE", ""}, {"STILL HERE", ""}}},
		Timeout: time.Second,
	})
	rig.Tick(1)

	liveLCDApp.ctx.EnterSleep()
	rig.Tick(1)
	if state := rig.RT.StandbyState(); state != engine.StateDark {
		t.Fatalf("standby state = %v, want dark", state)
	}
	if got := rig.PixelAt(0, 0); got != display.Red {
		t.Errorf("pixel = %+v after going dark under a preview, want the preview still showing", got)
	}
	hw := rig.RT.RawHardware().(*hardware.Virtual)
	if l1, _ := hw.Lines(0); l1 != "STILL HERE" {
		t.Errorf("line1 = %q, want the preview's text held through the dark transition", l1)
	}

	rig.TickFor("1100ms")
	if rig.RT.LabPreviewActive() {
		t.Fatal("preview should have expired")
	}
	if got := rig.PixelAt(0, 0); got != display.Black {
		t.Errorf("pixel = %+v after the preview ended in dark, want black", got)
	}
}

// An LCD-only preview (no Frames) on a dark cabinet must still trigger a paint so the desk console's hardware header refreshes; observed through Capture's FrameCount.
func TestLabPreviewLCDOnlyOnDarkCabinetRepaints(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     lcdAppSlug,
		AttractAfter: 500 * time.Millisecond,
		StandbyAfter: 500 * time.Millisecond,
	})
	rig.Tick(2)
	// Straight to DARK, as in TestLabPreviewOnDarkCabinet.
	liveLCDApp.ctx.EnterSleep()
	rig.Tick(1)
	if state := rig.RT.StandbyState(); state != engine.StateDark {
		t.Fatalf("standby state = %v, want dark", state)
	}

	before := rig.Capture.FrameCount()
	rig.RT.RequestLabPreview(engine.LabPreview{
		LCD:     &engine.LabPreviewLCD{Lines: [2][2]string{{"PREVIEW", ""}, {"PREVIEW", ""}}},
		Timeout: time.Second,
	})
	rig.Tick(1)

	if !rig.RT.LabPreviewActive() {
		t.Fatal("the LCD-only preview should be active")
	}
	hw := rig.RT.RawHardware().(*hardware.Virtual)
	if l1, _ := hw.Lines(0); l1 != "PREVIEW" {
		t.Fatalf("line1 = %q, want the preview's text on the hardware — the write path itself was never the bug", l1)
	}
	if got := rig.Capture.FrameCount(); got <= before {
		t.Errorf("FrameCount = %d, want > %d: an LCD-only preview on a dark cabinet must still trigger a paint,"+
			" or a Console's hardware header (the desk stand-in for the LCDs) never refreshes to show it", got, before)
	}
	if state := rig.RT.StandbyState(); state != engine.StateDark {
		t.Errorf("standby state = %v, want it left dark — a preview is not a wake", state)
	}
}

func hasLog(logs []string, substr string) bool {
	for _, l := range logs {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}
