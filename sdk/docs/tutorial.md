# Tutorial: build a game, start to finish

[Getting started](getting-started.md) gets a cartridge building and
playing in about ten minutes. This is the other thing: one small,
complete game, built from the starter template to a packed `.rcart`,
using the whole toolchain on the way — the sprite editor, the sound
designer, the hot-reloading dev server, the flight recorder, golden
tests and preflight.

It comes in four parts:

1. **This page** — design the game and write it. Falling fruit, a
   catcher you steer, lives, a score, screens, the cabinet's LCD and
   button lights.
2. **[Sprites and sound from scratch](tutorial-assets.md)** — draw the
   art in spritelab, synthesize the effects and a music loop in
   soundlab, and wire both into the build.
3. **[Waves you draw by hand](tutorial-levels.md)** — author the opening
   of a run as a level pack in levellab, and generate it through a hook
   that refuses a wave the game cannot play.
4. **[Play, record, replay, ship](tutorial-debug.md)** — iterate
   from the terminal, record a session and
   replay it, write a golden test, run preflight, and pack the bundle.

Read [Getting started](getting-started.md) first if you have not; this
picks up where it leaves off and does not repeat it.

## The game

**Catcher.** Fruit falls from the top of a 64×64 panel. You walk left
and right along the bottom with the stick and catch it in a basket.
Miss three and the run is over. The blue button gives you a short dash
for the ones you cannot walk to.

```panel
tutorial/testdata/golden/catcher-title.txt
tutorial/testdata/golden/catcher-playing.txt
tutorial/testdata/golden/catcher-wave.txt
tutorial/testdata/golden/catcher-gameover.txt
tutorial/testdata/golden/catcher-nameentry-confirm.txt
```

*The finished game, one screen at a time: title, a run, an authored wave, game over, initials.*

That is the whole design, and it is deliberately that small — because
what is *not* small is how much of the system it touches:

| It needs | So you meet |
| --- | --- |
| A character that walks | An animation strip drawn in spritelab, `ui.Sprite`, a cel clock |
| Fruit in random columns | `cartlib.RandomU64` and the determinism rules that make it replayable |
| A catch and a miss | Fixed-step simulation, collision, and pure logic you can unit-test |
| Feedback | Synthesized SFX, a music loop, the per-seat LCD, the button lights |
| An ending | Screens, the shared score table, name entry, the hold-B exit |
| Not shipping it broken | Golden frames, a determinism check, replay, preflight |

The finished cartridge is checked in at **`sdk/tutorial/`** in the
engine checkout. Every listing below is that directory's real source —
a test compares them byte for byte — so when a step misbehaves you have
something exact to diff against.

### Where this cartridge lives

You will build in a **cartridge library of your own**: a folder beside
the engine checkout, not inside it. The public cartridge collection is
its own repository, and what you are building is a personal library —
not a pull request — that you can publish from later. Any name works;
each subfolder is one cartridge, which makes the folder a *cartridge
root* the engine's tools can be pointed at:

```text
arkade/                 the engine checkout
my-cartridges/          your library — a cartridge root
my-cartridges/catcher/  this tutorial
```

Every path on these pages follows from that layout: the engine is
`../../arkade` from inside the cartridge, and the cartridge is
`../my-cartridges/catcher` from inside the engine. If yours differs,
those two are the only things that change.

## Before you start

