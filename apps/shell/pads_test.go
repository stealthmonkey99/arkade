// SPDX-License-Identifier: GPL-2.0-or-later

package shell

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

func pipSprite(seats [2]bool, loose int) display.Sprite {
	out := make(display.Sprite, padPipSize+2)
	for y := range out {
		out[y] = make([]display.Pixel, padPipsWidth+2)
	}
	drawPadPips(out, 0, 0, seats, loose)
	return out
}

func countPixel(art display.Sprite, want display.Pixel) int {
	n := 0
	for _, row := range art {
		for _, px := range row {
			if px == want {
				n++
			}
		}
	}
	return n
}

func TestPadPipsAlwaysShowBothSeats(t *testing.T) {
	none := pipSprite([2]bool{false, false}, 0)
	empty := display.Pixel{R: 70, G: 70, B: 78, A: 255}
	if countPixel(none, empty) == 0 {
		t.Fatal("no pips drawn when no pad is connected")
	}
	if none[2][2] == empty {
		t.Error("an empty seat is drawn filled; it should be an outline")
	}

	both := pipSprite([2]bool{true, true}, 0)
	if countPixel(both, padPipColors[0]) == 0 || countPixel(both, padPipColors[1]) == 0 {
		t.Error("both seats connected but not both pips are lit")
	}
	if both[2][2] != padPipColors[0] {
		t.Error("a connected seat is not drawn solid")
	}
}

func TestPadPipsUseTheSeatColours(t *testing.T) {
	p1 := pipSprite([2]bool{true, false}, 0)
	if countPixel(p1, padPipColors[0]) == 0 {
		t.Error("P1 connected but its red pip is not lit")
	}
	if countPixel(p1, padPipColors[1]) != 0 {
		t.Error("P2 is not connected but its blue pip is lit")
	}

	p2 := pipSprite([2]bool{false, true}, 0)
	if countPixel(p2, padPipColors[1]) == 0 {
		t.Error("P2 connected but its blue pip is not lit")
	}
	if countPixel(p2, padPipColors[0]) != 0 {
		t.Error("P1 is not connected but its red pip is lit")
	}
}

func TestPadPipsFitTheTile(t *testing.T) {
	for _, loose := range []int{0, 1} {
		if w := padPipsWidth; w >= screenW/2 {
			t.Errorf("the pips are %dpx of a %dpx tile (loose=%d); that is a label with nowhere to go", w, screenW, loose)
		}
	}
}

// A newly woken pad attaches beyond seat 1 and must still show.
func TestAnAwakePadWithNoSeatBorrowsAFreeSquare(t *testing.T) {
	art := pipSprite([2]bool{false, false}, 1)
	if art[2][2] != padLooseColor {
		t.Error("an awake unseated pad did not fill the first free square")
	}
	if countPixel(art, padEmptyColor) == 0 {
		t.Error("the other seat stopped showing as an outline")
	}

	art = pipSprite([2]bool{true, false}, 1)
	if art[2][2] != padPipColors[0] {
		t.Error("P1's own square stopped showing its seat colour")
	}
	if art[2][padPipSize+padPipGap+2] != padLooseColor {
		t.Error("the unseated pad did not fall through to P2's square")
	}

	art = pipSprite([2]bool{true, true}, 1)
	if countPixel(art, padLooseColor) != 0 {
		t.Error("a grey square was drawn with both seats already accounted for")
	}
}

func TestSeatingAPadReplacesTheGreySquare(t *testing.T) {
	before := pipSprite([2]bool{false, false}, 1)
	if countPixel(before, padPipColors[0]) != 0 {
		t.Fatal("P1 is lit before the pad was seated")
	}
	after := pipSprite([2]bool{true, false}, 0)
	if countPixel(after, padLooseColor) != 0 {
		t.Error("the grey survived the pad being seated")
	}
	if countPixel(after, padPipColors[0]) == 0 {
		t.Error("P1 is not lit after the pad took that seat")
	}
}

// BLUETOOTH fits beside the pips exactly; one more pixel would make it scroll.
func TestBluetoothLabelFitsBesideThePips(t *testing.T) {
	w := ui.FontLED{}.TextWidth("BLUETOOTH", 1)
	win := sliderRightX - padPipsWidth - 2 - labelX
	if w > win {
		t.Errorf("BLUETOOTH needs %dpx and has %dpx; the tile will scroll a label that should be still", w, win)
	}
}
