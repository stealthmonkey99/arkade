// SPDX-License-Identifier: GPL-2.0-or-later

package shell

import (
	"fmt"
	"math"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/config"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// Browser geometry (64×64 panel).
const (
	screenW, screenH = 64, 64

	headerTextY = 1
	underlineY  = 10

	listTop     = 13
	rowPitch    = 10
	visibleRows = 4
	listH       = visibleRows * rowPitch // y13..52
	labelX      = 14
	labelWindow = screenW - labelX - 3 // room for the scrollbar column

	// The inline wedge on a slider row, right-aligned so it never runs under the label.
	sliderWedgeX = screenW - 20
	sliderWedgeW = 18
	// sliderRightX is the right edge every inline tile indicator lines up on.
	sliderRightX = sliderWedgeX + sliderWedgeW

	// hintSlotMs is how long each footer corner hint shows.
	hintSlotMs = 2000

	footerSepY  = 54
	footerTextY = 56
)

var (
	colLabel     = display.Pixel{R: 132, G: 136, B: 144, A: 255}
	colLabelHot  = display.White
	colChevron   = display.Pixel{R: 96, G: 100, B: 108, A: 255}
	colSeparator = display.Pixel{R: 52, G: 54, B: 60, A: 255}
	colFooter    = display.Pixel{R: 168, G: 172, B: 180, A: 255}
	colGold      = display.Pixel{R: 252, G: 192, B: 32, A: 255}
	colDim       = display.Pixel{R: 200, G: 202, B: 208, A: 255}
	colMercury   = display.Pixel{R: 168, A: 255}
	// Physical button colors: green A, red B, yellow C, blue D.
	colButtonA = display.FromUI(ui.ButtonGreen)
	colButtonB = display.FromUI(ui.ButtonRed)
	colButtonC = display.FromUI(ui.ButtonYellow)
	colButtonD = display.FromUI(ui.ButtonBlue)
)

// browseState is the navigation model plus animation timers, all anchored to engine DurationMs.
type browseState struct {
	cats  []category
	cat   int
	focus int
	// showingResults swaps the active shelf for the search results.
	showingResults bool

	// favs mirrors settings.Favorites for the star badges.
	favs    map[string]bool
	favAnim favAnim

	// savedCat and savedFocus restore the browse position when search closes.
	query      string
	cursor     int
	results    []entry
	savedCat   int
	savedFocus int

	// Volume mirrors the settings app's storage.
	volume int
	muted  bool

	// Brightness and lamps mirror the runtime, which the --brightness flag and hotkeys change behind the shell.
	brightness int
	lamps      engine.LampPolicy

	// cpuTemp is the formatted reading, "--" without a thermal sensor.
	fanThreshold int
	fanMode      hardware.FanMode
	cpuTemp      string
	// padSeats[i] is true when a wireless pad is playing that seat.
	padSeats [2]bool
	// padsLoose counts wireless pads awake with no seat yet.
	padsLoose int
	cpuTempF  float64
	cpuTempOK bool

	// wifiQR is built on first engage.
	wifiQR display.Sprite

	// hiScore is the focused game's footer text ("HI 4242"), looked up on focus change.
	hiScore string

	// announce is a banner shown over the list until announceUntil.
	announce       string
	announceUntil  float64
	announceAccent display.Pixel

	scrollRow   float64 // eased top row of the list viewport
	slideX      float64 // eased horizontal offset after a category switch
	catAnchor   float64 // DurationMs of the last category switch
	focusAnchor float64 // DurationMs of the last focus move
	now         float64 // latest DurationMs, stamped each Update
}

// activeCat is the shelf shown: the search results in results mode, else the catalog.
func (s *browseState) activeCat() (category, bool) {
	if s.showingResults {
		return category{
			id:      "results",
			label:   "SEARCH",
			accent:  accentFor(catAll),
			entries: s.results,
		}, true
	}
	if len(s.cats) == 0 {
		return category{}, false
	}
	return s.cats[s.cat], true
}

func (s *browseState) focused() (entry, bool) {
	cur, ok := s.activeCat()
	if !ok || s.focus < 0 || s.focus >= len(cur.entries) {
		return entry{}, false
	}
	return cur.entries[s.focus], true
}

// focusedApp narrows focused() to a real app manifest.
func (s *browseState) focusedApp() (engine.Manifest, bool) {
	e, ok := s.focused()
	if !ok || e.kind != entryApp {
		return engine.Manifest{}, false
	}
	return e.m, true
}

// moveFocus steps the selection, clamped at the ends, and reports whether it moved.
func (s *browseState) moveFocus(delta int) bool {
	cur, ok := s.activeCat()
	if !ok {
		return false
	}
	next := min(max(s.focus+delta, 0), len(cur.entries)-1)
	if next == s.focus {
		return false
	}
	s.focus = next
	s.focusAnchor = s.now
	return true
}

// moveCat switches shelves with wraparound and arms the slide.
func (s *browseState) moveCat(delta int) bool {
	if len(s.cats) < 2 {
		return false
	}
	s.cat = ((s.cat+delta)%len(s.cats) + len(s.cats)) % len(s.cats)
	s.focus = 0
	s.scrollRow = 0
	s.slideX = float64(28 * delta)
	s.catAnchor = s.now
	s.focusAnchor = s.now
	return true
}

// jumpTo restores a category and a focused app slug or control label. Unknown
// ids keep the shelf; an unknown focus lands on the first app tile.
func (s *browseState) jumpTo(catID, slug, control string) {
	for i, c := range s.cats {
		if c.id == catID {
			s.cat = i
			break
		}
	}
	cur, ok := s.activeCat()
	if !ok {
		return
	}
	if control != "" {
		for i, e := range cur.entries {
			if e.kind != entryApp && e.label == control {
				s.focus = i
				s.scrollRow = math.Max(0, float64(i-visibleRows+1))
				return
			}
		}
	}
	for i, e := range cur.entries {
		if e.kind == entryApp && e.m.Slug == slug {
			s.focus = i
			s.scrollRow = math.Max(0, float64(i-visibleRows+1))
			return
		}
	}
	for i, e := range cur.entries {
		if e.kind == entryApp {
			s.focus = i
			return
		}
	}
}

func (s *browseState) animate(stats clock.FrameTime) {
	s.now = stats.DurationMs
	dt := stats.ElapsedMs / 1000

	target := s.scrollRow
	if f := float64(s.focus); f < target {
		target = f
	} else if f > target+visibleRows-1 {
		target = f - visibleRows + 1
	}
	s.scrollRow += (target - s.scrollRow) * math.Min(1, 14*dt)
	if math.Abs(target-s.scrollRow) < 0.03 {
		s.scrollRow = target
	}

	s.slideX -= s.slideX * math.Min(1, 12*dt)
	if math.Abs(s.slideX) < 0.5 {
		s.slideX = 0
	}
}

// browseEl paints the whole browser frame: whichever screen is on top of the stack.
type browseEl struct {
	element.Base
	a  *app
	st *browseState
}

func newBrowseEl(a *app) *browseEl {
	b := &browseEl{a: a, st: &a.st}
	b.InitBase(element.Options{})
	b.Impl = b.renderImpl
	return b
}

func (b *browseEl) renderImpl(stats clock.FrameTime) display.Sprite {
	out := newCanvas(screenW, screenH)
	b.a.screen().draw(b.a, out, stats.DurationMs)
	return out
}

func (b *browseEl) drawShelf(out display.Sprite, ms float64) {
	st := b.st
	cur, ok := st.activeCat()
	if !ok {
		empty := display.TextSprite("NO GAMES", colLabel)
		blit(out, empty, (screenW-spriteW(empty))/2, 28)
		return
	}
	slide := int(math.Round(st.slideX))

	b.drawHeader(out, cur, ms, slide)

	// A separate canvas crops scrolling rows at the viewport.
	list := newCanvas(screenW, listH)
	b.drawList(list, cur, ms, slide)
	blit(out, list, 0, listTop)
	b.drawScrollbar(out, cur)

	b.drawFooter(out, ms)

	if st.announce != "" && ms < st.announceUntil {
		drawBanner(out, st.announce, st.announceAccent)
	}
}

func (b *browseEl) drawHeader(out display.Sprite, cur category, ms float64, slide int) {
	if !b.st.showingResults && len(b.st.cats) > 1 {
		cv := &display.SpriteCanvas{Sprite: out}
		ui.Chevron{X: 1, Y: 2, Color: display.ToUI(colChevron)}.Draw(cv)
		ui.Chevron{X: 60, Y: 2, Color: display.ToUI(colChevron), Right: true}.Draw(cv)
	}
	label := cur.label
	if b.st.showingResults {
		label = "?" + b.st.query
	}
	title := display.TextSprite(label, cur.accent)
	blit(out, title, (screenW-spriteW(title))/2+slide/2, headerTextY)

	frac := math.Min(1, (ms-b.st.catAnchor)/220)
	w := int(float64(screenW) * frac)
	x := 0
	if b.st.slideX < 0 {
		x = screenW - w
	}
	fillRect(out, x, underlineY, w, 1, cur.accent.WithAlpha(150))
}

// drawList paints the rows into the viewport canvas, whose origin is its top-left.
func (b *browseEl) drawList(list display.Sprite, cur category, ms float64, slide int) {
	scrollPx := int(math.Round(b.st.scrollRow * rowPitch))
	for i, e := range cur.entries {
		rowY := 1 + i*rowPitch - scrollPx
		if rowY <= -rowPitch || rowY >= listH {
			continue
		}
		focused := i == b.st.focus
		if focused {
			fillRect(list, 0, rowY-1, screenW, rowPitch, cur.accent.WithAlpha(42))
			pulse := uint8(150 + 90*math.Sin(ms/260))
			ui.StrokeRect(&display.SpriteCanvas{Sprite: list}, slide+1, rowY-1, 10, 10, display.ToUI(cur.accent.WithAlpha(pulse)))
		}
		drawTileIcon(list, e, cur.accent, slide+2, rowY)
		b.drawFavStar(list, e, slide, rowY, ms)
		color := colLabel
		if focused {
			color = colLabelHot
		}
		label := display.TextSprite(e.name(), color)
		win := labelWindow
		switch e.kind {
		case entryVolume, entryBrightness:
			win = sliderWedgeX - 2 - labelX // stop the label short of the wedge
		case entryBluetooth:
			win = sliderRightX - padPipsWidth - 2 - labelX
		case entryTemp:
			val := display.TextSprite(b.st.cpuTemp, colFooter)
			vx := screenW - 3 - spriteW(val)
			blit(list, val, slide+vx, rowY)
			win = vx - 2 - labelX
		}
		if focused {
			drawMarquee(list, label, slide+labelX, rowY, win, ms-b.st.focusAnchor)
		} else {
			blitWindow(list, label, slide+labelX, rowY, 0, labelX+win)
		}
		switch e.kind {
		case entryVolume:
			drawWedge(list, sliderWedgeX, rowY+1, sliderWedgeW, 6, b.st.volume, b.st.muted, cur.accent)
		case entryBrightness:
			drawWedge(list, sliderWedgeX, rowY+1, sliderWedgeW, 6, b.st.brightness, false, cur.accent)
		case entryBluetooth:
			drawPadPips(list, slide+sliderRightX-padPipsWidth, rowY+1, b.st.padSeats, b.st.padsLoose)
		}
	}
}

func (b *browseEl) drawScrollbar(out display.Sprite, cur category) {
	n := len(cur.entries)
	if n <= visibleRows {
		return
	}
	fillRect(out, screenW-1, listTop, 1, listH, colSeparator)
	thumbH := max(3, listH*visibleRows/n)
	span := listH - thumbH
	maxRow := float64(n - visibleRows)
	y := listTop + int(math.Round(b.st.scrollRow/maxRow*float64(span)))
	fillRect(out, screenW-1, y, 1, thumbH, cur.accent)
}

// drawFooter draws the players badge and one corner hint. A game cycles PLAY,
// PIN and INFO, the sliders alternate the stick adjust with OPEN, and other tiles show their green verb.
func (b *browseEl) drawFooter(out display.Sprite, ms float64) {
	fillRect(out, 0, footerSepY, screenW, 1, colSeparator)
	e, focusedOK := b.st.focused()
	if focusedOK && e.kind == entryApp {
		if badge := playersBadge(e.m.Players); badge != "" {
			blit(out, display.TextSprite(badge, colFooter), 2, footerTextY)
		}
	}
	switch {
	case focusedOK && e.interactable():
		hints := [3]ui.Hint{
			{Text: "PLAY", Color: ui.ButtonGreen},
			{Text: "PIN", Color: ui.ButtonYellow},
			{Text: "INFO", Color: ui.ButtonBlue},
		}
		if b.st.favs[e.m.Slug] {
			hints[1].Text = "UNPIN"
		}
		drawCornerHint(out, hints[int(ms/hintSlotMs)%len(hints)])
	case focusedOK && e.kind != entryApp && e.control().slider != nil:
		if int(ms/hintSlotMs)%2 == 0 {
			drawCornerHint(out, ui.Hint{Text: e.control().hint, Control: ui.HintStick, Color: display.ToUI(colFooter)})
		} else {
			drawCornerHint(out, ui.Hint{Text: "OPEN", Color: ui.ButtonGreen})
		}
	case focusedOK && e.kind != entryApp && e.control().verb != "":
		drawCornerHint(out, ui.Hint{Text: e.control().verb, Color: ui.ButtonGreen})
	case focusedOK:
		drawCornerHint(out, ui.Hint{Text: "OPEN", Color: ui.ButtonGreen})
	case b.st.hiScore != "":
		s := display.TextSprite(b.st.hiScore, colGold)
		blit(out, s, screenW-2-spriteW(s), footerTextY)
	}
}

func drawCornerHint(out display.Sprite, h ui.Hint) {
	ui.DrawHint(&display.SpriteCanvas{Sprite: out}, screenW-2-ui.HintWidth(h), footerTextY, display.ToUI(colLabel), h)
}

// drawLevelPanel draws a panel's title and rule; the body is the caller's.
func drawLevelPanel(out display.Sprite, title string) {
	t := display.TextSprite(title, colLabelHot)
	blit(out, t, (screenW-spriteW(t))/2, 4)
	fillRect(out, 0, 14, screenW, 1, accentFor(catSystem).WithAlpha(150))
}

func drawVolume(st *browseState, out display.Sprite) {
	drawLevelPanel(out, "VOLUME")

	drawWedge(out, 6, 26, 52, 16, st.volume, st.muted, accentFor(catSystem))

	pct := display.TextSprite(fmt.Sprintf("%d%%", st.volume), colDim)
	if st.muted {
		pct = display.TextSprite("MUTED", colButtonB)
	}
	blit(out, pct, (screenW-spriteW(pct))/2, 44)

	mute := ui.Hint{Text: "MUTE", Color: ui.ButtonBlue}
	if st.muted {
		mute.Text = "UNMUTE"
	}
	drawDotHints(out, 56,
		ui.Hint{Text: "ADJ", Control: ui.HintStick, Color: display.ToUI(colFooter)},
		mute)
}

// drawBrightness draws the panel brightness wedge and the lamp policy.
func drawBrightness(st *browseState, out display.Sprite) {
	accent := accentFor(catSystem)
	drawLevelPanel(out, "BRIGHTNESS")

	drawWedge(out, 6, 24, 52, 14, st.brightness, false, accent)

	pct := display.TextSprite(fmt.Sprintf("%d%%", st.brightness), colDim)
	blit(out, pct, (screenW-spriteW(pct))/2, 40)

	lampCol := colFooter
	if st.lamps == engine.LampsOff {
		lampCol = colButtonB
	}
	lamps := display.TextSprite(lampsLabel(st.lamps), lampCol)
	blit(out, lamps, (screenW-spriteW(lamps))/2, 48)

	drawDotHints(out, 56,
		ui.Hint{Text: "ADJ", Control: ui.HintStick, Color: display.ToUI(colFooter)},
		ui.Hint{Text: "LAMPS", Color: ui.ButtonBlue})
}

// drawWedge draws a rising right-triangle level meter with a notch at pct; muted draws it dark.
func drawWedge(dst display.Sprite, x, y, w, h int, pct int, muted bool, accent display.Pixel) {
	track := display.Pixel{R: 56, G: 58, B: 66, A: 255}
	fillW := w * pct / 100
	for dx := 0; dx < w; dx++ {
		colH := 1 + dx*(h-1)/(w-1)
		c := track
		if !muted && pct > 0 && dx <= fillW {
			c = accent
		}
		fillRect(dst, x+dx, y+h-colH, 1, colH, c)
	}
	if !muted {
		fillRect(dst, x+min(fillW, w-1), y-2, 2, h+2, colLabelHot)
	}
}

// Cooling panel geometry; the track spans the adjustable threshold range.
const (
	fanTrackX = 6
	fanTrackW = 52
	fanTrackY = 44
)

// drawFan draws the cooling panel: the CPU temperature and the fan trip point on one track.
func drawFan(st *browseState, out display.Sprite) {
	accent := accentFor(catSystem)
	drawLevelPanel(out, "COOLING")

	cpu := display.TextSprite("CPU "+st.cpuTemp, colDim)
	blit(out, cpu, (screenW-spriteW(cpu))/2, 20)

	var trip display.Sprite
	switch st.fanMode {
	case hardware.FanOn:
		trip = display.TextSprite("- FANS ON -", colFooter)
	case hardware.FanOff:
		trip = display.TextSprite("- FANS OFF -", colButtonB)
	default:
		trip = display.TextSprite(fmt.Sprintf("FAN >=%d°F", st.fanThreshold), colFooter)
	}
	blit(out, trip, (screenW-spriteW(trip))/2, 32)

	fanPos := func(degF float64) int {
		f := (degF - config.FanThresholdMin) / (config.FanThresholdMax - config.FanThresholdMin)
		return fanTrackX + int(math.Round(min(1, max(0, f))*float64(fanTrackW-1)))
	}
	fillRect(out, fanTrackX, fanTrackY, fanTrackW, 1, colSeparator)
	if st.cpuTempOK {
		fillRect(out, fanPos(st.cpuTempF), fanTrackY-1, 1, 3, colGold)
	}
	knob := accent
	if st.fanMode != hardware.FanAuto {
		knob = colChevron // the trip point is inert outside auto
	}
	fillRect(out, fanPos(float64(st.fanThreshold)), fanTrackY-3, 2, 7, knob)

	drawDotHints(out, 56,
		ui.Hint{Text: "ADJ", Control: ui.HintStick, Color: display.ToUI(colFooter)},
		ui.Hint{Text: "FAN", Color: ui.ButtonBlue})
}

func drawConfirm(out display.Sprite) {
	fillRect(out, 0, 0, screenW, screenH, display.Pixel{A: 200})
	ui.StrokeRect(&display.SpriteCanvas{Sprite: out}, 4, 18, 56, 28, display.ToUI(colButtonB))
	q := display.TextSprite("SHUT DOWN?", colLabelHot)
	blit(out, q, (screenW-spriteW(q))/2, 22)
	drawDotHints(out, 34,
		ui.Hint{Text: "YES", Color: ui.ButtonGreen},
		ui.Hint{Text: "NO", Color: ui.ButtonRed})
}

var searchRows = []string{"ABCDEFGH", "IJKLMNOP", "QRSTUVWX", "YZ012345", "6789"}

const (
	searchGridTop  = 13
	searchCellW    = 7
	searchCellH    = 8
	searchGridX    = 4
	searchQueryMax = 12
	searchGridCols = 8
	searchBlinkMs  = 600
)

// searchClampCursor clamps a row and column into the ragged grid and returns the cursor ordinal.
func searchClampCursor(row, col int) int {
	row = min(max(row, 0), len(searchRows)-1)
	col = min(max(col, 0), len(searchRows[row])-1)
	return row*searchGridCols + col
}

// searchCellAt maps a cursor ordinal to its row/col.
func searchCellAt(cursor int) (row, col int) {
	row = min(cursor/searchGridCols, len(searchRows)-1)
	col = min(cursor%searchGridCols, len(searchRows[row])-1)
	return row, col
}

func drawSearch(st *browseState, out display.Sprite, ms float64) {
	accent := accentFor(catAll)

	// Query line with a blinking caret.
	q := st.query
	if int(ms/searchBlinkMs)%2 == 0 {
		q += "_"
	}
	blit(out, chevronSprite(controls[entrySearch].glyph, accent), 2, headerTextY)
	blit(out, display.TextSprite(q, colLabelHot), 12, headerTextY)
	fillRect(out, 0, underlineY, screenW, 1, accent.WithAlpha(150))

	curRow, curCol := searchCellAt(st.cursor)
	for r, rowChars := range searchRows {
		for c, ch := range rowChars {
			x := searchGridX + c*searchCellW
			y := searchGridTop + r*searchCellH
			color := colLabel
			if r == curRow && c == curCol {
				pulse := uint8(150 + 90*math.Sin(ms/260))
				fillRect(out, x-1, y, searchCellW, searchCellH, accent.WithAlpha(42))
				ui.StrokeRect(&display.SpriteCanvas{Sprite: out}, x-1, y, searchCellW, searchCellH, display.ToUI(accent.WithAlpha(pulse)))
				color = colLabelHot
			}
			glyph := display.TextSprite(string(ch), color)
			blit(out, glyph, x+(searchCellW-1-spriteW(glyph))/2, y)
		}
	}

	fillRect(out, 0, footerSepY, screenW, 1, colSeparator)
	count := fmt.Sprintf("%d HIT", len(st.results))
	if len(st.results) != 1 {
		count += "S"
	}
	blit(out, display.TextSprite(count, colFooter), 2, footerTextY)
	if len(st.results) > 0 {
		drawCornerHint(out, ui.Hint{Text: "GO", Color: ui.ButtonBlue})
	}
}

// drawDotHints centers a hint row; buttons are drawn as colored discs because the cabinet's carry no letters.
func drawDotHints(out display.Sprite, y int, hints ...ui.Hint) {
	row := ui.HintRow{X: (screenW - ui.HintRowWidth(hints)) / 2, Y: y, LabelColor: display.ToUI(colLabel), Hints: hints}
	row.Draw(&display.SpriteCanvas{Sprite: out})
}

func drawBanner(out display.Sprite, text string, accent display.Pixel) {
	label := display.TextSprite(text, display.Black)
	w := spriteW(label) + 6
	x := (screenW - w) / 2
	fillRect(out, x, 28, w, 11, accent)
	blit(out, label, x+3, 30)
}

// favAnim records which slug was just toggled, in which direction, and when.
type favAnim struct {
	slug   string
	added  bool
	anchor float64
}

const (
	favAnimMs = 450

	// favPulseMs is slow so it does not read as a fault beside the 260ms focus pulse.
	favPulseMs = 2600
	// favPulseFloor is the dimmest badge level; lower erases it over bright icons.
	favPulseFloor = 0.45
)

// colStarHalo separates the badge from the icon beneath it.
var colStarHalo = display.Pixel{R: 8, G: 8, B: 12, A: 215}

// Star pixel art, alpha-keyed (see alphaSprite): the 7×7 badge and the 9×9 pop frame.
var (
	starArt7 = []string{
		"...#...",
		"..+#+..",
		"+#####+",
		".+###+.",
		"..###..",
		".##.##.",
		"##-.-##",
	}
	starArt9 = []string{
		"....#....",
		"...+#+...",
		"...###...",
		"+#######+",
		".+#####+.",
		"..#####..",
		"..##.##..",
		".##...##.",
		"##+...+##",
	}
)

// alphaLevels maps star art characters to coverage; '.' is empty.
var alphaLevels = map[rune]float64{'#': 1, '+': 0.6, '-': 0.3}

// artCoverage decodes alpha-keyed art into per-pixel coverage, 0..1.
func artCoverage(art []string) [][]float64 {
	cov := make([][]float64, len(art))
	for y, line := range art {
		row := make([]float64, len(line))
		for x, ch := range line {
			row[x] = alphaLevels[ch]
		}
		cov[y] = row
	}
	return cov
}

// alphaSprite paints alpha-keyed art in one colour, each pixel at its
// coverage tier times scale.
func alphaSprite(art []string, c display.Pixel, scale float64) display.Sprite {
	cov := artCoverage(art)
	out := make(display.Sprite, len(cov))
	for y, row := range cov {
		px := make([]display.Pixel, len(row))
		for x, f := range row {
			if f > 0 {
				px[x] = c.ScaleAlpha(f * scale)
			}
		}
		out[y] = px
	}
	return out
}

// haloSprite is art's coverage dilated by one pixel minus the art itself, to blit at the art's origin minus (1,1).
// It outlines the star over arbitrary icons without dimming the star.
func haloSprite(art []string, c display.Pixel, scale float64) display.Sprite {
	cov := artCoverage(art)
	w := 0
	for _, row := range cov {
		w = max(w, len(row))
	}
	at := func(y, x int) float64 {
		if y < 0 || y >= len(cov) || x < 0 || x >= len(cov[y]) {
			return 0
		}
		return cov[y][x]
	}
	out := make(display.Sprite, len(cov)+2)
	for y := range out {
		px := make([]display.Pixel, w+2)
		for x := range px {
			spread := 0.0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					spread = math.Max(spread, at(y-1+dy, x-1+dx))
				}
			}
			if ring := spread - at(y-1, x-1); ring > 0 {
				px[x] = c.ScaleAlpha(ring * scale)
			}
		}
		out[y] = px
	}
	return out
}

