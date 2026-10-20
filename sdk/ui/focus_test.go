// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

func TestFocusManagerVerticalClampsNoWrap(t *testing.T) {
	m := NewFocusManager()
	m.Reset(4, FocusVertical)
	if got := m.Focused(); got != 0 {
		t.Fatalf("initial Focused() = %d, want 0", got)
	}

	m.Move(DirUp) // already at 0: clamp, no wrap to the end
	if got := m.Focused(); got != 0 {
		t.Fatalf("Move(DirUp) at top = %d, want 0 (clamped, not wrapped)", got)
	}

	for i, want := range []int{1, 2, 3} {
		m.Move(DirDown)
		if got := m.Focused(); got != want {
			t.Fatalf("Move(DirDown) #%d = %d, want %d", i+1, got, want)
		}
	}

	m.Move(DirDown) // already at last: clamp, no wrap to the start
	if got := m.Focused(); got != 3 {
		t.Fatalf("Move(DirDown) at bottom = %d, want 3 (clamped, not wrapped)", got)
	}

	m.Move(DirLeft)
	m.Move(DirRight)
	if got := m.Focused(); got != 3 {
		t.Fatalf("off-axis Move on vertical group = %d, want unchanged 3", got)
	}
}

func TestFocusManagerHorizontalClampsNoWrap(t *testing.T) {
	m := NewFocusManager()
	m.Reset(3, FocusHorizontal)

	m.Move(DirLeft) // clamp at start
	if got := m.Focused(); got != 0 {
		t.Fatalf("Move(DirLeft) at start = %d, want 0", got)
	}
	m.Move(DirRight)
	m.Move(DirRight)
	if got := m.Focused(); got != 2 {
		t.Fatalf("Focused() after two Move(DirRight) = %d, want 2", got)
	}
	m.Move(DirRight) // clamp at end
	if got := m.Focused(); got != 2 {
		t.Fatalf("Move(DirRight) at end = %d, want 2", got)
	}

	m.Move(DirUp)
	m.Move(DirDown)
	if got := m.Focused(); got != 2 {
		t.Fatalf("off-axis Move on horizontal group = %d, want unchanged 2", got)
	}
}

// TestFocusManagerGridTraversal walks a 3-column, 8-item grid through every
// Direction. The last row is short, and moving Down onto it clamps to the
// last item.
//
//	row0: 0 1 2
//	row1: 3 4 5
//	row2: 6 7
func TestFocusManagerGridTraversal(t *testing.T) {
	m := NewFocusManager()
	m.ResetGrid(8, 3)
	if got := m.Focused(); got != 0 {
		t.Fatalf("initial Focused() = %d, want 0", got)
	}

	steps := []struct {
		dir  Direction
		want int
	}{
		{DirRight, 1}, // row0: col0 -> col1
		{DirRight, 2}, // row0: col1 -> col2
		{DirDown, 5},  // row0 col2 -> row1 col2
		{DirDown, 7},  // row1 col2 -> row2 col2 doesn't exist (only 6,7): clamp to 7
		{DirLeft, 6},  // row2 col1 -> col0
		{DirUp, 3},    // row2 col0 -> row1 col0
	}
	for i, s := range steps {
		m.Move(s.dir)
		if got := m.Focused(); got != s.want {
			t.Fatalf("step %d Move(%v) = %d, want %d", i, s.dir, got, s.want)
		}
	}
}

func TestFocusManagerEmptyIsNoop(t *testing.T) {
	m := NewFocusManager()
	m.Reset(0, FocusVertical)
	m.Move(DirDown)
	m.Move(DirUp)
	if got := m.Focused(); got != 0 {
		t.Errorf("Focused() on empty manager = %d, want 0", got)
	}
}

func TestFocusManagerSetFocusedClamps(t *testing.T) {
	m := NewFocusManager()
	m.Reset(5, FocusVertical)

	m.SetFocused(10)
	if got := m.Focused(); got != 4 {
		t.Errorf("SetFocused(10) with count 5 = %d, want clamped 4", got)
	}
	m.SetFocused(-3)
	if got := m.Focused(); got != 0 {
		t.Errorf("SetFocused(-3) = %d, want clamped 0", got)
	}
}

func TestFocusManagerResetPreservesValidCurrent(t *testing.T) {
	m := NewFocusManager()
	m.Reset(5, FocusVertical)
	m.SetFocused(3)

	m.Reset(5, FocusVertical) // same count: still-valid current survives
	if got := m.Focused(); got != 3 {
		t.Errorf("Reset with unchanged count = %d, want preserved 3", got)
	}

	m.Reset(2, FocusVertical) // shrink past current: clamp
	if got := m.Focused(); got != 1 {
		t.Errorf("Reset shrinking past current = %d, want clamped 1", got)
	}
}

func TestFocusManagerCount(t *testing.T) {
	m := NewFocusManager()
	m.Reset(7, FocusVertical)
	if got := m.Count(); got != 7 {
		t.Errorf("Count() = %d, want 7", got)
	}
}
