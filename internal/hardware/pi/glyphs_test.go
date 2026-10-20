// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"bytes"
	"math/bits"
	"strings"
	"testing"
)

func TestCustomCharsCompleteAndFiveBitsWide(t *testing.T) {
	if len(customChars) != 34 {
		t.Errorf("expected the 32 base glyphs + 7 Årkade additions (the █ ▓ ▒ ░ shade ramp + ♠ ♦ ♣) + 6 die faces (⚀⚁⚂⚃⚄⚅) + the ⏶ column marker and its two hovered forms ☑ ☒, got %d", len(customChars))
	}
	for r, pattern := range customChars {
		for row, bits := range pattern {
			if bits > 0x1f {
				t.Errorf("glyph %q row %d uses more than 5 pixels: %#x", r, row, bits)
			}
		}
	}
}

// Fixed thresholds, so a rung cannot drift toward its neighbour while still climbing.
func TestShadeRampDensityClimbs(t *testing.T) {
	ramp := []struct {
		r        rune
		min, max int // lit pixels out of 40, inclusive
	}{
		{'░', 6, 12},  // light  ~20%
		{'▒', 18, 22}, // medium ~50%
		{'▓', 28, 34}, // dark   ~80%
		{'█', 40, 40}, // full   100%
	}
	prev := -1
	for _, step := range ramp {
		pattern, ok := customChars[step.r]
		if !ok {
			t.Fatalf("shade %q missing from customChars", step.r)
		}
		n := 0
		for _, row := range pattern {
			n += bits.OnesCount8(row)
		}
		if n < step.min || n > step.max {
			t.Errorf("shade %q lit %d/40 pixels, want %d..%d", step.r, n, step.min, step.max)
		}
		if n <= prev {
			t.Errorf("shade %q lit %d/40, not denser than the previous rung (%d/40) -- the ramp must climb", step.r, n, prev)
		}
		prev = n
	}
}

func TestDieFaceGlyphsPipCountsAndGaps(t *testing.T) {
	faces := []struct {
		r    rune
		pips int
	}{
		{'⚀', 1}, {'⚁', 2}, {'⚂', 3}, {'⚃', 4}, {'⚄', 5}, {'⚅', 6},
	}
	for _, f := range faces {
		pattern, ok := customChars[f.r]
		if !ok {
			t.Fatalf("die face %q missing from customChars", f.r)
		}
		n := 0
		for _, row := range pattern {
			n += bits.OnesCount8(row)
		}
		if n != f.pips {
			t.Errorf("die face %q lit %d pixels, want %d pips", f.r, n, f.pips)
		}
		for row := 0; row < 8; row++ {
			if pattern[row] == 0 {
				continue
			}
			if row > 0 && pattern[row-1] != 0 {
				t.Errorf("die face %q rows %d and %d are both lit -- no vertical gap", f.r, row-1, row)
			}
			if row < 7 && pattern[row+1] != 0 {
				t.Errorf("die face %q rows %d and %d are both lit -- no vertical gap", f.r, row, row+1)
			}
			if pattern[row]&(pattern[row]<<1) != 0 {
				t.Errorf("die face %q row %d has horizontally adjacent pips: %#x", f.r, row, pattern[row])
			}
		}
	}
}

func TestDieFaceSixIsTwoCleanColumns(t *testing.T) {
	pattern := customChars['⚅']
	leftRows, rightRows := 0, 0
	for _, row := range pattern {
		if row&0x04 != 0 {
			t.Fatalf("six's center column must stay dark, row pattern %#x", row)
		}
		if row&0x10 != 0 {
			leftRows++
		}
		if row&0x01 != 0 {
			rightRows++
		}
	}
	if leftRows != 3 || rightRows != 3 {
		t.Fatalf("six's pip columns = left:%d right:%d, want 3 and 3", leftRows, rightRows)
	}
}

