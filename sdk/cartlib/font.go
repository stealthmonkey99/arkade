// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

// Each 3x5 glyph packs into a uint16: bits 14..12 are row 0 (bit 14 the
// leftmost pixel), down to bits 2..0 for row 4.

const (
	glyphW       = 3
	glyphH       = 5
	glyphAdvance = glyphW + 1
)

// glyphBits returns the packed rows for c, or 0 (blank) for anything the
// font lacks.
func glyphBits(c byte) uint16 {
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
	case '?':
		return 0b111_001_010_000_010
	default:
		return 0
	}
}

func drawGlyph(fb *Framebuffer, x, y int, bits uint16, scale int, r, g, b byte) {
	for row := 0; row < glyphH; row++ {
		rowBits := (bits >> uint(12-3*row)) & 0b111
		for col := 0; col < glyphW; col++ {
			if rowBits&(0b100>>uint(col)) == 0 {
				continue
			}
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					fb.SetPixel(x+col*scale+sx, y+row*scale+sy, r, g, b)
				}
			}
		}
	}
}

// DrawText draws s in cartlib's 3x5 font into fb, top-left at (x, y), each
// pixel scaled to scale x scale, opaque in (r, g, b). The font covers
// uppercase A-Z, 0-9, '-' and '?'; any other byte, lowercase included,
// draws as a blank cell. Pixels outside the frame are clipped.
// Allocation-free.
func DrawText(fb *Framebuffer, x, y int, s string, scale int, r, g, b byte) {
	for i := 0; i < len(s); i++ {
		drawGlyph(fb, x+i*glyphAdvance*scale, y, glyphBits(s[i]), scale, r, g, b)
	}
}

// TextWidth returns the width in pixels that [DrawText] covers for n
// characters at scale: 4*n*scale less the trailing gap, 0 for n == 0.
func TextWidth(n, scale int) int {
	if n == 0 {
		return 0
	}
	return n*glyphAdvance*scale - scale
}

// NumDigits returns the number of decimal digits in v, which is how many
// characters [DrawUint] draws.
func NumDigits(v uint32) int {
	n := 1
	for v >= 10 {
		v /= 10
		n++
	}
	return n
}

// AppendUint writes v in decimal into buf starting at pos and returns the
// index just past the last digit written, for building [SetTextBytes] lines
// in a package-level buffer without allocating. When fewer than
// [NumDigits](v) bytes remain it keeps v's lowest digits that fit, so 12345
// written with two bytes left is "45"; with none left, or pos past the end,
// it writes nothing and returns pos. pos must not be negative.
// Allocation-free; callable in any phase.
func AppendUint(buf []byte, pos int, v uint32) int {
	d := NumDigits(v)
	if pos+d > len(buf) {
		d = len(buf) - pos
		if d <= 0 {
			return pos
		}
	}
	div := uint32(1)
	for i := 1; i < d; i++ {
		div *= 10
	}
	for i := 0; i < d; i++ {
		buf[pos+i] = byte('0' + (v/div)%10)
		div /= 10
	}
	return pos + d
}

// DrawUint draws v in decimal into fb as [DrawText] would draw its digits.
// Allocation-free.
func DrawUint(fb *Framebuffer, x, y int, v uint32, scale int, r, g, b byte) {
	d := NumDigits(v)
	for i := d - 1; i >= 0; i-- {
		drawGlyph(fb, x+i*glyphAdvance*scale, y, glyphBits(byte('0'+v%10)), scale, r, g, b)
		v /= 10
	}
}
