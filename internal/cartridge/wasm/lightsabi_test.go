// SPDX-License-Identifier: GPL-2.0-or-later

package wasm_test

// The browser host has a twin of this file.

import (
	"context"
	_ "embed"
	"encoding/binary"
	"testing"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/carttest"
	"github.com/stealthmonkey99/arkade/internal/cartridge/wasm"
)

//go:embed testdata/lights.wasm
var lightsWasm []byte

const lightsFixturePath = "assets/lights/win.arkadelights.json"

// winSequence is four 100 ms frames on p1 (all, dark, all, D alone), with
// frame 0 also naming p2.
func winSequence(t *testing.T) []byte {
	t.Helper()
	all := assetfile.LightMaskPtr(assetfile.MaxLightMask)
	dark := assetfile.LightMaskPtr(0)
	env := &assetfile.Envelope{
		Format:    assetfile.FormatVersion,
		Kind:      assetfile.KindLights,
		Name:      "win",
		Generator: assetfile.Generator{Tool: "lightsabi_test", Version: "1.0.0"},
		LightsPayload: assetfile.LightsPayload{
			Frames: []assetfile.LightFrame{
				{Lamps: assetfile.NewLightLamps(all, all), HoldMs: 100},
				{Lamps: assetfile.NewLightLamps(dark, nil), HoldMs: 100},
				{Lamps: assetfile.NewLightLamps(all, nil), HoldMs: 100},
				{Lamps: assetfile.NewLightLamps(assetfile.LightMaskPtr(1<<3), nil), HoldMs: 100},
			},
			Loop: assetfile.LoopOnce,
		},
	}
	out, err := assetfile.WriteBytes("win.arkadelights.json", env, nil)
	if err != nil {
		t.Fatalf("WriteBytes: %v", err)
	}
	return out
}

// bothSeatsSequence names both seats in every frame.
func bothSeatsSequence(t *testing.T) []byte {
	t.Helper()
	all := assetfile.LightMaskPtr(assetfile.MaxLightMask)
	dark := assetfile.LightMaskPtr(0)
	env := &assetfile.Envelope{
		Format:    assetfile.FormatVersion,
		Kind:      assetfile.KindLights,
		Name:      "both",
		Generator: assetfile.Generator{Tool: "lightsabi_test", Version: "1.0.0"},
		LightsPayload: assetfile.LightsPayload{
			Frames: []assetfile.LightFrame{
				{Lamps: assetfile.NewLightLamps(all, all), HoldMs: 100},
				{Lamps: assetfile.NewLightLamps(dark, dark), HoldMs: 100},
			},
			Loop: assetfile.LoopCycle,
		},
	}
	out, err := assetfile.WriteBytes("both.arkadelights.json", env, nil)
	if err != nil {
		t.Fatalf("WriteBytes: %v", err)
	}
	return out
}

// mountLights scripts no clock: a sequence walks on Update's deltas.
func mountLights(t *testing.T) (*wasm.Cartridge, *carttest.FakeHost) {
	t.Helper()
	return mountLightsSeq(t, winSequence(t))
}

func mountLightsSeq(t *testing.T, seq []byte) (*wasm.Cartridge, *carttest.FakeHost) {
	t.Helper()
	host := carttest.NewFakeHost()
	host.Assets[lightsFixturePath] = seq
	cart, err := wasm.NewCartridge(context.Background(), lightsWasm, host, wasm.Options{
		Width: 4, Height: 4,
		BundleLights: []string{lightsFixturePath},
	})
	if err != nil {
		t.Fatalf("NewCartridge: %v", err)
	}
	t.Cleanup(func() { _ = cart.Close(context.Background()) })
	if err := cart.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return cart, host
}

// command sends the fixture one record whose first byte is the command number.
func command(t *testing.T, cart *wasm.Cartridge, cmd byte) {
	t.Helper()
	rec := wasm.InputRecord{Kind: wasm.RecordKindKeydown, Key: cmd}
	if err := cart.DispatchInput(context.Background(), []wasm.InputRecord{rec}); err != nil {
		t.Fatalf("DispatchInput(%d): %v", cmd, err)
	}
}

func played(t *testing.T, host *carttest.FakeHost) uint32 {
	t.Helper()
	raw, ok := host.Storage["played"]
	if !ok || len(raw) != 4 {
		t.Fatalf("the fixture echoed %d bytes under \"played\", want 4", len(raw))
	}
	return binary.LittleEndian.Uint32(raw)
}

func masks(host *carttest.FakeHost) [][2]int {
	var out [][2]int
	for _, c := range host.ButtonLights {
		mask := 0
		for i, name := range assetfile.LightButtonNames {
			if c.Lights[name] {
				mask |= 1 << uint(i)
			}
		}
		out = append(out, [2]int{c.Player, mask})
	}
	return out
}

