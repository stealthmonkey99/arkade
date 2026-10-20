// SPDX-License-Identifier: GPL-2.0-or-later

package shell

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/stealthmonkey99/arkade/internal/cartridge/rcart"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/scores"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// detailState is the full-screen metadata card for the focused game; anchor restarts animations on each open.
type detailState struct {
	open   bool
	m      engine.Manifest
	accent display.Pixel
	best   string // "HI 4242 ANNA" line; "" when no score is recorded
	anchor float64
	alpha  float64 // eased layer opacity 0..1

	// rows is the body layout, fixed while the card is open. scroll is the line
	// position and scrollPx the eased pixel offset; maxScroll and maxScrollPx are their clamps.
	rows        []detailRow
	scroll      int
	scrollPx    float64
	maxScroll   int
	maxScrollPx int

	// webSlug is the arkade.build slug for games with a web play page, "" otherwise. qr is built lazily.
	webSlug string
	qrOpen  bool
	qr      display.Sprite
}

// webPlayURL is the QR payload; the https:// scheme is required for phone scanners to treat it as a link.
func webPlayURL(slug string) string { return "https://arkade.build/" + slug + "/play" }

// webSlugFor maps a manifest to its web slug, or "" when the game has no
// public play page.
func webSlugFor(m engine.Manifest) string {
	if m.Visibility != rcart.VisibilityPublic && m.Visibility != rcart.VisibilityPlayOnly {
		return ""
	}
	return strings.TrimPrefix(m.Slug, "cart/")
}

// buildQR renders the deeplink as one pixel per module with a 2-module quiet zone, as the wifi QR does.
func buildQR(slug string) (display.Sprite, error) {
	border := 2
	q, err := element.NewQRCode(webPlayURL(slug), element.QRCodeOptions{Border: &border})
	if err != nil {
		return nil, err
	}
	symbol := q.Render(clock.FrameTime{})
	if len(symbol) > screenH-6 {
		return nil, fmt.Errorf("qr for %q is %d px, too large for the modal", slug, len(symbol))
	}
	return symbol, nil
}

// detailBest formats the card's score line.
func detailBest(best scores.Entry, ok bool) string {
	if !ok {
		return ""
	}
	if best.Name != "" {
		return fmt.Sprintf("HI %d %s", best.Score, strings.ToUpper(best.Name))
	}
	return fmt.Sprintf("HI %d", best.Score)
}

// detailEl paints the card; title and hints are fixed and only the body scrolls.
type detailEl struct {
	element.Base
	st *detailState
}

func newDetailEl(st *detailState) *detailEl {
	d := &detailEl{st: st}
	d.InitBase(element.Options{})
	d.Impl = d.renderImpl
	return d
}

// Card geometry: the title block takes 11 rows and the hint strip 10, leaving 42 for the body.
const (
	detailTitleY = 1
	detailRuleY  = 10

	detailBodyY = 12
	detailBodyH = 42 // y12..y53
	detailPad   = 2
	detailBodyW = screenW - 2*detailPad

	detailSepY  = 54
	detailHintY = 56

	// detailScrollStep is one text line of travel per stick step.
	detailScrollStep = 8

	// detailFeather is the number of scrim rows on an edge with content past it.
	detailFeather = 3
)

func (d *detailEl) renderImpl(stats clock.FrameTime) display.Sprite {
	out := newCanvas(screenW, screenH)
	st := d.st
	if !st.open {
		return out
	}
	fillRect(out, 0, 0, screenW, screenH, display.Pixel{R: 8, G: 9, B: 12, A: 255})
	if st.m.Logo != nil {
		// Dimmed logo as background, with a scrim under the text.
		drawFaded(out, st.m.Logo, 0, 0, 0.30)
		fillRect(out, 0, detailRuleY, screenW, screenH-detailRuleY, display.Pixel{A: 150})
	}
	ms := stats.DurationMs - st.anchor

	drawTileIcon(out, entry{kind: entryApp, m: st.m}, st.accent, 2, detailTitleY)
	ui.StrokeRect(&display.SpriteCanvas{Sprite: out}, 1, detailTitleY-1, 10, 10, display.ToUI(st.accent.WithAlpha(140)))
	title := display.TextSprite(strings.ToUpper(st.m.Name), colLabelHot)
	drawMarquee(out, title, labelX, detailTitleY, screenW-labelX-2, ms)

	fillRect(out, 0, detailRuleY, screenW, 1, st.accent.WithAlpha(150))

	d.drawBody(out, ms)

	fillRect(out, 0, detailSepY, screenW, 1, colSeparator)

	// C's web play hint takes the slot of the universal BACK hint when the game has a web page.
	if st.webSlug != "" {
		drawDotHints(out, detailHintY,
			ui.Hint{Text: "PLAY", Color: ui.ButtonGreen},
			ui.Hint{Text: "ONLINE", Color: ui.ButtonYellow})
	} else {
		drawDotHints(out, detailHintY,
			ui.Hint{Text: "PLAY", Color: ui.ButtonGreen},
			ui.Hint{Text: "BACK", Color: ui.ButtonRed})
	}

	if st.qrOpen && st.qr != nil {
		drawQRModal(out, st.qr, st.accent)
	}
	return out
}

