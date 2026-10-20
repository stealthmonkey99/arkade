// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

func newPreviewScreen(t *testing.T) (*Suite, *screen, *fakeBus) {
	t.Helper()
	dev, bus := newTestLCD(t)
	glyphs := &glyphTable{}
	sc := newScreen(dev, glyphs, func(string, string) {})
	return &Suite{logger: func(string, string) {}, glyphs: glyphs, screens: [2]*screen{sc, nil}}, sc, bus
}

func TestPreviewGlyphDefinesANewRune(t *testing.T) {
	suite, sc, bus := newPreviewScreen(t)

	// '★' is deliberately absent from customChars
	if _, ok := customChars['★']; ok {
		t.Fatal("this test needs a rune the shipped table does not define")
	}
	sc.setText("★", "")
	if data := decodeData(t, bus.forAddr(0x27)); len(data) == 0 || !containsByte(data, '?') {
		t.Fatalf("undefined rune should print as '?', got %q", data)
	}

	star := hardware.GlyphRows{0x04, 0x0e, 0x1f, 0x0e, 0x1b, 0x11, 0x00, 0x00}
	suite.SetPreviewGlyphs(map[rune]hardware.GlyphRows{'★': star})
	bus.reset()
	sc.setText("★", "")

	got := pairBytes(t, decodeNibbles(t, bus.forAddr(0x27)))
	if got[0].rs || got[0].b != lcdCmdSetCGRAM {
		t.Fatalf("preview glyph should upload to CGRAM slot 0, first write %#x", got[0].b)
	}
	for i, row := range star {
		if got[i+1].b != row {
			t.Errorf("slot 0 row %d = %#x, want %#x", i, got[i+1].b, row)
		}
	}
	if data := decodeData(t, bus.forAddr(0x27)); containsByte(data, '?') {
		t.Errorf("printed bytes still contain '?': %q", data)
	}
}

func TestPreviewGlyphShadowsAShippedOne(t *testing.T) {
	suite, sc, bus := newPreviewScreen(t)

	sc.setText("♥", "")

	redesign := hardware.GlyphRows{0x1f, 0x11, 0x11, 0x11, 0x0a, 0x0a, 0x04, 0x00}
	suite.SetPreviewGlyphs(map[rune]hardware.GlyphRows{'♥': redesign})
	bus.reset()
	sc.setText("♥", "") // the SAME text: the redraw must happen anyway

	got := pairBytes(t, decodeNibbles(t, bus.forAddr(0x27)))
	if len(got) == 0 || got[0].rs || got[0].b != lcdCmdSetCGRAM {
		t.Fatalf("a redefined glyph must be re-uploaded even for unchanged text; writes = %v", got)
	}
	for i, row := range redesign {
		if got[i+1].b != row {
			t.Errorf("slot 0 row %d = %#x, want the redesign's %#x", i, got[i+1].b, row)
		}
	}
}

func TestPreviewGlyphsClearedRestoresShippedTable(t *testing.T) {
	suite, sc, bus := newPreviewScreen(t)

	redesign := hardware.GlyphRows{0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f}
	suite.SetPreviewGlyphs(map[rune]hardware.GlyphRows{'♥': redesign})
	sc.setText("♥", "")

	suite.SetPreviewGlyphs(nil)
	bus.reset()
	sc.setText("♥", "")

	got := pairBytes(t, decodeNibbles(t, bus.forAddr(0x27)))
	if len(got) == 0 || got[0].b != lcdCmdSetCGRAM {
		t.Fatalf("clearing the overlay must force a re-upload; writes = %v", got)
	}
	for i, row := range customChars['♥'] {
		if got[i+1].b != row {
			t.Errorf("slot 0 row %d = %#x, want the shipped %#x", i, got[i+1].b, row)
		}
	}
}

func TestPreviewGlyphResetRecyclesSlotsFromZero(t *testing.T) {
	suite, sc, _ := newPreviewScreen(t)

	sc.setText("Å©♡", "") // slots 0,1,2
	if got := sc.cache.slots['♡']; got != 2 {
		t.Fatalf("♡ should hold slot 2, got %d", got)
	}
	suite.SetPreviewGlyphs(map[rune]hardware.GlyphRows{'★': {}})
	sc.setText("♡", "")
	if n := len(sc.cache.order); n != 1 {
		t.Fatalf("slot map should hold only ♡ after a glyph reset, holds %d", n)
	}
	if got := sc.cache.slots['♡']; got != 0 {
		t.Errorf("after a reset the first glyph should land in slot 0, got %d", got)
	}
}

func containsByte(b []byte, want byte) bool {
	for _, v := range b {
		if v == want {
			return true
		}
	}
	return false
}
