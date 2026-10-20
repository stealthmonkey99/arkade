// SPDX-License-Identifier: GPL-2.0-or-later

package ui

// AttractScore is one line of attract-mode standings: a player name and a
// score. Name may be empty, in which case only the score is drawn.
type AttractScore struct {
	Name  string // player name; empty draws the score alone
	Score int    // points; negative values draw as 0
}

// attractRowH is the vertical pitch between AttractPanel lines, in pixels.
const attractRowH = 10

// AttractPanel draws the standard score block onto a Canvas: a title, then
// the leaderboard, one rank per line, #1 first. Text is drawn in FontLED,
// plus a small procedural trophy; a cartridge that wants a game icon draws
// its own [Sprite] beside the panel.
//
// It draws only inside [X, X+W) x [Y, Y+H), never clears, holds no state
// between calls and is a pure function of the struct, so it can serve as an
// in-game HUD row as well as an attract screen. Call it from Draw. It
// allocates nothing.
//
// # Layout
//
// Each present line sits attractRowH (10) pixels below the previous one:
// Title, then Top (trophy in the left gutter), then Board[0], Board[1], and
// so on. A nil Board omits the list. A zero-value Top (empty Name and zero
// Score) omits the #1 line; a Top or Board row with a score but no name
// draws the score alone. Rows are drawn in order and the first row that
// would fall outside [Y, Y+H) is skipped along with everything after it, so
// the bottom edge shows whole rows or nothing. Every score line has the
// shape `<name>  <score>` (two spaces between); first place is marked by
// AccentColor and the trophy, not by a different shape.
//
// Text is drawn as given, with no case conversion.
//
// # Ranks
//
// With Ranks set, every row carries its position number in a uniform left
// gutter. The gutter is measured once per Draw and fits the trophy or the
// widest numeral present, whichever is wider, plus a 2px gap. When Trophy is
// set the trophy stands in for the numeral on the Top row. FirstRank
// renumbers the stack for a caller that shows a window onto a longer board.
// With Ranks unset, the gutter is charged to the #1 row only.
//
// # Width
//
// A row whose name, gutter and score fit in the width available is drawn
// statically. A row that does not fit depends on T:
//
//   - T == 0: the score is drawn in full and the name is cut to whole glyphs
//     to fit, with no ellipsis.
//   - T != 0: the whole `<name>  <score>` strip scrolls inside the row, holding
//     on the name, sliding to the score, holding there and sliding back, on
//     a 3.5 second cycle.
//
// Rows are measured independently, so only an overflowing row moves, and all
// moving rows share one phase. The title follows the same rule. All text is
// clipped to [X, X+W).
//
// # Clock
//
// T is the caller's accumulated dtMicros; the panel never reads a clock of
// its own. Zero means "no clock", so a caller whose accumulator reads 0 on
// its first frame draws one truncated frame before the cycle starts.
type AttractPanel struct {
	X, Y, W, H int            // bounds, in pixels; nothing is drawn when W or H is not positive
	Title      string         // heading line in the accent color; empty omits it
	Top        AttractScore   // the #1 line
	Board      []AttractScore // ranks 2..N
	// Color is the board rows' text color; the zero value is White.
	Color Color
	// AccentColor is the Title and #1 row color; the zero value is Color.
	AccentColor Color
	// Trophy draws a gold cup in the left gutter of the #1 row and pushes
	// that row's text right. Board rows are indented to match only with Ranks.
	Trophy bool
	// TrophyColor overrides the trophy's gold; the zero value is gold.
	TrophyColor Color
	// T is the animation clock: the caller's accumulated dtMicros. Zero
	// means no clock and nothing animates.
	T uint64
	// Ranks draws each row's position number in a uniform left gutter.
	Ranks bool
	// RankColor overrides the rank numerals' color; the zero value is a
	// dimmed shade of the row's own color.
	RankColor Color
	// FirstRank is the number given to the first row drawn (the Top row, or
	// Board[0] when Top is empty). Zero means 1. Only meaningful with Ranks.
	FirstRank int
}

