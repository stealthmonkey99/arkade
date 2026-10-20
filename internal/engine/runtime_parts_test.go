// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"errors"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// The Runtime's sub-structs each test without a Runtime; these pin the
// contracts the engine loop leans on.

func TestHaltSignalCarriesTheAbortCause(t *testing.T) {
	var h haltSignal
	if h.requested() || h.cause() != nil {
		t.Fatal("a fresh signal is already raised")
	}
	h.request()
	if !h.requested() || h.cause() != nil {
		t.Error("a plain quit must stop the loop with no error")
	}
	var h2 haltSignal
	want := errors.New("boom")
	h2.abort(want)
	if !h2.requested() || h2.cause() != want {
		t.Error("an abort must stop the loop and surface its error")
	}
}

func TestPanelLevelDimRestoresTheFirstLevel(t *testing.T) {
	var p panelLevel
	p.set(70)
	p.dim(5, 70)
	p.dim(1, 5) // a second dim must not overwrite what to restore
	if p.pct != 1 {
		t.Errorf("pct = %d after dimming to 1, want 1: a screensaver is exempt from the floor", p.pct)
	}
	if r, ok := p.clearDim(); !ok || r != 70 {
		t.Errorf("clearDim = %d, %v; want the level before the first dim", r, ok)
	}
	if _, ok := p.clearDim(); ok {
		t.Error("a second clearDim reported a dim to undo")
	}
	p.set(3)
	if p.pct != MinPanelBrightness {
		t.Errorf("set(3) left pct %d, want the floor %d", p.pct, MinPanelBrightness)
	}
	p.set(140)
	if p.pct != 100 {
		t.Errorf("set(140) left pct %d, want 100", p.pct)
	}
}

func TestPanelLevelBloomScalesTheValueAndHidesTheRamp(t *testing.T) {
	var p panelLevel
	p.set(80)
	p.startBloom(80, 1000, Linear)
	if p.value() != 0 {
		t.Errorf("value at bloom start = %v, want 0", p.value())
	}
	if p.reported(1) != 80 {
		t.Error("during a bloom the intended level must be reported, not the live ramp")
	}
	if !p.stepBloom(500_000) || p.value() != 0.4 {
		t.Errorf("value halfway = %v, want 0.4", p.value())
	}
	p.stepBloom(500_000)
	if p.bloom.active || p.value() != 0.8 {
		t.Errorf("after the bloom: active=%v value=%v, want done at 0.8", p.bloom.active, p.value())
	}
	if p.reported(55) != 55 {
		t.Error("outside a bloom the live level is the truth")
	}
	if p.endBloom() || p.stepBloom(1) {
		t.Error("ending or stepping a finished bloom reported one")
	}
}

func TestOrientationPolicyApplyPushesOnceThenOnChange(t *testing.T) {
	p := orientationPolicy{room: display.CounterClockwise, seated: display.Clockwise}
	if o, changed := p.apply(StateAttract, false); !changed || o != display.CounterClockwise {
		t.Fatalf("first apply = %v, %v; want the room edge and a push", o, changed)
	}
	if _, changed := p.apply(StateAttract, false); changed {
		t.Error("an unchanged orientation was pushed again")
	}
	if o, changed := p.apply(StateActive, false); !changed || o != display.Clockwise {
		t.Errorf("waking = %v, %v; want the seated edge pushed", o, changed)
	}
	var upright orientationPolicy
	if _, changed := upright.apply(StateActive, false); !changed {
		t.Error("a policy that wants Upright must still push it once, whatever the renderer started on")
	}
}

type countingStrategy struct {
	AnyPlayerStrategy
	inits int
}

func (c *countingStrategy) Init() { c.inits++ }

func TestStrategyStackReinitialisesWhateverBecomesTheTop(t *testing.T) {
	var s strategyStack
	base, game := &countingStrategy{}, &countingStrategy{}
	s.push(base)
	s.push(game)
	if base.inits != 1 || game.inits != 1 || s.top() != game {
		t.Fatalf("after two pushes: base %d game %d top %T", base.inits, game.inits, s.top())
	}
	s.pop()
	if base.inits != 2 || s.top() != base {
		t.Errorf("popping the game left base at %d inits, want it re-initialised", base.inits)
	}
	s.pop()
	s.pop() // an empty stack tolerates a pop
	if s.top() != nil {
		t.Error("an empty stack has a top")
	}
}

