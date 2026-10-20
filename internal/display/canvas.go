// SPDX-License-Identifier: GPL-2.0-or-later

package display

import "github.com/stealthmonkey99/arkade/sdk/ui"

// SpriteCanvas adapts a display.Sprite to ui.Canvas so sdk/ui widgets draw
// into an engine sprite. Blends go through ui.BlendOver, a byte-identical
// replica of Merge.
//
// The wrapped Sprite must be rectangular and pre-sized (use NewMatrix);
// untouched pixels keep their initial value.
type SpriteCanvas struct{ Sprite Sprite }

// NewSpriteCanvas wraps a transparent w×h sprite as a Canvas and returns
// both, for the common "draw some widgets, then hand the sprite to a layer"
// flow.
func NewSpriteCanvas(w, h int) (*SpriteCanvas, Sprite) {
	s := NewMatrix(w, h, Transparent)
	return &SpriteCanvas{Sprite: s}, s
}

// Bounds is the sprite's width and height. A jagged sprite reports its first
// row's width; widgets clip per-row via Blend anyway.
func (c *SpriteCanvas) Bounds() (w, h int) {
	h = len(c.Sprite)
	if h > 0 {
		w = len(c.Sprite[0])
	}
	return w, h
}

// Blend composites src over the sprite pixel at (x, y) using the engine's
// own Merge, silently ignoring out-of-range coordinates.
func (c *SpriteCanvas) Blend(x, y int, src ui.Color) {
	if y < 0 || y >= len(c.Sprite) {
		return
	}
	row := c.Sprite[y]
	if x < 0 || x >= len(row) {
		return
	}
	row[x] = Merge(row[x], Pixel{src.R, src.G, src.B, src.A})
}

// FromUI and ToUI convert between Pixel and ui.Color, which share a layout.
func FromUI(c ui.Color) Pixel { return Pixel{c.R, c.G, c.B, c.A} }
func ToUI(p Pixel) ui.Color   { return ui.Color{R: p.R, G: p.G, B: p.B, A: p.A} }

// Drawable is any sdk/ui widget: a value that paints itself onto a Canvas.
type Drawable interface{ Draw(cv ui.Canvas) }

// Draw renders d onto a fresh transparent w×h sprite through a SpriteCanvas,
// blending as the widget would onto any surface.
func Draw(w, h int, d Drawable) Sprite {
	cv, s := NewSpriteCanvas(w, h)
	d.Draw(cv)
	return s
}

// Stamp renders d onto a fresh transparent w×h sprite writing each pixel as
// the widget gives it, alpha intact: where the widget paints a pixel twice the
// last write wins instead of compositing. An element's own art is authored,
// not composited, and the layer blends it exactly once when it paints.
func Stamp(w, h int, d Drawable) Sprite {
	s := NewMatrix(w, h, Transparent)
	d.Draw(stampCanvas{s})
	return s
}

// TextSprite is text in the LED font as a bare sprite, one line tall and as
// wide as the text, for composing into other drawings.
func TextSprite(text string, c Pixel) Sprite {
	font := ui.FontLED{}
	return Stamp(font.TextWidth(text, 1), font.Height(1), ui.Label{Text: text, Color: ToUI(c)})
}

// stampCanvas is the Canvas behind Stamp: Blend replaces the pixel.
type stampCanvas struct{ s Sprite }

func (c stampCanvas) Bounds() (w, h int) { return (&SpriteCanvas{Sprite: c.s}).Bounds() }

func (c stampCanvas) Blend(x, y int, src ui.Color) {
	if y < 0 || y >= len(c.s) || x < 0 || x >= len(c.s[y]) {
		return
	}
	c.s[y][x] = FromUI(src)
}
