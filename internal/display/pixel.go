// SPDX-License-Identifier: GPL-2.0-or-later

// Package display holds the pixel model, sprite/matrix types, layers and the
// compositor that feeds renderers.
package display

// Pixel is a single RGBA pixel. Alpha 0 is fully transparent, 255 opaque.
type Pixel struct {
	R, G, B, A uint8
}

// Sprite is a jagged pixel grid (rows may differ in width), the unit that
// elements render. Matrix is the same shape but conventionally rectangular.
type Sprite = [][]Pixel

// Matrix is a rectangular pixel grid, as produced by the compositor.
type Matrix = [][]Pixel

var (
	Transparent = Pixel{0, 0, 0, 0}
	Highlight   = Pixel{255, 255, 255, 70}
	Shadow      = Pixel{0, 0, 0, 70}
	DarkShadow  = Pixel{0, 0, 0, 100}
	Focus       = Pixel{150, 188, 227, 255}
	White       = Pixel{255, 255, 255, 255}
	LightGray   = Pixel{180, 180, 180, 255}
	Gray        = Pixel{120, 120, 120, 255}
	DarkGray    = Pixel{80, 80, 80, 255}
	Black       = Pixel{0, 0, 0, 255}
	Red         = Pixel{255, 0, 0, 255}
	Green       = Pixel{0, 255, 0, 255}
	DarkGreen   = Pixel{0, 180, 0, 255}
	Blue        = Pixel{0, 0, 255, 255}
	Yellow      = Pixel{255, 255, 0, 255}
	Cyan        = Pixel{0, 255, 255, 255}
	Magenta     = Pixel{255, 0, 255, 255}
	Orange      = Pixel{255, 160, 0, 255}
)

// RGB builds an opaque pixel.
func RGB(r, g, b uint8) Pixel { return Pixel{r, g, b, 255} }

// RGBA builds a pixel with an explicit alpha.
func RGBA(r, g, b, a uint8) Pixel { return Pixel{r, g, b, a} }

// WithAlpha returns a copy of p with alpha a.
func (p Pixel) WithAlpha(a uint8) Pixel { return Pixel{p.R, p.G, p.B, a} }

// ScaleAlpha multiplies the alpha channel by factor (0..1), flooring.
func (p Pixel) ScaleAlpha(factor float64) Pixel {
	if factor >= 1 {
		return p
	}
	if factor <= 0 {
		return Pixel{p.R, p.G, p.B, 0}
	}
	return Pixel{p.R, p.G, p.B, uint8(float64(p.A) * factor)}
}

// Merge alpha-composites src over dst, flooring on rounding. Over a fully
// transparent dst the result is src exactly.
func Merge(dst, src Pixel) Pixel {
	if src.A == 0 {
		return dst
	}
	if src.A == 255 || dst.A == 0 {
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

	return Pixel{
		R: uint8((rA*aA + rB*aB*(1-aA)) / aOut * 255),
		G: uint8((gA*aA + gB*aB*(1-aA)) / aOut * 255),
		B: uint8((bA*aA + bB*aB*(1-aA)) / aOut * 255),
		A: uint8(aOut * 255),
	}
}

// NewMatrix builds a w×h matrix filled with fill.
func NewMatrix(w, h int, fill Pixel) Matrix {
	m := make(Matrix, h)
	for y := range m {
		row := make([]Pixel, w)
		for x := range row {
			row[x] = fill
		}
		m[y] = row
	}
	return m
}

// CloneMatrix deep-copies a matrix/sprite.
func CloneMatrix(src Matrix) Matrix {
	out := make(Matrix, len(src))
	for y, row := range src {
		out[y] = append([]Pixel(nil), row...)
	}
	return out
}

// MatrixEquals reports whether two matrices hold identical pixels.
func MatrixEquals(a, b Matrix) bool {
	if len(a) != len(b) {
		return false
	}
	for y := range a {
		if len(a[y]) != len(b[y]) {
			return false
		}
		for x := range a[y] {
			if a[y][x] != b[y][x] {
				return false
			}
		}
	}
	return true
}
