// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

// TestLoadingBarHalfProgress pins a 12x4 bar (10x2 interior) at Progress=0.5:
// border on the frame, fill on interior columns 1..5 (round(0.5*10)=5), and
// the unfilled remainder left untouched.
func TestLoadingBarHalfProgress(t *testing.T) {
	cv := newByteCanvas(12, 4)
	fill := Color{0, 200, 0, 255}
	border := White
	LoadingBar{X: 0, Y: 0, W: 12, H: 4, Progress: 0.5, Fill: fill, Border: border}.Draw(cv)

	for _, p := range [][2]int{{0, 0}, {11, 0}, {0, 3}, {11, 3}, {6, 0}, {6, 3}, {0, 2}, {11, 2}} {
		if got := cv.at(p[0], p[1]); got != border {
			t.Errorf("border pixel (%d,%d) = %v, want %v", p[0], p[1], got, border)
		}
	}
	for y := 1; y <= 2; y++ {
		for x := 1; x <= 5; x++ {
			if got := cv.at(x, y); got != fill {
				t.Errorf("filled pixel (%d,%d) = %v, want %v", x, y, got, fill)
			}
		}
		for x := 6; x <= 10; x++ {
			if got := cv.at(x, y); got != Black {
				t.Errorf("unfilled pixel (%d,%d) = %v, want untouched Black", x, y, got)
			}
		}
	}
}

func TestLoadingBarZeroProgress(t *testing.T) {
	cv := newByteCanvas(12, 4)
	fill := Color{0, 200, 0, 255}
	LoadingBar{X: 0, Y: 0, W: 12, H: 4, Progress: 0, Fill: fill, Border: White}.Draw(cv)
	for y := 1; y <= 2; y++ {
		for x := 1; x <= 10; x++ {
			if got := cv.at(x, y); got != Black {
				t.Errorf("pixel (%d,%d) = %v, want untouched Black at Progress=0", x, y, got)
			}
		}
	}
}

func TestLoadingBarFullProgress(t *testing.T) {
	cv := newByteCanvas(12, 4)
	fill := Color{0, 200, 0, 255}
	LoadingBar{X: 0, Y: 0, W: 12, H: 4, Progress: 1, Fill: fill, Border: White}.Draw(cv)
	for y := 1; y <= 2; y++ {
		for x := 1; x <= 10; x++ {
			if got := cv.at(x, y); got != fill {
				t.Errorf("pixel (%d,%d) = %v, want %v at Progress=1", x, y, got, fill)
			}
		}
	}
}

func TestLoadingBarProgressClamped(t *testing.T) {
	cv1 := newByteCanvas(12, 4)
	cv2 := newByteCanvas(12, 4)
	fill := Color{0, 200, 0, 255}
	LoadingBar{X: 0, Y: 0, W: 12, H: 4, Progress: -5, Fill: fill, Border: White}.Draw(cv1)
	LoadingBar{X: 0, Y: 0, W: 12, H: 4, Progress: 0, Fill: fill, Border: White}.Draw(cv2)
	for y := 0; y < 4; y++ {
		for x := 0; x < 12; x++ {
			if cv1.at(x, y) != cv2.at(x, y) {
				t.Fatalf("Progress=-5 pixel (%d,%d) = %v, want same as Progress=0 (%v)", x, y, cv1.at(x, y), cv2.at(x, y))
			}
		}
	}

	cv3 := newByteCanvas(12, 4)
	cv4 := newByteCanvas(12, 4)
	LoadingBar{X: 0, Y: 0, W: 12, H: 4, Progress: 5, Fill: fill, Border: White}.Draw(cv3)
	LoadingBar{X: 0, Y: 0, W: 12, H: 4, Progress: 1, Fill: fill, Border: White}.Draw(cv4)
	for y := 0; y < 4; y++ {
		for x := 0; x < 12; x++ {
			if cv3.at(x, y) != cv4.at(x, y) {
				t.Fatalf("Progress=5 pixel (%d,%d) = %v, want same as Progress=1 (%v)", x, y, cv3.at(x, y), cv4.at(x, y))
			}
		}
	}
}

func TestLoadingBarNoBorder(t *testing.T) {
	cv := newByteCanvas(12, 4)
	fill := Color{0, 200, 0, 255}
	LoadingBar{X: 0, Y: 0, W: 12, H: 4, Progress: 0.5, Fill: fill}.Draw(cv)
	if got := cv.at(0, 0); got != Black {
		t.Errorf("corner with no Border = %v, want untouched Black", got)
	}
}
