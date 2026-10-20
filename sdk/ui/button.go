// SPDX-License-Identifier: GPL-2.0-or-later

package ui

// buttonFill and buttonFocusFill are Button's default base colors.
var (
	buttonFill      = Color{180, 180, 180, 255}
	buttonFocusFill = Color{150, 188, 227, 255}
)

// Button is a labeled, optionally focused box with a raised bevel (see
// EdgeBorder). A zero Button is inert: W or H <= 0 draws nothing. W and H come
// from the caller; measure text with FontLED.TextWidth and Height to size to
// content. The label is centered vertically with a fixed left inset. Button
// has no pressed or animated state.
type Button struct {
	X, Y, W, H int    // bounds, in pixels
	Text       string // label, drawn in [FontLED]; empty draws the box alone
	// Focused swaps the base fill from Fill to FocusFill.
	Focused bool
	// TextColor is the label color. Zero (Transparent) falls back to Black.
	TextColor Color
	// Fill is the base color when unfocused. Zero falls back to light gray.
	Fill Color
	// FocusFill is the base color when Focused. Zero falls back to light
	// blue.
	FocusFill Color
	// Scale multiplies the label's glyph size. 0 means 1.
	Scale int
	// TextDY nudges the label vertically from its bitmap-centered position,
	// in pixels (positive is down). The LED font's capitals ink only rows
	// 1-5 of their 8-row bitmap, so bitmap centering leaves text optically
	// high; a caller can drop it a pixel here.
	TextDY int
}

// buttonPadding is the inset between the button edge and its label, clear of
// the bevel.
const buttonPadding = 2

// Draw composites the button's beveled base and label onto cv.
func (b Button) Draw(cv Canvas) {
	if b.W <= 0 || b.H <= 0 {
		return
	}
	fill := b.Fill
	if fill.A == 0 {
		fill = buttonFill
	}
	if b.Focused {
		fill = b.FocusFill
		if fill.A == 0 {
			fill = buttonFocusFill
		}
	}
	EdgeBorder(b.X, b.Y, b.W, b.H, fill).Draw(cv)

	if b.Text == "" {
		return
	}
	textColor := b.TextColor
	if textColor.A == 0 {
		textColor = Black
	}
	scale := b.Scale
	if scale <= 0 {
		scale = 1
	}
	textH := FontLED{}.Height(scale)
	y := b.Y + (b.H-textH)/2 + b.TextDY
	Label{X: b.X + buttonPadding, Y: y, Text: b.Text, Color: textColor, Scale: scale}.Draw(cv)
}
