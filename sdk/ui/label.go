// SPDX-License-Identifier: GPL-2.0-or-later

package ui

// LabelFont is the glyph source a [Label] draws with. [FontLED] and [Font3x5]
// both satisfy it. Implementations must not allocate.
type LabelFont interface {
	// DrawText draws s with its top-left at (x, y), each glyph pixel scaled
	// to a scale x scale block. Label always passes scale >= 1.
	DrawText(cv Canvas, x, y int, s string, scale int, c Color)
	// MeasureText returns the width in pixels DrawText covers for the same s
	// and scale. Label aligns by it, so the two must agree.
	MeasureText(s string, scale int) int
}

// Align selects how [Label] positions its text horizontally relative to X.
type Align int

const (
	// AlignLeft draws text starting at X (the default, zero value).
	AlignLeft Align = iota
	// AlignCenter centers the text horizontally on X.
	AlignCenter
	// AlignRight ends the text at X.
	AlignRight
)

// Label draws a line of text, in the variable-width LED font unless Font
// says otherwise. A zero Label draws nothing (empty Text or transparent
// Color). Draw it from the render phase; it allocates nothing.
type Label struct {
	X, Y  int    // anchor, in pixels: top of the text, and the horizontal point Align refers to
	Text  string // single line; there is no wrapping
	Color Color  // text color; Transparent draws nothing
	// Scale multiplies every glyph pixel into a scale×scale block. 0 means 1
	// (so a zero-value Label is still usable at natural size).
	Scale int
	// Align positions Text horizontally relative to X. The zero value is
	// AlignLeft.
	Align Align
	// Font overrides the glyph source; nil uses FontLED{}.
	Font LabelFont
}

// Draw composites Text onto cv, anchored at (X, Y) per Align.
func (l Label) Draw(cv Canvas) {
	if l.Text == "" || l.Color.A == 0 {
		return
	}
	scale := l.Scale
	if scale <= 0 {
		scale = 1
	}
	font := l.Font
	if font == nil {
		font = FontLED{}
	}
	x := l.X
	switch l.Align {
	case AlignCenter:
		x -= font.MeasureText(l.Text, scale) / 2
	case AlignRight:
		x -= font.MeasureText(l.Text, scale)
	}
	font.DrawText(cv, x, l.Y, l.Text, scale, l.Color)
}
