// SPDX-License-Identifier: GPL-2.0-or-later

package ui

// Layer is an off-screen drawable surface: a pixel buffer that implements
// [Canvas], so any widget can Draw into it. A [Compositor] stacks Layers and
// flattens them, applying each layer's Opacity, onto a target Canvas.
type Layer struct {
	w, h int
	pix  []Color // straight-alpha, row-major, len == w*h

	// Opacity scales this layer's alpha when a Compositor flattens it, in
	// 0..1. It does not affect drawing into the layer: Blend always
	// composites at full strength.
	Opacity float64
	// Visible gates compositing; an invisible layer is skipped regardless of
	// Opacity and keeps its pixels.
	Visible bool
}

// NewLayer allocates a fully transparent, visible w×h layer at full opacity;
// negative sizes are treated as 0. It allocates, so call it during Init.
func NewLayer(w, h int) *Layer {
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return &Layer{
		w:       w,
		h:       h,
		pix:     make([]Color, w*h),
		Opacity: 1,
		Visible: true,
	}
}

// Bounds implements Canvas.
func (l *Layer) Bounds() (w, h int) { return l.w, l.h }

// Blend implements Canvas with straight-alpha source-over via [BlendOver].
// Coordinates outside Bounds are ignored.
func (l *Layer) Blend(x, y int, src Color) {
	if x < 0 || y < 0 || x >= l.w || y >= l.h {
		return
	}
	i := y*l.w + x
	l.pix[i] = BlendOver(l.pix[i], src)
}

// At returns the raw pixel at (x, y), before any Opacity scaling.
// Out-of-bounds coordinates return Transparent.
func (l *Layer) At(x, y int) Color {
	if x < 0 || y < 0 || x >= l.w || y >= l.h {
		return Transparent
	}
	return l.pix[y*l.w+x]
}

// Clear resets every pixel to Transparent without reallocating, the usual
// way to reuse a Layer across frames.
func (l *Layer) Clear() {
	for i := range l.pix {
		l.pix[i] = Transparent
	}
}

// Fill resets every pixel to c without reallocating the backing buffer.
func (l *Layer) Fill(c Color) {
	for i := range l.pix {
		l.pix[i] = c
	}
}
