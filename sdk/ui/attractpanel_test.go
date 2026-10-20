// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import (
	"fmt"
	"testing"
)

// assertCanvasEqual fails the test with the first mismatching pixel between two
// same-sized byteCanvases.
func assertCanvasEqual(t *testing.T, got, want *byteCanvas) {
	t.Helper()
	if got.w != want.w || got.h != want.h {
		t.Fatalf("size mismatch: got %dx%d, want %dx%d", got.w, got.h, want.w, want.h)
	}
	for y := 0; y < got.h; y++ {
		for x := 0; x < got.w; x++ {
			if got.at(x, y) != want.at(x, y) {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got.at(x, y), want.at(x, y))
			}
		}
	}
}

// TestAttractPanelLayout checks a title, the #1 row and two board rows
// pixel-for-pixel against plain FontLED.DrawText calls at attractRowH spacing.
// fmt builds the expected strings here to stay independent of the panel's own
// no-allocation digit drawing. The #1 row uses the same "<name>  <score>"
// expression as the board rows, which is the property under test.
func TestAttractPanelLayout(t *testing.T) {
	const w, h = 64, 64
	accent := Color{0, 255, 0, 255}
	panel := AttractPanel{
		X: 0, Y: 0, W: w, H: h,
		Title:       "GAME",
		Top:         AttractScore{Name: "MARK", Score: 42},
		Board:       []AttractScore{{Name: "BOB", Score: 10}, {Name: "AL", Score: 5}},
		Color:       White,
		AccentColor: accent,
	}

	got := newByteCanvas(w, h)
	panel.Draw(got)

	want := newByteCanvas(w, h)
	font := FontLED{}
	font.DrawText(want, 0, 0, "GAME", 1, accent)
	font.DrawText(want, 0, attractRowH, fmt.Sprintf("MARK  %d", 42), 1, accent)
	font.DrawText(want, 0, 2*attractRowH, fmt.Sprintf("BOB  %d", 10), 1, White)
	font.DrawText(want, 0, 3*attractRowH, fmt.Sprintf("AL  %d", 5), 1, White)

	assertCanvasEqual(t, got, want)
}

// TestAttractPanelTopRowMatchesBoardRow pins that the same entry as #1 and as
// the first board row renders identically apart from color and the trophy
// gutter.
func TestAttractPanelTopRowMatchesBoardRow(t *testing.T) {
	const w, h = 64, 64
	entry := AttractScore{Name: "MARK", Score: 4242}

	asTop := newByteCanvas(w, h)
	AttractPanel{X: 0, Y: 0, W: w, H: h, Top: entry, Color: White}.Draw(asTop)

	asRow := newByteCanvas(w, h)
	AttractPanel{X: 0, Y: 0, W: w, H: h, Board: []AttractScore{entry}, Color: White}.Draw(asRow)

	assertCanvasEqual(t, asTop, asRow)
}

// TestAttractPanelTrophyIndentsOnlyTopRow pins that the trophy pushes the #1
// row right by attractTrophyGutter and leaves board rows in place.
func TestAttractPanelTrophyIndentsOnlyTopRow(t *testing.T) {
	const w, h = 64, 64
	panel := AttractPanel{
		X: 0, Y: 0, W: w, H: h,
		Top:         AttractScore{Name: "MARK", Score: 42},
		Board:       []AttractScore{{Name: "BOB", Score: 10}},
		Color:       White,
		AccentColor: White,
		Trophy:      true,
		TrophyColor: White,
	}

	got := newByteCanvas(w, h)
	panel.Draw(got)

	want := newByteCanvas(w, h)
	font := FontLED{}
	drawTrophy(want, 0, 0, 1, White)
	font.DrawText(want, attractTrophyGutter, 0, fmt.Sprintf("MARK  %d", 42), 1, White)
	font.DrawText(want, 0, attractRowH, fmt.Sprintf("BOB  %d", 10), 1, White)

	assertCanvasEqual(t, got, want)
}

