// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

// TestButtonDotIsTheEnginesDisc pins the 7px disc's row widths to
// {3,5,7,7,7,5,3}.
func TestButtonDotIsTheEnginesDisc(t *testing.T) {
	const pad = 2
	cv := newByteCanvas(ButtonDotSize+2*pad, ButtonDotSize+2*pad)
	ButtonDot{X: pad, Y: pad, Color: ButtonGreen}.Draw(cv)

	want := [ButtonDotSize]int{3, 5, 7, 7, 7, 5, 3}
	for y := 0; y < ButtonDotSize; y++ {
		first, last, n := -1, -1, 0
		for x := 0; x < ButtonDotSize; x++ {
			if cv.at(pad+x, pad+y) == ButtonGreen {
				if first < 0 {
					first = x
				}
				last = x
				n++
			}
		}
		if n != want[y] {
			t.Errorf("row %d has %d lit pixels, want %d", y, n, want[y])
		}
		if n > 0 && last-first+1 != n {
			t.Errorf("row %d is not a contiguous run (%d..%d, %d lit)", y, first, last, n)
		}
		if n > 0 && first != (ButtonDotSize-n)/2 {
			t.Errorf("row %d starts at %d, want centred at %d", y, first, (ButtonDotSize-n)/2)
		}
	}
	// Corners stay clear, which is what makes it read round.
	for _, c := range [][2]int{{0, 0}, {ButtonDotSize - 1, 0}, {0, ButtonDotSize - 1}, {ButtonDotSize - 1, ButtonDotSize - 1}} {
		if got := cv.at(pad+c[0], pad+c[1]); got != Black {
			t.Errorf("corner (%d,%d) = %v, want untouched", c[0], c[1], got)
		}
	}
	// Nothing escapes the cell.
	for y := 0; y < ButtonDotSize+2*pad; y++ {
		for x := 0; x < ButtonDotSize+2*pad; x++ {
			inCell := x >= pad && x < pad+ButtonDotSize && y >= pad && y < pad+ButtonDotSize
			if !inCell && cv.at(x, y) != Black {
				t.Fatalf("pixel (%d,%d) outside the %dpx cell was painted", x, y, ButtonDotSize)
			}
		}
	}
}

// TestStickHintIsAChevronPair pins two 3x5 arrows pointing apart, in the
// disc's cell width.
func TestStickHintIsAChevronPair(t *testing.T) {
	cv := newByteCanvas(StickHintSize, chevronH)
	StickHint{Color: White}.Draw(cv)

	want := []string{
		"..#.#..",
		".#...#.",
		"#.....#",
		".#...#.",
		"..#.#..",
	}
	for y, row := range want {
		for x := 0; x < StickHintSize; x++ {
			lit := cv.at(x, y) == White
			if lit != (row[x] == '#') {
				t.Errorf("stick pixel (%d,%d) lit=%v, want %q", x, y, lit, row[x])
			}
		}
	}
	if StickHintSize != ButtonDotSize {
		t.Errorf("stick cell is %d wide, disc cell is %d — rows would jog", StickHintSize, ButtonDotSize)
	}
}

// auxInk counts painted pixels in the upper (rows 0..2) and lower (rows
// 4..6) halves of an aux cell, and how many of each are the lit colour.
func auxInk(t *testing.T, lower bool) (upper, lowerN, upperLit, lowerLit int) {
	t.Helper()
	cv := newByteCanvas(ButtonDotSize, ButtonDotSize)
	AuxDot{Lower: lower}.Draw(cv)
	for y := 0; y < ButtonDotSize; y++ {
		for x := 0; x < ButtonDotSize; x++ {
			c := cv.at(x, y)
			if c == Black {
				continue
			}
			if y == auxOvalH {
				t.Errorf("row %d is meant to be the gap between the two aux buttons, but (%d,%d) is painted", y, x, y)
			}
			if y < auxOvalH {
				upper++
				if c == ButtonAux {
					upperLit++
				}
			} else {
				lowerN++
				if c == ButtonAux {
					lowerLit++
				}
			}
		}
	}
	return upper, lowerN, upperLit, lowerLit
}

