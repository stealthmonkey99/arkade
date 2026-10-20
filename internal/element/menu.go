// SPDX-License-Identifier: GPL-2.0-or-later

package element

import (
	"math"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

const (
	menuScrollFactor = 2 // screen-heights scrolled per second
	menuViewportH    = 64
	menuScrollMargin = 8  // the focused button is kept this far inside the viewport
	menuTrailingRows = 64 // blank rows below the list so a scrolled view has content
)

// Menu is a scrolling vertical list of buttons with focus handling. Update
// drives its scroll animation and hold-to-repeat; Render only lays out. A
// menu whose Update skipped ticks (its scene was away) drops a held
// direction, since the release may have gone to another scene.
type Menu struct {
	Base
	buttons []*Button
	spacing int
	focus   ui.FocusManager
	scrollY int
	hold    ui.HoldRepeat

	lastTick  uint64
	stickSeen bool // a stick change arrived since the last Update
}

// MenuOptions configures a Menu. Zero Spacing defaults to 1.
type MenuOptions struct {
	Options
	Buttons []*Button
	Spacing int
}

// NewMenu builds a menu and focuses the first button.
func NewMenu(opts MenuOptions) *Menu {
	m := &Menu{buttons: opts.Buttons, spacing: 1}
	if opts.Spacing != 0 {
		m.spacing = opts.Spacing
	}
	m.InitBase(opts.Options)
	parts := make([]display.Element, len(m.buttons))
	for i, b := range m.buttons {
		parts[i] = b
	}
	m.compose(parts...)
	m.focus.Reset(len(m.buttons), ui.FocusVertical)
	m.syncFocus()
	m.Impl = m.renderImpl
	return m
}

// syncFocus tells each button whether it is the focused one.
func (m *Menu) syncFocus() {
	for i, b := range m.buttons {
		b.setFocused(i == m.focus.Focused())
	}
}

func (m *Menu) moveFocus(dir ui.Direction) {
	m.focus.Move(dir)
	m.syncFocus()
}

func (m *Menu) renderImpl(stats clock.FrameTime) display.Sprite {
	maxWidth := 0
	var disp display.Sprite
	for _, button := range m.buttons {
		render := button.Render(stats)
		if len(render) > 0 && len(render[0]) > maxWidth {
			maxWidth = len(render[0])
		}
		disp = append(disp, render...)
		for i := 0; i < m.spacing; i++ {
			disp = append(disp, []display.Pixel{})
		}
	}
	for i := 0; i < menuTrailingRows; i++ {
		disp = append(disp, []display.Pixel{})
	}
	for y, row := range disp {
		for len(row) < maxWidth {
			row = append(row, display.Transparent)
		}
		disp[y] = row
	}
	if m.scrollY < 0 {
		cut := min(-m.scrollY, len(disp))
		disp = disp[cut:]
	}
	return disp
}

// focusSpan returns the focused button's top and bottom rows in list
// coordinates, from the buttons' last rendered heights.
func (m *Menu) focusSpan() (top, bottom int) {
	for _, button := range m.buttons {
		if button.Focused() {
			return top, top + button.Height() + 2*m.spacing
		}
		top += button.Height() + m.spacing
	}
	return top, top
}

// scroll steps the list toward keeping the focused button inside the
// scrollport. It needs the buttons' heights, so it waits for the first
// render.
func (m *Menu) scroll(stats clock.FrameTime) {
	if m.height == 0 {
		return
	}
	focusTop, focusBottom := m.focusSpan()
	viewportTop := -m.scrollY
	viewportBottom := viewportTop + min(menuViewportH, m.Height())
	scrollportTop := viewportTop + menuScrollMargin
	scrollportBottom := viewportBottom - menuScrollMargin
	scrollPerFrame := int(math.Round(menuScrollFactor * menuViewportH * stats.ElapsedMs / 1000))
	before := m.scrollY
	switch {
	case focusBottom > scrollportBottom:
		m.scrollY -= min(focusBottom-scrollportBottom, scrollPerFrame)
	case focusTop < scrollportTop && focusTop > 16:
		m.scrollY += min(scrollportTop-focusTop, scrollPerFrame)
	case focusTop < viewportTop:
		m.scrollY += min(viewportTop-focusTop, scrollPerFrame)
	}
	m.dirty = m.dirty || m.scrollY != before
}

// HandleInput steps focus or selects on a keypress, and takes the held level
// for Update's hold-to-repeat from joystick changes. A pad's push arrives as
// both, so the change only arms the hold and the keypress takes the step. It
// returns true when consumed.
func (m *Menu) HandleInput(ev input.Event) bool {
	switch ev.Kind {
	case input.KindJoystickChange:
		m.hold.Hold(StickDir(ev.Position))
		m.stickSeen = true
		return true
	case input.KindKeypress:
		if dir := Dir(ev.Name); dir != ui.DirNone {
			m.moveFocus(dir)
			return true
		}
		if IsSelect(ev.Name) {
			if b := m.CurrentSelection(); b != nil {
				b.Select(ev)
			}
			return true
		}
	}
	return false
}

// Update advances hold-to-repeat for a held stick direction and the scroll
// animation. Hosts call it every tick.
func (m *Menu) Update(stats clock.FrameTime) {
	if m.lastTick != 0 && stats.Ticks > m.lastTick+1 && !m.stickSeen {
		m.hold.Reset()
	}
	m.lastTick, m.stickSeen = stats.Ticks, false
	for n := m.hold.Update(ElapsedMicros(stats)); n > 0; n-- {
		m.moveFocus(m.hold.Held())
	}
	m.scroll(stats)
}

// CurrentSelection returns the focused button, nil for an empty menu.
func (m *Menu) CurrentSelection() *Button {
	if len(m.buttons) == 0 {
		return nil
	}
	return m.buttons[m.focus.Focused()]
}
