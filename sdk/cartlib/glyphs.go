// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

// MaxCartGlyphs is how many distinct runes one cartridge may define. It caps
// the table, not what one screen can show; see [Setup.DefineLCDGlyph] for
// the eight-glyph screen budget.
const MaxCartGlyphs = 32

// GlyphRows is one 5x8 LCD glyph: eight pixel rows, top to bottom. Only the
// low 5 bits of each row are used, and bit 4 is the leftmost column, so
// 0x10 lights the left pixel and 0x01 the right one; this is the HD44780
// CGRAM encoding. A heart:
//
//	cartlib.GlyphRows{0x0, 0x0, 0xa, 0x1f, 0x1f, 0xe, 0x4, 0x0}
type GlyphRows = [8]byte

// packGlyphRows folds the rows into the ABI's single u64, row 0 in the low
// byte.
func packGlyphRows(rows GlyphRows) uint64 {
	var packed uint64
	for i := 0; i < len(rows); i++ {
		packed |= uint64(rows[i]&0x1f) << (8 * uint(i))
	}
	return packed
}
