// SPDX-License-Identifier: GPL-2.0-or-later

package shell

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/scores"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// cardGame is a manifest with every section and a description that wraps several times.
func cardGame() engine.Manifest {
	return engine.Manifest{
		Slug: "cart/snake", Name: "Snake", Type: engine.TypeGame,
		Description: "Eat the maze before the jellyfish sting you. Corner pellets turn the tables.",
		Tags:        []string{"arcade", "action", "classic"},
		Players:     []string{"1p", "2p-coop"},
		Author:      "Ada",
	}
}

func cardTable() scores.Table {
	t := scores.Table{}
	t.Insert(9000, "zoe")
	t.Insert(4242, "ada")
	return t
}

func kinds(rows []detailRow) []rowKind {
	out := make([]rowKind, len(rows))
	for i, r := range rows {
		out[i] = r.kind
	}
	return out
}

func TestDetailBodyOrder(t *testing.T) {
	rows := detailBody(cardGame(), "HI 9000 ZOE", cardTable())

	want := []rowKind{rowMeta, rowTags, rowLine, rowRule}
	if len(rows) < len(want) {
		t.Fatalf("body = %v, want it to start %v", kinds(rows), want)
	}
	for i, k := range want {
		if rows[i].kind != k {
			t.Fatalf("body = %v, want it to start %v", kinds(rows), want)
		}
	}
	if rows[2].text != "1P + 2P CO-OP" {
		t.Errorf("third row = %q, want the players line", rows[2].text)
	}

	paras := 0
	var joined []string
	for _, r := range rows {
		if r.kind == rowPara {
			paras++
			joined = append(joined, r.text)
		}
	}
	if paras < 3 {
		t.Errorf("description wrapped into %d lines, want a real paragraph (3+)", paras)
	}
	if got := strings.Join(joined, " "); got != cardGame().Description {
		t.Errorf("wrapped description reassembles to %q, want the manifest's text back", got)
	}

	head := -1
	for i, r := range rows {
		if r.text == "HIGH SCORES" {
			head = i
		}
	}
	if head < 0 {
		t.Fatalf("no leaderboard heading in %v", kinds(rows))
	}
	if rows[head-1].kind != rowRule {
		t.Errorf("leaderboard heading isn't preceded by a divider (%v)", kinds(rows))
	}
	if got := rows[head+1]; got.text != "1 ZOE" || got.right != "9000" {
		t.Errorf("first leaderboard row = %+v, want rank+name left, score right", got)
	}
	if got := rows[head+2]; got.text != "2 ADA" || got.right != "4242" {
		t.Errorf("second leaderboard row = %+v", got)
	}
}

func TestDetailBodyWithoutScores(t *testing.T) {
	rows := detailBody(cardGame(), "", scores.Table{})
	for _, r := range rows {
		if r.text == "HIGH SCORES" || r.right != "" {
			t.Fatalf("scoreless game grew a leaderboard: %+v", r)
		}
	}
	if last := rows[len(rows)-1]; last.kind != rowPara {
		t.Errorf("last row = %v, want the document to end on the description", last.kind)
	}
	// With nothing to cycle the metadata row is absent.
	bare := detailBody(engine.Manifest{Description: "Just a game."}, "", scores.Table{})
	for _, r := range bare {
		if r.kind == rowMeta {
			t.Errorf("bare manifest still reserved a metadata row: %v", kinds(bare))
		}
	}
}

func TestDetailScrollClampsAtBothEnds(t *testing.T) {
	st := detailState{open: true, m: cardGame(), best: "HI 9000 ZOE"}
	st.setContent(cardTable())

	if st.maxScrollPx <= 0 {
		t.Fatalf("a full card should overflow the %dpx viewport (height %d)", detailBodyH, detailBodyHeight(st.rows))
	}
	if st.scroll != 0 || st.scrollTarget() != 0 {
		t.Fatal("a freshly opened card must start at the top")
	}
	if st.scrollBy(-1) {
		t.Error("scrolled up off the top of the document")
	}

	for i := 0; i < 100; i++ {
		st.scrollBy(1)
	}
	if st.scroll != st.maxScroll {
		t.Errorf("scroll = %d after mashing down, want the clamp %d", st.scroll, st.maxScroll)
	}
	if st.scrollBy(1) {
		t.Error("scrolled past the end of the document")
	}
	if got := st.scrollTarget(); got != float64(st.maxScrollPx) {
		t.Errorf("bottom offset = %v, want exactly the %dpx span (the last line flush with the viewport)", got, st.maxScrollPx)
	}

	st.scrollBy(-1)
	if st.scroll != st.maxScroll-1 {
		t.Errorf("scroll = %d after one up, want %d", st.scroll, st.maxScroll-1)
	}

	short := detailState{open: true, m: engine.Manifest{Name: "Tiny", Description: "Short."}}
	short.setContent(scores.Table{})
	if short.maxScroll != 0 || short.scrollBy(1) {
		t.Errorf("a body that fits (%dpx) must not scroll", detailBodyHeight(short.rows))
	}

	st.qrOpen = true
	if st.scrollBy(-1) {
		t.Error("the card scrolled underneath the QR modal")
	}
}

