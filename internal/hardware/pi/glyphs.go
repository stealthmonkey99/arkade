// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"sync"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// customChars holds the 5x8 pixel rows for runes the HD44780 ROM lacks. Every
// entry has a glyphMeta entry. The chip has 8 CGRAM slots, so each screen
// pages glyphs in through an LRU cache; this table is the palette, not a
// limit.
var customChars = map[rune][8]byte{
	'♡': {0x0, 0x0, 0xa, 0x15, 0x11, 0xa, 0x4, 0x0},
	'♪': {0x2, 0x3, 0x2, 0x2, 0xe, 0x1e, 0xc, 0x0},
	'⏲': {0x0, 0xe, 0x15, 0x17, 0x11, 0xe, 0x0, 0x0},
	'⍾': {0x4, 0xe, 0xe, 0xe, 0x1f, 0x0, 0x4, 0x0},
	'⚿': {0x0, 0xe, 0x11, 0x11, 0x1f, 0x1b, 0x1f, 0x1f},
	'☺': {0x0, 0x0, 0xa, 0x0, 0x11, 0xe, 0x0, 0x0},
	'☹': {0x0, 0x0, 0xa, 0x0, 0xe, 0x11, 0x0, 0x0},
	'●': {0x0, 0xe, 0x1f, 0x1f, 0x1f, 0xe, 0x0, 0x0},
	'⏶': {0x04, 0x0e, 0x1f, 0x00, 0x00, 0x00, 0x00, 0x00},
	'☑': {0x04, 0x0e, 0x00, 0x01, 0x03, 0x16, 0x1c, 0x08},
	'☒': {0x04, 0x0e, 0x00, 0x1b, 0x0e, 0x04, 0x0e, 0x1b},
	'✔': {0x0, 0x1, 0x3, 0x16, 0x1c, 0x8, 0x0, 0x0},
	'✖': {0x0, 0x1b, 0xe, 0x4, 0xe, 0x1b, 0x0, 0x0},
	'→': {0x0, 0x8, 0xc, 0xe, 0xc, 0x8, 0x0, 0x0},
	'↑': {0x0, 0x0, 0x4, 0xe, 0x1f, 0x0, 0x0, 0x0},
	'←': {0x0, 0x2, 0x6, 0xe, 0x6, 0x2, 0x0, 0x0},
	'↓': {0x0, 0x0, 0x1f, 0xe, 0x4, 0x0, 0x0, 0x0},
	'⏎': {0x1c, 0x2, 0x1, 0x5, 0x9, 0x1e, 0x8, 0x4},
	'♥': {0x0, 0x0, 0xa, 0x1f, 0x1f, 0xe, 0x4, 0x0},
	'♠': {0x0, 0x4, 0xe, 0x1f, 0x1f, 0x4, 0xe, 0x0},
	'♦': {0x0, 0x4, 0xe, 0x1f, 0xe, 0x4, 0x0, 0x0},
	'♣': {0x4, 0xe, 0x4, 0x1f, 0x1f, 0x4, 0xe, 0x0},
	'⚀': {0x0, 0x0, 0x0, 0x4, 0x0, 0x0, 0x0, 0x0},
	'⚁': {0x0, 0x10, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0},
	'⚂': {0x0, 0x10, 0x0, 0x4, 0x0, 0x1, 0x0, 0x0},
	'⚃': {0x0, 0x11, 0x0, 0x0, 0x0, 0x11, 0x0, 0x0},
	'⚄': {0x0, 0x11, 0x0, 0x4, 0x0, 0x11, 0x0, 0x0},
	'⚅': {0x0, 0x11, 0x0, 0x11, 0x0, 0x11, 0x0, 0x0},
	'█': {0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f, 0x1f},
	'▓': {0x0f, 0x1b, 0x1e, 0x17, 0x1d, 0x0f, 0x1b, 0x1e},
	'▒': {0x15, 0xa, 0x15, 0xa, 0x15, 0xa, 0x15, 0xa},
	'░': {0x10, 0x04, 0x01, 0x08, 0x02, 0x10, 0x04, 0x01},
	'Å': {0x4, 0xa, 0x4, 0xe, 0x11, 0x1f, 0x11, 0x11},
	'©': {0xe, 0x11, 0x17, 0x19, 0x19, 0x17, 0x11, 0xe},
}

