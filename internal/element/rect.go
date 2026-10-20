// SPDX-License-Identifier: GPL-2.0-or-later

package element

import (
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// Rect is a retained filled rectangle with an optional (beveled) border,
// drawn by ui.Border.
type Rect struct {
	Base
	fill   display.Pixel
	border Border
}

// RectOptions configures a Rect. Zero Width/Height default to 10.
type RectOptions struct {
	Options
	W, H   int
	Fill   *display.Pixel
	Border *Border
}

// NewRect builds a rectangle (defaults: 10×10, transparent fill, red border).
func NewRect(opts RectOptions) *Rect {
	r := &Rect{fill: display.Transparent, border: BorderOf(display.Red)}
	if opts.Fill != nil {
		r.fill = *opts.Fill
	}
	if opts.Border != nil {
		r.border = *opts.Border
	}
	r.InitBase(opts.Options)
	r.width = 10
	r.height = 10
	if opts.W != 0 {
		r.width = opts.W
	}
	if opts.H != 0 {
		r.height = opts.H
	}
	r.Impl = r.renderImpl
	return r
}

func (r *Rect) renderImpl(clock.FrameTime) display.Sprite {
	return display.Stamp(r.width, r.height, ui.Border{
		W: r.width, H: r.height,
		Fill:  display.ToUI(r.fill),
		Kind:  r.border.Kind,
		Color: display.ToUI(r.border.Color),
	})
}

func (r *Rect) Fill() display.Pixel { return r.fill }

func (r *Rect) SetFill(v display.Pixel) {
	r.dirty = r.dirty || r.fill != v
	r.fill = v
}

func (r *Rect) Border() Border { return r.border }

func (r *Rect) SetBorder(v Border) {
	r.dirty = r.dirty || r.border != v
	r.border = v
}
