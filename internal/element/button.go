// SPDX-License-Identifier: GPL-2.0-or-later

package element

import (
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// The button's box around its label: ui.Button's fixed 2px left inset, one
// pixel of right margin, and a row above and below the font's line box, with
// the text dropped a row to sit optically centred.
const (
	buttonPadRight = 1
	buttonPadY     = 1
	buttonTextDY   = 1
)

// Button is a retained, focusable text control drawn by ui.Button. Menu owns
// its focus; Select runs its action.
type Button struct {
	Base
	text    string
	focused bool
	action  func(b *Button, ev input.Event)
	// Meta carries arbitrary caller data (the shell stores the app
	// manifest here).
	Meta any
}

// ButtonOptions configures a Button.
type ButtonOptions struct {
	Options
	Label  string
	Action func(b *Button, ev input.Event)
	Meta   any
}

// NewButton builds a button.
func NewButton(opts ButtonOptions) *Button {
	b := &Button{text: opts.Label, action: opts.Action, Meta: opts.Meta}
	b.InitBase(opts.Options)
	b.Impl = b.renderImpl
	return b
}

func (b *Button) renderImpl(clock.FrameTime) display.Sprite {
	font := ui.FontLED{}
	w := 2 + font.TextWidth(b.text, 1) + buttonPadRight
	h := font.Height(1) + 2*buttonPadY
	return display.Stamp(w, h, ui.Button{W: w, H: h, Text: b.text, Focused: b.focused, TextDY: buttonTextDY})
}

// Text returns the button's label text.
func (b *Button) Text() string { return b.text }

// Focused reports whether the button has focus.
func (b *Button) Focused() bool { return b.focused }

func (b *Button) setFocused(on bool) {
	b.dirty = b.dirty || b.focused != on
	b.focused = on
}

// Select runs the button's action.
func (b *Button) Select(ev input.Event) {
	if b.action != nil {
		b.action(b, ev)
	}
}
