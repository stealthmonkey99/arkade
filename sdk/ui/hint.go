// SPDX-License-Identifier: GPL-2.0-or-later

package ui

const (
	// ButtonDotSize is the disc's diameter in pixels, and the width of every
	// hint cell so a row does not shift when a slot changes control.
	ButtonDotSize = 7

	// StickHintSize is the width in pixels of the joystick hint, a left/right
	// chevron pair.
	StickHintSize = 7

	// HintRowHeight is the height in pixels of a hint row, matching the LED
	// font's line box.
	HintRowHeight = 8

	hintGap     = 2
	hintPairGap = 5

	chevronW = 3
	chevronH = 5
)

// The cabinet's button colours, as the panel draws them. The trailing names
// are the cartlib key names (KeyJoyA and friends), to help an author find the
// right colour; the buttons themselves carry no labels.
var (
	ButtonGreen  = Color{R: 60, G: 220, B: 90, A: 255}   // joyA
	ButtonRed    = Color{R: 235, G: 80, B: 70, A: 255}   // joyB
	ButtonYellow = Color{R: 240, G: 200, B: 40, A: 255}  // joyC
	ButtonBlue   = Color{R: 70, G: 140, B: 255, A: 255}  // joyD
	ButtonAux    = Color{R: 255, G: 255, B: 255, A: 255} // joyAux1 and joyAux2, both white
)

// HintControl selects which control a Hint draws. The zero value is the
// ordinary coloured button, which is what almost every hint is.
type HintControl uint8

const (
	// HintButton draws a filled disc in Hint.Color, one cabinet button.
	HintButton HintControl = iota
	// HintStick draws the joystick's left/right chevron pair.
	HintStick
	// HintAux1 draws the aux pair with the upper button lit.
	HintAux1
	// HintAux2 draws the aux pair with the lower button lit.
	HintAux2
)

// Hint is one entry in a key-hint row: what a control does and which control
// does it. Cabinet buttons carry no letters, so a hint draws the control
// itself, a disc in its own colour, beside the label. Color is one of the
// Button* values; for the aux variants it is optional and defaults to
// ButtonAux.
type Hint struct {
	Text    string      // label, drawn in [FontLED] beside the control
	Color   Color       // the control's color; one of the Button* values
	Control HintControl // which control to draw; the zero value is [HintButton]
}

// buttonDotRows is the disc's row widths. Circle{R: 3} rasterizes to
// {1,5,5,7,5,5,1}, so the disc is a separate table.
var buttonDotRows = [ButtonDotSize]int{3, 5, 7, 7, 7, 5, 3}

// ButtonDot is one cabinet button: a filled disc in its own colour,
// ButtonDotSize square, top-left at (X, Y).
type ButtonDot struct {
	X, Y  int   // top-left, in pixels
	Color Color // disc color; Transparent draws nothing
}

// Draw composites the disc onto cv.
func (d ButtonDot) Draw(cv Canvas) {
	if d.Color.A == 0 {
		return
	}
	for y, w := range buttonDotRows {
		x0 := (ButtonDotSize - w) / 2
		HLine(cv, d.X+x0, d.X+x0+w-1, d.Y+y, d.Color)
	}
}

// Chevron art: 3x5 arrows, 3 bits per row with bit 2 leftmost.
var (
	chevronLeftArt  = [chevronH]byte{0b001, 0b010, 0b100, 0b010, 0b001}
	chevronRightArt = [chevronH]byte{0b100, 0b010, 0b001, 0b010, 0b100}
)

// Chevron is a 3×5 arrow pointing left or, with Right set, right, its
// top-left at (X, Y): the glyph StickHint pairs, on its own for a "more this
// way" cue at a panel edge.
type Chevron struct {
	X, Y  int   // top-left, in pixels
	Color Color // Transparent draws nothing
	Right bool  // points right instead of left
}

// Draw composites the arrow onto cv.
func (c Chevron) Draw(cv Canvas) {
	if c.Color.A == 0 {
		return
	}
	art := &chevronLeftArt
	if c.Right {
		art = &chevronRightArt
	}
	for row := 0; row < chevronH; row++ {
		bits := art[row]
		for col := 0; col < chevronW; col++ {
			if bits&(1<<uint(chevronW-1-col)) != 0 {
				cv.Blend(c.X+col, c.Y+row, c.Color)
			}
		}
	}
}

// StickHint is the joystick: a left/right chevron pair, StickHintSize wide
// and 5 tall, top-left at (X, Y). It is the width of a button disc so hint
// rows line up whichever control a slot holds.
type StickHint struct {
	X, Y  int   // top-left, in pixels
	Color Color // Transparent draws nothing
}

// Draw composites the chevron pair onto cv.
func (s StickHint) Draw(cv Canvas) {
	if s.Color.A == 0 {
		return
	}
	Chevron{X: s.X, Y: s.Y, Color: s.Color}.Draw(cv)
	Chevron{X: s.X + StickHintSize - chevronW, Y: s.Y, Color: s.Color, Right: true}.Draw(cv)
}

