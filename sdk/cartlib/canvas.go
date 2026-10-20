// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

import "github.com/stealthmonkey99/arkade/sdk/ui"

var _ ui.Canvas = (*Framebuffer)(nil)

// Bounds returns [FBWidth] and [FBHeight], the frame's fixed size in pixels.
func (fb *Framebuffer) Bounds() (w, h int) { return FBWidth, FBHeight }

// Blend composites src over the pixel at (x, y) with [ui.BlendOver], the
// formula the engine uses, and stores the result opaque. Coordinates
// outside the frame and a fully transparent src do nothing.
func (fb *Framebuffer) Blend(x, y int, src Color) {
	if x < 0 || y < 0 || x >= FBWidth || y >= FBHeight || src.A == 0 {
		return
	}
	o := (y*FBWidth + x) * 4
	if src.A == 0xFF {
		fb.Pix[o], fb.Pix[o+1], fb.Pix[o+2], fb.Pix[o+3] = src.R, src.G, src.B, 0xFF
		return
	}
	dst := Color{R: fb.Pix[o], G: fb.Pix[o+1], B: fb.Pix[o+2], A: 0xFF}
	out := ui.BlendOver(dst, src)
	fb.Pix[o], fb.Pix[o+1], fb.Pix[o+2], fb.Pix[o+3] = out.R, out.G, out.B, 0xFF
}

// Color is ui.Color, re-exported so cartridge code needs only cartlib to
// name a color.
type Color = ui.Color
