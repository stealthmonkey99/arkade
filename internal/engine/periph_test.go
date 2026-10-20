// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// lampApp lights its own buttons every tick, standing in for a game with a button-hint light, so tests can show a show holds those writes back and the app gets its lamps back afterwards.
type lampApp struct {
	ctx   *engine.AppContext
	ticks int
	// quiet stops the per-tick rewrite for the test that changes the app's lamps once mid-show.
	quiet bool
}

const lampAppSlug = "games/lampapp"

// lampAppMask is what the app keeps lit: A on seat 1, D on seat 2, so a
// test can tell the app's state from a show's by looking at either seat.
var lampAppSeats = [2]hardware.LightMask{1 << 0, 1 << 3}

func (a *lampApp) Init() error {
	a.ctx.RT.Display.AddLayer(lampAppSlug, "bg", display.LayerOptions{Fill: &display.Blue})
	a.writeLamps()
	return nil
}
func (a *lampApp) HandleInput(clock.FrameTime) {}
func (a *lampApp) Update(clock.FrameTime) {
	a.ticks++
	if !a.quiet {
		a.writeLamps()
	}
}
func (a *lampApp) writeLamps() {
	for p, mask := range lampAppSeats {
		a.ctx.RT.Hardware.SetButtonLights(p, mask.ButtonLights(), true)
	}
}
func (a *lampApp) Render(clock.FrameTime, engine.RenderReason) {}
func (a *lampApp) Teardown() error                             { return nil }

var liveLampApp *lampApp

func init() {
	engine.Register(engine.Manifest{
		Slug:   lampAppSlug,
		Name:   "Lamp App",
		Type:   engine.TypeGame,
		Hidden: true,
		New: func(ctx *engine.AppContext) engine.App {
			liveLampApp = &lampApp{ctx: ctx}
			return liveLampApp
		},
	})
}

func newPeriphRig(t *testing.T) *testkit.Rig {
	t.Helper()
	return testkit.NewRig(t, testkit.Options{StartApp: lampAppSlug})
}

// virtualHW is the rig's raw suite; reading the device side catches a hold that fails to reach the pins.
func virtualHW(t *testing.T, rig *testkit.Rig) *hardware.Virtual {
	t.Helper()
	hw, ok := rig.RT.RawHardware().(*hardware.Virtual)
	if !ok {
		t.Fatalf("RawHardware is %T, want *hardware.Virtual", rig.RT.RawHardware())
	}
	return hw
}

func litMask(hw *hardware.Virtual, seat int) hardware.LightMask {
	states := make([]bool, len(hardware.LightButtons))
	for i, name := range hardware.LightButtons {
		states[i] = hw.ButtonLit(seat, name)
	}
	return hardware.LightMaskOf(states)
}

func seats(p1, p2 hardware.LightMask) [2]hardware.LightMask {
	return [2]hardware.LightMask{p1, p2}
}

func TestPeriphObservationCostsNothingUnwatched(t *testing.T) {
	rig := newPeriphRig(t)
	rig.Tick(2)
	rig.Press(input.KeyJoyA)
	rig.Tick(1)

	if rig.RT.PeriphWatched() {
		t.Error("nothing arms the observation by default")
	}
	if snap := rig.RT.PeriphSnapshot(); snap.Watching {
		t.Error("no snapshot should be published while unwatched")
	}
	events, _, _ := rig.RT.DrainPeriphInputs(0)
	if len(events) != 0 {
		t.Errorf("recorded %d input events while unwatched, want 0", len(events))
	}
}

