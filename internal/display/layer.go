// SPDX-License-Identifier: GPL-2.0-or-later

package display

import "github.com/stealthmonkey99/arkade/internal/clock"

// Element is the contract a UI element must satisfy to live on a layer. The
// element package provides the concrete implementations.
//
// A layer repaints only when some element IsDirty, and calls MarkClean on
// each element after painting it, so an element must report dirty for any
// change that would alter Render's output, including changes inside the
// elements it composes, and Render must not advance state: animation
// belongs in an Update the app drives.
type Element interface {
	// Render produces the element's sprite for this frame (post
	// opacity/rotation/alignment transforms).
	Render(stats clock.FrameTime) Sprite
	Visible() bool
	IsDirty() bool
	MarkClean()
	// OffsetX/OffsetY are the paint position including alignment shift.
	OffsetX() int
	OffsetY() int
	// Width/Height are the element's current (post-rotation) dimensions.
	Width() int
	Height() int
}

// Layer is one compositing plane: a fill slate plus a list of elements.
type Layer struct {
	Owner string
	Label string

	width, height int
	elements      []Element
	slate         Matrix
	lastRender    Matrix
	opacity       float64
	dirtySlate    bool
}

// LayerOptions configures a new layer.
type LayerOptions struct {
	Fill     *Pixel
	Elements []Element
	// Opacity 0..1; nil defaults to 1.
	Opacity *float64
}

// NewLayer builds a layer sized to the display.
func NewLayer(w, h int, owner, label string, opts LayerOptions) *Layer {
	l := &Layer{Owner: owner, Label: label, width: w, height: h, opacity: 1}
	fill := Transparent
	if opts.Fill != nil {
		fill = *opts.Fill
	}
	l.SetFill(fill)
	l.elements = append(l.elements, opts.Elements...)
	if opts.Opacity != nil {
		l.opacity = *opts.Opacity
	}
	return l
}

// SetFill replaces the background slate.
func (l *Layer) SetFill(p Pixel) {
	l.slate = NewMatrix(l.width, l.height, p)
	l.dirtySlate = true
}

// Opacity returns the layer opacity (0..1).
func (l *Layer) Opacity() float64 { return l.opacity }

// SetOpacity changes layer opacity, invalidating the render cache.
func (l *Layer) SetOpacity(v float64) {
	if l.opacity != v {
		l.lastRender = nil
	}
	l.opacity = v
}

// AddElement appends an element and invalidates the cache.
func (l *Layer) AddElement(e Element) {
	l.elements = append(l.elements, e)
	l.lastRender = nil
}

// RemoveElement removes an element by identity.
func (l *Layer) RemoveElement(e Element) {
	for i, existing := range l.elements {
		if existing == e {
			l.elements = append(l.elements[:i], l.elements[i+1:]...)
			l.lastRender = nil
			return
		}
	}
}

// Elements returns the layer's element list.
func (l *Layer) Elements() []Element { return l.elements }

func (l *Layer) hasDirtyElements() bool {
	for _, e := range l.elements {
		if e.IsDirty() {
			return true
		}
	}
	return false
}

// Paint renders the layer to a matrix. Fully transparent layers return an
// empty matrix; unchanged layers return the cached previous render.
func (l *Layer) Paint(stats clock.FrameTime) Matrix {
	if l.opacity <= 0 {
		return Matrix{{}}
	}
	if l.lastRender != nil && !l.dirtySlate && !l.hasDirtyElements() {
		return l.lastRender
	}
	canvas := CloneMatrix(l.slate)
	l.dirtySlate = false

	for _, e := range l.elements {
		if !e.Visible() {
			e.MarkClean()
			continue
		}
		sprite := e.Render(stats)
		offX, offY := e.OffsetX(), e.OffsetY()
		maxY := min(l.height, offY+min(len(sprite), e.Height()))
		for y := max(0, offY); y < maxY; y++ {
			row := sprite[y-offY]
			maxX := min(l.width, offX+min(l.width, e.Width()))
			for x := max(0, offX); x < maxX; x++ {
				sx := x - offX
				if sx >= len(row) {
					break
				}
				canvas[y][x] = Merge(canvas[y][x], row[sx])
			}
		}
		e.MarkClean()
	}

	if l.opacity < 1 {
		for y := range canvas {
			for x := range canvas[y] {
				canvas[y][x] = canvas[y][x].ScaleAlpha(l.opacity)
			}
		}
	}

	l.lastRender = canvas
	return canvas
}
