// SPDX-License-Identifier: GPL-2.0-or-later

package ui

// LoadingBar is a bordered horizontal progress bar: a 1px Border frame around
// (X, Y, W, H) and, in the interior, Progress (clamped to [0,1]) of the
// interior width filled from the left, rounded to the nearest pixel. The
// unfilled interior is left untouched so whatever is already drawn shows
// through; FillRect a track color first for a solid one. It is a plain value
// with a Draw(Canvas) method: no retained state, no allocation.
type LoadingBar struct {
	X, Y, W, H int // bounds including the frame, in pixels; nothing is drawn when W or H is not positive
	// Progress is the fraction filled, clamped to [0,1] before use.
	Progress float64
	// Fill paints the filled portion of the interior.
	Fill Color
	// Border paints the 1px frame. Leave Transparent for no border.
	Border Color
}

// Draw composites the loading bar onto cv.
func (b LoadingBar) Draw(cv Canvas) {
	if b.W <= 0 || b.H <= 0 {
		return
	}
	if b.Border.A != 0 {
		StrokeRect(cv, b.X, b.Y, b.W, b.H, b.Border)
	}
	innerX, innerY := b.X+1, b.Y+1
	innerW, innerH := b.W-2, b.H-2
	if innerW <= 0 || innerH <= 0 || b.Fill.A == 0 {
		return
	}
	p := b.Progress
	if p < 0 {
		p = 0
	} else if p > 1 {
		p = 1
	}
	filled := int(p*float64(innerW) + 0.5)
	if filled > innerW {
		filled = innerW
	}
	if filled > 0 {
		FillRect(cv, innerX, innerY, filled, innerH, b.Fill)
	}
}