// TestAttractPanelTruncatesNameToFit checks that an over-long name is cut to
// whole glyphs with the score complete and inside W. "MMMMMMM" is the widest
// name a 7-slot NameEntry produces.
func TestAttractPanelTruncatesNameToFit(t *testing.T) {
	const w, h = 64, 64
	// Trophy gutter 10 + name gutter 4 + 7 digits x 4 = 42, leaving 14px of
	// name in a 56px panel: two 'M's (12px), not three (18px).
	const panelW = 56
	panel := AttractPanel{
		X: 0, Y: 0, W: panelW, H: h,
		Top:         AttractScore{Name: "MMMMMMM", Score: 1234567},
		Color:       White,
		AccentColor: White,
		Trophy:      true,
		TrophyColor: White,
	}

	got := newByteCanvas(w, h)
	panel.Draw(got)

	want := newByteCanvas(w, h)
	font := FontLED{}
	drawTrophy(want, 0, 0, 1, White)
	font.DrawText(want, attractTrophyGutter, 0, fmt.Sprintf("MM  %d", 1234567), 1, White)

	assertCanvasEqual(t, got, want)

	// Nothing is drawn at or past the panel's right edge.
	bg := Color{R: 0, G: 0, B: 0, A: 0xFF}
	for y := 0; y < h; y++ {
		for x := panelW; x < w; x++ {
			if got.at(x, y) != bg {
				t.Fatalf("pixel (%d,%d) is outside the %dpx panel but was drawn", x, y, panelW)
			}
		}
	}
}

func TestAttractPanelAnonymousTopRow(t *testing.T) {
	const w, h = 64, 64
	panel := AttractPanel{
		X: 0, Y: 0, W: w, H: h,
		Top:         AttractScore{Score: 42},
		Color:       White,
		AccentColor: White,
		Trophy:      true,
		TrophyColor: White,
	}

	got := newByteCanvas(w, h)
	panel.Draw(got)

	want := newByteCanvas(w, h)
	drawTrophy(want, 0, 0, 1, White)
	FontLED{}.DrawText(want, attractTrophyGutter, 0, "42", 1, White)

	assertCanvasEqual(t, got, want)
}

// TestAttractPanelDefaultColors checks the zero-value fallbacks: Color is
// White and AccentColor is Color.
func TestAttractPanelDefaultColors(t *testing.T) {
	const w, h = 32, 16
	panel := AttractPanel{X: 0, Y: 0, W: w, H: h, Title: "HI"}

	got := newByteCanvas(w, h)
	panel.Draw(got)

	want := newByteCanvas(w, h)
	FontLED{}.DrawText(want, 0, 0, "HI", 1, White)

	assertCanvasEqual(t, got, want)
}

func TestAttractPanelEmptyTopAndBoard(t *testing.T) {
	const w, h = 64, 64
	panel := AttractPanel{X: 0, Y: 0, W: w, H: h, Title: "GAME", Color: White, Trophy: true}

	got := newByteCanvas(w, h)
	panel.Draw(got)

	want := newByteCanvas(w, h)
	FontLED{}.DrawText(want, 0, 0, "GAME", 1, White)

	assertCanvasEqual(t, got, want)
}

// TestAttractPanelClipsOverflowingBoard sizes H so the second board row
// falls past Y+H; it must be skipped while the rows that fit render normally.
func TestAttractPanelClipsOverflowingBoard(t *testing.T) {
	const w, h = 64, 64
	const panelH = 3*attractRowH + 2 // title + #1 row + one board row fit; a second does not
	panel := AttractPanel{
		X: 0, Y: 0, W: w, H: panelH,
		Title:       "GAME",
		Top:         AttractScore{Score: 42}, // anonymous: the score alone
		Board:       []AttractScore{{Name: "BOB", Score: 10}, {Name: "AL", Score: 5}},
		Color:       White,
		AccentColor: White,
	}

	got := newByteCanvas(w, h)
	panel.Draw(got)

	want := newByteCanvas(w, h)
	font := FontLED{}
	font.DrawText(want, 0, 0, "GAME", 1, White)
	font.DrawText(want, 0, attractRowH, "42", 1, White)
	font.DrawText(want, 0, 2*attractRowH, fmt.Sprintf("BOB  %d", 10), 1, White)

	assertCanvasEqual(t, got, want)
}

