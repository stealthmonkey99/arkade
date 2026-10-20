// SPDX-License-Identifier: GPL-2.0-or-later

package shell

import (
	"fmt"
	"strings"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// surface is one mode of the shell. The shell keeps a stack of screens with
// the shelf at the bottom, and the detail card, when open, above whichever is
// on top; the top surface owns the keys, the stick, the button lights and the
// LCD lines, and B always backs out one level.
type surface interface {
	// press is green (every select key arrives as KeyJoyA), yellow or blue.
	press(a *app, key input.KeyName)
	// step is a stick direction, on the press edge and on every hold repeat.
	step(a *app, dir input.KeyName)
	back(a *app)
	// lights are lit exactly when the button's press would do something.
	lights(a *app) hardware.ButtonLights
	lcd(a *app, stats clock.FrameTime) [2][2]string
}

// screen is a surface that paints the browse layer; the detail card has a layer of its own.
type screen interface {
	surface
	draw(a *app, out display.Sprite, ms float64)
}

var (
	shelf        = &shelfSurface{}
	resultsShelf = &shelfSurface{results: true}
	search       = &searchSurface{}
	wifi         = &wifiSurface{}
	confirmOff   = &confirmSurface{}
	detailCard   = &detailSurface{}
)

// shelfSurface is the browse root, or the SEARCH results shelf above the keyboard.
type shelfSurface struct{ results bool }

func (s *shelfSurface) press(a *app, key input.KeyName) {
	switch key {
	case input.KeyJoyA:
		if e, ok := a.st.focused(); ok {
			a.activate(e)
		}
	case input.KeyJoyC:
		a.toggleFavorite() // rejects itself on non-game tiles
	case input.KeyJoyD:
		e, ok := a.st.focused()
		if !ok || !e.interactable() {
			a.ctx.RejectKey()
			return
		}
		a.openDetail()
	}
}

// activate is the A press on a tile: launch an app, open a control.
func (a *app) activate(e entry) {
	if e.kind == entryApp {
		a.launch(e.m)
		return
	}
	e.control().open(a, e)
}

func (s *shelfSurface) step(a *app, dir input.KeyName) {
	switch dir {
	case input.KeyUp:
		if a.st.moveFocus(-1) {
			a.onFocusChange()
		}
	case input.KeyDown:
		if a.st.moveFocus(1) {
			a.onFocusChange()
		}
	case input.KeyLeft, input.KeyRight:
		delta := 1
		if dir == input.KeyLeft {
			delta = -1
		}
		// Slider tiles take left/right for an in-place adjust.
		if e, ok := a.st.focused(); ok && e.kind != entryApp && e.control().slider != nil {
			e.control().slider.adjust(a, delta)
			return
		}
		if s.results {
			return // results keep their query context; B goes back
		}
		if a.st.moveCat(delta) {
			a.onFocusChange()
		}
	}
}

func (s *shelfSurface) back(a *app) {
	if !s.results {
		a.leave()
		return
	}
	a.st.showingResults = false
	a.pop()
}

func (s *shelfSurface) lights(a *app) hardware.ButtonLights {
	lights := greenAndRed()
	if m, ok := a.st.focusedApp(); ok && m.Type == engine.TypeGame {
		lights["joyc"] = true // favorite toggle
	}
	if e, ok := a.st.focused(); ok && e.interactable() {
		lights["joyd"] = true
	}
	return lights
}

func (s *shelfSurface) lcd(a *app, stats clock.FrameTime) [2][2]string {
	return a.browseLCDs(stats, !s.results)
}

func (s *shelfSurface) draw(a *app, out display.Sprite, ms float64) {
	a.browse.drawShelf(out, ms)
}

// searchSurface is the on-screen keyboard; D moves up to the results shelf.
type searchSurface struct{}

func (*searchSurface) press(a *app, key input.KeyName) {
	switch key {
	case input.KeyJoyA:
		if len(a.st.query) >= searchQueryMax {
			a.ctx.RejectKey()
			return
		}
		row, col := searchCellAt(a.st.cursor)
		a.st.query += string(searchRows[row][col])
		a.refreshResults()
	case input.KeyJoyC:
		if a.st.query == "" {
			a.ctx.RejectKey()
			return
		}
		a.st.query = a.st.query[:len(a.st.query)-1]
		a.refreshResults()
	case input.KeyJoyD:
		if len(a.st.results) == 0 {
			a.ctx.RejectKey()
			return
		}
		a.st.focus = 0
		a.st.scrollRow = 0
		a.st.focusAnchor = a.st.now
		a.st.showingResults = true
		a.push(resultsShelf)
	}
}

func (*searchSurface) step(a *app, dir input.KeyName) {
	row, col := searchCellAt(a.st.cursor)
	switch dir {
	case input.KeyUp:
		row--
	case input.KeyDown:
		row++
	case input.KeyLeft:
		col--
	case input.KeyRight:
		col++
	}
	a.st.cursor = searchClampCursor(row, col)
}

// back restores the browse position the search tile was opened from.
func (*searchSurface) back(a *app) {
	a.st.cat, a.st.focus = a.st.savedCat, a.st.savedFocus
	a.pop()
}

func (*searchSurface) lights(a *app) hardware.ButtonLights {
	lights := greenAndRed()
	lights["joyc"] = a.st.query != ""      // backspace
	lights["joyd"] = len(a.st.results) > 0 // go to results
	return lights
}

func (*searchSurface) lcd(a *app, _ clock.FrameTime) (lines [2][2]string) {
	lines[0][0] = lcdWindow("SEARCH: "+a.st.query, 0)
	lines[0][1] = fmt.Sprintf("%d matches", len(a.st.results))
	lines[1][0] = "GREEN = TYPE"
	lines[1][1] = "YELLOW = DELETE"
	return lines
}

func (*searchSurface) draw(a *app, out display.Sprite, ms float64) {
	drawSearch(&a.st, out, ms)
}

// wifiSurface is the join-wifi QR modal over the shelf; only B does anything.
type wifiSurface struct{}

func (*wifiSurface) press(a *app, _ input.KeyName) { a.ctx.RejectKey() }
func (*wifiSurface) step(*app, input.KeyName)      {}
func (*wifiSurface) back(a *app)                   { a.pop() }

func (*wifiSurface) lights(*app) hardware.ButtonLights {
	return hardware.ButtonLights{"joyb": true}
}

func (*wifiSurface) lcd(*app, clock.FrameTime) (lines [2][2]string) {
	lines[0][0], lines[0][1] = "JOIN WIFI", "scan with phone"
	lines[1][0] = "JOIN WIFI"
	return lines
}

func (*wifiSurface) draw(a *app, out display.Sprite, ms float64) {
	a.browse.drawShelf(out, ms)
	drawQRModal(out, a.st.wifiQR, accentFor(catSystem))
}

// confirmSurface is the shutdown question over the shelf: green powers off, red cancels.
type confirmSurface struct{}

func (*confirmSurface) press(a *app, key input.KeyName) {
	if key != input.KeyJoyA {
		a.ctx.RejectKey()
		return
	}
	a.ctx.Log("shutdown confirmed from the SYSTEM shelf")
	a.ctx.RT.PowerOff()
}

func (*confirmSurface) step(*app, input.KeyName) {}
func (*confirmSurface) back(a *app)              { a.pop() }

func (*confirmSurface) lights(*app) hardware.ButtonLights { return greenAndRed() }

func (*confirmSurface) lcd(*app, clock.FrameTime) (lines [2][2]string) {
	lines[0][0], lines[0][1] = "SHUT DOWN?", "GREEN YES RED NO"
	lines[1] = lines[0]
	return lines
}

func (*confirmSurface) draw(a *app, out display.Sprite, ms float64) {
	a.browse.drawShelf(out, ms)
	drawConfirm(out)
}

// detailSurface is the open detail card, painted by its own layer.
type detailSurface struct{}

func (*detailSurface) press(a *app, key input.KeyName) {
	switch key {
	case input.KeyJoyA:
		a.launch(a.detail.m)
	case input.KeyJoyC:
		a.toggleQR()
	case input.KeyJoyD:
		a.ctx.RejectKey() // unlit: interact already happened
	}
}

// step scrolls the card; left and right do nothing on it.
func (*detailSurface) step(a *app, dir input.KeyName) {
	switch dir {
	case input.KeyUp:
		a.detail.scrollBy(-1)
	case input.KeyDown:
		a.detail.scrollBy(1)
	}
}

func (*detailSurface) back(a *app) {
	if a.detail.qrOpen {
		a.detail.qrOpen = false
		a.applyLights()
		return
	}
	a.closeDetail()
}

func (*detailSurface) lights(a *app) hardware.ButtonLights {
	lights := greenAndRed()
	// C opens the web play page when there is one and it is not showing.
	lights["joyc"] = a.detail.webSlug != "" && !a.detail.qrOpen
	return lights
}

func (*detailSurface) lcd(a *app, stats clock.FrameTime) (lines [2][2]string) {
	d := &a.detail
	ms := stats.DurationMs - d.anchor
	if d.qrOpen {
		// Readable URL as a fallback when the phone cannot scan the panel.
		lines[0][0] = "PLAY ONLINE"
		lines[0][1] = lcdWindow("arkade.build/"+d.webSlug, ms)
		lines[1][0] = "scan with phone"
		return lines
	}
	lines[0][0] = lcdWindow(d.m.Name, ms)
	lines[0][1] = "GREEN = PLAY"
	lines[1][0] = lcdWindow(strings.ToUpper(strings.Join(d.m.Tags, " ")), ms)
	lines[1][1] = lcdWindow(playersLine(d.m.Players), ms)
	// Scrolling has no button light, so the status line advertises it.
	if d.scroll < d.maxScroll && legendOn(stats) {
		lines[1][1] = scrollLegendLine
	}
	return lines
}

// levelPanel is a full-screen adjuster for one cabinet level: the stick moves
// it, blue toggles its companion setting, green returns to the shelf. The
// same panel serves as the in-place slider of its shelf tile.
type levelPanel struct {
	// adjust steps the level by delta notches, rejecting the key at a rail.
	adjust  func(a *app, delta int)
	toggle  func(a *app)
	paint   func(st *browseState, out display.Sprite)
	readout func(st *browseState) [2][2]string
}

func (p *levelPanel) press(a *app, key input.KeyName) {
	switch key {
	case input.KeyJoyA:
		a.pop()
	case input.KeyJoyD:
		p.toggle(a)
	case input.KeyJoyC:
		a.ctx.RejectKey()
	}
}

func (p *levelPanel) step(a *app, dir input.KeyName) {
	switch dir {
	case input.KeyLeft, input.KeyDown:
		p.adjust(a, -1)
	case input.KeyRight, input.KeyUp:
		p.adjust(a, 1)
	}
}

func (p *levelPanel) back(a *app) { a.pop() }

func (p *levelPanel) lights(*app) hardware.ButtonLights {
	lights := greenAndRed()
	lights["joyd"] = true
	return lights
}

func (p *levelPanel) lcd(a *app, _ clock.FrameTime) [2][2]string { return p.readout(&a.st) }

func (p *levelPanel) draw(a *app, out display.Sprite, _ float64) { p.paint(&a.st, out) }
