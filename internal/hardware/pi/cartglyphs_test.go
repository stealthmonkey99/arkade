// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"testing"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/lcdrune"
)

func TestEngineGlyphTableObeysTheRuneRule(t *testing.T) {
	for r := range customChars {
		if why := lcdrune.Reason(r); why != "" {
			t.Errorf("customChars[%q] (U+%04X) is %s — the engine table must stay inside the rule cartridges are held to", r, r, why)
		}
	}
}

func TestCartGlyphDefinesARuneTheEngineDoesNotKnow(t *testing.T) {
	suite, sc, bus := newPreviewScreen(t)

	const basket = '֍'
	if _, ok := customChars[basket]; ok {
		t.Fatal("this test needs a rune the shipped table does not define")
	}
	sc.setText(string(basket), "")
	if data := decodeData(t, bus.forAddr(0x27)); len(data) == 0 || !containsByte(data, '?') {
		t.Fatalf("undefined rune should print as '?', got %q", data)
	}

	rows := hardware.GlyphRows{0x00, 0x0e, 0x11, 0x1f, 0x15, 0x1f, 0x0e, 0x00}
	suite.SetCartGlyphs(map[rune]hardware.GlyphRows{basket: rows})
	bus.reset()
	sc.setText(string(basket), "")

	got := pairBytes(t, decodeNibbles(t, bus.forAddr(0x27)))
	if got[0].rs || got[0].b != lcdCmdSetCGRAM {
		t.Fatalf("cart glyph should upload to CGRAM slot 0, first write %#x", got[0].b)
	}
	for i, row := range rows {
		if got[i+1].b != row {
			t.Errorf("slot 0 row %d = %#x, want %#x", i, got[i+1].b, row)
		}
	}
	if data := decodeData(t, bus.forAddr(0x27)); containsByte(data, '?') {
		t.Errorf("printed bytes still contain '?': %q", data)
	}
}

func TestCartGlyphShadowsTheEngineTable(t *testing.T) {
	suite, sc, bus := newPreviewScreen(t)

	sc.setText("♥", "")

	redrawn := hardware.GlyphRows{0x1f, 0x11, 0x11, 0x11, 0x0a, 0x0a, 0x04, 0x00}
	suite.SetCartGlyphs(map[rune]hardware.GlyphRows{'♥': redrawn})
	bus.reset()
	sc.setText("♥", "")

	got := pairBytes(t, decodeNibbles(t, bus.forAddr(0x27)))
	if got[0].rs || got[0].b&0xf8 != lcdCmdSetCGRAM {
		t.Fatalf("shadowing a shipped rune must re-upload its slot, first write %#x", got[0].b)
	}
	for i, row := range redrawn {
		if got[i+1].b != row {
			t.Errorf("re-uploaded row %d = %#x, want %#x (the stale shipped bitmap is still in the slot)", i, got[i+1].b, row)
		}
	}
}

func TestPreviewGlyphOutranksACartGlyph(t *testing.T) {
	suite, sc, bus := newPreviewScreen(t)

	const r = '★'
	cart := hardware.GlyphRows{0x04, 0x0e, 0x1f, 0x0e, 0x1b, 0x11, 0x00, 0x00}
	preview := hardware.GlyphRows{0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f}
	suite.SetCartGlyphs(map[rune]hardware.GlyphRows{r: cart})
	suite.SetPreviewGlyphs(map[rune]hardware.GlyphRows{r: preview})

	bus.reset()
	sc.setText(string(r), "")

	got := pairBytes(t, decodeNibbles(t, bus.forAddr(0x27)))
	for i, row := range preview {
		if got[i+1].b != row {
			t.Fatalf("row %d = %#x, want the PREVIEW's %#x — the cart overlay outranked the preview", i, got[i+1].b, row)
		}
	}
}

func TestCartGlyphsClearBackToTheEngineTable(t *testing.T) {
	suite, sc, bus := newPreviewScreen(t)

	const basket = '֍'
	suite.SetCartGlyphs(map[rune]hardware.GlyphRows{basket: {0x1f, 0, 0, 0, 0, 0, 0, 0}})
	sc.setText(string(basket), "")

	suite.SetCartGlyphs(nil)
	bus.reset()
	sc.setText(string(basket), "")

	if data := decodeData(t, bus.forAddr(0x27)); !containsByte(data, '?') {
		t.Errorf("after teardown the rune should be unknown again, got %q", data)
	}
}

func TestCartGlyphsShareTheEightSlotBudget(t *testing.T) {
	suite, sc, _ := newPreviewScreen(t)

	table := map[rune]hardware.GlyphRows{}
	line := ""
	for i := 0; i < maxCGRAMSlots; i++ {
		r := rune(0xE000 + i) // private use area: definitely not in customChars
		table[r] = hardware.GlyphRows{byte(i + 1), 0, 0, 0, 0, 0, 0, 0}
		line += string(r)
	}
	suite.SetCartGlyphs(table)
	sc.setText(line+"♥", "")

	if _, ok := sc.cache.get('♥'); ok {
		t.Error("the 9th distinct custom rune got a CGRAM slot — the budget is 8 per screen, shared")
	}
	if n := len(sc.cache.slots); n != maxCGRAMSlots {
		t.Errorf("cache holds %d slots, want %d", n, maxCGRAMSlots)
	}
}

func TestGlyphFileFromABundleReachesCGRAM(t *testing.T) {
	suite, sc, bus := newPreviewScreen(t)

	const coin = '¤' // no engine table has ever heard of it
	if _, ok := customChars[coin]; ok {
		t.Fatal("this test needs a rune the shipped table does not define")
	}
	file := []byte(`{"arkade":1,"kind":"glyph","name":"coin",` +
		`"generator":{"tool":"editor","version":"1.0.0"},` +
		`"codepoint":164,"rows":[14,17,21,21,21,17,14,0],"category":"tetromino"}`)
	env, err := assetfile.ReadBytes("coin.arkadeglyph.json", file)
	if err != nil {
		t.Fatalf("reading the glyph file: %v", err)
	}
	if rune(env.Codepoint) != coin {
		t.Fatalf("fixture code point %d is not %q", env.Codepoint, coin)
	}

	sc.setText(string(coin), "")
	if data := decodeData(t, bus.forAddr(0x27)); len(data) == 0 || !containsByte(data, '?') {
		t.Fatalf("before the bundle is read the rune should print as '?', got %q", data)
	}

	var rows hardware.GlyphRows
	for i, v := range env.Rows {
		rows[i] = byte(v)
	}
	suite.SetCartGlyphs(map[rune]hardware.GlyphRows{coin: rows})
	bus.reset()
	sc.setText(string(coin), "")

	got := pairBytes(t, decodeNibbles(t, bus.forAddr(0x27)))
	if got[0].rs || got[0].b != lcdCmdSetCGRAM {
		t.Fatalf("a bundled glyph should upload to CGRAM slot 0, first write %#x", got[0].b)
	}
	for i, row := range rows {
		if got[i+1].b != row {
			t.Errorf("slot 0 row %d = %#x, want %#x (the file's own bytes)", i, got[i+1].b, row)
		}
	}
	if data := decodeData(t, bus.forAddr(0x27)); containsByte(data, '?') {
		t.Errorf("printed bytes still contain '?': %q", data)
	}
}