// rowKind is how a body row paints itself. Heights are fixed so the scroll extent needs no rendering.
type rowKind int

const (
	rowMeta rowKind = iota // the fading author/publisher/version cycle
	rowTags                // the pause-scrolling tag strip
	rowRule                // a hairline section divider
	rowLine                // a static line, optionally with a right column
	rowPara                // one wrapped line of the description
)

// detailRow is one line of the card's scrollable document.
type detailRow struct {
	kind  rowKind
	text  string
	right string // right-aligned column (a leaderboard score)
	color display.Pixel
}

func (r detailRow) height() int {
	switch r.kind {
	case rowRule:
		return 3
	case rowPara:
		return 8
	default:
		return 7
	}
}

// detailBody lays out the card top to bottom: metadata cycle, tags, players,
// wrapped description, leaderboard. Empty sections are omitted.
func detailBody(m engine.Manifest, best string, table scores.Table) []detailRow {
	var rows []detailRow
	if len(metaItems(m, best)) > 0 {
		rows = append(rows, detailRow{kind: rowMeta})
	}
	if len(m.Tags) > 0 {
		rows = append(rows, detailRow{kind: rowTags})
	}
	if line := playersLine(m.Players); line != "" {
		rows = append(rows, detailRow{kind: rowLine, text: line, color: colFooter})
	}
	if desc := strings.TrimSpace(m.Description); desc != "" {
		rows = append(rows, detailRow{kind: rowRule})
		for _, line := range wrapText(desc, detailBodyW) {
			rows = append(rows, detailRow{kind: rowPara, text: line, color: colDim})
		}
	}
	if len(table.Entries) > 0 {
		rows = append(rows,
			detailRow{kind: rowRule},
			detailRow{kind: rowLine, text: "HIGH SCORES", color: colGold})
		for i, e := range table.Entries {
			name := strings.ToUpper(e.Name)
			if name == "" {
				name = "---"
			}
			color := colLabel
			if i == 0 {
				color = colDim // the record holder, one notch brighter
			}
			rows = append(rows, detailRow{
				kind:  rowLine,
				text:  fmt.Sprintf("%d %s", i+1, name),
				right: strconv.Itoa(e.Score),
				color: color,
			})
		}
	}
	return rows
}

func detailBodyHeight(rows []detailRow) int {
	h := 0
	for _, r := range rows {
		h += r.height()
	}
	return h
}

// detailScrollSpan is how far the body can travel, 0 when it all fits.
func detailScrollSpan(rows []detailRow) int {
	return max(0, detailBodyHeight(rows)-detailBodyH)
}

// setContent lays out the body for a newly opened card and resets the scroll.
func (st *detailState) setContent(table scores.Table) {
	st.rows = detailBody(st.m, st.best, table)
	st.maxScrollPx = detailScrollSpan(st.rows)
	st.maxScroll = (st.maxScrollPx + detailScrollStep - 1) / detailScrollStep
	st.scroll, st.scrollPx = 0, 0
}

// scrollBy steps the document a line, clamped at both ends, and reports whether it moved. It does nothing under the QR modal.
func (st *detailState) scrollBy(delta int) bool {
	if !st.open || st.qrOpen {
		return false
	}
	next := min(max(st.scroll+delta, 0), st.maxScroll)
	if next == st.scroll {
		return false
	}
	st.scroll = next
	return true
}

// scrollTarget is the pixel offset the view eases toward, clamped to the document end.
func (st *detailState) scrollTarget() float64 {
	return float64(min(st.scroll*detailScrollStep, st.maxScrollPx))
}