// trophyGlyph is the 8x8 gold cup: 1 is bright gold, 2 is dark gold, 0 is
// transparent.
var trophyGlyph = [8 * 8]byte{
	0, 1, 1, 1, 1, 1, 1, 0,
	1, 0, 1, 1, 1, 1, 0, 1,
	1, 0, 1, 1, 1, 1, 0, 1,
	0, 0, 1, 1, 1, 1, 0, 0,
	0, 0, 0, 1, 1, 0, 0, 0,
	0, 0, 0, 1, 1, 0, 0, 0,
	0, 0, 2, 1, 1, 2, 0, 0,
	0, 2, 2, 2, 2, 2, 2, 0,
}

const trophyW = 8

var (
	trophyBright = Color{R: 252, G: 192, B: 32, A: 255}
	trophyDark   = Color{R: 176, G: 128, B: 16, A: 255}
)

// drawTrophy draws the two-tone gold cup. A non-transparent bright overrides
// the lit gold and the dark base is scaled from it.
func drawTrophy(cv Canvas, x, y, scale int, bright Color) {
	if scale <= 0 {
		scale = 1
	}
	hi, lo := trophyBright, trophyDark
	if bright.A != 0 {
		hi = bright
		dim := func(v byte) byte { return byte(uint32(v) * 11 / 16) }
		lo = Color{R: dim(bright.R), G: dim(bright.G), B: dim(bright.B), A: 255}
	}
	for row := 0; row < 8; row++ {
		for col := 0; col < trophyW; col++ {
			var c Color
			switch trophyGlyph[row*trophyW+col] {
			case 1:
				c = hi
			case 2:
				c = lo
			default:
				continue
			}
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					cv.Blend(x+col*scale+sx, y+row*scale+sy, c)
				}
			}
		}
	}
}

// An overflowing row holds on its name, slides until the score is flush
// right, holds there and slides back.
const (
	attractHoldMicros  = 1_200_000
	attractSlideMicros = 550_000
	attractCycleMicros = 2*attractHoldMicros + 2*attractSlideMicros
)

// attractScrollOffset is how far left, in pixels, an overflowing row's text
// strip is shifted at clock t, for a strip that overruns its space by maxOff
// pixels: 0 shows the first glyph at the row's left edge, maxOff the last
// glyph at its right edge. maxOff <= 0 returns 0 at every t.
func attractScrollOffset(t uint64, maxOff int) int {
	if maxOff <= 0 {
		return 0
	}
	u := int(t % attractCycleMicros)
	switch {
	case u < attractHoldMicros:
		return 0
	case u < attractHoldMicros+attractSlideMicros:
		return attractEase(u-attractHoldMicros, maxOff)
	case u < 2*attractHoldMicros+attractSlideMicros:
		return maxOff
	default:
		return maxOff - attractEase(u-(2*attractHoldMicros+attractSlideMicros), maxOff)
	}
}

// attractEase maps elapsed microseconds within one slide onto [0, span] with
// a smoothstep ramp, in 1/1024 fixed point so elapsed*1024 fits wasm32's
// 32-bit int.
func attractEase(elapsed, span int) int {
	if elapsed >= attractSlideMicros {
		return span
	}
	if elapsed <= 0 {
		return 0
	}
	const one = 1 << 10
	p := elapsed * one / attractSlideMicros
	p2 := p * p / one
	p3 := p2 * p / one
	return span * (3*p2 - 2*p3) / one
}

// Draw composites the panel onto cv, top line first. Nothing is drawn when W
// or H is not positive.
func (p AttractPanel) Draw(cv Canvas) {
	if p.W <= 0 || p.H <= 0 {
		return
	}
	textColor := p.Color
	if textColor.A == 0 {
		textColor = White
	}
	accent := p.AccentColor
	if accent.A == 0 {
		accent = textColor
	}

	const scale = 1
	font := FontLED{}
	lineH := font.Height(scale)
	bottom := p.Y + p.H
	y := p.Y
	clipL, clipR := p.X, p.X+p.W

	if p.Title != "" {
		if y+lineH > bottom {
			return
		}
		off := attractScrollOffset(p.T, font.TextWidth(p.Title, scale)-p.W)
		drawTextClipped(cv, p.X-off, y, p.Title, scale, accent, clipL, clipR)
		y += attractRowH
	}

	gutter := p.gutterW(scale)
	rank := p.FirstRank
	if rank <= 0 {
		rank = 1
	}

	if p.Top.Name != "" || p.Top.Score != 0 {
		if y+lineH > bottom {
			return
		}
		p.drawScoreRow(cv, y, p.Top, rank, gutter, p.Trophy, accent, scale)
		y += attractRowH
		rank++
	}

	for _, row := range p.Board {
		if y+lineH > bottom {
			return
		}
		p.drawScoreRow(cv, y, row, rank, gutter, false, textColor, scale)
		y += attractRowH
		rank++
	}
}