// TestAuxDotDistinguishesTheTwoWhiteButtons: aux1's upper half carries more
// ink than its lower, aux2's the reverse, with every lit pixel on the
// referenced side.
func TestAuxDotDistinguishesTheTwoWhiteButtons(t *testing.T) {
	up1, low1, upLit1, lowLit1 := auxInk(t, false) // joyaux1 — upper
	up2, low2, upLit2, lowLit2 := auxInk(t, true)  // joyaux2 — lower

	if up1 <= low1 {
		t.Errorf("aux1: upper half has %d painted pixels, lower %d — the upper must read heavier", up1, low1)
	}
	if low2 <= up2 {
		t.Errorf("aux2: lower half has %d painted pixels, upper %d — the lower must read heavier", low2, up2)
	}
	// The variants are mirror images: same total ink, swapped halves.
	if up1 != low2 || low1 != up2 {
		t.Errorf("aux1 halves (%d,%d) are not the mirror of aux2's (%d,%d)", up1, low1, up2, low2)
	}
	// The lit disc is entirely on the referenced side.
	if upLit1 == 0 || lowLit1 != 0 {
		t.Errorf("aux1 lit pixels: upper=%d lower=%d, want all upper", upLit1, lowLit1)
	}
	if lowLit2 == 0 || upLit2 != 0 {
		t.Errorf("aux2 lit pixels: upper=%d lower=%d, want all lower", upLit2, lowLit2)
	}
	// The dim half is an outline: same silhouette, hollow centre.
	if low1 >= up1 || low1 == 0 {
		t.Errorf("aux1 unlit half has %d pixels against the lit half's %d; want a hollowed-out copy", low1, up1)
	}
}

// TestAuxDotFitsAnOrdinaryHintCell is the layout constraint: an aux hint
// has to drop into a row beside ordinary discs without moving anything.
func TestAuxDotFitsAnOrdinaryHintCell(t *testing.T) {
	const pad = 3
	cv := newByteCanvas(ButtonDotSize+2*pad, ButtonDotSize+2*pad)
	AuxDot{X: pad, Y: pad, Lower: true}.Draw(cv)
	for y := 0; y < ButtonDotSize+2*pad; y++ {
		for x := 0; x < ButtonDotSize+2*pad; x++ {
			inCell := x >= pad && x < pad+ButtonDotSize && y >= pad && y < pad+ButtonDotSize
			if !inCell && cv.at(x, y) != Black {
				t.Fatalf("aux pixel (%d,%d) escaped the %dpx cell", x, y, ButtonDotSize)
			}
		}
	}
	aux := Hint{Text: "MED", Control: HintAux1}
	btn := Hint{Text: "MED", Color: ButtonBlue}
	if HintWidth(aux) != HintWidth(btn) {
		t.Errorf("aux hint is %d wide, button hint %d — a row would jog", HintWidth(aux), HintWidth(btn))
	}
}

// TestHintRowLaysOutControlThenLabel pins the row geometry: it is exactly
// as wide as HintRowWidth promised, each control sits in its own cell, and
// the labels take the caller's colour while the controls keep their own.
func TestHintRowLaysOutControlThenLabel(t *testing.T) {
	hints := []Hint{
		{Text: "GO", Color: ButtonGreen},
		{Text: "BACK", Color: ButtonRed},
	}
	label := Color{200, 200, 200, 255}
	row := HintRow{LabelColor: label, Hints: hints}
	w := row.Width()
	if w != HintRowWidth(hints) {
		t.Fatalf("Width()=%d, HintRowWidth=%d", w, HintRowWidth(hints))
	}
	cv := newByteCanvas(w+4, HintRowHeight)
	row.Draw(cv)

	// Column 3 (the disc's widest rows) of each cell carries the control's
	// own colour; the label ink is the caller's colour, never a control's.
	x0 := 0
	for _, h := range hints {
		if got := cv.at(x0+3, 2); got != h.Color {
			t.Errorf("hint %q: cell pixel = %v, want the control's own %v", h.Text, got, h.Color)
		}
		x0 += HintWidth(h) + hintPairGap
	}
	if x0-hintPairGap != w {
		t.Errorf("cells advanced to %d, row is %d wide", x0-hintPairGap, w)
	}
	sawLabel := false
	for y := 0; y < HintRowHeight; y++ {
		for x := 0; x < w; x++ {
			c := cv.at(x, y)
			if c == label {
				sawLabel = true
			}
			if c != Black && c != label && c != ButtonGreen && c != ButtonRed {
				t.Fatalf("pixel (%d,%d) = %v: neither background, label ink, nor a control colour", x, y, c)
			}
		}
	}
	if !sawLabel {
		t.Error("no label ink in the row")
	}
	// Nothing may spill past the measured width.
	for y := 0; y < HintRowHeight; y++ {
		for x := w; x < w+4; x++ {
			if cv.at(x, y) != Black {
				t.Fatalf("row painted (%d,%d), past its %dpx width", x, y, w)
			}
		}
	}
}