Everything from [Getting started's prerequisites](getting-started.md#0-prerequisites):
the TinyGo and Go pair it names, and an engine checkout. From the engine
checkout:

```sh
mkdir -p ../my-cartridges
cp -r sdk/template ../my-cartridges/catcher
cd ../my-cartridges/catcher
```

Then, exactly as [Getting started](getting-started.md#1-copy-the-template)
describes, make it yours — four edits, in this order.

**1. `go.mod`** — rename the module and point the two `replace` lines
at the engine checkout. The SDK modules are not published or tagged
yet, so a local checkout plus these lines is how a cartridge finds them
today; both lines are needed even though only `cartlib` is imported
directly, because `replace` directives are not transitive:

```go-mod file=go.mod
module arkade-catcher-cartridge

go 1.23

require github.com/stealthmonkey99/arkade/sdk/cartlib v0.1.0
require github.com/stealthmonkey99/arkade/sdk/ui v0.1.0

replace github.com/stealthmonkey99/arkade/sdk/cartlib => ../../arkade/sdk/cartlib
replace github.com/stealthmonkey99/arkade/sdk/ui => ../../arkade/sdk/ui
```

**2. `manifest.json`** — `slug` must be `catcher`, matching the directory
name and the eventual `.rcart` filename:

```json file=manifest.json
{
  "schemaVersion": 1,
  "abi": 1,
  "slug": "catcher",
  "title": "Catcher",
  "publisher": "arkade",
  "author": "your name",
  "version": "0.1.0",
  "description": "Catch the falling fruit before it hits the ground",
  "tags": ["action"],
  "players": ["1p"],
  "audience": "all-ages",
  "capabilities": [],
  "visibility": "public",
  "icon": "assets/icon.png",
  "links": { "source": null }
}
```

**3. `Makefile`** — one line: `ARKADE ?= ../../arkade`, the path to the
engine checkout. Nothing else in it needs editing, now or later: it
reads `slug` out of `manifest.json` to name its outputs
(`catcher.wasm`, `catcher.rcart`) and rebuilds on any `.go` file.

**4. Delete** the template's `template.wasm` (its own checked-in build),
its `main.go`, and its `assets/` directory. You are going to write
`main.go` from scratch, and every asset — including the shelf icon the
manifest already names — gets drawn in [part 2](tutorial-assets.md).
`manifest_gen.go` can stay: `make build` regenerates it from your
`manifest.json` before every compile. `MANUAL.md` and `manual-icon.png`
stay too, still the template's, until [part 4](tutorial-debug.md#preflight)
makes them Catcher's.
Until then a missing icon is a warning from the loader and from
`rcartpack`, never an error.

## Step 1 — a catcher you can walk

The first version has no fruit at all. It draws a green rectangle on the
ground and lets you push it around, which is enough to meet four things
every cartridge does: registration, the fixed-step clock, held-direction
input, and drawing.

**`main.go`** is the smallest of the five files and the one with the
most rules attached to it.

```go file=main.go step=1
// SPDX-License-Identifier: GPL-2.0-or-later

// Command catcher is the game the Årkade SDK tutorial builds: fruit
// falls, you walk under it, you catch it.
package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

func (g *catcherGame) Manifest() []byte { return manifestBytes }

// game is the one instance, registered at the PACKAGE level — not from
// inside main, which is never called under the ABI's reactor hosting
// model.
var game = &catcherGame{}
var _ = cartlib.Run(game)

// main is required by TinyGo's package-main convention and is never
// invoked. Keep it empty.
func main() {}
```

Three things there are not stylistic:

- **Registration happens at the package level**, in
  `var _ = cartlib.Run(game)`, never inside `main`. A cartridge is
  hosted as a wasm *reactor*: the host calls the exported `cart_*`
  functions and `main` is never invoked as a program entry point. The
  only pre-`Init` code that takes effect at all is what TinyGo's
  compile-time interpreter can fold into static data, which a simple
  package-level assignment is. See [Registration and the reactor
  model](cartlib.md#registration-and-the-reactor-model).
- **`Manifest` returns `manifestBytes` and nothing else.** That slice,
  and the `manifestJSON` constant behind it, live in `manifest_gen.go`,
  which `make build` generates from `manifest.json` before every
  compile. The build's mandatory `wasmfix` pass rewrites `cart_manifest`
  into a genuine two-result export and can only do that when both values
  resolve to constants; anything else fails the build loudly. See [The
  Manifest method](cartlib.md#the-manifest-method).
- **There is no `func init()`, and `main` stays empty.** Setup belongs in
  `Game.Init`.

So the running cartridge reports exactly what `manifest.json` says —
`title` as `name`, the player modes as counts — and you never write the
runtime manifest by hand. Commit `manifest_gen.go` with the rest of the
source; preflight's `manifest` check fails a cartridge whose copy has
fallen behind the file.

**`catcher.go`** is the game's rules, and it is the only file that
imports nothing at all:

```go file=catcher.go step=1
// SPDX-License-Identifier: GPL-2.0-or-later

package main

// The simulation. It reaches for no host call, so plain `go test` covers
// every rule in it.

const (
	// The panel. cartlib.FBWidth/FBHeight say the same thing, but this
	// file does not import cartlib.
	fbW = 64
	fbH = 64

	catcherW    = 7        // the catcher's width
	catcherH    = 9        // ...and height
	catcherY    = fbH - 12 // 52: their top edge, feet at 60
	catcherMaxX = fbW - catcherW

	// One sim step is 25ms (40 Hz), independent of the host's ~30 fps
	// frame rate. Update accumulates dtMicros and runs whole steps, so
	// the game plays identically on a host that stutters — and, more
	// importantly, reproducibly.
	stepMicros = 25_000

	walkStep = 2 // pixels per step walking
	dashStep = 5 // pixels per step dashing
	// A dash is short and its cooldown is long: it is the answer to one
	// fruit you cannot walk to, not a faster way to move.
	dashSteps         = 8  // 200ms
	dashCooldownSteps = 56 // 1.4s after it ends
)

// sim is the whole game state. Everything the player can see is derived
// from these fields and nothing else — no clock, no hidden randomness.
type sim struct {
	x        int // the catcher's left edge
	dashLeft int // steps of dash remaining
	dashCool int // steps until the next dash is allowed
}

// reset puts a fresh run on the board. Assigning a whole struct literal
// (rather than zeroing fields one at a time) means a field added later
// cannot be forgotten here.
func (s *sim) reset() {
	*s = sim{x: (fbW - catcherW) / 2}
}

// dashReady reports whether pressing the blue button would do something
// right now. Step 3 lights the button on exactly this answer.
func (s *sim) dashReady() bool { return s.dashLeft == 0 && s.dashCool == 0 }

// startDash begins a dash if one is available, reporting whether it did
// so the caller can respond accordingly.
func (s *sim) startDash() bool {
	if !s.dashReady() {
		return false
	}
	s.dashLeft = dashSteps
	return true
}

// step advances the simulation by exactly one stepMicros tick. dir is
// -1, 0 or +1 — the held direction, resolved by the caller.
func (s *sim) step(dir int) {
	// A dash overrides the walk speed and, when it ends, starts its own
	// cooldown; the cooldown only ticks down while not dashing.
	move := walkStep
	if s.dashLeft > 0 {
		move = dashStep
		s.dashLeft--
		if s.dashLeft == 0 {
			s.dashCool = dashCooldownSteps
		}
	} else if s.dashCool > 0 {
		s.dashCool--
	}

	s.x += dir * move
	if s.x < 0 {
		s.x = 0
	}
	if s.x > catcherMaxX {
		s.x = catcherMaxX
	}
}
```

That import list — the empty one — is the point of the file. `cartlib`
compiles under the ordinary host Go toolchain, but a host call there
needs the `sdk/cartlib/hosttest` fake installed first, so anything that
touches one carries that setup. Rules that live in a plain struct with
plain methods are testable with `go test` in a millisecond, and
[part 4](tutorial-debug.md) leans on that hard.

Two decisions worth naming while they are small:

**The sim runs on its own clock.** One step is 25ms, regardless of what
the host's frame rate happens to be. `Update` will accumulate the
frame's delta and run whole steps, which is what makes the game behave
identically on a host that stutters — and, more to the point,
reproducibly, which is what the golden tests and the flight recorder
both depend on.

**Positions are integers.** No floats anywhere the player can see. Two
hosts have to agree bit for bit, and while plain `float64` arithmetic
would actually be fine here, integers make the promise obvious instead
of merely true. (The rule that really bites is libm: `math.Sin` and
friends can diverge across toolchains. [ABI
reference](abi.md#determinism) has the full list.)

**`game.go`** implements the interface the host drives:

```go file=game.go step=1
// SPDX-License-Identifier: GPL-2.0-or-later

package main

// The cartlib.Game implementation. Everything here is allowed to call
// host functions; nothing here decides a game rule — catcher.go does
// that.

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

type catcherGame struct {
	sim  sim
	held ui.HeldDirs

	stepAcc uint64 // dtMicros accumulated toward the next sim step
}

func (g *catcherGame) Init(s *cartlib.Setup) {
	g.sim.reset()
}

// Update advances the game by the frame's delta. dtMicros is the
// PER-TICK delta, never a running total, and it is the only clock a
// cartridge has — there is no wall clock behind it.
func (g *catcherGame) Update(dtMicros uint64) {
	g.held.Update(dtMicros)

	dir := 0
	if g.held.Held(ui.DirLeft) {
		dir = -1
	} else if g.held.Held(ui.DirRight) {
		dir = 1
	}

	g.stepAcc += dtMicros
	for g.stepAcc >= stepMicros {
		g.stepAcc -= stepMicros
		g.sim.step(dir)
	}
}
```

`dtMicros` is the **per-tick delta**, not a running total — the single
most common misreading of [the Game
interface](cartlib.md#the-game-interface). It is also the only clock you
get: there is no wall clock behind it, and `cartlib.NowMicros` is engine
time for the rare absolute comparison, not a substitute.

**`input.go`** turns records into intentions:

```go file=input.go step=1
// SPDX-License-Identifier: GPL-2.0-or-later

package main

// Turning what the player did into what the game does.
//
// Two rules from Authoring patterns' "Input truths" shape this whole
// file, and each of them is a bug that shipped first:
//
//   - Act on KindKeypress (r.IsPress()), never also on KindKeydown. The
//     cabinet sends a keydown AND a keypress per physical press, so
//     handling both fires everything twice.
//   - Track held directions with ui.HeldDirs and nothing else. A
//     terminal keyboard cannot report releases, so hand-rolled hold
//     tracking ships broken.

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

func (g *catcherGame) HandleInput(records []cartlib.InputRecord) {
	for i := range records {
		r := records[i]
		g.trackHeld(r)

		if !r.IsPress() {
			continue // keypress is the canonical "pressed"
		}
		switch r.Key {
		case cartlib.KeyJoyD:
			// D is "interact" platform-wide. B is back/exit and nothing
			// else, ever — so a second action button is always D.
			g.sim.startDash()
		}
	}
}

// trackHeld feeds ui.HeldDirs, which fuses the three shapes a held
// direction arrives in: real keydown/keyup pairs, the stick's position
// level, and a terminal's keypress-only stream. The stick is then set
// again so its diagonals count as their horizontal half.
func (g *catcherGame) trackHeld(r cartlib.InputRecord) {
	g.held.Feed(r)
	if r.Kind == cartlib.KindJoystick {
		g.held.SetJoy(joyDir(r.Joy))
	}
}

// joyDir maps the stick's 8-way position onto the four directions the
// widgets speak. The diagonals resolve to their horizontal component:
// this game only moves left and right, and a player pushing up-left
// means left.
func joyDir(j uint8) ui.Direction {
	switch j {
	case cartlib.JoyN:
		return ui.DirUp
	case cartlib.JoyS:
		return ui.DirDown
	case cartlib.JoyW, cartlib.JoyNW, cartlib.JoySW:
		return ui.DirLeft
	case cartlib.JoyE, cartlib.JoyNE, cartlib.JoySE:
		return ui.DirRight
	}
	return ui.DirNone
}
```

This file is short and almost all of it is a rule someone learned the
hard way ([Input truths](patterns.md#input-truths)):

- **React to `KindKeypress`** — `r.IsPress()`. The cabinet sends a
  keydown *and* a keypress for every physical press, so a `switch` that
  also handles keydown fires everything twice.
- **Held directions go through [`ui.HeldDirs`](ui.md#helddirs).** A
  terminal keyboard emits only keypresses — a tty cannot see releases —
  so hand-rolled hold tracking ships broken. `HeldDirs.Feed` takes every
  record and fuses real keydown/keyup pairs, the stick's position level,
  and a 250ms credit per virtual keypress. Its stick decode ignores
  diagonals, so Catcher sets the stick a second time with its own
  `joyDir`, which counts a diagonal as its horizontal half.
- **B is back/exit and nothing else, ever.** A second action button is
  always D. That is a platform-wide rule, not a per-game choice
  ([Button semantics](patterns.md#button-semantics)).
- **Quitting is already handled.** There is no `KeyJoyB` case: for a
  game that does not say otherwise, cartlib answers every quit press (q,
  escape, the red B button) with `cartlib.Exit` before `HandleInput` sees
  it. Step 3 replaces that with a hold-to-quit confirm, and says so by
  implementing `cartlib.QuitOwner`.

**`render.go`** draws, for now, three rectangles:

```go file=render.go step=1
// SPDX-License-Identifier: GPL-2.0-or-later

package main

// Every screen, drawn through the shared sdk/ui toolkit onto the
// cartridge's framebuffer.
//
// A *cartlib.Framebuffer IS a ui.Canvas, so every widget in sdk/ui
// composites straight into the frame.

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// The palette. LED panels bloom, so these are deliberately dimmer than a
// monitor would suggest — review the result at 1x from across a table,
// not zoomed in.
var (
	colSky    = ui.Color{R: 8, G: 12, B: 26, A: 255}
	colGround = ui.Color{R: 34, G: 46, B: 38, A: 255}
	colAccent = ui.Color{R: 57, G: 215, B: 160, A: 255}
)

const groundY = catcherY + catcherH // 61: the line the catcher stands on

// Render may run BEFORE the first Update — the engine's init render — so
// it has to draw a valid frame from whatever Init alone set up.
func (g *catcherGame) Render(fb *cartlib.Framebuffer) {
	fb.Clear(colSky.R, colSky.G, colSky.B)
	ui.FillRect(fb, 0, groundY, cartlib.FBWidth, cartlib.FBHeight-groundY, colGround)
	ui.FillRect(fb, g.sim.x, catcherY, catcherW, catcherH, colAccent)
}
```

A `*cartlib.Framebuffer` **is** a `ui.Canvas`, which is why every widget
in the toolkit composites straight into your frame. It is also why draw
code is testable: a plain host `go test` can render any state into a
`ui.Layer` and assert pixels, no TinyGo involved.

### Play it

```sh
make build                            # from my-cartridges/catcher
cd ../../arkade && make build         # the console renderer, once
./arkade --cartridges ../my-cartridges/catcher --app cart/catcher
```

`--cartridges` names a directory to scan for `.wasm` files, and a bare
`.wasm` with a `manifest.json` beside it is mounted as a bundle, so no
packing is needed to play. `--app` drops you straight into the
cartridge instead of the shell menu.
Arrow keys move the block, `q` or Escape leaves. You have a cartridge.

## Step 2 — fruit that falls

Now the game. `catcher.go` grows fruit, catching, missing, lives, a
score, and a difficulty curve. It stays exactly this until [part
3](tutorial-levels.md), which gives the opening of a run an authored
shape.

```go file=catcher.go step=2
// SPDX-License-Identifier: GPL-2.0-or-later

package main

// The simulation. It reaches for no host call, so plain `go test` covers
// every rule in it. step takes its randomness as a parameter: the
// cartridge passes cartlib.RandomU64, a test passes a counter.

const (
	// The panel. cartlib.FBWidth/FBHeight say the same thing, but this
	// file does not import cartlib.
	fbW = 64
	fbH = 64

	catcherW    = 7        // one cel of assets/sprites/catcher.png
	catcherH    = 9        //
	catcherY    = fbH - 12 // 52: the catcher's top edge, feet at 60
	basketH     = 3        // the open basket is the sprite's top 3 rows
	catcherMaxX = fbW - catcherW

	fruitW = 5 // assets/sprites/fruit.png
	fruitH = 5

	// a fixed array rather than a growing slice: -gc=leaking never
	// collects (GOLDEN RULE 7)
	maxFruit   = 6
	startLives = 3

	// One sim step is 25ms (40 Hz), independent of the host's ~30 fps
	// frame rate. Update accumulates dtMicros and runs whole steps, so
	// the game plays identically on a host that stutters — and, more
	// importantly, reproducibly, which is what the golden tests pin.
	stepMicros = 25_000

	walkStep = 2 // pixels per step walking
	dashStep = 5 // pixels per step dashing
	// A dash is short and its cooldown is long: it is the answer to one
	// fruit you cannot walk to, not a faster way to move.
	dashSteps         = 8  // 200ms
	dashCooldownSteps = 56 // 1.4s after it ends

	// Difficulty is a function of fruit caught, so it ramps with skill
	// rather than with elapsed time. fallRate is in sixteenths of a
	// pixel per step: 14 is ~35 px/s (a little under two seconds to
	// fall), 40 is 100 px/s.
	fallBase  = 14
	fallMax   = 40
	spawnBase = 44 // steps between spawns at the start (1.1s)
	spawnMin  = 16 // ...and at full tilt (0.4s)

	catchScore = 10
)

// fruit is one falling piece. y16 is the top edge in sixteenths of a
// pixel: integer arithmetic, because two hosts must agree on the answer
// bit for bit and floats invite a divergence the golden tests would
// only find later (see the ABI reference's determinism rules).
type fruit struct {
	live bool
	x    int // left edge, 0..fbW-fruitW
	y16  int // top edge, in 1/16 px
}

// y is the fruit's top edge in whole pixels.
func (f fruit) y() int { return f.y16 / 16 }

// sim is the whole game state. Everything the player can see is derived
// from these fields, and nothing else — no clock, no randomness held
// back, no pointers into anywhere.
type sim struct {
	x        int // the catcher's left edge
	dashLeft int // steps of dash remaining
	dashCool int // steps until the next dash is allowed
	fruit    [maxFruit]fruit
	spawnIn  int // steps until the next spawn
	caught   int
	lives    int
	score    int
}

// reset puts a fresh run on the board. Assigning a whole struct literal
// (rather than zeroing fields one at a time) means a field added later
// cannot be forgotten here.
func (s *sim) reset() {
	*s = sim{
		x:       (fbW - catcherW) / 2,
		lives:   startLives,
		spawnIn: spawnBase,
	}
}

func (s *sim) over() bool { return s.lives <= 0 }

// fallRate is the current fall speed in 1/16 px per step.
func (s *sim) fallRate() int {
	r := fallBase + s.caught/2
	if r > fallMax {
		r = fallMax
	}
	return r
}

// spawnEvery is the current gap between spawns, in steps.
func (s *sim) spawnEvery() int {
	n := spawnBase - s.caught
	if n < spawnMin {
		n = spawnMin
	}
	return n
}

// dashReady reports whether pressing the blue button would do something
// right now. peripherals.go lights the button on exactly this answer —
// the eligibility-lighting rule (see Authoring patterns).
func (s *sim) dashReady() bool { return s.dashLeft == 0 && s.dashCool == 0 }

// startDash begins a dash if one is available, reporting whether it did
// so the caller can play a sound (or ring the reject bell) accordingly.
func (s *sim) startDash() bool {
	if !s.dashReady() {
		return false
	}
	s.dashLeft = dashSteps
	return true
}

// stepResult is what one step made happen. The sim returns events rather
// than playing sounds itself: sound is a host call, and keeping host
// calls out of this file is what keeps it testable.
type stepResult struct {
	caught int
	missed int
}

// step advances the simulation by exactly one stepMicros tick. dir is
// -1, 0 or +1 (the held direction, resolved by the caller); rand is the
// randomness source, called exactly once per spawn and never otherwise.
func (s *sim) step(dir int, rand func() uint64) stepResult {
	var out stepResult

	// 1. Move. A dash overrides the walk speed and, when it ends, starts
	// its own cooldown; the cooldown only ticks down while not dashing.
	move := walkStep
	if s.dashLeft > 0 {
		move = dashStep
		s.dashLeft--
		if s.dashLeft == 0 {
			s.dashCool = dashCooldownSteps
		}
	} else if s.dashCool > 0 {
		s.dashCool--
	}
	s.x += dir * move
	if s.x < 0 {
		s.x = 0
	}
	if s.x > catcherMaxX {
		s.x = catcherMaxX
	}

	// 2. Spawn. The single RandomU64 draw happens here and only here,
	// which is what lets a golden test script the whole run's fruit by
	// listing a few numbers.
	if s.spawnIn > 0 {
		s.spawnIn--
	}
	if s.spawnIn == 0 {
		x := int(rand() % uint64(fbW-fruitW+1))
		for i := range s.fruit {
			if !s.fruit[i].live {
				s.fruit[i] = fruit{live: true, x: x}
				break
			}
		}
		s.spawnIn = s.spawnEvery()
	}

	// 3. Fall, and resolve each fruit against the basket and the ground.
	rate := s.fallRate()
	for i := range s.fruit {
		f := &s.fruit[i]
		if !f.live {
			continue
		}
		f.y16 += rate
		if overlaps(f.x, f.y(), fruitW, fruitH, s.x, catcherY, catcherW, basketH) {
			f.live = false
			s.score += catchScore
			s.caught++
			out.caught++
			continue
		}
		if f.y() >= fbH-fruitH {
			f.live = false
			s.lives--
			out.missed++
		}
	}
	return out
}

// overlaps is axis-aligned rectangle intersection.
//
// Catcher collides rectangles because its catching surface IS a
// rectangle — the open mouth of the basket, the sprite's top three rows,
// the same shape in every walk cel. Art with an irregular silhouette
// wants pixel-perfect mask collision instead: Sheet.Cel has already
// built a ui.Mask for every sprite it lifts, and ui.Mask.Overlaps
// compares two of them directly. See the ui reference's "Sprites and
// masks" and Authoring patterns' "Sprite art as PNGs".
func overlaps(ax, ay, aw, ah, bx, by, bw, bh int) bool {
	return ax < bx+bw && bx < ax+aw && ay < by+bh && by < ay+ah
}
```

The two lines that matter most in that file are the ones about
randomness and about memory.

**`step` takes its randomness as a parameter.** The simulation needs a
random column and must not name a source: the cartridge hands it
`cartlib.RandomU64`, and a unit test hands it a counter. Neither can lie
about the other. And it draws **exactly one number per fruit and none at
any other time**, which is what will let a golden test describe an
entire run with a two-element script.

`cartlib.RandomU64` is not a convenience. It is the *only* randomness a
cartridge may use — `math/rand` has no engine seed behind it, and the
flight recorder replays a session by feeding the recorded draws back in
order, so a cartridge with a second source of chance simply cannot be
replayed.

**The fruit live in a fixed array.** TinyGo builds cartridges with
`-gc=leaking`, which never collects: a per-frame allocation leaks
forever, and a cabinet that has been up for a week falls over. Fixed
scratch, sized once in `Init`, is the standard answer to almost all of
it. The [hard rules](cartlib.md) list the rest — no goroutines,
no `time`, no `fmt`.

`game.go` grows the wiring for it:

```go file=game.go step=2
// SPDX-License-Identifier: GPL-2.0-or-later

package main

// The cartlib.Game implementation. Everything here is allowed to call
// host functions; nothing here decides a game rule — catcher.go does
// that.

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

type catcherGame struct {
	sim  sim
	held ui.HeldDirs

	stepAcc uint64 // dtMicros accumulated toward the next sim step
}

func (g *catcherGame) Init(s *cartlib.Setup) {
	g.sim.reset()
}

// Update advances the game by the frame's delta. dtMicros is the
// PER-TICK delta, never a running total, and it is the only clock a
// cartridge has — there is no wall clock behind it.
func (g *catcherGame) Update(dtMicros uint64) {
	g.held.Update(dtMicros)

	dir := 0
	if g.held.Held(ui.DirLeft) {
		dir = -1
	} else if g.held.Held(ui.DirRight) {
		dir = 1
	}
	playerWalk.advance(dtMicros, dir != 0)

	g.stepAcc += dtMicros
	for g.stepAcc >= stepMicros {
		g.stepAcc -= stepMicros

		// The sim reports what happened rather than acting on it: sound
		// is a host call, and keeping host calls out of catcher.go is
		// what keeps it testable. cartlib.RandomU64 is the ONLY
		// randomness a cartridge may use — never math/rand, which has no
		// engine seed behind it and would break replay outright.
		res := g.sim.step(dir, cartlib.RandomU64)
		_ = res

		if g.sim.over() {
			// Nowhere to go yet. Step 4 gives the run an ending.
			g.sim.reset()
			playerWalk.reset()
			return
		}
	}
}
```

and `render.go` learns to draw fruit and a HUD. The catcher is still a
rectangle; [part 2](tutorial-assets.md) replaces it with a drawing.

```go file=render.go step=2
// SPDX-License-Identifier: GPL-2.0-or-later

package main

// Every screen, drawn through the shared sdk/ui toolkit onto the
// cartridge's framebuffer.
//
// A *cartlib.Framebuffer IS a ui.Canvas, so every widget in sdk/ui
// composites straight into the frame.

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// The palette. LED panels bloom, so these are deliberately dimmer than a
// monitor would suggest — review the result at 1x from across a table,
// not zoomed in.
var (
	colSky    = ui.Color{R: 8, G: 12, B: 26, A: 255}
	colGround = ui.Color{R: 34, G: 46, B: 38, A: 255}
	colScore  = ui.Color{R: 255, G: 214, B: 92, A: 255}
	colLives  = ui.Color{R: 232, G: 66, B: 74, A: 255}
	colDim    = ui.Color{R: 150, G: 156, B: 170, A: 255}
	colAccent = ui.Color{R: 57, G: 215, B: 160, A: 255}
	colFruit  = ui.Color{R: 232, G: 66, B: 74, A: 255}
	colDash   = ui.Color{R: 92, G: 150, B: 255, A: 255}
)

const groundY = catcherY + catcherH // 61: the line the catcher stands on

// Render may run BEFORE the first Update — the engine's init render — so
// it has to draw a valid frame from whatever Init alone set up.
func (g *catcherGame) Render(fb *cartlib.Framebuffer) {
	g.drawPlaying(fb)
}

func (g *catcherGame) drawPlaying(fb *cartlib.Framebuffer) {
	fb.Clear(colSky.R, colSky.G, colSky.B)
	ui.FillRect(fb, 0, groundY, cartlib.FBWidth, cartlib.FBHeight-groundY, colGround)

	for i := range g.sim.fruit {
		f := g.sim.fruit[i]
		if f.live {
			ui.FillRect(fb, f.x, f.y(), fruitW, fruitH, colFruit)
		}
	}
	ui.FillRect(fb, g.sim.x, catcherY, catcherW, catcherH, colAccent)
	g.drawHUD(fb)
}

// drawHUD is the panel's own readout: score top left, lives top right.
func (g *catcherGame) drawHUD(fb *cartlib.Framebuffer) {
	small := ui.Font3x5{}
	// DrawUint draws a number straight from an integer, with no string
	// built and thrown away on the way.
	small.DrawUint(fb, 1, 1, uint32(g.sim.score), 1, colScore)

	for i := 0; i < g.sim.lives; i++ {
		ui.FillRect(fb, cartlib.FBWidth-3-i*4, 1, 2, 2, colLives)
	}

	// The dash meter: a short bar under the score that fills back up as
	// the cooldown runs out, so "why can't I dash" has a visible answer.
	if g.sim.dashReady() {
		ui.FillRect(fb, 1, 7, 8, 1, colDash)
		return
	}
	w := 8 - 8*g.sim.dashCool/dashCooldownSteps
	if w > 0 {
		ui.FillRect(fb, 1, 7, w, 1, colDim)
	}
}

// playerWalk is the gameplay catcher's cycle. It lives with the drawing
// rather than in catcherGame because that is what it is: the game only
// ever has to say whether the catcher is moving.
//
// Part 2 replaces the rectangle above with a three-cel animation strip
// and this counter with the strip's own timing table. For now it counts,
// so that the wiring is in place and only the art has to change.
var playerWalk walker

type walker struct {
	acc  uint64
	step int
}

func (w *walker) reset() { w.acc, w.step = 0, 0 }

func (w *walker) advance(dtMicros uint64, moving bool) {
	if !moving {
		w.reset()
		return
	}
	w.acc += dtMicros
	for w.acc >= 130_000 {
		w.acc -= 130_000
		w.step = (w.step + 1) % 4
	}
}
```

Rebuild and play. It is a game now: fruit falls, you catch some, the
score climbs, three misses and it silently starts over. The dash works
and the little blue bar under the score shows you when it is back.

### Test the rules while they are still rules

Before adding screens, lock the behaviour down. Because `catcher.go`
imports nothing, this is a plain unit test in a `catcher_test.go` of
your own. (`sdk/tutorial/catcher_test.go` is the finished suite, but it
also tests [part 3](tutorial-levels.md)'s authored waves and will not
compile against this step; part 3 tells you when to take it.) The shape
is:

```go fragment
func fixedRand(v uint64) func() uint64 { return func() uint64 { return v } }

func runSteps(s *sim, n, dir int, rand func() uint64) stepResult {
	var total stepResult
	for i := 0; i < n; i++ {
		r := s.step(dir, rand)
		total.caught += r.caught
		total.missed += r.missed
	}
	return total
}

// The catcher starts centred, so a fruit spawned in column s.x
// lands in the basket...
res := runSteps(s, 400, 0, fixedRand(uint64(s.x)))
// ...and one spawned in column 0 never does.
res := runSteps(s, 400, 0, fixedRand(0))
```

```sh
go test ./...      # from my-cartridges/catcher
```

Under a second, no TinyGo, no wasm. Everything you can push down into
this layer is a test you will actually run.

## Step 3 — screens, scores, and the way out

A run that restarts silently is not a game anyone finishes. This step
adds the three screens around the one you have — a title, a game-over
banner, an initials entry — plus the cabinet hardware that goes with
them, and the exit flow that has to work from all of it.

The three files land together because they genuinely depend on each
other: the screens need the peripherals, and the peripherals need to
know which screen is up.

One more dependency first — the screen machine, `sdk/fsm`. Add it to
`go.mod` beside the other two, pointed at the same engine checkout:

```go-mod
require github.com/stealthmonkey99/arkade/sdk/fsm v0.1.0

replace github.com/stealthmonkey99/arkade/sdk/fsm => ../../arkade/sdk/fsm
```

### The screen machine

Every cartridge with more than one screen keeps a variable saying which
one is up and a `switch` that dispatches on it. **That part is good and
`sdk/fsm` does not change it** — the switch below is exactly the switch
you would write by hand. What the package owns is the part that hangs off
*entering* a screen, which would otherwise be re-implemented per cartridge
and shipped wrong repeatedly:

- **Every screen entry owes its peripherals.** The LCD and the button
  lamps are never restored automatically, so by hand every path that
  reaches a screen has to remember to re-assert them. Here they
  are asserted once, in `enterScreen`, which the machine runs on every
  arrival — and the variable cannot move any other way, so the call
  cannot be forgotten.
- **A modal needs somewhere to go back to.** `Push` and `Pop` keep the
  covered screen on a stack, so "back to where?" is not a question
  anyone has to answer.
- **A screen spelled as a boolean stops the state variable telling the
  truth.** An overlay tracked as `g.confirm.Active()` is an invisible
  fifth screen that `Update`, `Render` and `HandleInput` each test for
  separately. It is a screen, so it is `stConfirm`.
- **A per-screen clock has to be zeroed on the way in.** `Elapsed` is
  that clock, zeroed by the machine, frozen while an overlay covers the
  screen and resumed when it lifts.

Honestly: at five screens Catcher is *below* the size where this pays for
itself in lines — the package's own design note works that arithmetic out
and says so. It is used here because it is how a cartridge structures a
game, and a tutorial that taught the hand-rolled version would be
teaching you to write the bugs above. See [when not to
use it](#when-not-to-reach-for-it) at the end of this section.

```go file=game.go step=3
// SPDX-License-Identifier: GPL-2.0-or-later

package main

// The cartlib.Game implementation: the screen machine and the lifecycle
// transitions between its five screens. Everything here is allowed to
// call host functions; nothing here decides a game rule (catcher.go does
// that).

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/fsm"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// The screens. sdk/fsm holds which one is up; a cartridge declares its own
// dense enum exactly as it would for a plain uint8, and dispatches on it
// with the same switch. What the package adds is that the variable can
// only move through Go/Push/Pop, and every arrival runs enterScreen — so a
// screen cannot be entered without asserting what it owns.
const (
	stTitle fsm.State = iota
	stPlaying
	stOver
	stName
	// stConfirm is the hold-B quit overlay. It is a screen, not a flag:
	// input goes to it, Render draws it, and Update freezes everything
	// under it, which is what a screen means.
	stConfirm
)

// legalEdges is the map of this cartridge, one bit per destination. It is
// validation and editor data — it never decides anything. Pop has no entry
// because its destination is the stack, not an edge.
var legalEdges = []uint16{
	stTitle:   1 << stPlaying,
	stPlaying: 1<<stOver | 1<<stConfirm,
	stOver:    1<<stName | 1<<stTitle,
	stName:    1<<stTitle | 1<<stConfirm,
	stConfirm: 0,
}

const (
	// overHoldMicros is how long the game-over banner holds before the
	// game moves on by itself. Everything except quitting is ignored for
	// all of it — Authoring patterns' death-screen lockout, because
	// players are still mashing when they die.
	overHoldMicros = 2_500_000

	// nameMaxLen is the initials budget. Three is the arcade convention
	// and what every other cartridge uses.
	nameMaxLen = 3
)

type catcherGame struct {
	// machine is which screen is up. There is deliberately no `st` field
	// beside it: one truth, and no way to change it that skips the hook.
	machine fsm.Machine
	sim     sim

	held    ui.HeldDirs
	confirm ui.HoldConfirm
	name    *ui.NameEntry

	// seat is the physical seat this run is locked to, or
	// cartlib.SeatNone when the player is on a keyboard with no seat.
	// It addresses the LCD and the button lights: the input record's
	// Physical byte, never its Player byte (see the cartlib reference,
	// "Input").
	seat cartlib.Seat

	stepAcc uint64 // dtMicros accumulated toward the next sim step
	lcdAcc  uint64 // LCD refresh throttle

	// Scratch owned by the game, filled once per use. Building these in
	// Render or Update would allocate every frame, which -gc=leaking
	// never gives back (GOLDEN RULE 7).
	top     ui.AttractScore
	nameBuf [8 + cartlib.MaxNameLen]byte
}

func (g *catcherGame) Init(s *cartlib.Setup) {
	loadSounds(s)
	defineGlyphs(s)
	g.name = ui.NewNameEntry(17, 30, nameMaxLen)
	g.confirm.W, g.confirm.H = cartlib.FBWidth, cartlib.FBHeight
	g.seat = cartlib.SeatNone
	g.machine.OnEnter(g.enterScreen)
	g.machine.Edges(legalEdges)

	// A push alone is inert — input stays permissive until something
	// locks it — so the title can be started from either controller.
	// startPlay locks to whichever seat actually pressed.
	cartlib.PushPlayerStrategy(cartlib.StrategyOnePlayerSolo)

	// Reset places the machine and fires the title's entry hook, which is
	// the whole of "show the title screen".
	g.machine.Reset(stTitle)
}

// enterScreen is the ONE place a screen asserts what it owns: its LCD
// line, its button lamps, and whatever it starts fresh. Nothing on the
// cabinet restores peripherals for you, and this hook runs on EVERY
// arrival — including the Pop back out from under a dismissed overlay,
// which is what puts the covered screen's LCD and lamps back.
//
// "Every arrival" is also the constraint on what belongs here. Work that
// must not be redone on a resume — reading the seat's last initials into
// the name widget, resetting the sim — stays at the call site below. The
// hook is for what is true whenever this screen is on top, not for how the
// player got to it.
func (g *catcherGame) enterScreen(prev, next fsm.State) {
	switch next {
	case stTitle:
		// The title belongs to no seat. Releasing the run's lock here is
		// what lets the next player start from either controller.
		g.sim.reset()
		g.held.Reset()
		stopMusic()
		cartlib.UnlockPlayerStrategy()
		g.seat = cartlib.SeatNone
		g.top = topScore(g.nameBuf[:])
		showTitlePeripherals()
	case stPlaying:
		g.pushLCD()
		g.showPlayingLights()
	case stOver:
		g.held.Reset()
		g.pushLCD()
		showNoLights(g.seat)
	case stName:
		g.pushLCD()
		showNameLights(g.seat)
	case stConfirm:
		showConfirmPeripherals(g.seat)
	}
}

// startPlay locks the panel and the input gate to the seat we have
// actually seen a press from. Never lock to a seat you haven't observed:
// an unobserved lock gates the real player out of their own game.
func (g *catcherGame) startPlay(seat cartlib.Seat) {
	g.sim.reset()
	playerWalk.reset()
	g.stepAcc = 0
	g.seat = seat
	cartlib.LockPlayerToSeat(seat)
	startMusic()
	g.machine.Go(stPlaying)
}

func (g *catcherGame) enterOver() {
	stopMusic()
	playOver()
	g.machine.Go(stOver)
}

// enterName opens the initials entry, prefilled with this seat's last
// name so a returning player accepts it with one press. The prefill is
// here rather than in the entry hook on purpose: the hook also runs when
// the quit overlay is dismissed, and re-prefilling then would wipe
// initials the player had already typed.
func (g *catcherGame) enterName() {
	g.name.Reset()
	if prev, ok := cartlib.RecentName(g.seat, g.nameBuf[:]); ok {
		g.name.SetName(string(prev))
	}
	g.name.SetRecents(cartlib.RecentNames(g.seat))
	g.machine.Go(stName)
}

func (g *catcherGame) submitName() {
	name := g.name.Name()
	cartlib.ScoreSubmit(g.sim.score, name)
	cartlib.NoteRecentName(g.seat, name)
	g.machine.Go(stTitle)
}

func (g *catcherGame) Update(dtMicros uint64) {
	g.held.Update(dtMicros)
	g.lcdAcc += dtMicros

	// Tick feeds the clock of the screen on TOP of the stack, and only
	// that one. That is what freezes a covered screen for free: while the
	// overlay is up, the banner timer below it does not advance.
	g.machine.Tick(dtMicros)

	switch g.machine.State() {
	case stPlaying:
		g.updatePlaying(dtMicros)
	case stOver:
		// Elapsed is time since this screen was entered — the per-screen
		// clock a cartridge would otherwise hand-roll and forget to zero.
		if g.machine.Elapsed() >= overHoldMicros {
			if cartlib.ScoreQualifies(g.sim.score) {
				g.enterName()
			} else {
				g.machine.Go(stTitle)
			}
		}
	case stName:
		// Move is level-triggered: the currently HELD direction, every
		// frame. Do is edge-triggered and lives in HandleInput.
		g.name.Move(g.held.Current())
		g.name.Update(dtMicros)
		if g.name.Done() {
			g.submitName()
		}
	case stConfirm:
		if g.confirm.Update(dtMicros) {
			cartlib.Exit()
		}
	}
	// stTitle has no per-frame work: Tick above IS its animation clock,
	// which render.go reads back as machine.Elapsed().
}

func (g *catcherGame) updatePlaying(dtMicros uint64) {
	dir := 0
	if g.held.Held(ui.DirLeft) {
		dir = -1
	} else if g.held.Held(ui.DirRight) {
		dir = 1
	}
	playerWalk.advance(dtMicros, dir != 0)

	// Fixed-step integration: accumulate the frame's delta and run whole
	// 25ms steps. dtMicros is the per-tick delta, never a running total
	// (see the cartlib reference, "The Game interface").
	g.stepAcc += dtMicros
	for g.stepAcc >= stepMicros {
		g.stepAcc -= stepMicros
		res := g.sim.step(dir, cartlib.RandomU64)
		if res.caught > 0 {
			playCatch()
			g.lcdAcc = lcdIntervalMicros // force the readout up now
		}
		if res.missed > 0 {
			playMiss()
			g.lcdAcc = lcdIntervalMicros
		}
		if g.sim.over() {
			g.enterOver()
			return
		}
	}
	g.showPlayingLights()
	g.tickLCD()
}

// topScore reads rank 1 out of the cabinet's shared table for the title
// screen. buf is caller-owned scratch; the name string is the one copy,
// made once per visit to the title.
func topScore(buf []byte) ui.AttractScore {
	score, name, ok := cartlib.ScoreRead(1, buf)
	if !ok {
		return ui.AttractScore{}
	}
	return ui.AttractScore{Name: string(name), Score: score}
}
```

Read `enterScreen` twice, because it carries the one rule that is easy to
get wrong: **it runs on every arrival, including a `Pop`.** That is the
whole point — the covered screen's LCD and lamps come back because
re-entering re-asserts them — but it also means anything that must *not*
be redone on a resume does not belong in the hook. Prefilling the name
widget from the seat's last initials is the example: it lives in
`enterName`, at the call site, because running it again when the player
dismisses the quit overlay would wipe initials they had already typed.
The hook is for what is true *whenever this screen is on top*, not for
how the player got here.

The other thing worth noticing is what is *not* in the machine:
`stepAcc`, the fixed-step accumulator, and `lcdAcc`, the LCD throttle.
`Elapsed` is "time since this screen was entered"; a repeating or
stepping accumulator is a different thing and stays a plain field.

<a id="when-not-to-reach-for-it"></a>
**When not to reach for it.** `sdk/fsm` is opt-in and it is not a
line-count win — adopting it rarely makes a cartridge shorter. Its
value is that four bug classes become impossible or caught: a screen
entered without its peripherals, a modal resumed to the wrong screen, a
per-screen clock left unzeroed, and a transition nobody declared. If a
cartridge has one screen, no modal and no peripherals to assert, a
plain `uint8` and a `switch` is the right answer and always will be.
[`sdk/fsm`'s README](../fsm/README.md) has the adoption recipe.

Three more things in there are cabinet doctrine rather than code style:

**Seats.** `LockPlayerToSeat` gates input to one controller and rotates
the panel toward that player — but only to a seat you have actually seen
a press from. Locking to an assumed seat gates the real player out of
their own game. The seat comes from the record's `Physical` byte, never
its `Player` byte: `Player` is the pairing slot ("whose paddle moves"),
`Physical` is the cabinet port ("whose LCD and lights these are"), and
they diverge the moment somebody picks up the other controller.
[Seating & rotation](seating.md) is the full doctrine; a one-player game
like this one uses about three lines of it.

**Scores go to the shared table, always.** `cartlib.ScoreQualifies` /
`ScoreSubmit` write the per-game top ten that the shell's LCD readout,
the attract trophy and the leaderboard ticker all read. A high score
squirrelled away in private storage is invisible to every one of them.
The qualification policy lives host-side, once, for every game
([Scores and recent names](cartlib.md#scores-and-recent-names)).

**The game-over banner ignores input for two and a half seconds.**
Players are still mashing when they die, and a death screen that accepts
the mash skips straight past whatever it was trying to say.

### The way out

```go file=input.go step=3
// SPDX-License-Identifier: GPL-2.0-or-later

package main

// Turning what the player did into what the game does: the frame's input
// records decoded, dispatched to the screen that is up, and the
// platform-standard hold-B exit that has to work from every one of them.
//
// Three rules from Authoring patterns' "Input truths" shape this whole
// file, and each of them is a bug that shipped first:
//
//   - Act on KindKeypress (r.IsPress()), never also on KindKeydown. The
//     cabinet sends a keydown AND a keypress per physical press, so
//     handling both fires everything twice.
//   - Route keyup to ui.HoldConfirm BEFORE any keypress-only early
//     return, or the confirm can never arm on a real pad.
//   - Track held directions with ui.HeldDirs and nothing else. A
//     terminal keyboard cannot report releases, so hand-rolled hold
//     tracking ships broken.

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// OwnsQuitKeys tells cartlib that this game runs its own quit flow (the
// hold-B overlay), so cartlib must not answer B with an immediate Exit.
func (g *catcherGame) OwnsQuitKeys() bool { return true }

func (g *catcherGame) HandleInput(records []cartlib.InputRecord) {
	for i := range records {
		g.handleRecord(records[i])
	}
}

func (g *catcherGame) handleRecord(r cartlib.InputRecord) {
	g.trackHeld(r)

	// ui.HoldConfirm needs keydown AND keyup, so it routes before the
	// keypress-only filter below. This is why the overlay is a screen and
	// not a widget flag: the machine says which screen owns the record,
	// and this one owns it before the filter rather than after.
	if g.machine.State() == stConfirm {
		if !g.confirm.Feed(r) && r.IsPress() {
			g.closeConfirm() // any other press cancels
		}
		return
	}

	if !r.IsPress() {
		return // keypress is the canonical "pressed"; keydown would double-fire
	}

	switch g.machine.State() {
	case stTitle:
		g.titleInput(r)
	case stPlaying:
		g.playingInput(r)
	case stOver:
		// Locked out except for quitting — see overHoldMicros.
		if r.IsQuit() {
			cartlib.Exit()
		}
	case stName:
		g.nameInput(r)
	}
}

// trackHeld feeds ui.HeldDirs, the one sanctioned way to know a
// direction is being held: Feed fuses real keydown/keyup pairs, the
// stick's position level and a terminal's keypress-only stream. The stick
// is then set again so its diagonals count as their horizontal half —
// this game only moves left and right, and a player pushing up-left means
// left.
func (g *catcherGame) trackHeld(r cartlib.InputRecord) {
	g.held.Feed(r)
	if r.Kind == cartlib.KindJoystick {
		g.held.SetJoy(joyDir(r.Joy))
	}
}

func (g *catcherGame) titleInput(r cartlib.InputRecord) {
	switch r.Key {
	case cartlib.KeyJoyA, cartlib.KeyEnter, cartlib.KeySpace:
		g.startPlay(r.Physical)
	case cartlib.KeyQuit, cartlib.KeyEscape, cartlib.KeyJoyB:
		cartlib.Exit() // B at the title is immediate; no confirm
	}
}

func (g *catcherGame) playingInput(r cartlib.InputRecord) {
	switch r.Key {
	case cartlib.KeyJoyD:
		// D is "interact" platform-wide, and it is lit exactly when it
		// would do something. Pressing it when it wouldn't rings the
		// cabinet's polite "that does nothing" bell rather than
		// silently ignoring the player.
		if g.sim.startDash() {
			playDash()
			g.showPlayingLights()
		} else {
			cartlib.RejectKey()
		}
	case cartlib.KeyQuit, cartlib.KeyEscape, cartlib.KeyJoyB:
		g.openConfirm(r)
	}
}

func (g *catcherGame) nameInput(r cartlib.InputRecord) {
	switch r.Key {
	case cartlib.KeyJoyA, cartlib.KeyEnter, cartlib.KeySpace:
		g.name.Do(ui.ActionActivate)
		if g.name.Done() {
			g.submitName()
		}
	case cartlib.KeyQuit, cartlib.KeyEscape, cartlib.KeyJoyB:
		// The placement is not in the table yet, so a fumbled B here
		// would throw the whole run away. The platform-wide rule is that a
		// qualifying name entry asks first, and the overlay keeps clear of
		// the initials so the player can see what they would lose.
		g.openConfirm(r)
		g.confirm.Avoid(g.name.LetterBand())
	}
}

// openConfirm PUSHES the overlay over whatever raised it, and feeds it the
// press that opened it. The covered screen stays on the stack, which is
// why there is no "return state" variable here: the answer to "back to
// where?" is the stack itself, and a stack cannot disagree with the screen
// it was pushed from.
func (g *catcherGame) openConfirm(r cartlib.InputRecord) {
	g.confirm.Open()
	g.confirm.Feed(r)
	g.machine.Push(stConfirm)
}

// closeConfirm dismisses it. Nothing restores peripheral state for you —
// but Pop re-enters the covered screen, and entering a screen runs
// enterScreen, so its LCD text and lamps come back without this function
// knowing which screen that was.
func (g *catcherGame) closeConfirm() {
	g.confirm.Close()
	g.machine.Pop()
}

// joyDir maps the stick's 8-way position onto the four directions the
// widgets speak, with the diagonals resolved to their horizontal component.
func joyDir(j uint8) ui.Direction {
	switch j {
	case cartlib.JoyN:
		return ui.DirUp
	case cartlib.JoyS:
		return ui.DirDown
	case cartlib.JoyW, cartlib.JoyNW, cartlib.JoySW:
		return ui.DirLeft
	case cartlib.JoyE, cartlib.JoyNE, cartlib.JoySE:
		return ui.DirRight
	}
	return ui.DirNone
}
```

The hold-B flow is platform-standard and deliberately not a per-game
decision: B opens an overlay, releasing it arms the hold, holding for
about a second confirms, and the confirm fires on release so the shell
never inherits a held button. [`ui.HoldConfirm`](ui.md#holdconfirm) is
the widget, and `HoldConfirm.Feed` takes each record whole — it drives
the hold from B's keydown, keyup and keypress, and reports `false` for
anything else, which is the cancel. The rules around it are

- **declare that you own the quit keys.** `OwnsQuitKeys` returning true
  (`cartlib.QuitOwner`) stops cartlib answering B with an immediate exit,
  which is what step 1 relied on and what would now skip the overlay;
- **route records to it before any keypress-only early return**, or it
  can never arm on a real pad; and
- **it must work from every state** — including, and especially, over a
  qualifying name entry, where the placement is not in the table yet and
  a fumbled B would throw the whole run away. [Part 4](tutorial-debug.md)
  shows the shared harness that asserts it.

`openConfirm` and `closeConfirm` are a handful of lines between them,
and the reason is the machine. The overlay is `Push`ed, so the screen it covers
stays on the stack; `Pop` re-enters that screen, and re-entering runs
`enterScreen`, which puts its LCD line and its lamps back. Neither
function has to know which screen that was.

### The hardware

```go file=peripherals.go step=3
// SPDX-License-Identifier: GPL-2.0-or-later

package main

// Sound, the seat's LCD and the lit buttons. The LCD is the HUD and never
// repeats the panel; a button is lit exactly when pressing it would do
// something.

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// Sound handles, resolved once in Init. Paths are bundle-relative,
// exactly as packed into the .rcart. On the browser host these resolve
// asynchronously during the splash phase; playing one that has not
// landed yet (or a bare .wasm with no bundle at all) is a safe no-op, so
// a dev build stays quiet instead of crashing.
var (
	sndCatch uint32
	sndMiss  uint32
	sndDash  uint32
	sndOver  uint32
	sndTheme uint32
)

func loadSounds(s *cartlib.Setup) {
	sndCatch = s.LoadSound("assets/catch.wav")
	sndMiss = s.LoadSound("assets/miss.wav")
	sndDash = s.LoadSound("assets/dash.wav")
	sndOver = s.LoadSound("assets/gameover.wav")
	sndTheme = s.LoadSound("assets/theme.wav")
}

func playCatch() { cartlib.PlaySound(sndCatch, 0) }
func playMiss()  { cartlib.PlaySound(sndMiss, 0) }
func playDash()  { cartlib.PlaySound(sndDash, 0) }

// playOver stops the bed first, so the sting lands in silence rather
// than fighting the loop for the same speaker.
func playOver() {
	cartlib.StopSound(sndTheme)
	cartlib.PlaySound(sndOver, 0)
}

func startMusic() { cartlib.PlaySound(sndTheme, cartlib.PlaySoundForever) }
func stopMusic()  { cartlib.StopSound(sndTheme) }

// lcdIntervalMicros throttles the readout to about 4 Hz. A 16x2
// character LCD cannot legibly redraw at frame rate, and a readout
// driven by the sim rather than by an event will blow past that without
// anyone noticing. Events (a catch, a miss, a screen change) push
// immediately instead of waiting for the next tick.
const lcdIntervalMicros uint64 = 250_000

// The basket. A 16x2 character LCD draws ASCII out of the chip's own mask
// ROM and nothing else, so anything pictorial has to be uploaded as a 5x8
// bitmap first. The engine ships a table of those for the runes games kept
// asking for — the hearts on the lives line below are one — and
// Setup.DefineLCDGlyph is how a game brings a rune that table has never
// heard of, without anybody editing the engine.
//
// Read the hex down the left and the picture is right there: an arch for
// the handle, a full-width rim, a woven body, a narrower base. Bit 4 is the
// leftmost of the five pixels, so 0x11 is "both edges lit, middle dark".
//
// The rune is ֍ (U+058D, ARMENIAN ETERNITY SIGN): a plain, single-width,
// monochrome character that already reads as woven. Never an emoji —
// terminals draw those in colour and two cells wide, and both hosts refuse
// them (Setup.DefineLCDGlyph returns false).
const basket = '֍'

var basketRows = cartlib.GlyphRows{
	0x00, // . . . . .
	0x0e, // . # # # .   handle
	0x11, // # . . . #   handle ends
	0x1f, // # # # # #   rim
	0x15, // # . # . #   weave
	0x1f, // # # # # #   weave
	0x0e, // . # # # .   base
	0x00, // . . . . .
}

// defineGlyphs runs once, from Init. Nothing is reserved by defining a
// glyph: what is scarce is the chip's eight CGRAM slots, and that budget is
// per SCREEN and per MOMENT. The playing screen spends two of the eight —
// this basket on line 1, the engine's heart on line 2 — and the eight are
// shared between the two sources, so the number worth watching is how many
// DIFFERENT custom runes can be on one panel together.
func defineGlyphs(s *cartlib.Setup) {
	s.DefineLCDGlyph(basket, basketRows)
}

// LCD text, built once: formatting a fresh string or []byte every frame
// would allocate under an allocator that never gives memory back.
var (
	lcdTitle1 = []byte("CATCHER ֍")
	lcdTitle2 = []byte("GREEN TO PLAY")
	lcdOver1  = []byte("GAME OVER")
	lcdName1  = []byte("TOP 10!")
	lcdName2  = []byte("YOUR INITIALS")

	// The hearts come from the engine's built-in glyph table — no
	// definition needed, they have been there for years. A rune in NEITHER
	// table (not the engine's, not this game's) renders as "?".
	lcdLives = [4][]byte{
		[]byte("LIVES"),
		[]byte("LIVES ♥"),
		[]byte("LIVES ♥♥"),
		[]byte("LIVES ♥♥♥"),
	}

	// lcdScore is the one line that is formatted at runtime, into a
	// fixed buffer.
	lcdScore [16]byte
)

// The two prefixes lcdScore is built with. In play the basket names the
// number without spending four of sixteen columns on the word — the panel
// is already showing the basket, so the LCD reads as the same object's
// tally. On the game-over screen the run is over and the word is the point,
// so it says SCORE.
const (
	scorePrefixPlaying = "֍ "
	scorePrefixOver    = "SCORE "
)

func (g *catcherGame) tickLCD() {
	if g.lcdAcc < lcdIntervalMicros {
		return
	}
	g.pushLCD()
}

// pushLCD writes the current screen's readout, unconditionally. Every
// transition calls it directly, so the display is never left showing the
// previous screen's text.
func (g *catcherGame) pushLCD() {
	g.lcdAcc = 0
	if g.seat == cartlib.SeatNone {
		return // a keyboard player has no seat, and no LCD in front of them
	}
	seat := g.seat
	switch g.machine.State() {
	case stTitle:
		cartlib.SetTextBytes(seat, lcdTitle1, lcdTitle2)
	case stPlaying:
		n := formatScore(scorePrefixPlaying, g.sim.score)
		cartlib.SetTextBytes(seat, lcdScore[:n], lcdLives[clampLives(g.sim.lives)])
	case stOver:
		n := formatScore(scorePrefixOver, g.sim.score)
		cartlib.SetTextBytes(seat, lcdOver1, lcdScore[:n])
	case stName:
		cartlib.SetTextBytes(seat, lcdName1, lcdName2)
	}
}

func clampLives(lives int) int {
	if lives < 0 {
		return 0
	}
	if lives > 3 {
		return 3
	}
	return lives
}

// formatScore writes "<prefix><n>" into lcdScore and returns its length in
// BYTES, which is what SetTextBytes wants. Hand-rolled because a cartridge
// avoids fmt: it costs binary size and drags reflection in behind it. Note
// that a prefix carrying a custom glyph is more than one byte per column —
// the basket is two, the engine's heart three — so size the buffer by
// bytes, not by the LCD's sixteen columns.
func formatScore(prefix string, v int) int {
	n := copy(lcdScore[:], prefix)
	if v < 0 {
		v = 0
	}
	var digits [10]byte
	i := len(digits)
	for {
		i--
		digits[i] = byte('0' + v%10)
		v /= 10
		if v == 0 {
			break
		}
	}
	return n + copy(lcdScore[n:], digits[i:])
}

// showPlayingLights is eligibility lighting in its purest form: the blue
// D button is lit exactly while a dash is available and goes dark the
// instant one is spent. only=true means the other five buttons are
// turned off in the same call, so the lamp table always tells the whole
// truth.
func (g *catcherGame) showPlayingLights() {
	if g.seat == cartlib.SeatNone {
		return
	}
	mask := cartlib.ButtonMask(0)
	if g.sim.dashReady() {
		mask = cartlib.MaskJoyD
	}
	cartlib.SetButtonLights(g.seat, mask, true)
}

// showTitlePeripherals lights green (start) on BOTH seats: the title
// invites either controller, and the seat the run locks to is whichever
// one presses.
func showTitlePeripherals() {
	cartlib.SetText(cartlib.SeatBoth, "CATCHER ֍", "GREEN TO PLAY")
	cartlib.SetButtonLights(cartlib.SeatBoth, cartlib.MaskJoyA, true)
}

func showNameLights(seat cartlib.Seat) {
	if seat == cartlib.SeatNone {
		return
	}
	cartlib.SetButtonLights(seat, cartlib.MaskJoyA|cartlib.MaskJoyB, true)
}

// showNoLights darkens the seat's panel for the game-over banner. Nothing
// the player can press does anything for its two and a half seconds, so
// by the eligibility rule nothing is lit: a mask of 0 with only=true. Part
// 2 replaces this one call with a lamp cue played from a file.
func showNoLights(seat cartlib.Seat) {
	if seat == cartlib.SeatNone {
		return
	}
	cartlib.SetButtonLights(seat, 0, true)
}

// showConfirmPeripherals puts up the shared quit legend. The LCD is
// monochrome text, so it cannot draw a coloured disc the way the panel
// can — it names the colour instead and lets the lit lamp identify the
// button. Never a letter, on either surface.
func showConfirmPeripherals(seat cartlib.Seat) {
	if seat == cartlib.SeatNone {
		return
	}
	cartlib.SetText(seat, ui.HoldConfirmLCD1, ui.HoldConfirmLCD2)
	cartlib.SetButtonLights(seat, cartlib.MaskJoyB, true)
}
```

The 16×2 character LCD in front of each seat is a free second screen,
and the rule for it is that **the panel is the playfield and the LCD is
the HUD**. It never duplicates a panel affordance; it says the things
that are awkward to draw. It also cannot legibly redraw at frame rate —
about 4 Hz is the budget — so the readout is throttled, and every screen
transition pushes immediately so it is never left stale.

#### The basket is a glyph this game drew itself

An HD44780 panel draws ASCII out of a mask ROM and nothing else. Anything
pictorial has to be uploaded to the chip first as a 5×8 bitmap, and the
engine ships a table of those for the runes games kept asking for — the
hearts on the lives line are one of them.

The basket is not. `defineGlyphs` hands the panel eight bytes and a code
point, and from then on `֍` is a character this cartridge can print:

```go fragment
var basketRows = cartlib.GlyphRows{
	0x00, // . . . . .
	0x0e, // . # # # .   handle
	0x11, // # . . . #   handle ends
	0x1f, // # # # # #   rim
	0x15, // # . # . #   weave
	0x1f, // # # # # #   weave
	0x0e, // . # # # .   base
	0x00, // . . . . .
}

s.DefineLCDGlyph('֍', basketRows) // s is Init's *cartlib.Setup
```

```lcd glyph
glyph ֍ 00 0e 11 1f 15 1f 0e 00
```

*The eight rows, blown up: the handle's arch, the rim, the weave, the
base — five pixels wide, eight tall, exactly what the chip's CGRAM
holds.*

Read the hex down the left and the picture is right there. Bit 4 is the
leftmost of the five pixels, so `0x11` is "both edges lit, middle dark".

The title line is the first place it shows. `lcdTitle1` is `CATCHER ֍`
over `GREEN TO PLAY`, and once the run starts the same rune becomes the
score's prefix — the panel is already showing the basket, so the LCD
reads as the same object's tally without spending four of sixteen
columns on the word:

```lcd
glyph ֍ 00 0e 11 1f 15 1f 0e 00
CATCHER ֍
GREEN TO PLAY
```

Three things are worth knowing before you reach for it:

- **`Init` is where it goes**, and the definitions die with the
  cartridge. Nothing you define can be seen by the shell, the attract
  loop or the next game.
- **Pick a plain, single-width, monochrome character that MEANS what
  you are drawing** — never an emoji: terminals draw those in colour and
  two cells wide, and the hosts refuse them. `֍` (U+058D, the Armenian
  eternity sign) is narrow, monochrome and already reads as woven, which
  is why it is the basket; `🧺` would have been refused. `DefineLCDGlyph`
  returns `false` when it refuses, and it refuses: code points below
  U+0080 (the chip already draws ASCII, and redefining a letter would
  spend a scarce slot on every line containing it); emoji and pictographs
  (all of U+1F000–U+1FFFF, the emoji-presentation symbols such as ⚡ ⭐ ✅,
  variation selectors and the zero-width joiner); East Asian wide runes
  (CJK, Hangul, the fullwidth forms, everything from U+20000 up);
  surrogates and noncharacters; and a 33rd distinct rune. `♥`, `☺`, `✔`,
  arrows, box drawing and shades are all fine — that is where the
  engine's own table lives.
- **Defining a glyph reserves nothing.** What is scarce is the chip's
  eight CGRAM slots, and that budget is per screen and per moment,
  shared between your glyphs and the engine's. The playing screen here
  spends two of the eight: this basket on line 1, the engine's heart on
  line 2. The number to watch is how many *different* custom runes can be
  on one panel together — not how many you defined (you get 32).

Before this existed, a game that wanted one character the engine had
never heard of was stuck behind an engine change and a cabinet rebuild.
Reach for the engine's palette when every game wants a rune; define
your own when one game does.

A glyph can also ship as a **file** instead of a call:
`assets/glyphs/<name>.arkadeglyph.json`, drawn in glyphlab, listed under
the manifest's `assets.glyphs`, and registered by both hosts at mount —
before `cart_init`, so the first frame can already print it — with no
loading code in the cartridge at all. Files and `DefineLCDGlyph` calls
land in the same per-cartridge table under the same 32-rune cap, and a
call in `Init` runs after the bundle and wins for a rune both name.
Pick the file when the glyph is art someone will want to open in a tool
and edit — a themed alphabet, a set of dice faces — or when it must
exist before `Init`; keep `DefineLCDGlyph` for a single rune that
belongs beside the code that prints it, like this basket. [Glyph
reference](reference-glyph.md) has the format.

The six lit arcade buttons follow one rule, and it is the reason this
game has a dash at all: **light a button exactly when pressing it would
do something**. A statically lit button reads as a mystery; an unlit one
reads as a hidden control. Blue is lit while a dash is available and
dark while it is not, and pressing it while dark rings
`cartlib.RejectKey` — the cabinet's polite "that does nothing" bell —
rather than being silently dropped.

Note also what the LCD does *not* do: it never names a button by letter.
The cabinet's buttons are round, coloured and completely unlettered, so
"PRESS A" names a control the player cannot find. On the panel you draw
the button ([`ui.HintRow`](ui.md#button-hints) does it in the real
colours); on the LCD you name the action and let the lit lamp identify
the button, spelling a colour word only where no lamp is available.

### Play it properly

```sh
make build && cd ../../arkade && ./arkade --cartridges ../my-cartridges/catcher --app cart/catcher
```

Title screen, a run, a game over, and — if the table has room — your
initials. Hold Escape (the terminal's stand-in for B) mid-game and the
quit overlay fills.

It is also completely silent, and the catcher is still a rectangle.

### Test the screens

The screens call the host — the LCD, the lamps, the score table — so
the plain unit test above cannot reach them. `sdk/cartlib/hosttest` can:
`hosttest.New(t)` installs an in-process fake cabinet for one test,
`h.Init` runs your real `Init` with a live `*cartlib.Setup`, and
`h.Input` and `h.Tick` drive the frame loop the way the cabinet does.
What the game wrote lands on the `Host` where a test can read it.

```go file=catcher_screens_test.go step=3
// SPDX-License-Identifier: GPL-2.0-or-later

package main

// The screens make host calls, so these tests install hosttest's in-process
// fake cabinet and drive the real Game through it: no TinyGo, no wasm.

import (
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/cartlib/hosttest"
)

const frameMicros = 33_333 // the cabinet's 30 fps

// startCatcher is a fresh game on its title screen, load phase done.
func startCatcher(t *testing.T) (*hosttest.Host, *catcherGame) {
	h := hosttest.New(t)
	g := &catcherGame{}
	h.Init(g)
	h.AdvancePastSplash()
	return h, g
}

func TestGreenStartsARunThatEndsOnGameOver(t *testing.T) {
	h, g := startCatcher(t)
	if line1, _, _ := h.LCD(cartlib.SeatP1); line1 != "CATCHER ֍" {
		t.Fatalf("title LCD line 1 = %q, want %q", line1, "CATCHER ֍")
	}

	h.Input(hosttest.Press(cartlib.SeatP1, cartlib.KeyJoyA)...)
	if g.machine.State() != stPlaying {
		t.Fatalf("green at the title left state %d, want stPlaying", g.machine.State())
	}

	// a catcher nobody steers misses its way to the end of the run
	for i := 0; i < 3000 && g.machine.State() == stPlaying; i++ {
		h.Tick(frameMicros)
	}
	if g.machine.State() != stOver {
		t.Fatalf("an unsteered run never reached game over (state %d)", g.machine.State())
	}
	if line1, _, _ := h.LCD(cartlib.SeatP1); line1 != "GAME OVER" {
		t.Errorf("game-over LCD line 1 = %q, want %q", line1, "GAME OVER")
	}
}

func TestBMidRunOpensTheConfirmAndAnotherPressResumes(t *testing.T) {
	h, g := startCatcher(t)
	h.Input(hosttest.Press(cartlib.SeatP1, cartlib.KeyJoyA)...)
	h.Tick(frameMicros)
	playing, _, _ := h.LCD(cartlib.SeatP1)

	h.Input(hosttest.Press(cartlib.SeatP1, cartlib.KeyJoyB)...)
	h.Input(hosttest.Release(cartlib.SeatP1, cartlib.KeyJoyB))
	if g.machine.State() != stConfirm {
		t.Fatalf("B mid-run left state %d, want stConfirm", g.machine.State())
	}
	if line1, _, _ := h.LCD(cartlib.SeatP1); line1 == playing {
		t.Errorf("the confirm left the playing LCD up (%q)", line1)
	}

	h.Input(hosttest.Press(cartlib.SeatP1, cartlib.KeyJoyA)...)
	if g.machine.State() != stPlaying {
		t.Fatalf("a press over the confirm left state %d, want stPlaying", g.machine.State())
	}
	// Pop re-enters the covered screen, and entering re-asserts its LCD
	if line1, _, _ := h.LCD(cartlib.SeatP1); line1 != playing {
		t.Errorf("LCD line 1 after resuming = %q, want the playing line %q", line1, playing)
	}
	if h.Exits != 0 {
		t.Errorf("a cancelled confirm still asked to exit %d time(s)", h.Exits)
	}
}
```

The second test is the modal stack from [the way out](#the-way-out),
asserted: the overlay is pushed over the run, and popping it re-enters
the playing screen, whose `enterScreen` arm puts its LCD line back.
`go test ./...` runs both beside the rules, still without TinyGo or a
`.wasm`. The fake answers as a bare `.wasm` would, so a sheet or a
sound the bundle would carry is simply absent — exactly the case
`sprites.go` and `loadSounds` already handle.

## What you have

Four hundred-odd lines, a complete game loop, and every golden rule
observed. What is missing is everything you can see and hear.

**Next: [sprites and sound from scratch](tutorial-assets.md)** — draw
the catcher as a three-cel walk cycle in spritelab, synthesize the
catch, miss, dash and game-over sounds in soundlab, write a music loop
in the loop pad, and wire all of it into the build.