func TestLightsPlayWalksTheSequenceOnTheEngineClock(t *testing.T) {
	// 60 ms ticks over a 400 ms sequence.
	cart, host := mountLights(t)

	command(t, cart, 1) // play both seats, the file's own mode (once)
	if got := played(t, host); got != 1 {
		t.Fatalf("host_lights_play returned %d, want 1", got)
	}
	if got := masks(host); len(got) != 2 || got[0] != [2]int{0, 63} || got[1] != [2]int{1, 63} {
		t.Fatalf("after play the lamp calls are %v, want both seats fully lit", got)
	}

	for i := 0; i < 10; i++ {
		if err := cart.Update(context.Background(), 60_000); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	// Seat 1 sees only frame 0, the only one naming it.
	want := [][2]int{{0, 63}, {1, 63}, {0, 0}, {0, 63}, {0, 1 << 3}}
	got := masks(host)
	if len(got) != len(want) {
		t.Fatalf("lamp calls = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("lamp call %d = %v, want %v (full sequence %v)", i, got[i], want[i], got)
		}
	}
	for i, c := range host.ButtonLights {
		if !c.Only {
			t.Errorf("lamp call %d was a merge, want a complete state (only=true)", i)
		}
	}
}

func TestLightsPlayRefusesAnUnlistedPath(t *testing.T) {
	cart, host := mountLights(t)
	command(t, cart, 3)
	if got := played(t, host); got != 0 {
		t.Fatalf("host_lights_play on an unlisted path returned %d, want 0", got)
	}
	if len(host.ButtonLights) != 0 {
		t.Fatalf("a refused play still wrote lamps: %v", masks(host))
	}
}

func TestLightsExplicitCallWins(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  byte
		mask int
	}{
		{"host_set_button_lights", 5, 1},
		{"host_set_lights", 6, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cart, host := mountLights(t)
			command(t, cart, 2) // seat 0, forced to cycle
			command(t, cart, tc.cmd)
			before := len(host.ButtonLights)

			for i := 0; i < 8; i++ {
				if err := cart.Update(context.Background(), 60_000); err != nil {
					t.Fatalf("Update: %v", err)
				}
			}
			if len(host.ButtonLights) != before {
				t.Fatalf("the sequence kept writing after %s took the lamps: %v",
					tc.name, masks(host)[before:])
			}
		})
	}
}

func TestLightsStopIsNotDark(t *testing.T) {
	cart, host := mountLights(t)
	command(t, cart, 2) // seat 0, cycle
	command(t, cart, 4) // stop
	before := len(host.ButtonLights)
	for i := 0; i < 8; i++ {
		if err := cart.Update(context.Background(), 60_000); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	if len(host.ButtonLights) != before {
		t.Fatalf("stop() was followed by %v", masks(host)[before:])
	}
}

// The host records the call as made; per-tick interpolation is the engine's.
func TestLampLevelsRecordTheCallAsMade(t *testing.T) {
	cart, host := mountLights(t)
	command(t, cart, 7)

	want := carttest.LampLevelsCall{Player: 0, Levels: [6]uint8{1, 2, 3, 4, 5, 6}, Touch: 0x3f, FadeMs: 250}
	if len(host.LampLevels) != 1 || host.LampLevels[0] != want {
		t.Fatalf("lamp level calls = %v, want [%v]", host.LampLevels, want)
	}
	if len(host.ButtonLights) != 0 || len(host.LightSets) != 0 {
		t.Errorf("a levels call also wrote the on/off arrays: %v %v", host.ButtonLights, host.LightSets)
	}
}

func TestLampLevelsTouchingNothingIsANoOp(t *testing.T) {
	cart, host := mountLightsSeq(t, bothSeatsSequence(t))
	command(t, cart, 2) // seat 0, cycling
	command(t, cart, 8) // touch 0
	if len(host.LampLevels) != 0 {
		t.Fatalf("a call touching nothing was recorded: %v", host.LampLevels)
	}
	before := len(host.ButtonLights)
	for i := 0; i < 4; i++ {
		if err := cart.Update(context.Background(), 60_000); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	if len(host.ButtonLights) == before {
		t.Fatal("a call touching nothing stopped the sequence")
	}
}

func TestLampLevelsRefuseAnOutOfRangeSeat(t *testing.T) {
	cart, host := mountLights(t)
	command(t, cart, 9)
	if len(host.LampLevels) != 0 {
		t.Fatalf("seat 2 was honored: %v", host.LampLevels)
	}
}

func TestLampLevelsFadeIsClamped(t *testing.T) {
	cart, host := mountLights(t)
	command(t, cart, 10)
	if len(host.LampLevels) != 1 {
		t.Fatalf("lamp level calls = %v, want one", host.LampLevels)
	}
	got := host.LampLevels[0]
	if got.Player != -1 || got.FadeMs != 10_000 {
		t.Fatalf("call = %v, want both seats and the 10000 ms ceiling", got)
	}
}

func TestLampLevelsStopTheSequenceOnThatSeatOnly(t *testing.T) {
	cart, host := mountLightsSeq(t, bothSeatsSequence(t))
	command(t, cart, 1)  // both seats, cycling
	command(t, cart, 11) // seat 0's joya alone, no fade

	want := carttest.LampLevelsCall{Player: 0, Levels: [6]uint8{15}, Touch: 1, FadeMs: 0}
	if len(host.LampLevels) != 1 || host.LampLevels[0] != want {
		t.Fatalf("lamp level calls = %v, want [%v]", host.LampLevels, want)
	}

	before := masks(host)
	for i := 0; i < 8; i++ {
		if err := cart.Update(context.Background(), 60_000); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	for _, m := range masks(host)[len(before):] {
		if m[0] == 0 {
			t.Fatalf("the sequence kept writing seat 0 after the levels call took it: %v", masks(host))
		}
	}
	if len(masks(host)) == len(before) {
		t.Fatal("seat 1's sequence stopped too; a levels call takes only the seat it addresses")
	}
}

func TestLightsIdleCartridgeWritesNothing(t *testing.T) {
	cart, host := mountLights(t)
	for i := 0; i < 4; i++ {
		if err := cart.Update(context.Background(), 16_000); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	if len(host.ButtonLights) != 0 {
		t.Fatalf("an idle cartridge wrote lamps: %v", masks(host))
	}
}
