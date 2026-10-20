# cartlib reference

`cartlib` (`github.com/stealthmonkey99/arkade/sdk/cartlib`, at
`sdk/cartlib`) is the cartridge SDK library: an ergonomic Go wrapper
around the raw wasm [ABI](abi.md). You implement one interface;
`cartlib` supplies every `cart_*` export and wraps every `host_*`
import. This page is the author-facing tour of that surface: how the pieces
fit and where they trip you. The symbol-by-symbol reference is generated
from the doc comments into [api/cartlib.md](api/cartlib.md) (and
[api/cartlib-hosttest.md](api/cartlib-hosttest.md)); `go doc` and
pkg.go.dev show the same comments.

Everything here compiles under **both** TinyGo (the real cartridge
build) and the ordinary host Go toolchain (your unit tests): the
host-import plumbing is split behind `tinygo`/`!tinygo` build tags, so
`go test ./...` in your cartridge module just works.

## The Game interface

You implement [Game](api/cartlib.md#type-game): `Manifest`, `Init`,
`HandleInput`, `Update` and `Render`.

Call order across a lifetime: `Manifest` (any time), `Init` once, then
`HandleInput → Update → Render` per frame, then `Teardown` (if
implemented) at unload. The canvas is always `cartlib.FBWidth` by
`cartlib.FBHeight`. Three contract points people trip on:

- **`Init`'s [`*cartlib.Setup`](api/cartlib.md#type-setup) is the only way to load a sound or sheet,
  bind a channel or define a glyph**, and it is dead the moment `Init`
  returns: a call through it later returns the failure value (0, a nil
  cel, an unbound channel, `false`), panics under `go test`, and logs at
  `LogError` on the cabinet. Keep what you loaded, not the Setup.

- **`Render` may run before the first `Update`** (the engine's init
  render). Draw a valid frame from whatever `Init` set up.
- **`Update`'s `dtMicros` is the per-tick delta**, not a running
  total. Accumulate it for cadences:

```go
package snippet

type paced struct {
	acc   uint64
	steps int
}

const stepMicros = 100_000 // act 10×/second

func (p *paced) Update(dtMicros uint64) {
	p.acc += dtMicros
	for p.acc >= stepMicros {
		p.acc -= stepMicros
		p.steps++ // one fixed "tick" of game logic
	}
}
```

### Optional interfaces

Implement any of these on your game type and cartlib finds it:
[Teardowner](api/cartlib.md#type-teardowner) (flush something before
unload), [Loader](api/cartlib.md#type-loader) (a custom loading screen;
without it you get a free splash, and either way the load phase lasts at
least `cartlib.MinSplashMicros`, which also gives the browser host time to
resolve async sound loads issued in `Init`),
[QuitOwner](api/cartlib.md#type-quitowner) (your own quit flow; see
[Input](#input)) and [Attractor](api/cartlib.md#type-attractor), a slot in
the cabinet's idle screensaver. Opting in to attract takes the
implementation **plus** a one-line export shim (below). Use
`fb.ClearTransparent()` to overlay the engine's starfield. House rule:
attract plays **zero sounds** and touches no LCDs/lights; see
[Authoring patterns](patterns.md#attract-mode).

The attract export shim, verbatim:

```go
package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

//export cart_attract
func cartAttract(dt uint64) { cartlib.DispatchAttract(dt) }

func main() {}
```

## Registration and the reactor model

Register exactly one instance, at the package level:

```go
package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

type myGame struct{}

func (g *myGame) Manifest() []byte                          { return manifestBytes }
func (g *myGame) Init(s *cartlib.Setup)                     {}
func (g *myGame) HandleInput(records []cartlib.InputRecord) {}
func (g *myGame) Update(dtMicros uint64)                    {}
func (g *myGame) Render(fb *cartlib.Framebuffer)            { fb.Clear(0, 0, 0) }

var game = &myGame{}
var _ = cartlib.Run(game)

func main() {} // runs once during _start; keep it empty
```

Why this exact shape: a cartridge is hosted as a wasm *reactor*. The
host runs TinyGo's `_start` exactly once at instantiation, which runs
package initializers and `main` and boots the allocator (the "runtime
boot", see the [ABI reference](abi.md#runtime-boot)); after that it only
calls the exported `cart_*` functions. Setup belongs in `Game.Init`, which
is the first export the host calls, so:

- **No `func init()`** and **no logic in `main`**: `main` must exist and
  stay empty. Preflight refuses both.
- **No computed package-level initializers**: literals and simple
  constructor-shaped calls only, which TinyGo's compile-time interpreter
  folds into static data. `var _ = cartlib.Run(game)` is one of those.

## The Manifest method

`Manifest` returns `manifestBytes`, the package-level slice declared in
`manifest_gen.go` beside your sources, and nothing else:

```go fragment
func (*g) Manifest() []byte { return manifestBytes }
```

`manifest_gen.go` is written from `manifest.json` by the SDK's
`manifestgen` on every build (`make build` runs it before `tinygo`) and
declares the `manifestJSON` constant that slice is built from. Never
concatenate a manifest at call time or derive it from runtime state:
the build's mandatory `wasmfix` post-pass can only patch a
`cart_manifest` body whose two values resolve to compile-time
constants, and it fails the build loudly on anything else. The runtime
manifest is a small JSON object (`abi`, `slug`, `name`, `description`,
`version`, `players` as integers, optional `icon` and `rawDirections`);
the richer bundle `manifest.json` is the file you edit — [Manifests &
packaging](packaging.md) covers how one becomes the other.

## The TinyGo rules

A cartridge runs under TinyGo `-scheduler=none -gc=leaking`, where a per-frame allocation, a goroutine, a wall clock or `math/rand`
compile cleanly and then fail at runtime or break replay. The list,
one line per trap with the symbol or check that enforces it, is
[Gotchas](gotchas.md); `docs/GAME_AUTHORING_AI.md`'s golden rules carry
the full reasoning.

## Drawing

`Render` hands you a `*cartlib.Framebuffer` — a 64×64 RGBA buffer
(`cartlib.FBWidth` × `cartlib.FBHeight`) blitted to the host for you
after `Render` returns. Raw helpers:

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

func draw(fb *cartlib.Framebuffer) {
	fb.Clear(0, 0, 0)               // opaque fill — start of a normal frame
	fb.SetPixel(10, 12, 255, 0, 0)  // one opaque pixel, silently clipped off-frame
	fb.FillRect(20, 20, 8, 8, 0, 255, 0)
	// fb.ClearTransparent()        // attract mode only: show the starfield through
}
```

A [`*cartlib.Framebuffer`](api/cartlib.md#type-framebuffer) **is** a `ui.Canvas`, so the entire shared
widget toolkit composites straight into it — labels, menus, borders,
fonts, sprites. That is the recommended way to draw anything beyond
raw pixels; see the [ui reference](ui.md). `cartlib.Color` is an alias
of `ui.Color`, so simple cartridges don't need the second import.

cartlib also carries its own tiny 3×5 pixel font for the smallest
games ([DrawText](api/cartlib.md#func-drawtext) and friends): `A`–`Z`,
`0`–`9`, `-` and `?` only; anything else, lowercase included, draws as a blank
cell. `ui.FontLED` (the cabinet's LED font) and
`ui.Font3x5` are the fuller-featured versions.

Everything reaches the screen through that framebuffer. The ABI's
host-side draw imports (`host_draw_pixel`, `host_fill_rect`,
`host_draw_sprite`, `host_draw_cel`) are not wrapped by cartlib: it sends
the whole framebuffer after every `Render`, which would overwrite them.
Sprite art is a guest-side `ui.Sprite`, compiled in or lifted from the
bundle ([patterns](patterns.md#sprite-art-as-pngs)).

## Input

`HandleInput` receives the frame's decoded 8-byte records
([InputRecord](api/cartlib.md#type-inputrecord), re-exported from
`sdk/ui/input`). Two fields are easy to confuse: `Player` is the pairing
slot (who is P1 versus P2) and `Physical` is the seat. The record's
methods are the standard decodes (`IsPress` for the one-shot press,
`Dir` for the level decode of a direction, `PressDir` for the edge decode
counted once per push, `IsQuit`, `Virtual`), and the canonical handling
pattern is built from them:

```go
package snippet

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

type player struct {
	dirX, dirY int
	held       ui.HeldDirs // the level signal, for hold-to-repeat
}

func (p *player) handleInput(records []cartlib.InputRecord) {
	for _, r := range records {
		p.held.Feed(r) // every record, before any filter

		// PressDir counts one push once: a real stick's duplicate
		// keypress is dropped, a keyboard arrow is kept.
		switch r.PressDir() {
		case ui.DirUp:
			p.dirX, p.dirY = 0, -1
		case ui.DirDown:
			p.dirX, p.dirY = 0, 1
		}
		if !r.IsPress() {
			continue // keypress is the canonical "pressed"; keydown too would double-fire
		}
		if r.Key == cartlib.KeyJoyD {
			cartlib.RejectKey() // nothing to interact with yet
		}
	}
}
```

Rules of the road:

- **React to `KindKeypress`** for "button pressed" — the cabinet sends
  a keydown+keypress pair per physical press, so acting on keydown too
  double-fires. Use keydown/keyup only for hold-state tracking (and
  then use `ui.HeldDirs`, which also handles terminal keyboards that
  can't report releases).
- **`Player` ≠ `Physical`.** `Player` is the pairing slot — use it to
  decide whose paddle moves. `Physical` is the cabinet port — use it
  to address that human's LCD and button lights. They diverge the
  moment someone picks up the other controller.
- **Quit is handled unless you take it.** cartlib declares the
  `host_exit` import, which on the cabinet hands your cartridge its
  quit keys, so cartlib answers a `KeyQuit`/`KeyEscape`/`KeyJoyB` press
  with `cartlib.Exit()` before `HandleInput` sees the batch. A game with
  its own quit flow (`ui.HoldConfirm` is the standard widget)
  implements `cartlib.QuitOwner` — `OwnsQuitKeys() bool`, consulted once
  per batch — and calls `Exit()` itself.
- The stick is 8-way digital at the ABI (`Joy` ordinals); no analog
  magnitude ever reaches a cartridge.
- `cartlib.RejectKey()` rings the cabinet's "that does nothing" bell —
  polite feedback for a keypress with no meaning in the current state.

### Channels: reading input as levels

A record says what HAPPENED. When you want what IS — this button is held,
this spinner has turned nine counts since your last `Update` — bind a
**channel** in `Init` and read it in `Update`:

```go
package snippet2

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

var fire cartlib.Channel

func bindChannels(s *cartlib.Setup) { // from Game.Init: s is the only binder
	fire = s.BindButton(cartlib.P1 + cartlib.ChanA)
}
```

The cabinet's own panel is offered as channels (`p1.joy.x`, `p1.a`,
`p2.start`, ...) on every host, so this needs no hardware and changes
nothing about the records you also receive. Beyond the panel it is how a
cartridge reaches a spinner, a trackball or a pressure mat at all — from a
device profile the cartridge may ship itself. A bind that is not offered
returns the zero `Channel`, and the game is expected to play differently
rather than insist. The whole of it, including the profile format and what
a browser tab cannot do, is in
[the HID profile and input channel reference](reference-hid.md).

## Sound

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

type audio struct{ pop, music uint32 }

func (a *audio) init(s *cartlib.Setup) { // call from Game.Init with its Setup
	a.pop = s.LoadSound("assets/pop.wav")     // bundle-relative path
	a.music = s.LoadSound("assets/theme.wav")
}

func (a *audio) events() {
	cartlib.PlaySound(a.pop, 0)                          // play once
	cartlib.PlaySound(a.music, cartlib.PlaySoundForever) // loop until stopped
	cartlib.StopSound(a.music)
	cartlib.StopAllSounds()
}
```

Paths are bundle-relative, exactly as packed in the `.rcart`
(`"assets/pop.wav"`, no leading `./`). Load in `Init`; on the browser
host loads resolve asynchronously during the splash phase, and playing
a not-yet-resolved (or missing) sound is a safe no-op. A bare `.wasm`
with no bundle plays nothing and warns once per missing sound — dev
builds stay quiet, never crash. Concurrent sounds mix; keep peaks
around 0.9 full scale, and synthesize for small mid-range speakers
([patterns](patterns.md#sound-design)).

## Storage

Private per-cartridge persistence, namespaced by slug host-side
([StorageRead](api/cartlib.md#func-storageread),
[StorageWrite](api/cartlib.md#func-storagewrite)):

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

const bestKey = "best" // keys must match [A-Za-z0-9_.-]+ — no slashes!

func loadBest() uint32 {
	var buf [4]byte
	if cartlib.StorageRead(bestKey, buf[:]) == 4 {
		return uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
	}
	return 0
}

func saveBest(v uint32) {
	buf := [4]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}
	cartlib.StorageWrite(bestKey, buf[:])
}
```

Encode values as fixed-size binary (no `encoding/json` in a
cartridge). Keys are 1 to `cartlib.MaxStorageKeyLen` (256) bytes and
**must match `[A-Za-z0-9_.-]+`** (`cartlib.StorageKeyOK`); values cap at
`cartlib.MaxStorageValueLen` (64 KiB). `StorageWrite` returns false and
writes nothing — not a truncated value — when either rule is broken.
Don't store anything score-shaped here — that's what the score table
is for.

## Scores and recent names

The cabinet keeps a **shared top-10 table per game**, read from
outside the game by the shell's LCD readout, the attract trophy, and
the leaderboard ticker. A high score kept in private storage is
invisible to all of that — always use the score API
([ScoreSubmit](api/cartlib.md#func-scoresubmit) and its siblings):

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

func gameOver(score int, seat cartlib.Seat, entered func(prefill []byte) string) {
	if !cartlib.ScoreQualifies(score) {
		return // don't even show name entry
	}
	var buf [cartlib.MaxNameLen]byte
	prefill, _ := cartlib.RecentName(seat, buf[:]) // a view into buf: this seat's last initials
	name := entered(prefill)                       // your ui.NameEntry flow
	rank := cartlib.ScoreSubmit(score, name)       // 1-based rank, 0 = didn't place
	_ = rank
	cartlib.NoteRecentName(seat, name) // so their NEXT game offers it back
}

func readBoard() {
	var buf [8 + cartlib.MaxNameLen]byte
	for r := 1; ; r++ {
		score, name, ok := cartlib.ScoreRead(r, buf[:]) // name is a view into buf
		if !ok {
			break
		}
		_, _ = score, name
	}
}
```

`ScoreRead`, `RecentName` and `RecentNameAt` return the name as a view
into the buffer you passed, so reading allocates nothing; copy it to
keep it past the next call.

The qualification policy lives host-side, once, for every game: top
ten, a tie with 10th place does **not** qualify, equal scores rank
after existing equals. `ScoreRank(score)` gives the would-be rank for
a "you placed 3rd!" banner before name entry. Submit only completed
runs. `RecentName`/`RecentNameAt`/`NoteRecentName` are keyed by **physical
seat** (the input record's `Physical` byte) and remembered cabinet-wide.
Two memories, two questions: `RecentName` is the auto-suggest prefill and
lasts an hour, so a player still at the machine accepts their initials
with one press; `RecentNameAt` walks that seat's five-deep MRU of
regulars, which never expires and survives a reboot — that's the list
`RecentNames` puts under the widget's charset. One `NoteRecentName` on
confirm feeds both.
There is no slug parameter anywhere: a cartridge structurally cannot
read another game's table.

## Player strategy, rotation, and raw directions

The host can gate input to one controller and rotate the LED panel
toward a seated player; the browser host ports the engine's strategies
and does both the same way. They are **affordances, not guarantees** —
neither host gates a keyboard record — so never rely on them for
correctness: a 2P game still demuxes on the record's `Player` byte.

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

func titleScreen() { cartlib.PushPlayerStrategy(cartlib.StrategyOnePlayerSolo) }

func onePlayerChosen(r cartlib.InputRecord) {
	cartlib.LockPlayerToSeat(r.Physical) // rotate + gate to a KNOWN seat
}

func backToTitle() { cartlib.UnlockPlayerStrategy() }
```

- A **push alone is inert** — input stays permissive until locked.
  Push on the title screen so any controller can start; lock when play
  begins.
- Prefer **`LockPlayerToSeat(seat)`** over `LockPlayerStrategy()`
  when you know the seat (the player who pressed "1 PLAYER" — use
  that keypress's `Physical` byte). Lock the moment you know, so even
  the "get ready" screen faces them. Never lock to a seat you haven't
  observed input from.
- Pick the [StrategyKind](api/cartlib.md#type-strategykind) that matches
  the game: solo rotates, against-AI gates without rotating, turn-based
  pairs with [SwitchPlayer](api/cartlib.md#func-switchplayer). The turn-based patterns have sharp edges — read
  [patterns](patterns.md#seats-strategy-and-rotation) before building
  any 2P turn-based game.
- `UnlockPlayerStrategy()` on every path back to a menu;
  `PopPlayerStrategy()` for scene-level pushes (the engine cleans up
  leftovers at unload).

**`SetRawDirections(enabled)`** opts gameplay out of the cabinet's
seat-mount direction remap at runtime. The remap is right for
*screen-spatial* controls (a cursor moving toward the top of the
board) and wrong for *egocentric* ones (tank rotate/thrust — up is
always "forward"). Keep the manifest default (screen-spatial) so menus
feel standard, call `SetRawDirections(true)` when gameplay starts and
`SetRawDirections(false)` on **every** path back to the title. The
override resets automatically when the cartridge unloads. The browser
host remaps for rotation, and honors the override, the same way.

## Peripherals: LCDs and button lights

Each cabinet seat has a 16×2 character LCD and six lit arcade buttons
([SetText](api/cartlib.md#func-settext),
[SetButtonLights](api/cartlib.md#func-setbuttonlights)).
Address them by a [Seat](api/cartlib.md#type-seat) — the record's **`Physical`** seat,
`cartlib.SeatP1`/`SeatP2`, or `cartlib.SeatBoth` for both sides.
`cartlib.SeatNone` (what a keyboard record carries) does nothing, so a
record's `Physical` can be passed straight through:

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

func hud(seat cartlib.Seat, score uint32) {
	cartlib.SetText(seat, "SCORE", "42 ♥♥♥") // ♥ — the LCD is not ASCII-only
	_ = score

	// SetText does not copy the strings it is given, so a constant like
	// the one above is free. What costs is BUILDING a string to pass —
	// a Sprintf, or a string(buf[:n]) around a formatted number. For a
	// readout you format, keep the buffer at package scope and use
	// SetTextBytes, which takes the bytes directly:
	//
	//	var scoreBuf [16]byte
	//	n := appendUint(scoreBuf[:], 0, score)
	//	cartlib.SetTextBytes(seat, scoreLabel, scoreBuf[:n]) // both package-level
	//
	// And throttle it. A 16x2 character LCD cannot legibly redraw at frame
	// rate; keep updates to around 4 Hz or slower, and a readout driven
	// by the SIM rather than by an event will blow past that without
	// anyone noticing. cartridges/life's setText/tickLcd is the worked
	// example, down to pushing immediately on every transition out of
	// the running sim so the readout is never left stale.

	// Light exactly the buttons that would DO something right now:
	cartlib.SetButtonLights(seat, cartlib.MaskJoyA|cartlib.MaskJoyB, true)
	// only=true: unmentioned buttons turn OFF (a full six-button table).
	// only=false: set bits turn ON, everything else keeps its state.

	cartlib.SetLights(cartlib.SeatBoth, false) // whole-set convenience switch
}
```

- LCD lines are UTF-8, capped at 64 bytes on the wire; the display
  clips at 16 *runes*. The driver pages custom 5×8 glyphs (♥ ♠ ♦ ♣
  arrows ░▒▓█ …) from the engine's palette — up to 8 distinct custom
  glyphs per screen at once; unknown non-ASCII renders as `?`. Your
  cartridge can add its own — see [below](#your-own-glyphs). Both hosts
  draw the same bitmap.
- Seats and slots are distinct types, so a seat cannot be passed where a
  slot is wanted. To turn a zero-based index into one, use
  `cartlib.SeatOf(i)` / `cartlib.SlotOf(i)` (0 is player one; anything
  else, `-1` included, is `SeatNone` / `SlotNone`, which peripheral calls
  ignore) rather than a `Seat(i)` conversion — slots are one-based on the
  wire, so `Slot(0)` is `SlotNone`. `seat.Index()` and `slot.Index()` go
  back, returning `-1` when there is no single seat or slot, for indexing
  a `[2]T` array.
- UX rule: **light a button exactly when pressing it would do
  something** — an always-lit button reads as a mystery
  ([patterns](patterns.md#buttons-and-lights)).
- The LCD is the HUD; the panel is the playfield. Blank the idle
  seat's LCD in 1P, and re-assert LCD/light state when resuming from
  any pause/confirm overlay — nothing is restored for you.

### Your own glyphs

The panel's chip draws ASCII from a mask ROM and nothing else; anything
pictorial has to be uploaded to it as a 5×8 bitmap first. The engine
ships a palette of those, and `DefineLCDGlyph` lets your cartridge bring
runes that palette has never heard of — no engine change, no cabinet
rebuild.

```go
package snippet

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

// Read the hex down the left and the picture is there: an arch for the
// handle, a full rim, a woven body, a narrower base. Bit 4 is the
// LEFTMOST of the five pixels, so 0x11 is "both edges lit, middle dark".
var basket = cartlib.GlyphRows{
	0x00, 0x0e, 0x11, 0x1f, 0x15, 0x1f, 0x0e, 0x00,
}

func Init(s *cartlib.Setup) {
	s.DefineLCDGlyph('֍', basket) // now printable in SetText
}
```

```lcd
glyph ֍ 00 0e 11 1f 15 1f 0e 00
CATCHER ֍
GREEN TO PLAY
```

- **Call it from `Init`.** The definitions die with the cartridge —
  nothing you define is visible to the shell, the attract loop or the
  next game.
- **Pick a plain, single-width, monochrome character** — the kind a
  terminal draws white-on-black in one cell — never an emoji: terminals
  draw those in colour and two cells wide, and the hosts refuse them.
  `֍` (U+058D) for a basket, not `🧺`.
- **Up to `cartlib.MaxCartGlyphs` (32) runes**, and a definition
  shadows the engine's own drawing of the same rune, for your game only.
- **Returns `false` when refused**: a code point below U+0080 (the chip
  already draws ASCII, and redefining a letter would spend a scarce slot
  on every line containing it); an emoji or pictograph (all of
  U+1F000–U+1FFFF, the emoji-presentation symbols such as ⚡ ⭐ ✅ ❌,
  variation selectors U+FE00–U+FE0F and the zero-width joiner U+200D);
  an East Asian wide rune (CJK, Hangul, kana, the fullwidth forms, and
  every code point from U+20000 up); a surrogate or noncharacter; or a
  full table. `♥ ☺ ✔ ← █ ░`, box drawing and the private-use area are
  all fine. The answer is the same on every host, so it is safe to
  branch on, and `preflight`'s `lcd` check flags a refused literal
  before the cabinet does.
- **Defining a glyph reserves nothing.** The scarce thing is still the
  eight CGRAM slots, shared by both LCD rows and between your glyphs and
  the engine's. `"֍ 12"` over `"LIVES ♥♥♥"` spends two of eight. Past
  eight distinct customs across the two rows, the row being drawn shows
  `?` for the extras; the other row's glyphs stay pinned.

`sdk/tutorial/peripherals.go` is a worked example
([part 2](tutorial.md#the-hardware)).

## Environment

- [NowMicros](api/cartlib.md#func-nowmicros) is engine time, never wall
  time, and [RandomU64](api/cartlib.md#func-randomu64) is the engine-seeded
  RNG, your only randomness source.
- [Log](api/cartlib.md#func-log) writes to the host log sink; on the
  cabinet it lands in the service journal.
- [Exit](api/cartlib.md#func-exit) requests unload. Safe to call from
  anywhere; the host applies it between ticks, never mid-call.

## Sandbox caps

The hosts cap every string, buffer and table a cartridge hands them, and
an over-cap call is a warn-and-no-op rather than a crash. The caps that
cartlib exposes are named in its [constants](api/cartlib.md#constants); the
whole table, bundle caps included, is in the
[ABI reference](abi.md#sandbox-caps).

## Testing

[Getting started](getting-started.md#5-test-as-you-go) walks the three
layers: plain `go test` against the in-process fake in
`sdk/cartlib/hosttest` (`h := hosttest.New(t)`, `h.Init(game)`,
`h.Input`, `h.Tick`, then read what the fake cabinet saw), the
`carttest` golden harness over the built `.wasm`
(`internal/cartridge/wasm/sdk_template_test.go` is the starter
skeleton, `sdk/tutorial/catcher_golden_test.go` a fuller one), and the
web player. `h.AllocsPerRun` is the fast check that nothing allocates
after `Init`; `carttest.RequireNoAllocsAfterInit`, which reads TinyGo's
own report, is the oracle.