func TestColumnMarkerHugsTopOfCell(t *testing.T) {
	pattern, ok := customChars['⏶']
	if !ok {
		t.Fatal("⏶ missing from customChars")
	}
	for row := 3; row < 8; row++ {
		if pattern[row] != 0 {
			t.Errorf("⏶ row %d = %#x, want dark -- the marker must sit in rows 0-2", row, pattern[row])
		}
	}
	if bits.OnesCount8(pattern[0]) >= bits.OnesCount8(pattern[1]) ||
		bits.OnesCount8(pattern[1]) >= bits.OnesCount8(pattern[2]) {
		t.Errorf("⏶ rows %#x must widen from tip to base", pattern[:3])
	}
	if pattern[0] != 0x04 {
		t.Errorf("⏶ tip = %#x, want the center column 0x04", pattern[0])
	}
}

func TestHoveredMarksAreTheCursorCaretOverTheirBaseMark(t *testing.T) {
	caret := customChars['⏶']
	for _, c := range []struct{ hovered, base rune }{{'☑', '✔'}, {'☒', '✖'}} {
		got, ok := customChars[c.hovered]
		if !ok {
			t.Fatalf("%q missing from customChars", c.hovered)
		}
		if got[0] != caret[0] || got[1] != caret[1] {
			t.Errorf("%q rows 0-1 = %#x %#x, want ⏶'s tip and shoulder %#x %#x", c.hovered, got[0], got[1], caret[0], caret[1])
		}
		if got[2] != 0 {
			t.Errorf("%q row 2 = %#x, want a dark row separating the caret from the mark", c.hovered, got[2])
		}
		base := customChars[c.base]
		for i := 0; i < 5; i++ {
			if got[3+i] != base[1+i] {
				t.Errorf("%q row %d = %#x, want %q's row %d (%#x)", c.hovered, 3+i, got[3+i], c.base, 1+i, base[1+i])
			}
		}
		if got == customChars[c.base] || got == caret {
			t.Errorf("%q must be distinguishable from both %q and ⏶", c.hovered, c.base)
		}
	}
}

func TestLRUAssignsAndRecyclesSlots(t *testing.T) {
	c := newLRUCache(3)
	if c.insert('a') != 0 || c.insert('b') != 1 || c.insert('c') != 2 {
		t.Fatal("slots should fill in order 0,1,2")
	}
	if v, ok := c.get('a'); !ok || v != 0 {
		t.Fatal("get should return the cached slot")
	}
	if got := c.insert('d'); got != 1 {
		t.Errorf("full cache should recycle the LRU slot (b=1), got %d", got)
	}
	if c.has('b') {
		t.Error("evicted key should be gone")
	}
	if v, _ := c.get('a'); v != 0 {
		t.Error("refreshed key should survive eviction")
	}
	if got := c.insert('c'); got != 2 {
		t.Errorf("existing key keeps its slot, got %d", got)
	}
}

func TestGlyphUploadAndSubstitution(t *testing.T) {
	dev, bus := newTestLCD(t)
	sc := newScreen(dev, nil, func(string, string) {})
	bus.reset()

	sc.setText("♥ up ↑", "")
	got := pairBytes(t, decodeNibbles(t, bus.forAddr(0x27)))

	if got[0].rs || got[0].b != lcdCmdSetCGRAM {
		t.Fatalf("first write should address CGRAM slot 0, got %#x", got[0].b)
	}
	for i, row := range customChars['♥'] {
		if got[i+1].b != row {
			t.Fatalf("slot 0 pattern row %d = %#x, want %#x", i, got[i+1].b, row)
		}
	}
	if got[9].rs || got[9].b != lcdCmdSetCGRAM|1<<3 {
		t.Fatalf("second upload should address slot 1, got %#x", got[9].b)
	}

	var data []byte
	for _, hb := range got[18:] { // skip both uploads (2 × 9 bytes)
		if hb.rs {
			data = append(data, hb.b)
		}
	}
	want := []byte(strings.Repeat(" ", 5) + "\x00 up \x01" + strings.Repeat(" ", 5))
	if len(data) < lcdCols || !bytes.Equal(data[:lcdCols], want) {
		t.Errorf("printed bytes = %q, want prefix %q", data, want)
	}
}

