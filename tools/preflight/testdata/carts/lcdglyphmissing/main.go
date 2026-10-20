// Fixture: a cartridge that defines its own glyph correctly, but a
// SEPARATE line uses a rune neither table carries. Cart-glyph support
// must not blur into "any custom rune passes" — an undefined one must
// still fail even in a cartridge that otherwise uses the feature
// correctly.
package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

const basket = '֍'

var basketRows = cartlib.GlyphRows{0x0, 0xe, 0x11, 0x1f, 0x15, 0x1f, 0xe, 0x0}

func setup() {
	cartlib.DefineLCDGlyph(basket, basketRows)
}

func showTitle() {
	// Defined by this cartridge: must not be reported.
	cartlib.SetText(0, "CATCHER ֍", "")
	// In NEITHER the engine table nor this cartridge's own definitions.
	cartlib.SetText(0, "DANGER ☠", "")
}

func main() {}
