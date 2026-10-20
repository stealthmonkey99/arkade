// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
)

type lampCall struct {
	seat int
	mask int
	only bool
}

type levelCall struct {
	seat   int
	levels [6]uint8
	touch  uint8
	fadeMs int
}

// stubLightsHost deliberately has no clock: playback runs on dt_micros alone.
type stubLightsHost struct {
	assets map[string][]byte
	calls  []lampCall
	levels []levelCall
	logs   []string
}

func newStubLightsHost() *stubLightsHost {
	return &stubLightsHost{assets: map[string][]byte{}}
}

func (h *stubLightsHost) BundleAsset(p string) ([]byte, bool) {
	b, ok := h.assets[p]
	return b, ok
}

func (h *stubLightsHost) SetButtonLights(seats input.Seats, lights map[string]bool, only bool) {
	mask := 0
	for i, name := range buttonOrdinal {
		if lights[name] {
			mask |= 1 << uint(i)
		}
	}
	h.calls = append(h.calls, lampCall{seat: seatIndex(seats), mask: mask, only: only})
}

func (h *stubLightsHost) SetLampLevels(seats input.Seats, levels [6]uint8, touch uint8, fadeMs int) {
	h.levels = append(h.levels, levelCall{seat: seatIndex(seats), levels: levels, touch: touch, fadeMs: fadeMs})
}

func (h *stubLightsHost) Log(level LogLevel, msg string) { h.logs = append(h.logs, msg) }

func (h *stubLightsHost) logged(want string) bool {
	for _, l := range h.logs {
		if strings.Contains(l, want) {
			return true
		}
	}
	return false
}

func (h *stubLightsHost) SetLights(input.Seats, bool) {}

func seatIndex(seats input.Seats) int {
	i, _ := seats.Index()
	return i
}

var _ lampHost = (*stubLightsHost)(nil)

func lightsFile(t *testing.T, name string, loop string, frames ...assetfile.LightFrame) []byte {
	t.Helper()
	env := &assetfile.Envelope{
		Format:    assetfile.FormatVersion,
		Kind:      assetfile.KindLights,
		Name:      name,
		Generator: assetfile.Generator{Tool: "lights_test", Version: "1.0.0"},
		LightsPayload: assetfile.LightsPayload{
			Frames: frames,
			Loop:   loop,
		},
	}
	out, err := assetfile.WriteBytes(name+".arkadelights.json", env, nil)
	if err != nil {
		t.Fatalf("WriteBytes(%s) = %v", name, err)
	}
	return out
}

func p1Frame(mask, hold int) assetfile.LightFrame {
	return assetfile.LightFrame{Lamps: assetfile.NewLightLamps(assetfile.LightMaskPtr(mask), nil), HoldMs: hold}
}

// assetfile may not import internal/*, so its copy is checked from here.
func TestLightButtonNamesMatchHardware(t *testing.T) {
	if len(assetfile.LightButtonNames) != len(hardware.LightButtons) {
		t.Fatalf("assetfile.LightButtonNames has %d entries, hardware.LightButtons has %d",
			len(assetfile.LightButtonNames), len(hardware.LightButtons))
	}
	for i := range hardware.LightButtons {
		if assetfile.LightButtonNames[i] != hardware.LightButtons[i] {
			t.Errorf("bit %d: assetfile says %q, hardware says %q", i, assetfile.LightButtonNames[i], hardware.LightButtons[i])
		}
	}
	if assetfile.MaxLightMask != int(hardware.AllLights) {
		t.Errorf("assetfile.MaxLightMask = %d, hardware.AllLights = %d", assetfile.MaxLightMask, hardware.AllLights)
	}
}

func TestLampConstantsMatchHardware(t *testing.T) {
	if lampLevelMax != hardware.LampLevelABIMax {
		t.Errorf("lampLevelMax = %d, hardware.LampLevelABIMax = %d", lampLevelMax, hardware.LampLevelABIMax)
	}
	if lampTouchAll != int(hardware.AllLights) {
		t.Errorf("lampTouchAll = %#x, hardware.AllLights = %#x", lampTouchAll, int(hardware.AllLights))
	}
}

