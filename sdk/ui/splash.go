// SPDX-License-Identifier: GPL-2.0-or-later

package ui

// splashGap is the vertical gap, in pixels, between Splash's Title and
// Subtitle lines.
const splashGap = 2

// Splash is the default loading screen: a solid background with a centered
// title and an optional subtitle beneath it. It is a plain value with no
// retained state and no Update.
type Splash struct {
	W, H     int    // size of the filled area from (0, 0), in pixels; nothing is drawn when not positive
	Title    string // first line, centered horizontally
	Subtitle string // optional second line, e.g. "LOADING"
	// Background is the full-frame fill color. The zero value defaults to
	// opaque Black, so a Splash always covers what was on screen before it.
	Background Color
	// Color is the text color. The zero value defaults to White.
	Color Color
}

// Draw fills the W x H rectangle at the canvas origin with Background, then
// centers Title (and Subtitle beneath it, when set) in it using [FontLED] at
// scale 1.
func (s Splash) Draw(cv Canvas) {
	if s.W <= 0 || s.H <= 0 {
		return
	}
	bg := s.Background
	if bg.A == 0 {
		bg = Black
	}
	FillRect(cv, 0, 0, s.W, s.H, bg)

	textColor := s.Color
	if textColor.A == 0 {
		textColor = White
	}

	const scale = 1
	font := FontLED{}
	lineH := font.Height(scale)

	blockH := lineH
	if s.Subtitle != "" {
		blockH += splashGap + lineH
	}
	top := (s.H - blockH) / 2

	if s.Title != "" {
		x := (s.W - font.TextWidth(s.Title, scale)) / 2
		font.DrawText(cv, x, top, s.Title, scale, textColor)
	}
	if s.Subtitle != "" {
		x := (s.W - font.TextWidth(s.Subtitle, scale)) / 2
		font.DrawText(cv, x, top+lineH+splashGap, s.Subtitle, scale, textColor)
	}
}
