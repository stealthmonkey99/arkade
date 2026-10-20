// SPDX-License-Identifier: GPL-2.0-or-later

package element

import (
	"math"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
)

// Line rasterizes a line of a given length and angle (degrees, 0 = up,
// clockwise) with its own Bresenham: ui.Line breaks ties the other way on
// about a sixth of angles, and the clock face is pinned to this one.
type Line struct {
	Base
	color  display.Pixel
	length int
	angle  float64
	lineX  int
	lineY  int
}

// LineOptions configures a Line. Zero Length defaults to 10.
type LineOptions struct {
	Options
	Color  *display.Pixel
	Length int
	Angle  *float64
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// NewLine builds a line (defaults: red, length 10, angle 225°).
func NewLine(opts LineOptions) *Line {
	l := &Line{color: display.Red, length: 10, angle: 225}
	if opts.Color != nil {
		l.color = *opts.Color
	}
	if opts.Length != 0 {
		l.length = opts.Length
	}
	if opts.Angle != nil {
		l.angle = *opts.Angle
	}
	l.InitBase(opts.Options)
	l.Impl = l.renderImpl
	return l
}

func (l *Line) renderImpl(clock.FrameTime) display.Sprite {
	radians := (l.angle - 90) * math.Pi / 180
	x2 := int(math.Round(float64(l.length) * math.Cos(radians)))
	y2 := int(math.Round(float64(l.length) * math.Sin(radians)))
	dx, dy := x2, y2
	dx1, dy1 := abs(dx), abs(dy)
	ox := 0
	if x2 < 0 {
		ox = dx1
	}
	oy := 0
	if y2 < 0 {
		oy = dy1
	}
	l.lineX, l.lineY = -ox, -oy

	data := make(display.Sprite, dy1+1)
	for y := range data {
		row := make([]display.Pixel, dx1+1)
		for x := range row {
			row[x] = display.Transparent
		}
		data[y] = row
	}

	px := 2*dy1 - dx1
	py := 2*dx1 - dy1
	var x, y int
	if dy1 <= dx1 {
		var xe int
		if dx >= 0 {
			x, y, xe = 0, 0, x2
		} else {
			x, y, xe = x2, y2, 0
		}
		data[y+oy][x+ox] = l.color
		for x < xe {
			x++
			if px < 0 {
				px += 2 * dy1
			} else {
				if (dx < 0 && dy < 0) || (dx > 0 && dy > 0) {
					y++
				} else {
					y--
				}
				px += 2 * (dy1 - dx1)
			}
			data[y+oy][x+ox] = l.color
		}
	} else {
		var ye int
		if dy >= 0 {
			x, y, ye = 0, 0, y2
		} else {
			x, y, ye = x2, y2, 0
		}
		data[y+oy][x+ox] = l.color
		for y < ye {
			y++
			if py <= 0 {
				py += 2 * dx1
			} else {
				if (dx < 0 && dy < 0) || (dx > 0 && dy > 0) {
					x++
				} else {
					x--
				}
				py += 2 * (dx1 - dy1)
			}
			data[y+oy][x+ox] = l.color
		}
	}
	return data
}

// OffsetX uses the line's own logical origin rather than alignment.
func (l *Line) OffsetX() int { return l.offX + l.lineX }

// OffsetY uses the line's own logical origin rather than alignment.
func (l *Line) OffsetY() int { return l.offY + l.lineY }

func (l *Line) SetColor(v display.Pixel) {
	l.dirty = l.dirty || l.color != v
	l.color = v
}

func (l *Line) Length() int { return l.length }

func (l *Line) SetLength(v int) {
	l.dirty = l.dirty || l.length != v
	l.length = v
}

func (l *Line) Angle() float64 { return l.angle }

func (l *Line) SetAngle(v float64) {
	l.dirty = l.dirty || l.angle != v
	l.angle = v
}