// The narrow-row constants describe the case the scroll cycle exists for: a
// score of 89472 in a 40px row leaves room for about three letters of a
// seven-letter name.
//
//	name "MARLOWE"  M6 A4 R4 L4 O4 W6 E4      = 32px
//	gutter "  "                               =  4px
//	score 89472     5 digits x 4px            = 20px
//	                                            ----
//	                                            56px
//
// 16px of overflow, which is what the strip slides by.
const (
	worstName    = "MARLOWE"
	worstScore   = 89472
	narrowStripW = 56
	narrowRowW   = 40
	narrowOver   = narrowStripW - narrowRowW
)

// renderScrollRow draws a single overflowing score row at clock T into a canvas
// one row tall and rowW wide.
func renderScrollRow(rowW int, at uint64) *byteCanvas {
	cv := newByteCanvas(rowW, attractRowH)
	AttractPanel{
		X: 0, Y: 0, W: rowW, H: attractRowH,
		Top:   AttractScore{Name: worstName, Score: worstScore},
		Color: White,
		T:     at,
	}.Draw(cv)
	return cv
}

// scrollShift is the horizontal offset, in pixels, at which got matches ref, a
// render of the same row given all the width it wants. It fails the test if no
// shift matches or more than one does.
func scrollShift(t *testing.T, got, ref *byteCanvas, rowW, maxShift int) int {
	t.Helper()
	found := -1
	for k := 0; k <= maxShift; k++ {
		match := true
		for y := 0; y < attractRowH && match; y++ {
			for x := 0; x < rowW; x++ {
				if got.at(x, y) != ref.at(x+k, y) {
					match = false
					break
				}
			}
		}
		if !match {
			continue
		}
		if found >= 0 {
			t.Fatalf("row window matches BOTH shift %d and shift %d — the comparison is ambiguous", found, k)
		}
		found = k
	}
	if found < 0 {
		t.Fatal("row window matches no shift of the full-width render — it is not a window onto that row at all")
	}
	return found
}

// TestAttractPanelScrollCycleStages walks one cycle of an overflowing row and
// pins what shows at each stage: the name at rest, an eased slide, the score at
// rest flush right, and a slide back. Each stage is checked by finding which
// window of the full-width render the narrow row is showing.
func TestAttractPanelScrollCycleStages(t *testing.T) {
	// The reference: the identical row with room to spare, drawn static.
	ref := newByteCanvas(narrowStripW+narrowRowW, attractRowH)
	AttractPanel{
		X: 0, Y: 0, W: narrowStripW + narrowRowW, H: attractRowH,
		Top:   AttractScore{Name: worstName, Score: worstScore},
		Color: White,
	}.Draw(ref)

	const (
		slideOut  = attractHoldMicros
		scoreHold = attractHoldMicros + attractSlideMicros
		slideBack = 2*attractHoldMicros + attractSlideMicros
	)

	cases := []struct {
		name string
		at   uint64
		want int // -1 == "strictly between the two rests"
	}{
		{"name hold, first frame", 1, 0},
		{"name hold, last frame", slideOut - 1, 0},
		{"sliding out, quarter", slideOut + attractSlideMicros/4, -1},
		{"sliding out, half", slideOut + attractSlideMicros/2, -1},
		{"sliding out, three quarters", slideOut + 3*attractSlideMicros/4, -1},
		{"score hold, first frame", scoreHold, narrowOver},
		{"score hold, last frame", slideBack - 1, narrowOver},
		{"sliding back, half", slideBack + attractSlideMicros/2, -1},
		{"back at the name, next cycle", attractCycleMicros, 0},
		{"same cycle position, hours later", 977*attractCycleMicros + 1, 0},
	}
	for _, tc := range cases {
		got := scrollShift(t, renderScrollRow(narrowRowW, tc.at), ref, narrowRowW, narrowOver)
		switch {
		case tc.want >= 0 && got != tc.want:
			t.Errorf("%s (T=%d): row is shifted %dpx, want %dpx", tc.name, tc.at, got, tc.want)
		case tc.want < 0 && (got <= 0 || got >= narrowOver):
			t.Errorf("%s (T=%d): row is shifted %dpx, want strictly between 0 and %d (mid-slide)",
				tc.name, tc.at, got, narrowOver)
		}
	}
}