func TestPeriphObserverRecordsOnlyWhileWatched(t *testing.T) {
	var o periphObserver
	o.note(PeriphInput{Name: "dropped"})
	if ev, _, _ := o.inputs.drain(0); len(ev) != 0 {
		t.Fatal("an unwatched event was recorded")
	}
	release1 := o.watch()
	release2 := o.watch()
	o.note(PeriphInput{Name: string(input.KeyJoyA)})
	o.publish(PeriphSnapshot{Watching: true, Frame: 7})
	release1()
	release1() // idempotent
	if !o.watched() || o.snapshot().Frame != 7 {
		t.Error("one stream closing disarmed the other")
	}
	release2()
	if o.watched() || o.snapshot().Watching {
		t.Error("the last release left the observer armed or its snapshot live")
	}
	if ev, _, _ := o.inputs.drain(0); len(ev) != 1 {
		t.Errorf("drain found %d events, want the one noted while watched", len(ev))
	}
}

func TestAttractHoldTimesOneHold(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	var h attractHold
	if h.active() || h.fraction(epoch) != 0 {
		t.Fatal("nothing is held yet")
	}
	if !h.begin(epoch) || h.begin(epoch.Add(time.Second)) {
		t.Error("begin must start once and refuse while held")
	}
	half := epoch.Add(attractLaunchHold / 2)
	if h.fraction(half) != 0.5 || h.complete(half) {
		t.Errorf("halfway: fraction %v complete %v", h.fraction(half), h.complete(half))
	}
	done := epoch.Add(2 * attractLaunchHold)
	if h.fraction(done) != 1 || !h.complete(done) {
		t.Error("a long hold must read as complete at fraction 1")
	}
	h.clear()
	if h.active() {
		t.Error("clear left the hold active")
	}
}

func TestDarkFadeMeasuresFromArming(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	var d darkFade
	if d.running() {
		t.Fatal("the zero value is running")
	}
	d.arm(epoch)
	if !d.running() || d.elapsedMs(epoch.Add(1500*time.Millisecond)) != 1500 {
		t.Error("elapsed must count from the arming instant")
	}
	d.cancel()
	if d.running() {
		t.Error("cancel left the fade running")
	}
}

func TestBestScreensaverPrefersPriorityThenSlug(t *testing.T) {
	a := Manifest{Slug: "savers/a"}
	b := Manifest{Slug: "savers/b"}
	c := Manifest{Slug: "savers/c"}
	if _, ok := bestScreensaver(nil); ok {
		t.Fatal("no candidates picked something")
	}
	if m, _ := bestScreensaver([]saverCandidate{{a, 1}, {b, 5}, {c, 2}}); m.Slug != b.Slug {
		t.Errorf("picked %s, want the highest priority", m.Slug)
	}
	if m, _ := bestScreensaver([]saverCandidate{{c, 1}, {b, 1}, {a, 1}}); m.Slug != a.Slug {
		t.Errorf("picked %s, want the tie broken by slug", m.Slug)
	}
}

func TestScreensaverPickerCachesSchedulesForTheTTL(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	var p screensaverPicker
	m := Manifest{Slug: "savers/a", DefaultSchedule: Schedule{Enabled: true, From: "01:00", To: "02:00"}}
	loads := 0
	stored := Schedule{}
	load := func(string) (Schedule, error) { loads++; return stored, nil }

	if sc, _ := p.schedule(m, epoch, load); sc != m.DefaultSchedule {
		t.Errorf("a zero stored schedule read as %+v, want the manifest default", sc)
	}
	p.schedule(m, epoch.Add(30*time.Second), load)
	if loads != 1 {
		t.Errorf("the schedule was read %d times inside the TTL, want 1", loads)
	}
	stored = Schedule{Enabled: true, From: "03:00", To: "04:00"}
	if sc, _ := p.schedule(m, epoch.Add(scheduleCacheTTL), load); sc != stored || loads != 2 {
		t.Errorf("after the TTL: %+v after %d loads, want the stored schedule re-read", sc, loads)
	}
	p.invalidate()
	p.schedule(m, epoch.Add(scheduleCacheTTL+time.Second), load)
	if loads != 3 {
		t.Error("invalidate did not force a re-read")
	}
	p.pickedAt = epoch
	if !p.due(epoch.Add(screensaverPickInterval)) || p.due(epoch.Add(screensaverPickInterval-1)) {
		t.Error("due must turn over exactly at the pick interval")
	}
}