func TestUnpackLampLevelsIgnoresTheTopByte(t *testing.T) {
	if got, want := unpackLampLevels(0xff654321), ([6]uint8{1, 2, 3, 4, 5, 6}); got != want {
		t.Errorf("unpackLampLevels(0xff654321) = %v, want %v", got, want)
	}
	if got, want := unpackLampLevels(0), ([6]uint8{}); got != want {
		t.Errorf("unpackLampLevels(0) = %v, want all dark", got)
	}
	if got, want := lampLevelsOf(1<<0|1<<5), ([6]uint8{15, 0, 0, 0, 0, 15}); got != want {
		t.Errorf("lampLevelsOf(0x21) = %v, want %v", got, want)
	}
}

func TestLightsFadingFrameWritesLevels(t *testing.T) {
	host := newStubLightsHost()
	cut := p1Frame(1, 100)
	fade := p1Frame(1<<3, 100)
	fade.FadeMs = 120
	host.assets["a.arkadelights.json"] = lightsFile(t, "a", assetfile.LoopOnce, cut, fade)
	reg := newLightsRegistry(host)
	reg.loadBundle([]string{"a.arkadelights.json"})
	reg.play("a.arkadelights.json", 0, LightsLoopFile)

	if len(host.calls) != 1 || host.calls[0] != (lampCall{seat: 0, mask: 1, only: true}) {
		t.Fatalf("the cut frame wrote %v, want one SetButtonLights(seat 0, joya, only)", host.calls)
	}
	if len(host.levels) != 0 {
		t.Fatalf("the cut frame also wrote levels: %v", host.levels)
	}

	reg.advance(150_000)
	if len(host.calls) != 1 {
		t.Fatalf("the fading frame cut the lamps as well: %v", host.calls[1:])
	}
	want := levelCall{seat: 0, levels: [6]uint8{0, 0, 0, 15, 0, 0}, touch: lampTouchAll, fadeMs: 120}
	if len(host.levels) != 1 || host.levels[0] != want {
		t.Fatalf("the fading frame wrote %v, want %v", host.levels, want)
	}
}

func TestLightsPlaybackIsAFunctionOfTheTickDeltas(t *testing.T) {
	host := newStubLightsHost()
	host.assets["assets/lights/pulse.arkadelights.json"] = lightsFile(t, "pulse", assetfile.LoopOnce,
		p1Frame(assetfile.MaxLightMask, 100),
		p1Frame(0, 100),
		p1Frame(1<<3, 100),
	)
	reg := newLightsRegistry(host)
	reg.loadBundle([]string{"assets/lights/pulse.arkadelights.json"})

	if got := reg.play("assets/lights/pulse.arkadelights.json", -1, LightsLoopFile); got != 1 {
		t.Fatalf("play() = %d, want 1", got)
	}
	// The play call itself writes frame 0.
	if len(host.calls) != 1 || host.calls[0] != (lampCall{seat: 0, mask: assetfile.MaxLightMask, only: true}) {
		t.Fatalf("after play, calls = %v, want one all-lamps write to seat 0", host.calls)
	}

	// 990 ms, past the 300 ms sequence: three frames, three calls in total.
	for i := 0; i < 30; i++ {
		reg.advance(33_000)
	}
	want := []lampCall{
		{seat: 0, mask: assetfile.MaxLightMask, only: true},
		{seat: 0, mask: 0, only: true},
		{seat: 0, mask: 1 << 3, only: true},
	}
	if len(host.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", host.calls, want)
	}
	for i := range want {
		if host.calls[i] != want[i] {
			t.Fatalf("call %d = %v, want %v", i, host.calls[i], want[i])
		}
	}
	if reg.playing[0] != nil {
		t.Error("a `once` sequence is still playing after its last frame")
	}
}

func TestLightsCycleWraps(t *testing.T) {
	host := newStubLightsHost()
	host.assets["a.arkadelights.json"] = lightsFile(t, "a", assetfile.LoopCycle, p1Frame(1, 100), p1Frame(2, 100))
	reg := newLightsRegistry(host)
	reg.loadBundle([]string{"a.arkadelights.json"})
	reg.play("a.arkadelights.json", 0, LightsLoopFile)

	// 250 ms is one period plus 50, back on frame 0.
	reg.advance(250_000)
	if got := host.calls[len(host.calls)-1].mask; got != 1 {
		t.Fatalf("at 250 ms the mask is %d, want 1 (a cycle wrapped)", got)
	}
	if reg.playing[0] == nil {
		t.Fatal("a cycling sequence stopped")
	}
}

