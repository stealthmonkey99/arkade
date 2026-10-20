// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

// drawnGrid renders draw onto a fresh gridCanvas and returns it, for
// pixel-by-pixel comparisons against a hand-built expectation.
func drawnGrid(w, h int, draw func(cv Canvas)) *gridCanvas {
	cv := newGridCanvas(w, h)
	draw(cv)
	return cv
}

func gridsEqual(a, b *gridCanvas) bool {
	if a.w != b.w || a.h != b.h {
		return false
	}
	for y := 0; y < a.h; y++ {
		for x := 0; x < a.w; x++ {
			if a.at(x, y) != b.at(x, y) {
				return false
			}
		}
	}
	return true
}

func TestLabelLeftAlignMatchesFontLED(t *testing.T) {
	got := drawnGrid(16, 8, func(cv Canvas) {
		Label{X: 5, Y: 1, Text: "I", Color: White}.Draw(cv)
	})
	want := drawnGrid(16, 8, func(cv Canvas) {
		FontLED{}.DrawText(cv, 5, 1, "I", 1, White)
	})
	if !gridsEqual(got, want) {
		t.Error("Label default (AlignLeft) should draw exactly where FontLED.DrawText does")
	}
}

func TestLabelDefaultScaleIsOne(t *testing.T) {
	got := drawnGrid(16, 8, func(cv Canvas) {
		Label{X: 0, Y: 0, Text: "I", Color: White, Scale: 0}.Draw(cv)
	})
	want := drawnGrid(16, 8, func(cv Canvas) {
		Label{X: 0, Y: 0, Text: "I", Color: White, Scale: 1}.Draw(cv)
	})
	if !gridsEqual(got, want) {
		t.Error("Scale 0 should render identically to Scale 1")
	}
}

func TestLabelAlignCenterAndRight(t *testing.T) {
	// "HI" is 6px wide at scale 1 (H=4, I=2, no inter-glyph gap).
	const width = 6

	right := drawnGrid(32, 8, func(cv Canvas) {
		Label{X: 20, Y: 0, Text: "HI", Color: White, Align: AlignRight}.Draw(cv)
	})
	wantRight := drawnGrid(32, 8, func(cv Canvas) {
		FontLED{}.DrawText(cv, 20-width, 0, "HI", 1, White)
	})
	if !gridsEqual(right, wantRight) {
		t.Error("AlignRight should end the text at X")
	}

	center := drawnGrid(32, 8, func(cv Canvas) {
		Label{X: 20, Y: 0, Text: "HI", Color: White, Align: AlignCenter}.Draw(cv)
	})
	wantCenter := drawnGrid(32, 8, func(cv Canvas) {
		FontLED{}.DrawText(cv, 20-width/2, 0, "HI", 1, White)
	})
	if !gridsEqual(center, wantCenter) {
		t.Error("AlignCenter should center the text on X")
	}
}

func TestLabelCentersFont3x5LikeAHandCenteredDraw(t *testing.T) {
	f := Font3x5{}
	got := drawnGrid(64, 16, func(cv Canvas) {
		Label{X: 32, Y: 3, Text: "SNAKE", Scale: 2, Align: AlignCenter, Font: f, Color: White}.Draw(cv)
	})
	want := drawnGrid(64, 16, func(cv Canvas) {
		f.DrawText(cv, (64-f.TextWidth(len("SNAKE"), 2))/2, 3, "SNAKE", 2, White)
	})
	if !gridsEqual(got, want) {
		t.Error("a centred Font3x5 Label should draw where DrawText at the centred x does")
	}
}

func TestLabelFontsMeasureWhatTheyDraw(t *testing.T) {
	for _, tc := range []struct {
		name string
		font LabelFont
	}{{"FontLED", FontLED{}}, {"Font3x5", Font3x5{}}} {
		for scale := 1; scale <= 3; scale++ {
			cv := drawnGrid(96, 32, func(cv Canvas) { tc.font.DrawText(cv, 0, 0, "HELLO 42", scale, White) })
			right := -1
			for x := 0; x < cv.w; x++ {
				for y := 0; y < cv.h; y++ {
					if cv.at(x, y) != Black {
						right = x
					}
				}
			}
			if got := tc.font.MeasureText("HELLO 42", scale); right >= got || right < got-2*scale {
				t.Errorf("%s scale %d: MeasureText = %d, rightmost lit column %d", tc.name, scale, got, right)
			}
		}
	}
}

func TestLabelEmptyTextAndTransparentColorNoop(t *testing.T) {
	blank := newGridCanvas(8, 8)

	empty := drawnGrid(8, 8, func(cv Canvas) {
		Label{X: 0, Y: 0, Text: "", Color: White}.Draw(cv)
	})
	if !gridsEqual(empty, blank) {
		t.Error("empty Text should draw nothing")
	}

	transparent := drawnGrid(8, 8, func(cv Canvas) {
		Label{X: 0, Y: 0, Text: "I", Color: Transparent}.Draw(cv)
	})
	if !gridsEqual(transparent, blank) {
		t.Error("Transparent Color should draw nothing")
	}
}
