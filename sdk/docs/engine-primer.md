# How game engines work (and how Årkade does it)

A game engine is a small number of ideas repeated since the 1970s: a
loop that runs the world forward, a buffer of pixels, some pictures you
move around in it, a way to hear what a human just pressed, and a
decision about what happens when the frame takes longer than it should.

This page walks those ideas in order. Each one gets the general version
first — what every engine has to solve and why — and then **in Årkade**,
where it lives in this SDK, with the call you'd actually make. You do
not need any gamedev background to read it. When you're done you'll know
both how engines do this in general and exactly which file to open here.

If you'd rather start by building something, go to
[Getting started](getting-started.md) and come back — nothing below is a
prerequisite for the template. This is the tour, not the manual.

## 1. The game loop

**In general.** Every game is one loop:

```figure game-loop
You write the three boxes; the host runs the circle. Everything else in
this document hangs off that arrangement — what arrives in the first box,
what the second box is allowed to depend on, and what the third box is
allowed to touch.
```

```text
forever:
    read whatever the human did since last time
    advance the world by some amount of time
    draw the world
```

The interesting part is the middle line, because "some amount of time"
is a choice with consequences. Draw a frame, and the next one arrives
16 ms later — or 40 ms later, because something hitched.

Two schools:

- **Variable timestep** — the loop hands the simulation the real elapsed
  time and the simulation scales itself by it. Simple, and smooth when
  the frame rate is smooth. Its failure mode is that physics stops being
  reproducible: the same inputs on a slower machine produce a different
  game, and one enormous `dt` after a stall teleports things through
  walls.
- **Fixed timestep** — the simulation only ever advances in identical
  quanta. The loop accumulates real time and runs however many whole
  steps have piled up. Now the sim is a pure function of *step count*,
  which is what makes replays, recordings and frame-exact tests
  possible at all.

Almost every serious engine ends up at the second one, and the reason is
rarely physics. It's that a deterministic simulation can be **written
down and played back**.

The difference is invisible until frames get expensive, which is exactly
when it matters. Drag the lever:

```demo lag
One throw, simulated both ways — the lever starts at half travel, so you
are looking at the problem already. The **frame-based** throw advances a
fixed 16 ms of simulation per tick whatever the clock said, so it traces a
perfect parabola *in slow motion*: at the top of the lever it is still
climbing when the real throw has long since landed. The **delta-based**
throw scales by elapsed time and lands exactly where and when it should,
on the line, at any tick rate — what lag costs it is *resolution*, and you
can watch its per-tick dots thin out until the ball is jumping a
noticeable distance each step. That gap is the whole reason a large `dt`
gets clamped rather than trusted: a step big enough is a step straight
through a wall. Slide lag to zero and the two become the same ball. The
dashed line is the true arc; the hollow marker is real time.
```

