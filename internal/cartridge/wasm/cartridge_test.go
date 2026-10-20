// SPDX-License-Identifier: GPL-2.0-or-later

package wasm_test

import (
	"context"
	_ "embed"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/stealthmonkey99/arkade/carttest"
	"github.com/stealthmonkey99/arkade/internal/cartridge/wasm"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/input"
)

//go:embed testdata/good.wasm
var goodWasm []byte

//go:embed testdata/abimismatch.wasm
var abiMismatchWasm []byte

//go:embed testdata/hostile.wasm
var hostileWasm []byte

func le64(b []byte) uint64 { return binary.LittleEndian.Uint64(b) }

// newGoodCartridge uses the 4x4 canvas good.wat's data segments assume.
func newGoodCartridge(t *testing.T, host wasm.Host) *wasm.Cartridge {
	t.Helper()
	cart, err := wasm.NewCartridge(context.Background(), goodWasm, host, wasm.Options{Width: 4, Height: 4})
	if err != nil {
		t.Fatalf("NewCartridge: %v", err)
	}
	t.Cleanup(func() { _ = cart.Close(context.Background()) })
	return cart
}

func assertStoredU64(t *testing.T, host *carttest.FakeHost, key string, want uint64) {
	t.Helper()
	raw, ok := host.Storage[key]
	if !ok || len(raw) != 8 {
		t.Fatalf("storage[%q] = %v, want a stored 8-byte u64", key, raw)
	}
	if got := le64(raw); got != want {
		t.Errorf("storage[%q] = %d, want %d", key, got, want)
	}
}

func containsLog(logs []carttest.LogCall, level wasm.LogLevel, msg string) bool {
	for _, l := range logs {
		if l.Level == level && l.Msg == msg {
			return true
		}
	}
	return false
}

