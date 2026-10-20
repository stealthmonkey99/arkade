// SPDX-License-Identifier: GPL-2.0-or-later

package element

import (
	"math"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
)

// Circle is a filled circle with an optional (beveled) border. It keeps its
// own rasterizer: ui.Circle is integer-exact and draws a different disc, and
// has no bevel.
type Circle struct {
	Base
	fill   display.Pixel
	border Border
	radius int
}

// CircleOptions configures a Circle. Zero Radius defaults to 5.
type CircleOptions struct {
	Options
	Radius int
	Fill   *display.Pixel
	Border *Border
}

// NewCircle builds a circle (defaults: radius 5, transparent fill, red border).
func NewCircle(opts CircleOptions) *Circle {
	c := &Circle{fill: display.Transparent, border: BorderOf(display.Red), radius: 5}
	if opts.Fill != nil {
		c.fill = *opts.Fill
	}
	if opts.Border != nil {
		c.border = *opts.Border
	}
	if opts.Radius != 0 {
		c.radius = opts.Radius
	}
	c.InitBase(opts.Options)
	c.width = c.radius*2 + 1
	c.height = c.radius*2 + 1
	c.Impl = c.renderImpl
	return c
}

func (c *Circle) renderImpl(clock.FrameTime) display.Sprite {
	size := c.radius*2 + 1
	data := make(display.Sprite, size)
	for y := range data {
		row := make([]display.Pixel, size)
		for x := range row {
			row[x] = display.Transparent
		}
		data[y] = row
	}

	r := float64(c.radius)
	for i := -r + 0.5; i <= r-0.5; i++ {
		for j := -r + 0.5; j <= r-0.5; j++ {
			dist := math.Round(math.Sqrt(i*i + j*j))
			if dist > r {
				continue
			}
			by := int(math.Floor(i)) + c.radius
			bx := int(math.Floor(j)) + c.radius
			if by < 0 || by >= len(data) || bx < 0 || bx >= len(data[by]) {
				continue
			}
			data[by][bx] = c.fill
			if dist == r && c.border.Kind != BorderNone {
				highlight, shadow := c.border.edgeColors(c.fill)
				data[by][bx] = highlight
				if -j < i {
					data[by][bx] = shadow
				}
			}
		}
	}
	return data
}

func (c *Circle) Fill() display.Pixel { return c.fill }

func (c *Circle) SetFill(v display.Pixel) {
	c.dirty = c.dirty || c.fill != v
	c.fill = v
}

func (c *Circle) SetBorder(v Border) {
	c.dirty = c.dirty || c.border != v
	c.border = v
}

func (c *Circle) Radius() int { return c.radius }

func (c *Circle) SetRadius(v int) {
	c.dirty = c.dirty || c.radius != v
	c.radius = v
}