func TestPeriphObservesPostRemapInput(t *testing.T) {
	rig := newPeriphRig(t)
	rig.Tick(2)
	release := rig.RT.WatchPeriph()
	defer release()

	rig.PressAs(input.KeyJoyA, 1)
	rig.Tick(1)

	events, next, missed := rig.RT.DrainPeriphInputs(0)
	if missed != 0 {
		t.Errorf("missed = %d, want 0", missed)
	}
	if len(events) != 1 {
		t.Fatalf("observed %d events, want 1: %+v", len(events), events)
	}
	ev := events[0]
	if ev.Kind != string(input.KindKeypress) || ev.Name != string(input.KeyJoyA) {
		t.Errorf("event = %s/%s, want keypress/joya", ev.Kind, ev.Name)
	}
	if ev.Source != string(input.SourceController) || ev.Slot != input.SlotOf(1) || ev.Seat != input.SeatP2 {
		t.Errorf("source = %+v, want the player-2 controller", ev)
	}
	if ev.Frame == 0 || ev.AtMs <= 0 {
		t.Errorf("event carries no frame/time stamp: frame=%d atMs=%v", ev.Frame, ev.AtMs)
	}
	if next != 1 {
		t.Errorf("next cursor = %d, want 1", next)
	}

	// Draining again from the returned cursor yields nothing; the cursor is the contract, not a clear.
	again, _, _ := rig.RT.DrainPeriphInputs(next)
	if len(again) != 0 {
		t.Errorf("re-drain from the cursor returned %d events, want 0", len(again))
	}
}

func TestPeriphRingReportsWhatItOverwrote(t *testing.T) {
	rig := newPeriphRig(t)
	rig.Tick(2)
	release := rig.RT.WatchPeriph()
	defer release()

	// Overfill the ring by half again, in batches the input handler's own
	// 256-event channel can carry.
	const want = engine.PeriphInputRingEvents + engine.PeriphInputRingEvents/2
	for sent := 0; sent < want; {
		for i := 0; i < 128 && sent < want; i++ {
			rig.Press(input.KeyJoyA)
			sent++
		}
		rig.Tick(1)
	}

	events, _, missed := rig.RT.DrainPeriphInputs(0)
	if len(events) != engine.PeriphInputRingEvents {
		t.Errorf("retained %d events, want the ring's %d", len(events), engine.PeriphInputRingEvents)
	}
	if missed == 0 {
		t.Error("a reader starting at 0 after an overflow must be told events were lost")
	}
	if snap := rig.RT.PeriphSnapshot(); snap.Dropped == 0 {
		t.Error("the snapshot should carry the lifetime drop count")
	}
}

func TestPeriphSnapshotCarriesLampsAndMic(t *testing.T) {
	rig := newPeriphRig(t)
	rig.Tick(2)
	release := rig.RT.WatchPeriph()
	defer release()
	rig.Tick(1)

	snap := rig.RT.PeriphSnapshot()
	if !snap.Watching {
		t.Fatal("snapshot should be published once armed")
	}
	if snap.Lights != lampAppSeats {
		t.Errorf("snapshot lamps = %v/%v, want the app's %v/%v",
			snap.Lights[0], snap.Lights[1], lampAppSeats[0], lampAppSeats[1])
	}
	if snap.LightShowActive {
		t.Error("no choreography is playing")
	}
	// The virtual suite ships the synthetic microphone and must announce itself as synthetic.
	if !snap.MicInfo.Available || !snap.MicInfo.Synthetic {
		t.Errorf("micInfo = %+v, want an available synthetic source", snap.MicInfo)
	}
	if !snap.MicOK {
		t.Error("the synthetic source should always produce a level frame")
	}
}

func TestPeriphWatchIsRefCounted(t *testing.T) {
	rig := newPeriphRig(t)
	rig.Tick(2)

	a := rig.RT.WatchPeriph()
	b := rig.RT.WatchPeriph()
	rig.Tick(1)
	if !rig.RT.PeriphWatched() {
		t.Fatal("two watchers should arm the observation")
	}
	a()
	a() // releasing twice must not disarm the other watcher
	rig.Tick(1)
	if !rig.RT.PeriphWatched() {
		t.Fatal("one watcher remains; observation must stay armed")
	}
	if !rig.RT.PeriphSnapshot().Watching {
		t.Error("snapshots should still be published for the remaining watcher")
	}
	b()
	if rig.RT.PeriphWatched() {
		t.Error("the last release should disarm the observation")
	}
	if rig.RT.PeriphSnapshot().Watching {
		t.Error("the published snapshot should be dropped when the last watcher leaves")
	}
}

