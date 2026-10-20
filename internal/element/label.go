// SPDX-License-Identifier: GPL-2.0-or-later

package element

import (
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
)

// Label is a retained line of text in the LED font, drawn by ui.FontLED.
type Label struct {
	Base
	text  string
	color display.Pixel
}

// LabelOptions configures a Label.
type LabelOptions struct {
	Options
	Text  string
	Color *display.Pixel
}

// NewLabel builds a text label (default color white).
func NewLabel(opts LabelOptions) *Label {
	l := &Label{text: opts.Text, color: display.White}
	if opts.Color != nil {
		l.color = *opts.Color
	}
	l.InitBase(opts.Options)
	l.Impl = l.renderImpl
	return l
}

func (l *Label) renderImpl(clock.FrameTime) display.Sprite {
	return display.TextSprite(l.text, l.color)
}

func (l *Label) Text() string { return l.text }

func (l *Label) SetText(v string) {
	l.dirty = l.dirty || l.text != v
	l.text = v
}

func (l *Label) Color() display.Pixel { return l.color }

func (l *Label) SetColor(v display.Pixel) {
	l.dirty = l.dirty || l.color != v
	l.color = v
}