// TestCartridgeLifecycle also pins that the WASI clock and random stubs
// consume the same scripted Host sequence as the host_* imports.
func TestCartridgeLifecycle(t *testing.T) {
	ctx := context.Background()
	host := carttest.NewFakeHost()
	host.NowScript = []uint64{1_000_000, 2_000_000}
	host.RandScript = []uint64{0x1111111111111111, 0x2222222222222222}

	cart := newGoodCartridge(t, host)

	// Asserts the range the host mounts, not equality.
	v, err := cart.ABIVersion(ctx)
	if err != nil || v < wasm.MinSupportedABIVersion || v > wasm.SupportedABIVersion {
		t.Fatalf("ABIVersion() = %d, %v; want %d..%d, nil",
			v, err, wasm.MinSupportedABIVersion, wasm.SupportedABIVersion)
	}
	manifest, err := cart.Manifest(ctx)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	wantManifest := `{"abi":1,"slug":"fixture","name":"Fixture","version":"1.0.0","players":[1,2],"icon":"assets/sprites/icon.png"}`
	if string(manifest) != wantManifest {
		t.Fatalf("Manifest = %q, want %q", manifest, wantManifest)
	}

	if err := cart.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !containsLog(host.Logs, wasm.LogDebug, "init ok") {
		t.Fatalf("cart_init did not log 'init ok'; logs=%v", host.Logs)
	}

	// The fixture echoes the packed buffer verbatim.
	records := []wasm.InputRecord{
		{Kind: wasm.RecordKindKeydown, Key: 6 /* KeyUp, see keyOrdinal */, Player: 1, Joy: 0},
		{Kind: wasm.RecordKindJoystickChange, Key: 0, Player: 2, Joy: 3 /* E */},
	}
	if err := cart.DispatchInput(ctx, records); err != nil {
		t.Fatalf("DispatchInput: %v", err)
	}
	echoed, ok := host.Storage["last_input"]
	if !ok {
		t.Fatalf("cart_handle_input did not echo into storage; storage=%v", host.Storage)
	}
	got := wasm.UnpackRecords(echoed)
	if len(got) != len(records) {
		t.Fatalf("echoed %d records, want %d", len(got), len(records))
	}
	for i, r := range records {
		if got[i] != r {
			t.Errorf("record %d = %+v, want %+v", i, got[i], r)
		}
	}

	const dt = uint64(999)
	if err := cart.Update(ctx, dt); err != nil {
		t.Fatalf("Update: %v", err)
	}
	assertStoredU64(t, host, "now_micros", 1_000_000)
	assertStoredU64(t, host, "random_u64", 0x1111111111111111)
	assertStoredU64(t, host, "wasi_rand", 0x2222222222222222)
	assertStoredU64(t, host, "wasi_now_ns", 2_000_000*1000)
	assertStoredU64(t, host, "dt_micros", dt)

	// The fixture makes one of every draw call, plus host_blit.
	if err := cart.Render(ctx); err != nil {
		t.Fatalf("Render: %v", err)
	}
	wantPixels := []carttest.PixelCall{
		{X: 1, Y: 1, C: display.Pixel{R: 0xFF, A: 0xFF}},
		{X: 2, Y: 2, C: display.Pixel{G: 0xFF, A: 0xFF}},
	}
	if len(host.Pixels) != len(wantPixels) {
		t.Fatalf("pixels = %+v, want %+v", host.Pixels, wantPixels)
	}
	for i, p := range wantPixels {
		if host.Pixels[i] != p {
			t.Errorf("pixel %d = %+v, want %+v", i, host.Pixels[i], p)
		}
	}
	wantRect := carttest.RectCall{X: 0, Y: 3, W: 4, H: 1, C: display.Pixel{B: 0xFF, A: 0xFF}}
	if len(host.Rects) != 1 || host.Rects[0] != wantRect {
		t.Errorf("rects = %+v, want [%+v]", host.Rects, wantRect)
	}
	// The fixture calls each of these thirteen imports once; this catches one
	// wired to the wrong Host method.
	if host.Rejects != 1 {
		t.Errorf("host_reject_key calls = %d, want 1", host.Rejects)
	}

	wantStrategy := []carttest.StrategyCall{
		{Op: "push", Kind: wasm.StrategyOnePlayerSolo},
		{Op: "lock"},
		{Op: "switch", Mode: wasm.SwitchToggle},
		{Op: "unlock"},
		{Op: "pop"},
	}
	if len(host.StrategyCalls) != len(wantStrategy) {
		t.Fatalf("strategy calls = %+v, want %+v", host.StrategyCalls, wantStrategy)
	}
	for i, want := range wantStrategy {
		if host.StrategyCalls[i] != want {
			t.Errorf("strategy call %d = %+v, want %+v", i, host.StrategyCalls[i], want)
		}
	}

	if got, name, ok := host.ScoreRead(1); !ok || got != 100 || name != "ABC" {
		t.Errorf("score rank 1 = (%d, %q, %v), want (100, \"ABC\", true)", got, name, ok)
	}
	if n := host.ScoreCount(); n != 1 {
		t.Errorf("score count = %d, want 1", n)
	}

	wantNote := []carttest.RecentNameCall{{Player: 0, Name: "ABC"}}
	if len(host.NoteNameCalls) != 1 || host.NoteNameCalls[0] != wantNote[0] {
		t.Errorf("recent-name notes = %+v, want %+v", host.NoteNameCalls, wantNote)
	}
	if name, ok := host.RecentName(0); !ok || name != "ABC" {
		t.Errorf("RecentName(0) = (%q, %v), want (\"ABC\", true)", name, ok)
	}

	// The fixture walks the MRU, echoing entry 0; a read past the end
	// returns zero bytes, the guest's stop signal.
	if got, ok := host.Storage["mru_name"]; !ok || string(got) != "ABC" {
		t.Errorf("MRU entry 0 echoed = %q (present=%v), want \"ABC\"", got, ok)
	}
	if got, ok := host.Storage["mru_past"]; !ok || len(got) != 4 || got[0]|got[1]|got[2]|got[3] != 0 {
		t.Errorf("read past the end of a one-entry MRU echoed %v, want four zero bytes", got)
	}

	if len(host.Sprites) != 1 {
		t.Errorf("sprites loaded = %d, want 1", len(host.Sprites))
	}
	if len(host.Draws) != 1 || host.Draws[0].X != 0 || host.Draws[0].Y != 0 {
		t.Errorf("sprite draws = %+v, want one draw at (0,0)", host.Draws)
	}
	wantSoundSeq := []string{"load", "play", "stop", "stopall", "load", "play"}
	if len(host.Sounds) != len(wantSoundSeq) {
		t.Fatalf("sound calls = %+v, want sequence %v", host.Sounds, wantSoundSeq)
	}
	for i, action := range wantSoundSeq {
		if host.Sounds[i].Action != action {
			t.Errorf("sound call %d action = %q, want %q", i, host.Sounds[i].Action, action)
		}
	}
	if host.Sounds[0].Path != "assets/sound.wav" {
		t.Errorf("loaded sound path = %q, want assets/sound.wav", host.Sounds[0].Path)
	}
	// host_load_sound never reads the envelope, so an .arkadesound.wav loads
	// and plays like a plain .wav.
	if host.Sounds[4].Path != "assets/blip.arkadesound.wav" {
		t.Errorf("loaded asset-file sound path = %q, want assets/blip.arkadesound.wav", host.Sounds[4].Path)
	}
	if host.Sounds[4].ID == host.Sounds[0].ID {
		t.Errorf("both loads returned handle %d; each path gets its own", host.Sounds[4].ID)
	}
	if len(host.Blits) != 1 || len(host.Blits[0]) != 16 {
		t.Fatalf("blits = %+v, want one 16-pixel (4x4) frame", host.Blits)
	}
	if host.Blits[0][0] != (display.Pixel{R: 0xFF, A: 0xFF}) {
		t.Errorf("blit pixel 0 = %+v, want {R:255 A:255}", host.Blits[0][0])
	}
	if !containsLog(host.Logs, wasm.LogInfo, "render ok") {
		t.Errorf("cart_render did not log 'render ok'; logs=%v", host.Logs)
	}

	// Teardown also exercises the WASI fd_write to host_log bridge.
	if err := cart.Teardown(ctx); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	if !containsLog(host.Logs, wasm.LogInfo, "teardown") {
		t.Errorf("cart_teardown did not log 'teardown'; logs=%v", host.Logs)
	}
	if !containsLog(host.Logs, wasm.LogInfo, "bye\n") {
		t.Errorf("wasi fd_write did not bridge to host_log; logs=%v", host.Logs)
	}
}

