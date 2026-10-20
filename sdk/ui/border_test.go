// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

func TestBorderEdgeBevel(t *testing.T) {
	fill := Color{40, 60, 200, 255}
	cv := newGridCanvas(6, 6)
	Border{X: 0, Y: 0, W: 6, H: 6, Fill: fill, Kind: BorderEdge}.Draw(cv)

	wantHi := BlendOver(fill, Highlight)
	wantSh := BlendOver(fill, Shadow)

	for x := 0; x < 6; x++ {
		if cv.at(x, 0) != wantHi {
			t.Errorf("top row (%d,0) = %v, want highlight %v", x, cv.at(x, 0), wantHi)
		}
	}
	for x := 0; x < 6; x++ {
		if cv.at(x, 5) != wantSh {
			t.Errorf("bottom row (%d,5) = %v, want shadow %v", x, cv.at(x, 5), wantSh)
		}
	}
	for y := 1; y < 5; y++ {
		if cv.at(0, y) != wantHi {
			t.Errorf("left col (0,%d) = %v, want highlight %v", y, cv.at(0, y), wantHi)
		}
		if cv.at(5, y) != wantSh {
			t.Errorf("right col (5,%d) = %v, want shadow %v", y, cv.at(5, y), wantSh)
		}
		for x := 1; x < 5; x++ {
			if cv.at(x, y) != fill {
				t.Errorf("interior (%d,%d) = %v, want fill %v", x, y, cv.at(x, y), fill)
			}
		}
	}
}

func TestBorderSmallHeightHasNoHighlight(t *testing.T) {
	fill := Color{10, 20, 30, 255}
	wantSh := BlendOver(fill, Shadow)
	wantHi := BlendOver(fill, Highlight)

	for _, h := range []int{1, 2, 3} {
		cv := newGridCanvas(4, h)
		Border{X: 0, Y: 0, W: 4, H: h, Fill: fill, Kind: BorderEdge}.Draw(cv)
		for y := 0; y < h; y++ {
			for x := 0; x < 4; x++ {
				got := cv.at(x, y)
				if got == wantHi {
					t.Errorf("H=%d: (%d,%d) is highlighted; H<=3 should never draw highlight", h, x, y)
				}
			}
			if y == h-1 {
				for x := 0; x < 4; x++ {
					if cv.at(x, y) != wantSh {
						t.Errorf("H=%d: bottom row (%d,%d) = %v, want shadow", h, x, y, cv.at(x, y))
					}
				}
			} else {
				if cv.at(3, y) != wantSh {
					t.Errorf("H=%d: right col (3,%d) = %v, want shadow", h, y, cv.at(3, y))
				}
				for x := 0; x < 3; x++ {
					if cv.at(x, y) != fill {
						t.Errorf("H=%d: (%d,%d) = %v, want fill (untouched)", h, x, y, cv.at(x, y))
					}
				}
			}
		}
	}
}

func TestBorderDarkEdgeDeeperShadow(t *testing.T) {
	fill := White
	edgeSh := BlendOver(fill, Shadow)
	darkSh := BlendOver(fill, DarkShadow)
	if edgeSh == darkSh {
		t.Fatal("test setup: Shadow and DarkShadow should blend to different colors over white")
	}

	cv := newGridCanvas(6, 6)
	Border{X: 0, Y: 0, W: 6, H: 6, Fill: fill, Kind: BorderDarkEdge}.Draw(cv)
	if got := cv.at(0, 5); got != darkSh {
		t.Errorf("BorderDarkEdge bottom row = %v, want DarkShadow blend %v", got, darkSh)
	}
}

func TestBorderColorKindIsUniform(t *testing.T) {
	fill := Color{0, 100, 0, 255}
	edge := Color{255, 0, 0, 255}
	cv := newGridCanvas(6, 6)
	Border{X: 0, Y: 0, W: 6, H: 6, Fill: fill, Kind: BorderColor, Color: edge}.Draw(cv)

	for x := 0; x < 6; x++ {
		if cv.at(x, 0) != edge || cv.at(x, 5) != edge {
			t.Errorf("BorderColor edge should be flat %v at (%d,0)/(%d,5), got %v/%v", edge, x, x, cv.at(x, 0), cv.at(x, 5))
		}
	}
	for y := 1; y < 5; y++ {
		if cv.at(0, y) != edge || cv.at(5, y) != edge {
			t.Errorf("BorderColor side columns should be flat %v at row %d", edge, y)
		}
	}
}

func TestBorderNoneDrawsOnlyFill(t *testing.T) {
	fill := Color{5, 6, 7, 255}
	cv := newGridCanvas(4, 4)
	Border{X: 0, Y: 0, W: 4, H: 4, Fill: fill, Kind: BorderNone}.Draw(cv)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if cv.at(x, y) != fill {
				t.Errorf("BorderNone (%d,%d) = %v, want plain fill %v", x, y, cv.at(x, y), fill)
			}
		}
	}
}

func TestBorderTransparentFillUsesGrayBevelBase(t *testing.T) {
	cv := newGridCanvas(6, 6) // starts opaque Black
	Border{X: 0, Y: 0, W: 6, H: 6, Fill: Transparent, Kind: BorderEdge}.Draw(cv)

	wantHi := BlendOver(borderBase, Highlight)
	if got := cv.at(0, 0); got != wantHi {
		t.Errorf("transparent-fill bevel top row = %v, want gray-base highlight %v", got, wantHi)
	}
	// Fill.A==0 skips FillRect, so the interior keeps the starting color.
	if got := cv.at(2, 2); got != Black {
		t.Errorf("transparent-fill interior = %v, want untouched canvas background %v", got, Black)
	}
}