func TestLightShowOverlaysAndRestores(t *testing.T) {
	rig := newPeriphRig(t)
	rig.Tick(2)
	hw := virtualHW(t, rig)

	if got := litMask(hw, 0); got != lampAppSeats[0] {
		t.Fatalf("before the show: seat 0 lamps = %v, want the app's %v", got, lampAppSeats[0])
	}

	show := seats(hardware.AllLights, 0)
	rig.RT.RequestLightShow(engine.LightShow{
		Steps:   []engine.LightStep{{Seats: show}},
		Timeout: time.Second,
	})
	rig.Tick(1)

	if !rig.RT.LightShowActive() {
		t.Fatal("the show should be active after its request is drained")
	}
	if !rig.RT.Status().LightShowActive {
		t.Error("Status().LightShowActive should be true during a show")
	}
	if got := litMask(hw, 0); got != show[0] {
		t.Errorf("during the show: seat 0 lamps = %v, want %v", got, show[0])
	}
	if got := litMask(hw, 1); got != show[1] {
		t.Errorf("during the show: seat 1 lamps = %v, want %v (dark)", got, show[1])
	}

	// The app underneath kept running and writing its lamps, unaware of the show.
	before := liveLampApp.ticks
	rig.Tick(5)
	if liveLampApp.ticks <= before {
		t.Error("the app stopped updating under a light show")
	}
	if got := litMask(hw, 0); got != show[0] {
		t.Errorf("the app's writes reached the pins through the hold: seat 0 = %v", got)
	}

	rig.TickFor("1100ms")
	if rig.RT.LightShowActive() {
		t.Fatal("the show should have expired")
	}
	if got := litMask(hw, 0); got != lampAppSeats[0] {
		t.Errorf("after the show: seat 0 lamps = %v, want the app's %v back", got, lampAppSeats[0])
	}
	if got := litMask(hw, 1); got != lampAppSeats[1] {
		t.Errorf("after the show: seat 1 lamps = %v, want the app's %v back", got, lampAppSeats[1])
	}
}

func TestLightShowRestoresWhatTheAppWroteDuringIt(t *testing.T) {
	rig := newPeriphRig(t)
	rig.Tick(2)
	hw := virtualHW(t, rig)

	rig.RT.RequestLightShow(engine.LightShow{
		Steps:   []engine.LightStep{{Seats: seats(hardware.AllLights, hardware.AllLights)}},
		Timeout: 500 * time.Millisecond,
	})
	rig.Tick(1)

	// A different app state, written WHILE the show owns the lamps. The
	// mirror must take it even though the pins do not.
	liveLampApp.quiet = true
	changed := hardware.LightMask(1 << 2) // joyc
	rig.RT.Hardware.SetButtonLights(0, changed.ButtonLights(), true)
	if got := litMask(hw, 0); got != hardware.AllLights {
		t.Fatalf("the app's mid-show write reached the pins: seat 0 = %v", got)
	}

	rig.RT.CancelLightShow()
	rig.Tick(1)
	if got := litMask(hw, 0); got != changed {
		t.Errorf("after the show: seat 0 lamps = %v, want the mid-show write %v", got, changed)
	}
}

func TestLightShowStepsAndLoops(t *testing.T) {
	rig := newPeriphRig(t)
	rig.Tick(2)
	hw := virtualHW(t, rig)

	a := hardware.LightMask(1 << 0)
	b := hardware.LightMask(1 << 1)
	rig.RT.RequestLightShow(engine.LightShow{
		Steps: []engine.LightStep{
			{Seats: seats(a, 0), Hold: 100 * time.Millisecond},
			{Seats: seats(b, 0), Hold: 100 * time.Millisecond},
		},
		Loop:    true,
		Timeout: 2 * time.Second,
	})
	rig.Tick(1)
	if got := litMask(hw, 0); got != a {
		t.Fatalf("step 0: seat 0 = %v, want %v", got, a)
	}
	rig.TickFor("120ms")
	if got := litMask(hw, 0); got != b {
		t.Fatalf("step 1: seat 0 = %v, want %v", got, b)
	}
	rig.TickFor("120ms")
	if got := litMask(hw, 0); got != a {
		t.Fatalf("wrapped step: seat 0 = %v, want %v (looped)", got, a)
	}
	if snap := rig.RT.PeriphSnapshot(); snap.Watching {
		t.Error("a light show must not arm the observation by itself")
	}
}