// glyphInfo is one glyph's name, category and one-line purpose, the same
// fields a bundled glyph envelope carries.
type glyphInfo struct {
	name        string
	category    string
	description string
}

const (
	catArrows  = "arrows"
	catShades  = "shades"
	catSuits   = "suits"
	catDice    = "dice"
	catMarks   = "marks"
	catSymbols = "symbols"
)

// glyphMeta is customChars' prose, kept apart so the bitmap literal keeps the
// shape other tooling parses. TestGlyphMetaCoversEveryGlyph keeps the two in
// step.
var glyphMeta = map[rune]glyphInfo{
	'♡': {"heart-hollow", catSymbols, "Outlined heart: a life already spent, or an unclaimed slot."},
	'♪': {"note", catSymbols, "Eighth note: the sound-is-on indicator."},
	'⏲': {"timer", catSymbols, "Timer face with a stem, for a countdown label."},
	'⍾': {"bell", catSymbols, "Bell: an alert, or a turn that has come round."},
	'⚿': {"key", catSymbols, "Key: something locked, or the key that opens it."},
	'☺': {"face-happy", catSymbols, "Smiling face: a good outcome in one cell."},
	'☹': {"face-sad", catSymbols, "Frowning face: a bad outcome in one cell."},
	'●': {"disc", catMarks, "Filled disc: a bullet, a pip, an occupied slot."},
	'⏶': {"caret", catMarks, "Caret in the cell's top rows: a marker under the value above it."},
	'☑': {"caret-check", catMarks, "The caret over ✔: the cursor is on a card already picked."},
	'☒': {"caret-cross", catMarks, "The caret over ✖: the cursor is on a card already refused."},
	'✔': {"check", catMarks, "Check mark: chosen, confirmed, correct."},
	'✖': {"cross", catMarks, "Heavy cross: refused, wrong, out of reach."},
	'→': {"arrow-right", catArrows, "Right arrow, centred in the cell."},
	'↑': {"arrow-up", catArrows, "Up arrow, centred in the cell (see ⏶ for a column marker)."},
	'←': {"arrow-left", catArrows, "Left arrow, centred in the cell."},
	'↓': {"arrow-down", catArrows, "Down arrow, centred in the cell."},
	'⏎': {"return", catArrows, "Return arrow: the confirm key, as the panel draws it."},
	'♥': {"heart", catSuits, "Filled heart: the hearts suit, and the pip a lives counter spends."},
	'♠': {"spade", catSuits, "Spade suit, drawn in the heart's six-row style."},
	'♦': {"diamond", catSuits, "Diamond suit, drawn in the heart's six-row style."},
	'♣': {"club", catSuits, "Club suit; its knobbed top row keeps it clear of the spade."},
	'⚀': {"die-1", catDice, "Die face one: a single centre pip."},
	'⚁': {"die-2", catDice, "Die face two: pips on the falling diagonal."},
	'⚂': {"die-3", catDice, "Die face three: pips on the falling diagonal through centre."},
	'⚃': {"die-4", catDice, "Die face four: pips in the four corners."},
	'⚄': {"die-5", catDice, "Die face five: four corners and a centre pip."},
	'⚅': {"die-6", catDice, "Die face six: two clean pip columns, never a merged block."},
	'█': {"shade-full", catShades, "Density ramp, 100% lit: a bar's filled cell."},
	'▓': {"shade-dark", catShades, "Density ramp, ~80% lit: the rung below full."},
	'▒': {"shade-medium", catShades, "Density ramp, ~50% lit: the checkerboard middle rung."},
	'░': {"shade-light", catShades, "Density ramp, ~20% lit: a bar's barely-started cell."},
	'Å': {"a-ring", catSymbols, "Capital A with a ring: the Å the cabinet spells its own name with."},
	'©': {"copyright", catSymbols, "Copyright sign, for an attract-screen byline."},
}