// drawStar stamps a halo then the star at (x,y), both at the same alpha.
func drawStar(dst display.Sprite, art []string, x, y int, c display.Pixel, scale float64) {
	blit(dst, haloSprite(art, colStarHalo, scale), x-1, y-1)
	blit(dst, alphaSprite(art, c, scale), x, y)
}

// favPulse is the resting badge alpha, between favPulseFloor and 1.
func favPulse(ms float64) float64 {
	return favPulseFloor + (1-favPulseFloor)*(0.5+0.5*math.Sin(2*math.Pi*ms/favPulseMs))
}

// drawFavStar badges a favorited tile's icon corner, and animates the toggle:
// a white pop settling into the badge on favorite, a gray fade on un-favorite.
func (b *browseEl) drawFavStar(list display.Sprite, e entry, slide, rowY int, ms float64) {
	if e.kind != entryApp {
		return
	}
	faved := b.st.favs[e.m.Slug]
	anim := b.st.favAnim
	animating := anim.slug == e.m.Slug && ms-anim.anchor < favAnimMs
	if !faved && !animating {
		return
	}
	// Overhangs the icon's top-right corner; the halo ends one column short of the label.
	x, y := slide+6, rowY-1
	switch {
	case animating && anim.added:
		frac := (ms - anim.anchor) / favAnimMs
		if frac < 0.5 {
			drawStar(list, starArt9, x-1, y-1, colLabelHot, 1)
		} else {
			drawStar(list, starArt7, x, y, colButtonC, 1)
		}
	case animating: // un-favorite: gray ghost fading out, visibly not yellow
		frac := (ms - anim.anchor) / favAnimMs
		drawStar(list, starArt7, x, y, colLabel, 1-frac)
	default:
		drawStar(list, starArt7, x, y, colButtonC, favPulse(ms))
	}
}