func (st *detailState) animate(stats clock.FrameTime) {
	target := st.scrollTarget()
	st.scrollPx += (target - st.scrollPx) * math.Min(1, 14*stats.ElapsedMs/1000)
	if math.Abs(target-st.scrollPx) < 0.3 {
		st.scrollPx = target
	}
}

// drawBody paints the document into a viewport canvas, then a scrollbar and feathered edges where content continues.
func (d *detailEl) drawBody(out display.Sprite, ms float64) {
	st := d.st
	body := newCanvas(screenW, detailBodyH)
	off := int(math.Round(st.scrollPx))
	y := -off
	for _, r := range st.rows {
		h := r.height()
		if y+h > 0 && y < detailBodyH {
			d.drawRow(body, r, y, ms)
		}
		y += h
	}
	blit(out, body, 0, detailBodyY)

	if st.maxScrollPx <= 0 {
		return
	}
	total := detailBodyHeight(st.rows)
	fillRect(out, screenW-1, detailBodyY, 1, detailBodyH, colSeparator)
	thumb := max(4, detailBodyH*detailBodyH/total)
	ty := detailBodyY + int(math.Round(float64(off)/float64(st.maxScrollPx)*float64(detailBodyH-thumb)))
	fillRect(out, screenW-1, ty, 1, thumb, st.accent)

	// The bottom fade is one row deeper than the top; both stop short of the scrollbar column.
	for i := 0; i < detailFeather; i++ {
		scrim := display.Pixel{A: uint8(70 + 60*i)}
		if off < st.maxScrollPx {
			fillRect(out, 0, detailBodyY+detailBodyH-detailFeather+i, screenW-1, 1, scrim)
		}
		if off > 0 && i < detailFeather-1 {
			fillRect(out, 0, detailBodyY+detailFeather-2-i, screenW-1, 1, scrim)
		}
	}
}

func (d *detailEl) drawRow(body display.Sprite, r detailRow, y int, ms float64) {
	switch r.kind {
	case rowMeta:
		d.drawMetaCycle(body, y, ms)
	case rowTags:
		drawTagLoop(body, d.st.m.Tags, d.st.accent, detailPad, y, detailBodyW, ms)
	case rowRule:
		fillRect(body, detailPad, y+1, detailBodyW, 1, d.st.accent.WithAlpha(70))
	case rowPara:
		blit(body, display.TextSprite(r.text, r.color), detailPad, y)
	default:
		maxX := screenW - detailPad
		if r.right != "" {
			right := display.TextSprite(r.right, r.color)
			maxX -= spriteW(right)
			blit(body, right, maxX, y)
			maxX -= 2 // gap that long names clip against
		}
		blitWindow(body, display.TextSprite(r.text, r.color), detailPad, y, detailPad, maxX)
	}
}

// The tags loop rests on a tag, slides left to the next, and wraps through a gap to the first.
const (
	tagHoldMs = 1100
	// tagMsPerPx is one speed for every slide, including the wrap.
	tagMsPerPx  = 12
	tagMinGapPx = 10
)

// tagStrip composes the tags into one sprite and returns each tag's start x.
func tagStrip(tags []string, accent display.Pixel) (display.Sprite, []int) {
	sep := display.TextSprite(" · ", colChevron)
	anchors := make([]int, 0, len(tags))
	w := 0
	for i, t := range tags {
		anchors = append(anchors, w)
		w += ui.FontLED{}.TextWidth(strings.ToUpper(t), 1)
		if i < len(tags)-1 {
			w += spriteW(sep)
		}
	}
	if w <= 0 {
		return nil, nil
	}
	out := newCanvas(w, 8)
	x := 0
	for i, t := range tags {
		s := display.TextSprite(strings.ToUpper(t), accent)
		blit(out, s, x, 0)
		x += spriteW(s)
		if i < len(tags)-1 {
			blit(out, sep, x, 0)
			x += spriteW(sep)
		}
	}
	return out, anchors
}

// tagStops is the loop's rest positions plus the wrapped first tag's, which is also the repeat span.
// The wrap gap fills the rest of the window so the wrapped tag rests whole at its right edge.
func tagStops(anchors []int, rowW, window int) []int {
	if len(anchors) == 0 {
		return nil
	}
	gap := max(tagMinGapPx, window-(rowW-anchors[len(anchors)-1]))
	return append(append([]int{}, anchors...), rowW+gap)
}