func TestLightsPingPongBounces(t *testing.T) {
	host := newStubLightsHost()
	host.assets["a.arkadelights.json"] = lightsFile(t, "a", assetfile.LoopPingPong,
		p1Frame(1, 100), p1Frame(2, 100), p1Frame(4, 100))
	reg := newLightsRegistry(host)
	reg.loadBundle([]string{"a.arkadelights.json"})
	reg.play("a.arkadelights.json", 0, LightsLoopFile)

	var masks []int
	masks = append(masks, host.calls[len(host.calls)-1].mask)
	for i := 0; i < 7; i++ {
		reg.advance(100_000)
		masks = append(masks, host.calls[len(host.calls)-1].mask)
	}
	want := []int{1, 2, 4, 2, 1, 2, 4, 2}
	for i := range want {
		if masks[i] != want[i] {
			t.Fatalf("masks = %v, want %v", masks, want)
		}
	}
}

func TestLightsLoopArgumentOverridesTheFile(t *testing.T) {
	host := newStubLightsHost()
	host.assets["a.arkadelights.json"] = lightsFile(t, "a", assetfile.LoopOnce, p1Frame(1, 100), p1Frame(2, 100))
	reg := newLightsRegistry(host)
	reg.loadBundle([]string{"a.arkadelights.json"})

	reg.play("a.arkadelights.json", 0, LightsLoopCycle)
	reg.advance(1_000_000)
	if reg.playing[0] == nil {
		t.Fatal("a `once` file forced to cycle stopped anyway")
	}

	host = newStubLightsHost()
	host.assets["a.arkadelights.json"] = lightsFile(t, "a", assetfile.LoopCycle, p1Frame(1, 100), p1Frame(2, 100))
	reg = newLightsRegistry(host)
	reg.loadBundle([]string{"a.arkadelights.json"})
	reg.play("a.arkadelights.json", 0, LightsLoopOnce)
	reg.advance(1_000_000)
	if reg.playing[0] != nil {
		t.Fatal("a cycling file forced to once kept playing")
	}
}

func TestLightsSeatSelection(t *testing.T) {
	host := newStubLightsHost()
	host.assets["a.arkadelights.json"] = lightsFile(t, "a", assetfile.LoopCycle,
		assetfile.LightFrame{
			Lamps:  assetfile.NewLightLamps(assetfile.LightMaskPtr(1), assetfile.LightMaskPtr(2)),
			HoldMs: 100,
		},
		p1Frame(4, 100),
	)
	reg := newLightsRegistry(host)
	reg.loadBundle([]string{"a.arkadelights.json"})
	reg.play("a.arkadelights.json", -1, LightsLoopFile)
	if len(host.calls) != 2 {
		t.Fatalf("seat -1 wrote %v, want one call per seat", host.calls)
	}
	// Frame 1 names p1 only, so p2's lamps stay where frame 0 left them.
	reg.advance(150_000)
	last := host.calls[len(host.calls)-1]
	if last.seat != 0 || last.mask != 4 {
		t.Fatalf("frame 1 wrote %v, want seat 0 mask 4 and nothing for seat 1", last)
	}

	host2 := newStubLightsHost()
	host2.assets["p1only.arkadelights.json"] = lightsFile(t, "p1only", assetfile.LoopCycle, p1Frame(1, 100))
	reg2 := newLightsRegistry(host2)
	reg2.loadBundle([]string{"p1only.arkadelights.json"})
	if got := reg2.play("p1only.arkadelights.json", 1, LightsLoopFile); got != 0 {
		t.Fatalf("play(seat 1) on a p1-only sequence = %d, want 0", got)
	}
	if !host2.logged("has no frames for the seat(s) asked for") {
		t.Errorf("no warning explaining the refusal; logs = %v", host2.logs)
	}
}

func TestLightsPlayRefusesWhatItShould(t *testing.T) {
	host := newStubLightsHost()
	host.assets["a.arkadelights.json"] = lightsFile(t, "a", assetfile.LoopCycle, p1Frame(1, 100))
	reg := newLightsRegistry(host)
	reg.loadBundle([]string{"a.arkadelights.json"})

	if reg.play("b.arkadelights.json", 0, LightsLoopFile) != 0 {
		t.Error("play() accepted a path the manifest never listed")
	}
	if !host.logged("is not one of this cartridge's assets.lights sequences") {
		t.Errorf("no warning for an unlisted path; logs = %v", host.logs)
	}
	if reg.play("a.arkadelights.json", 2, LightsLoopFile) != 0 {
		t.Error("play() accepted seat 2")
	}
	if reg.play("a.arkadelights.json", 0, LightsLoopMode(9)) != 0 {
		t.Error("play() accepted an unknown loop mode")
	}
}

