// SPDX-License-Identifier: GPL-2.0-or-later

package ui

// FocusOrientation is the axis a single-row-or-column focus group moves
// along, for [FocusManager.Reset].
type FocusOrientation int

// The FocusOrientation values.
const (
	// FocusVertical moves on [DirUp] and [DirDown].
	FocusVertical FocusOrientation = iota
	// FocusHorizontal moves on [DirLeft] and [DirRight].
	FocusHorizontal
)

// FocusManager tracks which of a set of focusable targets is focused and
// moves that focus on [FocusManager.Move]. It owns no widgets, only a count and
// a current index: register the target count with [FocusManager.Reset] or
// [FocusManager.ResetGrid], read [FocusManager.Focused] for the current index,
// and let the caller (such as [Menu]) decide what focused looks like. Only
// [NewFocusManager] allocates, so call it from Init, or use a zero value and
// call Reset.
//
// Targets form a row-major grid. Reset(count, FocusVertical) is one column
// and Reset(count, FocusHorizontal) is one row. Move clamps at the edges and
// never wraps; in a grid, a short last row clamps to the last valid target.
type FocusManager struct {
	count   int
	cols    int
	current int
}

// NewFocusManager returns an empty, single-column FocusManager (Move is a
// no-op until Reset or [FocusManager.ResetGrid] registers at least one target).
func NewFocusManager() *FocusManager {
	return &FocusManager{cols: 1}
}

// Reset registers count focusable targets laid out along a single axis. The
// current index is clamped into the new range (0 if count is 0), so a
// still-valid selection survives a resize; call [FocusManager.SetFocused](0) after Reset to
// restart at the first target.
func (m *FocusManager) Reset(count int, orientation FocusOrientation) {
	cols := 1
	if orientation == FocusHorizontal && count > 0 {
		cols = count
	}
	m.ResetGrid(count, cols)
}

// ResetGrid registers count focusable targets laid out row-major with cols
// columns per row (the last row may be short if count isn't a multiple of
// cols). cols < 1 is treated as 1.
func (m *FocusManager) ResetGrid(count, cols int) {
	if cols < 1 {
		cols = 1
	}
	m.count = count
	m.cols = cols
	switch {
	case m.count <= 0:
		m.current = 0
	case m.current >= m.count:
		m.current = m.count - 1
	case m.current < 0:
		m.current = 0
	}
}

func (m *FocusManager) rows() int {
	if m.cols < 1 || m.count <= 0 {
		return 0
	}
	return (m.count + m.cols - 1) / m.cols
}

// Move shifts focus one step toward dir (once per press edge), clamped at the edges of the grid;
// it never wraps. Off-axis directions on a single-axis group do nothing.
func (m *FocusManager) Move(dir Direction) {
	if m.count <= 0 {
		return
	}
	row := m.current / m.cols
	col := m.current % m.cols
	switch dir {
	case DirUp:
		if row > 0 {
			m.moveTo(row-1, col)
		}
	case DirDown:
		if row+1 < m.rows() {
			m.moveTo(row+1, col)
		}
	case DirLeft:
		if col > 0 {
			m.current--
		}
	case DirRight:
		if col < m.cols-1 && m.current+1 < m.count {
			m.current++
		}
	}
}

// moveTo focuses (row, col), clamping to the last valid target if that row
// is short (only reachable via ResetGrid with cols > 1).
func (m *FocusManager) moveTo(row, col int) {
	idx := row*m.cols + col
	if idx >= m.count {
		idx = m.count - 1
	}
	if idx < 0 {
		idx = 0
	}
	m.current = idx
}

// SetFocused sets the focused index directly, clamped into [0, count).
// A no-op (current stays 0) when count is 0.
func (m *FocusManager) SetFocused(i int) {
	if m.count <= 0 {
		m.current = 0
		return
	}
	if i < 0 {
		i = 0
	} else if i >= m.count {
		i = m.count - 1
	}
	m.current = i
}

// Focused returns the index of the focused target.
func (m *FocusManager) Focused() int { return m.current }

// Count returns the number of registered targets.
func (m *FocusManager) Count() int { return m.count }
