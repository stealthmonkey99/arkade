// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

func TestMenuMoveChangesSelectionClamped(t *testing.T) {
	m := NewMenu([]string{"A", "B", "C"})
	if got := m.Selected(); got != 0 {
		t.Fatalf("initial Selected() = %d, want 0", got)
	}

	m.Move(DirUp) // clamp at top, no wrap
	if got := m.Selected(); got != 0 {
		t.Fatalf("Move(DirUp) at top = %d, want 0", got)
	}

	m.Move(DirDown)
	m.Move(DirDown)
	if got := m.Selected(); got != 2 {
		t.Fatalf("Selected() after two Move(DirDown) = %d, want 2", got)
	}

	m.Move(DirDown) // clamp at bottom, no wrap
	if got := m.Selected(); got != 2 {
		t.Fatalf("Move(DirDown) at bottom = %d, want 2", got)
	}
}

func TestMenuActivateReturnsSelectedIndex(t *testing.T) {
	m := NewMenu([]string{"New Game", "Options", "Quit"})
	m.Move(DirDown)
	if got := m.Activate(); got != 1 {
		t.Fatalf("Activate() = %d, want 1", got)
	}
}

func TestMenuEmptyIsSafe(t *testing.T) {
	m := NewMenu(nil)
	if got := m.Selected(); got != -1 {
		t.Errorf("Selected() on empty Menu = %d, want -1", got)
	}
	if got := m.Activate(); got != -1 {
		t.Errorf("Activate() on empty Menu = %d, want -1", got)
	}
	m.Move(DirDown) // must not panic
	m.Update(1000)  // must not panic
	m.Draw(newGridCanvas(4, 4))
}

func TestMenuDrawsFocusedItemDifferently(t *testing.T) {
	items := []string{"Alpha", "Beta"}

	first := NewMenu(items)
	first.W = 30
	firstGrid := drawnGrid(40, 30, func(cv Canvas) { first.Draw(cv) })

	second := NewMenu(items)
	second.W = 30
	second.Move(DirDown)
	secondGrid := drawnGrid(40, 30, func(cv Canvas) { second.Draw(cv) })

	if gridsEqual(firstGrid, secondGrid) {
		t.Fatal("moving the selection should change which item renders focused")
	}
}

// TestMenuSetItemsClampsSelection: SetItems keeps a still-valid selection and
// clamps one now out of range.
func TestMenuSetItemsClampsSelection(t *testing.T) {
	m := NewMenu([]string{"A", "B", "C"})
	m.Move(DirDown)
	m.Move(DirDown)
	if got := m.Selected(); got != 2 {
		t.Fatalf("setup: Selected() = %d, want 2", got)
	}

	m.SetItems([]string{"X", "Y"})
	if got := m.Selected(); got != 1 {
		t.Errorf("Selected() after SetItems shrinking past current = %d, want clamped 1", got)
	}
}

func TestMenuSetItemsResetsScroll(t *testing.T) {
	items := buildMenuItems(20)
	m := NewMenu(items)
	m.W, m.H = 40, 40
	for i := 0; i < len(items)-1; i++ {
		m.Move(DirDown)
	}
	m.Update(5_000_000)
	if m.scroll == 0 {
		t.Fatal("setup: expected scroll to have advanced")
	}

	m.SetItems(items)
	if m.scroll != 0 {
		t.Errorf("scroll after SetItems = %d, want reset to 0", m.scroll)
	}
}

// buildMenuItems returns n placeholder labels.
func buildMenuItems(n int) []string {
	items := make([]string, n)
	names := []string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon"}
	for i := range items {
		items[i] = names[i%len(names)]
	}
	return items
}

// TestMenuLongListScrolls moves the selection to the last item of a 20-item
// menu in a short viewport and drives Update far enough to land exactly at
// contentHeight-H.
func TestMenuLongListScrolls(t *testing.T) {
	items := buildMenuItems(20)
	m := NewMenu(items)
	m.W, m.H = 40, 40

	for i := 0; i < len(items)-1; i++ {
		m.Move(DirDown)
	}
	if got := m.Selected(); got != len(items)-1 {
		t.Fatalf("Selected() after moving to end = %d, want %d", got, len(items)-1)
	}
	if m.scroll != 0 {
		t.Fatalf("scroll before Update = %d, want 0", m.scroll)
	}

	m.Update(5_000_000) // 5s: far more than enough to reach the target in one step

	wantScroll := m.contentHeight() - m.H
	if m.scroll != wantScroll {
		t.Fatalf("scroll after Update = %d, want %d (contentHeight-H, the max-scroll clamp)", m.scroll, wantScroll)
	}
	if m.scroll <= 0 {
		t.Fatal("a 20-item list in a 40px viewport should have scrolled")
	}

	// The scrolled-off first item must be skipped by Draw.
	cv := newGridCanvas(40, 40)
	m.Draw(cv)
	if got := cv.at(1, 1); got == buttonFill || got == buttonFocusFill {
		t.Errorf("pixel under the scrolled-off first item = %v, want untouched background", got)
	}
}

// TestMenuScrollAnimatesGradually: Update chases the target in steps, and
// repeated small steps converge on the same target as one large step.
func TestMenuScrollAnimatesGradually(t *testing.T) {
	items := buildMenuItems(20)
	m := NewMenu(items)
	m.W, m.H = 40, 40
	for i := 0; i < len(items)-1; i++ {
		m.Move(DirDown)
	}

	m.Update(16_000) // one ~60fps frame's worth of dtMicros
	afterOneFrame := m.scroll
	if afterOneFrame <= 0 {
		t.Fatal("expected some scroll progress after one frame")
	}
	target := m.contentHeight() - m.H
	if afterOneFrame >= target {
		t.Fatalf("scroll after one small frame = %d, want strictly less than target %d (should animate, not snap)", afterOneFrame, target)
	}

	for i := 0; i < 1000 && m.scroll != target; i++ {
		m.Update(16_000)
	}
	if m.scroll != target {
		t.Fatalf("scroll after many frames = %d, want converged to target %d", m.scroll, target)
	}
}

func TestMenuUpdateNoopWithoutViewport(t *testing.T) {
	items := buildMenuItems(20)
	m := NewMenu(items)
	// m.H left at zero: scrolling disabled.
	for i := 0; i < len(items)-1; i++ {
		m.Move(DirDown)
	}
	m.Update(5_000_000)
	if m.scroll != 0 {
		t.Errorf("scroll with H<=0 = %d, want 0 (scrolling disabled)", m.scroll)
	}
}

func TestMenuSubstrateParity(t *testing.T) {
	build := func() *Menu {
		m := NewMenu(buildMenuItems(6))
		m.W, m.H = 30, 24
		m.Move(DirDown)
		m.Update(200_000)
		return m
	}
	bc := newByteCanvas(30, 24)
	gc := newGridCanvas(30, 24)
	build().Draw(bc)
	build().Draw(gc)
	for y := 0; y < 24; y++ {
		for x := 0; x < 30; x++ {
			if bc.at(x, y) != gc.at(x, y) {
				t.Fatalf("pixel (%d,%d): byteCanvas=%v gridCanvas=%v", x, y, bc.at(x, y), gc.at(x, y))
			}
		}
	}
}