func TestLightsLoadBundleWarnsAndSkips(t *testing.T) {
	host := newStubLightsHost()
	host.assets["wrongkind.arkadeglyph.json"] = []byte("{}")
	host.assets["broken.arkadelights.json"] = []byte(`{"arkade":1,"kind":"lights","name":"x","generator":{"tool":"t","version":"1"},"frames":[]}`)
	reg := newLightsRegistry(host)
	reg.loadBundle([]string{"missing.arkadelights.json", "wrongkind.arkadeglyph.json", "broken.arkadelights.json"})

	if len(reg.seqs) != 0 {
		t.Fatalf("registered %d sequences, want none", len(reg.seqs))
	}
	for _, want := range []string{
		"no such asset in this cartridge's bundle",
		"not a <name>.arkadelights.json file",
		"has no frames",
	} {
		if !host.logged(want) {
			t.Errorf("no warning mentioning %q; logs = %v", want, host.logs)
		}
	}
}

func TestLightsSequenceCap(t *testing.T) {
	host := newStubLightsHost()
	var paths []string
	for i := 0; i < maxLightSequences+4; i++ {
		p := string(rune('a'+i)) + ".arkadelights.json"
		host.assets[p] = lightsFile(t, "s", assetfile.LoopCycle, p1Frame(1, 100))
		paths = append(paths, p)
	}
	reg := newLightsRegistry(host)
	reg.loadBundle(paths)
	if len(reg.seqs) != maxLightSequences {
		t.Fatalf("registered %d sequences, want the %d cap", len(reg.seqs), maxLightSequences)
	}
	if !host.logged("already holding") {
		t.Errorf("no warning at the cap; logs = %v", host.logs)
	}
}

func TestLightsStopLeavesTheLampsAlone(t *testing.T) {
	host := newStubLightsHost()
	host.assets["a.arkadelights.json"] = lightsFile(t, "a", assetfile.LoopCycle, p1Frame(1, 100), p1Frame(2, 100))
	reg := newLightsRegistry(host)
	reg.loadBundle([]string{"a.arkadelights.json"})
	reg.play("a.arkadelights.json", 0, LightsLoopFile)
	before := len(host.calls)

	reg.stop(input.BothSeats)
	reg.advance(500_000)
	if len(host.calls) != before {
		t.Fatalf("stop() wrote %v; stopping a cue is not a request to go dark", host.calls[before:])
	}
}

func TestLightsTeardownDropsEverything(t *testing.T) {
	host := newStubLightsHost()
	host.assets["a.arkadelights.json"] = lightsFile(t, "a", assetfile.LoopCycle, p1Frame(1, 100))
	reg := newLightsRegistry(host)
	reg.loadBundle([]string{"a.arkadelights.json"})
	reg.play("a.arkadelights.json", 0, LightsLoopFile)
	reg.teardown()

	if reg.playing[0] != nil || len(reg.seqs) != 0 {
		t.Fatal("teardown left playback or sequences behind")
	}
	if reg.play("a.arkadelights.json", 0, LightsLoopFile) != 0 {
		t.Error("a torn-down registry still plays")
	}
}

func TestSeatArgReadsTheABIAddress(t *testing.T) {
	cases := []struct {
		wire int32
		want input.Seats
		ok   bool
	}{
		{-1, input.BothSeats, true},
		{0, input.SeatsOf(input.SeatP1), true},
		{1, input.SeatsOf(input.SeatP2), true},
		{2, input.NoSeats, false},
		{-2, input.NoSeats, false},
	}
	for _, tc := range cases {
		host := newStubLightsHost()
		got, ok := seatArg(host, "seat", tc.wire)
		if got != tc.want || ok != tc.ok {
			t.Errorf("seatArg(%d) = %v, %v; want %v, %v", tc.wire, got, ok, tc.want, tc.ok)
		}
		if !ok && !host.logged("out of range") {
			t.Errorf("seatArg(%d) refused without a warning", tc.wire)
		}
	}
}
