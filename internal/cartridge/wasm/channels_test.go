// SPDX-License-Identifier: GPL-2.0-or-later

package wasm_test

import (
	"context"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/carttest"
	"github.com/stealthmonkey99/arkade/internal/cartridge/wasm"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// The browser host has a twin of this file.

//go:embed testdata/channels.wasm
var channelsWasm []byte

func mountChannels(t *testing.T, prep func(*carttest.FakeHost), opts wasm.Options) (*wasm.Cartridge, *carttest.FakeHost) {
	t.Helper()
	host := carttest.NewFakeHost()
	if prep != nil {
		prep(host)
	}
	opts.Width, opts.Height = 4, 4
	cart, err := wasm.NewCartridge(context.Background(), channelsWasm, host, opts)
	if err != nil {
		t.Fatalf("NewCartridge: %v", err)
	}
	t.Cleanup(func() { _ = cart.Close(context.Background()) })
	if err := cart.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return cart, host
}

func binds(t *testing.T, host *carttest.FakeHost) []int32 {
	t.Helper()
	raw, ok := host.Storage["binds"]
	if !ok || len(raw) != 24 {
		t.Fatalf("the fixture echoed %d bind bytes, want 24", len(raw))
	}
	out := make([]int32, 6)
	for i := range out {
		out[i] = int32(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return out
}

func reads(t *testing.T, cart *wasm.Cartridge, host *carttest.FakeHost) (button, axis, delta, blobLen int32, head [4]byte) {
	t.Helper()
	if err := cart.Update(context.Background(), 33000); err != nil {
		t.Fatalf("Update: %v", err)
	}
	raw, ok := host.Storage["read"]
	if !ok || len(raw) != 20 {
		t.Fatalf("the fixture echoed %d read bytes, want 20", len(raw))
	}
	v := func(i int) int32 { return int32(binary.LittleEndian.Uint32(raw[i*4:])) }
	copy(head[:], raw[16:20])
	return v(0), v(1), v(2), v(3), head
}

// The FakeHost offers nothing, yet the panel channels bind and read.
func TestPanelChannelsComeFromTheRecords(t *testing.T) {
	cart, host := mountChannels(t, nil, wasm.Options{})
	h := binds(t, host)
	if h[0] < 0 || h[1] < 0 {
		t.Fatalf("the panel channels did not bind: %v", h)
	}
	if h[2] != -1 || h[3] != -1 {
		t.Errorf("a spinner and a mat bound on a host with no HID at all: %v", h)
	}
	if h[4] != -1 {
		t.Errorf("p1.joy.x bound as a BUTTON (handle %d); the kind disagrees and must fail", h[4])
	}
	if h[5] != -1 {
		t.Errorf("an output bind returned %d; outputs are reserved and always fail", h[5])
	}

	if button, axis, _, _, _ := reads(t, cart, host); button != 0 || axis != 0 {
		t.Fatalf("at rest the panel read button=%d axis=%d, want 0 and 0", button, axis)
	}

	dispatch(t, cart, keyRecord(wasm.RecordKindKeydown, input.KeyJoyA, 1))
	dispatch(t, cart, keyRecord(wasm.RecordKindKeydown, input.KeyRight, 1))
	if button, axis, _, _, _ := reads(t, cart, host); button != 1 || axis != 1 {
		t.Fatalf("held: button=%d axis=%d, want 1 and 1", button, axis)
	}

	dispatch(t, cart, keyRecord(wasm.RecordKindKeyup, input.KeyJoyA, 1))
	dispatch(t, cart, keyRecord(wasm.RecordKindKeyup, input.KeyRight, 1))
	dispatch(t, cart, keyRecord(wasm.RecordKindKeydown, input.KeyLeft, 1))
	if button, axis, _, _, _ := reads(t, cart, host); button != 0 || axis != -1 {
		t.Fatalf("released and pushed left: button=%d axis=%d, want 0 and -1", button, axis)
	}
}

// The fixture binds only p1, so this asserts a player 2 record leaves it alone.
func TestPanelChannelsAreSeated(t *testing.T) {
	cart, host := mountChannels(t, nil, wasm.Options{})
	dispatch(t, cart, keyRecord(wasm.RecordKindKeydown, input.KeyJoyA, 2))
	if button, _, _, _, _ := reads(t, cart, host); button != 0 {
		t.Fatalf("p2's press moved p1.a (button=%d)", button)
	}
	dispatch(t, cart, keyRecord(wasm.RecordKindKeydown, input.KeyJoyA, 1))
	if button, _, _, _, _ := reads(t, cart, host); button != 1 {
		t.Fatalf("p1's press did not move p1.a (button=%d)", button)
	}
}

func TestSpinnerAndBlobChannels(t *testing.T) {
	mat := []byte{9, 8, 7, 6, 5, 4, 3, 2}
	cart, host := mountChannels(t, func(h *carttest.FakeHost) {
		h.SetChannel("p1.spinner", carttest.FakeChannel{Kind: carttest.ChannelDelta, Value: 5})
		h.SetChannel("p1.mat", carttest.FakeChannel{Kind: carttest.ChannelBlob, Blob: mat})
	}, wasm.Options{})

	h := binds(t, host)
	if h[2] < 0 || h[3] < 0 {
		t.Fatalf("an offered spinner and mat did not bind: %v", h)
	}

	_, _, delta, blobLen, head := reads(t, cart, host)
	if delta != 5 {
		t.Errorf("delta = %d, want the 5 counts the host had accumulated", delta)
	}
	if blobLen != int32(len(mat)) {
		t.Errorf("blob copied %d bytes, want %d", blobLen, len(mat))
	}
	if head != [4]byte{9, 8, 7, 6} {
		t.Errorf("blob head = %v, want [9 8 7 6]", head)
	}

	// A delta is motion, so a quiet tick reads zero.
	if _, _, delta, _, _ := reads(t, cart, host); delta != 0 {
		t.Errorf("delta = %d on a quiet tick, want 0 — the first read consumed it", delta)
	}
}

// dancePadFile aliases an unknown device's arrow buttons onto the panel
// stick. Hand-built JSON, for oneGlyphFile's reason.
func dancePadFile(t *testing.T) []byte {
	t.Helper()
	doc := map[string]any{
		"arkade":    1,
		"kind":      "hid",
		"name":      "dancepad",
		"generator": map[string]any{"tool": "carttest", "version": "1.0.0"},
		"device":    map[string]any{"vendor": 0x1234, "product": 0x5678, "name": "Dance pad"},
		"channels": []any{
			map[string]any{"name": "pad.up", "kind": "button", "report": map[string]any{"byte": 0, "bit": 0}},
			map[string]any{"name": "pad.down", "kind": "button", "report": map[string]any{"byte": 0, "bit": 1}},
			map[string]any{"name": "pad.left", "kind": "button", "report": map[string]any{"byte": 0, "bit": 2}},
			map[string]any{"name": "pad.right", "kind": "button", "report": map[string]any{"byte": 0, "bit": 3}},
			map[string]any{"name": "spinner", "kind": "delta", "report": map[string]any{"byte": 1, "size": 2, "signed": true}},
			map[string]any{"name": "mat", "kind": "blob", "report": map[string]any{"byte": 3, "size": 8}},
		},
		"standard": map[string]any{
			"pad.up": "joy.north", "pad.down": "joy.south",
			"pad.left": "joy.west", "pad.right": "joy.east",
		},
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshalling the fixture: %v", err)
	}
	return data
}

func TestBundledProfileIsInstalledAtMountAndDroppedAtTeardown(t *testing.T) {
	const path = "assets/dancepad.arkadehid.json"
	host := carttest.NewFakeHost()
	host.Assets[path] = dancePadFile(t)
	cart, err := wasm.NewCartridge(context.Background(), channelsWasm, host, wasm.Options{
		Width: 4, Height: 4, BundleHID: []string{path},
	})
	if err != nil {
		t.Fatalf("NewCartridge: %v", err)
	}
	defer func() { _ = cart.Close(context.Background()) }()

	// Installed before cart_init, so its binds see the profile's channels.
	if len(host.SessionProfiles) != 1 {
		t.Fatalf("the bundle's profile was not installed at mount (%d profiles)", len(host.SessionProfiles))
	}
	spec := host.SessionProfiles[0]
	if got := spec.Envelope.Device.Name; got != "Dance pad" {
		t.Errorf("installed profile is %q", got)
	}
	if len(spec.Channels) != 6 {
		t.Errorf("compiled profile has %d channels, want 6", len(spec.Channels))
	}
	if got := spec.Decode([]byte{0b0001, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}); got.Y != -1 {
		t.Errorf("pad.up decoded to Y=%d, want -1 (north)", got.Y)
	}

	if err := cart.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cart.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	if host.SessionProfiles != nil {
		t.Errorf("the bundle's profiles outlived the cartridge (%d still installed)", len(host.SessionProfiles))
	}
}

func TestBundledProfileFailuresAreWarningsNotErrors(t *testing.T) {
	host := carttest.NewFakeHost()
	host.Assets["assets/good.arkadehid.json"] = dancePadFile(t)
	host.Assets["assets/wrongname.json"] = dancePadFile(t)
	host.Assets["assets/broken.arkadehid.json"] = []byte(`{"arkade":1,"kind":"hid","name":"x","generator":{"tool":"t","version":"1"},"channels":[]}`)
	cart, err := wasm.NewCartridge(context.Background(), channelsWasm, host, wasm.Options{
		Width: 4, Height: 4,
		BundleHID: []string{
			"assets/missing.arkadehid.json",
			"assets/wrongname.json",
			"assets/broken.arkadehid.json",
			"assets/good.arkadehid.json",
		},
	})
	if err != nil {
		t.Fatalf("NewCartridge: %v — a broken profile must never keep a cartridge from launching", err)
	}
	defer func() { _ = cart.Close(context.Background()) }()

	if len(host.SessionProfiles) != 1 {
		t.Fatalf("installed %d profiles, want only the good one", len(host.SessionProfiles))
	}
	for _, want := range []string{
		"no such asset",
		"not a <name>.arkadehid.json file",
		"no channels",
	} {
		if !loggedContaining(host, want) {
			t.Errorf("no warning mentioning %q; logs: %v", want, hostLogLines(host))
		}
	}
}

func keyRecord(kind uint8, key input.KeyName, player uint8) wasm.InputRecord {
	ord, _ := wasm.KeyToOrdinal(key)
	return wasm.InputRecord{
		Kind: kind, Key: ord, Player: player,
		Source: wasm.RecordSourceController, Physical: player - 1, Index: player - 1,
	}
}

func dispatch(t *testing.T, cart *wasm.Cartridge, recs ...wasm.InputRecord) {
	t.Helper()
	if err := cart.DispatchInput(context.Background(), recs); err != nil {
		t.Fatalf("DispatchInput: %v", err)
	}
}

func hostLogLines(host *carttest.FakeHost) []string {
	host.Mu.Lock()
	defer host.Mu.Unlock()
	out := make([]string, len(host.Logs))
	for i, l := range host.Logs {
		out[i] = l.Msg
	}
	return out
}

func loggedContaining(host *carttest.FakeHost, want string) bool {
	for _, msg := range hostLogLines(host) {
		if strings.Contains(msg, want) {
			return true
		}
	}
	return false
}