// chevronSprite paints '#'-keyed pixel art in one colour.
func chevronSprite(art []string, c display.Pixel) display.Sprite {
	out := make(display.Sprite, len(art))
	for y, line := range art {
		row := make([]display.Pixel, len(line))
		for x, ch := range line {
			if ch == '#' {
				row[x] = c
			}
		}
		out[y] = row
	}
	return out
}

// drawTileIcon draws the app icon or control glyph, or a letter placeholder for an app without an icon.
func drawTileIcon(out display.Sprite, e entry, accent display.Pixel, x, y int) {
	if e.kind != entryApp {
		ctl := e.control()
		blit(out, chevronSprite(ctl.glyph, colDim), x, y)
		if ctl.fill != nil {
			blit(out, chevronSprite(ctl.fill, colMercury), x, y)
		}
		return
	}
	if e.m.Icon != nil {
		blit(out, e.m.Icon, x, y)
		return
	}
	fillRect(out, x, y, 8, 8, display.Pixel{R: 34, G: 36, B: 42, A: 255})
	ui.StrokeRect(&display.SpriteCanvas{Sprite: out}, x, y, 8, 8, display.ToUI(accent.WithAlpha(90)))
	if e.m.Name != "" {
		letter := display.TextSprite(string([]rune(e.m.Name)[:1]), accent)
		blitWindow(out, letter, x+1, y, x+1, x+7)
	}
}