// attractRowGutter separates a row's name from its score.
const attractRowGutter = "  "

// attractTokenGap is the blank pixel columns between the left gutter's token
// and the row's text.
const attractTokenGap = 2

// attractTrophyGutter is the trophy's width plus the gap before the text.
const attractTrophyGutter = trophyW + attractTokenGap

// gutterW measures the left gutter once per Draw so every row's text starts
// on the same column. Without Ranks it is the trophy's gutter or nothing;
// with Ranks it is the widest token in the stack plus the gap.
func (p AttractPanel) gutterW(scale int) int {
	if !p.Ranks {
		if p.Trophy {
			return attractTrophyGutter * scale
		}
		return 0
	}
	first := p.FirstRank
	if first <= 0 {
		first = 1
	}
	n := len(p.Board)
	if p.Top.Name != "" || p.Top.Score != 0 {
		n++
	}
	if n < 1 {
		n = 1
	}
	w := ledUintWidth(first+n-1, scale)
	if p.Trophy && trophyW*scale > w {
		w = trophyW * scale
	}
	return w + attractTokenGap*scale
}

// drawScoreRow draws one leaderboard line: the left gutter's token, then the
// "<name>  <score>" strip in the width left. The gutter is charged to this row
// under Ranks or when it carries the trophy.
func (p AttractPanel) drawScoreRow(cv Canvas, y int, row AttractScore, rank, gutter int, trophy bool, c Color, scale int) {
	indent := 0
	if p.Ranks || trophy {
		p.drawRankToken(cv, p.X, y, rank, gutter, trophy, c, scale)
		indent = gutter
	}
	p.drawScoreStrip(cv, p.X+indent, y, row, p.W-indent, c, scale)
}

// drawRankToken fills the left gutter for one row with the trophy or, under
// Ranks, the rank numeral right-aligned against the gap.
func (p AttractPanel) drawRankToken(cv Canvas, x, y, rank, gutter int, trophy bool, c Color, scale int) {
	if trophy {
		tc := p.TrophyColor
		if tc.A == 0 {
			tc = Color{R: 255, G: 200, B: 0, A: 255}
		}
		drawTrophy(cv, x, y, scale, tc)
		return
	}
	if !p.Ranks {
		return
	}
	rc := p.RankColor
	if rc.A == 0 {
		rc = dimColor(c)
	}
	tokenW := gutter - attractTokenGap*scale
	drawLEDUintClipped(cv, x+tokenW-ledUintWidth(rank, scale), y, rank, scale, rc,
		p.X, p.X+p.W)
}

