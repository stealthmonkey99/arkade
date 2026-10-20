// SPDX-License-Identifier: GPL-2.0-or-later

// Package ui is the rendering toolkit shared by the cabinet and
// every cartridge. Widgets draw onto an abstract [Canvas], so one widget
// library produces identical pixels on a cartridge's framebuffer and on the
// cabinet's display.
//
// The package compiles unchanged under TinyGo inside a cartridge, which binds
// all of it:
//
//   - No package init() and no computed package-level initializers: tables
//     are compile-time literals TinyGo folds into static data, and setup
//     that computes belongs in the cartridge's Init.
//   - No goroutines, channels, time, math/rand or wall clock: a frame is a
//     pure function of its inputs.
//   - Avoid fmt and reflection (binary size).
//
// Widgets are plain values: construct one (or fill in a struct literal) and
// call its Draw method with a [Canvas] from the cartridge's render phase.
// Drawing never fails; geometry outside the canvas is clipped by
// [Canvas.Blend]. Stateful widgets such as [Menu], [NameEntry] and
// [HoldConfirm] are driven from Update with elapsed dtMicros and read back
// by the cartridge. Unless a doc says otherwise a call allocates nothing; the
// calls that do allocate say so and belong in Init.
//
// Coordinates and sizes are in pixels, durations in microseconds.
package ui

// Color is a straight (non-premultiplied) RGBA color in wire byte order.
type Color struct {
	R, G, B byte // channels, 0-255
	A       byte // opacity: 0 is transparent, 255 is opaque
}

// Common colors. Transparent is the zero value, so a zeroed Color is a
// no-op when blended.
var (
	Transparent = Color{}
	Black       = Color{0, 0, 0, 255}
	White       = Color{255, 255, 255, 255}
)

// Opaque reports whether c fully covers whatever is beneath it (the fast
// path in [Canvas] implementations).
func (c Color) Opaque() bool { return c.A == 0xFF }

// Canvas is a mutable pixel surface widgets draw onto. Widgets never assume
// which concrete surface backs it; an implementation needs only these two
// methods.
type Canvas interface {
	// Bounds is the drawable size in pixels.
	Bounds() (w, h int)
	// Blend alpha-composites src over the existing pixel at (x, y) with
	// straight-alpha source-over. Coordinates outside Bounds are ignored.
	// src.A==0 draws nothing; src.A==0xFF overwrites.
	Blend(x, y int, src Color)
}

// BlendOver returns src composited over dst with straight-alpha source-over.
// Every Canvas implementation must use it so a semi-transparent pixel lands
// identically on every surface. Over a fully transparent dst the result is
// src exactly. Otherwise the arithmetic is float64 with truncating byte
// conversion and no transcendentals, so it is bit-exact under TinyGo and
// host Go.
func BlendOver(dst, src Color) Color {
	if src.A == 0 {
		return dst
	}
	if src.A == 0xFF || dst.A == 0 {
		return src
	}
	aA := float64(src.A) / 255
	rA := float64(src.R) / 255
	gA := float64(src.G) / 255
	bA := float64(src.B) / 255
	aB := float64(dst.A) / 255
	rB := float64(dst.R) / 255
	gB := float64(dst.G) / 255
	bB := float64(dst.B) / 255
	aOut := aA + aB*(1-aA)
	if aOut == 0 {
		return Transparent
	}
	return Color{
		R: byte((rA*aA + rB*aB*(1-aA)) / aOut * 255),
		G: byte((gA*aA + gB*aB*(1-aA)) / aOut * 255),
		B: byte((bA*aA + bB*aB*(1-aA)) / aOut * 255),
		A: byte(aOut * 255),
	}
}

// Set blends one pixel (the common case: an opaque widget pixel).
func Set(cv Canvas, x, y int, c Color) { cv.Blend(x, y, c) }

// FillRect fills the rectangle [x, x+w) × [y, y+h) with c, clipped to bounds
// by Blend.
func FillRect(cv Canvas, x, y, w, h int, c Color) {
	if c.A == 0 || w <= 0 || h <= 0 {
		return
	}
	for py := y; py < y+h; py++ {
		for px := x; px < x+w; px++ {
			cv.Blend(px, py, c)
		}
	}
}

// StrokeRect draws a 1px outline around [x, x+w) × [y, y+h). Each outline
// pixel is blended exactly once, so a translucent stroke has even corners.
func StrokeRect(cv Canvas, x, y, w, h int, c Color) {
	if c.A == 0 || w <= 0 || h <= 0 {
		return
	}
	HLine(cv, x, x+w-1, y, c)
	if h == 1 {
		return
	}
	HLine(cv, x, x+w-1, y+h-1, c)
	if h == 2 {
		return
	}
	VLine(cv, x, y+1, y+h-2, c)
	if w > 1 {
		VLine(cv, x+w-1, y+1, y+h-2, c)
	}
}

// HLine draws a horizontal run from x0 to x1 (inclusive, either order) at y.
func HLine(cv Canvas, x0, x1, y int, c Color) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	for x := x0; x <= x1; x++ {
		cv.Blend(x, y, c)
	}
}

// VLine draws a vertical run from y0 to y1 (inclusive, either order) at x.
func VLine(cv Canvas, x, y0, y1 int, c Color) {
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	for y := y0; y <= y1; y++ {
		cv.Blend(x, y, c)
	}
}

// Line draws a 1px line between two points with integer Bresenham, so it is
// bit-identical across toolchains.
func Line(cv Canvas, x0, y0, x1, y1 int, c Color) {
	dx := x1 - x0
	if dx < 0 {
		dx = -dx
	}
	dy := y1 - y0
	if dy < 0 {
		dy = -dy
	}
	sx := 1
	if x0 > x1 {
		sx = -1
	}
	sy := 1
	if y0 > y1 {
		sy = -1
	}
	err := dx - dy
	for {
		cv.Blend(x0, y0, c)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}
