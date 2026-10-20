// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

import (
	"unsafe"

	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// Sheet is a handle on one sprite sheet loaded by [Setup.LoadSheet]
// (docs/ASSETS.md, "Sprites from the bundle"). The zero Sheet is the
// failed-load value: its Cel returns nil, so a cartridge can lift cels
// without checking first.
//
// Lifting a cel allocates, under a collector that never frees, so a Sheet is
// as Init-only as the Setup that loaded it: lift every cel the game will
// draw in [Game.Init], keep the sprites, and never call Cel again. Each
// cel's collision mask is [ui.OpaqueMask]'s: exactly the A==255 pixels.
type Sheet struct {
	handle uint32
	setup  *Setup
}

// Ok reports whether the sheet loaded.
func (s Sheet) Ok() bool { return s.handle != 0 }

// celInfoBytes is host_cel_info's record: four little-endian u16s: w, h,
// anchorX, anchorY.
const celInfoBytes = 8

// maxCelDim is the host's per-axis cel cap and the width of ui.Mask's
// uint64 rows.
const maxCelDim = 64

// Cel copies the named cel into a new [ui.Sprite] whose collision mask is
// its pixels with alpha 255; lower alpha draws blended but never collides.
// It returns nil when the sheet did not load, has no cel by that name, or
// the cel exceeds 64 pixels on a side. Each call allocates the pixels, the
// mask and the sprite, so Cel is live only while the [Setup] that loaded
// the sheet is: after Init it returns nil and fails as a stale Setup call
// does.
func (s Sheet) Cel(name string) *ui.Sprite {
	if s.handle != 0 && !s.setup.ok("cartlib: Sheet.Cel after Init returned; the cel was not lifted") {
		return nil
	}
	handle := s.celHandle(name)
	if handle == 0 {
		return nil
	}
	var info [celInfoBytes]byte
	if host.CelInfo(handle, info[:]) == 0 {
		return nil
	}
	w := int(info[0]) | int(info[1])<<8
	h := int(info[2]) | int(info[3])<<8
	if w < 1 || h < 1 || w > maxCelDim || h > maxCelDim {
		return nil
	}

	pix := make([]byte, w*h*4)
	if int(host.CelRead(handle, pix)) != len(pix) {
		return nil
	}

	// pix is never written again, so aliasing it avoids a permanent second
	// copy under -gc=leaking.
	return &ui.Sprite{
		W:    w,
		H:    h,
		Pix:  unsafe.String(&pix[0], len(pix)),
		Mask: ui.OpaqueMask(w, h, pix),
	}
}

func (s Sheet) celHandle(name string) uint32 {
	if s.handle == 0 {
		return 0
	}
	return host.SheetCel(s.handle, name)
}
