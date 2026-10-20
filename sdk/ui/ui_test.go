// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

// byteCanvas stores a flat RGBA slice and gridCanvas a 2-D Color grid. Both
// start opaque black, so a widget must render byte-identical on each,
// semi-transparent pixels included.

type byteCanvas struct {
	w, h int
	pix  []byte // RGBA, row-major
}

func newByteCanvas(w, h int) *byteCanvas {
	c := &byteCanvas{w: w, h: h, pix: make([]byte, w*h*4)}
	for i := 3; i < len(c.pix); i += 4 {
		c.pix[i] = 0xFF // opaque black
	}
	return c
}
func (c *byteCanvas) Bounds() (int, int) { return c.w, c.h }
func (c *byteCanvas) Blend(x, y int, src Color) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	o := (y*c.w + x) * 4
	dst := Color{c.pix[o], c.pix[o+1], c.pix[o+2], c.pix[o+3]}
	out := BlendOver(dst, src)
	c.pix[o], c.pix[o+1], c.pix[o+2], c.pix[o+3] = out.R, out.G, out.B, out.A
}
func (c *byteCanvas) at(x, y int) Color {
	o := (y*c.w + x) * 4
	return Color{c.pix[o], c.pix[o+1], c.pix[o+2], c.pix[o+3]}
}

type gridCanvas struct {
	w, h int
	px   [][]Color
}

func newGridCanvas(w, h int) *gridCanvas {
	px := make([][]Color, h)
	for y := range px {
		px[y] = make([]Color, w)
		for x := range px[y] {
			px[y][x] = Black
		}
	}
	return &gridCanvas{w: w, h: h, px: px}
}
func (c *gridCanvas) Bounds() (int, int) { return c.w, c.h }
func (c *gridCanvas) Blend(x, y int, src Color) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	c.px[y][x] = BlendOver(c.px[y][x], src)
}
func (c *gridCanvas) at(x, y int) Color { return c.px[y][x] }

// paintScene draws a mix of fills, strokes, lines, translucent overlays and
// scaled text onto any Canvas.
func paintScene(cv Canvas) {
	Rect{X: 2, Y: 2, W: 20, H: 12, Fill: Color{40, 160, 80, 255}, Stroke: White, StrokeW: 2}.Draw(cv)
	Line(cv, 0, 0, 30, 20, Color{255, 0, 0, 255})
	FillRect(cv, 5, 5, 10, 6, Color{0, 0, 255, 90})    // semi-transparent → exercises BlendOver
	FillRect(cv, 8, 3, 4, 14, Color{255, 255, 0, 128}) // overlap, another partial alpha
	Font3x5{}.DrawText(cv, 1, 24, "SNAKE", 2, Color{0, 255, 0, 255})
	Font3x5{}.DrawUint(cv, 1, 40, 12345, 1, White)
}

// TestCanvasSubstrateParity: the same widget code on two Canvas
// implementations with different internals produces identical pixels.
func TestCanvasSubstrateParity(t *testing.T) {
	const w, h = 48, 48
	bc := newByteCanvas(w, h)
	gc := newGridCanvas(w, h)
	paintScene(bc)
	paintScene(gc)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if bc.at(x, y) != gc.at(x, y) {
				t.Fatalf("pixel (%d,%d): byteCanvas=%v gridCanvas=%v", x, y, bc.at(x, y), gc.at(x, y))
			}
		}
	}
}

// TestBlendOverEndpoints pins the transparent and opaque fast paths and one
// partial-alpha result: 50% white (A=128) over opaque black yields 128, not
// 127, because the float path floors 128.0002.
func TestBlendOverEndpoints(t *testing.T) {
	if got := BlendOver(Black, Transparent); got != Black {
		t.Errorf("transparent over black = %v, want black", got)
	}
	if got := BlendOver(White, Black); got != Black {
		t.Errorf("opaque black over white = %v, want black", got)
	}
	half := Color{255, 255, 255, 128}
	got := BlendOver(Black, half)
	if got.R != 128 || got.G != 128 || got.B != 128 {
		t.Errorf("50%% white over black = %v, want RGB 128 (float-floor, matches display.Merge)", got)
	}
	faint := Color{3, 3, 3, 3}
	if got := BlendOver(Color{200, 0, 0, 0}, faint); got != faint {
		t.Errorf("translucent over transparent = %v, want src exactly", got)
	}
}

type countCanvas struct {
	w, h   int
	blends map[[2]int]int
}

func (c *countCanvas) Bounds() (int, int) { return c.w, c.h }
func (c *countCanvas) Blend(x, y int, _ Color) {
	c.blends[[2]int{x, y}]++
}

// TestStrokeRectBlendsEachPixelOnce: a translucent outline composited twice at
// a corner comes out darker than its edges.
func TestStrokeRectBlendsEachPixelOnce(t *testing.T) {
	sizes := [][2]int{{1, 1}, {1, 4}, {4, 1}, {2, 2}, {2, 5}, {5, 2}, {6, 4}}
	for _, sz := range sizes {
		w, h := sz[0], sz[1]
		cv := &countCanvas{w: w, h: h, blends: map[[2]int]int{}}
		StrokeRect(cv, 0, 0, w, h, Color{255, 255, 255, 100})
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				want := 0
				if x == 0 || y == 0 || x == w-1 || y == h-1 {
					want = 1
				}
				if got := cv.blends[[2]int{x, y}]; got != want {
					t.Errorf("%dx%d: pixel (%d,%d) blended %d times, want %d", w, h, x, y, got, want)
				}
			}
		}
	}
}

func TestFont3x5Width(t *testing.T) {
	f := Font3x5{}
	if w := f.TextWidth(0, 1); w != 0 {
		t.Errorf("empty width = %d, want 0", w)
	}
	if w := f.TextWidth(5, 2); w != 5*4*2-2 {
		t.Errorf("SNAKE@2 width = %d, want %d", w, 5*4*2-2)
	}
	if got := f.TextWidth(2, 3) - f.TextWidth(1, 3); got != f.Advance(3) {
		t.Errorf("Advance(3) = %d, DrawText steps %d", f.Advance(3), got)
	}
	if d := f.NumDigits(12345); d != 5 {
		t.Errorf("NumDigits(12345) = %d, want 5", d)
	}
}
