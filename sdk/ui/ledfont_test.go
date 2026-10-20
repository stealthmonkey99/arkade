// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

func TestLEDGlyphWidths(t *testing.T) {
	f := FontLED{}
	cases := []struct {
		r    rune
		want int
	}{
		{' ', 2},
		{'!', 2},
		{'0', 4},
		{'#', 7},
		{'M', 6},
		{'|', 1},
		{'☃', 0},
	}
	for _, c := range cases {
		if got := f.GlyphWidth(c.r, 1); got != c.want {
			t.Errorf("GlyphWidth(%q,1) = %d, want %d", c.r, got, c.want)
		}
		if got := f.GlyphWidth(c.r, 2); got != c.want*2 {
			t.Errorf("GlyphWidth(%q,2) = %d, want %d", c.r, got, c.want*2)
		}
	}
}

func TestLEDDigitsShareOneWidthForTheFixedStrideNumerals(t *testing.T) {
	want := FontLED{}.glyphWidth('0')
	for d := '1'; d <= '9'; d++ {
		if got := (FontLED{}).glyphWidth(d); got != want {
			t.Errorf("glyph %q is %d wide, want %d: drawLEDUintClipped and ledUintWidth place digits on a fixed stride", d, got, want)
		}
	}
}

func TestLEDTextWidth(t *testing.T) {
	f := FontLED{}
	if w := f.TextWidth("", 1); w != 0 {
		t.Errorf("TextWidth(\"\") = %d, want 0", w)
	}
	// "HI": H is 4px wide, I is 2px wide (no inter-glyph gap: the font bakes
	// its own spacing into each glyph's trailing blank column).
	if w := f.TextWidth("HI", 1); w != 6 {
		t.Errorf(`TextWidth("HI") = %d, want 6`, w)
	}
	if w := f.TextWidth("HI", 3); w != 18 {
		t.Errorf(`TextWidth("HI",3) = %d, want 18`, w)
	}
	if w := f.TextWidth("H☃I", 1); w != 6 {
		t.Errorf(`TextWidth("H☃I") = %d, want 6 (unmapped rune skipped)`, w)
	}
}

func TestLEDHeight(t *testing.T) {
	f := FontLED{}
	if h := f.Height(0); h != 8 {
		t.Errorf("Height(0) = %d, want 8 (scale 0 means 1)", h)
	}
	if h := f.Height(3); h != 24 {
		t.Errorf("Height(3) = %d, want 24", h)
	}
}

func TestLEDDrawTextLightsExactlyTheGlyphsBits(t *testing.T) {
	cv := newGridCanvas(8, 8)
	FontLED{}.DrawText(cv, 0, 0, "0", 1, White)

	bits := LEDGlyphs['0']
	width := len(bits) / ledGlyphRows
	wantLit := 0
	for _, b := range bits {
		if b == 1 {
			wantLit++
		}
	}

	gotLit := 0
	for y := 0; y < ledGlyphRows; y++ {
		for x := 0; x < width; x++ {
			if cv.at(x, y) == White {
				gotLit++
			}
		}
	}
	if gotLit != wantLit {
		t.Errorf("lit pixels = %d, want %d", gotLit, wantLit)
	}
	if cv.at(0, 0) != Black {
		t.Errorf("glyph '0' top-left pixel should be unlit, got %v", cv.at(0, 0))
	}
}
