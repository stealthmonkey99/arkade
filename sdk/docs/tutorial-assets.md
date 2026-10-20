# Tutorial part 2: sprites and sound from scratch

[Part 1](tutorial.md) left you with a working game made of rectangles
that plays in complete silence. This part draws it and gives it a voice,
using the platform's own tools: **spritelab** for pixel art
and **soundlab** for synthesis.

Neither tool exports finished game assets on its own, and that is
deliberate. spritelab writes PNGs and soundlab writes WAVs and Go
snippets; a *generator* in your cartridge turns those into the things
the build consumes. The PNGs and the snippets are the source of truth,
they are hand-editable, and regenerating from them is a reviewable
diff.

## Open the lab

Open `arkade.build/dev/tools`. The seven tools — spritelab, soundlab,
glyphlab, hidlab, periphlab, levellab and fsmlab — are compiled to wasm
and keep their files in the browser, so each has **Import** and
**Export** buttons: import the files you want to edit (the catcher's
`assets/sprites/` PNGs for spritelab, its `assets/` sounds for
soundlab), edit them, and export them back into the cartridge's
directory.

## Draw the fruit

Start with the easy one. In **spritelab**, the
left column has a **New file** box. Type `fruit.png` — the whole name,
extension included — set the size to `5` × `5`, and press **Create**.

The extension is a choice, and the box's placeholder shows the other
one: `untitled.arkadesprite.png`. A bare name (`fruit`) becomes
`fruit.arkadesprite.png`, a **sheet** — the platform's asset-file form
of a sprite, one cel to start with and room for as many more as you
add. Spelling out `fruit.png` still means what it always did: an
ordinary picture. The fruit is one drawing that never animates, so it
gets the plain name; the catcher, below, does not.

The toolbar across the top of the canvas has six tools — **Pencil**
(`P`), **Fill** (`G`), **Line** (`L`), **Rect** (`R`), **Pick** (`I`)
and **Anchor** (`A`, which is for sheets and can wait) — plus
**Undo**/**Redo**, a **zoom** box (`[` and `]`), a **mirror** checkbox
(`M`), and **guides** with a spacing box. There is no eraser: the first
swatch in the palette is transparent, and painting with it is how you
rub out.

On the right, the **Colour** panel is the one to understand before you
draw anything. Under the R/G/B/A boxes are two buttons, **A=255** and
**A=254**, and the note beside them is the whole rule:

> **A=255** is the collision mask. *A<255* draws but never collides.

That is not a spritelab convention, it is the platform's, applied
wherever a PNG becomes a `ui.Sprite` (`ui.OpaqueMask` is its home, and
`Setup.LoadSheet` applies it at Init on the path this tutorial takes):
fully opaque pixels become the sprite's collision `Mask`, anything below
255 draws and nothing more. So an anti-aliased edge is automatically
forgiving, and a bit of art that should look solid but not *be* solid is
authored at A=254. Pixels below full alpha are shown hatched in the grid
so you can see which is which.

Draw a five-pixel disc — a red body around `#e8424a`, a lighter pixel at
the top left for a highlight, and a green stem pixel on the top row.

```panel zoom=24
tutorial/assets/sprites/fruit.png
```

*The fruit as the cabinet lights it: 5×5 pixels, one drawing.*

Press **Save** (or `Ctrl+S`), then **Export** to write the file back
into the cartridge's `assets/sprites/`. A plainly named file holding one cel with
nothing to add is written as an ordinary PNG with no metadata at all —
byte for byte what spritelab wrote before sheets existed. (The
**Import**/**Export** buttons exist because a browser tab has no
directory to save into.)

## Draw the catcher as an animation strip

Now the interesting one. **New file**, type just `catcher` — no
extension — `7` × `9`, **Create**. The file bar above the canvas shows
what you got: `catcher.arkadesprite.png`, a sheet. That is the right
form for anything with more than one drawing in it, and spritelab holds
you to it — a plainly named `catcher.png` would refuse to save the moment
it held a second cel, telling you to rename it to exactly this.

Under the canvas is the **cel row** — one thumbnail so far, a cel called
`0` — with its controls above it: **+ Cel** and **Duplicate**,
**Remove**, **◀** and **▶** to reorder, a **name** box, a **size** box,
an **anchor**, and an **onion** checkbox that ghosts the previous cel
underneath. Rename the cel first: type `catcher.0` into **name** and
press Enter. Cel names are how a cartridge will ask for its art, and
**+ Frame** below counts up from whichever name it copies, so naming this
one now is what makes the next two come out as `catcher.1` and
`catcher.2` without a rename each.

Draw a figure holding a basket: the top three rows are the basket — a
light rim, hollow sides, a woven base — and below it a head, a body, and
two legs on the bottom two rows. Keep the basket's mouth a full seven
pixels wide; that rectangle is the catching surface the game already
collides against.

```panel zoom=18
tutorial/assets/sprites/catcher.png
```

*The finished strip: three 7×9 cels side by side in one 21×9 image —
standing, then each foot out in turn.*

The **mirror** checkbox is worth turning on for the basket and the body:
every stroke is reflected across the vertical centre, which is how a
symmetrical 7px sprite stops looking hand-wobbled.

