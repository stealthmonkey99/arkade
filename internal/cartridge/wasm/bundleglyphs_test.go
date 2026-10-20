// SPDX-License-Identifier: GPL-2.0-or-later

package wasm_test

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/carttest"
	"github.com/stealthmonkey99/arkade/internal/cartridge/wasm"
)

// Bundled glyphs share host_define_lcd_glyph's table and rules. The browser
// host has a twin of this file.

// oneGlyphFile is hand-built JSON, so writer and parser cannot agree on a wrong shape.
func oneGlyphFile(t *testing.T, codepoint int, rows []int) []byte {
	t.Helper()
	doc := map[string]any{
		"arkade":    1,
		"kind":      "glyph",
		"name":      "fixture",
		"generator": map[string]any{"tool": "carttest", "version": "1.0.0"},
		"codepoint": codepoint,
		"rows":      rows,
		"category":  "tetromino",
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshalling the fixture: %v", err)
	}
	return data
}

// mountWithGlyphs stops at NewCartridge: registration happens before cart_init.
func mountWithGlyphs(t *testing.T, files map[string][]byte, paths []string) (*wasm.Cartridge, *carttest.FakeHost) {
	t.Helper()
	host := carttest.NewFakeHost()
	for p, data := range files {
		host.Assets[p] = data
	}
	cart, err := wasm.NewCartridge(context.Background(), glyphsWasm, host, wasm.Options{
		Width: 4, Height: 4, BundleGlyphs: paths,
	})
	if err != nil {
		t.Fatalf("NewCartridge: %v", err)
	}
	t.Cleanup(func() { _ = cart.Close(context.Background()) })
	return cart, host
}

// theCoin is in no engine table, so drawing it can only be the bundle's doing.
const theCoin = '¤'

func TestBundleGlyphRegistersAtMount(t *testing.T) {
	const path = "assets/glyphs/coin.arkadeglyph.json"
	rows := []int{0x0e, 0x11, 0x15, 0x15, 0x15, 0x11, 0x0e, 0x00}
	_, host := mountWithGlyphs(t,
		map[string][]byte{path: oneGlyphFile(t, int(theCoin), rows)},
		[]string{path})

	got, ok := host.CartGlyphs[theCoin]
	if !ok {
		t.Fatalf("the bundled rune never reached the host (table has %d entries)", len(host.CartGlyphs))
	}
	for i, want := range rows {
		if int(got[i]) != want {
			t.Errorf("row %d = %#x, want %#x", i, got[i], want)
		}
	}
}

func TestNoBundleGlyphsLeavesTheTableAlone(t *testing.T) {
	_, host := mountWithGlyphs(t, nil, nil)
	if host.CartGlyphs != nil {
		t.Errorf("mounting with no assets.glyphs installed %v", host.CartGlyphs)
	}
}

func TestADefinitionOverridesTheBundledGlyph(t *testing.T) {
	const path = "assets/glyphs/basket.arkadeglyph.json"
	// The fixture defines this rune itself, with a different bitmap.
	fromBundle := []int{0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f}
	cart, host := mountWithGlyphs(t,
		map[string][]byte{path: oneGlyphFile(t, int('֍'), fromBundle)},
		[]string{path})

	if got := host.CartGlyphs['֍']; got != [8]byte{0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f} {
		t.Fatalf("before cart_init the bundle's bitmap should be installed, got %#v", got)
	}
	if err := cart.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	want := [8]byte{0x00, 0x0e, 0x11, 0x1f, 0x15, 0x1f, 0x0e, 0x00} // the fixture's own
	if got := host.CartGlyphs['֍']; got != want {
		t.Errorf("after cart_init rows = %#v, want the cartridge's own %#v", got, want)
	}
}

func TestBundleGlyphIsClearedAtTeardown(t *testing.T) {
	const path = "assets/glyphs/coin.arkadeglyph.json"
	cart, host := mountWithGlyphs(t,
		map[string][]byte{path: oneGlyphFile(t, int(theCoin), []int{1, 2, 3, 4, 5, 6, 7, 0})},
		[]string{path})
	if err := cart.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cart.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	if host.CartGlyphs != nil {
		t.Errorf("a bundled glyph outlived its cartridge: %v", host.CartGlyphs)
	}
}

func TestBundleGlyphRefusalsWarnAndSkip(t *testing.T) {
	files := map[string][]byte{
		"assets/glyphs/ascii.arkadeglyph.json":    oneGlyphFile(t, 'A', []int{1, 1, 1, 1, 1, 1, 1, 1}),
		"assets/glyphs/wide.arkadeglyph.json":     oneGlyphFile(t, int(theCoin), []int{0xff, 0, 0, 0, 0, 0, 0, 0}),
		"assets/glyphs/short.arkadeglyph.json":    oneGlyphFile(t, int(theCoin), []int{1, 2, 3}),
		"assets/glyphs/notaglyph.arkadesound.wav": []byte("RIFF"),
	}
	paths := []string{
		"assets/glyphs/ascii.arkadeglyph.json",
		"assets/glyphs/wide.arkadeglyph.json",
		"assets/glyphs/short.arkadeglyph.json",
		"assets/glyphs/notaglyph.arkadesound.wav",
		"assets/glyphs/absent.arkadeglyph.json",
	}
	cart, host := mountWithGlyphs(t, files, paths)
	if err := cart.Init(context.Background()); err != nil {
		t.Fatalf("a bundle of broken glyph files must still mount: %v", err)
	}
	if _, ok := host.CartGlyphs['A']; ok {
		t.Error("an ASCII code point was accepted from a file — the panel's own ROM draws those")
	}
	if _, ok := host.CartGlyphs[theCoin]; ok {
		t.Error("a glyph with out-of-range rows was accepted")
	}

	// Each reason once, in the same words the browser host produces.
	want := []string{
		"below U+0080",                       // ascii.arkadeglyph.json
		"want within 0..",                    // wide.arkadeglyph.json (row over 5 bits)
		"rows, want 8",                       // short.arkadeglyph.json
		"not a <name>.arkadeglyph.json file", // the .wav listed as a glyph
		"no such asset",                      // absent.arkadeglyph.json
	}
	joined := strings.Join(warnings(host), "\n")
	for _, w := range want {
		if !strings.Contains(joined, w) {
			t.Errorf("no warning mentioned %q; warnings were:\n%s", w, joined)
		}
	}
}

func TestTeardownBeforeInitDropsBundleGlyphs(t *testing.T) {
	const path = "assets/glyphs/coin.arkadeglyph.json"
	cart, host := mountWithGlyphs(t,
		map[string][]byte{path: oneGlyphFile(t, int(theCoin), []int{0, 0, 0, 0, 0, 0, 0, 0x1f})},
		[]string{path})
	ctx := context.Background()
	if err := cart.Update(ctx, 33_333); !errors.Is(err, wasm.ErrNotInitialized) {
		t.Errorf("Update before Init = %v, want ErrNotInitialized", err)
	}
	if err := cart.Render(ctx); !errors.Is(err, wasm.ErrNotInitialized) {
		t.Errorf("Render before Init = %v, want ErrNotInitialized", err)
	}
	if err := cart.Teardown(ctx); err != nil {
		t.Fatalf("Teardown before Init = %v, want nil", err)
	}
	if host.CartGlyphs != nil {
		t.Errorf("glyphs still installed after teardown: %v", host.CartGlyphs)
	}
}

func warnings(host *carttest.FakeHost) []string {
	var out []string
	for _, l := range host.Logs {
		if l.Level == wasm.LogWarn {
			out = append(out, l.Msg)
		}
	}
	return out
}
