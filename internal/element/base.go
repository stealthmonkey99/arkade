// SPDX-License-Identifier: GPL-2.0-or-later

// Package element is the cabinet's retained-mode side of the sdk/ui widget
// library: elements that live on display layers, track their own dirtiness
// for the layer cache, and draw through sdk/ui onto a display.Sprite. Only
// what the SDK has no equal for is rasterized here (Circle's bevel, Line,
// the engine's LoadingBar, QRCode, PNG and JSON sprites).
package element

import (
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
)

// Alignment anchors an element's offset point.
type Alignment string

const (
	TopLeft      Alignment = "top-left"
	TopCenter    Alignment = "top-center"
	TopRight     Alignment = "top-right"
	Left         Alignment = "left"
	Center       Alignment = "center"
	Right        Alignment = "right"
	BottomLeft   Alignment = "bottom-left"
	BottomCenter Alignment = "bottom-center"
	BottomRight  Alignment = "bottom-right"
)

// Options configures the shared element fields.
type Options struct {
	OffsetX int
	OffsetY int
	Hidden  bool // inverted so the zero value is visible
	Opacity *float64
	Align   Alignment
	// Width/Height set an explicit viewport that overrides natural size.
	Width  *int
	Height *int
}

// Base carries shared element behavior: visibility, opacity, rotation,
// alignment and dirty tracking. Concrete elements embed it and set Impl; one
// that renders other elements registers them with compose.
type Base struct {
	dirty         bool
	parts         []display.Element
	width, height int // natural dims, updated from each render
	offX, offY    int
	logicalX      int
	logicalY      int
	align         Alignment
	visible       bool
	opacity       float64
	rotation      display.Orientation
	viewportW     *int
	viewportH     *int

	// Impl produces the raw (untransformed) sprite for a frame.
	Impl func(stats clock.FrameTime) display.Sprite
}

// InitBase applies Options; concrete constructors call this.
func (b *Base) InitBase(opts Options) {
	b.dirty = true
	b.offX = opts.OffsetX
	b.offY = opts.OffsetY
	b.visible = !opts.Hidden
	b.opacity = 1
	if opts.Opacity != nil {
		b.opacity = *opts.Opacity
	}
	b.align = opts.Align
	if b.align == "" {
		b.align = TopLeft
	}
	b.viewportW = opts.Width
	b.viewportH = opts.Height
}

// Render produces the transformed sprite and refreshes dimensions and
// alignment offsets.
func (b *Base) Render(stats clock.FrameTime) display.Sprite {
	sprite := b.Impl(stats)
	b.height = len(sprite)
	b.width = 0
	for _, row := range sprite {
		if len(row) > b.width {
			b.width = len(row)
		}
	}
	transformed := b.applyOpacity(b.applyRotation(sprite))

	totalW := 0
	if len(transformed) > 0 {
		totalW = len(transformed[0])
	}
	totalH := len(transformed)
	switch b.align {
	case TopLeft, Left, BottomLeft:
		b.logicalX = 0
	case TopCenter, Center, BottomCenter:
		b.logicalX = -(totalW + 1) / 2 // rounds half up
	default:
		b.logicalX = -totalW
	}
	switch b.align {
	case TopLeft, TopCenter, TopRight:
		b.logicalY = 0
	case Left, Center, Right:
		b.logicalY = -(totalH + 1) / 2
	default:
		b.logicalY = -totalH
	}

	return transformed
}

func (b *Base) applyOpacity(sprite display.Sprite) display.Sprite {
	if !b.visible || b.opacity == 0 {
		return display.Sprite{{}}
	}
	if b.opacity == 1 {
		return sprite
	}
	out := make(display.Sprite, len(sprite))
	for y, row := range sprite {
		newRow := make([]display.Pixel, len(row))
		for x, p := range row {
			newRow[x] = p.ScaleAlpha(b.opacity)
		}
		out[y] = newRow
	}
	return out
}

