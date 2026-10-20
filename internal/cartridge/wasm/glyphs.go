// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"context"

	"github.com/tetratelabs/wazero/api"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/lcdrune"
)

type lcdImports struct {
	host interface {
		LCD
		Logger
	}
	glyphs *glyphRegistry
}

func (l lcdImports) setText(_ context.Context, mod api.Module, player int32, l1Ptr, l1Len, l2Ptr, l2Len uint32) {
	seats, ok := seatArg(l.host, "host_set_text: player", player)
	if !ok {
		return
	}
	line1, ok := guestString(l.host, mod, "host_set_text: line1", l1Ptr, l1Len, maxLcdLineBytes)
	if !ok {
		return
	}
	line2, ok := guestString(l.host, mod, "host_set_text: line2", l2Ptr, l2Len, maxLcdLineBytes)
	if !ok {
		return
	}
	l.host.SetText(seats, line1, line2)
}

func (l lcdImports) defineGlyph(_ context.Context, _ api.Module, codepoint int32, rows uint64) uint32 {
	return l.glyphs.define("host_define_lcd_glyph", rune(codepoint), unpackGlyphRows(rows))
}

// unpackGlyphRows puts the top row in the low byte and keeps 5 bits per row,
// bit 4 leftmost as HD44780 CGRAM expects, so junk high bits draw the same
// glyph on every host.
func unpackGlyphRows(packed uint64) [8]byte {
	var rows [8]byte
	for i := range rows {
		rows[i] = byte(packed>>(8*uint(i))) & 0x1f
	}
	return rows
}

// glyphRegistry is one cartridge's LCD glyph table, filled by both
// host_define_lcd_glyph and assets.glyphs; a later definition of a rune wins.
type glyphRegistry struct {
	host    bundleReader
	definer GlyphDefiner // nil when the Host has no character LCDs
	table   map[rune][8]byte
}

func newGlyphRegistry(host bundleReader) *glyphRegistry {
	definer, _ := host.(GlyphDefiner)
	return &glyphRegistry{host: host, definer: definer, table: map[rune][8]byte{}}
}

// define returns 1 when stored, 0 when refused. who names the caller in
// warnings: the import or the glyph file's path.
func (g *glyphRegistry) define(who string, r rune, rows [8]byte) uint32 {
	// The browser host's defineGlyph spells the same three warnings.
	switch lcdrune.Reason(r) {
	case "":
	case lcdrune.ReasonASCII:
		// Redefining ASCII would burn a CGRAM slot on every line using it.
		hostWarnf(g.host, "%s: code point %#x is ASCII — the LCD's own ROM draws those", who, r)
		return 0
	case lcdrune.ReasonNotACodePoint:
		hostWarnf(g.host, "%s: %#x is not a code point", who, r)
		return 0
	default:
		hostWarnf(g.host, "%s: %#x is %s — an LCD glyph rune must be a plain, single-width, monochrome character (terminals draw this one in colour or two cells wide)", who, r, lcdrune.Reason(r))
		return 0
	}
	if _, known := g.table[r]; !known && len(g.table) >= lcdrune.MaxCartGlyphs {
		hostWarnf(g.host, "%s: table is full at %d glyphs, dropping %q", who, lcdrune.MaxCartGlyphs, r)
		return 0
	}
	g.table[r] = rows
	g.publish()
	return 1
}

// publish hands over the whole table each time: the overlay contract is replacement.
func (g *glyphRegistry) publish() {
	if g.definer == nil {
		return
	}
	table := make(map[rune][8]byte, len(g.table))
	for k, v := range g.table {
		table[k] = v
	}
	g.definer.SetCartGlyphs(table)
}

func (g *glyphRegistry) loadBundle(paths []string) {
	for _, p := range paths {
		env, ok := readBundleEnvelope(g.host, "glyph", p, assetfile.KindGlyph)
		if !ok {
			continue
		}
		var rows [8]byte
		for i, v := range env.Rows {
			// Matches unpackGlyphRows' mask.
			rows[i] = byte(v) & 0x1f
		}
		g.define(p, rune(env.Codepoint), rows)
	}
}

// dropBundle clears the overlay, which is global to the hardware suite, and
// with it the runtime-defined glyphs.
func (g *glyphRegistry) dropBundle() {
	if g.definer != nil {
		g.definer.SetCartGlyphs(nil)
	}
}
