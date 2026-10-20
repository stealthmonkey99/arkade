// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

func TestButtonFocusedVsUnfocusedDiffer(t *testing.T) {
	unfocused := drawnGrid(24, 12, func(cv Canvas) {
		Button{X: 2, Y: 2, W: 18, H: 8, Text: "OK", Focused: false}.Draw(cv)
	})
	focused := drawnGrid(24, 12, func(cv Canvas) {
		Button{X: 2, Y: 2, W: 18, H: 8, Text: "OK", Focused: true}.Draw(cv)
	})
	if gridsEqual(unfocused, focused) {
		t.Fatal("a focused Button should render differently than an unfocused one")
	}
}

// Sampled at an interior pixel the bevel leaves as plain Fill.
func TestButtonDefaultFillColors(t *testing.T) {
	unfocused := newGridCanvas(20, 10)
	Button{X: 0, Y: 0, W: 20, H: 10}.Draw(unfocused)
	if got := unfocused.at(10, 5); got != buttonFill {
		t.Errorf("unfocused interior pixel = %v, want buttonFill %v", got, buttonFill)
	}

	focused := newGridCanvas(20, 10)
	Button{X: 0, Y: 0, W: 20, H: 10, Focused: true}.Draw(focused)
	if got := focused.at(10, 5); got != buttonFocusFill {
		t.Errorf("focused interior pixel = %v, want buttonFocusFill %v", got, buttonFocusFill)
	}
}

func TestButtonExplicitColorsOverrideDefaults(t *testing.T) {
	custom := Color{10, 20, 30, 255}
	customFocus := Color{200, 20, 30, 255}
	cv := newGridCanvas(20, 10)
	Button{X: 0, Y: 0, W: 20, H: 10, Fill: custom, FocusFill: customFocus}.Draw(cv)
	if got := cv.at(10, 5); got != custom {
		t.Errorf("unfocused interior pixel = %v, want explicit Fill %v", got, custom)
	}

	cv2 := newGridCanvas(20, 10)
	Button{X: 0, Y: 0, W: 20, H: 10, Focused: true, Fill: custom, FocusFill: customFocus}.Draw(cv2)
	if got := cv2.at(10, 5); got != customFocus {
		t.Errorf("focused interior pixel = %v, want explicit FocusFill %v", got, customFocus)
	}
}

func TestButtonTextDraws(t *testing.T) {
	withText := drawnGrid(20, 10, func(cv Canvas) {
		Button{X: 0, Y: 0, W: 20, H: 10, Text: "OK"}.Draw(cv)
	})
	withoutText := drawnGrid(20, 10, func(cv Canvas) {
		Button{X: 0, Y: 0, W: 20, H: 10}.Draw(cv)
	})
	if gridsEqual(withText, withoutText) {
		t.Fatal("a non-empty Text should draw label pixels distinct from an empty Button")
	}
}

func TestButtonZeroSizeIsNoop(t *testing.T) {
	cv := newGridCanvas(10, 10)
	Button{X: 0, Y: 0, W: 0, H: 5, Text: "X"}.Draw(cv)
	Button{X: 0, Y: 0, W: 5, H: 0, Text: "X"}.Draw(cv)
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if got := cv.at(x, y); got != Black {
				t.Fatalf("pixel (%d,%d) = %v after zero-size Draw, want untouched Black", x, y, got)
			}
		}
	}
}

func TestButtonSubstrateParity(t *testing.T) {
	bc := newByteCanvas(24, 12)
	gc := newGridCanvas(24, 12)
	draw := func(cv Canvas) { Button{X: 2, Y: 2, W: 18, H: 8, Text: "OK", Focused: true}.Draw(cv) }
	draw(bc)
	draw(gc)
	for y := 0; y < 12; y++ {
		for x := 0; x < 24; x++ {
			if bc.at(x, y) != gc.at(x, y) {
				t.Fatalf("pixel (%d,%d): byteCanvas=%v gridCanvas=%v", x, y, bc.at(x, y), gc.at(x, y))
			}
		}
	}
}

func TestButtonTextDY(t *testing.T) {
	render := func(dy int) *Layer {
		lay := NewLayer(48, 16)
		lay.Fill(Color{0, 0, 0, 255})
		Button{X: 2, Y: 2, W: 44, H: 10, Text: "TEXT", TextDY: dy}.Draw(lay)
		return lay
	}
	base := render(0)
	down := render(1)
	for y := 3; y < 11; y++ {
		for x := 4; x < 44; x++ {
			if base.At(x, y) == Black && down.At(x, y+1) != Black {
				t.Fatalf("label pixel (%d,%d) did not shift down 1 with TextDY=1", x, y)
			}
			if down.At(x, y+1) == Black && base.At(x, y) != Black {
				t.Fatalf("unexpected label pixel at (%d,%d) with TextDY=1", x, y+1)
			}
		}
	}
}