func TestDieFaceLineUsesSixOfEightSlotsWithRoomToSpare(t *testing.T) {
	dev, bus := newTestLCD(t)
	sc := newScreen(dev, nil, func(string, string) {})
	bus.reset()

	line := "⚀⚁⚂⚃⚄⚅" // all six faces, space-free (worst case density)
	sc.setText(line, "")

	order := []rune{'⚀', '⚁', '⚂', '⚃', '⚄', '⚅'}
	for i, r := range order {
		slot, ok := sc.cache.get(r)
		if !ok {
			t.Fatalf("die face %q was never paged into a CGRAM slot", r)
		}
		if slot != i {
			t.Errorf("die face %q landed in slot %d, want %d (upload order)", r, slot, i)
		}
	}
	if len(sc.cache.order) != 6 {
		t.Fatalf("cache holds %d entries after a 6-distinct-glyph line, want exactly 6 (2 slots left free)", len(sc.cache.order))
	}

	got := pairBytes(t, decodeNibbles(t, bus.forAddr(0x27)))
	for slot, r := range order {
		base := slot * 9
		if got[base].rs || got[base].b != lcdCmdSetCGRAM|byte(slot)<<3 {
			t.Fatalf("upload %d should address CGRAM slot %d, got %#x", slot, slot, got[base].b)
		}
		for row, want := range customChars[r] {
			if got[base+1+row].b != want {
				t.Fatalf("slot %d (%q) pattern row %d = %#x, want %#x", slot, r, row, got[base+1+row].b, want)
			}
		}
	}

	var data []byte
	for _, hb := range got[6*9:] {
		if hb.rs {
			data = append(data, hb.b)
		}
	}
	want := []byte(strings.Repeat(" ", 5) + "\x00\x01\x02\x03\x04\x05" + strings.Repeat(" ", 5))
	if len(data) < lcdCols || !bytes.Equal(data[:lcdCols], want) {
		t.Errorf("printed bytes = %v, want prefix %v", data, want)
	}
}

func distinctCustoms(lines ...string) int {
	seen := map[rune]bool{}
	for _, ln := range lines {
		for _, r := range ln {
			if _, ok := customChars[r]; ok {
				seen[r] = true
			}
		}
	}
	return len(seen)
}

func TestCribbageDiscardScreenFitsTheSlotBudget(t *testing.T) {
	dev, _ := newTestLCD(t)
	sc := newScreen(dev, nil, func(string, string) {})

	const hand = "A♠7♥T♦J♣Q♠K♥" // all four suits at once
	for _, line2 := range []string{
		"  ⏶             ", // fresh: cursor on card 0, nothing picked
		"  ✔   ⏶     ✔ OK", // two picks in, cursor on a third card
		"  ✔         ☑ OK", // cursor moved ONTO a picked card
	} {
		sc.setText(hand, line2)
		if n := distinctCustoms(hand, line2); n > 6 {
			t.Errorf("%q over %q needs %d CGRAM slots at once, want at most 6", line2, hand, n)
		}
	}

	for _, r := range []rune{'♠', '♥', '♦', '♣', '⏶', '✔', '☑'} {
		if !sc.cache.has(r) {
			t.Errorf("%q was evicted during the selection -- it will be re-uploaded on the next cursor move", r)
		}
	}
	if n := len(sc.cache.order); n != 7 {
		t.Fatalf("discard screen holds %d CGRAM entries across a whole selection, want 7 of %d (1 spare, no eviction)", n, maxCGRAMSlots)
	}
}