func TestHintRowEmpty(t *testing.T) {
	if got := HintRowWidth(nil); got != 0 {
		t.Errorf("HintRowWidth(nil) = %d, want 0", got)
	}
	cv := newByteCanvas(8, 8)
	HintRow{LabelColor: White}.Draw(cv)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if cv.at(x, y) != Black {
				t.Fatalf("empty row painted (%d,%d)", x, y)
			}
		}
	}
}

// TestDrawHintReturnsTheNextCell: two hints drawn by hand land the same
// pixels as a HintRow over the same two.
func TestDrawHintReturnsTheNextCell(t *testing.T) {
	hints := []Hint{
		{Text: "SET", Color: White, Control: HintStick},
		{Text: "MED", Control: HintAux2},
	}
	label := White
	w := HintRowWidth(hints)

	byRow := newByteCanvas(w, HintRowHeight)
	HintRow{LabelColor: label, Hints: hints}.Draw(byRow)

	byHand := newByteCanvas(w, HintRowHeight)
	x := 0
	for _, h := range hints {
		x = DrawHint(byHand, x, 0, label, h)
	}
	if x-hintPairGap != w {
		t.Errorf("DrawHint advanced to %d, HintRowWidth says %d", x-hintPairGap, w)
	}
	for y := 0; y < HintRowHeight; y++ {
		for px := 0; px < w; px++ {
			if byRow.at(px, y) != byHand.at(px, y) {
				t.Fatalf("pixel (%d,%d): row=%v hand=%v", px, y, byRow.at(px, y), byHand.at(px, y))
			}
		}
	}
}

func TestChevronPointsEitherWay(t *testing.T) {
	left := newByteCanvas(chevronW, chevronH)
	Chevron{Color: White}.Draw(left)
	right := newByteCanvas(chevronW, chevronH)
	Chevron{Color: White, Right: true}.Draw(right)
	for y := 0; y < chevronH; y++ {
		for x := 0; x < chevronW; x++ {
			if left.at(x, y) != right.at(chevronW-1-x, y) {
				t.Fatalf("pixel (%d,%d): right chevron is not the mirror of left", x, y)
			}
		}
	}
	if left.at(0, 2) != White || left.at(2, 0) != White || left.at(2, 4) != White {
		t.Error("left chevron should point left: tip at (0,2), tails at the right corners")
	}
	none := newByteCanvas(chevronW, chevronH)
	Chevron{}.Draw(none)
	if none.at(0, 2) != Black {
		t.Error("a transparent chevron must draw nothing")
	}
}

func TestStickHintSitsOnTheLabelsInkLine(t *testing.T) {
	h := Hint{Text: "VOL", Color: White, Control: HintStick}
	cv := newByteCanvas(HintWidth(h), HintRowHeight)
	DrawHint(cv, 0, 0, White, h)
	for x := 0; x < StickHintSize; x++ {
		if cv.at(x, 0) != Black {
			t.Fatalf("chevron ink on row 0 at x=%d; it should start on row 1", x)
		}
	}
	if cv.at(0, 3) == Black {
		t.Error("left chevron's point should be at (0,3) once dropped a row")
	}
}
