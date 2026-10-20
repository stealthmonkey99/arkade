// Fixture: a cartridge that defines one distinct rune more than
// sdk/cartlib's MaxCartGlyphs (32) allows. The runtime accepts the
// first 32 distinct definitions and rejects every new rune after —
// check_lcd.go must say so before the cabinet does.
package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

var rows = cartlib.GlyphRows{0x0, 0xe, 0x11, 0x1f, 0x15, 0x1f, 0xe, 0x0}

// 33 distinct runes, one over the cap. All but the last are accepted.
func setup() {
	cartlib.DefineLCDGlyph('¡', rows)
	cartlib.DefineLCDGlyph('¢', rows)
	cartlib.DefineLCDGlyph('£', rows)
	cartlib.DefineLCDGlyph('¤', rows)
	cartlib.DefineLCDGlyph('¥', rows)
	cartlib.DefineLCDGlyph('¦', rows)
	cartlib.DefineLCDGlyph('§', rows)
	cartlib.DefineLCDGlyph('¨', rows)
	cartlib.DefineLCDGlyph('©', rows)
	cartlib.DefineLCDGlyph('ª', rows)
	cartlib.DefineLCDGlyph('«', rows)
	cartlib.DefineLCDGlyph('¬', rows)
	cartlib.DefineLCDGlyph('­', rows)
	cartlib.DefineLCDGlyph('®', rows)
	cartlib.DefineLCDGlyph('¯', rows)
	cartlib.DefineLCDGlyph('°', rows)
	cartlib.DefineLCDGlyph('±', rows)
	cartlib.DefineLCDGlyph('²', rows)
	cartlib.DefineLCDGlyph('³', rows)
	cartlib.DefineLCDGlyph('´', rows)
	cartlib.DefineLCDGlyph('µ', rows)
	cartlib.DefineLCDGlyph('¶', rows)
	cartlib.DefineLCDGlyph('·', rows)
	cartlib.DefineLCDGlyph('¸', rows)
	cartlib.DefineLCDGlyph('¹', rows)
	cartlib.DefineLCDGlyph('º', rows)
	cartlib.DefineLCDGlyph('»', rows)
	cartlib.DefineLCDGlyph('¼', rows)
	cartlib.DefineLCDGlyph('½', rows)
	cartlib.DefineLCDGlyph('¾', rows)
	cartlib.DefineLCDGlyph('¿', rows)
	cartlib.DefineLCDGlyph('À', rows)
	cartlib.DefineLCDGlyph('Á', rows)
}

func showTitle() {
	cartlib.SetText(0, "OK", "")
}

func main() {}
