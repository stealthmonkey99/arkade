// SPDX-License-Identifier: GPL-2.0-or-later

package wasm_test

import (
	"context"
	_ "embed"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/carttest"
	"github.com/stealthmonkey99/arkade/internal/cartridge/wasm"
)

// The glyphs fixture deliberately misbehaves: the rejections are what both
// hosts must agree on, and the browser host has a twin of this file.

//go:embed testdata/glyphs.wasm
var glyphsWasm []byte

func newGlyphCartridge(t *testing.T, host wasm.Host) *wasm.Cartridge {
	t.Helper()
	cart, err := wasm.NewCartridge(context.Background(), glyphsWasm, host, wasm.Options{Width: 4, Height: 4})
	if err != nil {
		t.Fatalf("NewCartridge: %v", err)
	}
	t.Cleanup(func() { _ = cart.Close(context.Background()) })
	return cart
}

func initGlyphCartridge(t *testing.T) (*wasm.Cartridge, *carttest.FakeHost) {
	t.Helper()
	host := carttest.NewFakeHost()
	cart := newGlyphCartridge(t, host)
	if err := cart.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return cart, host
}

func TestDefineLCDGlyphInstallsTheCartridgeTable(t *testing.T) {
	_, host := initGlyphCartridge(t)

	// Low byte of the wire's u64 is the top row.
	want := [8]byte{0x00, 0x0e, 0x11, 0x1f, 0x15, 0x1f, 0x0e, 0x00}
	got, ok := host.CartGlyphs['֍']
	if !ok {
		t.Fatalf("the cartridge's own rune never reached the host (table has %d entries)", len(host.CartGlyphs))
	}
	if got != want {
		t.Errorf("֍ rows = %#v, want %#v — the u64 unpacked wrong", got, want)
	}
}

func TestDefineLCDGlyphRejectsASCIIAndSurrogates(t *testing.T) {
	_, host := initGlyphCartridge(t)

	if _, ok := host.CartGlyphs['A']; ok {
		t.Error("'A' was accepted — the panel's own ROM draws ASCII, and redefining it burns a CGRAM slot on every line")
	}
	if _, ok := host.CartGlyphs[rune(0xD800)]; ok {
		t.Error("a surrogate half was accepted as a code point")
	}

	var ascii, notACodePoint bool
	for _, l := range host.Logs {
		if l.Level != wasm.LogWarn {
			continue
		}
		if strings.Contains(l.Msg, "host_define_lcd_glyph") && strings.Contains(l.Msg, "is ASCII") {
			ascii = true
		}
		if strings.Contains(l.Msg, "host_define_lcd_glyph") && strings.Contains(l.Msg, "not a code point") {
			notACodePoint = true
		}
	}
	if !ascii || !notACodePoint {
		t.Errorf("rejections went unlogged (ascii=%v, notACodePoint=%v)", ascii, notACodePoint)
	}
}

// The fixture tries U+1F9FA and U+4E00 with the accepted U+058D's bitmap;
// the return values are pinned by TestDefineLCDGlyphCapsTheTable's sum.
func TestDefineLCDGlyphRejectsEmojiAndWideRunes(t *testing.T) {
	_, host := initGlyphCartridge(t)

	if _, ok := host.CartGlyphs['֍']; !ok {
		t.Error("֍ U+058D — plain, narrow, monochrome — was refused; that is exactly the rune the rule is for")
	}
	if _, ok := host.CartGlyphs['🧺']; ok {
		t.Error("🧺 U+1F9FA was accepted — a terminal draws an emoji in colour and two cells wide")
	}
	if _, ok := host.CartGlyphs['一']; ok {
		t.Error("一 U+4E00 was accepted — an East Asian wide rune is two cells by definition")
	}

	var emoji, wide bool
	for _, l := range host.Logs {
		if l.Level != wasm.LogWarn || !strings.Contains(l.Msg, "host_define_lcd_glyph") {
			continue
		}
		if strings.Contains(l.Msg, "0x1f9fa is emoji") && strings.Contains(l.Msg, "single-width, monochrome") {
			emoji = true
		}
		if strings.Contains(l.Msg, "0x4e00 is wide") && strings.Contains(l.Msg, "single-width, monochrome") {
			wide = true
		}
	}
	if !emoji || !wide {
		t.Errorf("rejections went unlogged (emoji=%v, wide=%v) — an author staring at a '?' needs the rule spelled out", emoji, wide)
	}
}

func TestDefineLCDGlyphCapsTheTable(t *testing.T) {
	_, host := initGlyphCartridge(t)

	// The fixture asks for 1 + 41 = 42 valid runes. 32 is the cap.
	const cap = 32
	if n := len(host.CartGlyphs); n != cap {
		t.Errorf("table holds %d glyphs, want the cap of %d", n, cap)
	}

	// The fixture stores the sum of every return: 1 basket, four refusals,
	// then 31 private-use runes that fit.
	raw, ok := host.Storage["accepted"]
	if !ok || len(raw) != 4 {
		t.Fatalf("storage[\"accepted\"] = %v, want 4 bytes", raw)
	}
	if got := binary.LittleEndian.Uint32(raw); got != cap {
		t.Errorf("the cartridge saw %d acceptances, want %d — the return value disagrees with the table", got, cap)
	}

	var full bool
	for _, l := range host.Logs {
		if l.Level == wasm.LogWarn && strings.Contains(l.Msg, "table is full") {
			full = true
		}
	}
	if !full {
		t.Error("overflowing the table went unlogged")
	}
}

func TestCartGlyphsAreClearedAtTeardown(t *testing.T) {
	cart, host := initGlyphCartridge(t)
	if len(host.CartGlyphs) == 0 {
		t.Fatal("nothing was installed, so there is nothing to prove cleared")
	}
	if err := cart.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	if host.CartGlyphs != nil {
		t.Errorf("the cartridge's glyphs outlived it: %v", host.CartGlyphs)
	}
}

func TestDefineLCDGlyphWithoutAGlyphSink(t *testing.T) {
	fake := carttest.NewFakeHost()
	var bare wasm.Host = noGlyphHost{Host: fake}
	if _, ok := bare.(wasm.GlyphDefiner); ok {
		t.Fatal("noGlyphHost was supposed to NOT implement GlyphDefiner — this test proves nothing")
	}
	cart := newGlyphCartridge(t, bare)
	if err := cart.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	raw, ok := fake.Storage["accepted"]
	if !ok || len(raw) != 4 {
		t.Fatalf("storage[\"accepted\"] = %v, want 4 bytes", raw)
	}
	if got := binary.LittleEndian.Uint32(raw); got != 32 {
		t.Errorf("a host with nowhere to put glyphs reported %d acceptances, want 32", got)
	}
	if fake.CartGlyphs != nil {
		t.Error("the bridge reached past the missing interface and installed glyphs anyway")
	}
}

// noGlyphHost embeds the interface, not *FakeHost, so SetCartGlyphs is not promoted.
type noGlyphHost struct{ wasm.Host }

var _ wasm.Host = noGlyphHost{}
