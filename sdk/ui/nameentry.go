// SPDX-License-Identifier: GPL-2.0-or-later

package ui

// NameEntry is the cabinet-standard high-score name entry: classic arcade
// slot cycling over a fixed number of letter slots (DefaultNameEntryMaxLen by
// default) plus a trailing OK cell, so a name can be submitted with
// directions and one activate button alone.
//
// # Input
//
// [NameEntry.Move] is a level call: pass the currently held direction every
// frame ([DirNone] when nothing is pressed). NameEntry detects the press edge itself
// and cycles or moves the cursor once on that edge. Holding Up or Down then
// auto-repeats through Update; Left and Right do not repeat.
//
// [NameEntry.Do] is an edge call: call it once per newly pressed action
// button, not every frame it is held. Read the result with [NameEntry.Done]
// and [NameEntry.Name].
//
// # Timing
//
// [NameEntry.Update] accumulates the widget's own clock from dtMicros, which
// drives the cursor blink (500 ms period) and the hold-to-scroll repeat (350 ms
// delay, then 90 ms interval). Call it every tick while the entry is active.
//
// # Layout
//
// Cells are 6px wide, the widest glyph in the charset, so the row is
// monospaced. Each glyph is centered in its cell. Confirmed letters draw
// white, empty slots a dim gray underscore and the OK cell a green checkmark;
// the cell under the cursor blinks yellow and white, never the dim empty
// color. Colors are the exported NameEntry*Color variables.
//
// Construction and configuration ([NewNameEntry], [NameEntry.SetName],
// [NameEntry.SetRecents]) allocate and belong in Init; Move, Do, Update and
// Draw do not.
//
// # Recent names
//
// SetRecents offers up to NameEntryMaxRecents names. They hang off the bottom
// of the first slot's cycling ring: at slot 0, pressing Down past the blank
// walks the list instead of wrapping to '9', and each name previews across
// the whole row. Activate takes the previewed name and parks on OK, Up backs
// out to the blank with the typed letters restored, and continuing Down past
// the list resumes the charset at '9'. The next name down is drawn faded
// under the row as the only hint the list exists.
type NameEntry struct {
	X, Y int // top-left of the letter row, in pixels

	slots   []rune
	cursor  int // 0..maxLen; maxLen is the OK cell
	maxLen  int
	prefill []rune
	done    bool

	// recentIdx is -1 in charset mode, else the index of the recent name being
	// previewed. saved holds the letters the preview paints over.
	recents   [NameEntryMaxRecents]string
	recentN   int
	recentIdx int
	saved     []rune

	lastDir Direction
	// pendingHold is the Up or Down just pressed; the hold starts on the next
	// Update, so the press frame's dt does not count toward the delay.
	pendingHold Direction
	hold        HoldRepeat

	totalMicros uint64
	blinkOn     bool
}

// DefaultNameEntryMaxLen is the default number of letter slots.
const DefaultNameEntryMaxLen = 7

// NameEntryMaxRecents is how many names SetRecents takes.
const NameEntryMaxRecents = 5

// nameEntrySlotWidth is the per-cell pixel width: the widest glyph in the
// charset ('M' and 'W').
const nameEntrySlotWidth = 6

// nameEntryInset is the x offset inside a slot cell that centers glyph's inked
// columns. w is the glyph's width in columns. A glyph with no ink, or one
// wider than the cell, gets 0.
func nameEntryInset(glyph []byte, w int) int {
	lo, hi := w, 0
	for row := 0; row < ledGlyphRows; row++ {
		for col := 0; col < w; col++ {
			if glyph[w*row+col] == 0 {
				continue
			}
			if col < lo {
				lo = col
			}
			if col >= hi {
				hi = col + 1
			}
		}
	}
	if lo >= hi {
		return 0
	}
	inset := (nameEntrySlotWidth-(hi-lo))/2 - lo
	if inset < 0 {
		return 0
	}
	return inset
}

// nameEntryCharset is the cycling order: blank, A-Z, 0-9, wrapping in both
// directions.
const nameEntryCharset = " ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

const nameEntryBlinkPeriodMicros = 500_000

var (
	// NameEntryFilledColor paints a confirmed letter, and is also the
	// blink-off color of the active cell (never the dim empty color).
	NameEntryFilledColor = Color{255, 255, 255, 255}
	// NameEntryEmptyColor paints an inactive blank slot's underscore.
	NameEntryEmptyColor = Color{120, 120, 120, 255}
	// NameEntryActiveColor is the blink-on color of the cursor's cell.
	NameEntryActiveColor = Color{255, 255, 0, 255}
	// NameEntryOKColor paints the trailing OK cell's checkmark when it is
	// not the active cell.
	NameEntryOKColor = Color{0, 255, 0, 255}
	// NameEntryPeekColor paints the faded recent-name hint under the row.
	NameEntryPeekColor = Color{80, 80, 80, 255}
)

