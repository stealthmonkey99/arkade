// SPDX-License-Identifier: GPL-2.0-or-later

package ui

// Rect is a filled and/or outlined rectangle. Like every widget it is a plain
// value with a Draw(Canvas) method: no retained state, no host calls, no
// allocation.
type Rect struct {
	X, Y, W, H int // bounds, in pixels; nothing is drawn when W or H is not positive
	// Fill paints the interior; leave it Transparent for an outline only.
	Fill Color
	// Stroke paints a border StrokeW pixels thick, inset into the rectangle
	// (so a Rect never draws outside W×H). Leave Transparent for no border.
	Stroke  Color
	StrokeW int // border thickness, in pixels; 0 or less draws no border, and it is capped at half the smaller side
}

// Draw composites the rectangle onto cv.
func (r Rect) Draw(cv Canvas) {
	if r.W <= 0 || r.H <= 0 {
		return
	}
	if r.Fill.A != 0 {
		FillRect(cv, r.X, r.Y, r.W, r.H, r.Fill)
	}
	if r.Stroke.A != 0 && r.StrokeW > 0 {
		t := r.StrokeW
		if t > r.W/2+r.W%2 {
			t = r.W/2 + r.W%2
		}
		if t > r.H/2+r.H%2 {
			t = r.H/2 + r.H%2
		}
		for i := 0; i < t; i++ {
			StrokeRect(cv, r.X+i, r.Y+i, r.W-2*i, r.H-2*i, r.Stroke)
		}
	}
}
