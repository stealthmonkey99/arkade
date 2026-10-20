// Fixture: a cartridge defining and using its own LCD glyph.
// The pass path for check_lcd.go's cart-glyph support — a rune outside
// the engine's table is not a violation once cartlib.DefineLCDGlyph
// accepts it, and it may sit alongside a rune the engine already knows.
package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

// The cart's own glyph, defined through a const the way sdk/tutorial's
// Catcher does it — one level of indirection the check must still
// resolve, just as it does for stringConsts.
const basket = '֍'

var basketRows = cartlib.GlyphRows{0x0, 0xe, 0x11, 0x1f, 0x15, 0x1f, 0xe, 0x0}

func setup() {
	cartlib.DefineLCDGlyph(basket, basketRows)
}

func showTitle() {
	// The cart's own glyph and one from the engine's table on the same
	// screen — both are legitimate sources, so neither is unmapped.
	cartlib.SetText(0, "CATCHER ֍", "LIVES ♥")
}

func main() {}
