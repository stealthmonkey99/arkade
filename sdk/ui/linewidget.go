// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "math"

// LineSeg is a straight 1px line between two absolute points, drawn with [Line].
type LineSeg struct {
	X0, Y0, X1, Y1 int   // endpoints, in pixels; both are drawn
	Color          Color // line color
}

// Draw composites the line segment onto cv.
func (l LineSeg) Draw(cv Canvas) { Line(cv, l.X0, l.Y0, l.X1, l.Y1, l.Color) }

// LineFromAngle builds the [LineSeg] of the given length and color starting at
// (originX, originY) and running at angleDegrees: 0 degrees points up (-Y)
// and the angle turns clockwise (90 is +X, 180 is +Y, 270 is -X).
//
// Multiples of 90 use exact integer arithmetic. Other angles use math.Sin and
// math.Cos, which are not guaranteed bit-identical between TinyGo and host
// Go, so build the line when the angle changes and Draw the resulting value
// each frame rather than calling LineFromAngle per frame.
func LineFromAngle(originX, originY, length int, angleDegrees float64, c Color) LineSeg {
	dx, dy := angleOffset(length, angleDegrees)
	return LineSeg{X0: originX, Y0: originY, X1: originX + dx, Y1: originY + dy, Color: c}
}

// angleOffset returns the (dx, dy) endpoint offset for a line of the given
// length at angleDegrees, with 0 up and angles turning clockwise.
func angleOffset(length int, angleDegrees float64) (dx, dy int) {
	norm := math.Mod(angleDegrees, 360)
	if norm < 0 {
		norm += 360
	}
	switch norm {
	case 0:
		return 0, -length
	case 90:
		return length, 0
	case 180:
		return 0, length
	case 270:
		return -length, 0
	}
	radians := (angleDegrees - 90) * math.Pi / 180
	return int(math.Round(float64(length) * math.Cos(radians))),
		int(math.Round(float64(length) * math.Sin(radians)))
}