**In Årkade.** The host owns the outer loop; you implement five methods
of [`cartlib.Game`](cartlib.md#the-game-interface):

```text
Manifest()                  who you are
Init(setup)                 once, before anything runs; setup loads and binds
HandleInput(records)        one batch of events per frame
Update(dtMicros)            advance the sim by this delta
Render(fb)                  draw one frame
```

The host calls `HandleInput → Update → Render` about 30 times a second
and blits your frame for you. The cabinet's engine paces itself to a
30 FPS budget (`internal/engine`'s `DefaultFPS`) and counts dropped
frames when it overruns; the browser player runs a fixed-step
accumulator at 33 333 µs per tick.

Two things follow, and they're the two things people get wrong first:

**`dtMicros` is the delta for this tick, not a running total.** On the
cabinet it is the *real* elapsed time since the previous tick began, so
it is not always 33 333. Don't assume a number — accumulate it:

```go
package snippet

// The classic fixed-timestep accumulator, living inside Update because
// the host owns the outer loop.
const stepMicros uint64 = 33_333 // this sim's own tick: 30 steps/second

type world struct {
	acc   uint64
	ticks uint64
}

func (w *world) Update(dtMicros uint64) {
	if dtMicros > 250_000 {
		dtMicros = 250_000 // a stall must not spiral into 100 catch-up steps
	}
	w.acc += dtMicros
	for w.acc >= stepMicros {
		w.acc -= stepMicros
		w.ticks++
		w.step()
	}
}

func (w *world) step() { /* one quantum of game logic */ }
```

**`Render` may run before the first `Update`.** The engine takes an
initial frame straight after `Init`, so `Render` has to draw something
valid from whatever `Init` left behind.

And the rule underneath both: **`dtMicros` is your only clock.** There
is no wall clock in a cartridge — not `time.Now`, not a frame counter
you sync to real seconds. `cartlib.NowMicros()` exists for the rare
absolute comparison and is engine time, not wall time. This is not
pedantry; [determinism](#6-randomness-and-determinism) is what it buys
you.

More: [cartlib reference](cartlib.md#the-game-interface),
[ABI reference](abi.md#cartridge-exports).

## 2. Frames and the display

**In general.** A framebuffer is a rectangle of memory where one entry
is one pixel. Draw into it, hand it to the display, repeat. Historically
that memory was scarce, so hardware cheated: **palettes** (store a
4-bit index per pixel and a lookup table of real colours, and you get 16
colours for a quarter of the RAM), **tilemaps** (store an 8×8 tile id
per cell instead of pixels), **double buffering** (draw into the buffer
nobody is scanning out). Compositing — layering a background, a
playfield, and a HUD into one final image — was often done by the video
chip itself, one scanline at a time.

Modern engines keep the *ideas* (layers, compositing, a clear each
frame) and drop the *tricks*, because memory is free now.

```figure panel-pixels
① is the panel, set into the tabletop. ② is what it is actually made of:
4,096 individual lamps in a 64×64 grid, each one either lit or not.
There is no palette and no tile layer between your framebuffer and those
lamps — one entry in, one lamp out.
```

**In Årkade.** The panel is a 64×64 RGB LED matrix — 4,096 pixels, no
palette, no tiles. `cartlib.Framebuffer` is exactly that: a fixed
`FBWidth × FBHeight` array of straight-alpha RGBA bytes, blitted whole
to the host once per frame. One crossing of the wasm boundary per frame
instead of 4,096 (`host_blit` — [design rule
4](abi.md#design-rules)).

The lever that makes the whole toolkit work: **a `*cartlib.Framebuffer`
*is* a `ui.Canvas`**. `Canvas` is two methods — `Bounds()` and
`Blend(x, y, src)` — so every widget in [`sdk/ui`](ui.md) composites
straight into your frame, and so does the cabinet's own shell. Your
menus are literally the cabinet's menus.

```go
package snippet

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

func render(fb *cartlib.Framebuffer) {
	fb.Clear(0, 0, 0)                                   // raw pixels…
	ui.FillRect(fb, 8, 8, 12, 12, ui.White)             // …and widgets…
	ui.Label{X: 1, Y: 1, Text: "SCORE", Color: ui.White}.Draw(fb) // …same buffer
}
```

Compositing is there when you want it: `ui.Layer` is an off-screen
Canvas, `ui.Compositor` flattens a fixed stack of layers in add order with
per-layer opacity, and `ui.Fade` animates one number from a `dt`. A layer you
allocate in `Init` and *never clear* is how you get trails and smears
([ui reference](ui.md#layers-and-compositing)).

Two panel-specific truths that no amount of general knowledge will tell
you:

- **Everything is drawn guest-side, into the framebuffer.** cartlib
  sends the whole frame after every `Render`, so the ABI's host-side
  draw imports would be overwritten; cartlib does not offer them.
- **64×64 is a window, not a canvas to shrink onto.** LED panels bloom;
  near-white fields glare across the table. Move a camera instead of
  scaling the world down, and review at 1× from across the room
  ([patterns](patterns.md#designing-for-6464)).

## 3. Sprites

**In general.** A sprite started as a hardware object: a small bitmap
the video chip drew *over* the background at coordinates you poked into
a register, without disturbing the framebuffer at all. The chip had a
fixed number of them, which is why so many old games have exactly eight
of something on screen.

Sprites brought two ideas that outlived the hardware:

- **Sheets.** Every frame of an animation packed into one image, cut by
  offset. One asset, one load, one cache line.
- **Masks.** A sprite is rectangular but its *body* is not. Collide with
  the rectangle and the player gets hit by empty air; collide per-pixel
  and it feels right.

**In Årkade.** `ui.Sprite` is pixel art in guest memory: `W`, `H`, a `Pix`
string of RGBA bytes, plus a `Mask` — a bit-per-pixel bitset of the
collidable body, one `uint64` per row.

The alpha rule is the whole design, and `ui.OpaqueMask` fixes it:

> **Only `A == 255` pixels enter the mask.**

So an anti-aliased edge draws softly *and* is automatically forgiving,
and a decoration that should look solid but hit nothing is authored at
`A = 254`. The artist shapes the look and the hitbox in one PNG.

```figure sprite-mask
The same 8x8 sprite, drawn and collided. The soft edge is dimmer, so it
falls out of the mask and the hitbox forgives near-misses. The exhaust is
authored at `A = 254` — indistinguishable on the panel, absent from the
mask, so it never hits anything. One PNG decides both.
```

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/ui"

// A sprite written out as a literal, in miniature: a 2×2 sprite whose
// bottom-left pixel is drawn at A=254 — visible, but deliberately not collidable.
var player = ui.Sprite{
	W: 2, H: 2,
	Pix:  "\xff\xff\xff\xff\x00\x00\x00\x00\xff\xff\xff\xfe\xff\xff\xff\xff",
	Mask: ui.Mask{W: 2, H: 2, Rows: []uint64{0x1, 0x2}},
}

func step(cv ui.Canvas, px, py int, rock *ui.Mask, rx, ry int) bool {
	player.Draw(cv, px, py)
	return player.Mask.Overlaps(px, py, rock, rx, ry) // pixel-perfect
}
```

You rarely write those literals. Art lives as **hand-editable PNGs**
under `assets/`, ships in the `.rcart`, and `Setup.LoadSheet` lifts each
cel into a `ui.Sprite` at Init. The PNG is what a human edits and what a
diff shows.

**Sheets** are the same idea, kept honest. A `.arkadesprite.png` whose
envelope names its cels becomes *one ordinary `ui.Sprite` per cel* when
`Sheet.Cel` lifts it at Init — each with its own mask, each
byte-for-byte what that drawing would have been as its own file. An
animation is a sequence over those cels, and it has no clock of its
own: what a tick is worth is a per-game answer, so the cartridge
advances it from its own accumulated `dtMicros`. (The 64px sprite limit
applies to a *cel*, not the file: an eight-cel strip of 16px frames is a
legal 128×16 PNG.)

**Levels** take the other road and are compiled in: hand-authored
`.arkadelevels` pack files → `tools/genlevelpack` → generated Go,
with the guest-side runtime in `sdk/levels`. Source of truth is the text
file; every cartridge build regenerates the Go, and you commit both halves.

More: [ui reference](ui.md#sprites-and-masks),
[patterns](patterns.md#sprite-art-as-pngs).

## 4. Input

**In general.** Two models. **Polling** asks "is the jump button down
right now?" once per frame — perfect for movement, useless for anything
that should happen exactly once. **Events** deliver "the jump button
went down" as a discrete record — perfect for actions, useless for "how
long have they been holding left". Real games need both, and most input
bugs are someone using the wrong one.

Local multiplayer adds a second question that single-player never
raises: not just *what* was pressed, but *by whom* — and, on a physical
cabinet, *where they are sitting*.

```figure seat-geometry
The cabinet is a table, and the two seats face each other across it.
**① is physical seat 0 — the panel's RIGHT edge, and the RED joystick.
② is seat 1: the LEFT edge, blue.** ③ is the panel itself. Nobody sits
at its top or bottom edges, which is why a game with per-player
territory lays that territory out along **x** — see
[Seating & rotation](seating.md).
```

**In Årkade.** Input is event-shaped. Each frame the host packs 8-byte
records and hands you the batch:

```text
offset  field     meaning
0       kind      0 keydown, 1 keyup, 2 keypress, 3 joystick
1       key       key ordinal (KeyJoyA, KeyQuit, …)
2       player    1 or 2 — the PAIRING slot
3       joy       0 center, 1..8 = N, NE, E, … clockwise
4       source    keyboard or controller
5       physical  the seat (cabinet port), or SeatNone
6       index     controller slot, or 0xFF
7       flags     bit 0 virtual, bit 1 shift
```

Four rules cover most of it:

1. **`KindKeypress` is the canonical "pressed"** (`r.IsPress()`). A
   cabinet controller emits a keydown *and* a keypress per physical
   press. Act on both and everything double-fires.
2. **Take direction edges from `r.PressDir()`.** A real stick sends both
   a joystick record and a synthetic direction keypress per push;
   `PressDir` counts the push once.
3. **Held state goes through `ui.HeldDirs`** — `held.Feed(r)` for every
   record. Terminal keyboards can't observe releases and never send
   keyup at all; `HeldDirs` fuses real keydown/keyup, joystick position
   levels, and a 250 ms credit per virtual keypress, which hand-rolled
   hold tracking always gets wrong for one of the three.
4. **The quit keys exit unless you take them.** Declaring the
   `host_exit` import — which cartlib always does — transfers quit-key
   ownership to your cartridge, so cartlib answers `KeyQuit`/`KeyEscape`/
   `KeyJoyB` presses with `cartlib.Exit()` for you. A game with its own
   quit flow implements `cartlib.QuitOwner` and calls `Exit` itself.

And the distinction with no analogue on a desktop: **`Player` is not
`Physical`.** `Player` is the pairing slot — who is P1 — and decides
whose paddle moves. `Physical` is the cabinet port — which little LCD
and which six lit buttons are in front of that specific human. They
diverge the moment somebody picks up the other controller.

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

type match struct{ scores [3]uint32 }

// Two different bytes, two different questions.
func (m *match) award(r cartlib.InputRecord, points uint32) {
	if r.Player == cartlib.SlotP1 || r.Player == cartlib.SlotP2 {
		m.scores[r.Player] += points // WHO is playing
	}
	cartlib.SetText(r.Physical, "SCORE", "") // WHERE they sit (SeatNone: no-op)
}
```

**Seats and rotation, in one paragraph.** The cabinet is a *table*: the
panel lies flat and a player sits on each of two opposite sides.
Physical seat 0 is the panel's right edge (the red stick), seat 1 the
left (blue) — so a board with per-player territory lays that territory
out along **x**, because the top and bottom edges face nobody. The
engine can additionally gate input to one seat and physically rotate the
panel toward them (`PushPlayerStrategy` + `LockPlayerToSeat`), which is
right for a shared field facing whoever started and wrong for a
fixed-sides board. Both hosts do it, but neither gates a keyboard, so
a 2P game still demuxes on `Player` regardless. This is the easiest
thing in the SDK to get subtly wrong, so it has its own page:
[Seating & rotation](seating.md#which-shape-is-your-game).

More: [cartlib reference](cartlib.md#input),
[ABI reference](abi.md#input-records).

## 5. State and scenes

**In general.** A game is never in one state. It's on a title screen, or
counting down, or playing, or dead, or typing initials — and each of
those wants different input handling, different drawing, and a different
answer to "what does B do?". The universal idiom is a **mode machine**:
one enum, one switch at the top of each of input/update/draw, and every
mode's code in its own function. Bigger engines dress this up as a
*scene stack* so that a pause overlay can sit on top of a live game
without the game knowing.

The failure mode this prevents is the pile of booleans — `paused`,
`dying`, `showingMenu` — that can be true in combinations nobody
designed.

**In Årkade.** The switch itself is the good part and stays exactly as
you would write it by hand:

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

const (
	modeTitle uint8 = iota // the zero value: where Init leaves you
	modePlaying
	modeGameOver
)

type game struct{ mode uint8 }

// Exactly one mode is active, so a record can never be read twice.
func (g *game) HandleInput(records []cartlib.InputRecord) {
	for _, r := range records {
		switch g.mode {
		case modePlaying:
			g.playingInput(r)
		case modeGameOver:
			g.gameOverInput(r)
		default:
			g.titleInput(r)
		}
	}
}

func (g *game) titleInput(cartlib.InputRecord)    {}
func (g *game) playingInput(cartlib.InputRecord)  {}
func (g *game) gameOverInput(cartlib.InputRecord) {}
```

```figure scene-states
Three modes, one active at a time — which is what makes the switch above
safe. The dots are the part worth stealing: every arrival runs that
screen's entry hook, so no screen can be reached without asserting its own
LCD and lamps. A modal suspends the screen underneath and hands it back;
it never becomes a fourth mode, and that is exactly the pile of booleans
this shape exists to prevent.
```

What the SDK adds is not a replacement for that switch but the part that
hangs off *entering* a screen: **[`sdk/fsm`](ui.md#screen-state-sdkfsm)**
holds the variable, runs one entry hook on every arrival (so a screen
cannot be reached without asserting its LCD and lamps), and gives modals
a real `Push`/`Pop` stack instead of a hand-saved return address. Read
that section for the API and, more importantly, for when *not* to reach
for it — under about six screens plus a modal, the plain enum above
remains a perfectly good answer.

`cartridges/life` is the clean reference — four screens, one switch per
entry point, and a nice judgement call worth stealing: its end-state
fade is a *sub-state of playing*, not a fifth screen, because the player
can still open the settings overlay over a fading world. A modifier of a
running screen is a bool; a screen is a screen, and `sdk/fsm` does not
change that call.

## 6. Randomness and determinism

**In general.** Games want randomness — a shuffled deck, a spawn point,
a dungeon — but they want it *reproducible*. Seed a pseudo-random
generator and the same seed replays the same dungeon, which is how
roguelikes share worlds, how speedruns are verified, and how a bug that
happens once can be made to happen again.

Determinism is the stronger version of that promise: if a frame is a
pure function of its inputs, then *writing the inputs down is writing
the game down*. That single property is what makes replays, frame-exact
tests, and server-verified scores possible. It is also brittle —
one `time.Now()`, one `math/rand`, one goroutine racing another, and
it's gone.

```figure determinism
Why the invariant is worth the rules it costs. ① somebody plays, and the
cabinet records nothing but what they pressed and when. ② that is the
whole recording — a strip of input records, no frames in it. ③ the same
cartridge, fed the same records, produces the same game with nobody in
the chair. A replay, a frame-exact test and a verified score are all the
same trick.
```

**In Årkade.** The invariant is stated once and defended everywhere:

> Everything observable in a frame is a pure function of
> **(`dtMicros`, the input records, the `RandomU64` draws)**.

Which is why the hard rules read the way they do: no wall clock, no
`math/rand` or `crypto/rand`, no goroutines, and no libm transcendentals
(`math.Sin`, `Cos`, `Pow`) on observable paths — plain `float64`
arithmetic is IEEE-754-identical across toolchains, but libm
implementations are not. See
[Gotchas](gotchas.md#determinism).

Your only entropy source:

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

// The engine-seeded RNG, and nothing else. Bounded with a modulo.
func spawnColumn(width int) int { return int(cartlib.RandomU64() % uint64(width)) }
```

What it buys, in increasing order of payoff:

- **Golden tests.** A test next to your cartridge drives the checked-in
  `.wasm` through the `carttest` harness with a scripted RNG stream,
  scripted input and fixed timing, asserts ASCII snapshots of the
  frames, and checks that two runs are byte-identical. The minimum bar
  is determinism plus one correctness anchor
  ([patterns](patterns.md#testing-strategy),
  [Getting started](getting-started.md#5-test-as-you-go)).
- **Cross-host agreement.** The *same* `.wasm` under the *same* script on
  both hosts, asserted byte-for-byte — the browser host checks itself
  against the same golden files the Go tests write. A divergence is a
  host bug, and the fix is never to fork the goldens.
- **The flight recorder.** The cabinet is always recording the cartridge
  it is running: a bounded ring of 16-byte events — each tick's
  `dt_micros`, each input record, each `RandomU64` result — under a JSON
  header carrying the slug, the manifest version, the **SHA-256 of the
  exact `.wasm` bytes**, the seed and the build. When something goes
  wrong, that ring becomes a file and one command re-lives the session
  at your desk:

```sh
arkade --replay data/flight/<recording>.arkflt
```

Two details in there are quietly instructive. Input is recorded **where
the guest receives it** — after the seat-mount remap, after standby
gating, after the strategy gate — because a controller's RIGHT is not
the cartridge's RIGHT, and a recording made any earlier would replay a
different game. And the recorder writes the RNG *values*, not just the
seed, because the engine's RNG is one stream shared with the shell and
the screensaver: a cartridge's position in it depends on everything that
ran before it.

More: [ABI reference](abi.md#determinism).

## 7. Audio

**In general.** Two ways to make a sound. **Play a sample** — record or
render a waveform ahead of time and stream the bytes. **Synthesize** —
generate the waveform at runtime from oscillators and envelopes. Early
consoles had no memory for samples, so their sound chips gave you a
handful of voices (two pulse waves, a triangle, a noise channel, say)
and you *wrote music as note data*. The chiptune sound isn't nostalgia,
it's a bill of materials: those specific timbres are what that specific
silicon could do.

Every era's game audio is shaped by its speaker as much as its chip.

**In Årkade.** The runtime side is small. WAVs travel inside the
`.rcart` bundle; you load them by bundle-relative path in `Init` and
play them by handle:

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

type audio struct{ pop, music uint32 }

func (a *audio) init(s *cartlib.Setup) { // from Game.Init — loads resolve during the splash
	a.pop = s.LoadSound("assets/pop.wav")
	a.music = s.LoadSound("assets/theme.wav")
}

func (a *audio) start() {
	cartlib.PlaySound(a.pop, 0)                          // once
	cartlib.PlaySound(a.music, cartlib.PlaySoundForever) // loop verbatim
}
```

The interesting part is where those WAVs come from, because the
platform rule is **synthesize, don't ship**: a cartridge generates its own sound
effects at build time with a small `tools/gen-sfx` program over
`tools/sfxlib` — oscillators, ADSR envelopes, noise, mixing,
peak normalization to ~0.9 full scale — and commits the result. That
generator is host-only offline code, so unlike cartridge runtime code it
is perfectly free to call `math.Sin`.

Why bother? Because of the speaker floor. **The cabinet's small speakers
reproduce nothing below about 150 Hz.** A sound whose energy sits down
there plays "correctly" through an entire working audio chain and is
simply inaudible on the actual hardware — a chest-beat drum built with
most of its energy at 40–120 Hz, say, would pass every test on a desk
and vanish on the cabinet. The fix is not a louder mix; it's making the
*generator* assert its own spectrum: `sfxlib` carries Goertzel
band-energy checks so that bug class fails the build instead of
shipping silently broken.

The practical shape of that discipline:

- Put fundamentals in the **250–1500 Hz** band.
- Fake rumble by amplitude-modulating a mid-band carrier at 30–60 Hz —
  perceptual bass that a small driver can actually move.
- Add a ~90 ms retrigger cooldown anywhere physics can machine-gun a
  collision sound.
- Music gets the same treatment one level up: `tools/musiclib`
  renders whole songs from note data — pulse/triangle/noise voices,
  tempo, a loop length in beats — and guarantees the buffer's end meets
  its own beginning click-free, because the engine loops a music WAV
  verbatim.
- Attract mode plays **zero** sounds. It runs a dedicated instance with
  audio fully live, so silence has to be structural: fire your title
  sting from `LoadFrame`, a path attract never runs.

More: [cartlib reference](cartlib.md#sound),
[patterns](patterns.md#sound-design),
[patterns](patterns.md#attract-mode).

## 8. Where to go next

You now have the map. The territory:

- **[Getting started](getting-started.md)** — copy the template, build
  it with TinyGo, play it in the browser and in your terminal, pack a
  `.rcart`. About twenty minutes.
- **[cartlib reference](cartlib.md)** — the whole author-facing surface:
  drawing, input, sound, storage, the shared score table, the cabinet's
  LCDs and button lights.
- **[ui reference](ui.md)** — canvas, fonts, widgets, menus, name entry,
  hold-to-confirm, scenes, layers.
- **[Authoring patterns](patterns.md)** — every rule was a shipped bug
  first. Read it before you polish, not after.
- **[Seating & rotation](seating.md)** — which of three shapes your game
  is, and what that decides about your layout.
- **[Manifests & packaging](packaging.md)** — the bundle format, the
  caps, and how a `.rcart` gets onto a cabinet.
- **[ABI reference](abi.md)** — the wasm contract itself. Skippable
  until you want it; unavoidable if you write a host or target the ABI
  from Rust or C.

And the tooling, which is more fun than reading:

- **The asset lab** at [arkade.build/dev/tools](https://arkade.build/dev/tools) — spritelab (a pixel
  editor that previews art with the LED panel's own look), soundlab (WAV
  audition, an SFX designer, a chiptune loop sequencer), glyphlab (the
  16×2 LCD, dot by dot), hidlab (gamepad debugging), periphlab (live
  inputs and button-light choreography), levellab (the level-pack grid
  editor) and fsmlab (the screen-state-machine editor). Hosted on the
  site; import a cartridge's files, edit them, and export them back
  into its directory.
- **The terminal loop** — `make rcart DIR=../my-cartridges/<slug>`
  builds and packs a cartridge, and `./arkade --cartridges
  ../my-cartridges` runs it in a desktop window; the browser player at
  [arkade.build](https://arkade.build) plays the same `.rcart`.
- **The reference cartridges** — in the
  [arkade-cartridges](https://github.com/stealthmonkey99/arkade-cartridges)
  repo, `cartridges/life` is the simplest complete game and
  `cartridges/snake` has menus, two players, high scores, sound and an
  attract mode.

And when you're ready to build one game start to finish — art and sound
and goldens included — [the Catcher tutorial](tutorial.md) is exactly
that, and this page is its prerequisite reading.