func TestWrapText(t *testing.T) {
	width := ui.FontLED{}.TextWidth("HELLO WORLD", 1)
	got := wrapText("HELLO WORLD HELLO", width)
	if len(got) != 2 || got[0] != "HELLO WORLD" || got[1] != "HELLO" {
		t.Errorf("wrapText greedy fill = %q, want [HELLO WORLD, HELLO]", got)
	}
	for _, line := range wrapText(cardGame().Description, detailBodyW) {
		if w := (ui.FontLED{}).TextWidth(line, 1); w > detailBodyW {
			t.Errorf("wrapped line %q is %dpx, wider than the %dpx column", line, w, detailBodyW)
		}
	}
	long := strings.Repeat("W", 40)
	for _, line := range wrapText(long, detailBodyW) {
		if w := (ui.FontLED{}).TextWidth(line, 1); w > detailBodyW {
			t.Errorf("unbreakable word left a %dpx line %q", w, line)
		}
	}
	if got := wrapText("   ", detailBodyW); got != nil {
		t.Errorf("wrapText(blank) = %q, want no lines", got)
	}
}

func countPixels(frame display.Sprite, c display.Pixel, y0, y1 int) int {
	n := 0
	for y := y0; y < y1 && y < len(frame); y++ {
		for _, p := range frame[y] {
			if p.R == c.R && p.G == c.G && p.B == c.B && p.A == c.A {
				n++
			}
		}
	}
	return n
}

func renderCard(st *detailState, ms float64) display.Sprite {
	st.scrollPx = st.scrollTarget()
	return newDetailEl(st).renderImpl(clock.FrameTime{DurationMs: ms})
}

// buttonDotPixels is drawButtonDot's ink: the 3/5/7/7/7/5/3 disc.
const buttonDotPixels = 3 + 5 + 7 + 7 + 7 + 5 + 3

func TestDetailHintsAreButtonDiscs(t *testing.T) {
	st := detailState{open: true, m: cardGame(), accent: colButtonD, webSlug: "snake"}
	st.setContent(scores.Table{})
	frame := renderCard(&st, 0)

	if got := countPixels(frame, colButtonA, detailHintY, screenH); got != buttonDotPixels {
		t.Errorf("green A hint = %d px in the hint row, want the %d-px disc", got, buttonDotPixels)
	}
	if got := countPixels(frame, colButtonC, detailHintY, screenH); got != buttonDotPixels {
		t.Errorf("yellow C hint = %d px, want the %d-px disc", got, buttonDotPixels)
	}
	if got := countPixels(frame, colButtonA, 0, detailHintY); got != 0 {
		t.Errorf("%d green pixels above the hint row", got)
	}

	st.webSlug = ""
	frame = renderCard(&st, 0)
	if got := countPixels(frame, colButtonB, detailHintY, screenH); got != buttonDotPixels {
		t.Errorf("red B hint = %d px, want the %d-px disc", got, buttonDotPixels)
	}
	if got := countPixels(frame, colButtonC, 0, screenH); got != 0 {
		t.Error("ONLINE hint still drawn for a game with no web play page")
	}
}

