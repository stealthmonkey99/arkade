// SPDX-License-Identifier: GPL-2.0-or-later

// Package shell is the cabinet's game browser: shelves by manifest tag, a
// search keyboard, system controls on the SYSTEM shelf, and a detail card per
// game. Each of those is a surface (surface.go); the one on top owns the
// keys, the stick, the button lights and the LCDs.
package shell

import (
	"fmt"
	"os"
	"strings"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/config"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scores"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// A stick held past holdFastAfter steps doubles its repeat speed.
const (
	holdFastIntervalMicros = 60_000
	holdFastAfter          = 12
)

// announceMs is how long a favorite-toggle banner stays up.
const announceMs = 1400

type settings struct {
	engine.ShellRecents
	// Favorites are in the order they were starred.
	Favorites []string `json:"favorites,omitempty"`
	Category  string   `json:"category,omitempty"`
	Focused   string   `json:"focused,omitempty"`
	// FocusedControl is the label of a focused control tile, which Focused cannot name.
	FocusedControl string `json:"focusedControl,omitempty"`
}

type app struct {
	ctx      *engine.AppContext
	queue    input.Queue
	settings settings

	st     browseState
	detail detailState
	browse *browseEl
	card   *detailEl
	// screens is the stack under the detail card; the shelf is always at the bottom.
	screens []screen

	// Focused game's best score, looked up on focus change.
	best   scores.Entry
	bestOK bool

	// The stick reports position changes only, so a held direction persists until the next joystickchange.
	hold ui.HoldRepeat

	// Sampling cadence for the pad pips and CPU temperature.
	padsPrimed bool
	padsAt     float64
	tempPrimed bool
	tempAt     float64
}

func init() {
	engine.Register(engine.Manifest{
		Slug:        "shell",
		Name:        "Shell",
		Type:        engine.TypeShell,
		Hidden:      true,
		ExitsOnQuit: true,
		New:         func(ctx *engine.AppContext) engine.App { return &app{ctx: ctx} },
	})
}

func (a *app) Init() error {
	if err := a.ctx.ReadStorage("", &a.settings); err != nil {
		a.ctx.Log(fmt.Sprintf("settings read failed: %v", err))
	}
	cab, err := config.Load(a.ctx.RT.Store)
	if err != nil {
		a.ctx.Log(fmt.Sprintf("cabinet settings read failed: %v", err))
	}
	a.hold = ui.HoldRepeat{FastAfter: holdFastAfter, FastIntervalMicros: holdFastIntervalMicros}
	a.initAudio(cab)
	a.initFan(cab)
	a.initBrightness()
	a.rebuild()
	a.st.jumpTo(a.settings.Category, a.settings.Focused, a.settings.FocusedControl)
	a.screens = []screen{shelf}

	a.browse = newBrowseEl(a)
	a.card = newDetailEl(&a.detail)
	a.ctx.AddFilledLayer("bg", display.Black, nil, 1)
	a.ctx.AddLayer("browse", []display.Element{a.browse}, 1)
	a.ctx.AddLayer("detail", []display.Element{a.card}, 0)

	joystickChange := input.KindJoystickChange
	a.ctx.AddKeyMap([]*input.Mapping{{
		Conditions: []input.Condition{
			input.Key(input.KeyUp), input.Key(input.KeyDown),
			input.Key(input.KeyLeft), input.Key(input.KeyRight),
			input.Key(input.KeyEnter), input.Key(input.KeySpace),
			input.Key(input.KeyJoyA), input.Key(input.KeyJoyC), input.Key(input.KeyJoyD),
			// Joystickchange feeds hold-to-repeat; a keypress condition never sees it.
			{Kind: &joystickChange},
		},
		Queue: &a.queue,
	}})

	// Registered here to shadow the engine's default quit mapping, which would exit the process.
	a.ctx.AddKeyMap([]*input.Mapping{{
		Conditions: []input.Condition{
			input.Key(input.KeyQuit), input.Key(input.KeyEscape), input.Key(input.KeyJoyB),
		},
		Trigger: func(input.Event) { a.top().back(a) },
	}})

	a.onFocusChange()
	a.updateLCDs(clock.FrameTime{}) // first paint now, not a tick later
	total := 0
	for _, c := range a.st.cats {
		if c.id == catAll {
			total = len(c.entries) - 1 // minus the search tile
		}
	}
	a.ctx.Log(fmt.Sprintf("%d apps found in library. %d shelves.", total, len(a.st.cats)))
	return nil
}

// top is the surface that owns input, lights and LCDs: the detail card while it is open.
func (a *app) top() surface {
	if a.detail.open {
		return detailCard
	}
	return a.screen()
}

// screen is the surface the browse layer paints.
func (a *app) screen() screen { return a.screens[len(a.screens)-1] }

func (a *app) push(s screen) {
	a.screens = append(a.screens, s)
	a.onFocusChange()
}

func (a *app) pop() {
	if len(a.screens) > 1 {
		a.screens = a.screens[:len(a.screens)-1]
	}
	a.onFocusChange()
}

func (a *app) showing(s screen) bool {
	for _, on := range a.screens {
		if on == s {
			return true
		}
	}
	return false
}

// leave is the shelf's B: into standby from the browse root, or out when standby is disabled.
func (a *app) leave() { a.ctx.EnterStandby() }

// rebuild derives the catalog from the registry and keeps the current shelf and focus.
func (a *app) rebuild() {
	catID, slug, control := "", "", ""
	if cur, ok := a.st.activeCat(); ok {
		catID = cur.id
	}
	if m, ok := a.st.focusedApp(); ok {
		slug = m.Slug
	} else if e, ok := a.st.focused(); ok {
		control = e.label
	}
	a.st.cats = buildCatalog(engine.Apps(), a.settings.MRU, a.settings.Favorites)
	a.st.favs = map[string]bool{}
	for _, slug := range a.settings.Favorites {
		a.st.favs[slug] = true
	}
	a.st.cat, a.st.focus = 0, 0
	a.st.jumpTo(catID, slug, control)
}

func (a *app) touchMRU(slug string) {
	mru := []string{slug}
	for _, s := range a.settings.MRU {
		if s != slug {
			mru = append(mru, s)
		}
	}
	a.settings.MRU = mru
}

func (a *app) HandleInput(clock.FrameTime) {
	for _, ev := range a.queue.Drain() {
		if ev.Kind == input.KindJoystickChange {
			a.hold.Hold(element.StickDir(ev.Position))
			continue
		}
		if ev.Kind == input.KindKeypress {
			a.dispatch(ev.Name)
		}
	}
}

// dispatch routes a keypress to the top surface: directions step, the three
// select keys arrive as green, C and D as themselves. B has its own mapping.
func (a *app) dispatch(name input.KeyName) {
	switch name {
	case input.KeyUp, input.KeyDown, input.KeyLeft, input.KeyRight:
		a.top().step(a, name)
	case input.KeyEnter, input.KeySpace, input.KeyJoyA:
		a.top().press(a, input.KeyJoyA)
	case input.KeyJoyC, input.KeyJoyD:
		a.top().press(a, name)
	}
}

// openWifi shows the wifi-join QR from WIFI_SSID and WIFI_PASS as a modal.
func (a *app) openWifi() {
	if a.st.wifiQR == nil {
		border := 2
		qr, err := element.NewWifiQRCode(os.Getenv("WIFI_SSID"), os.Getenv("WIFI_PASS"), element.QRCodeOptions{Border: &border})
		if err != nil {
			a.ctx.Log(fmt.Sprintf("wifi qr: %v", err))
			a.ctx.RejectKey()
			return
		}
		a.st.wifiQR = qr.Render(clock.FrameTime{})
	}
	a.push(wifi)
}

func (a *app) refreshResults() {
	a.st.results = searchMatches(engine.Apps(), a.st.query)
	a.applyLights() // D's "go to results" eligibility tracks the count
}

func (a *app) openSearch() {
	a.st.savedCat, a.st.savedFocus = a.st.cat, a.st.focus
	a.st.query, a.st.cursor = "", 0
	a.st.results = nil
	a.push(search)
}

func (a *app) launch(m engine.Manifest) {
	a.touchMRU(m.Slug)
	a.rememberPlace()
	a.ctx.LaunchManifest(m)
}

// toggleFavorite stars or un-stars the focused game and persists immediately.
func (a *app) toggleFavorite() {
	m, ok := a.st.focusedApp()
	if !ok || m.Type != engine.TypeGame {
		a.ctx.RejectKey()
		return
	}
	added := true
	for i, slug := range a.settings.Favorites {
		if slug == m.Slug {
			a.settings.Favorites = append(a.settings.Favorites[:i:i], a.settings.Favorites[i+1:]...)
			added = false
			break
		}
	}
	if added {
		a.settings.Favorites = append(a.settings.Favorites, m.Slug)
		a.ctx.PlaySound(boopFile)
		a.st.announce = "FAVORITED"
	} else {
		a.st.announce = "UN-FAVORITED"
	}
	if err := a.ctx.WriteStorage("", a.settings); err != nil {
		a.ctx.Log(fmt.Sprintf("favorites save failed: %v", err))
	}
	a.st.favAnim = favAnim{slug: m.Slug, added: added, anchor: a.st.now}
	a.st.announceAccent = accentFor(catFavorites)
	a.st.announceUntil = a.st.now + announceMs
	a.rebuild() // the FAVORITES shelf appears/updates/vanishes live
	a.onFocusChange()
}

// openDetail opens the focused game's detail card.
func (a *app) openDetail() {
	m, ok := a.st.focusedApp()
	if !ok || m.Type != engine.TypeGame {
		a.ctx.RejectKey()
		return
	}
	cur, _ := a.st.activeCat()
	table := scores.Load(a.ctx.RT.Store, m.Slug)
	best, bestOK := table.Best()
	a.detail = detailState{
		open:    true,
		m:       m,
		accent:  cur.accent,
		best:    detailBest(best, bestOK),
		anchor:  a.st.now,
		alpha:   a.detail.alpha,
		webSlug: webSlugFor(m),
	}
	a.detail.setContent(table)
	a.applyLights()
}

// toggleQR flips the web-play QR overlay; it rejects for games with no public play page.
func (a *app) toggleQR() {
	d := &a.detail
	if d.webSlug == "" {
		a.ctx.RejectKey()
		return
	}
	if d.qr == nil {
		qr, err := buildQR(d.webSlug)
		if err != nil {
			a.ctx.Log(fmt.Sprintf("qr for %s: %v", d.webSlug, err))
			a.ctx.RejectKey()
			return
		}
		d.qr = qr
	}
	d.qrOpen = !d.qrOpen
	a.applyLights()
}

func (a *app) closeDetail() {
	a.detail.open = false
	a.applyLights()
}

// onFocusChange refreshes the best score, footer text and button lights.
func (a *app) onFocusChange() {
	a.best, a.bestOK = scores.Entry{}, false
	a.st.hiScore = ""
	if m, ok := a.st.focusedApp(); ok {
		if best, ok := scores.Load(a.ctx.RT.Store, m.Slug).Best(); ok {
			a.best, a.bestOK = best, true
			a.st.hiScore = fmt.Sprintf("HI %d", best.Score)
		}
	}
	a.applyLights()
}

// applyLights lights each button exactly when pressing it does something on the top surface.
func (a *app) applyLights() {
	a.ctx.SetButtonLights(input.BothSeats, a.top().lights(a), true)
}

// greenAndRed is the light set every surface starts from: A and B always do something.
func greenAndRed() hardware.ButtonLights {
	return hardware.ButtonLights{"joya": true, "joyb": true}
}

// Update advances animations, hold-to-repeat, the detail fade and the LCDs; the browser repaints every tick.
func (a *app) Update(stats clock.FrameTime) {
	a.st.animate(stats)
	a.detail.animate(stats)
	a.refreshCPUTemp(stats)
	a.refreshPadSeats(stats)
	for n := a.hold.Update(element.ElapsedMicros(stats)); n > 0; n-- {
		a.top().step(a, element.Key(a.hold.Held()))
	}

	target := 0.0
	if a.detail.open {
		target = 1
	}
	if a.detail.alpha != target {
		step := stats.ElapsedMs / 150 // ~150ms fade either way
		if a.detail.alpha < target {
			a.detail.alpha = min(target, a.detail.alpha+step)
		} else {
			a.detail.alpha = max(target, a.detail.alpha-step)
		}
		a.ctx.SetLayerAlpha("detail", a.detail.alpha)
	}

	a.browse.MarkDirty()
	if a.detail.alpha > 0 {
		a.card.MarkDirty()
	}
	a.updateLCDs(stats)
}

// Resumed drops a stick hold whose release arrived while the shell was parked in standby.
func (a *app) Resumed() { a.hold.Reset() }

// LCD legends name a button by colour, since the cabinet's buttons are unlettered.
// The shelf legend alternates with the status line on this duty cycle.
const (
	legendPrimaryMs = 3500
	legendHintMs    = 1500

	// scrollLegendLine rides the same duty cycle on the detail card.
	scrollLegendLine = "PUSH DOWN: MORE"
)

// legendOn reports whether the duty cycle is in its legend phase.
func legendOn(stats clock.FrameTime) bool {
	return int(stats.DurationMs)%(legendPrimaryMs+legendHintMs) >= legendPrimaryMs
}

// legendFor is the focused tile's 16-column shelf legend.
func legendFor(e entry) string {
	if e.kind == entryApp {
		if e.m.Type == engine.TypeGame {
			return "YELLOW = PIN"
		}
		return "GREEN = OPEN"
	}
	if legend := e.control().legend; legend != "" {
		return legend
	}
	return "GREEN = OPEN"
}

// updateLCDs mirrors the top surface onto the two LCDs every frame; the engine drops unchanged lines.
func (a *app) updateLCDs(stats clock.FrameTime) {
	lines := a.top().lcd(a, stats)
	a.ctx.SetText(input.SeatsOf(input.SeatP1), lines[0][0], lines[0][1])
	a.ctx.SetText(input.SeatsOf(input.SeatP2), lines[1][0], lines[1][1])
}

// browseLCDs fills both LCDs identically from the focused tile. The status line
// shows, in priority order, the announce banner, the legend, the best score, the players badge.
func (a *app) browseLCDs(stats clock.FrameTime, legend bool) (lines [2][2]string) {
	e, ok := a.st.focused()
	if !ok {
		lines[0][0], lines[0][1] = "Årkade", "no games found"
		lines[1] = lines[0]
		return lines
	}
	ms := stats.DurationMs - a.st.focusAnchor

	if e.kind != entryApp {
		lines[0][0], lines[0][1] = e.control().lcd(a)
	} else {
		lines[0][0] = lcdWindow(e.m.Name, ms)
		lines[0][1] = lcdWindow(e.m.Description, ms)
	}

	switch {
	case a.st.announce != "" && stats.DurationMs < a.st.announceUntil:
		lines[0][1] = a.st.announce
	case legend && legendOn(stats):
		lines[0][1] = legendFor(e)
	case a.bestOK && a.best.Name != "":
		lines[0][1] = fmt.Sprintf("%s %d", strings.ToUpper(a.best.Name), a.best.Score)
	case a.bestOK:
		lines[0][1] = fmt.Sprintf("high: %d", a.best.Score)
	case e.kind == entryApp:
		if badge := playersBadge(e.m.Players); badge != "" {
			lines[0][1] = badge
		}
	}

	lines[1] = lines[0]
	return lines
}

// lcdBar renders a percentage as a 16-column block bar.
func lcdBar(pct int) string {
	fill := min(hardware.LcdCols, max(0, (pct*hardware.LcdCols+50)/100))
	return strings.Repeat("█", fill) + strings.Repeat("░", hardware.LcdCols-fill)
}

func (a *app) Render(clock.FrameTime, engine.RenderReason) {}

// rememberPlace stores the shelf and entry for the next load; search results remember ALL.
func (a *app) rememberPlace() {
	if a.showing(search) {
		a.settings.Category = catAll
	} else if cur, ok := a.st.activeCat(); ok {
		a.settings.Category = cur.id
	}
	if m, ok := a.st.focusedApp(); ok {
		a.settings.Focused = m.Slug
		a.settings.FocusedControl = ""
	} else if e, ok := a.st.focused(); ok {
		a.settings.FocusedControl = e.label
	}
}

func (a *app) Teardown() error {
	a.rememberPlace()
	return a.ctx.WriteStorage("", a.settings)
}

// saveCabinet re-reads the cabinet settings, applies set and writes them back,
// so a field another owner changed meanwhile survives.
func (a *app) saveCabinet(set func(*config.Cabinet)) {
	cab, _ := config.Load(a.ctx.RT.Store)
	set(&cab)
	if err := cab.Save(a.ctx.RT.Store); err != nil {
		a.ctx.Log(fmt.Sprintf("cabinet settings save failed: %v", err))
	}
}

var _ engine.Resumer = (*app)(nil)