// TestAttractPanelScrollShowsWholeNameThenWholeScore pins that over one cycle
// there is a frame showing the whole name and a frame showing the whole score.
func TestAttractPanelScrollShowsWholeNameThenWholeScore(t *testing.T) {
	font := FontLED{}
	nameW := font.TextWidth(worstName, 1)
	scoreW := ledUintWidth(worstScore, 1)

	// Whole name: a full-width render of the name alone, at x=0.
	nameOnly := newByteCanvas(narrowRowW, attractRowH)
	font.DrawText(nameOnly, 0, 0, worstName, 1, White)

	// Whole score: right-aligned in the row, where the cycle's second rest parks it.
	scoreOnly := newByteCanvas(narrowRowW, attractRowH)
	drawLEDUintClipped(scoreOnly, narrowRowW-scoreW, 0, worstScore, 1, White, 0, narrowRowW)

	sawName, sawScore := false, false
	for at := uint64(1); at <= attractCycleMicros; at += 10_000 {
		got := renderScrollRow(narrowRowW, at)
		if regionEqual(got, nameOnly, 0, nameW) {
			sawName = true
		}
		if regionEqual(got, scoreOnly, narrowRowW-scoreW, narrowRowW) {
			sawScore = true
		}
	}
	if !sawName {
		t.Errorf("no frame in the cycle shows %q whole at the row's left edge", worstName)
	}
	if !sawScore {
		t.Errorf("no frame in the cycle shows %d whole at the row's right edge", worstScore)
	}
}

// regionEqual compares the columns [x0, x1) of two same-height canvases.
func regionEqual(a, b *byteCanvas, x0, x1 int) bool {
	for y := 0; y < a.h; y++ {
		for x := x0; x < x1; x++ {
			if a.at(x, y) != b.at(x, y) {
				return false
			}
		}
	}
	return true
}

// TestAttractPanelFittingRowNeverAnimates pins that a row that fits is at rest
// at every clock value, sampled across two cycles including the slide windows
// and hold boundaries.
func TestAttractPanelFittingRowNeverAnimates(t *testing.T) {
	const w, h = 64, 40
	draw := func(at uint64) *byteCanvas {
		cv := newByteCanvas(w, h)
		AttractPanel{
			X: 0, Y: 0, W: w, H: h,
			Title:       "GAME",
			Top:         AttractScore{Name: "MAX", Score: 128700},
			Board:       []AttractScore{{Name: "BOB", Score: 96450}, {Name: "AL", Score: 5}},
			Color:       White,
			AccentColor: White,
			Trophy:      true,
			TrophyColor: White,
			T:           at,
		}.Draw(cv)
		return cv
	}
	rest := draw(0)
	for at := uint64(0); at < 2*attractCycleMicros; at += 17_000 {
		assertCanvasEqual(t, draw(at), rest)
	}
}

// TestAttractPanelRowsAnimateIndependently pins that measurement is per row:
// with an overflowing #1 row and a fitting #2 row, row 2 is identical between
// two clock values a slide apart while row 1 is not.
func TestAttractPanelRowsAnimateIndependently(t *testing.T) {
	const w = 40
	draw := func(at uint64) *byteCanvas {
		cv := newByteCanvas(w, 2*attractRowH)
		AttractPanel{
			X: 0, Y: 0, W: w, H: 2 * attractRowH,
			Top:   AttractScore{Name: worstName, Score: worstScore}, // overflows
			Board: []AttractScore{{Name: "AL", Score: 5}},           // fits easily
			Color: White,
			T:     at,
		}.Draw(cv)
		return cv
	}
	atName := draw(1)                                       // name hold
	atScore := draw(attractHoldMicros + attractSlideMicros) // score hold

	rowsEqual := func(a, b *byteCanvas, y0 int) bool {
		for y := y0; y < y0+attractRowH; y++ {
			for x := 0; x < w; x++ {
				if a.at(x, y) != b.at(x, y) {
					return false
				}
			}
		}
		return true
	}
	if rowsEqual(atName, atScore, 0) {
		t.Error("the overflowing #1 row is identical at both ends of the cycle — it never scrolled")
	}
	if !rowsEqual(atName, atScore, attractRowH) {
		t.Error("the fitting #2 row moved: a row that fits must be at rest at every T")
	}
}