// AuxDot is the pair of aux buttons with one referenced: two small ovals
// stacked vertically, the referenced one filled and the other a dim outline.
// Both aux buttons are white, so colour cannot tell them apart; the stacked
// pair mirrors their physical layout. The cell is ButtonDotSize square, so an
// aux hint drops into a row without disturbing its layout.
type AuxDot struct {
	X, Y int // top-left, in pixels
	// Color is the lit disc's colour; zero defaults to ButtonAux (white).
	Color Color
	// Lower selects which of the pair is lit: false the upper button
	// (aux 1), true the lower (aux 2).
	Lower bool
}

var auxOvalRows = [3]int{3, 5, 3}

const (
	auxOvalW = 5
	auxOvalH = 3
	// auxLowerY is the top row of the lower oval: rows 0-2, a blank row,
	// then rows 4-6.
	auxLowerY = ButtonDotSize - auxOvalH
)

// dimHint is c at a third of the brightness; integer division keeps it
// bit-identical on both hosts.
func dimHint(c Color) Color {
	return Color{R: c.R / 3, G: c.G / 3, B: c.B / 3, A: c.A}
}

// Draw composites the stacked aux pair onto cv.
func (a AuxDot) Draw(cv Canvas) {
	c := a.Color
	if c.A == 0 {
		c = ButtonAux
	}
	litY, dimY := a.Y, a.Y+auxLowerY
	if a.Lower {
		litY, dimY = dimY, litY
	}
	a.fillOval(cv, litY, c)
	a.strokeOval(cv, dimY, dimHint(c))
}

func (a AuxDot) fillOval(cv Canvas, y int, c Color) {
	x0 := a.X + (ButtonDotSize-auxOvalW)/2
	for row, w := range auxOvalRows {
		sx := x0 + (auxOvalW-w)/2
		HLine(cv, sx, sx+w-1, y+row, c)
	}
}

// strokeOval paints one aux button as an outline: every oval pixel not
// surrounded by other oval pixels.
func (a AuxDot) strokeOval(cv Canvas, y int, c Color) {
	x0 := a.X + (ButtonDotSize-auxOvalW)/2
	for row, w := range auxOvalRows {
		sx := x0 + (auxOvalW-w)/2
		for i := 0; i < w; i++ {
			if row > 0 && row < auxOvalH-1 && i > 0 && i < w-1 {
				continue
			}
			cv.Blend(sx+i, y+row, c)
		}
	}
}

// HintWidth returns one hint's width in pixels: its control cell, the gap and
// its label, with no trailing separator.
func HintWidth(h Hint) int {
	return ButtonDotSize + hintGap + FontLED{}.TextWidth(h.Text, 1)
}

// HintRowWidth returns the width in pixels a row of hints occupies, including
// the separators between hints but none after the last. It takes a slice rather
// than variadic arguments so a per-frame call allocates nothing.
func HintRowWidth(hints []Hint) int {
	w := -hintPairGap
	for i := range hints {
		w += hintPairGap + HintWidth(hints[i])
	}
	if w < 0 {
		return 0
	}
	return w
}

// DrawHint draws one hint with its control cell's top-left at (x, y) and
// returns the x where the next hint's cell starts, past the inter-hint gap.
// Use it for a single hint or a row composed by hand; call it from the render
// phase. It allocates nothing.
func DrawHint(cv Canvas, x, y int, labelColor Color, h Hint) int {
	switch h.Control {
	case HintStick:
		// 5 tall against the disc's 7: drop a row to sit on the label's ink line.
		StickHint{X: x, Y: y + 1, Color: h.Color}.Draw(cv)
	case HintAux1:
		AuxDot{X: x, Y: y, Color: h.Color}.Draw(cv)
	case HintAux2:
		AuxDot{X: x, Y: y, Color: h.Color, Lower: true}.Draw(cv)
	default:
		ButtonDot{X: x, Y: y, Color: h.Color}.Draw(cv)
	}
	x += ButtonDotSize + hintGap
	Label{X: x, Y: y, Text: h.Text, Color: labelColor}.Draw(cv)
	return x + FontLED{}.TextWidth(h.Text, 1) + hintPairGap
}

// HintRow composes hints into one strip, control then label, with its
// top-left at (X, Y). Labels take LabelColor; each control brings its own
// colour. Hints is caller-owned: build it once in Init or when the controls
// change, not in Draw.
type HintRow struct {
	X, Y       int    // top-left, in pixels
	LabelColor Color  // color of every hint's label
	Hints      []Hint // drawn left to right
}

// Width returns the strip's width in pixels, what [HintRow.Draw] covers.
func (r HintRow) Width() int { return HintRowWidth(r.Hints) }

// Draw composites the row onto cv.
func (r HintRow) Draw(cv Canvas) {
	x := r.X
	for i := range r.Hints {
		x = DrawHint(cv, x, r.Y, r.LabelColor, r.Hints[i])
	}
}
