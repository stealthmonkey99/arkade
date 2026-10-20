// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

// TestCircleFillOnly pins the Circle{R: 3} disc: row 0 spans x in [-3,3],
// rows ±1 and ±2 span [-2,2], and rows ±3 are a single pixel.
func TestCircleFillOnly(t *testing.T) {
	cv := newByteCanvas(21, 21)
	cx, cy := 10, 10
	fill := Color{0, 200, 0, 255}
	Circle{CX: cx, CY: cy, R: 3, Fill: fill}.Draw(cv)

	lit := map[[2]int]bool{}
	rows := map[int]int{0: 3, 1: 2, 2: 2, 3: 0}
	for dy, ext := range rows {
		for dx := -ext; dx <= ext; dx++ {
			lit[[2]int{dx, dy}] = true
			if dy != 0 {
				lit[[2]int{dx, -dy}] = true
			}
		}
	}
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			got := cv.at(cx+dx, cy+dy)
			want := Black // opaque black background, untouched
			if lit[[2]int{dx, dy}] {
				want = fill
			}
			if got != want {
				t.Errorf("pixel (%d,%d) [offset %d,%d] = %v, want %v", cx+dx, cy+dy, dx, dy, got, want)
			}
		}
	}
	if got := cv.at(cx+3, cy+3); got != Black {
		t.Errorf("corner (%d,%d) = %v, want untouched Black", cx+3, cy+3, got)
	}
}

func TestCircleStrokeOnly(t *testing.T) {
	cv := newByteCanvas(21, 21)
	cx, cy := 10, 10
	stroke := Color{255, 0, 0, 255}
	Circle{CX: cx, CY: cy, R: 3, Stroke: stroke}.Draw(cv)

	want := [][2]int{
		{-3, 0}, {3, 0},
		{-2, 1}, {2, 1}, {-2, -1}, {2, -1},
		{-2, 2}, {2, 2}, {-2, -2}, {2, -2},
		{0, 3}, {0, -3},
	}
	wantSet := map[[2]int]bool{}
	for _, p := range want {
		wantSet[p] = true
	}
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			got := cv.at(cx+dx, cy+dy)
			if wantSet[[2]int{dx, dy}] {
				if got != stroke {
					t.Errorf("edge pixel (%d,%d) = %v, want stroke %v", dx, dy, got, stroke)
				}
			} else if got != Black {
				t.Errorf("non-edge pixel (%d,%d) = %v, want untouched Black", dx, dy, got)
			}
		}
	}
}

// Stroke draws after Fill, so an edge pixel ends up Stroke-colored.
func TestCircleFillAndStroke(t *testing.T) {
	cv := newByteCanvas(21, 21)
	cx, cy := 10, 10
	fill := Color{0, 200, 0, 255}
	stroke := Color{255, 0, 0, 255}
	Circle{CX: cx, CY: cy, R: 3, Fill: fill, Stroke: stroke}.Draw(cv)

	if got := cv.at(cx, cy); got != fill {
		t.Errorf("center = %v, want fill %v", got, fill)
	}
	if got := cv.at(cx+3, cy); got != stroke {
		t.Errorf("right edge = %v, want stroke %v", got, stroke)
	}
	if got := cv.at(cx, cy+3); got != stroke {
		t.Errorf("bottom cap = %v, want stroke %v (single pixel, not double-blended)", got, stroke)
	}
}

// A zero radius is a single pixel; Stroke wins when both are set.
func TestCircleZeroRadius(t *testing.T) {
	cv := newByteCanvas(5, 5)
	fill := Color{0, 200, 0, 255}
	stroke := Color{255, 0, 0, 255}
	Circle{CX: 2, CY: 2, R: 0, Fill: fill, Stroke: stroke}.Draw(cv)
	if got := cv.at(2, 2); got != stroke {
		t.Errorf("zero-radius pixel = %v, want stroke %v", got, stroke)
	}
	if got := cv.at(1, 2); got != Black {
		t.Errorf("neighbor pixel = %v, want untouched Black", got)
	}
}

func TestCircleTransparentIsNoop(t *testing.T) {
	cv := newByteCanvas(9, 9)
	Circle{CX: 4, CY: 4, R: 3}.Draw(cv)
	for y := 0; y < 9; y++ {
		for x := 0; x < 9; x++ {
			if got := cv.at(x, y); got != Black {
				t.Fatalf("pixel (%d,%d) = %v, want untouched Black", x, y, got)
			}
		}
	}
}
