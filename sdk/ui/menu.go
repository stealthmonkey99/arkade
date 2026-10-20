// SPDX-License-Identifier: GPL-2.0-or-later

package ui

// menuScrollFactor is the scroll speed in viewport-heights per second.
const menuScrollFactor = 2

// menuScrollPadding is the margin in pixels kept between the focused item and
// the viewport edge.
const menuScrollPadding = 8

// Menu is a vertical list of selectable text items built on [Button] and
// [FocusManager]. It retains the selection and scroll position across calls.
// Construct with [NewMenu] in Init, drive with [Menu.Move] and [Menu.Update]
// from Update, read with [Menu.Selected] or [Menu.Activate], and render with
// [Menu.Draw]. Update and Draw allocate nothing.
//
// Update steps the scroll toward the position that keeps the focused item
// inside the viewport, so a long list scrolls smoothly and never past its
// content. Draw skips items wholly outside the viewport.
type Menu struct {
	// X, Y position the menu's top-left; W is every item's width.
	X, Y, W int
	// H is the viewport height in pixels. H <= 0 disables scrolling and
	// clipping entirely (every item is drawn, however long the list).
	H int
	// Items are the selectable labels, in order.
	Items []string
	// Spacing is the vertical gap between items, in pixels. 0 defaults to 1.
	Spacing int
	// ItemH is each item's box height, in pixels. 0 falls back to the
	// label's own height (FontLED.Height(Scale)) plus Button's padding.
	ItemH int
	// Scale multiplies each item's label glyph size. 0 means 1.
	Scale int
	// TextColor, Fill, FocusFill and TextDY forward to each item's Button; see
	// its field docs for their zero-value fallbacks.
	TextColor Color // label color
	Fill      Color // unfocused item fill
	FocusFill Color // focused item fill
	TextDY    int   // vertical label nudge, in pixels (positive is down)

	focus  FocusManager
	scroll int
}

// NewMenu builds a Menu over items with the first item focused. It allocates,
// so call it from Init. The Menu keeps items without copying it.
func NewMenu(items []string) *Menu {
	m := &Menu{Items: items}
	m.focus.Reset(len(items), FocusVertical)
	return m
}

// SetItems replaces the item list and resets scroll to the top. It does not
// allocate itself; the Menu keeps items without copying it.
func (m *Menu) SetItems(items []string) {
	m.Items = items
	m.focus.Reset(len(items), FocusVertical)
	m.scroll = 0
}

func (m *Menu) itemHeight() int {
	if m.ItemH > 0 {
		return m.ItemH
	}
	scale := m.Scale
	if scale <= 0 {
		scale = 1
	}
	return FontLED{}.Height(scale) + 2*buttonPadding
}

func (m *Menu) spacing() int {
	if m.Spacing > 0 {
		return m.Spacing
	}
	return 1
}

func (m *Menu) step() int { return m.itemHeight() + m.spacing() }

func (m *Menu) contentHeight() int {
	n := len(m.Items)
	if n == 0 {
		return 0
	}
	return n*m.itemHeight() + (n-1)*m.spacing()
}

// Move shifts the selection one item toward dir; call it once per press edge.
// A vertical list reacts only to [DirUp] and [DirDown], clamped at the first and last item with no wraparound.
func (m *Menu) Move(dir Direction) { m.focus.Move(dir) }

// Selected returns the currently highlighted item's index, or -1 if Items
// is empty.
func (m *Menu) Selected() int {
	if len(m.Items) == 0 {
		return -1
	}
	return m.focus.Focused()
}

// Activate returns the index of the current selection, or -1 if Items is
// empty. Menu has no callbacks, so the caller acts on the returned index.
func (m *Menu) Activate() int { return m.Selected() }

// Update steps the scroll toward keeping the focused item within the
// viewport, at twice the viewport height per second of dtMicros. It does
// nothing when H <= 0 or Items is empty. Call once per frame.
func (m *Menu) Update(dtMicros uint64) {
	if m.H <= 0 || len(m.Items) == 0 {
		return
	}
	itemH := m.itemHeight()
	step := m.step()
	cur := m.focus.Focused()
	focusTop := cur * step
	focusBottom := focusTop + itemH

	maxScroll := m.contentHeight() - m.H
	if maxScroll < 0 {
		maxScroll = 0
	}

	bottomLimit := m.H - menuScrollPadding
	if bottomLimit < itemH {
		bottomLimit = itemH // guard viewports too short for the padding margin
	}

	target := m.scroll
	switch {
	case focusBottom-m.scroll > bottomLimit:
		target = focusBottom - bottomLimit
	case focusTop-m.scroll < menuScrollPadding:
		target = focusTop - menuScrollPadding
	}
	if target < 0 {
		target = 0
	} else if target > maxScroll {
		target = maxScroll
	}

	speed := float64(menuScrollFactor * m.H) // px/sec
	delta := int(speed*float64(dtMicros)/1e6 + 0.5)
	switch {
	case m.scroll < target:
		m.scroll += delta
		if m.scroll > target {
			m.scroll = target
		}
	case m.scroll > target:
		m.scroll -= delta
		if m.scroll < target {
			m.scroll = target
		}
	}
}

// Draw composites every visible item onto cv as a [Button], skipping any item
// entirely outside the viewport (when H > 0).
func (m *Menu) Draw(cv Canvas) {
	if len(m.Items) == 0 {
		return
	}
	itemH := m.itemHeight()
	step := m.step()
	cur := m.focus.Focused()
	for i, text := range m.Items {
		top := i*step - m.scroll
		if m.H > 0 && (top+itemH <= 0 || top >= m.H) {
			continue
		}
		Button{
			X: m.X, Y: m.Y + top, W: m.W, H: itemH,
			Text:      text,
			Focused:   i == cur,
			TextColor: m.TextColor,
			Fill:      m.Fill,
			FocusFill: m.FocusFill,
			Scale:     m.Scale,
			TextDY:    m.TextDY,
		}.Draw(cv)
	}
}
