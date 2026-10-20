// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

func TestLineSegDrawsViaLine(t *testing.T) {
	c := Color{255, 0, 0, 255}
	cvWidget := newByteCanvas(10, 10)
	cvFunc := newByteCanvas(10, 10)
	LineSeg{X0: 0, Y0: 0, X1: 8, Y1: 5, Color: c}.Draw(cvWidget)
	Line(cvFunc, 0, 0, 8, 5, c)
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if cvWidget.at(x, y) != cvFunc.at(x, y) {
				t.Fatalf("pixel (%d,%d): LineSeg=%v Line()=%v", x, y, cvWidget.at(x, y), cvFunc.at(x, y))
			}
		}
	}
}

// TestLineFromAngleCardinals pins the four right angles (0 is up, turning
// clockwise) on the integer path.
func TestLineFromAngleCardinals(t *testing.T) {
	cases := []struct {
		angle          float64
		wantDX, wantDY int
	}{
		{0, 0, -5},
		{90, 5, 0},
		{180, 0, 5},
		{270, -5, 0},
		{360, 0, -5}, // wraps to 0
		{-90, -5, 0}, // wraps to 270
		{450, 5, 0},  // wraps to 90
	}
	for _, tc := range cases {
		l := LineFromAngle(100, 100, 5, tc.angle, White)
		gotDX, gotDY := l.X1-l.X0, l.Y1-l.Y0
		if gotDX != tc.wantDX || gotDY != tc.wantDY {
			t.Errorf("angle %v: offset = (%d,%d), want (%d,%d)", tc.angle, gotDX, gotDY, tc.wantDX, tc.wantDY)
		}
		if l.X0 != 100 || l.Y0 != 100 {
			t.Errorf("angle %v: origin = (%d,%d), want (100,100)", tc.angle, l.X0, l.Y0)
		}
		if l.Color != White {
			t.Errorf("angle %v: color = %v, want White", tc.angle, l.Color)
		}
	}
}

// TestLineFromAngleDiagonal pins the trig fallback at a non-right angle.
// At 45 degrees the offset is up-and-right: cos(-45deg)=+0.7071,
// sin(-45deg)=-0.7071, so round(10*0.7071)=7 and round(10*-0.7071)=-7.
func TestLineFromAngleDiagonal(t *testing.T) {
	l := LineFromAngle(0, 0, 10, 45, Black)
	if l.X1 != 7 || l.Y1 != -7 {
		t.Errorf("45deg endpoint = (%d,%d), want (7,-7)", l.X1, l.Y1)
	}
}