// tagCycleMs is one full loop, holds and slides included.
func tagCycleMs(stops []int) float64 {
	total := 0.0
	for i := 0; i+1 < len(stops); i++ {
		total += tagHoldMs + float64(stops[i+1]-stops[i])*tagMsPerPx
	}
	return total
}

// tagOffset is how far the row has travelled left at ms.
func tagOffset(stops []int, ms float64) int {
	if len(stops) < 2 {
		return 0
	}
	t := math.Mod(math.Max(ms, 0), tagCycleMs(stops))
	for i := 0; i+1 < len(stops); i++ {
		if t < tagHoldMs {
			return stops[i]
		}
		t -= tagHoldMs
		travel := float64(stops[i+1]-stops[i]) * tagMsPerPx
		if t < travel {
			return stops[i] + int(t/tagMsPerPx)
		}
		t -= travel
	}
	return stops[0]
}

// drawTagLoop renders the tags row, static when it fits the window.
func drawTagLoop(dst display.Sprite, tags []string, accent display.Pixel, x, y, window int, ms float64) {
	strip, anchors := tagStrip(tags, accent)
	rowW := spriteW(strip)
	if rowW <= window {
		blitWindow(dst, strip, x, y, x, x+window)
		return
	}
	stops := tagStops(anchors, rowW, window)
	off := tagOffset(stops, ms)
	span := stops[len(stops)-1]
	blitWindow(dst, strip, x-off, y, x, x+window)
	blitWindow(dst, strip, x-off+span, y, x, x+window) // the wrapped copy
}

// wrapText greedily word-wraps s to width px, chopping words that are too long.
func wrapText(s string, width int) []string {
	if width <= 0 {
		return nil
	}
	var lines []string
	cur := ""
	for _, field := range strings.Fields(s) {
		for _, word := range breakWord(field, width) {
			switch {
			case cur == "":
				cur = word
			case (ui.FontLED{}).TextWidth(cur+" "+word, 1) <= width:
				cur += " " + word
			default:
				lines = append(lines, cur)
				cur = word
			}
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// breakWord chops a word wider than width into width-sized pieces.
func breakWord(word string, width int) []string {
	if (ui.FontLED{}).TextWidth(word, 1) <= width {
		return []string{word}
	}
	var out []string
	cur := ""
	for _, r := range word {
		if cur != "" && (ui.FontLED{}).TextWidth(cur+string(r), 1) > width {
			out = append(out, cur)
			cur = ""
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// Metadata cycle: best score, author, publisher, version, audience, each held then faded.
const (
	metaFadeMs = 250
	metaHoldMs = 1400
	metaSlotMs = metaFadeMs + metaHoldMs + metaFadeMs
)

// metaItems builds the cycle's lines, skipping empty fields.
func metaItems(m engine.Manifest, best string) []string {
	var items []string
	if best != "" {
		items = append(items, best)
	}
	if m.Author != "" {
		items = append(items, "BY "+strings.ToUpper(m.Author))
	}
	if m.Publisher != "" {
		items = append(items, strings.ToUpper(m.Publisher))
	}
	if m.Version != "" {
		items = append(items, "V"+m.Version)
	}
	if m.Audience != "" {
		items = append(items, strings.ToUpper(m.Audience))
	}
	return items
}

func (d *detailEl) drawMetaCycle(out display.Sprite, y int, ms float64) {
	items := metaItems(d.st.m, d.st.best)
	if len(items) == 0 {
		return
	}
	idx := int(ms/metaSlotMs) % len(items)
	phase := ms - float64(int(ms/metaSlotMs))*metaSlotMs
	alpha := 1.0
	switch {
	case phase < metaFadeMs:
		alpha = phase / metaFadeMs
	case phase > metaFadeMs+metaHoldMs:
		alpha = (metaSlotMs - phase) / metaFadeMs
	}
	color := colLabel
	if strings.HasPrefix(items[idx], "HI ") {
		color = colGold
	}
	drawFaded(out, display.TextSprite(items[idx], color), detailPad, y, alpha)
}

func drawFaded(dst, src display.Sprite, x, y int, alpha float64) {
	for sy, row := range src {
		dy := y + sy
		if dy < 0 || dy >= len(dst) {
			continue
		}
		for sx, p := range row {
			dx := x + sx
			if dx < 0 || dx >= len(dst[dy]) || p.A == 0 {
				continue
			}
			dst[dy][dx] = display.Merge(dst[dy][dx], p.ScaleAlpha(alpha))
		}
	}
}