// TestDetailScrollCues checks the scrollbar and the feathered edge: bottom at the top of the document, top at the bottom, neither when it fits.
func TestDetailScrollCues(t *testing.T) {
	st := detailState{open: true, m: cardGame(), accent: colButtonD, best: "HI 9000 ZOE"}
	st.setContent(cardTable())

	frame := renderCard(&st, 0)
	track, thumb := 0, 0
	for y := detailBodyY; y < detailBodyY+detailBodyH; y++ {
		switch p := frame[y][screenW-1]; {
		case p == colSeparator:
			track++
		case p == st.accent:
			thumb++
		}
	}
	if track+thumb != detailBodyH {
		t.Errorf("scrollbar column = %d track + %d thumb px, want all %d body rows", track, thumb, detailBodyH)
	}
	if thumb == 0 || thumb == detailBodyH {
		t.Errorf("thumb is %d of %d rows — it has to read as a position", thumb, detailBodyH)
	}

	// The left margin never holds text, so it shows the feather scrim.
	margin := func(f display.Sprite, y int) int {
		p := f[y][0]
		return int(p.R) + int(p.G) + int(p.B)
	}
	mid := margin(frame, detailBodyY+detailBodyH/2)
	if margin(frame, detailBodyY+detailBodyH-1) >= mid {
		t.Errorf("bottom edge not feathered with more content below (%d vs %d mid-body)",
			margin(frame, detailBodyY+detailBodyH-1), mid)
	}
	if margin(frame, detailBodyY) != mid {
		t.Error("top edge feathered while the card is already at the top")
	}

	for i := 0; i < 100; i++ {
		st.scrollBy(1)
	}
	end := renderCard(&st, 0)
	if margin(end, detailBodyY) >= margin(end, detailBodyY+detailBodyH/2) {
		t.Error("top edge not feathered at the end of the document")
	}
	if margin(end, detailBodyY+detailBodyH-1) != margin(end, detailBodyY+detailBodyH/2) {
		t.Error("bottom edge still feathered with nothing below it")
	}

	short := detailState{open: true, m: engine.Manifest{Name: "Tiny", Description: "Short."}, accent: colButtonD}
	short.setContent(scores.Table{})
	shortFrame := renderCard(&short, 0)
	for y := detailBodyY; y < detailBodyY+detailBodyH; y++ {
		if p := shortFrame[y][screenW-1]; p == colSeparator || p == short.accent {
			t.Fatalf("scrollbar drawn at y%d for a card that fits", y)
		}
	}
	if margin(shortFrame, detailBodyY+detailBodyH-1) != margin(shortFrame, detailBodyY) {
		t.Error("a card that fits should carry no edge fade")
	}
}

// TestTagLoopRestsOnlyOnWholeTags checks that at every stop the wrapped copy lies outside the window.
func TestTagLoopRestsOnlyOnWholeTags(t *testing.T) {
	strip, anchors := tagStrip([]string{"arcade", "action", "classic"}, colButtonD)
	rowW := spriteW(strip)
	if rowW <= detailBodyW {
		t.Fatalf("the fixture tags fit (%dpx in %dpx) — this test needs an overflowing row", rowW, detailBodyW)
	}
	stops := tagStops(anchors, rowW, detailBodyW)
	span := stops[len(stops)-1]

	for i, stop := range stops[:len(stops)-1] {
		if stop != anchors[i] {
			t.Errorf("stop %d at %dpx is not a tag boundary (%v)", i, stop, anchors)
		}
		// The wrapped copy starts span px along the strip.
		if span-stop < detailBodyW {
			t.Errorf("at stop %d (%dpx) the wrapped copy shows %dpx inside the window — a tag rests bisected",
				i, stop, detailBodyW-(span-stop))
		}
	}
}

// TestTagOffsetLoopsForward checks the offset never decreases except at the wrap.
func TestTagOffsetLoopsForward(t *testing.T) {
	strip, anchors := tagStrip([]string{"arcade", "action", "classic"}, colButtonD)
	stops := tagStops(anchors, spriteW(strip), detailBodyW)
	span := stops[len(stops)-1]
	cycle := tagCycleMs(stops)

	prev, wraps, held := tagOffset(stops, 0), 0, 0
	for ms := 0.0; ms <= 2*cycle; ms += 16 {
		off := tagOffset(stops, ms)
		switch {
		case off == prev:
			held++
		case off < prev:
			wraps++ // the one legal reversal: back to the top of the loop
			if prev < span-detailBodyW {
				t.Fatalf("offset fell from %d to %d at %vms, nowhere near the wrap — that's a bounce", prev, off, ms)
			}
		}
		if off < 0 || off >= span {
			t.Fatalf("offset %d at %vms is outside the row's span %d", off, ms, span)
		}
		prev = off
	}
	if wraps != 2 {
		t.Errorf("saw %d wraps over two cycles, want exactly 2", wraps)
	}
	if held == 0 {
		t.Error("the row never paused — the loop is supposed to rest on each tag")
	}
	seen := map[int]bool{}
	for ms := 0.0; ms < cycle; ms += 16 {
		seen[tagOffset(stops, ms)] = true
	}
	for i, stop := range stops[:len(stops)-1] {
		if !seen[stop] {
			t.Errorf("stop %d (%dpx) never came to rest", i, stop)
		}
	}
}

func TestTagLoopStaticWhenItFits(t *testing.T) {
	out := newCanvas(screenW, 8)
	drawTagLoop(out, []string{"sim"}, colButtonD, detailPad, 0, detailBodyW, 0)
	first := fmt.Sprint(out)
	out2 := newCanvas(screenW, 8)
	drawTagLoop(out2, []string{"sim"}, colButtonD, detailPad, 0, detailBodyW, 9999)
	if fmt.Sprint(out2) != first {
		t.Error("a tags row that fits the window moved anyway")
	}
}