// maxCGRAMSlots is the HD44780 CGRAM size.
const maxCGRAMSlots = 8

// glyphTable is the preview and cartridge overlays on customChars for one
// pair of screens. The engine goroutine replaces overlays while screen workers
// read them; each replacement bumps the generation, which tells a screen its
// CGRAM slots hold stale bitmaps.
type glyphTable struct {
	mu      sync.RWMutex
	preview map[rune][8]byte
	cart    map[rune][8]byte
	gen     uint64
}

// glyphView is a consistent read of a glyphTable. Overlay maps are replaced,
// never mutated, so a view stays valid after the table moves on.
type glyphView struct {
	gen     uint64
	preview map[rune][8]byte
	cart    map[rune][8]byte
}

func (t *glyphTable) view() glyphView {
	if t == nil {
		return glyphView{}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return glyphView{gen: t.gen, preview: t.preview, cart: t.cart}
}

// lookup resolves a rune through the preview overlay, the cartridge overlay,
// then the built-in table.
func (v glyphView) lookup(r rune) ([8]byte, bool) {
	if rows, ok := v.preview[r]; ok {
		return rows, true
	}
	if rows, ok := v.cart[r]; ok {
		return rows, true
	}
	rows, ok := customChars[r]
	return rows, ok
}

func (t *glyphTable) setPreview(glyphs map[rune]hardware.GlyphRows) {
	next := copyGlyphs(glyphs)
	t.mu.Lock()
	t.preview = next
	t.gen++
	t.mu.Unlock()
}

func (t *glyphTable) setCart(glyphs map[rune]hardware.GlyphRows) {
	next := copyGlyphs(glyphs)
	t.mu.Lock()
	t.cart = next
	t.gen++
	t.mu.Unlock()
}

func copyGlyphs(in map[rune]hardware.GlyphRows) map[rune][8]byte {
	if len(in) == 0 {
		return nil
	}
	out := make(map[rune][8]byte, len(in))
	for r, rows := range in {
		out[r] = rows
	}
	return out
}

// SetPreviewGlyphs installs the preview overlay, or clears it with an empty
// map. Both LCDs draw through it from their next text write, re-uploading
// runes already showing.
func (s *Suite) SetPreviewGlyphs(glyphs map[rune]hardware.GlyphRows) { s.glyphs.setPreview(glyphs) }

// SetCartGlyphs installs the cartridge overlay, or clears it with an empty
// map, under the same redraw rule as SetPreviewGlyphs.
func (s *Suite) SetCartGlyphs(glyphs map[rune]hardware.GlyphRows) { s.glyphs.setCart(glyphs) }

var _ hardware.GlyphPager = (*Suite)(nil)

// lruCache maps a custom rune to its CGRAM slot. Insert reuses an evicted
// entry's slot number.
type lruCache struct {
	cap   int
	order []rune // index 0 = least recently used
	slots map[rune]int
}

func newLRUCache(capacity int) *lruCache {
	return &lruCache{cap: capacity, slots: make(map[rune]int, capacity)}
}

func (c *lruCache) has(k rune) bool {
	_, ok := c.slots[k]
	return ok
}

func (c *lruCache) get(k rune) (int, bool) {
	v, ok := c.slots[k]
	if !ok {
		return 0, false
	}
	c.touch(k)
	return v, true
}

// insert assigns k a slot: its existing one, the evicted LRU entry's when
// full, else the next free index.
func (c *lruCache) insert(k rune) int {
	if v, ok := c.slots[k]; ok {
		c.touch(k)
		return v
	}
	var v int
	if len(c.order) == c.cap {
		oldest := c.order[0]
		v = c.slots[oldest]
		delete(c.slots, oldest)
		c.order = c.order[1:]
	} else {
		v = len(c.order)
	}
	c.order = append(c.order, k)
	c.slots[k] = v
	return v
}

func (c *lruCache) touch(k rune) {
	for i, r := range c.order {
		if r == k {
			c.order = append(append(append([]rune{}, c.order[:i]...), c.order[i+1:]...), k)
			return
		}
	}
}
