# ui reference

`ui` (`github.com/stealthmonkey99/arkade/sdk/ui`, at `sdk/ui`) is
the widget toolkit **shared by the cabinet engine and every
cartridge** — the same `Menu`, the same fonts, the same blend math.
Drawing through it means your game speaks the cabinet's native visual
vocabulary, and renders identical bytes on both hosts. This page is the
tour: which widget to reach for and how they fit together. The
symbol-by-symbol reference is generated from the doc comments into
[api/ui.md](api/ui.md), [api/ui-input.md](api/ui-input.md) and
[api/fsm.md](api/fsm.md); `go doc` and pkg.go.dev show the same comments.

The package is host-free by construction: no `init()`, no clock, no
goroutines, no `math/rand`; it imports nothing but `math` and its own `sdk/ui/input`. Everything
draws onto an abstract `Canvas`, and a `*cartlib.Framebuffer` *is* a
`Canvas`, so every widget composites straight into your frame:

```go
package snippet

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

func render(fb *cartlib.Framebuffer) {
	fb.Clear(0, 0, 0)
	ui.Label{X: 1, Y: 1, Text: "SCORE", Color: ui.White}.Draw(fb)
}
```

Depending on it takes its own `require` + `replace` pair in your
`go.mod` — replace directives are not transitive, so cartlib's isn't
enough ([Getting started](getting-started.md#1-copy-the-template)).

## Canvas, Color, and primitives

[Canvas](api/ui.md#type-canvas) is two methods — `Bounds` and `Blend` — which is why anything can be one: a
cartridge framebuffer, an engine sprite, an off-screen `Layer`, or an
`image.RGBA` adapter in an art-generator tool. `Blend` is straight-
alpha source-over via `ui.BlendOver`, the single blend formula both
hosts share; out-of-bounds coordinates are silently ignored.

[Color](api/ui.md#type-color) is straight-alpha RGBA. The zero value is
`Transparent` (drawing it is a no-op); `A: 255` is an opaque overwrite.

Free-function primitives over any Canvas:

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/ui"

func primitives(cv ui.Canvas) {
	c := ui.Color{R: 57, G: 255, B: 143, A: 255}
	ui.Set(cv, 3, 3, c)                // one pixel
	ui.FillRect(cv, 10, 10, 8, 8, c)   // filled rectangle
	ui.StrokeRect(cv, 10, 10, 8, 8, c) // 1px outline
	ui.HLine(cv, 0, 63, 32, c)         // horizontal run, inclusive ends
	ui.VLine(cv, 32, 0, 63, c)
	ui.Line(cv, 0, 0, 63, 40, c)       // integer Bresenham — bit-identical everywhere
}
```

## Fonts

Two fonts, both taking a `scale` multiplier and both usable as a
[Label](api/ui.md#type-label)'s `Font`. [FontLED](api/ui.md#type-fontled)
is the engine's variable-width LED font, full printable ASCII; it is what
the cabinet's own menus and labels use. [Font3x5](api/ui.md#type-font3x5)
is a tiny fixed-width font for scores and dense HUDs (digits and
capitals), with `DrawUint` to draw a number without a string allocation.

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/ui"

func fonts(cv ui.Canvas) {
	led := ui.FontLED{}
	w := led.TextWidth("READY", 1)
	led.DrawText(cv, (64-w)/2, 20, "READY", 1, ui.White) // centered

	small := ui.Font3x5{}
	small.DrawUint(cv, 1, 58, 4200, 1, ui.Color{R: 255, G: 176, B: 32, A: 255})
}
```

Mind the glyph coverage: `Font3x5` silently draws nothing for
characters it lacks (`&`, `/`, most punctuation) — verify every
literal string, or hand-plot a missing glyph locally.

## Display widgets

Plain values — configure fields, call `Draw(cv)`. The set is
[Label](api/ui.md#type-label), [Rect](api/ui.md#type-rect),
[Circle](api/ui.md#type-circle), [Border](api/ui.md#type-border),
[LineSeg](api/ui.md#type-lineseg), [LoadingBar](api/ui.md#type-loadingbar),
[Splash](api/ui.md#type-splash) (what the default load screen uses),
[AttractPanel](api/ui.md#type-attractpanel) and
[Sprite](api/ui.md#type-sprite); the last two are covered below.

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/ui"

func widgets(cv ui.Canvas) {
	ui.Rect{X: 4, Y: 4, W: 20, H: 12, Fill: ui.Color{R: 16, G: 53, B: 34, A: 255},
		Stroke: ui.Color{R: 57, G: 255, B: 143, A: 255}, StrokeW: 1}.Draw(cv)
	ui.Circle{CX: 40, CY: 10, R: 6, Fill: ui.Color{R: 255, G: 176, B: 32, A: 255}}.Draw(cv)
	ui.LoadingBar{X: 8, Y: 50, W: 48, H: 5, Progress: 0.65,
		Fill: ui.White, Border: ui.Color{R: 107, G: 114, B: 128, A: 255}}.Draw(cv)
}
```

### Score panels

`ui.AttractPanel` is the shared score block: a `Title`, a `Top`
row with the trophy, and a `Board` of the ranks below it, all stacked
10px apart inside `X, Y, W, H`. Despite the name it is not attract-only
— it draws strictly inside its own rectangle and never clears, so an
in-game HUD can drop a one-row panel in a corner just as an attract
screen puts a five-row one down the side.

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/ui"

var atElapsed uint64 // accumulated dtMicros, the caller's own clock

func scores(cv ui.Canvas, dtMicros uint64, top ui.AttractScore, board []ui.AttractScore) {
	atElapsed += dtMicros
	ui.AttractPanel{
		X: 13, Y: 6, W: 51, H: 52,
		Title: "SNAKE", Top: top, Board: board,
		Trophy:      true,
		Color:       ui.Color{R: 210, G: 210, B: 210, A: 255},
		AccentColor: ui.Color{R: 255, G: 120, B: 40, A: 255},
		T:           atElapsed, // enables the scroll cycle
		Ranks:       true,      // number every row
	}.Draw(cv)
}
```

**Height** is the caller's: pass one entry for a bare high-score line or
ten for a full board, and size `H` to taste — rows that would fall
outside `[Y, Y+H)` are skipped whole, never half-drawn.

**Width** is measured per row before anything is drawn. A row that fits
is drawn motionless — the common case, and the panel must not twitch
when it has no reason to. A row that does *not* fit either truncates the
name (keeping the score whole) or, if you gave it a clock, scrolls:

| `T` | An overflowing row |
| --- | --- |
| `0` (unset) | Truncates the name; the score is always drawn in full |
| your `dtMicros` accumulator | Holds on the name 1.2s, eases left 0.55s, holds on the score 1.2s, eases back — a 3.5s cycle |

`T` is the one field to add to make an existing panel animate, and
leaving it off keeps today's pixels exactly. It must be **accumulated
`dtMicros`, never a wall clock** — `Draw` is a pure function of the
struct, which is what keeps golden frames reproducible.

`Ranks` numbers every row in a uniform left gutter. The rank token and
the trophy share that gutter rather than competing for it: rank 1's
token *is* the cup when `Trophy` is set. The gutter is sized once from
the widest numeral in the stack, so a board running to rank 10 still
lines its names up. `FirstRank` renumbers the stack for a panel showing
a window onto a longer board (one rank at a time, say). Budget for it:
the gutter costs 10px beside a trophy, 6px without one, which is real
money on the narrowest score panel (40px).

### Attract

`ui.Attract` drives the attract-mode phase cycle — a panel for a
while, then a silent demo, back to the panel, forever — which a
cartridge would otherwise hand-roll as an `atInited` bool, a phase int,
and an elapsed-in-phase accumulator zeroed by hand at every transition. It
is explicitly *not* an FSM — the duplication is across cartridges, not
complexity within one — so this is a driver, not `sdk/fsm`.

```go
package snippet

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

const (
	attractPanelUS uint64 = 8_000_000
	attractDemoUS  uint64 = 16_000_000
)

// Built ONCE, like Menu.Items.
var atAttract = ui.Attract{Phases: []ui.AttractPhase{
	{ // panel
		DurationMicros: attractPanelUS,
		OnEnter:        loadAttractScores, // re-reads the score table every time
		Draw:           func(_ uint64, cv ui.Canvas) { drawAttractPanel(cv) },
	},
	{ // demo
		DurationMicros: attractDemoUS,
		OnEnter:        startAttractDemo, // arms `silent`, seeds a local RNG stream
		Draw:           func(_ uint64, cv ui.Canvas) { drawAttractDemo(cv) },
	},
}}

func attract(dtMicros uint64, fb *cartlib.Framebuffer) {
	atAttract.Update(dtMicros)
	// An early exit ends the demo like a natural timeout would — guard
	// on Phase() so it can't double-advance a tick that already timed out.
	if atAttract.Phase() == 1 && demoIsOver() {
		atAttract.Next()
	}
	fb.ClearTransparent() // overlay on the starfield
	atAttract.Draw(fb)
}

func loadAttractScores()            {}
func drawAttractPanel(cv ui.Canvas) {}
func startAttractDemo()             {}
func drawAttractDemo(cv ui.Canvas)  {}
func demoIsOver() bool              { return false }
```

A phase's `DurationMicros` is the usual `attractPanelUS`/`attractDemoUS`
— when it elapses, `Update` advances to the next phase in `Phases`
(wrapping past the last back to the first) and fires the new phase's
`OnEnter`, exactly once. `DurationMicros: 0` means the phase never times
out on its own — the cartridge calls `Next` (fixed order) or `Goto`
(picks a specific phase) when it decides the phase is over, which is how
an early-exit demo (`gameOver()`, `!alive`) or a beat whose NEXT phase
depends on data rather than a fixed order (a memory game's grown-sequence loop) is expressed.

`OnEnter` generalizes a hand-written `startAttractDemo()` /
`loadAttractScores()` pair into one mechanism: it also fires once, lazily,
for `Phases[0]` before the very first `Update` or `Draw` call, so a
converted cartridge needs no `atInited` bool of its own. It is also where
a demo that replays live gameplay code arms its own `silent` flag and
swaps in a local deterministic RNG stream (a small splitmix generator) — `ui` cannot import `cartlib` and so cannot gate a
cartridge's sound calls itself, but every cartridge gets the same one
hook to do it in instead of a bespoke function. A demo drawn entirely
from scratch (never calling a sound-producing function at all) is silent
by construction and needs no flag.

`Elapsed()` replaces a hand-kept `atT`, minus the hand-zeroing at every
transition — a phase's `Draw` callback receives the same value, so a
sway/bob animation keyed off it (`sway := int(atT/700_000) % 4`, a common
idiom) needs no change. `Progress()` normalizes it to `[0, 1]`
against the phase's own `DurationMicros` (0 always reports 0) — the
generalization of a fill-in/fade animation keyed to "how far through
this beat am I" (a hand-rolled
`frac := float64(t) / float64(attractFillWindowMicros)`).

### Sprites and masks

`ui.Sprite` is gameplay art: a `W×H` pixel string plus a `Mask` of its
collidable body. Author sprites as PNGs under `assets/` and lift them
with `Sheet.Cel` at Init (or compile the literals in yourself);
`ui.OpaqueMask`'s alpha rule (only `A == 255` pixels enter the mask)
makes anti-aliased edges automatically forgiving.
`Mask.Overlaps` is pixel-perfect mask-vs-mask collision. The full
pipeline: [patterns](patterns.md#sprite-art-as-pngs).

### Button hints

**Never draw a button as a letter.** The cabinet's buttons are round,
coloured, and carry no writing at all — a player at the tabletop sees a
green button and a red one, never an "A" or a "B", so `PRESS A` names a
control they cannot find on the machine. The platform identifies a button
two ways only: **by its colour**, and **by being lit**.

On the LED panel, draw the button itself:
[ButtonDot](api/ui.md#type-buttondot) is one cabinet button,
[StickHint](api/ui.md#type-stickhint) the joystick,
[Chevron](api/ui.md#type-chevron) one arrow for a "more this way" cue,
[AuxDot](api/ui.md#type-auxdot) the stacked aux pair, and
[HintRow](api/ui.md#type-hintrow) a strip of hints with labels.

`ui.ButtonGreen`, `ButtonRed`, `ButtonYellow`, `ButtonBlue` and
`ButtonAux` are the panel's actual button colours; use them rather
than mixing your own, or your hint will not match the button under the
player's hand. These rasters and metrics are pinned pixel-for-pixel
against the shell's own hints, so a cartridge hint and a cabinet hint
are the same picture.

Both aux buttons are white, so `AuxDot` distinguishes them by
**position** instead: two small discs stacked the way the hardware is,
the referenced one filled and the other a dim outline.

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/ui"

// Built ONCE — Hints is caller-owned, like Menu.Items. Never build a
// slice inside a render function.
var footer = ui.HintRow{
	Y:          56,
	LabelColor: ui.Color{R: 180, G: 180, B: 180, A: 255},
	Hints: []ui.Hint{
		{Text: "PLAY", Color: ui.ButtonGreen},
		{Text: "AIM", Color: ui.White, Control: ui.HintStick},
		{Text: "BACK", Color: ui.ButtonRed},
	},
}

func hints(cv ui.Canvas) {
	footer.X = (64 - footer.Width()) / 2 // centred on the panel
	footer.Draw(cv)

	// One hint, no slice at all: DrawHint returns where the next would go.
	ui.DrawHint(cv, 1, 1, ui.White, ui.Hint{Text: "FAST", Control: ui.HintAux2})
}
```

The 16×2 LCD is a different problem — it is monochrome text, so it can
draw neither a disc nor a colour. There, **name the action and let the
lit lamp identify the button**: light the eligible button with
`cartlib.SetButtonLights` and let the LCD say `OK` or `DEAL`. Spell a
colour word (`GREEN = START`) only where no lamp is available. Never a
letter, on either surface.

## Interactive widgets

Interactive widgets speak an abstract vocabulary — `ui.Direction`
(`DirUp/Down/Left/Right`) and `ui.Action` (`ActionActivate`,
`ActionBack`) — through `Move`/`Activate`/`Do`, so the same widget runs
under the engine and a cartridge. The input record lives in
`sdk/ui/input` (cartlib re-exports it as `cartlib.InputRecord`), and its
methods do the decoding: `r.PressDir()` is the direction pressed,
counted once per push, and `r.IsPress()` is the canonical press.
`HoldConfirm` and `HeldDirs` take records directly through `Feed`.

```go
package snippet

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// The standard decode: direction edges from PressDir (a real stick's
// duplicate keypress already dropped), actions from the canonical press.
func decode(r cartlib.InputRecord) (ui.Direction, ui.Action) {
	if d := r.PressDir(); d != ui.DirNone {
		return d, ui.ActionNone
	}
	if r.IsPress() {
		switch r.Key {
		case cartlib.KeyJoyA, cartlib.KeyEnter, cartlib.KeySpace:
			return ui.DirNone, ui.ActionActivate
		case cartlib.KeyJoyB, cartlib.KeyEscape:
			return ui.DirNone, ui.ActionBack
		}
	}
	return ui.DirNone, ui.ActionNone
}
```

### Menu

A vertical list of selectable items built on `Button` +
`FocusManager`. The standard geometry — earned across seven
shipped-then-fixed menus — is `ItemH: 10, Spacing: 1, TextDY: 1`:

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/ui"

type title struct {
	menu *ui.Menu
}

func newTitle() *title {
	m := ui.NewMenu([]string{"1P GAME", "2P GAME", "SETTINGS"})
	m.X, m.Y, m.W = 8, 24, 48
	m.ItemH, m.Spacing, m.TextDY = 10, 1, 1 // the standard menu geometry
	return &title{menu: m}
}

func (t *title) move(d ui.Direction)  { t.menu.Move(d) }         // clamped, no wrap
func (t *title) confirm() int         { return t.menu.Activate() } // selected index
func (t *title) tick(dtMicros uint64) { t.menu.Update(dtMicros) }
func (t *title) draw(cv ui.Canvas)    { t.menu.Draw(cv) }
```

Set `H` (viewport height) only for genuinely long lists — it opts in
to animated scrolling; for four items or fewer, fit with geometry
instead. `Button` is the single-item version, and `FocusManager` the
bare selection engine (`Reset`/`ResetGrid` for lists and grids,
`Move`, `Focused`) when your "menu" is a board, not a list.

### NameEntry

The high-score initials widget, wired for the recent-name flow:

```go
package snippet

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

type entry struct {
	ne   *ui.NameEntry
	seat cartlib.Seat
}

func newEntry(seat cartlib.Seat) *entry {
	ne := ui.NewNameEntry(14, 30, 3) // x, y, max length
	var buf [cartlib.MaxNameLen]byte
	if name, ok := cartlib.RecentName(seat, buf[:]); ok {
		ne.SetName(string(name)) // returning player: accept with one press
	}
	ne.SetRecents(cartlib.RecentNames(seat)) // the seat's regulars, below the charset
	return &entry{ne: ne, seat: seat}
}

func (e *entry) input(d ui.Direction, a ui.Action, score int) {
	e.ne.Move(d) // level-triggered: call each frame with the HELD direction
	e.ne.Do(a)   // edge-triggered: once per press
	if e.ne.Done() {
		cartlib.ScoreSubmit(score, e.ne.Name())
		cartlib.NoteRecentName(e.seat, e.ne.Name())
	}
}

func (e *entry) tick(dt uint64)     { e.ne.Update(dt) } // drives blink & hold-repeat
func (e *entry) draw(cv ui.Canvas)  { e.ne.Draw(cv) }
```

`Move` is level-triggered (poll the held direction every frame — pair
it with `HeldDirs`); `Do` is edge-triggered.

### HoldConfirm

The platform-standard mid-game exit flow: B opens the overlay, release
arms it, press-and-hold ~1 s confirms, and the confirm fires only on
release (so the shell never inherits a held B). `Open` it on the quit
press, then hand it every record with `Feed` — keydown and keyup
included, so before any keypress-only filter. `Feed` reports false for
anything but a quit key, and a press among those is the cancel:

```go
package snippet

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

type playing struct {
	confirm ui.HoldConfirm
}

// OwnsQuitKeys stops cartlib answering B with an immediate Exit.
func (p *playing) OwnsQuitKeys() bool { return true }

func (p *playing) input(r cartlib.InputRecord) {
	if p.confirm.Active() {
		if !p.confirm.Feed(r) && r.IsPress() {
			p.confirm.Close() // any other press cancels
		}
		return
	}
	if r.IsPress() && r.IsQuit() {
		p.confirm.Open()
		p.confirm.Feed(r)
	}
}

func (p *playing) tick(dt uint64) {
	if p.confirm.Active() && p.confirm.Update(dt) {
		cartlib.Exit()
	}
}
```

Draw it last, over the frozen game. Opened over a `NameEntry`, call
`confirm.Avoid(entry.LetterBand())` right after `Open`, so the panel
moves off the initials; `Open` forgets the band, so an overlay raised
later mid-game is centred again. On seat-rotated boards set
`QuarterTurns` to counter the display rotation so the prompt stands
upright. Defaults (1 s hold, "QUIT GAME?" title) are the tuned standard —
don't retune per game.

### HeldDirs

The one sanctioned way to track held directions. Terminal keyboards
emit *only* keypress records (no keyup — a tty can't see releases), so
any hand-rolled keydown/keyup tracker ships broken; `HeldDirs` fuses
real keydown/keyup, joystick position levels, and a 250 ms credit per
virtual keypress. Feed it every record:

```go
package snippet

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

type mover struct {
	held ui.HeldDirs
}

func (m *mover) input(r cartlib.InputRecord) {
	m.held.Feed(r) // all kinds, before any keypress-only filter
}

func (m *mover) tick(dt uint64) {
	m.held.Update(dt)
	if m.held.Held(ui.DirLeft) {
		// steer left this frame
	}
}
```

`Feed` treats a diagonal stick position as no direction. A game that
wants diagonals to count calls `SetJoy` itself after `Feed`, with its
own mapping.

### HoldRepeat

A held stick that should *step* a list, not slew it, uses `HoldRepeat`:
the press edge is the caller's own step, then nothing for 220 ms, then
one step every 120 ms — the cabinet's own menus' cadence. `Hold` takes
the stick level (`DirNone` releases), `Update(dt)` returns how many steps
are due this frame, and `Held()` says which way:

```go
package snippet

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

type list struct {
	menu ui.Menu
	hold ui.HoldRepeat
}

func (l *list) input(r cartlib.InputRecord) {
	if r.Kind == cartlib.KindJoystick {
		l.hold.Hold(r.Dir())
	}
	if d := r.PressDir(); d != ui.DirNone {
		l.menu.Move(d) // the press edge is the first step
	}
}

func (l *list) tick(dt uint64) {
	for n := l.hold.Update(dt); n > 0; n-- {
		l.menu.Move(l.hold.Held())
	}
	l.menu.Update(dt)
}
```

`Reset` it when the screen is re-entered: a release while the screen
was inactive was never seen, and the stale hold would step on resume.
`FastAfter` and `FastIntervalMicros` speed a long hold up, the way the
cabinet's shelf browser does after twelve steps.

## Layers and compositing

[Layer](api/ui.md#type-layer) is an off-screen Canvas. Two big uses:
*persistent* surfaces (allocate in `Init`, never clear — draws
accumulate into trail/smear effects), and **host-Go render tests** —
because your draw code targets `ui.Canvas`, a plain `go test` can
render any state into a `Layer` and assert pixels, no TinyGo, no
wasm.

[Compositor](api/ui.md#type-compositor) flattens up to `MaxLayers`
layers in z-order, with per-layer opacity for fades and dimmed
backgrounds; `AddLayer` returns the `*Layer`, which is the handle: set
its `Opacity` directly. [Fade](api/ui.md#type-fade) is a dt-driven 0→1
opacity ramp for anything that needs one number animated.

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/ui"

type trails struct {
	comp  *ui.Compositor
	trail *ui.Layer
}

func newTrails() *trails {
	c := ui.NewCompositor(64, 64)
	bg := c.AddLayer(1.0)
	bg.Fill(ui.Black)
	trail := c.AddLayer(0.85) // never cleared: draws accumulate
	return &trails{comp: c, trail: trail}
}

func (t *trails) frame(cv ui.Canvas, x, y int) {
	ui.FillRect(t.trail, x, y, 2, 2, ui.White)
	t.comp.Composite(cv)
}
```

## Screen state (`sdk/fsm`)

`fsm` (`github.com/stealthmonkey99/arkade/sdk/fsm`, at `sdk/fsm`) is
a separate small module — not part of `ui`, because it draws nothing —
and it is documented here because this is where readers come looking for
"which screen is up".

It does **not** dispatch. `switch m.State()` stays exactly the switch
every cartridge already writes, and guards stay in the code that decides
them. What the machine owns is the variable itself and the obligations
that hang off *entering* a screen:

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/fsm"

const (
	stTitle fsm.State = iota
	stPlay
	stOver
	stConfirm
)

type game struct{ machine fsm.Machine }

func (g *game) Init() {
	g.machine.OnEnter(g.enterScreen)
	g.machine.Reset(stTitle)
}

// enterScreen runs on EVERY arrival — Go, Push, Pop and Reset alike —
// which is what puts a covered screen's LCD and lamps back when a modal
// lifts. It is also the constraint: work that must not be redone on a
// resume belongs at the call site, not here.
func (g *game) enterScreen(prev, next fsm.State) {
	switch next {
	case stTitle:
		showTitlePeripherals()
	case stPlay:
		showPlayPeripherals()
	case stOver:
		showGameOverPeripherals()
	case stConfirm:
		showConfirmPeripherals()
	}
}

func showTitlePeripherals()    {}
func showPlayPeripherals()     {}
func showGameOverPeripherals() {}
func showConfirmPeripherals()  {}
```

The whole API is [Machine](api/fsm.md#type-machine): `Reset` places the
machine and fires the entry hook (`Init` calls it); `Go` replaces the
screen on top, and its argument must be a **constant**; `Push` layers a
modal over it and `Pop` dismisses it, re-entering the covered screen with
its clock resumed; `State`, `Under` and `In` say what is showing, what a
modal is drawn over, and whether a screen is anywhere on the stack; `Tick`
and `Elapsed` are the per-screen clock, zeroed on entry and frozen under a
modal; `OnEnter` and `OnExit` are the hooks (most cartridges skip
`OnExit`); `Edges` and `Blocked` are the optional legal-transition table
and the counter a test pins at zero.

`MaxDepth` is 4 live entries: a base screen plus three overlays. No imports,
no allocation ever, 80 bytes per machine on wasm32.

**What it buys.** Four bug classes, each of them easy to ship more
than once: a screen entered without its peripherals re-asserted (there is
no variable to assign, and every arrival runs the hook); a modal resumed
to the wrong screen (`Pop` takes no argument — the answer is the stack);
a per-screen clock left unzeroed; and an undeclared transition, which is
refused, counted, and caught by a test rather than discovered by a
player.

**What it does not buy: brevity.** Adopting it rarely makes a cartridge
shorter — the edge table has no predecessor, and promoting a boolean
sub-state to a real screen costs lines to write, which is exactly why
so many cartridges spell it as a boolean instead, and exactly why that
keeps going wrong. Adopt this for the bug classes it prevents, not for
brevity.

**When not to.** Below roughly six screens plus a modal it is a wash.
`sdk/fsm` is opt-in, nothing in the SDK requires it, and a plain
`uint8` enum and a `switch` is a perfectly good answer at that size.
The tutorial's Catcher uses it at five screens *to teach the pattern*,
and says so.

**Arrays of entities.** `OnEnter`'s hook doesn't say WHICH machine called
it, which is fine for a screen (there is only one) and not for four
jellies or a pool of cars sharing one machine definition. `IDMachine` is
`Machine` plus a recorded id and an `OnEnterID(fn func(id uint8, prev,
next fsm.State))` hook for exactly that case — everything else about it
reads identically to `Machine`'s. A singleton never needs it.
[`sdk/fsm/README.md`](../fsm/README.md#arrays-of-entities-idmachine) has
the details.

Depending on it takes its own `require` + `replace` pair, like `ui`.
[`sdk/fsm/README.md`](../fsm/README.md) has the five-step adoption
recipe and the design's list of things it deliberately refuses to be.

## Sizing note

Changing anything in `sdk/ui` re-renders **every cartridge that draws
with it** — cartridge golden tests pin exact pixels, so a shared-
widget change means rebuilding every cartridge and rebaselining their
goldens. Budget for
that before "just tweaking" a widget.
