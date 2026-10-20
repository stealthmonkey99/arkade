// Fixture: a cartridge that defines its glyphs on an emoji and on a CJK
// ideograph. Both hosts refuse those (internal/lcdrune: an LCD glyph rune
// must be a plain, single-width, monochrome character — a terminal draws
// these in colour and two cells wide) — check_lcd.go must say so
// statically rather than let the cabinet print '?' first. The third
// definition, on a narrow monochrome symbol, is fine and must NOT be
// flagged.
package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

const (
	basket = '🧺' // U+1F9FA, an emoji
	one    = '一' // U+4E00, East Asian wide
	woven  = '֍' // U+058D, the tutorial's basket: plain, narrow, monochrome
)

var rows = cartlib.GlyphRows{0x0, 0xe, 0x11, 0x1f, 0x15, 0x1f, 0xe, 0x0}

func setup() {
	cartlib.DefineLCDGlyph(basket, rows)
	cartlib.DefineLCDGlyph(one, rows)
	cartlib.DefineLCDGlyph(woven, rows)
}

func showTitle() {
	// Printing the refused runes is reported under the same rule — a '?'
	// is what the glass would show — while the accepted one passes.
	cartlib.SetText(0, "CATCHER 🧺 ֍", "")
}

func main() {}
