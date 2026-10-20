// SPDX-License-Identifier: GPL-2.0-or-later

// Package attract is the power-saving screensaver: a silent app that shows a
// dim star-warp animation and cycles each game's high score on top of it.
// Only the engine's standby state machine loads it.
package attract

import (
	"embed"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"

	"github.com/stealthmonkey99/arkade/internal/brand"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scores"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

//go:embed assets
var assetsFS embed.FS

// slotMs is the base duration of one showcase slot; gameSlotMs extends it
// for games with a leaderboard ticker.
const slotMs = 8000

// ambientDim caps ambient art brightness to match the starfield's alpha cap
// (max about 0.55).
const ambientDim = 0.55

// chaseStepMs paces the button-light hop.
const chaseStepMs = 1000

// wordmarkFrame draws the wordmark at the boot splash's resting state. It is
// a pure function of t (ms since attract started) and always draws at full
// strength; ambientDim is applied by the element's Opacity.
func wordmarkFrame(t float64) display.Matrix {
	c := brand.NewCanvas(64, 64)
	c.RestingWordmark(t)
	return c.Matrix()
}

func init() {
	engine.Register(engine.Manifest{
		Slug:   "attract",
		Name:   "Attract",
		Type:   engine.TypeAttract,
		Hidden: true,
		New:    newApp,
	})
}

// lcdLine1/lcdLine2 are attract's constant LCD text.
const (
	lcdLine1 = "Årkade"
	lcdLine2 = "press any button"
)

// greenButton is the light name of the green (JoyA) button, hardware.LightButtons[0].
const greenButton = "joya"

// highScore is one game's showcase entry.
type highScore struct {
	slug       string // the game's manifest slug, for green-button hold-to-launch
	name       string
	playerName string // the recorded high-scorer's name; "" when none was saved
	high       int
	rest       []scores.Entry // ranks 2-10, for the leaderboard ticker
	icon       display.Sprite // the manifest's 8×8 logo; nil when a game has none
	// newAttract is non-nil for a cartridge that draws its own attract slot
	// (see engine.Manifest.NewAttract).
	newAttract func(ctx *engine.AppContext) (engine.AttractRenderer, bool)
}

type app struct {
	ctx      *engine.AppContext
	field    *starfield
	wordmark *element.SpriteEl
	figure   *element.SpriteEl
	icon     *element.SpriteEl
	name     *element.Label
	// score is the whole #1 line, name and score together.
	score  *element.Label
	trophy *element.SpriteEl
	board  *ticker

	scores []highScore
	// focused is set for a dashboard preview of one game; scores then holds
	// only that game.
	focused bool

	schedule      []slot
	scheduleTotal float64
	cur           int
	haveSlot      bool
	// bootSettle is set when attract follows the boot splash; see wordmarkOpacity.
	bootSettle bool

	chaseStep int
	haveChase bool
	lit       [2]int // currently lit button per player

	// holdShown tracks the hold-to-launch feedback; see updateHoldFeedback.
	holdShown bool

	// attractR is the current slot's cartridge renderer, nil for a default slot.
	attractR engine.AttractRenderer
}

func newApp(ctx *engine.AppContext) engine.App { return &app{ctx: ctx} }

// Init builds the layers bottom-first: backstop, starfield, ambient views,
// showcase.
func (a *app) Init() error {
	a.scores = readHighScores(a.ctx)
	a.applyFocus(a.ctx.RT.TakeAttractFocus())
	a.bootSettle = a.ctx.RT.TakeBootSettle()

	// Opaque backstop: the engine keeps a suspended app's layers alive
	// underneath (quick-resume), and attract must cover them.
	a.ctx.AddFilledLayer("bg", display.Black, nil, 1)

	a.field = newStarfield(a.ctx.RT.Rand)
	a.ctx.AddLayer("ambient", []display.Element{a.field}, 1)

	wordDim, figureDim := ambientDim, ambientDim // separate pointers so the two fade independently
	a.wordmark = element.NewSprite(element.SpriteOptions{
		Options: element.Options{Hidden: true, Opacity: &wordDim},
	})
	// Hand-drawn at panel resolution (hairlines go sub-pixel at 64px). White
	// with alpha as coverage; regenerate as NRGBA, since premultiplied
	// image.RGBA at partial alpha encodes as its own inverse.
	figure, err := element.SpriteFromPNG(assetsFS, "assets/figure.png", element.Options{
		Hidden: true, Opacity: &figureDim,
	})
	if err != nil {
		return fmt.Errorf("figure sprite: %w", err)
	}
	a.figure = figure
	a.ctx.AddLayer("ambient-fg", []display.Element{a.wordmark, a.figure}, 1)

	white := display.White
	// x 28 centres the 8px logo; sprites don't self-centre like labels.
	a.icon = element.NewSprite(element.SpriteOptions{
		Options: element.Options{OffsetX: 28, OffsetY: 12, Hidden: true},
	})
	a.name = element.NewLabel(element.LabelOptions{
		Options: element.Options{OffsetX: 32, OffsetY: 24, Align: element.TopCenter},
		Color:   &white,
	})
	// The #1 line, e.g. "ANNA 4242", the same shape as the ticker rows.
	a.score = element.NewLabel(element.LabelOptions{
		Options: element.Options{OffsetX: 32, OffsetY: 34, Align: element.TopCenter},
		Color:   &white,
	})
	// applySlot positions the trophy per game since text width varies.
	a.trophy = element.NewSprite(element.SpriteOptions{
		Options: element.Options{Hidden: true},
		Data:    trophySprite(),
	})
	a.board = newTicker()
	a.ctx.AddLayer("showcase", []display.Element{a.icon, a.name, a.score, a.trophy, a.board}, 1)

	a.schedule = a.buildSchedule()
	for _, s := range a.schedule {
		a.scheduleTotal += s.dur
	}

	// Applied here rather than in the first Update: loadApp renders as soon
	// as Init returns, and a bare starfield frame would blink.
	cur, rem := a.slotAt(0)
	a.cur, a.haveSlot = cur, true
	a.applySlot()
	a.board.start(-rem)
	a.driveAmbient(0, rem)

	a.ctx.SetLights(input.BothSeats, false)
	a.ctx.SetText(input.BothSeats, lcdLine1, lcdLine2)
	return nil
}

// applyFocus narrows the showcase to one game for a dashboard preview. A slug
// with no slot (no score, no cart_attract, or unmounted since the request)
// falls back to the normal rotation.
func (a *app) applyFocus(slug string) {
	if slug == "" {
		return
	}
	for _, sc := range a.scores {
		if sc.slug == slug {
			a.scores = []highScore{sc}
			a.focused = true
			return
		}
	}
	a.ctx.RT.Log("warn", fmt.Sprintf("attract: preview %q has no showcase slot; rotating normally", slug))
}

// slot is one schedule entry, either an ambient view or a game, held for dur ms.
type slot struct {
	dur     float64
	ambient *element.SpriteEl
	game    *highScore
}

// buildSchedule is the wordmark, the figure, then each game; a focused
// preview is its game alone.
func (a *app) buildSchedule() []slot {
	var sched []slot
	if !a.focused {
		sched = append(sched, slot{dur: slotMs, ambient: a.wordmark}, slot{dur: slotMs, ambient: a.figure})
	}
	for i := range a.scores {
		sc := &a.scores[i]
		sched = append(sched, slot{dur: gameSlotMs(sc), game: sc})
	}
	return sched
}

// gameSlotMs is slotMs, or long enough for the ticker to crawl every row
// once plus 2s of slack.
func gameSlotMs(sc *highScore) float64 {
	if len(sc.rest) == 0 {
		return slotMs
	}
	cycle := float64((len(sc.rest)*tickerPitch + tickerWindowH) * tickerMsPerPx)
	return max(slotMs, cycle+2000)
}

func (a *app) current() slot {
	if !a.haveSlot || a.cur >= len(a.schedule) {
		return slot{}
	}
	return a.schedule[a.cur]
}

// HandleInput is a no-op; the engine consumes input while attract is loaded.
func (a *app) HandleInput(clock.FrameTime) {}

// slotAt returns the schedule index showing at ms and the offset into it.
func (a *app) slotAt(ms float64) (idx int, rem float64) {
	rem = math.Mod(ms, a.scheduleTotal)
	for i, s := range a.schedule {
		if rem < s.dur {
			return i, rem
		}
		rem -= s.dur
	}
	return 0, rem
}

// driveAmbient paints the ambient views for this instant. firstLap marks the
// wordmark's first appearance, which does not fade up from black.
func (a *app) driveAmbient(ms, rem float64) {
	firstLap := ms < a.scheduleTotal
	switch s := a.current(); s.ambient {
	case nil:
	case a.wordmark:
		a.wordmark.SetOpacity(wordmarkOpacity(rem, s.dur, firstLap, a.bootSettle))
	default:
		s.ambient.SetOpacity(fadeEdges(rem, s.dur, ambientDim))
	}
	a.wordmark.SetData(wordmarkFrame(ms))
}

// Update drives rotation, ticker and chase as pure functions of engine time
// so golden frames reproduce.
func (a *app) Update(stats clock.FrameTime) {
	cur, rem := a.slotAt(stats.DurationMs)
	if !a.haveSlot || cur != a.cur {
		a.haveSlot = true
		a.cur = cur
		a.applySlot()
		a.board.start(stats.DurationMs - rem)
	}
	a.board.Advance(stats)

	a.driveAmbient(stats.DurationMs, rem)

	step := int(stats.DurationMs / chaseStepMs)
	if !a.haveChase || step != a.chaseStep {
		a.haveChase = true
		a.chaseStep = step
		a.advanceChase()
	}

	a.field.Advance(stats, a.ctx.RT.Rand)

	// Last, so a cartridge's own button lights override the chase.
	if a.attractR != nil {
		a.attractR.Attract(uint64(stats.ElapsedMs * 1000))
	}

	// After the cartridge, so solid green overrides it too.
	a.updateHoldFeedback()
}

// AttractGameSlug implements engine.AttractSlot: the slug a green-button hold
// launches, or "" on an ambient slot.
func (a *app) AttractGameSlug() string {
	if g := a.current().game; g != nil {
		return g.slug
	}
	return ""
}

// updateHoldFeedback is display only: while green is held on a game slot, the
// LCD shows the name over a filling bar and green lights solid. The engine
// decides the launch. When a hold ends unlaunched, LCD and chase are restored.
func (a *app) updateHoldFeedback() {
	frac := a.ctx.RT.AttractHoldFraction()
	if frac <= 0 {
		if a.holdShown {
			a.holdShown = false
			a.ctx.SetText(input.BothSeats, lcdLine1, lcdLine2)
			a.applyChaseLights()
		}
		return
	}
	g := a.current().game
	if g == nil {
		return
	}
	name := strings.ToUpper(g.name)
	a.ctx.SetText(input.BothSeats, name, holdBar(frac))
	a.holdShown = true
	for p := 0; p < 2; p++ {
		a.ctx.SetButtonLights(input.SeatsOf(input.SeatOf(p)), hardware.ButtonLights{greenButton: true}, true)
	}
}

// applyChaseLights re-asserts each side's current chase light without advancing it.
func (a *app) applyChaseLights() {
	for p := range a.lit {
		a.ctx.SetButtonLights(input.SeatsOf(input.SeatOf(p)), hardware.ButtonLights{hardware.LightButtons[a.lit[p]]: true}, true)
	}
}

// holdBar renders progress as '█' over a '░' track across the LCD width; both
// are custom CGRAM glyphs. frac is clamped to 0..1.
func holdBar(frac float64) string {
	const width = hardware.LcdCols
	fill := int(frac*float64(width) + 0.5)
	if fill < 0 {
		fill = 0
	}
	if fill > width {
		fill = width
	}
	return strings.Repeat("█", fill) + strings.Repeat("░", width-fill)
}

// advanceChase hops each side's lit button to a different random one, rolled
// independently. ctx.RT.Rand keeps it deterministic under test seeds.
func (a *app) advanceChase() {
	n := len(hardware.LightButtons)
	for p := range a.lit {
		a.lit[p] = (a.lit[p] + 1 + a.ctx.RT.Rand.Intn(n-1)) % n
		a.ctx.SetButtonLights(input.SeatsOf(input.SeatOf(p)), hardware.ButtonLights{hardware.LightButtons[a.lit[p]]: true}, true)
	}
}

// applySlot shows the current slot over the always-visible starfield: an
// ambient view alone, a cartridge's own attract renderer, or a game's logo,
// name, #1 line with trophy and the rank 2-10 ticker.
func (a *app) applySlot() {
	// Tear down the previous slot's renderer and restore the LCD text.
	if a.attractR != nil {
		a.attractR.Close()
		a.attractR = nil
		a.ctx.SetText(input.BothSeats, lcdLine1, lcdLine2)
	}
	a.wordmark.SetVisible(false)
	a.figure.SetVisible(false)

	cur := a.current()
	if cur.game == nil {
		a.hideShowcase()
		if cur.ambient != nil {
			cur.ambient.SetVisible(true)
		}
		return
	}
	sc := cur.game

	// A failed renderer falls through to the default showcase.
	if sc.newAttract != nil {
		if r, ok := sc.newAttract(a.ctx); ok {
			a.attractR = r
			// cart_init often sets a "PRESS A" title prompt, which is false
			// during attract; a renderer that wants the LCD rewrites it itself.
			a.ctx.SetText(input.BothSeats, lcdLine1, lcdLine2)
			a.hideShowcase()
			return
		}
	}
	// The score alone for an anonymous entry, as in ticker.setRows.
	scoreText := fmt.Sprintf("%d", sc.high)
	if sc.playerName != "" {
		scoreText = fmt.Sprintf("%s %d", strings.ToUpper(sc.playerName), sc.high)
	}
	a.name.SetText(strings.ToUpper(sc.name))
	a.score.SetText(scoreText)
	// The trophy sits left of the centred score line (mirroring the label's
	// alignment); a wide line slides the pair right rather than clip the cup.
	const trophyGutter = 10 // 8px cup + 2px gap
	half := (ui.FontLED{}.TextWidth(scoreText, 1) + 1) / 2
	left := 32 - half
	if left < trophyGutter {
		left = trophyGutter
	}
	a.score.SetOffsetX(left + half)
	a.trophy.SetOffsetX(left - trophyGutter)
	a.trophy.SetOffsetY(34)
	a.trophy.SetVisible(true)
	a.board.setRows(sc.rest)
	a.board.SetVisible(len(sc.rest) > 0)
	if sc.icon != nil {
		a.icon.SetData(sc.icon)
	}
	a.icon.SetVisible(sc.icon != nil)
	a.name.SetVisible(true)
	a.score.SetVisible(true)
}

func (a *app) hideShowcase() {
	a.icon.SetVisible(false)
	a.name.SetVisible(false)
	a.score.SetVisible(false)
	a.trophy.SetVisible(false)
	a.board.SetVisible(false)
}

func (a *app) Render(clock.FrameTime, engine.RenderReason) {}

func (a *app) Teardown() error {
	if a.attractR != nil {
		a.attractR.Close()
		a.attractR = nil
	}
	return nil
}

// readHighScores loads every game's top-10 table, skipping games with no
// score and no cartridge attract slot, ordered by orderByRecency.
func readHighScores(ctx *engine.AppContext) []highScore {
	var out []highScore
	for _, m := range engine.Apps() {
		if m.Type != engine.TypeGame {
			continue
		}
		table := scores.Load(ctx.RT.Store, m.Slug)
		best, ok := table.Best()
		// A cart_attract cartridge earns a slot even with an empty table.
		if !ok && m.NewAttract == nil {
			continue
		}
		var rest []scores.Entry
		if ok {
			rest = table.Entries[1:]
		}
		out = append(out, highScore{
			slug:       m.Slug,
			name:       m.Name,
			playerName: best.Name,
			high:       best.Score,
			rest:       rest,
			icon:       m.Icon,
			newAttract: m.NewAttract,
		})
	}
	return orderByRecency(out, ctx.RT.RecentSlugs(), ctx.RT.Rand)
}

// orderByRecency puts recently launched games first, most recent leading
// (recent is front-first), then shuffles the rest into a tail. A nil recent
// shuffles everything.
func orderByRecency(games []highScore, recent []string, rnd *rand.Rand) []highScore {
	rank := make(map[string]int, len(recent))
	for i, slug := range recent {
		rank[slug] = i
	}
	var known, rest []highScore
	for _, g := range games {
		if _, ok := rank[g.slug]; ok {
			known = append(known, g)
		} else {
			rest = append(rest, g)
		}
	}
	sort.Slice(known, func(i, j int) bool { return rank[known[i].slug] < rank[known[j].slug] })
	rnd.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
	return append(known, rest...)
}