func TestAttractPanelNoClockStillTruncates(t *testing.T) {
	const w = narrowRowW
	got := newByteCanvas(w, attractRowH)
	AttractPanel{
		X: 0, Y: 0, W: w, H: attractRowH,
		Top:   AttractScore{Name: worstName, Score: worstScore},
		Color: White,
	}.Draw(got)

	// 40px row - 4px name gutter - 20px score = 16px of name budget:
	// "MARL" is 6+4+4+4 = 18px, so only "MAR" (14px) fits.
	want := newByteCanvas(w, attractRowH)
	font := FontLED{}
	font.DrawText(want, 0, 0, "MAR", 1, White)
	font.DrawText(want, font.TextWidth("MAR", 1)+font.TextWidth("  ", 1), 0, "89472", 1, White)

	assertCanvasEqual(t, got, want)
}

func TestAttractPanelClipsAndScrollsTitle(t *testing.T) {
	const w, canvasW = 24, 64
	const title = "LEADERBOARD"

	got := newByteCanvas(canvasW, attractRowH)
	AttractPanel{X: 0, Y: 0, W: w, H: attractRowH, Title: title, Color: White}.Draw(got)

	bg := Color{R: 0, G: 0, B: 0, A: 0xFF}
	for y := 0; y < attractRowH; y++ {
		for x := w; x < canvasW; x++ {
			if got.at(x, y) != bg {
				t.Fatalf("title pixel at (%d,%d) is outside the %dpx panel", x, y, w)
			}
		}
	}

	// With a clock the far end of the title reaches the panel at the second rest.
	scrolled := newByteCanvas(canvasW, attractRowH)
	AttractPanel{X: 0, Y: 0, W: w, H: attractRowH, Title: title, Color: White,
		T: attractHoldMicros + attractSlideMicros}.Draw(scrolled)

	want := newByteCanvas(canvasW, attractRowH)
	drawTextClipped(want, w-FontLED{}.TextWidth(title, 1), 0, title, 1, White, 0, w)
	assertCanvasEqual(t, scrolled, want)
}

// TestAttractPanelRanksShareTheTrophysGutter pins that with Ranks there is one
// left gutter: the trophy is rank 1's token and ranks 2..N are numerals
// right-aligned against the text.
func TestAttractPanelRanksShareTheTrophysGutter(t *testing.T) {
	const w, h = 64, 64
	panel := AttractPanel{
		X: 0, Y: 0, W: w, H: h,
		Top:         AttractScore{Name: "AAA", Score: 300},
		Board:       []AttractScore{{Name: "BOB", Score: 20}, {Name: "CAT", Score: 1}},
		Color:       White,
		AccentColor: White,
		Trophy:      true,
		TrophyColor: White,
		Ranks:       true,
	}

	got := newByteCanvas(w, h)
	panel.Draw(got)

	// Gutter = max(trophy 8px, widest rank numeral "3" 4px) + 2px gap = 10.
	const gutter = 10
	want := newByteCanvas(w, h)
	font := FontLED{}
	drawTrophy(want, 0, 0, 1, White) // rank 1's token IS the cup
	font.DrawText(want, gutter, 0, fmt.Sprintf("AAA  %d", 300), 1, White)
	// Numerals right-aligned in the 8px token column (8-4 = x 4).
	font.DrawText(want, 4, attractRowH, "2", 1, dimColor(White))
	font.DrawText(want, gutter, attractRowH, fmt.Sprintf("BOB  %d", 20), 1, White)
	font.DrawText(want, 4, 2*attractRowH, "3", 1, dimColor(White))
	font.DrawText(want, gutter, 2*attractRowH, fmt.Sprintf("CAT  %d", 1), 1, White)

	assertCanvasEqual(t, got, want)
}