func TestCribbagePeggingScreenFitsTheSlotBudget(t *testing.T) {
	dev, _ := newTestLCD(t)
	sc := newScreen(dev, nil, func(string, string) {})

	const hand = "K♠7♥T♦9♣" // four cards, all four suits, 8 columns
	for _, line2 := range []string{
		"    ⏶   ✖ ✖ PLAY", // cursor on a playable card, two over 31
		"        ☒ ✖ >31 ", // cursor moved onto one that busts
	} {
		sc.setText(hand, line2)
		if n := distinctCustoms(hand, line2); n > 6 {
			t.Errorf("%q over %q needs %d CGRAM slots at once, want at most 6", line2, hand, n)
		}
	}

	for _, r := range []rune{'♠', '♥', '♦', '♣', '⏶', '✖', '☒'} {
		if !sc.cache.has(r) {
			t.Errorf("%q was evicted during the play -- it will be re-uploaded on the next cursor move", r)
		}
	}
	if n := len(sc.cache.order); n != 7 {
		t.Fatalf("pegging screen holds %d CGRAM entries across a whole hand, want 7 of %d (1 spare, no eviction)", n, maxCGRAMSlots)
	}
}

func TestGlyphCacheReusesSlotWithoutReupload(t *testing.T) {
	dev, bus := newTestLCD(t)
	sc := newScreen(dev, nil, func(string, string) {})
	sc.setText("♥", "")
	bus.reset()

	sc.setText("♥♥", "") // same glyph, different text
	for _, hb := range pairBytes(t, decodeNibbles(t, bus.forAddr(0x27))) {
		if !hb.rs && hb.b >= lcdCmdSetCGRAM && hb.b < lcdCmdSetDDRAMRow0 {
			t.Fatalf("cached glyph should not re-upload CGRAM, saw %#x", hb.b)
		}
	}
}

func TestGlyphLRUEvictsAfterEightRunes(t *testing.T) {
	dev, bus := newTestLCD(t)
	sc := newScreen(dev, nil, func(string, string) {})

	sc.setText("Å©♡♥●✔✖♪", "")
	if got := sc.cache.slots['Å']; got != 0 {
		t.Fatalf("first glyph should hold slot 0, got %d", got)
	}

	bus.reset()
	sc.setText("→", "")
	got := pairBytes(t, decodeNibbles(t, bus.forAddr(0x27)))
	if got[0].rs || got[0].b != lcdCmdSetCGRAM {
		t.Fatalf("ninth glyph should recycle slot 0, first write %#x", got[0].b)
	}
	for i, row := range customChars['→'] {
		if got[i+1].b != row {
			t.Fatalf("slot 0 should hold →'s pattern, row %d = %#x", i, got[i+1].b)
		}
	}
	if sc.cache.has('Å') {
		t.Error("Å should be evicted")
	}

	bus.reset()
	sc.setText("Å", "")
	got = pairBytes(t, decodeNibbles(t, bus.forAddr(0x27)))
	if got[0].rs || got[0].b != lcdCmdSetCGRAM|1<<3 {
		t.Fatalf("Å should recycle slot 1, first write %#x", got[0].b)
	}
	if sc.cache.has('©') {
		t.Error("© should be evicted")
	}
}

func TestGlyphMetaCoversEveryGlyph(t *testing.T) {
	for r := range customChars {
		info, ok := glyphMeta[r]
		if !ok {
			t.Errorf("%q has a bitmap but no glyphMeta entry — name, category and description are part of shipping a glyph", r)
			continue
		}
		if info.name == "" || info.category == "" || info.description == "" {
			t.Errorf("%q's metadata is incomplete: %+v", r, info)
		}
	}
	for r := range glyphMeta {
		if _, ok := customChars[r]; !ok {
			t.Errorf("glyphMeta describes %q, which customChars does not define", r)
		}
	}
}

func TestGlyphMetaNamesAreUniqueAndCategoriesAreKnown(t *testing.T) {
	known := map[string]bool{
		catArrows: true, catShades: true, catSuits: true,
		catDice: true, catMarks: true, catSymbols: true,
	}
	seen := map[string]rune{}
	for r, info := range glyphMeta {
		if prev, dup := seen[info.name]; dup {
			t.Errorf("%q and %q are both named %q", prev, r, info.name)
		}
		seen[info.name] = r
		if !known[info.category] {
			t.Errorf("%q is in category %q, which is not one of the six", r, info.category)
		}
	}
}