// drawScoreStrip draws "<name>  <score>" into avail pixels from x, clipped to
// that span: static when it fits, name truncated when it overflows with no
// clock, scrolled on the shared cycle when it overflows with one.
//
// The clip starts at the strip's own left edge so a scrolling name cannot
// crawl over the trophy or rank numeral beside it.
func (p AttractPanel) drawScoreStrip(cv Canvas, x, y int, row AttractScore, avail int, c Color, scale int) {
	if avail <= 0 {
		return
	}
	font := FontLED{}
	clipL, clipR := x, x+avail

	nameW := font.TextWidth(row.Name, scale)
	gapW := 0
	if nameW > 0 {
		gapW = font.TextWidth(attractRowGutter, scale)
	}
	scoreW := ledUintWidth(row.Score, scale)

	switch over := nameW + gapW + scoreW - avail; {
	case over <= 0:
		drawTextClipped(cv, x, y, row.Name, scale, c, clipL, clipR)
		drawLEDUintClipped(cv, x+nameW+gapW, y, row.Score, scale, c, clipL, clipR)

	case p.T == 0:
		if n := fitPrefix(font, row.Name, scale, avail-gapW-scoreW); n > 0 {
			drawTextClipped(cv, x, y, row.Name[:n], scale, c, clipL, clipR)
			x += font.TextWidth(row.Name[:n], scale) + gapW
		}
		drawLEDUintClipped(cv, x, y, row.Score, scale, c, clipL, clipR)

	default:
		x -= attractScrollOffset(p.T, over)
		drawTextClipped(cv, x, y, row.Name, scale, c, clipL, clipR)
		drawLEDUintClipped(cv, x+nameW+gapW, y, row.Score, scale, c, clipL, clipR)
	}
}

// dimColor is five eighths of c, alpha untouched.
func dimColor(c Color) Color {
	d := func(v byte) byte { return byte(uint32(v) * 5 / 8) }
	return Color{R: d(c.R), G: d(c.G), B: d(c.B), A: c.A}
}

// fitPrefix is the byte length of the longest whole-rune prefix of s that
// fits in budget pixels. Runes with no glyph cost nothing.
func fitPrefix(font FontLED, s string, scale, budget int) int {
	if budget <= 0 {
		return 0
	}
	w := 0
	for i, r := range s {
		w += font.glyphWidth(r) * scale
		if w > budget {
			return i
		}
	}
	return len(s)
}

// ledUintWidth is drawLEDUintClipped's rendered width for v.
func ledUintWidth(v, scale int) int {
	if scale <= 0 {
		scale = 1
	}
	if v < 0 {
		v = 0
	}
	n := 1
	for t := v; t >= 10; t /= 10 {
		n++
	}
	return n * FontLED{}.glyphWidth('0') * scale
}

// blitLEDGlyph blits one LEDGlyphs bitmap with its top-left at (x, y),
// skipping pixel columns outside [clipL, clipR). Clipping by column avoids
// wrapping cv in a clipping Canvas, which would allocate.
func blitLEDGlyph(cv Canvas, bits []byte, x, y, scale int, c Color, clipL, clipR int) {
	width := len(bits) / ledGlyphRows
	for row := 0; row < ledGlyphRows; row++ {
		for col := 0; col < width; col++ {
			if bits[width*row+col] == 0 {
				continue
			}
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					px := x + col*scale + sx
					if px < clipL || px >= clipR {
						continue
					}
					cv.Blend(px, y+row*scale+sy, c)
				}
			}
		}
	}
}

// drawTextClipped is FontLED.DrawText restricted to the pixel columns
// [clipL, clipR).
func drawTextClipped(cv Canvas, x, y int, s string, scale int, c Color, clipL, clipR int) {
	if scale <= 0 {
		scale = 1
	}
	pen := x
	for _, r := range s {
		bits, ok := LEDGlyphs[r]
		if !ok {
			continue
		}
		w := len(bits) / ledGlyphRows * scale
		if pen+w > clipL && pen < clipR {
			blitLEDGlyph(cv, bits, pen, y, scale, c, clipL, clipR)
		}
		pen += w
	}
}

// drawLEDUintClipped draws v in decimal with its top-left at (x, y) in the LED
// font, restricted to [clipL, clipR). Negative v is clamped to 0.
func drawLEDUintClipped(cv Canvas, x, y, v, scale int, c Color, clipL, clipR int) {
	if scale <= 0 {
		scale = 1
	}
	if v < 0 {
		v = 0
	}
	digitW := FontLED{}.glyphWidth('0') * scale

	n := 1
	for t := v; t >= 10; t /= 10 {
		n++
	}
	for i := n - 1; i >= 0; i-- {
		bits := LEDGlyphs[rune('0'+v%10)]
		v /= 10
		dx := x + i*digitW
		if dx+digitW > clipL && dx < clipR {
			blitLEDGlyph(cv, bits, dx, y, scale, c, clipL, clipR)
		}
	}
}
