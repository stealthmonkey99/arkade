// SPDX-License-Identifier: GPL-2.0-or-later

package main

// Catcher's art, read out of the cartridge's own bundle at Init — the
// worked example of "sprites from the bundle" (docs/ASSETS.md;
// sdk/docs/reference-sprite.md; the tutorial page is
// sdk/docs/tutorial-assets.md).
//
// # Where the pixels live
//
// One packed sheet — assets/catcher.arkadesprite.png, written by
// tools/gen-sheet from assets/sprites/*.png — travels in the .rcart, so
// the pixels stay out of catcher.wasm. The host decodes it once at
// host_load_sheet, and Setup.LoadSheet lifts the cels this cartridge wants
// into ui.Sprite values here: ordinary ui.Sprites with ordinary collision
// masks, drawn and collided guest-side, under ui.OpaqueMask's alpha rule
// (A==255 collides).
//
// # Why loading happens exactly once
//
// A cartridge is built with -gc=leaking, so every allocation is permanent
// (GOLDEN RULE 7). LoadSheet and Sheet.Cel allocate — pixels and mask rows,
// once per cel — which is why they hang off the Setup Init receives and
// nothing else. Everything below is a package var holding the result.

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// sheetPath is the bundle-relative path the host resolves, in the same
// path space Setup.LoadSound uses. Regenerate the file it names with
// `make sheet` after editing assets/sprites/*.png.
const sheetPath = "assets/catcher.arkadesprite.png"

// blank stands in for any cel the bundle could not supply — a dev build
// run from a bare .wasm with no .rcart around it, most often. A zero
// ui.Sprite draws nothing and collides with nothing, so the game keeps
// running (soundless, artless) instead of trapping on a nil sprite in the
// middle of a frame.
var blank ui.Sprite

// spriteFruit is the falling fruit, and the still life on the title.
var spriteFruit = &blank

// walkCels are the four STEPS of the catcher's walk cycle, in play order —
// note steps, not cels: step 0 and step 2 are the same drawing
// ("catcher.0", the standing pose), which is why the cycle reads as a
// stride rather than a shuffle.
//
// The sheet's envelope carries this animation too, under the name "walk",
// for spritelab's benefit. The cartridge keeps its own copy rather than
// asking the host for it because the host deliberately exposes cels and not
// animations: how long a cel is held is a question about THIS game's clock,
// and a fixed array of four is something -gc=leaking is happy with.
var walkCels = [4]*ui.Sprite{&blank, &blank, &blank, &blank}

// walkHoldMicros is how long each STEP is held. Copied from the envelope's
// holdMs (130ms a step), in microseconds so advance() can compare against
// accumulated dtMicros without dividing.
var walkHoldMicros = [4]uint64{130_000, 130_000, 130_000, 130_000}

// loadSprites lifts every cel this cartridge draws out of the bundle.
// Called once, from Init, beside loadSounds and defineGlyphs.
func loadSprites(s *cartlib.Setup) {
	sheet := s.LoadSheet(sheetPath)
	if !sheet.Ok() {
		// The host has already logged why. Every sprite stays blank and
		// the game runs on without art rather than not at all.
		cartlib.Log(cartlib.LogWarn, "catcher: no sprite sheet at "+sheetPath)
		return
	}
	spriteFruit = celOr(sheet, "fruit")

	// The walk's step order, spelled out: stand, stride, stand, stride
	// again on the other foot.
	stand := celOr(sheet, "catcher.0")
	walkCels = [4]*ui.Sprite{stand, celOr(sheet, "catcher.1"), stand, celOr(sheet, "catcher.2")}
}

// celOr lifts one named cel, falling back to the blank sprite so no call
// site ever has to hold a nil. A missing cel is a mismatch between this
// file and tools/gen-sheet's cel names, and the host logs it by name.
func celOr(sheet cartlib.Sheet, name string) *ui.Sprite {
	if s := sheet.Cel(name); s != nil {
		return s
	}
	return &blank
}
