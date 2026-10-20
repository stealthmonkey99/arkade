// SPDX-License-Identifier: GPL-2.0-or-later

package ui

// Circle is a filled and/or outlined circle, a plain value with a
// Draw(Canvas) method. It rasterizes with integer arithmetic only, as a
// symmetric disc of diameter 2R+1 centered exactly on (CX, CY). R < 0, or a
// transparent Fill and Stroke, draws nothing.
type Circle struct {
	CX, CY, R int // center and radius, in pixels
	// Fill paints the interior disc; leave it Transparent for outline only.
	Fill Color
	// Stroke paints a flat 1px outline at the circle's edge. Leave
	// Transparent for no border.
	Stroke Color
}

// Draw composites the circle onto cv.
func (c Circle) Draw(cv Canvas) {
	if c.R < 0 {
		return
	}
	doFill := c.Fill.A != 0
	doStroke := c.Stroke.A != 0
	if !doFill && !doStroke {
		return
	}
	if c.R == 0 {
		// Stroke wins if both are set, as in the loop below.
		if doFill {
			Set(cv, c.CX, c.CY, c.Fill)
		}
		if doStroke {
			Set(cv, c.CX, c.CY, c.Stroke)
		}
		return
	}

	r2 := c.R * c.R
	x := c.R
	for y := 0; y <= c.R; y++ {
		for x*x+y*y > r2 {
			x--
		}
		if doFill {
			HLine(cv, c.CX-x, c.CX+x, c.CY+y, c.Fill)
			if y != 0 {
				HLine(cv, c.CX-x, c.CX+x, c.CY-y, c.Fill)
			}
		}
		if doStroke {
			// x==0 on the pole rows would blend the same pixel twice, wrong for
			// a translucent stroke.
			Set(cv, c.CX+x, c.CY+y, c.Stroke)
			if x != 0 {
				Set(cv, c.CX-x, c.CY+y, c.Stroke)
			}
			if y != 0 {
				Set(cv, c.CX+x, c.CY-y, c.Stroke)
				if x != 0 {
					Set(cv, c.CX-x, c.CY-y, c.Stroke)
				}
			}
		}
	}
}
