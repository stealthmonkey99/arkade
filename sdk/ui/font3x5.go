// SPDX-License-Identifier: GPL-2.0-or-later

package ui

const (
	glyph3x5W = 3
	glyph3x5H = 5
	// glyph3x5Advance is the pen advance per character at scale 1: the glyph
	// plus a 1px gap.
	glyph3x5Advance = glyph3x5W + 1
)

// Font3x5 is the 3×5 pixel font (A-Z, 0-9, space, dash, colon, question
// mark, apostrophe, period, bang). It is stateless; a zero value is usable.
type Font3x5 struct{}

// glyph3x5 returns the packed rows for c, or 0 (blank) for anything the font
// doesn't cover (including space). Row 0 sits in bits 14..12 with bit 14
// leftmost, down to row 4 in bits 2..0.
func glyph3x5(c byte) uint16 {
	switch c {
	case '0':
		return 0b111_101_101_101_111
	case '1':
		return 0b010_110_010_010_111
	case '2':
		return 0b111_001_111_100_111
	case '3':
		return 0b111_001_011_001_111
	case '4':
		return 0b101_101_111_001_001
	case '5':
		return 0b111_100_111_001_111
	case '6':
		return 0b111_100_111_101_111
	case '7':
		return 0b111_001_001_010_010
	case '8':
		return 0b111_101_111_101_111
	case '9':
		return 0b111_101_111_001_111
	case 'A':
		return 0b010_101_111_101_101
	case 'B':
		return 0b110_101_110_101_110
	case 'C':
		return 0b011_100_100_100_011
	case 'D':
		return 0b110_101_101_101_110
	case 'E':
		return 0b111_100_111_100_111
	case 'F':
		return 0b111_100_111_100_100
	case 'G':
		return 0b011_100_101_101_011
	case 'H':
		return 0b101_101_111_101_101
	case 'I':
		return 0b111_010_010_010_111
	case 'J':
		return 0b001_001_001_101_010
	case 'K':
		return 0b101_110_100_110_101
	case 'L':
		return 0b100_100_100_100_111
	case 'M':
		return 0b101_111_111_101_101
	case 'N':
		return 0b110_101_101_101_101
	case 'O':
		return 0b111_101_101_101_111
	case 'P':
		return 0b111_101_111_100_100
	case 'Q':
		return 0b111_101_101_111_001
	case 'R':
		return 0b111_101_110_101_101
	case 'S':
		return 0b011_100_010_001_110
	case 'T':
		return 0b111_010_010_010_010
	case 'U':
		return 0b101_101_101_101_111
	case 'V':
		return 0b101_101_101_101_010
	case 'W':
		return 0b101_101_111_111_101
	case 'X':
		return 0b101_101_010_101_101
	case 'Y':
		return 0b101_101_010_010_010
	case 'Z':
		return 0b111_001_010_100_111
	case '-':
		return 0b000_000_111_000_000
	case ':':
		return 0b000_010_000_010_000
	case '?':
		return 0b111_001_010_000_010
	case '\'': // apostrophe, so "X'S TURN" has no silent gap
		return 0b010_010_000_000_000
	case '.':
		return 0b000_000_000_000_010
	case '!':
		return 0b010_010_010_000_010
	default: // covers ' ' and anything unmapped
		return 0
	}
}

// drawGlyph rasterizes one glyph at (x, y) onto cv, scaled by scale.
func (Font3x5) drawGlyph(cv Canvas, x, y int, bits uint16, scale int, c Color) {
	for row := 0; row < glyph3x5H; row++ {
		rowBits := (bits >> uint(12-3*row)) & 0b111
		for col := 0; col < glyph3x5W; col++ {
			if rowBits&(0b100>>uint(col)) == 0 {
				continue
			}
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					cv.Blend(x+col*scale+sx, y+row*scale+sy, c)
				}
			}
		}
	}
}

// DrawText draws s onto cv with its top-left at (x, y), one glyph per byte.
// Lowercase letters and any byte the font lacks draw blank but still advance.
// It does not allocate.
func (f Font3x5) DrawText(cv Canvas, x, y int, s string, scale int, c Color) {
	for i := 0; i < len(s); i++ {
		f.drawGlyph(cv, x+i*glyph3x5Advance*scale, y, glyph3x5(s[i]), scale, c)
	}
}

// TextWidth is DrawText's rendered width for n characters at scale: n
// glyph-advances minus the final (scaled) 1px gap.
func (Font3x5) TextWidth(n, scale int) int {
	if n <= 0 {
		return 0
	}
	return n*glyph3x5Advance*scale - scale
}

// MeasureText is TextWidth for the characters of s, so Font3x5 satisfies
// LabelFont. It counts bytes, as DrawText does.
func (f Font3x5) MeasureText(s string, scale int) int { return f.TextWidth(len(s), scale) }

// Advance is the pen step from one character's left edge to the next at
// scale: the glyph width plus a scaled 1px gap. A caller drawing one
// character at a time steps by it to match DrawText.
func (Font3x5) Advance(scale int) int { return glyph3x5Advance * scale }

// Height is one line's pixel height at scale.
func (Font3x5) Height(scale int) int { return glyph3x5H * scale }

// NumDigits returns how many decimal characters DrawUint(v) draws.
func (Font3x5) NumDigits(v uint32) int {
	n := 1
	for v >= 10 {
		v /= 10
		n++
	}
	return n
}

// DrawUint draws v in decimal onto cv with its top-left at (x, y). It does
// not allocate.
func (f Font3x5) DrawUint(cv Canvas, x, y int, v uint32, scale int, c Color) {
	d := f.NumDigits(v)
	for i := d - 1; i >= 0; i-- {
		f.drawGlyph(cv, x+i*glyph3x5Advance*scale, y, glyph3x5(byte('0'+v%10)), scale, c)
		v /= 10
	}
}