func TestLightShowNonLoopingHoldsLastStep(t *testing.T) {
	rig := newPeriphRig(t)
	rig.Tick(2)
	hw := virtualHW(t, rig)

	last := hardware.AllLights
	rig.RT.RequestLightShow(engine.LightShow{
		Steps: []engine.LightStep{
			{Seats: seats(1<<0, 0), Hold: 50 * time.Millisecond},
			{Seats: seats(last, last), Hold: 50 * time.Millisecond},
		},
		Timeout: time.Second,
	})
	rig.Tick(1)
	rig.TickFor("400ms")
	if got := litMask(hw, 0); got != last {
		t.Errorf("a non-looping show should hold its final step: seat 0 = %v, want %v", got, last)
	}
	if !rig.RT.LightShowActive() {
		t.Error("holding the last step is still an active show")
	}
}

func TestLightShowReplacementDoesNotLeakTheHold(t *testing.T) {
	rig := newPeriphRig(t)
	rig.Tick(2)
	hw := virtualHW(t, rig)

	for i := 0; i < 3; i++ {
		rig.RT.RequestLightShow(engine.LightShow{
			Steps:   []engine.LightStep{{Seats: seats(hardware.AllLights, 0)}},
			Timeout: 300 * time.Millisecond,
		})
		rig.Tick(1)
	}
	rig.TickFor("400ms")
	if rig.RT.LightShowActive() {
		t.Fatal("the last show should have expired")
	}
	if got := litMask(hw, 0); got != lampAppSeats[0] {
		t.Errorf("three stacked shows leaked a hold: seat 0 = %v, want %v", got, lampAppSeats[0])
	}
}

func TestLightShowTimeoutIsClamped(t *testing.T) {
	cases := []struct {
		name string
		in   time.Duration
		want time.Duration
	}{
		{"zero takes the default", 0, engine.DefaultLightShowTimeout},
		{"below the floor", time.Millisecond, engine.MinLightShowTimeout},
		{"above the ceiling", time.Hour, engine.MaxLightShowTimeout},
		{"inside the bounds is untouched", 5 * time.Second, 5 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := engine.ClampLightShowTimeout(tc.in); got != tc.want {
				t.Errorf("ClampLightShowTimeout(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestLightShowEmptyRequestIsIgnored(t *testing.T) {
	rig := newPeriphRig(t)
	rig.Tick(2)
	hw := virtualHW(t, rig)

	rig.RT.RequestLightShow(engine.LightShow{})
	rig.Tick(1)
	if rig.RT.LightShowActive() {
		t.Error("a stepless choreography should not install a show")
	}
	if got := litMask(hw, 0); got != lampAppSeats[0] {
		t.Errorf("an ignored request touched the lamps: seat 0 = %v", got)
	}
}

func TestPeriphInputWireNamesSeatAndSlot(t *testing.T) {
	cases := []struct {
		ev   engine.PeriphInput
		want []string
	}{
		{engine.PeriphInput{Source: "controller", Slot: input.SlotOf(1), Seat: input.SeatP2}, []string{`"slot":1`, `"seat":"P2"`}},
		{engine.PeriphInput{Source: "controller", Slot: input.SlotOf(0), Seat: input.NoSeat}, []string{`"slot":0`, `"seat":null`}},
		{engine.PeriphInput{Source: "keyboard"}, []string{`"slot":null`, `"seat":null`}},
	}
	for _, tc := range cases {
		b, err := json.Marshal(tc.ev)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range tc.want {
			if !strings.Contains(string(b), w) {
				t.Errorf("%s lacks %s", b, w)
			}
		}
		var back engine.PeriphInput
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatalf("decoding %s: %v", b, err)
		}
		if back != tc.ev {
			t.Errorf("round trip of %s = %+v, want %+v", b, back, tc.ev)
		}
	}
	var bad engine.PeriphInput
	if err := json.Unmarshal([]byte(`{"seat":"P3"}`), &bad); err == nil {
		t.Error("an unknown seat name decoded without error")
	}
}