func (b *Base) applyRotation(sprite display.Sprite) display.Sprite {
	switch b.rotation {
	case display.Clockwise:
		out := make(display.Sprite, b.width)
		for i := range out {
			out[i] = make([]display.Pixel, b.height)
		}
		for y, row := range sprite {
			for x, p := range row {
				out[b.width-1-x][y] = p
			}
		}
		return out
	case display.CounterClockwise:
		out := make(display.Sprite, b.width)
		for i := range out {
			out[i] = make([]display.Pixel, b.height)
		}
		for y, row := range sprite {
			for x, p := range row {
				out[x][b.height-1-y] = p
			}
		}
		return out
	case display.UpsideDown:
		out := make(display.Sprite, len(sprite))
		for y := range sprite {
			src := sprite[len(sprite)-1-y]
			row := make([]display.Pixel, len(src))
			for x := range src {
				row[x] = src[len(src)-1-x]
			}
			out[y] = row
		}
		return out
	default:
		return sprite
	}
}

func (b *Base) isPerpendicular() bool {
	return b.rotation == display.Clockwise || b.rotation == display.CounterClockwise
}

// Width returns the effective width (viewport override, rotation-aware).
func (b *Base) Width() int {
	if b.isPerpendicular() {
		if b.viewportH != nil {
			return *b.viewportH
		}
		return b.height
	}
	if b.viewportW != nil {
		return *b.viewportW
	}
	return b.width
}

// SetWidth overrides the width (viewport if one was set, else natural).
func (b *Base) SetWidth(v int) {
	target, dim := b.viewportW, &b.width
	if b.isPerpendicular() {
		target, dim = b.viewportH, &b.height
	}
	if target != nil {
		b.dirty = b.dirty || *target != v
		*target = v
		return
	}
	b.dirty = b.dirty || *dim != v
	*dim = v
}

// Height returns the effective height.
func (b *Base) Height() int {
	if b.isPerpendicular() {
		if b.viewportW != nil {
			return *b.viewportW
		}
		return b.width
	}
	if b.viewportH != nil {
		return *b.viewportH
	}
	return b.height
}

// SetHeight overrides the height.
func (b *Base) SetHeight(v int) {
	target, dim := b.viewportH, &b.height
	if b.isPerpendicular() {
		target, dim = b.viewportW, &b.width
	}
	if target != nil {
		b.dirty = b.dirty || *target != v
		*target = v
		return
	}
	b.dirty = b.dirty || *dim != v
	*dim = v
}

func (b *Base) Visible() bool { return b.visible }

func (b *Base) SetVisible(shown bool) {
	b.dirty = b.dirty || b.visible != shown
	b.visible = shown
}

func (b *Base) Opacity() float64 { return b.opacity }

func (b *Base) SetOpacity(v float64) {
	capped := max(0, min(1, v))
	b.dirty = b.dirty || b.opacity != capped
	b.opacity = capped
}

func (b *Base) Rotation() display.Orientation { return b.rotation }

func (b *Base) SetRotation(o display.Orientation) {
	b.dirty = b.dirty || b.rotation != o
	b.rotation = o
}

// OffsetX includes the alignment shift computed at last render.
func (b *Base) OffsetX() int { return b.offX + b.logicalX }

func (b *Base) SetOffsetX(v int) {
	b.dirty = b.dirty || b.offX != v
	b.offX = v
}

// OffsetY includes the alignment shift computed at last render.
func (b *Base) OffsetY() int { return b.offY + b.logicalY }

func (b *Base) SetOffsetY(v int) {
	b.dirty = b.dirty || b.offY != v
	b.offY = v
}

// compose registers the elements this one renders: a change inside any of
// them repaints this element, and a paint cleans them all.
func (b *Base) compose(parts ...display.Element) { b.parts = parts }

// IsDirty reports whether this element or any composed part changed since
// the last MarkClean.
func (b *Base) IsDirty() bool {
	if b.dirty {
		return true
	}
	for _, p := range b.parts {
		if p.IsDirty() {
			return true
		}
	}
	return false
}

// MarkDirty forces a repaint on the next frame.
func (b *Base) MarkDirty() { b.dirty = true }

// MarkClean clears this element's and its parts' dirty flags; the layer
// calls it after painting.
func (b *Base) MarkClean() {
	b.dirty = false
	for _, p := range b.parts {
		p.MarkClean()
	}
}
