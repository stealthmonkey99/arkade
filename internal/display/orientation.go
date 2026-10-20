// SPDX-License-Identifier: GPL-2.0-or-later

package display

// Orientation is a 90°-step rotation of the render surface.
type Orientation int

const (
	Upright Orientation = iota
	Clockwise
	UpsideDown
	CounterClockwise
)

// Rotate applies orientation to a square matrix (non-square input is
// returned unchanged for perpendicular rotations).
func Rotate(data Matrix, o Orientation) Matrix {
	if len(data) == 0 {
		return data
	}
	h := len(data)
	w := len(data[0])
	switch o {
	case Clockwise:
		if w != h {
			return data
		}
		out := make(Matrix, h)
		for y := 0; y < h; y++ {
			row := make([]Pixel, w)
			for x := 0; x < w; x++ {
				row[x] = data[w-1-x][y]
			}
			out[y] = row
		}
		return out
	case CounterClockwise:
		if w != h {
			return data
		}
		out := make(Matrix, h)
		for y := 0; y < h; y++ {
			row := make([]Pixel, w)
			for x := 0; x < w; x++ {
				row[x] = data[x][h-1-y]
			}
			out[y] = row
		}
		return out
	case UpsideDown:
		out := make(Matrix, h)
		for y := 0; y < h; y++ {
			row := make([]Pixel, len(data[h-1-y]))
			src := data[h-1-y]
			for x := range row {
				row[x] = src[len(src)-1-x]
			}
			out[y] = row
		}
		return out
	default:
		return data
	}
}
