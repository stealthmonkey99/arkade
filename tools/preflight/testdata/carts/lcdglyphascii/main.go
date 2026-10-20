// Fixture: a cartridge that tries to redefine an ASCII code point. The
// runtime refuses this (sdk/cartlib/glyphs.go: code points below U+0080
// are rejected outright) — check_lcd.go must say so statically rather
// than let the cabinet discover it first.
package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

var bRows = cartlib.GlyphRows{0x0, 0xe, 0x11, 0x1f, 0x15, 0x1f, 0xe, 0x0}

func setup() {
	// 'b' is U+0062 — well inside ASCII, and the ROM font already draws it.
	cartlib.DefineLCDGlyph('b', bRows)
}

func showTitle() {
	cartlib.SetText(0, "HELLO", "")
}

func main() {}