func TestEventRecordsReachTheGuest(t *testing.T) {
	ctx := context.Background()
	host := carttest.NewFakeHost()
	cart := newGoodCartridge(t, host)
	if err := cart.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// A plain keyboard, and a real stick in port 1 paired as player 2.
	events := []input.Event{
		{Kind: input.KindKeydown, Name: input.KeyUp, Source: input.PlayerSource{Type: input.SourceKeyboard, ID: "keyboard"}},
		{Kind: input.KindJoystickChange, Position: input.JoyRight, Source: input.PlayerSource{
			Type: input.SourceController, ID: "hid-1", Slot: input.SlotOf(1), Seat: input.SeatP2,
		}},
	}
	if err := cart.DispatchInput(ctx, wasm.EventRecords(events)); err != nil {
		t.Fatalf("DispatchInput: %v", err)
	}
	echoed, ok := host.Storage["last_input"]
	if !ok {
		t.Fatal("no input echoed into storage")
	}
	upOrdinal, ok := wasm.KeyToOrdinal(input.KeyUp)
	if !ok {
		t.Fatal("no ordinal for KeyUp")
	}
	want := []wasm.InputRecord{
		// A plain keyboard is SeatNone, not seat 0, and virtual.
		{
			Kind: wasm.RecordKindKeydown, Key: upOrdinal, Player: 1, Joy: 0,
			Source: wasm.RecordSourceKeyboard, Physical: wasm.SeatNone, Index: wasm.SeatNone,
			Flags: wasm.RecordFlagVirtual,
		},
		{
			Kind: wasm.RecordKindJoystickChange, Key: 0, Player: 2, Joy: 3,
			Source: wasm.RecordSourceController, Physical: 1, Index: 1,
			Flags: 0,
		},
	}
	got := wasm.UnpackRecords(echoed)
	if len(got) != len(want) {
		t.Fatalf("got %d records, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("record %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestABIMismatch(t *testing.T) {
	ctx := context.Background()
	host := carttest.NewFakeHost()
	cart, err := wasm.NewCartridge(ctx, abiMismatchWasm, host, wasm.Options{Width: 4, Height: 4})
	if err != nil {
		t.Fatalf("NewCartridge: %v", err)
	}
	defer func() { _ = cart.Close(ctx) }()

	if err := cart.Init(ctx); err == nil {
		t.Fatal("Init succeeded on a cartridge with an unsupported ABI version, want an error")
	} else if !errors.Is(err, wasm.ErrABIMismatch) {
		t.Fatalf("Init error = %v, want it to wrap wasm.ErrABIMismatch", err)
	}
	if len(host.Logs) != 0 || len(host.Blits) != 0 || len(host.Pixels) != 0 {
		t.Fatalf("mismatched cartridge touched the host before rejection: %+v", host)
	}
}

// TestHostileCartridgeFailsClean hands every import a bad pointer or length;
// none may reach the Host, and a panic here is the failure guarded against.
func TestHostileCartridgeFailsClean(t *testing.T) {
	ctx := context.Background()
	host := carttest.NewFakeHost()
	host.NowScript = []uint64{1}
	host.RandScript = []uint64{2}
	cart, err := wasm.NewCartridge(ctx, hostileWasm, host, wasm.Options{Width: 4, Height: 4})
	if err != nil {
		t.Fatalf("NewCartridge: %v", err)
	}
	defer func() { _ = cart.Close(ctx) }()

	if err := cart.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cart.Update(ctx, 1); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := cart.Render(ctx); err != nil {
		t.Fatalf("Render (hostile host_* calls should no-op, not error): %v", err)
	}
	// host_play_sound(0xDEADBEEF) does reach the Host: only the Host owns
	// that handle space. Every (ptr, len) call must not.
	if len(host.Pixels) != 0 || len(host.Rects) != 0 ||
		len(host.Blits) != 0 || len(host.Sprites) != 0 || len(host.Draws) != 0 {
		t.Fatalf("hostile cartridge's malformed (ptr, len) calls reached the host: %+v", host)
	}
	warnCount := 0
	for _, l := range host.Logs {
		if l.Level == wasm.LogWarn {
			warnCount++
		}
	}
	if warnCount == 0 {
		t.Fatalf("expected sandbox-boundary warnings, got none; logs=%v", host.Logs)
	}

	// The fixture's cart_alloc returns an out-of-bounds pointer.
	err = cart.DispatchInput(ctx, []wasm.InputRecord{{Kind: wasm.RecordKindKeypress, Key: 1, Player: 1}})
	if err == nil {
		t.Fatal("DispatchInput against an out-of-bounds cart_alloc pointer succeeded, want an error")
	}

	if err := cart.Teardown(ctx); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
}

func TestPackUnpackRecordsRoundtrip(t *testing.T) {
	in := []wasm.InputRecord{{Kind: 1, Key: 2, Player: 3, Joy: 4}, {Kind: 5, Key: 6, Player: 7, Joy: 8}}
	buf := wasm.PackRecords(in)
	if len(buf) != 16 {
		t.Fatalf("len(buf) = %d, want 16", len(buf))
	}
	out := wasm.UnpackRecords(buf)
	if len(out) != len(in) {
		t.Fatalf("unpack len = %d, want %d", len(out), len(in))
	}
	for i := range in {
		if out[i] != in[i] {
			t.Errorf("record %d = %+v, want %+v", i, out[i], in[i])
		}
	}
}

func BenchmarkUpdateRender(b *testing.B) {
	ctx := context.Background()
	host := carttest.NewFakeHost()
	cart, err := wasm.NewCartridge(ctx, goodWasm, host, wasm.Options{Width: 4, Height: 4})
	if err != nil {
		b.Fatalf("NewCartridge: %v", err)
	}
	defer func() { _ = cart.Close(ctx) }()
	if err := cart.Init(ctx); err != nil {
		b.Fatalf("Init: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cart.Update(ctx, 16_666); err != nil {
			b.Fatal(err)
		}
		if err := cart.Render(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