// TestAttractPanelRanksWithoutTrophyUseANarrowGutter checks that with no trophy
// the gutter is the widest numeral plus its gap (6px, not 10).
func TestAttractPanelRanksWithoutTrophyUseANarrowGutter(t *testing.T) {
	const w, h = 40, 64
	panel := AttractPanel{
		X: 0, Y: 0, W: w, H: h,
		Top:   AttractScore{Name: "AAA", Score: 300},
		Board: []AttractScore{{Name: "BOB", Score: 20}},
		Color: White,
		Ranks: true,
	}

	got := newByteCanvas(w, h)
	panel.Draw(got)

	const gutter = 6 // one 4px numeral + the 2px gap
	want := newByteCanvas(w, h)
	font := FontLED{}
	font.DrawText(want, 0, 0, "1", 1, dimColor(White))
	font.DrawText(want, gutter, 0, fmt.Sprintf("AAA  %d", 300), 1, White)
	font.DrawText(want, 0, attractRowH, "2", 1, dimColor(White))
	font.DrawText(want, gutter, attractRowH, fmt.Sprintf("BOB  %d", 20), 1, White)

	assertCanvasEqual(t, got, want)
}

// TestAttractPanelRankGutterFitsTheWidestNumeral checks that the gutter is
// sized from the whole stack: a board running to rank 10 indents rank 2 as
// far as rank 10.
func TestAttractPanelRankGutterFitsTheWidestNumeral(t *testing.T) {
	const w, h = 64, 64
	board := make([]AttractScore, 9) // ranks 2..10
	for i := range board {
		board[i] = AttractScore{Name: "AA", Score: 9 - i}
	}
	panel := AttractPanel{
		X: 0, Y: 0, W: w, H: h,
		Top: AttractScore{Name: "AA", Score: 10}, Board: board,
		Color: White, Ranks: true,
	}

	got := newByteCanvas(w, h)
	panel.Draw(got)

	// Widest numeral is "10" (8px), so the gutter is 8+2 = 10 and every numeral
	// is right-aligned against x=8.
	const tokenR, gutter = 8, 10
	want := newByteCanvas(w, h)
	font := FontLED{}
	dim := dimColor(White)
	for rank := 1; rank <= 10; rank++ {
		y := (rank - 1) * attractRowH
		if y+font.Height(1) > h {
			break
		}
		num := fmt.Sprintf("%d", rank)
		font.DrawText(want, tokenR-font.TextWidth(num, 1), y, num, 1, dim)
		score := 10 - (rank - 1)
		font.DrawText(want, gutter, y, fmt.Sprintf("AA  %d", score), 1, White)
	}

	assertCanvasEqual(t, got, want)
}

// TestAttractPanelFirstRankRenumbers covers a windowed board: FirstRank sets
// the token on a single drawn row.
func TestAttractPanelFirstRankRenumbers(t *testing.T) {
	const w, h = 64, attractRowH
	got := newByteCanvas(w, h)
	AttractPanel{
		X: 0, Y: 0, W: w, H: h,
		Board:     []AttractScore{{Name: "CAT", Score: 7}},
		Color:     White,
		Ranks:     true,
		FirstRank: 4,
	}.Draw(got)

	want := newByteCanvas(w, h)
	font := FontLED{}
	font.DrawText(want, 0, 0, "4", 1, dimColor(White))
	font.DrawText(want, 6, 0, fmt.Sprintf("CAT  %d", 7), 1, White)

	assertCanvasEqual(t, got, want)
}

// TestAttractPanelOneRowPanel covers the smallest panel: no title, no board, one
// clocked row.
func TestAttractPanelOneRowPanel(t *testing.T) {
	const w, h = 64, 64
	got := newByteCanvas(w, h)
	AttractPanel{
		X: 20, Y: 50, W: 40, H: attractRowH,
		Top:         AttractScore{Name: "MAX", Score: 42},
		Color:       White,
		AccentColor: White,
		Trophy:      true,
		TrophyColor: White,
		T:           attractHoldMicros, // a clock, but nothing to animate
	}.Draw(got)

	want := newByteCanvas(w, h)
	drawTrophy(want, 20, 50, 1, White)
	FontLED{}.DrawText(want, 20+attractTrophyGutter, 50, fmt.Sprintf("MAX  %d", 42), 1, White)

	assertCanvasEqual(t, got, want)
}