// nameEntryOKGlyph is the OK cell's checkmark, in the flat row-major format of
// LEDGlyphs entries.
var nameEntryOKGlyph = []byte{
	0, 0, 0, 0, 0,
	0, 0, 0, 0, 0,
	0, 0, 0, 0, 1,
	0, 0, 0, 1, 0,
	1, 0, 1, 0, 0,
	0, 1, 0, 0, 0,
	0, 0, 0, 0, 0,
	0, 0, 0, 0, 0,
}

// NameEntryHoldDelayMicros and NameEntryHoldIntervalMicros are the hold
// schedule, in microseconds, a NameEntry scrolls its letters with: a held direction steps once
// after the delay and then once per interval. The delay is the cabinet's
// "did you mean to hold?" threshold, the same one a cartridge should use for
// any held-direction repeat; the interval suits a 26-letter ring and a
// shorter list may want a longer one.
const (
	NameEntryHoldDelayMicros    = 350_000
	NameEntryHoldIntervalMicros = 90_000
)

// NewNameEntry builds a blank NameEntry with its top-left at (x, y) pixels and
// maxLen letter slots; maxLen <= 0 means [DefaultNameEntryMaxLen]. Call it from Init: it allocates.
func NewNameEntry(x, y, maxLen int) *NameEntry {
	if maxLen <= 0 {
		maxLen = DefaultNameEntryMaxLen
	}
	n := &NameEntry{X: x, Y: y, maxLen: maxLen,
		hold: HoldRepeat{DelayMicros: NameEntryHoldDelayMicros, IntervalMicros: NameEntryHoldIntervalMicros}}
	n.slots = make([]rune, maxLen)
	n.saved = make([]rune, maxLen)
	n.reset()
	n.recomputeBlink()
	return n
}

// sanitizeNameEntryInput maps a name onto the charset: lowercase is
// uppercased, characters outside A-Z, 0-9 and space are dropped, trailing
// spaces are trimmed and the result is cut to maxLen.
func sanitizeNameEntryInput(name string, maxLen int) []rune {
	out := make([]rune, 0, maxLen)
	for _, c := range name {
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != ' ' {
			continue
		}
		out = append(out, c)
		if len(out) == maxLen {
			break
		}
	}
	for len(out) > 0 && out[len(out)-1] == ' ' {
		out = out[:len(out)-1]
	}
	return out
}

// SetName sets the prefill and resets the widget to it. The name is
// uppercased, stripped of anything outside A-Z, 0-9 and space, trimmed of
// trailing spaces and cut to the slot count. A non-empty prefill starts the
// cursor on OK so a returning player confirms with one [NameEntry.Do] of
// [ActionActivate]. An empty name clears
// the prefill and restores the blank start at slot 0. It allocates, so call it
// from Init, not per frame.
func (n *NameEntry) SetName(name string) {
	n.prefill = sanitizeNameEntryInput(name, n.maxLen)
	n.reset()
}

// Reset returns the widget to its starting state: the prefill with the cursor
// on OK when one is set, otherwise blank at slot 0. The prefill is unchanged.
func (n *NameEntry) Reset() { n.reset() }

func (n *NameEntry) reset() {
	for i := range n.slots {
		n.slots[i] = ' '
	}
	copy(n.slots, n.prefill)
	n.cursor = 0
	if len(n.prefill) > 0 {
		n.cursor = n.maxLen
	}
	n.done = false
	n.lastDir = DirNone
	n.recentIdx = -1
	n.clearHold()
}

