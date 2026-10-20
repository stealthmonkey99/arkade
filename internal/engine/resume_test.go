// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

import (
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// resumeSpy writes its HUD every tick and records each Resumed call with
// what the LCD showed at that moment, so the test can pin the order: the
// engine restores the peripherals, then tells the app.
type resumeSpy struct {
	ctx     *engine.AppContext
	resumed int
	lcdAt   [2]string
	updates int
	// updatesAtResume is the Update count when Resumed ran, so the test can
	// place it before the wake tick's own Update.
	updatesAtResume int
}

const resumeSpySlug = "test/resume-spy"

var liveResumeSpy *resumeSpy

func (a *resumeSpy) Init() error {
	a.ctx.SetText(input.BothSeats, "SPY", "HUD")
	return nil
}
func (a *resumeSpy) HandleInput(clock.FrameTime) {}
func (a *resumeSpy) Update(clock.FrameTime) {
	a.updates++
	a.ctx.SetText(input.BothSeats, "SPY", "HUD")
}
func (a *resumeSpy) Render(clock.FrameTime, engine.RenderReason) {}
func (a *resumeSpy) Teardown() error                             { return nil }
func (a *resumeSpy) Resumed() {
	a.resumed++
	a.updatesAtResume = a.updates
	v := a.ctx.RT.RawHardware().(*hardware.Virtual)
	a.lcdAt[0], a.lcdAt[1] = v.Lines(0)
}

func init() {
	engine.Register(engine.Manifest{
		Slug:   resumeSpySlug,
		Name:   "Resume Spy",
		Type:   engine.TypeGame,
		Hidden: true,
		New: func(ctx *engine.AppContext) engine.App {
			liveResumeSpy = &resumeSpy{ctx: ctx}
			return liveResumeSpy
		},
	})
}

func TestResumedFiresOnTheWakeAfterThePeripheralsComeBack(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     resumeSpySlug,
		AttractAfter: 100 * time.Millisecond,
		StandbyAfter: time.Minute,
	})
	spy := liveResumeSpy
	rig.Tick(2)
	if spy.resumed != 0 {
		t.Fatal("Resumed fired on a plain load")
	}

	rig.TickFor("150ms")
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("StandbyState = %v, want attract before the wake", got)
	}
	// Attract owns the LCDs meanwhile.
	rig.RT.Hardware.SetText(-1, "ATTRACT", "REEL")

	updatesBefore := spy.updates
	rig.Press("joya")
	rig.Tick(1)
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("StandbyState = %v after the press, want active", got)
	}
	if spy.resumed != 1 {
		t.Fatalf("Resumed fired %d times across one wake, want 1", spy.resumed)
	}
	if spy.lcdAt != [2]string{"SPY", "HUD"} {
		t.Errorf("LCD showed %q when Resumed ran, want the app's HUD already restored", spy.lcdAt)
	}
	if spy.updatesAtResume != updatesBefore || spy.updates != updatesBefore+1 {
		t.Errorf("Resumed ran at update %d of %d; want before the wake tick's Update (%d)", spy.updatesAtResume, spy.updates, updatesBefore)
	}
	rig.Tick(1)
	if spy.resumed != 1 {
		t.Error("Resumed fired again on an ordinary tick")
	}
}

func TestAppContextTextWritesAreDeduped(t *testing.T) {
	rec := &countingSuite{Virtual: hardware.NewVirtual()}
	rig := testkit.NewRig(t, testkit.Options{StartApp: resumeSpySlug, Hardware: rec})
	rig.Tick(10)
	if rec.textWrites != 1 {
		t.Errorf("eleven identical HUD writes reached the device %d times, want 1", rec.textWrites)
	}
}

// countingSuite is a Virtual that counts the text writes reaching it.
type countingSuite struct {
	*hardware.Virtual
	textWrites int
}

func (c *countingSuite) SetText(index int, line1, line2 string) {
	c.textWrites++
	c.Virtual.SetText(index, line1, line2)
}