Below the cel row is the **animation bar**: **+ Anim**, **Delete**, a
**name** box, a loop select (**cycle** / **once** / **ping-pong**), then
**+ Frame**, a **reuse a cel…** picker, and **Play**. Press **+ Anim**.
You get an animation called `frames` with one frame showing `catcher.0`;
rename it to `walk`. The frame appears as a chip — `1. catcher.0` — with
its own **hold** box in milliseconds, **◀**/**▶** to move it, and **×**
to drop it. Set that hold to `130`; frames you add from it inherit the
value.

The two controls beside it are the whole vocabulary, and their tooltips
say the difference out loud:

> **+ Frame** — a new frame on a *new* cel: a copy of this frame's
> drawing, ready to change.
>
> **reuse a cel…** — add a frame that shows a cel this sheet already
> has, instead of a new one.

A **cel** is a drawing; a **frame** is one showing of a cel for a hold
time. Adding a frame does not have to make a drawing — but usually you
press it because you want something to change, which is why it copies.

You want three cels that differ only in the legs, played as
**contact, pass, contact, pass** — that is what makes a three-drawing
walk read as a walk — so:

1. Press **+ Frame** twice. Frame 2 lands on a new cel `catcher.1` and
   frame 3 on `catcher.2`, both copies of your drawing, and the canvas
   follows the newest one.
2. Click the name on frame 2's chip (`2. catcher.1`) to put the canvas
   on that cel, turn **mirror** off, and move one foot outward by a
   pixel.
3. Click frame 3's chip and move the *other* foot outward instead.
4. Click frame 2's chip again and choose `catcher.0` from
   **reuse a cel…**. A fourth frame appears after it, showing the
   standing pose a second time — the same drawing, its own hold. The
   info line beside **Play** now reads
   `4 frames over 3 cels · 520 ms · cycle`.
5. Press **Play** (or Space). The animation runs in the **Preview**
   panel on the right while the canvas stays on the cel you are editing,
   so you can tune a foot and watch the gait at the same time. Turn on
   **LED mode** in that panel to see it at cabinet dot pitch.

`,` and `.` walk the canvas along the cel row; the **onion** box is the
other way to line a foot up against the cel before it.

**Save.** The status line answers with
`saved catcher.arkadesprite.png — 3 cels, 1 animation — packs to 21×9`.
spritelab lays same-height cels out in one row, in your order, so the
image is a 21×9 filmstrip — three 7×9 cels side by side — and the
layout rides in a `tEXt` chunk under the keyword `arkade`, the asset
envelope every lab tool writes ([Asset file
reference](reference-assets.md)). Reformatted for reading:

```text
{"arkade":1,"kind":"sprite","name":"catcher",
 "generator":{"tool":"spritelab","version":"2.0.0"},
 "cels":[{"name":"catcher.0","x":0,"y":0,"w":7,"h":9,"anchor":[0,0]},
         {"name":"catcher.1","x":7,"y":0,"w":7,"h":9,"anchor":[0,0]},
         {"name":"catcher.2","x":14,"y":0,"w":7,"h":9,"anchor":[0,0]}],
 "animations":[{"name":"walk","frames":[{"cel":"catcher.0","holdMs":130},
                                       {"cel":"catcher.1","holdMs":130},
                                       {"cel":"catcher.0","holdMs":130},
                                       {"cel":"catcher.2","holdMs":130}],
                "loop":"cycle"}]}
```

Metadata never costs pixels. Any image viewer still opens it as an
ordinary PNG; the chunk is simply ignored by everything that does not
know about it. [Sprite reference](reference-sprite.md) has every field.

> **What the checked-in cartridge carries.** `sdk/tutorial`'s copy of
> this file, `assets/sprites/catcher.png`, is the same 21×9 strip with
> the same three cels and walk in its envelope, under a plain name. The
> next step reads only its pixels.

```panel inline
tutorial/assets/icon.png
```

While you are here, draw the shelf icon too: **New file**, `icon.png` —
a plain name for a plain picture — `8` × `8`. It belongs in the
cartridge's `assets/` directory rather than `assets/sprites/`. Eight
pixels is not much — a fruit over a basket rim is enough.

## Step 4 — art instead of rectangles

Two PNGs, and the cartridge cannot see either of them. A `.png` sitting
in `assets/sprites/` is a file on your disk; the game needs pixels in
memory. A cartridge may compile `ui.Sprite` literals in, by hand or
with its own generator, so the pixels ride inside the wasm. Catcher
takes the other way across the gap.

It puts the art in the **bundle**. One
packed sheet travels in the `.rcart` beside the wasm, the host decodes it
once when the game starts, and the cartridge asks for the drawings it
wants by name — the way a console reads its ROM. Two things get better:
the wasm stays small, and the art can be re-exported without rebuilding
the binary.

### Pack the sheet

`tools/gen-sheet` is a small program that lives in the cartridge. It is
not listed here: copy it from the finished cartridge, from inside yours.

```sh
mkdir -p tools
cp -r ../../arkade/sdk/tutorial/tools/gen-sheet tools/
```

It is the first host-side code in the cartridge, and it imports the
engine module itself (`assetfile`, the envelope reader and writer), so
`go.mod` gains the engine. The engine resolves `sdk/levels` by a
`replace` of its own, and `replace` directives are not transitive, so
that one comes along too:

```go-mod
require github.com/stealthmonkey99/arkade v0.1.0

replace github.com/stealthmonkey99/arkade => ../../arkade
replace github.com/stealthmonkey99/arkade/sdk/levels => ../../arkade/sdk/levels
```

`go mod tidy` then fills in the engine's own dependencies and writes
`go.sum`. None of this reaches the TinyGo build: only host-side code
(this tool, the next one, and part 4's golden test) imports the engine.

Give each generator a target in the `Makefile`, and list them as
`.PHONY` — `sheet` and `sfx` share their names with directories under
`tools/`, and without that line `make` reports them up to date and does
nothing:

```make
.PHONY: build test rcart clean sheet sfx

sheet:
	go run ./tools/gen-sheet

sfx:
	go run ./tools/gen-sfx
```

Then run it:

```sh
make sheet
```

```text
gen-sheet: wrote assets/catcher.arkadesprite.png (26x9, 4 cels, 847 bytes)
```

That name is the platform's convention, not this game's:
`<name>.arkade<kind>.<container>` (see
[reference-assets.md](reference-assets.md)) — the same rule spritelab
applied when it named your file. It is an ordinary PNG — 26×9, your
three catcher cels and the fruit laid out in a row, openable in any
image viewer — carrying the same `arkade` envelope spritelab wrote, this
time spanning both source files and saying what every rectangle is
called:

```text
{"arkade":1,"kind":"sprite","name":"catcher",
 "cels":[{"name":"catcher.0","x":0,"y":0,"w":7,"h":9,"anchor":[0,0]},
         {"name":"catcher.1","x":7,"y":0,"w":7,"h":9,"anchor":[0,0]},
         {"name":"catcher.2","x":14,"y":0,"w":7,"h":9,"anchor":[0,0]},
         {"name":"fruit","x":21,"y":0,"w":5,"h":5,"anchor":[0,0]}],
 "animations":[{"name":"walk","frames":[{"cel":"catcher.0","holdMs":130},
                                       {"cel":"catcher.1","holdMs":130},
                                       {"cel":"catcher.0","holdMs":130},
                                       {"cel":"catcher.2","holdMs":130}],
                "loop":"cycle"}]}
```

A **cel** is one drawing; a **frame** is one showing of one cel for a
hold time. Four frames off three drawings — `catcher.0` plays twice —
which is why the two are separate lists.

**Cel names are your art's API.** `render.go` is about to ask for
`"catcher.0"` and `"fruit"`, so renaming one in `gen-sheet` is a change
the cartridge has to agree to — which is exactly the visibility a name
should buy. Commit both the source PNGs and the packed sheet; the PNGs
stay the thing you edit, and re-running the generator over unchanged
PNGs rewrites byte-identical output.

`gen-sheet` reads pixels, never chunks: it decodes each source PNG and
cuts the catcher strip at 7px, which is why the checked-in strip and
the sheet you saved from spritelab are interchangeable to it. Its
`sources` table names the file it expects under `assets/sprites/` —
`catcher.png`, the checked-in copy — so if yours is
`catcher.arkadesprite.png`, that one `file:` entry is the only thing to
point at it.

### Read it at Init

`Setup.LoadSheet` is the whole of the runtime half. Create
`sprites.go`:

```go file=sprites.go step=4
// SPDX-License-Identifier: GPL-2.0-or-later

package main

// Catcher's art, read out of the cartridge's own bundle at Init — the
// worked example of "sprites from the bundle" (docs/ASSETS.md;
// sdk/docs/reference-sprite.md; the tutorial page is
// sdk/docs/tutorial-assets.md).
//
// # Where the pixels live
//
// One packed sheet — assets/catcher.arkadesprite.png, written by
// tools/gen-sheet from assets/sprites/*.png — travels in the .rcart, so
// the pixels stay out of catcher.wasm. The host decodes it once at
// host_load_sheet, and Setup.LoadSheet lifts the cels this cartridge wants
// into ui.Sprite values here: ordinary ui.Sprites with ordinary collision
// masks, drawn and collided guest-side, under ui.OpaqueMask's alpha rule
// (A==255 collides).
//
// # Why loading happens exactly once
//
// A cartridge is built with -gc=leaking, so every allocation is permanent
// (GOLDEN RULE 7). LoadSheet and Sheet.Cel allocate — pixels and mask rows,
// once per cel — which is why they hang off the Setup Init receives and
// nothing else. Everything below is a package var holding the result.

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// sheetPath is the bundle-relative path the host resolves, in the same
// path space Setup.LoadSound uses. Regenerate the file it names with
// `make sheet` after editing assets/sprites/*.png.
const sheetPath = "assets/catcher.arkadesprite.png"

// blank stands in for any cel the bundle could not supply — a dev build
// run from a bare .wasm with no .rcart around it, most often. A zero
// ui.Sprite draws nothing and collides with nothing, so the game keeps
// running (soundless, artless) instead of trapping on a nil sprite in the
// middle of a frame.
var blank ui.Sprite

// spriteFruit is the falling fruit, and the still life on the title.
var spriteFruit = &blank

// walkCels are the four STEPS of the catcher's walk cycle, in play order —
// note steps, not cels: step 0 and step 2 are the same drawing
// ("catcher.0", the standing pose), which is why the cycle reads as a
// stride rather than a shuffle.
//
// The sheet's envelope carries this animation too, under the name "walk",
// for spritelab's benefit. The cartridge keeps its own copy rather than
// asking the host for it because the host deliberately exposes cels and not
// animations: how long a cel is held is a question about THIS game's clock,
// and a fixed array of four is something -gc=leaking is happy with.
var walkCels = [4]*ui.Sprite{&blank, &blank, &blank, &blank}

// walkHoldMicros is how long each STEP is held. Copied from the envelope's
// holdMs (130ms a step), in microseconds so advance() can compare against
// accumulated dtMicros without dividing.
var walkHoldMicros = [4]uint64{130_000, 130_000, 130_000, 130_000}

// loadSprites lifts every cel this cartridge draws out of the bundle.
// Called once, from Init, beside loadSounds and defineGlyphs.
func loadSprites(s *cartlib.Setup) {
	sheet := s.LoadSheet(sheetPath)
	if !sheet.Ok() {
		// The host has already logged why. Every sprite stays blank and
		// the game runs on without art rather than not at all.
		cartlib.Log(cartlib.LogWarn, "catcher: no sprite sheet at "+sheetPath)
		return
	}
	spriteFruit = celOr(sheet, "fruit")

	// The walk's step order, spelled out: stand, stride, stand, stride
	// again on the other foot.
	stand := celOr(sheet, "catcher.0")
	walkCels = [4]*ui.Sprite{stand, celOr(sheet, "catcher.1"), stand, celOr(sheet, "catcher.2")}
}

// celOr lifts one named cel, falling back to the blank sprite so no call
// site ever has to hold a nil. A missing cel is a mismatch between this
// file and tools/gen-sheet's cel names, and the host logs it by name.
func celOr(sheet cartlib.Sheet, name string) *ui.Sprite {
	if s := sheet.Cel(name); s != nil {
		return s
	}
	return &blank
}
```

Three things in there are worth slowing down for.

**It is an ordinary `ui.Sprite`.** `Sheet.Cel` hands back the same type
a compiled-in literal would be — pixels plus a collision mask — so
drawing stays guest-side, collision stays guest-side, and a golden test
can still see every pixel.

**The alpha rule is the same rule.** The A=255 note from the spritelab
panel above is applied here, by `cartlib` through `ui.OpaqueMask`. Fully
opaque pixels are the collision `Mask`; A<255 draws and nothing more.

**Init only.** `LoadSheet` and `Cel` allocate — pixel buffers and mask
rows — and a cartridge runs under `-gc=leaking`, where nothing is ever
freed (GOLDEN RULE 7). Load once, keep the `*ui.Sprite` values in
package vars, never call either function again. That is why every sprite
below starts as `&blank`: a game whose bundle is missing draws nothing
rather than dereferencing a nil pointer halfway through a frame.

The animation table is hand-written rather than read back out of the
sheet, and deliberately so. The envelope carries a `walk` animation for
spritelab's benefit, but the ABI hands out **cels, not animations**: how
long a drawing is held is a question about *this game's* clock, and a
fixed array of four is something `-gc=leaking` is perfectly happy with.

### Call it

`Init` gains one line — art before sound, both before anything draws:

```go file=game.go step=4
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
	// Art first: loadSprites reads the cartridge's sheet out of its own
	// bundle (sprites.go). Everything that loads or binds goes through s,
	// the Setup that is live only until Init returns — it allocates, and
	// -gc=leaking never gives that back.
	loadSprites(s)
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

Now `render.go` reaches its final form: sprites instead of rectangles,
the title screen, the game-over banner, the initials screen, and the
walk cycle driven off `walkCels`.

```go file=render.go step=4
// SPDX-License-Identifier: GPL-2.0-or-later

package main

// Every screen, drawn through the shared sdk/ui toolkit onto the
// cartridge's framebuffer.
//
// A *cartlib.Framebuffer IS a ui.Canvas, so widgets composite straight
// into the frame — and because the draw code targets ui.Canvas rather
// than the framebuffer type, a plain host `go test` can render any state
// into a ui.Layer and assert pixels without TinyGo or wasm.

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
	colDash   = ui.Color{R: 92, G: 150, B: 255, A: 255}
)

const groundY = catcherY + catcherH // 61: the line the catcher stands on

// footer is built ONCE, at package level. Hints is caller-owned, exactly
// like Menu.Items, and building a slice inside a render function would
// allocate a new one every frame.
//
// Note what the hints draw: the buttons themselves, as coloured discs in
// the cabinet's own button colours. Never a letter — the machine's
// buttons are round, coloured and completely unlettered, so "PRESS A"
// names a control the player cannot find.
var footer = ui.HintRow{
	Y:          56,
	LabelColor: ui.Color{R: 170, G: 176, B: 190, A: 255},
	Hints: []ui.Hint{
		{Text: "PLAY", Color: ui.ButtonGreen},
		{Text: "MOVE", Color: ui.White, Control: ui.HintStick},
	},
}

func (g *catcherGame) Render(fb *cartlib.Framebuffer) {
	// Render may run BEFORE the first Update — the engine's init render —
	// so every branch here has to draw a valid frame from whatever Init
	// alone set up.
	//
	// Under is the screen BENEATH the top of the stack, or the top itself
	// when nothing is layered over it. Switching on it means the overlay
	// composes for free: the frozen screen draws exactly as it always
	// does, and the overlay goes on top of it.
	switch g.machine.Under() {
	case stTitle:
		g.drawTitle(fb)
	case stPlaying:
		g.drawPlaying(fb)
	case stOver:
		g.drawPlaying(fb)
		g.drawOverBanner(fb)
	case stName:
		g.drawNameEntry(fb)
	}

	// The overlay draws LAST, over the frozen screen underneath it.
	if g.machine.State() == stConfirm {
		g.confirm.Draw(fb)
	}
}

func (g *catcherGame) drawPlaying(fb *cartlib.Framebuffer) {
	fb.Clear(colSky.R, colSky.G, colSky.B)
	ui.FillRect(fb, 0, groundY, cartlib.FBWidth, cartlib.FBHeight-groundY, colGround)

	for i := range g.sim.fruit {
		f := g.sim.fruit[i]
		if f.live {
			spriteFruit.Draw(fb, f.x, f.y())
		}
	}
	playerWalk.cel().Draw(fb, g.sim.x, catcherY)
	g.drawHUD(fb)
}

// drawHUD is the panel's own readout: score top left, lives top right.
// It does not repeat the LCD, and the LCD does not repeat it — the same
// numbers, but the panel is what a player watching the fruit can see
// without looking down.
func (g *catcherGame) drawHUD(fb *cartlib.Framebuffer) {
	small := ui.Font3x5{}
	// DrawUint draws a number straight from an integer, with no string
	// built and thrown away on the way.
	small.DrawUint(fb, 1, 1, uint32(g.sim.score), 1, colScore)

	for i := 0; i < g.sim.lives; i++ {
		ui.FillRect(fb, cartlib.FBWidth-3-i*4, 1, 2, 2, colLives)
	}

	// The dash meter: a short bar under the score that fills back up as
	// the cooldown runs out, so "why is blue dark" has a visible answer.
	if g.sim.dashReady() {
		ui.FillRect(fb, 1, 7, 8, 1, colDash)
		return
	}
	w := 8 - 8*g.sim.dashCool/dashCooldownSteps
	if w > 0 {
		ui.FillRect(fb, 1, 7, w, 1, colDim)
	}
}

func (g *catcherGame) drawTitle(fb *cartlib.Framebuffer) {
	fb.Clear(colSky.R, colSky.G, colSky.B)
	ui.FillRect(fb, 0, groundY, cartlib.FBWidth, cartlib.FBHeight-groundY, colGround)

	led := ui.FontLED{}
	w := led.TextWidth("CATCHER", 1)
	led.DrawText(fb, (cartlib.FBWidth-w)/2, 6, "CATCHER", 1, colAccent)

	// Three pieces of fruit sitting still above the catcher, so the
	// title screen says what the game is without a word of explanation.
	spriteFruit.Draw(fb, 14, 22)
	spriteFruit.Draw(fb, 30, 18)
	spriteFruit.Draw(fb, 45, 24)
	walkCels[0].Draw(fb, (cartlib.FBWidth-catcherW)/2, catcherY)

	// A one-row AttractPanel: the same widget the cabinet's screensaver
	// uses for a full leaderboard, sized down to a single high-score
	// line. Rows that would fall outside [Y, Y+H) are skipped whole.
	// T is the accumulated dtMicros — never a wall clock — which is what
	// lets a long name scroll without breaking golden reproducibility.
	// Elapsed is the machine's per-screen clock: time since the title was
	// entered, zeroed on the way in by the machine rather than by hand.
	if g.top.Score > 0 {
		ui.AttractPanel{
			X: 6, Y: 34, W: 52, H: 12,
			Top:    g.top,
			Trophy: true,
			Color:  colDim,
			T:      g.machine.Elapsed(),
		}.Draw(fb)
	}

	footer.X = (cartlib.FBWidth - footer.Width()) / 2
	footer.Draw(fb)
}

func (g *catcherGame) drawOverBanner(fb *cartlib.Framebuffer) {
	ui.Rect{
		X: 4, Y: 22, W: 56, H: 20,
		Fill:    ui.Color{R: 10, G: 10, B: 14, A: 235},
		Stroke:  colLives,
		StrokeW: 1,
	}.Draw(fb)

	led := ui.FontLED{}
	w := led.TextWidth("GAME OVER", 1)
	led.DrawText(fb, (cartlib.FBWidth-w)/2, 26, "GAME OVER", 1, ui.White)

	small := ui.Font3x5{}
	sw := small.TextWidth(small.NumDigits(uint32(g.sim.score)), 1)
	small.DrawUint(fb, (cartlib.FBWidth-sw)/2, 35, uint32(g.sim.score), 1, colScore)
}

func (g *catcherGame) drawNameEntry(fb *cartlib.Framebuffer) {
	fb.Clear(colSky.R, colSky.G, colSky.B)

	led := ui.FontLED{}
	w := led.TextWidth("TOP 10!", 1)
	led.DrawText(fb, (cartlib.FBWidth-w)/2, 8, "TOP 10!", 1, colScore)

	small := ui.Font3x5{}
	sw := small.TextWidth(small.NumDigits(uint32(g.sim.score)), 1)
	small.DrawUint(fb, (cartlib.FBWidth-sw)/2, 18, uint32(g.sim.score), 1, colDim)

	g.name.Draw(fb)
}

// playerWalk is the gameplay catcher's cycle. The walk lives with the
// drawing rather than in catcherGame because that is what it is — how
// long a cel is held is a property of the art, and the game only ever
// has to say whether the catcher is moving.
var playerWalk walker

// walker plays the catcher's walk cycle over walkCels — the four steps
// sprites.go lifted out of the bundle's sheet, with walkHoldMicros saying
// how long each is held. It has no clock of its own, deliberately: what a
// tick is worth is a per-game answer, so the cartridge advances it.
type walker struct {
	acc  uint64 // time spent on the current step
	step int    // index into walkCels / walkHoldMicros
}

func (w *walker) reset() { w.acc, w.step = 0, 0 }

// advance moves the cycle on while the catcher is moving and parks it on
// the standing cel when they stop.
func (w *walker) advance(dtMicros uint64, moving bool) {
	if !moving {
		w.reset()
		return
	}
	w.acc += dtMicros
	for {
		hold := walkHoldMicros[w.step]
		if hold == 0 || w.acc < hold {
			return
		}
		w.acc -= hold
		// The cycle loops, which is the only mode this game needs; the
		// envelope also spells "once" (clamp at the last step) and
		// "pingpong" (walk back down the order).
		w.step++
		if w.step >= len(walkCels) {
			w.step = 0
		}
	}
}

func (w *walker) cel() *ui.Sprite { return walkCels[w.step] }
```

A few things earn their place there:

- **`ui.Font3x5.DrawUint`** draws a number straight from an integer,
  with no string built and discarded on the way. Its sibling
  `ui.FontLED` is the cabinet's own variable-width font — the one the
  shell's menus use — and is what the titles are set in. Mind
  `Font3x5`'s coverage: it silently draws nothing for characters it
  lacks ([Fonts](ui.md#fonts)).
- **[`ui.AttractPanel`](ui.md#score-panels)** is the shared
  score block, and despite the name it is not attract-only: it draws
  strictly inside its own rectangle and never clears, so a one-row
  version is a perfectly good in-game high-score line. Its `T` field is
  an animation clock and must be **accumulated `dtMicros`, never a wall
  clock** — `Draw` is a pure function of the struct, which is exactly
  what keeps golden frames reproducible.
- **[`ui.HintRow`](ui.md#button-hints)** is built once at package level.
  `Hints` is caller-owned, like `Menu.Items`; building a slice inside a
  render function allocates a new one every frame, forever.

Rebuild and look at it. The catcher walks when you walk and stands still
when you stop, because the cel clock only advances while `moving` is
true.

> **On collision and masks.** Catcher compares rectangles, because its
> catching surface *is* a rectangle — the basket's mouth, the same three
> rows in every cel. When your art has an irregular silhouette, reach for
> `ui.Mask.Overlaps` instead: `Sheet.Cel` has already built a mask for
> every sprite it lifts, and mask-vs-mask collision is pixel-perfect.
> [Sprites and masks](ui.md#sprites-and-masks) and [Sprite art as
> PNGs](patterns.md#sprite-art-as-pngs) have the pipeline.

## Design the sounds

Over in **soundlab** there are four tabs:
**Listen**, **SFX**, **Loops** and **Compose**. The founding rule of the
whole tool is that *the browser never synthesizes* — every knob move
posts the parameters to Go and gets back the rendered WAV, its spectrum
analysis, and a Go snippet. What you hear and what you export cannot
disagree, because they are the same code.

Open **SFX**. The left column is presets. The first five —
`pluck`, `thud`, `sizzle`, `twang`, `chime` — are the **classic panel**:
one oscillator under one envelope, with the waveform chips (`sine`,
`square`, `triangle`, `saw`, `noise`, `wood`) and the knobs grouped as
**Pitch**, **Envelope**, **Colour**, **Echo** and **Output**. They are
five recognisably different corners of a parameter space rather than
five finished sounds, and the panel is the quickest way to find a pitch
and a length by ear.

`patch` is the form a cartridge's cue ships in. The stage
becomes a **stack**: one card per layer, top to bottom in the order they
are summed, each a source shaped by its own envelope and optionally gated
or filtered, with the whole sound's **duration** and **peak** beside
them. That is `sfxlib.Patch` — a sound written entirely as data, in a
grammar `sfxlib` publishes — and it is what the generator below writes,
because a patch can be reopened here and retuned months later and a
hand-written loop of oscillator calls cannot.
[The `patch` params](reference-sound.md#the-patch-params) is the
grammar in full; this part uses about a tenth of it.

### The catch

Press **patch**. It opens as a tap: a `body` under a `click` — a few
milliseconds of noise gated over the body's onset. That is the shape of
nearly every hit in the fleet, and the click is what makes a note read
as *contact* rather than as a beep. `sfxlib` builds it as `sfxlib.Tap`,
one of a handful of **templates** a cue starts from, and every cue in
the generator below starts from one.

On the body's card, pick **wood**. The note under that chip explains
what it is:

> A struck bar: the fundamental plus a partial at 2.756x it. That ratio
> is the physics of a real free-free bar, and it is the entire
> difference between wooden and metallic.

Set the body's **freq** to 880 Hz, the patch's **duration** to 0.13 s,
and the envelope's **rate** up to about 30 so the bar has died by the
time the sound ends. The catch sound wants to be **short**: a run of
five quick catches should read as five events, not as one smear.

Watch the spectrum card under the waveform while you do this. The
verdict line reads something like
`cabinet-ready — 0% below 150 Hz, 90% in 200-2000 Hz, peak 0.90`.

**The cabinet's speakers reproduce nothing below about 150 Hz.** A sound
with most of its energy down there plays perfectly through the entire
engine chain and is simply inaudible on the machine — the failure is
silent, and it has shipped. A wooden bar at 880 Hz has nothing down
there to lose; the miss is where the floor starts to bite.

### The miss and the rest

- **Miss** — one tone travelling, which is `sfxlib.Sweep`: a `square`
  at 420 Hz sliding to 190, with a **slide curve** below 1 so most of
  the drop happens at once (that is what makes a pitch drop read as an
  impact rather than a swoop), about 220 ms. Note what it is *not*: a
  real thud's fundamental would be under the floor, so the weight has
  to come from the slide. Set the layer's sweep mode to `phase`. The
  default, `pointwise`, overshoots on purpose — it is most of why the
  fleet's zaps sound energetic — but a *falling* pointwise sweep dives
  under its target, and 190 Hz has no room under it.
- **Dash** — `sfxlib.Scrape`: the `noise` chip through a band-pass
  centred near 1500 Hz. That band is what makes noise read as *air*
  rather than as static. Give it an `arch` envelope over 160 ms —
  silence at both ends, peak in the middle, the shape of something
  passing — in place of the template's strike.
- **Game over** — a `Sweep` again: a `triangle` at 660 falling to 220
  over 800 ms with a slide curve above 1, so it saves the drop for the
  end, and a slow, shallow vibrato so the fall reads as an instrument
  rather than as a siren. A game over deserves its own beat, sequenced
  after the miss that caused it rather than being a louder version of
  it.

### Export

At the top right of the stage is **Copy Go snippet**. Press it (the
snippet is also always visible in the panel under the spectrum, if
clipboard access is blocked). You get something like:

```text
// genCatch is catch: a 2-layer patch over 130 ms — body: wood at 880 Hz; click: noise, gated.
// Designed in soundlab. 130 ms, peak 0.90, 0% of its energy below 150 Hz,
// 90% in the 200-2000 Hz band the cabinet's speakers reproduce best.
func genCatch() []float64 {
	p := sfxlib.Patch{
		Duration: 0.13, Peak: 0.9, Seed: 0x65,
		Layers: []sfxlib.Layer{
			sfxlib.Layer{
				Name: "body", Wave: sfxlib.WaveWood, Freq: 880,
				...
			},
			...
		},
	}
	return p.Render(sfxlib.SampleRate44100)
}
```

The measured spectrum travels with the sound as a comment, and the
function is short because the synthesis is a library call rather than
fifty lines of inlined oscillator maths. That is the point: a snippet
that re-derived the synthesis would be a second implementation, free to
drift from the one you actually listened to — and the drift would only
turn up on the cabinet.

The generator below does not paste that literal in. It writes the same
patch as the template it came from plus the fields you changed —
`sfxlib.Tap(880, 30, 0.13)` with its body's wave set to wood — because
that spelling says which of the numbers were decisions. The two are the
same `Patch`; the literal is what you reach for once a cue has been
retuned far enough from its template that the edits outnumber the
defaults.

## A loop for the background

Open the **Loops** tab. This one renders at 22050 Hz and produces a
**seamlessly looping** WAV, which is a harder guarantee than "does not
click at the ends": the engine loops music verbatim, so the last sample
has to meet the first one continuously.

The transport controls are **tempo**, **bars**, **steps** (16 or 32), a
**root** note, and a **scale** select offering `hirajoshi`,
`major-pent`, `minor-pent` and `chromatic`. Hirajoshi is first and is
the default, for a reason the source states plainly: it is the one that
makes a four-note doodle sound *intended*. Its minor seconds do the
work; you do not have to be a composer to get something that sounds
deliberate out of it.

Set **tempo** to about 104, **bars** to 2, **root** to E4, scale
**hirajoshi**. The grid below has four tracks — `pulse 1`, `pulse 2`,
`triangle` and `noise` — each with a mute box, a gain slider and a
clear button; the pulses add a **duty** knob (0.125 is thin and nasal,
0.25 a bright lead, 0.5 a hollow square) and the noise track a **tone**
knob (negative is a lowpassed thump, positive a highpassed hat). The
melodic tracks have fifteen rows; the noise track has four, labelled
`hat`, `snare`, `tom` and `thump`.

> Click a cell to place a soft note, click again for an accent, again to
> clear. Drag to paint.

Put a simple rising-and-falling figure on `pulse 1`, two long notes per
bar on `triangle`, and a hat on every off-beat. Press **Play**, adjust,
and when it stops annoying you, **Copy Go snippet** — which this time
emits a `musiclib.Song` literal, every note carrying its name as a
comment.

## Wire the generator up

The snippets go into a small build-time program at
`tools/gen-sfx/main.go`. It is host-only code that never enters the
cartridge's wasm build, so it is free to use `math`, `sfxlib` and
`musiclib` — the golden rules about libm and allocation do not apply to
it.

```go file=tools/gen-sfx/main.go step=5
// SPDX-License-Identifier: GPL-2.0-or-later

// Command gen-sfx synthesizes Catcher's five audio assets into
// sdk/tutorial/assets. Run it from the cartridge directory:
//
//	go run ./tools/gen-sfx      # from sdk/tutorial
//
// This is host-only, offline, build-time code — never compiled into the
// .wasm — so it is free to use math, sfxlib and musiclib. The cartridge
// runtime's golden rules about libm and allocation do not apply here.
//
// # Where these numbers came from
//
// Every cue below is an sfxlib.Patch — a sound written as data, a stack
// of layers over one clock — started from one of sfxlib's templates and
// tuned by ear in soundlab's SFX tab
// (arkade.build/dev/tools). That is honest for a structural reason: the
// tab renders a patch through sfxlib.Patch.Render, the exact code this
// program renders with, so what you heard in the browser is what this
// program writes. A hand-written loop of oscillator calls would be a
// second implementation, free to drift — and one nothing could reopen.
//
// The theme is the same story one level up: soundlab's Loops tab exports
// a musiclib.Song literal, and musiclib.WriteLoopWAV renders it the way
// the loop pad auditioned it, with the loop seam guaranteed continuous.
//
// # Why every asset is asserted before it is written
//
// The cabinet's speakers reproduce nothing below about 150 Hz. A sound
// whose energy sits down there plays "correctly" through the entire
// engine chain and is simply inaudible on the machine — the failure is
// silent, and it has shipped. So the rule is that a generator
// asserts its own output's spectrum with a Goertzel band check and
// refuses to write an asset that fails: the bug class fails the build
// instead of reaching a cabinet.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/stealthmonkey99/arkade/tools/musiclib"
	"github.com/stealthmonkey99/arkade/tools/sfxlib"
)

const sampleRate = sfxlib.SampleRate44100

func main() {
	_, thisFile, _, _ := runtime.Caller(0)
	assets := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "assets"))
	if err := os.MkdirAll(assets, 0o755); err != nil {
		fatal(err)
	}

	write(assets, "catch.wav", genCatch())
	write(assets, "miss.wav", genMiss())
	write(assets, "dash.wav", genDash())
	write(assets, "gameover.wav", genGameOver())

	// The music bed goes through musiclib, which renders a seamless loop
	// rather than a one-shot — the engine loops the WAV verbatim, so the
	// last sample has to meet the first one without a click.
	//
	// Its spectrum check uses the FRAMED analysis, not the one the
	// transients above use. AssertSpectrum spot-samples Goertzel bins
	// every 20 Hz across the whole signal, which is right for a 130 ms
	// hit whose bins are that wide and quietly meaningless for a signal
	// seconds long, where the same grid steps straight over every peak.
	// FrameSignal restores the short-sound assumption by measuring Hann
	// windows spread along the loop.
	song := themeSong()
	theme := musiclib.RenderLoop(song, musiclib.SampleRate22050)
	frames := sfxlib.FrameSignal(theme, musiclib.SampleRate22050, 0, 0)
	if err := frames.AssertSpectrum(20, 10000, []sfxlib.SpectrumCheck{
		{Name: "sub-150Hz", Lo: 20, Hi: 150, Max: 0.5},
		{Name: "mid-200-2000Hz", Lo: 200, Hi: 2000, Min: 0.3},
	}); err != nil {
		fatal(fmt.Errorf("theme.wav: %w", err))
	}
	if err := musiclib.WriteLoopWAV(filepath.Join(assets, "theme.wav"), song, musiclib.SampleRate22050); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote theme.wav (%.1fs loop, %d notes, %d analysis frames)\n",
		song.LengthSeconds(), song.NoteCount(), frames.Count())
}

// genCatch is the pluck: a short struck-bar hit at 880 Hz, bright and
// dry, so a fast run of catches reads as a run rather than a smear. A
// Tap is a decaying body under a few milliseconds of noise; the only
// change is that the body is a wooden bar rather than a sine.
func genCatch() sfxlib.Patch {
	p := sfxlib.Tap(880, 30, 0.13)
	p.Layers[0].Wave = sfxlib.WaveWood
	return p
}

// genMiss is the thud: a square tone sliding down fast, the classic
// "you dropped it". The pitch stays inside the band the cabinet can
// actually reproduce — a real thud's fundamental would be inaudible
// there, so the weight comes from the slide, not from depth. The sweep
// is integrated rather than pointwise so the tone arrives at 190 Hz
// instead of diving under it.
func genMiss() sfxlib.Patch {
	p := sfxlib.Sweep(420, 190, 12, 0.22)
	p.Layers[0].Wave = sfxlib.WaveSquare
	p.Layers[0].SlideCurve = 0.45
	p.Layers[0].Sweep = sfxlib.SweepPhase
	return p
}

// genDash is the whoosh: noise through a resonance, band-limited hard
// so it reads as air rather than as static. A Scrape strikes; this one
// swells and fades instead, the shape of something passing.
func genDash() sfxlib.Patch {
	p := sfxlib.Scrape(1500, 1.2, 0.16)
	p.Layers[0].Env = sfxlib.Env{Kind: sfxlib.EnvArch}
	return p
}

// genGameOver is the sting: a longer falling triangle tone with a slow
// wobble on it. Structurally it is a separate asset from the miss that
// precedes it, because a game over deserves its own beat rather than a
// louder version of the thing that caused it.
func genGameOver() sfxlib.Patch {
	p := sfxlib.Sweep(660, 220, 4, 0.8)
	p.Layers[0].Wave = sfxlib.WaveTriangle
	p.Layers[0].SlideCurve = 1.6
	p.Layers[0].Sweep = sfxlib.SweepPhase
	p.Layers[0].Vibrato = &sfxlib.Vibrato{Hz: 6, DepthHz: 5}
	return p
}

// themeSong is the bed: a seamless 8-beat loop at 104 BPM in E
// hirajoshi, drawn on soundlab's Loops tab. Hirajoshi is the loop pad's
// default scale for a good reason — its minor seconds make a four-note
// doodle sound intended rather than random, which is exactly what a
// background loop nobody is listening to needs.
func themeSong() musiclib.Song {
	return musiclib.Song{
		BPM:   104,
		Beats: 8,
		Tracks: []musiclib.Track{
			{
				Name: "pulse1",
				Inst: musiclib.Instrument{Wave: musiclib.WavePulse, Duty: 0.25, AttackMs: 2, DecayMs: 60, SustainLevel: 0.35, ReleaseMs: 90, Gain: 0.85},
				Notes: []musiclib.Note{
					{Midi: 64, Start: 0, Dur: 0.5, Vel: 1},     // E4
					{Midi: 71, Start: 0.5, Dur: 0.5, Vel: 0.7}, // B4
					{Midi: 72, Start: 1, Dur: 0.5, Vel: 1},     // C5
					{Midi: 71, Start: 1.5, Dur: 0.5, Vel: 0.7}, // B4
					{Midi: 67, Start: 2, Dur: 1, Vel: 1},       // G4
					{Midi: 66, Start: 3, Dur: 1, Vel: 0.7},     // F#4
					{Midi: 64, Start: 4, Dur: 0.5, Vel: 1},     // E4
					{Midi: 67, Start: 4.5, Dur: 0.5, Vel: 0.7}, // G4
					{Midi: 71, Start: 5, Dur: 0.5, Vel: 1},     // B4
					{Midi: 72, Start: 5.5, Dur: 0.5, Vel: 0.7}, // C5
					{Midi: 76, Start: 6, Dur: 1, Vel: 1},       // E5
					{Midi: 71, Start: 7, Dur: 1, Vel: 0.7},     // B4
				},
			},
			{
				Name: "triangle",
				Inst: musiclib.Instrument{Wave: musiclib.WaveTriangle, AttackMs: 4, DecayMs: 80, SustainLevel: 0.5, ReleaseMs: 120, Gain: 0.7},
				Notes: []musiclib.Note{
					{Midi: 52, Start: 0, Dur: 2, Vel: 0.9}, // E3
					{Midi: 55, Start: 2, Dur: 2, Vel: 0.8}, // G3
					{Midi: 52, Start: 4, Dur: 2, Vel: 0.9}, // E3
					{Midi: 59, Start: 6, Dur: 2, Vel: 0.8}, // B3
				},
			},
			{
				Name: "noise",
				Inst: musiclib.Instrument{Wave: musiclib.WaveNoise, NoiseTone: 0.5, AttackMs: 1, DecayMs: 40, SustainLevel: 0, ReleaseMs: 30, Gain: 0.4},
				Notes: []musiclib.Note{
					{Midi: 72, Start: 0.5, Dur: 0.25, Vel: 0.6},
					{Midi: 72, Start: 1.5, Dur: 0.25, Vel: 0.6},
					{Midi: 72, Start: 2.5, Dur: 0.25, Vel: 0.6},
					{Midi: 72, Start: 3.5, Dur: 0.25, Vel: 0.6},
					{Midi: 72, Start: 4.5, Dur: 0.25, Vel: 0.6},
					{Midi: 72, Start: 5.5, Dur: 0.25, Vel: 0.6},
					{Midi: 72, Start: 6.5, Dur: 0.25, Vel: 0.6},
					{Midi: 72, Start: 7.5, Dur: 0.25, Vel: 0.6},
				},
			},
		},
		Echo: musiclib.Echo{DelayBeats: 0.75, Feedback: 0.25, Mix: 0.14},
	}
}

// write renders the patch, asserts its spectrum, then writes the WAV.
// The order matters: nothing reaches assets/ that has not been proven
// audible on the hardware it is for.
func write(dir, name string, p sfxlib.Patch) {
	samples := p.Render(sampleRate)
	if err := sfxlib.AssertSpectrum(samples, sampleRate, 20, 10000, []sfxlib.SpectrumCheck{
		{Name: "sub-150Hz", Lo: 20, Hi: 150, Max: 0.5},
		{Name: "mid-200-2000Hz", Lo: 200, Hi: 2000, Min: 0.3},
	}); err != nil {
		fatal(fmt.Errorf("%s: %w", name, err))
	}
	path := filepath.Join(dir, name)
	if err := sfxlib.WriteFile(path, samples, sampleRate); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s (%d samples, %.0fms)\n", name, len(samples), 1000*float64(len(samples))/sampleRate)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "gen-sfx:", err)
	os.Exit(1)
}
```

Two things there are the platform's rules rather than this game's.

**No asset is written without its spectrum being asserted first.**
`sfxlib.AssertSpectrum` is a Goertzel band-energy check, and the
generator refuses to write a sound that fails it. The bug class — a
perfectly synthesized sound that no one can hear — fails the build
instead of reaching a cabinet.

**The music uses the framed analysis, not the plain one.**
`AssertSpectrum` spot-samples bins every 20 Hz across the whole signal,
which is right for a 130ms transient whose DFT bins are about that wide
and quietly meaningless for a signal seconds long, where the same grid
steps straight over every peak. `sfxlib.FrameSignal` restores the
short-sound assumption by measuring Hann windows spread along the loop.

Run it:

```sh
make sfx      # or go run ./tools/gen-sfx, from ../my-cartridges/catcher
```

```text
wrote catch.wav (5733 samples, 130ms)
wrote miss.wav (9702 samples, 220ms)
wrote dash.wav (7056 samples, 160ms)
wrote gameover.wav (35280 samples, 800ms)
wrote theme.wav (4.6s loop, 24 notes, 16 analysis frames)
```

`peripherals.go` has been asking for those five files since part 1 —
`Setup.LoadSound` on a missing asset is a safe no-op that warns once,
which is why the game has been running silently rather than crashing.
Now they exist.

Commit the WAVs alongside the generator. A cartridge synthesizes its own
audio and never ships a raw external sample, and committing the output
means a build uses exactly the bytes that were reviewed.

**Plain `.wav`, on purpose.** soundlab's own **Save** button writes
something slightly richer: `<name>.arkadesound.wav`, the same 16-bit
audio with one extra RIFF chunk, `arkd`, carrying the asset envelope —
the sound's name, the tool that rendered it, and the recipe. For a patch
the recipe *is* the patch, layer by layer, and that is what lets the
tool reopen a cue six months later with every knob where you left it,
and what lets soundlab (hosted at
[arkade.build/dev/tools](https://arkade.build/dev/tools)) re-render one
from its own recipe, with no generator involved. The cartridge cannot tell the two apart:
`host_load_sound` never reads the chunk, and `Setup.LoadSound` loads
either by path.

Catcher writes plain `.wav` because `peripherals.go` has been asking for
`catch.wav` by that name since part 1, and because the `Patch` values
above are its recipe, checked in beside the output and rendered by the
same `sfxlib` soundlab uses. Life and Snake go the one step
further, and a new cartridge should: `sfxlib.WriteAssetFile` in place of
`WriteFile`, `sfxlib.SoundFileName(name)` for the path, and
`p.MarshalParams(name)` as the recipe node's `Params`, so the file
carries what made it and soundlab opens it without your generator. The
swap moves no samples. The
[Sound reference](reference-sound.md#generating-one) has the chunk's
byte layout, the recipe's fields, and that swap spelled out; a sound a
mouth would *say* — an eat cue, an "ouch" — is the other published
grammar, a [`vocal`](reference-sound.md#the-vocal-params), and rides in
the file the same way.

## A lamp cue in a file

The panel's six buttons per seat light up, and the game has been driving
them by hand since part 1 — one `cartlib.SetButtonLights` per screen,
asserted in `enterScreen`. That is exactly right for STATE: blue is lit
while a dash is available and dark when it is not, and a state is one
call.

A FLOURISH is a different shape. "Flash the whole panel twice, then go
dark" is three more states and two timers, and writing it by hand means a
frame counter and a switch running inside `Update` for the rest of the
cartridge's life. So the platform has a fourth asset kind for it: a
`<name>.arkadelights.json` file the host walks for you, on the same tick
your game already runs on. The full spec is
[reference-lights.md](reference-lights.md); this is the short version.

Save this as `assets/lights/gameover.arkadelights.json`:

```json file=assets/lights/gameover.arkadelights.json
{
  "arkade": 1,
  "kind": "lights",
  "name": "gameover",
  "description": "Two flashes of the whole panel, then dark: the run is over.",
  "generator": { "tool": "periphlab", "version": "1.0.0" },
  "frames": [
    { "lamps": { "p1": 63, "p2": 63 }, "holdMs": 90 },
    { "lamps": { "p1": 0,  "p2": 0  }, "holdMs": 90 },
    { "lamps": { "p1": 63, "p2": 63 }, "holdMs": 90 },
    { "lamps": { "p1": 0,  "p2": 0  }, "holdMs": 90 }
  ],
  "loop": "once"
}
```

It is the sprite animation's frame model with lamp masks where the cels
would be: a list of states, each held for a while, played once or round
and round. `63` is all six lamps (bits 0–5, `joya` through `joyaux2` —
the same bit order `SetButtonLights` takes), and `0` is dark.

The manifest has to list it, because that list is also the NAMESPACE the
play call draws from — a sequence the manifest does not name cannot be
played at all, so a typo is a `rcartpack` error rather than a lamp that
quietly never lights. `manifest.json` gains an `assets` block (it sits
between `visibility` and `icon` in the reference copy, but order does
not matter):

```json
"assets": {
  "lights": ["assets/lights/gameover.arkadelights.json"]
},
```

And then the hand-rolled call becomes a play call. `peripherals.go` in
full — everything above the button-lights section is exactly as part 1
left it; the change is at the bottom, where `showNoLights` is gone and a
constant naming the file plus the play call stand in its place:

```go file=peripherals.go step=5
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

// lightsGameOver is the bundle path of the run's closing lamp cue, listed
// under this cartridge's manifest assets.lights — which is also the
// namespace host_lights_play draws from, so a typo here is a build error
// from rcartpack, not a lamp that quietly never lights.
const lightsGameOver = "assets/lights/gameover.arkadelights.json"

// playGameOverLights is the lamp half of the game-over beat, and it is a
// FILE rather than code (sdk/docs/reference-lights.md). The old version of
// this function was one SetButtonLights(seat, 0, true) — an instant dark
// panel — and the flourish that replaces it, two flashes of the whole panel
// before the dark, would have cost a timer, a frame index and a switch in
// Update to write by hand.
//
// The sequence names BOTH seats in every frame; passing this seat filters
// it to the one panel in front of the player, which is what the seat
// argument is for. It cannot MOVE a p1 cue onto p2 — a mask is per-seat
// data — so a cue meant for either seat has to say both, and this one does.
//
// The last frame is dark, so where the run leaves the panel is exactly
// where the hand-written call left it. That is deliberate: the flourish is
// an addition to the moment, not a change to the state after it.
func playGameOverLights(seat cartlib.Seat) {
	if seat == cartlib.SeatNone {
		return
	}
	cartlib.PlayLights(lightsGameOver, seat, cartlib.LightsLoopFile)
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

And `game.go`, where `enterScreen`'s `stOver` arm now calls
`playGameOverLights(g.seat)` in place of step 4's
`showNoLights(g.seat)` — the only line that moved:

```go file=game.go step=5
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
	// Art first: loadSprites reads the cartridge's sheet out of its own
	// bundle (sprites.go). Everything that loads or binds goes through s,
	// the Setup that is live only until Init returns — it allocates, and
	// -gc=leaking never gives that back.
	loadSprites(s)
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
		playGameOverLights(g.seat)
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

Three things in that small change are worth naming.

**The last frame is dark, and that is not decoration.** The call it
replaced left the panel dark; so does this one, four frames later. A
flourish is an addition to a moment, not a change to the state after it —
and if you ever want a cue to LEAVE something lit, say so in its last
frame, because `once` rests there exactly as a `once` sprite animation
rests on its last cel.

**The sequence names both seats and the call picks one.** Passing
`seat` filters the file's frames to the panel in front of this
player. It does not MOVE a `p1` cue onto `p2` — a mask is per-seat data,
and re-seating it would invent choreography nobody wrote — so a cue meant
for either seat has to say both, which is why every frame above does.

**You can always take the lamps back.** A `SetButtonLights` (or
`SetLights`) call on a seat stops whatever is playing there, in the same
call. That is the rule that keeps two writers from fighting at tick
resolution, and it means you never have to remember a `StopLights` before
writing a lamp by hand.

The editor for these files is **periphlab** — one of the lab's seven
tools (import the cartridge's `assets/lights/` files, edit, and export them
back). It choreographs against a real cabinet: toggle the six lamps per
frame, set each hold, and push the sequence down the dashboard relay to
watch it play on actual buttons before it is ever in a bundle.

## Step 5 — the screensaver slot

One thing left. The cabinet has an idle screensaver that gives each
cartridge a slot, and opting in takes two pieces: implementing
`cartlib.Attractor`, and a one-line export shim.

```go file=main.go step=5
// SPDX-License-Identifier: GPL-2.0-or-later

// Command catcher is the finished game from the Årkade SDK's
// follow-along tutorial: fruit falls, you walk under it, you catch it.
// Miss three and the run is over.
//
// Read sdk/docs/tutorial.md instead of reading this directory cold — the
// tutorial builds every one of these files a step at a time and explains
// why each line is the shape it is. This is the destination, checked in
// so you can diff your own copy against it.
//
// # Scope
//
//   - catcher.go: the simulation. Imports nothing, so catcher_test.go
//     covers the rules under plain `go test`.
//   - game.go: the cartlib.Game implementation — the sdk/fsm screen
//     machine and the transitions between its five screens.
//   - input.go: input decode, dispatch, and the hold-B confirm.
//   - render.go: every screen, drawn through sdk/ui onto the framebuffer.
//   - peripherals.go: sound, the per-seat LCD, and the button lights.
//   - attract.go: the cabinet's screensaver slot.
//   - sprites.go: the bundle's sprite sheet, lifted into ui.Sprites at Init.
//
// # Building
//
//	make build     # manifestgen + tinygo + wasmfix -> catcher.wasm
//	make test      # unit + screen + golden + determinism
package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

func (g *catcherGame) Manifest() []byte { return manifestBytes }

// game is the one instance, registered at the PACKAGE level. Not from
// inside main: a cartridge is hosted as a wasm reactor, main is never
// called, and the only pre-Init code that takes effect is what TinyGo's
// compile-time interpreter can fold into static data. See the cartlib
// reference, "Registration and the reactor model".
var game = &catcherGame{}
var _ = cartlib.Run(game)

// cart_attract opts Catcher into the cabinet's screensaver rotation.
// Implementing cartlib.Attractor is not enough on its own — the export
// shim is the other half. See attract.go.
//
//export cart_attract
func cartAttract(dtMicros uint64) { cartlib.DispatchAttract(dtMicros) }

// main is required by TinyGo's package-main convention and is never
// invoked. Keep it empty.
func main() {}
```

```go file=attract.go step=5
// SPDX-License-Identifier: GPL-2.0-or-later

package main

// Catcher's slot in the cabinet's idle screensaver.
//
// Attract runs in a DEDICATED cartridge instance that only ever sees
// cart_init, cart_attract and cart_teardown — no load phase, no Update,
// no HandleInput. So none of the game state built up by playing exists
// here; this file starts from nothing every time the screensaver comes
// around.
//
// Audio and the peripherals are fully live in that instance, which is
// why the convention is STRUCTURAL silence rather than a careful
// one: attract plays zero sounds and touches no LCDs or lights, and
// attract_test.go asserts the sound count is zero across a long run. A
// title sting, if a game wants one, is fired from a path attract never
// reaches (LoadFrame, not Init).

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// attractDemo is the screensaver's own tiny simulation — deliberately
// not the game's. It needs no lives, no score and no input, and reusing
// sim would mean every rule change had to be re-reasoned about here too.
type attractDemo struct {
	acc     uint64
	elapsed uint64
	x       int
	dir     int
	walk    walker
	drops   [3]fruit
	top     ui.AttractScore
	init    bool
	buf     [8 + cartlib.MaxNameLen]byte
}

var demo attractDemo

// Attract implements cartlib.Attractor. main.go's //export cart_attract
// shim is the other half — implementing this method alone does not opt
// in.
func (g *catcherGame) Attract(dtMicros uint64, fb *cartlib.Framebuffer) {
	demo.step(dtMicros)
	demo.draw(fb)
}

func (d *attractDemo) step(dtMicros uint64) {
	if !d.init {
		d.init = true
		d.x, d.dir = 8, 1
		// Spread the three drops down the screen so the first frame
		// already looks like a game rather than an empty sky.
		for i := range d.drops {
			d.drops[i] = fruit{live: true, x: 10 + i*18, y16: -i * 22 * 16}
		}
		d.top = topScore(d.buf[:])
	}

	d.elapsed += dtMicros
	d.walk.advance(dtMicros, true)

	d.acc += dtMicros
	for d.acc >= stepMicros {
		d.acc -= stepMicros
		d.x += d.dir * walkStep
		if d.x <= 0 {
			d.x, d.dir = 0, 1
		}
		if d.x >= catcherMaxX {
			d.x, d.dir = catcherMaxX, -1
		}
		for i := range d.drops {
			f := &d.drops[i]
			f.y16 += fallBase
			// No RandomU64 here: attract is a decoration, and drawing
			// from the engine's shared RNG stream from a screensaver
			// would move every other consumer's position in it.
			if f.y() >= fbH-fruitH {
				f.y16 = -8 * 16
				f.x = (f.x + 23) % (fbW - fruitW + 1)
			}
		}
	}
}

func (d *attractDemo) draw(fb *cartlib.Framebuffer) {
	// ClearTransparent, not Clear: the engine's starfield shows through
	// underneath, which is what makes a cartridge's attract slot look
	// like part of the cabinet's screensaver rather than a takeover.
	fb.ClearTransparent()

	led := ui.FontLED{}
	w := led.TextWidth("CATCHER", 1)
	led.DrawText(fb, (cartlib.FBWidth-w)/2, 4, "CATCHER", 1, colAccent)

	for i := range d.drops {
		f := d.drops[i]
		if f.live {
			spriteFruit.Draw(fb, f.x, f.y())
		}
	}
	d.walk.cel().Draw(fb, d.x, catcherY)

	if d.top.Score > 0 {
		ui.AttractPanel{
			X: 6, Y: 40, W: 52, H: 12,
			Top:    d.top,
			Trophy: true,
			Color:  colDim,
			T:      d.elapsed,
		}.Draw(fb)
	}
}
```

Attract runs in a **dedicated instance** that only ever sees
`cart_init`, `cart_attract` and `cart_teardown` — no load phase, no
`Update`, no `HandleInput`. None of the state a played game builds up
exists there. Audio and the peripherals, however, are entirely live,
which is why the convention is *structural* silence rather than
careful silence: attract plays zero sounds and touches no LCDs or
lights, and [part 4](tutorial-debug.md) adds a test that counts sound
calls across a long attract run and asserts zero. A title sting, if a
game wants one, is fired from a path attract never reaches.

Two smaller details: `fb.ClearTransparent` instead of `Clear` lets the
engine's starfield show through underneath, which is what makes the slot
look like part of the screensaver instead of a takeover. And the demo
draws no random numbers at all — the engine RNG is a single stream
shared with the shell and the attract loop, so a screensaver drawing
from it would move every other consumer's position in it.

## Where you are

A game that looks and sounds like one:

```sh
make build && cd ../../arkade && ./arkade --cartridges ../my-cartridges/catcher --app cart/catcher
```

The console renderer plays the sound through your computer's audio
output (`--mute` silences it); the browser player, which [part
4](tutorial-debug.md) sets up for iterating, plays it too.

**Next: [waves you draw by hand](tutorial-levels.md)** — design the
opening minute of a run in the level editor, generate it through a hook
that refuses the waves the cartridge cannot play, and hand off to the
endless game you already have.