// SetRecents offers up to [NameEntryMaxRecents] recently used names below the
// charset; see the type doc. Names are sanitized and truncated like a
// prefill, blanks and duplicates are dropped, and names past the fifth
// survivor are ignored. With no surviving names the widget behaves as if
// recents were never offered. Call it from Init, alongside SetName.
func (n *NameEntry) SetRecents(names []string) {
	n.recentN = 0
	n.recentIdx = -1
	for _, raw := range names {
		name := string(sanitizeNameEntryInput(raw, n.maxLen))
		if name == "" {
			continue
		}
		dup := false
		for i := 0; i < n.recentN; i++ {
			if n.recents[i] == name {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		n.recents[n.recentN] = name
		n.recentN++
		if n.recentN == NameEntryMaxRecents {
			return
		}
	}
}

// enterRecents starts previewing the recent list from its top.
func (n *NameEntry) enterRecents() {
	copy(n.saved, n.slots)
	n.recentIdx = 0
	n.showRecent()
}

// showRecent paints the previewed name across the slots without allocating.
func (n *NameEntry) showRecent() {
	i := 0
	for _, r := range n.recents[n.recentIdx] {
		if i >= n.maxLen {
			break
		}
		n.slots[i] = r
		i++
	}
	for ; i < n.maxLen; i++ {
		n.slots[i] = ' '
	}
}

// leaveRecents drops the preview, restores the typed letters and sets the
// cursor's slot to c, the charset character the ring continues from.
func (n *NameEntry) leaveRecents(c rune) {
	copy(n.slots, n.saved)
	n.recentIdx = -1
	n.slots[n.cursor] = c
}

// keepRecent ends the preview by accepting it.
func (n *NameEntry) keepRecent() {
	n.recentIdx = -1
	copy(n.saved, n.slots)
}

// cycleRecents walks the recent list, leaving into the charset at either end.
// delta follows cycleChar: +1 is Up, -1 is Down, and Down goes deeper.
func (n *NameEntry) cycleRecents(delta int) {
	idx := n.recentIdx + 1
	if delta > 0 {
		idx = n.recentIdx - 1
	}
	switch {
	case idx < 0:
		n.leaveRecents(' ')
	case idx >= n.recentN:
		n.leaveRecents(rune(nameEntryCharset[len(nameEntryCharset)-1]))
	default:
		n.recentIdx = idx
		n.showRecent()
	}
}

func (n *NameEntry) clearHold() {
	n.pendingHold = DirNone
	n.hold.Reset()
}

func (n *NameEntry) pressHold(dir Direction) {
	n.clearHold()
	n.pendingHold = dir
}

// Done reports whether the player has submitted with [ActionActivate] on the
// OK cell. Once true the widget ignores input until [NameEntry.Reset].
func (n *NameEntry) Done() bool { return n.done }

// Name returns the contents with trailing blanks removed, valid mid-entry and
// after Done. An all-blank entry returns "".
func (n *NameEntry) Name() string {
	end := len(n.slots)
	for end > 0 && n.slots[end-1] == ' ' {
		end--
	}
	return string(n.slots[:end])
}

// cycleChar moves c through nameEntryCharset by delta, wrapping at either end.
func cycleChar(c rune, delta int) rune {
	idx := -1
	for i := 0; i < len(nameEntryCharset); i++ {
		if rune(nameEntryCharset[i]) == c {
			idx = i
			break
		}
	}
	if idx < 0 {
		idx = 0
	}
	n := len(nameEntryCharset)
	idx = ((idx+delta)%n + n) % n
	return rune(nameEntryCharset[idx])
}

// cycleCurrent moves the focused slot one step around its ring. At slot 0
// with recents offered and the slot blank, a downward step enters the recent
// list instead.
func (n *NameEntry) cycleCurrent(delta int) {
	if n.cursor >= n.maxLen {
		return
	}
	if n.recentIdx >= 0 {
		n.cycleRecents(delta)
		return
	}
	if delta < 0 && n.atRecentsDoor() {
		n.enterRecents()
		return
	}
	n.slots[n.cursor] = cycleChar(n.slots[n.cursor], delta)
}

// atRecentsDoor reports whether a Down step would enter the recent list.
func (n *NameEntry) atRecentsDoor() bool {
	return n.cursor == 0 && n.recentN > 0 && n.slots[0] == ' '
}

// Move reports the currently held direction; call it every frame with [DirNone]
// when nothing is pressed. On the press edge, Up and Down cycle the focused
// slot (auto-repeating through Update while held) and Left and Right move the
// cursor across all cells including OK, clamped, without repeating. Ignored
// once Done.
func (n *NameEntry) Move(dir Direction) {
	if dir == n.lastDir {
		return
	}
	n.lastDir = dir
	if n.done {
		n.clearHold()
		return
	}
	switch dir {
	case DirUp:
		n.cycleCurrent(1)
		n.pressHold(DirUp)
	case DirDown:
		n.cycleCurrent(-1)
		n.pressHold(DirDown)
	case DirLeft:
		n.keepIfPreviewing()
		if n.cursor > 0 {
			n.cursor--
		}
		n.clearHold()
	case DirRight:
		n.keepIfPreviewing()
		if n.cursor < n.maxLen {
			n.cursor++
		}
		n.clearHold()
	case DirNone:
		n.clearHold()
	}
}

// Do fires one edge-triggered action; call it once per newly pressed button.
// [ActionActivate] on a filled slot confirms it and advances one cell (off the
// last slot onto OK), on an empty slot jumps to the OK cell, and on the OK
// cell submits. During a recent-name preview it accepts the name and jumps to
// OK. [ActionErase] clears the current slot and moves left, or just moves left on
// an empty slot or the OK cell; during a preview it backs out to the blank
// with the typed letters restored. [ActionBack] and every other action are
// ignored, leaving B free for the host's quit handling. Everything is ignored
// once Done.
func (n *NameEntry) Do(act Action) {
	if n.done {
		return
	}
	switch act {
	case ActionActivate:
		if n.recentIdx >= 0 {
			n.keepRecent()
			n.cursor = n.maxLen
			return
		}
		switch {
		case n.cursor == n.maxLen:
			n.submit()
		case n.slots[n.cursor] == ' ':
			n.cursor = n.maxLen
		default:
			n.cursor++
		}
	case ActionErase:
		if n.recentIdx >= 0 {
			n.leaveRecents(' ')
			return
		}
		if n.cursor < n.maxLen {
			n.slots[n.cursor] = ' '
		}
		if n.cursor > 0 {
			n.cursor--
		}
	}
}

// keepIfPreviewing accepts a recent-name preview if one is up.
func (n *NameEntry) keepIfPreviewing() {
	if n.recentIdx >= 0 {
		n.keepRecent()
	}
}

func (n *NameEntry) submit() {
	n.done = true
	n.clearHold()
}

// Update advances the blink phase and the hold-to-scroll auto-repeat by
// dtMicros microseconds. Call it every tick from Update while the entry is
// active.
func (n *NameEntry) Update(dtMicros uint64) {
	n.totalMicros += dtMicros
	n.recomputeBlink()
	if n.done {
		return
	}
	if n.pendingHold != DirNone {
		n.hold.Hold(n.pendingHold)
		n.pendingHold = DirNone
		return
	}
	delta := 1
	if n.hold.Held() == DirDown {
		delta = -1
	}
	for steps := n.hold.Update(dtMicros); steps > 0; steps-- {
		n.cycleCurrent(delta)
	}
}

func (n *NameEntry) recomputeBlink() {
	phase := (n.totalMicros / nameEntryBlinkPeriodMicros) % 2
	n.blinkOn = phase == 0
}

// Draw composites the letter slots, the OK cell and, when recents are
// offered, the faded peek line under the row.
func (n *NameEntry) Draw(cv Canvas) {
	for i := 0; i <= n.maxLen; i++ {
		var glyph []byte
		var base Color
		switch {
		case i == n.maxLen:
			glyph = nameEntryOKGlyph
			base = NameEntryOKColor
		case n.slots[i] == ' ':
			glyph = LEDGlyphs['_']
			base = NameEntryEmptyColor
		default:
			glyph = LEDGlyphs[n.slots[i]]
			base = NameEntryFilledColor
		}
		color := base
		if i == n.cursor {
			color = NameEntryActiveColor
			if !n.blinkOn {
				color = NameEntryFilledColor
			}
		}
		w := 0
		if ledGlyphRows > 0 {
			w = len(glyph) / ledGlyphRows
		}
		ox := n.X + i*nameEntrySlotWidth + nameEntryInset(glyph, w)
		for row := 0; row < ledGlyphRows; row++ {
			for col := 0; col < w; col++ {
				if glyph[w*row+col] == 1 {
					cv.Blend(ox+col, n.Y+row, color)
				}
			}
		}
	}
	n.drawPeek(cv)
}

// LetterBand returns the canvas rows the letter slots and OK cell occupy: rows
// y through y+rows-1 that the letter slots and OK cell occupy, in pixels. The
// faded recent-name line below them is not included. Pass it to [HoldConfirm.Avoid] when the quit overlay opens over
// the entry.
func (n *NameEntry) LetterBand() (y, rows int) { return n.Y, ledGlyphRows }

// nameEntryPeekChevron is the 3x2 mark left of the peek line showing that the
// faded name is further down.
var nameEntryPeekChevron = []byte{
	1, 0, 1,
	0, 1, 0,
}

// drawPeek renders the faded line under the slot row: the recent name one
// step further Down from the cursor, shown while the cursor is on a blank
// slot 0 or a preview is up, and not once the list is exhausted or the entry
// is submitted. A Canvas clips anything past its bounds, so an entry at the
// bottom of the panel loses the hint, not the frame.
func (n *NameEntry) drawPeek(cv Canvas) {
	if n.recentN == 0 || n.done {
		return
	}
	name := ""
	switch {
	case n.recentIdx >= 0:
		if n.recentIdx+1 < n.recentN {
			name = n.recents[n.recentIdx+1]
		}
	case n.atRecentsDoor():
		name = n.recents[0]
	}
	if name == "" {
		return
	}
	y := n.Y + ledGlyphRows + 1
	for row := 0; row < 2; row++ {
		for col := 0; col < 3; col++ {
			if nameEntryPeekChevron[3*row+col] == 1 {
				cv.Blend(n.X+col, y+row+2, NameEntryPeekColor)
			}
		}
	}
	Font3x5{}.DrawText(cv, n.X+glyph3x5Advance, y, name, 1, NameEntryPeekColor)
}
