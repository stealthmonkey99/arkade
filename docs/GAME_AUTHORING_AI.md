# Building an Årkade game cartridge — AI context

Load this file at the start of a session whose job is to build a new game as
a **cartridge** (a wasm module that runs on the cabinet AND in the browser).
It is the checklist: the rules, in order, with the reasoning kept short.
The reference it leans on is the SDK documentation set in `sdk/docs/`, which
is canonical; follow a link when a section says to.

- [getting-started](../sdk/docs/getting-started.md) and
  [the tutorial](../sdk/docs/tutorial.md) — the walkthrough.
- [abi](../sdk/docs/abi.md), [cartlib](../sdk/docs/cartlib.md),
  [ui](../sdk/docs/ui.md), [packaging](../sdk/docs/packaging.md) and the
  `reference-*.md` pages — the contract and the API.
- [gotchas](../sdk/docs/gotchas.md), [seating](../sdk/docs/seating.md) and
  [patterns](../sdk/docs/patterns.md) — every trap, the rotation doctrine,
  and the design lessons, each in long form.
- `docs/CARTRIDGE.md` — the ABI's design record (why it is shaped this way).
- Reference cartridges to crib from, in the
  [arkade-cartridges](https://github.com/stealthmonkey99/arkade-cartridges)
  repo: **`cartridges/life`** (the simplest — `cartlib.Game` + a grid, no
  input/scores) and **`cartridges/snake`** (the full-featured one — menu,
  name entry, 2P, scores, attract, peripherals). In this repo,
  `sdk/template` is a minimal starter and `sdk/tutorial` (Catcher) is a
  finished small game with sprites, sound, levels and goldens.

Toolchain versions: [README.md#requirements](../README.md#requirements).

## The 30-second model

- A cartridge is a **wasm module** built with **TinyGo**. It is NOT part of
  the main `github.com/stealthmonkey99/arkade` module — it is its own Go
  module in a cartridge folder outside the engine checkout
  (`../my-cartridges/<name>/`) that imports the SDK.
- You implement **`cartlib.Game`** (5 methods). `cartlib` provides all the
  wasm `cart_*` exports and wraps every `host_*` import; you never write raw
  ABI plumbing.
- You draw with **`sdk/ui`** (host-free widgets: fonts, shapes, Label, Menu,
  NameEntry, AttractPanel) and/or `cartlib.Framebuffer` directly.
- The **host** (cabinet engine via wazero, or the browser host)
  drives your game: `Init` once, then `HandleInput`/`Update`/`Render` each
  frame. Same `.wasm` on both — this is the whole point, and the golden
  tests enforce it byte-for-byte.

## GOLDEN RULES — a cartridge runs under TinyGo `-scheduler=none -gc=leaking`

Violating any of these compiles fine and then FAILS silently or traps at
runtime. Internalize them before writing code.

Most are machine-checked: preflight's `golden-rules` check fails rules 1–5
and warns on 6 and 8, and `carttest.RequireNoAllocsAfterInit` checks rule 7.
One rule no longer needs stating at all: the allocating SDK calls (sound and
sheet loads, channel binds, glyph definitions) are methods of the
`*cartlib.Setup` only `Init` receives, so the compiler refuses them anywhere
else. `sdk/docs/gotchas.md` is the one-line-per-trap version of this list.

1. **No `func init()` and no non-trivial package-level initializers.**
   Setup goes in `Game.Init` (a real call); `func main() {}` must exist
   but stays empty. (The hosts DO call `_start` once at
   instantiation — but only because TinyGo's runtime needs it to
   initialize its allocator, see docs/CARTRIDGE.md "Runtime boot". That
   boot is not a license to put logic in `main`/`init`: package vars that
   aren't compile-time-foldable are still zero on older-host replays, and
   the convention keeps every cartridge's setup in one observable place.)
2. **Maps are ordinary Go.** A map that grows after `Init` is an
   allocation and falls under rule 7.
3. **Time comes only from `Update`'s `dtMicros`.** Never a wall clock.
   Accumulate `dtMicros` for cadences. `cartlib.NowMicros` exists but is
   engine time, not wall time; prefer `dtMicros`.
4. **Randomness comes only from `cartlib.RandomU64`.** Never `math/rand`,
   never `crypto/rand`. The engine seeds it; this is what makes replays
   deterministic. `cartlib` ships a `Float64`/`Intn`-free discipline — if you
   need a bounded int, `cartlib.RandomU64() % n`.
5. **No goroutines, channels, `time`, `sync`.** `-scheduler=none` means a
   goroutine fails the build.
6. **No `fmt`/`reflect`/`encoding/json` on any path** if avoidable (binary
   size + reflection issues). Build strings by hand (see snake's
   `buildLcdLine*`, or `Font3x5.DrawUint` which draws digits without a
   string). The manifest is a **compile-time string constant** (see below).
7. **No allocation after `Init`.** Steady-state `Update`/`Render` must not
   allocate — use fixed-size package-level scratch buffers (e.g. static
   `rankStrs`-style string tables, never `string(b[:])` per frame).
   `-gc=leaking` never collects, so a per-frame allocation leaks forever
   on a long-lived cabinet process. TinyGo also heap-allocates where Go
   would not: any value over 1 KB built in a function (a struct literal
   that resets a big struct, `*dst = *src` on a big array — use field
   resets and `copy`), and a local array whose slice is handed to a host
   call (make it package-level). `carttest.RequireNoAllocsAfterInit` runs
   TinyGo's `-print-allocs` over your source and fails on each such site
   outside `Init`. Slow event-driven allocation (LCD
   strings at ≤4 Hz) is the tolerated idiom — and
   the LCD path need not allocate at all: `cartlib.SetText` hands the host
   the strings' own bytes (so a string CONSTANT is free), and
   `cartlib.SetTextBytes` takes `[]byte` so a cart formatting into a
   reusable buffer skips the `string(buf[:n])` copy too. Both the rate and
   the allocation are the author's to get right: `cartridges/life`'s
   `setText`/`tickLcd` is the reference for a readout driven by the SIM
   (format into package-level buffers, push at 4 Hz, push IMMEDIATELY on
   any transition out of the running sim so the readout is never stale).
   A readout driven by a score or a phase change is already event-driven
   and needs none of this. A host that skips the runtime-boot call (see
   rule 1) turns crossing the initial heap boundary from a leak into
   silent memory CORRUPTION (overlapping strings, then an OOB trap frames
   later); if you see that signature, check that the host calls `_start`
   before theorizing.
8. **Basic `float64` arithmetic is fine** (IEEE-754, bit-exact across
   TinyGo and host Go). Only libm *transcendentals* (`math.Pow/Sin/Cos`)
   risk cross-host divergence — avoid them on observable paths, or the
   golden tests will disagree between hosts. (`math.Sin/Cos` for a
   one-off angle computed rarely is tolerable; per-frame is not.)

## The `cartlib.Game` interface

```go
type Game interface {
    Manifest() []byte                  // cart_manifest — see below
    Init(s *cartlib.Setup)             // cart_init — ONE-TIME setup; s is the ONLY way to load sounds/sheets, bind channels, define glyphs, and it is dead once Init returns
    HandleInput(records []cartlib.InputRecord) // one batch per frame
    Update(dtMicros uint64)            // advance one frame; dtMicros is the per-tick delta
    Render(fb *cartlib.Framebuffer)    // draw; cartlib blits for you after
}
```

Register it at package level (NOT in `main`):

```go
var game = &myGame{}
var _ = cartlib.Run(game)
func main() {}
```

**Manifest** returns `manifestBytes`, declared in the generated
`manifest_gen.go` (the build's `wasmfix` pass depends on `cart_manifest`
folding to constants, so never build the slice at call time):

```go
func (*myGame) Manifest() []byte { return manifestBytes }
```

`manifest_gen.go` is written from `manifest.json` by `make build` and is
committed; never edit it. Level packs follow the same rule:
hand-authored grids live in `assets/<name>.arkadelevels` and are the only
level files you edit; `levels_gen.go` is build output you commit, compiled
in rather than packed into the `.rcart`. Generation-time checks (a solver,
a derived par) go in `tools/genlevels`, whose `main` is `packfile.Main`.
Preflight's `levels` check fails a stale or hand-edited `levels_gen.go`.
Detail: [packaging](../sdk/docs/packaging.md),
[reference-levels](../sdk/docs/reference-levels.md).

**Optional interfaces** (implementing one is the opt-in; only `Attractor`
also needs an export):

- `Teardowner{ Teardown() }` — flush on unload.
- `QuitOwner{ OwnsQuitKeys() bool }` — take over the quit keys. Without it
  cartlib answers every q/escape/B press with `cartlib.Exit()`; a game
  with a hold-B confirm returns true and calls `Exit` itself.
- `Loader{ LoadFrame(elapsedMicros uint64, fb *Framebuffer) (ready bool) }` —
  a custom loading screen. Default (no Loader) is a splash of the manifest
  name for ~500ms. **Draw everything into `fb`**: it is the only way a
  pixel reaches the screen. The usual pattern: the title is an authored
  sprite, `assets/title.arkadesprite.png`, that the manifest names as the
  bundle's logo; Init lifts its cel with `s.LoadSheet` and `Sheet.Cel`, and
  LoadFrame blits it under whatever moves (Snake's `sprites.go` and
  `loader.go`).
- `Attractor{ Attract(dtMicros uint64, fb *Framebuffer) }` — draw the
  cabinet's attract/screensaver slot. **Requires an export shim** (opt-in):
  ```go
  //export cart_attract
  func cartAttract(dt uint64) { cartlib.DispatchAttract(dt) }
  ```

## Recipe: create a new game `mygame`

Commands run from the engine checkout unless a step says otherwise; the
game lives in a cartridge library beside it (`../my-cartridges/mygame`).

1. Copy the starter: `mkdir -p ../my-cartridges && cp -r sdk/template
   ../my-cartridges/mygame && rm ../my-cartridges/mygame/template.wasm`.
   Crib from Life (simplest) or Catcher as the game grows.
2. **`go.mod`** — its own module, with local `replace`s pointing at the
   engine checkout. Replaces are NOT transitive: declare BOTH cartlib and
   ui even though main.go only imports cartlib, and add `sdk/levels` and
   `sdk/fsm` the same way when the game uses them (`sdk/tutorial/go.mod`
   has all four):
   ```
   module arkade-mygame-cartridge
   go 1.23
   require github.com/stealthmonkey99/arkade/sdk/cartlib v0.1.0
   require github.com/stealthmonkey99/arkade/sdk/ui v0.1.0 // indirect if unused directly
   replace github.com/stealthmonkey99/arkade/sdk/cartlib => ../../arkade/sdk/cartlib
   replace github.com/stealthmonkey99/arkade/sdk/ui => ../../arkade/sdk/ui
   ```
3. **`manifest.json`** (a real file, packed into the `.rcart`): the slug
   matching the directory, `"abi":1`, title, version, description and
   players. It is the only manifest you write.
4. **`main.go`** — `cartlib.Game` impl + `cartlib.Run` + empty `main`.
   Split gameplay into more files freely; all plain (no build tags — cartlib
   carries the tinygo/!tinygo build-tag split, so `go test ./...` in the
   cartridge runs pure logic under host Go).
5. **Makefile** — the template's; set `ARKADE ?= ../../arkade`. `make build`
   is genlevelpack → manifestgen → tinygo → wasmfix (the generators always
   run and leave an up-to-date file untouched), `make rcart` bundles the
   `.rcart`, `make test` runs host-Go tests.
6. **Tests**: add `mygame_golden_test.go` beside the game in `package main`
   (so goldens and journeys share the game's helpers), using the exported
   harness `github.com/stealthmonkey99/arkade/carttest`; add
   `require github.com/stealthmonkey99/arkade v0.1.0` and
   `replace github.com/stealthmonkey99/arkade => ../../arkade` to `go.mod`.
   Model the golden on Life's `life_golden_test.go`: instantiate the built
   `.wasm`, drive deterministic frames with a scripted RNG + fixed `dt`,
   assert determinism and a correctness anchor. Test gameplay logic through
   a `sdk/cartlib/hosttest` journey (see Snake's `journey_test.go`): it
   boots the real `Game` under host Go and reads what the fake cabinet saw,
   with no wasm rebuild. See "Determinism & cross-host goldens" below.
7. Build and drive it: `make build && go test ./...` in the cartridge. To
   see it in a browser, drag the `.wasm` or `.rcart` onto the player at
   arkade.build; in a terminal, `./arkade --cartridges ../my-cartridges/mygame`.
8. **Player manual**: write `MANUAL.md` beside `manifest.json` to the
   format in the cartridges repo's
   [docs/MANUALS.md](https://github.com/stealthmonkey99/arkade-cartridges/blob/main/docs/MANUALS.md)
   (read it first — skeleton, numbers-over-adjectives, ≤80 lines): a
   `manual-icon.png` header image (the 8×8 icon upscaled ×16;
   `ARKADE_CARTRIDGE_ROOTS=../my-cartridges go run ./tools/manualicon`
   writes it from `assets/icon.arkadesprite.png`), a `# <title>` heading,
   and a `*Manual for version X.Y.Z*` footer quoting the manifest version.
   Preflight's `manifest` check enforces all three, so any later gameplay
   change that bumps the version comes with a manual pass.
9. **Preflight**:
   `ARKADE_CARTRIDGE_ROOTS=../my-cartridges go run ./tools/preflight mygame`.

## Drawing

A `*cartlib.Framebuffer` (64×64 RGBA) **is a `ui.Canvas`**, so every `sdk/ui`
widget composites into it. cartlib sends it to the host after `Render`;
there is no other way to draw. The widget, font and compositor catalogue is
[ui](../sdk/docs/ui.md); the screen-state machine is `fsm.Machine` in its own
opt-in module `sdk/fsm`.

- **Write draw helpers against `ui.Canvas`, not `*cartlib.Framebuffer`**
  Two payoffs: the same stamps draw into
  a persistent `ui.Layer` (allocate it in `Init`; never `Clear` it and you
  get the classic trail-smear effect),
  and the whole board renderer runs under HOST Go in unit tests (see
  "Determinism & cross-host goldens" below) because it never touches a host call.

### PNG sprite assets

Gameplay art lives as hand-editable PNGs
instead of vector draw code: a `<name>.arkadesprite.png` sheet (spritelab at
arkade.build/dev/tools writes its envelope of named cels and animations)
travels in the `.rcart`, and `Setup.LoadSheet` + `Sheet.Cel` lift each cel
into a `ui.Sprite` in `Init`. Draw with `sprite.Draw(cv, x, y)` — an
ordinary Canvas draw into the guest framebuffer, so goldens hold on both
hosts. A cartridge may still compile `ui.Sprite` literals in, by hand or
with its own generator; the result is the same type under the same rule.
Fields, caps and the worked example:
[reference-sprite](../sdk/docs/reference-sprite.md).

**The alpha rule**: pixels with `A==255` become the sprite's `ui.Mask` — its
COLLIDABLE body. Anything below 255 draws (alpha-blended as usual) but never
collides. The rule lives in `ui.OpaqueMask` (`sdk/ui/sprite.go`), which
every lifted or compiled sprite's mask comes from; a mask row is a `uint64`,
which is why a cel is at most 64 px on a side. So anti-aliased soft edges
are automatically forgiving, and a "solid-looking but benign" part is
authored by dropping it to **A=254 exactly**.

**Write 254, not "254-ish".** `preflight`'s sprite check
treats A=254 as the authored decision it is and passes it silently;
250–253 and scattered odd alphas keep warning, because that is what a
slipped opacity slider looks like. If a sprite genuinely needs a different
near-opaque alpha, answer for it once in the cartridge's `preflight.json`
(see `tools/preflight/doc.go`) rather than leaving a permanent yellow line
on its report card.

**Collision is mask-based, not bounding-box**: `ui.Mask.Overlaps` is
pixel-perfect mask-vs-mask (one AND per overlapping row — cheap). Give a
round/procedurally-drawn character a standalone `ui.Mask` literal for its
hitbox (a 5×5 disc, say) so it stops dying on bounding-box corners its
visible shape never touched. For legacy rect hazards, test each mask pixel
as its 1×1 cell against the rect.

**Animation**: a sheet's cels can hold a walk cycle, with the envelope
naming each cel and the animation's frames. The cartridge owns the clock —
lift the cels in `Init`, advance a step index off your own tick, and draw
the current cel.

Keep phase-ANIMATED effects (steam puffs, blinking) code-drawn on top of
the sprite — splitting "appliance = PNG, effect = code" keeps both halves in
their best medium. Anchor the sprite to the hazard via shared
`<name>SpriteDX/DY` consts next to the collision code so art and hitbox move
together. Background art works the same way as TILES: keep the tiling /
stagger / parallax placement in code and make the repeating unit the PNG.
When extracting existing vector art, transcribe it exactly and let the
golden suite prove the swap pixel-identical before anyone starts editing.
Converting art does NOT have to convert collision: a sprite whose hazard
keeps its tuned collide rects is fine — but when you DO want art-exact
collision, route every sprite kind through one
`hazardSprite(h) (sprite, x, y, ok)` function that both the renderer and
the collision code call: one anchor, zero drift. Keep phase-gated/geometric
kinds (fans, gates, telegraphed swipes) rect-based.

## Designing for 64×64

The camera, emotional-beat, LCD-as-HUD, bloom and dense-board lessons are
in [patterns](../sdk/docs/patterns.md#designing-for-6464). Beyond those:

- **8-way stick feel constants**: first step of a held
  direction lands the SAME frame (no debounce), then auto-repeat ~+220 ms
  and ~120 ms per step; for value stepping (angle/power),
  hold-to-repeat 320 ms delay ramping 140→45 ms plus an Aux fine mode. For
  "positional" feel from a digital stick: jump instantly to the pressed
  direction's canonical value, then slew while held.
- **Hash-based decoration needs a real bit mixer.** A linear congruence
  (`a·x + b·y mod m`) over a regular grid selects points on lattice lines —
  visible diagonal stripes; anything time-synchronized blinks in lockstep.
  Use an avalanche mix (wang/splitmix-style xor-shift-multiply) and derive
  per-element phases from its bits.
- **UX rules for cursor-driven games**: (a) when an action auto-routes somewhere
  else, move the cursor WITH the result — landing off-cursor reads as a
  glitch; (b) cursors wrap around edges; (c) light a button exactly when
  pressing it would do something — a statically lit button reads as a
  mystery, and an unlit synonym (two keys, one light) reads as a hidden
  control, so bind one; (d) a picked-up item must VANISH from its source
  pile while carried (render the source with it subtracted) — a reference
  that still draws in place reads as a duplicate; (e) tune "juice" physics
  with a trajectory-log test, then pin the feel with a behavior assertion
  (for example, at least 3 floor bounces per card on average) so retuning can't
  silently regress it.
- **Cabinet button colors** (internal/hardware/virtual.go): A green,
  B red, C yellow, D blue, and Aux1/Aux2 beyond. Players describe buttons
  by color — design prompts/LCD hints accordingly.

## Input

`HandleInput(records []cartlib.InputRecord)` — one batch per frame. Record
fields and kinds: [abi](../sdk/docs/abi.md) and
[cartlib](../sdk/docs/cartlib.md). The checklist:

- React to `cartlib.KindKeypress` (`r.IsPress()`). Acting on `KindKeydown`
  too double-fires.
- `Player` (a `cartlib.Slot`) decides gameplay; `Physical` (a
  `cartlib.Seat`, `SeatNone` = none) addresses peripherals. **`Player !=
  Physical`.**
- Take direction edges from `r.PressDir()`, which counts a real stick's
  joystick record + synthetic keypress once. `r.Dir()` is the level decode
  for hold tracking. For menus/name entry, map to `ui.Direction` /
  `ui.Action` and call the widget; `ui.HoldConfirm` and `ui.HeldDirs` take
  records directly through `Feed`.
- **Quit keys**: cartlib answers every quit-key press with `cartlib.Exit()`
  before `HandleInput` sees the batch, unless the game implements
  `cartlib.QuitOwner` and calls `Exit` itself.

**Microphone input (cabinet-only, opt-in).** A cabinet run with `--mic`
delivers a whistle and a bundled sound model's cues as ordinary
keypresses (`cartlib.KeyMicWhistle`, `KeyMicCue0`…`KeyMicCue7`; see
`sdk/docs/reference-mic.md`). Seven rules:

1. **Declare `"mic"`** in `manifest.json`'s `capabilities`, and list any
   `<name>.arkademic.json` model under `assets.mic`. Without the
   capability the host withholds every mic record.
2. **Handle the mic keys in the same `switch` as the buttons.** They are
   `KindKeypress` records, so `case cartlib.KeyJoyD, cartlib.KeyMicWhistle,
   cartlib.KeyMicCue0:` is the whole integration.
3. **Bypass the turn filter for mic keys** — and only for those. Every
   mic record is seatless (`Player` 1, `Physical` = `SeatNone`),
   so a cartridge-side turn arbiter would pass it on P1's turn and
   silently eat it on P2's. Route it to whoever holds the turn instead.
   A button press from the wrong seat must still be filtered.
4. **Never gate an action solely on a gesture.** A quiet room, a sore
   throat, someone who cannot whistle, and a cabinet with no microphone
   at all must all still play the whole game. Sound is a second path to
   something a button already does.
5. **A spoken action still spends what the button spends.** If the yellow
   button costs a treat, so does saying "treat" — otherwise the cue is a
   cheat code that only some cabinets have.
6. **The room is never "any key".** A confirmation that any other press
   dismisses, a title that any press starts, a name entry that any press
   advances: exclude the mic keys there (`k >= KeyMicWhistle && k <=
   KeyMicCue7`). The red button's own click can read as a clap, and a
   clap that closes the dialog it just opened is a dialog nobody can open.
7. **Test it**: build the
   record by hand and call the input handler. Five tests are the set —
   the sound does what the button does; it reaches **player two's turn**;
   a wrong-seat button press is still filtered; a cue that costs a
   resource spends it; and no mic key dismisses the quit confirmation.

**Screen-spatial vs. egocentric control schemes.** The engine remaps a
controller's directions by seat mount + display orientation before your
cartridge sees them (`internal/engine/engine.go`'s `remapEvent`). That is
right for a **screen-spatial** scheme (a direction points at an edge of the
panel: a board cursor, a menu) and WRONG for an **egocentric** one (a
direction is relative to the player's own avatar: tank rotate/thrust,
steering). 1P can look fine because that seat's remap happens to be the
identity, so test the scheme on BOTH pads. An egocentric game keeps
`manifest.json`'s `rawDirections` at its default and toggles raw mode at
RUNTIME with `cartlib.SetRawDirections(enabled bool)`, so its menus keep the
remapped feel:

- `cartlib.SetRawDirections(true)` once gameplay input begins (entering a
  GET READY screen is fine — it has no directional menu input).
- `cartlib.SetRawDirections(false)` on EVERY path back to the title menu —
  audit all of them: the normal match-end flow, quitting from the match-end
  screen, and the hold-B confirm-exit overlay that goes straight to
  `cartlib.Exit()`.
- The override resets to the manifest default when the app switches or
  unloads, so there is no matching "clear" call. A manifest-wide
  `"rawDirections": true` is for a cartridge with no directional menu at all.

## Scores, name entry, recent names

Use the **score ABI**, not private storage — the shell LCD, attract trophy,
and leaderboard ticker read a game's table by slug from outside the game.
Signatures: [cartlib](../sdk/docs/cartlib.md).

- `cartlib.ScoreQualifies(score)` gates BEFORE showing name entry;
  `cartlib.ScoreSubmit(score, name)` only on a COMPLETED run.
- Prefill `ui.NameEntry` via `cartlib.RecentName(physicalSeat, buf)` and
  call `cartlib.NoteRecentName(seat, name)` on submit, so a returning
  player accepts their name with one press. `NameEntry.Move` is
  level-triggered (poll the held direction each frame), `NameEntry.Do` is
  edge-triggered.
- **The prefill and the list are different questions.**
  `RecentName` asks "is this still the same person?" and expires after an
  hour; the list asks "who are the regulars at this seat?" and never
  expires. Call `NoteRecentName` only on a CONFIRMED name — an abandoned
  entry must teach the cabinet nothing.
- **Offer the recent list too**: one line next to the prefill,
  `nameEntryWidget.SetRecents(cartlib.RecentNames(physicalSeat))`. A
  name-entry screen without it is a bug.
- **A confirms, C erases, B is yours.** Route JoyC into
  `Do(ui.ActionErase)` and JoyA into `Do(ui.ActionActivate)`. Do NOT feed
  the quit key into the widget: handle B as a hold-B confirm, not an
  instant exit, because the placement is not in the table yet (see
  "Exit / confirm" below). B as the widget's backspace would swallow
  hold-B-to-quit for the whole entry, and on a panel with no Escape key a
  player who made the top ten would be stuck.
- If the entry is gated to one player, gate on a **pairing slot**, and
  answer the quit key BEFORE the gate. Comparing a slot against a queue
  position wedges the game: the two coincide in 2P and diverge in 1P.
- **A cheated session cannot submit, and you do not have to remember that.**
  The score calls and `NoteRecentName` all refuse once the shared debug
  combo has fired (below).

## The shared debug cheat code

**🟣 Aux1 HELD + UP UP DOWN DOWN LEFT RIGHT LEFT RIGHT.** One combo, every
game. If your cartridge wants a debug shortcut — skip to the ending, fly
invincibly, jump the board — opt into `cartlib.CheatCode` rather than
inventing an entry of your own.

```go
var cheat cartlib.CheatCode                       // package var, zero value ready

func (g myGame) Init(s *cartlib.Setup) {
    cheat.OnStep = func(int) { cartlib.PlaySound(sndTick, 0) } // optional feedback
}

func handleRecord(r cartlib.InputRecord) {
    if debugPhaseIsEligible() {
        if seat, ok := cheat.Feed(r); ok {
            enterDebugMode(seat) // the PHYSICAL seat that entered it, or SeatNone
            return
        }
    }
    ...
}
```

- **Feed it EVERY record**, above any `if r.Kind != KindKeypress { return }`:
  the Aux1 modifier is tracked from keydown/keyup pairs (and, on the dev
  console keyboard, from a keypress latch — a terminal cannot report a hold).
- **Feed it only where the shortcut means something.** Completion taints the
  session whether you act on it or not.
- **Once it fires, this cartridge instance can never post a score again**
  (`cartlib.CheatTainted()`). The flag is one-way; it dies with the
  instance, so relaunching from the shell starts fresh.
- **Say so on screen.** Recolor the playfield for the whole session and tag
  the game-over LCD, or put `<GAME> - DEBUG / NO HIGH SCORES` on the title.
- `cartlib.MarkCheated()` shuts the same door for a debug affordance that
  is not the combo.
- Anything else you persist is your problem: stop writing a saved-match
  checkpoint too, because that blob outlives the instance the taint dies
  with.

## Peripherals & sound

Signatures: [cartlib](../sdk/docs/cartlib.md),
[reference-lights](../sdk/docs/reference-lights.md),
[reference-sound](../sdk/docs/reference-sound.md),
[reference-glyph](../sdk/docs/reference-glyph.md).

- `cartlib.SetText(seat, line1, line2)` — a 16×2 LCD, addressed by the
  input's `Physical`, `SeatP1`/`SeatP2` or `SeatBoth`. Use
  `cartlib.SetTextBytes` with a reusable package-level buffer for anything
  you format, and throttle a sim-driven readout to ≤4 Hz (rule 7). **Never
  mirror a live widget to the LCD every frame**; guard on "did the text
  actually change".
- **The LCD is not ASCII-only**: the Pi driver pages custom 5×8 glyphs
  through an 8-slot CGRAM LRU from the palette in
  `internal/hardware/pi/glyphs.go` (suits, arrows, the ░ ▒ ▓ █ shade ramp —
  keep meters climbing in Unicode's order). Need a symbol the palette
  lacks? `s.DefineLCDGlyph(r, rows)` on `Init`'s Setup, up to 32 per
  cartridge ([your own glyphs](../sdk/docs/cartlib.md#your-own-glyphs)).
  The eight slots are SHARED between the engine's glyphs and yours. The
  rune must be plain, single-width and monochrome — never an emoji or a
  CJK-wide one. Preflight's LCD check reads your `DefineLCDGlyph` calls.
- Lamps: `cartlib.SetButtonLights(seat, mask, only)`, `cartlib.SetLights`,
  `cartlib.PlayLights`/`StopLights` for a bundled `<name>.arkadelights.json`
  cue, and `cartlib.SetLampLevels`/`FadeLamps` for per-lamp brightness
  (`0`..`cartlib.LampLevelMax`). The level you write is your INTENT (the
  cabinet's brightness setting scales it invisibly — never branch on it);
  a fade is host-clocked off `dtMicros`; anything that later touches a lamp
  cancels its running fade.
- Sound: `id := s.LoadSound("assets/foo.wav")` on `Init`'s Setup, then
  `cartlib.PlaySound(id, loopCount)` (`0` = once,
  `cartlib.PlaySoundForever`). Assets live in the `.rcart` bundle. On the
  web host loads are async — the load-phase splash covers the gap.
- **Synthesize SFX for small cabinet speakers** (a "chest-beat drum" with
  98% of its energy at 40–120 Hz plays correctly through the whole engine
  chain and is simply inaudible — the speakers can't reproduce sub-bass).
  Voice percussion as toms/mid-band bodies (~180–350 Hz + a noise-slap
  transient), explosions as mid-centered noise bursts with only a modest
  low thump underneath. Make the asset GENERATOR assert its spectrum (<50%
  of energy below 150 Hz, >30% in 200–2000 Hz) so the bug class can't
  return. Peak ~0.9 FS to avoid clipping under overlap. How to write the
  cue itself is under "Sound design" below.
- A first-frame sting belongs in `LoadFrame`, not `Init` (see the attract
  lifecycle below; on web an early play may be a silent no-op — accept
  that rather than building readiness machinery).

## Player strategy & display rotation

The doctrine — which of three board shapes your game is and which calls
each needs — is [seating](../sdk/docs/seating.md). Neither host gates a
keyboard record, so NEVER depend on a strategy for correctness (a 2P game
still demuxes on the `Player` byte itself).

- `cartlib.PushPlayerStrategy(kind)` in `Init`; a push is inert until locked.
- **`cartlib.LockPlayerToSeat(physical)`** rotates + gates input to a KNOWN
  seat. Do it the moment you know (e.g. at menu selection, so the "get
  ready" screen is already rotated — see snake's `handleTitleInput`).
  `cartlib.UnlockPlayerStrategy()` on return to a menu.
- **`LockPlayerToSeat` silently no-ops unless a one-player strategy is on
  top of the stack**, and `cartlib.SwitchPlayer` under
  `StrategyTwoPlayerTurnBased` flips the gate WITHOUT rotating. Give the
  panel ~2 s (a "get ready" beat) to physically rotate.
- **Never lock to a seat you haven't observed input from.** The gate then
  drops EVERY non-quit record from the real player — an unrecoverable wedge.
- **A sided board never seat-locks**: lay per-player territory along **x**
  (seat 0 at high x). If you are writing a point-of-view flip, the board is
  on the wrong axis.
- Peripherals are not restored automatically on resume: returning from an
  overlay must re-assert the resumed state's LCD text and light masks, and
  per-seat lights go to the active seat only, never `SeatBoth` out of
  convenience.
- The joystick is 8-way digital at the ABI; no analog magnitude reaches a
  cartridge — design aiming around jump+slew, not vector reads.

## Attract / screensaver (optional, cabinet-only)

Implement `Attractor` + the `cart_attract` export shim. In `Attract`:
`fb.ClearTransparent()` to overlay the engine's starfield, or `fb.Clear(...)`
to replace it. Draw `ui.AttractPanel{Title, Top, Board, Trophy:true, ...}`
fed your own scores plus any animation (`T:` scrolls an over-wide row,
`Ranks: true` numbers rows; both work off-attract too). See
`cartridges/snake/attract.go` (animated) and `cartridges/life/attract.go`
(slow fading sim). A cartridge with no `cart_attract` is omitted from the
rotation.

The attract instance calls ONLY `cart_init`, `cart_attract` and
`cart_teardown` (no load phase, no updates), and its audio and peripherals
are fully live. So attract plays **zero sounds** and touches **no
LCDs/lights**, structurally: a title sting is a pending flag consumed from
`LoadFrame` or the title `Update`, never played in `Init`. Guard it with a
golden assertion counting `host_play_sound` across `cart_init` + a long
attract run: zero. Detail: [patterns](../sdk/docs/patterns.md#attract-mode).

## Determinism & cross-host goldens

The whole ABI rests on: **everything a frame shows is a pure function of
`(dtMicros, input records, RandomU64 draws)`.** No other inputs. This lets
the cabinet host and the browser host run the SAME `.wasm` with the SAME
script and get byte-identical frames.

- **Go golden test** (`<game>_golden_test.go` beside the game):
  instantiate the checked-in `.wasm` via a scripted `carttest.FakeHost`
  (fixed `RandomU64` sequence, fixed `dt`), drive a journey, assert ASCII
  golden frames (`carttest.GoldenFrame`, regen with `UPDATE_GOLDENS=1`),
  a determinism check (two runs identical), and a correctness anchor.
- The browser host keeps its own cross-host check against the same golden
  files; the Go golden is what a cartridge author writes. Porting logic
  from native `math/rand` to cartlib's RNG is a one-time rebaseline — a
  conscious choice, not a bug.

Note the **load phase**: cartlib shows a splash, or your `Loader`, before
ticking your game, so a golden test must skip it first.
`carttest.AdvancePastSplash` does: one `Update(carttest.SplashMicros)`, long
enough for any loader, then a `Render`. The game isn't ticked during load, so
nothing advances and no RNG is drawn.

## The wazero smoke host

Pure-logic unit tests miss cabinet-feel bugs (seat gating, peripheral
routing, sound timing, camera behavior). Drive whole sessions (loader →
menu → full game → name entry → attract) on the checked-in harness while
asserting host-call side effects (lock-seat sequences, per-seat light
masks, sound counts, LCD text). The testing patterns in long form:
[patterns](../sdk/docs/patterns.md#testing-strategy).

- **PNG dump hooks inside the golden test**, gated on an env var
  (`MYGAME_DUMP=<dir>` writes every asserted frame as a PNG). This is the
  whole pixel-art iteration loop, and it stays in the repo.
- **Host-Go render tests via `ui.Layer`**: because the renderer takes
  `ui.Canvas`, a plain `go test` can render any board state into a
  `ui.NewLayer(64,64)`, assert pixels (`lay.At`), and dump a PNG — no
  TinyGo, no wazero. Use it for worst-case layouts a journey can't reach.
- **Keep the input mash IN the golden test file** (seeded, a few thousand
  records) — it catches heap-corruption bugs within ~120 events. Same for
  an alloc soak: thousands of input-bearing ticks with LCD churn.
- **Pure physics/feel tests**: log a trajectory once to understand the
  motion, then keep a behavioral assertion.
- **Use realistic input records or the harness lies.** The engine derives
  the `Player` byte from the seat (`internal/cartridge/wasm/keys.go`), so a
  solo player on physical seat 1 sends records with `Player` byte 2. A
  real stick emits BOTH a joystick record and a synthetic keypress; buttons
  emit keydown+keypress pairs. Emulate the engine's input gate from the
  cart's own lock/unlock calls.
- **Seeded input fuzzing** on the same harness finds `panic=trap` crashes
  (see below) that scripted journeys never reach: random-but-seeded scripts
  of edge mashing, boundary holds, second-seat record injection, long runs.

## Debugging a wasm crash on the cabinet

TinyGo `-panic=trap` turns EVERY runtime panic into a bare `unreachable`
trap. On the cabinet: frozen visuals + still-playing audio + per-frame
`wasm error: unreachable` in `journalctl -u arkade` = dead instance; the
engine logs a wasm stack trace on the first trap. Prevention: audit every
slice index and divide on `Update` paths; the input record's bytes are host
data — never index by `Player`/`Physical` unchecked.

**Before you go reading logs, look for the flight recording.** The engine
dumps every crashed session to `data/flight/`, replayable with
`arkade --replay data/flight/<timestamp>-cart_<slug>-crash.arkflt` — same
seed, inputs and cadence, with pause and single-step. See
**[FLIGHT_RECORDER.md](FLIGHT_RECORDER.md)**, including the hash-pinning
rule (a replay needs the *cabinet's* `.wasm`, not a local rebuild).

## Build / test / deploy commands

In the cartridge folder (`../my-cartridges/mygame`):

```
make build      # genlevelpack + manifestgen + tinygo + wasmfix -> mygame.wasm
make rcart      # bundle mygame.wasm + manifest.json + assets/ into mygame.rcart
go test ./...   # golden/determinism + pure-logic unit tests
```

From the engine checkout:

```
ARKADE_CARTRIDGE_ROOTS=../my-cartridges go run ./tools/preflight mygame
```

**Repack before you ship**: `make build` rebuilds the `.wasm` but does
NOT repack the `.rcart` — always run `make rcart` after a wasm rebuild or
you deploy a stale bundle that still registers fine. Installing on a
cabinet and the health check after it:
[CABINET.md](CABINET.md#deploying-a-cartridge).

## Pitfalls checklist (verify before considering it done)

- [ ] No `init()`; setup is in `Init`. `func main(){}` present and empty.
- [ ] Time only from `dtMicros`; randomness only from `RandomU64`.
- [ ] No allocation after `Init` (fixed scratch buffers, in-place resets);
      `carttest.RequireNoAllocsAfterInit` in the golden suite checks it.
- [ ] `Manifest` returns the generated `manifestBytes`; `manifest.json` has
      `abi:1` and the right slug/title/players (preflight `manifest`).
- [ ] `go.mod` has BOTH cartlib and ui replaces (not transitive), plus
      `sdk/levels`/`sdk/fsm` when used.
- [ ] Quit keys: either cartlib's default exit, or `QuitOwner` plus your
      own call to `cartlib.Exit()`.
- [ ] Gameplay demuxed by `Player` (the compiler already refuses a `Slot`
      where a peripheral call wants a `Seat`); direction edges from
      `PressDir` (the stick double-fire guard).
- [ ] Scores via the score ABI (not private storage); submit only on a
      completed run; name entry gated by `ScoreQualifies`.
- [ ] `make build` builds under TinyGo (the real safety check).
- [ ] Go golden test: determinism + a correctness anchor pass; keep the
      seeded input mash + PNG dump hooks in the test file, not scratch.
- [ ] Strategy/rotation only relied on for UX, never correctness.
- [ ] **2P quit is reachable from BOTH seats, always.** During any 2P
      phase, `LockPlayerToSeat` must
      never be the thing enforcing turns — grep your game.go for
      play-time lock calls: the only locks allowed are 1P seat adoption,
      a one-time 2P orientation lock (kind 1, seat-rotated boards), and
      documented secrecy phases (a hidden-placement phase). Turn enforcement
      lives in a cartridge-side slot/seat filter with quit keys checked
      BEFORE it; the gating golden test pins "exactly the startup push
      shape, zero play-time locks"; an off-turn/off-seat-B-opens-confirm
      regression exists; confirm LCDs/lights address `SeatBoth`; and
      post-game "press any key" screens accept either seat.
- [ ] Buttons light by ELIGIBILITY (and only bound keys light); carried
      items render subtracted from their source; auto-routed actions move
      the cursor with the result.
- [ ] `.rcart` repacked (`make rcart`) after the last wasm
      rebuild, before deploying.
- [ ] MANUAL.md written (the cartridges repo's docs/MANUALS.md format),
      manual-icon.png generated; footer version == manifest version.
      Gameplay changes to an EXISTING cartridge: bump the manifest
      version AND re-pass the manual (preflight's `manifest` check
      enforces).
- [ ] `ARKADE_CARTRIDGE_ROOTS=../my-cartridges go run ./tools/preflight
      mygame` passes.

## Reference files, ranked by usefulness

- `sdk/template/main.go` — the minimal cartlib.Game skeleton, over-commented.
- `sdk/tutorial/` — Catcher, the tutorial's finished game: sprite sheet,
  sound, levels, peripherals and goldens in a small codebase.
- `cartridges/life/` (arkade-cartridges repo) — simplest real game: grid, `RandomU64` seeding,
  dt-cadence, LCD text, a slow-fading `Attract`. Start here.
- `cartridges/snake/` (arkade-cartridges repo) — the full example: 1P/2P mode `ui.Menu`, `ui.NameEntry`
  with recent-name recall, per-seat LCDs, button lights, sound, the score
  ABI, `LockPlayerToSeat` rotation, and an animated `Attract` with the trophy
  panel. Crib patterns here. It is also the asset-first layout: the title
  and the pickups are spritelab sheets lifted in Init (`sprites.go`), every
  cue is a soundlab file, and code draws only what is computed per frame.
- `sdk/cartlib/` — the API you call (wrappers.go, game.go, framebuffer.go,
  input.go). `sdk/ui/` — the widgets. `sdk/docs/abi.md` — the ABI.

## Cabinet conventions

These rules each began as a shipped bug or a rework. Read this section
before designing ANY new game — it is the difference between one review
round and five. The short forms of many of them are in
[patterns](../sdk/docs/patterns.md); the ones below are the checklist.

### Loading splash screens

- **No informational text.** Never "1P or 2P", never button hints
  ("A to fire") — loaders show a fun ANIMATION that hints the game's
  core mechanic, subtly or overtly: snake's loader snake eats a food
  pixel (nom sound, tail grows); a platformer's hero hop-lands three
  times; a falling object drops from the top edge and bounces into the
  title; a trail game draws the menu's own trail motif tail-to-head.
- **Every loader has a sound**, synced to the animation's beats — not a
  timer. ~0.5 peak (quieter than gameplay's 0.9), spectrum-checked,
  fired via one-shot flags consumed only in `LoadFrame` (structurally
  attract-safe: attract never runs the load phase). Reuse the game's
  own sound family where it fits the fiction.
- **The loader's title art sits at EXACTLY the post-load title's
  position** — a 1-6px shift on load completion reads as a glitch.
  Pin it with an alignment regression test; best case the final
  loader frame is pixel-identical to the menu.

### Input

- The **terminal keyboard emits ONLY keypress** records (no
  keydown/keyup — a tty can't see release). ANY held-state consumer
  (name entry scrolling, tank rotation, paddle slew, brake/tuck) must
  use `ui.HeldDirs`, fed every record with `Feed`. Never hand-roll held
  tracking from keydown/keyup alone.
- **Stick double-fire**: a real stick emits a joystick record AND a
  synthetic non-virtual keypress per push. Take direction EDGES from
  `rec.PressDir()`, which acts once. Without it a menu jumps two items
  per push; 2-item menus hide the bug (clamping) until a third row
  appears.
- **Keyup must REACH the confirm widget.** An early
  "keypress-only" filter that returns before the state switch eats the
  B release, and `ui.HoldConfirm`'s press-then-hold can then never arm
  on a real pad. Route confirm-state records to `HoldConfirm.Feed`
  BEFORE any such filter, or never early-return at all (snake's shape).
- **The engine's cabinet-mount remap is ground truth** — including on
  the dev desk. NEVER add cartridge-side seat direction swaps on top of
  it; pin that with a test that 2P keys pass through unswapped.
- Hold-to-scroll is `ui.HoldRepeat` (joystick level + 220ms/120ms
  repeat); the cabinet's own menus and a cartridge's lists share it.

### Input, turn changes, and pacing

- **Re-target seat locks the INSTANT a screen demands that seat's
  input** — not when the following phase begins. A placement curtain that
  asks P2 to press A while the engine gate still filters to P1 is
  hard-stuck on real pads and invisible to fake-host tests. When a
  test must prove gating, emulate `AllowInput` from the recorded
  strategy calls.
- **Death/game-over screens need an input lockout** (~1.5-2s) with a
  real animation beat — players are still mashing the fire button when
  they die. **The lockout gates BUTTONS ONLY — never the stick**: A, B
  and C COMMIT, LEAVE or DESTROY (submit a name, exit, erase a recalled
  letter) — exactly what a post-death mash lands on — while directions
  only move a cursor the player can already see. So:

  ```go
  func handleGameOverInput(action, quit, erase bool, r cartlib.InputRecord) {
      trackUIHeld(r)          // ALWAYS first: feeds NameEntry.Move's per-frame poll
      if stateT < gameOverLockMicros {
          return              // A/B/C only — the stick already got through
      }
      if nameEntryActive && !quit { /* Do(ActionActivate) / Do(ActionErase) */ }
      ...
  }
  ```

  A lockout over an animation with no entry drawn yet (a crash, a death
  scene) refuses everything. Pin it with a test that the end lockout
  gates buttons, not directions.
- **Hold-to-move (DAS) numbers**: 260ms delay,
  100ms repeat. The virtual keypress credit (250ms) is deliberately
  under the delay, so CLI taps can never auto-repeat.
- The CLI's 1/2 seat-simulation emits controller-source records that
  DO carry FlagVirtual (virtual- ID prefix) — cover that shape in any
  bespoke input tracker (a hidden secret sequence, say), and give hidden
  sequences a quiet per-step feedback tick.

### Seats, colors, and rotation

The geometry, the three board shapes, quit-is-never-gated and the
alternating-turn lock sequence are [seating](../sdk/docs/seating.md). The
checklist:

- **Physical seat 0 = panel's RIGHT side = the RED joystick; seat 1 =
  LEFT = BLUE.** Identity decisions key off the OBSERVED physical seat of
  the press that chose the mode, never assumptions.
- **1P seat identity**: the solo human's color/side/corner follows
  their joystick (red seat → red pieces on red's side; blue seat → blue
  on blue's side), and the AI takes the vacated identity INCLUDING its
  consequences (the AI moves first when it holds the side that opens).
  Watch for palettes keyed by array index — the solo entity is always
  index 0 and will render the wrong color for a seat-1 player.
- **A sided board never takes a seat lock.** Test it: the first playing
  frame must be pixel-IDENTICAL for 1P-from-seat-0, 1P-from-seat-1 and
  2P, with the same side on the high-x edge in all three.
- Gating input without rotating = `StrategyOnePlayerAgainstAI` (kind
  2) locked to a seat; rotating toward a seat = `StrategyOnePlayerSolo`
  (kind 1) locked. The wasm harness cannot see host-side rotation —
  assert the recorded STRATEGY-CALL SHAPE as the oracle.
- **The two turn gates must AGREE.** Turn-based 2P games gate twice: the
  ENGINE gate (`LockPlayerToSeat(seatForTurn())` on kind 2 — whose pad's
  records arrive) and the CARTRIDGE gate (`r.Player-1 == slot` — whose
  records act). Slots are pairing-anchored, so in 2P `seatOfPlayer[0] = 0;
  seatOfPlayer[1] = 1` ALWAYS — never the picker's seat, or whenever seat 1
  starts the game NEITHER player can move. Regression pattern: start 2P
  from seat 1, play the opener from seat 0.
- **2P turn-based games never leave the engine gate LOCKED during
  play.** The engine passes quit from either seat, but a locked gate still
  eats the off-turn player's every OTHER key, which a shared quit
  confirm's A answer needs. The cartridge-side slot/seat filter is always
  the real turn arbiter.
- **Never rotate toward the QUITTER, and the 2P quit confirm is
  SHARED and UPRIGHT.** Either seat opens AND answers it with zero
  strategy calls; on a board that IS seat-locked while the overlay is up,
  set `ui.HoldConfirm.QuarterTurns` to counter the rotation (seat
  0-facing boards are drawn CCW, so QuarterTurns 1). Changing sdk/ui
  invalidates every cartridge's WasmMatchesSource — budget a rebuild of
  every cartridge into any shared-widget change.
- **Pre-game sub-menus in 2P stay upright and ungated.** Do the match's
  entire strategy setup at GET READY, and track whether kind 2 was
  actually pushed so backing out of a sub-menu doesn't blind-pop the base
  strategy.

### Exit / confirm

- Standard: mid-game B opens `ui.HoldConfirm` — press, RELEASE
  (arms), press-and-HOLD ~1s, blink-blink acknowledgment, exit fires
  only after release (so the shell never inherits a held B). Title/
  menu/game-over B acts immediately. A stateless toy (life) may exit
  instantly, and a game may answer with a pause menu instead. The dot is
  a plain red circle — no letter on it.
- **A qualifying name entry is the exception to "game-over B acts
  immediately"**: the placement is not in the table yet, so one fumbled B
  discards a top-ten score. B opens the hold-B overlay OVER the entry
  (draw the entry underneath, frozen); any other button resumes typing; a
  full hold leaves without submitting. Push the overlay as its own screen
  on an `sdk/fsm` machine (`sdk/tutorial`'s `openConfirm`/`closeConfirm`
  is the reference): `Pop` re-enters the covered entry, which restores its
  peripherals. Four things that are settled:
  - **The confirm you raise is the one your cartridge already means by
    B.** If the overlay exits, the completed hold exits; if B has always
    gone back to the title, so does the hold (a game may retitle it
    `LOSE SCORE?`).
  - **It stays BELOW the death-screen lockout**, never above it — it is
    what a post-death mash lands on. Pin it per cartridge with a test that
    the lockout still swallows B.
  - **A 2P `QuarterTurns` counter-rotation gets suppressed over an
    entry**: the panel is already locked to the entrant.
  - **A queued multi-seat entry**: either seat may raise the overlay, a
    cancel returns to exactly the queue position that was up, and a
    completed hold abandons the whole remaining queue.

  A game whose entire exit vocabulary is menus may answer with a menu
  (`LOSE SCORE?` / `KEEP TYPING` / `DISCARD`, KEEP preselected). A game
  that cannot be driven to a qualifying entry by any scripted `.wasm`
  journey covers the path with host-Go tests. The harness is
  `carttest.NameEntryB` — use it rather than rewriting the assertions, and
  reach for its exported `HoldB` alone if your cart's confirm doesn't exit.

### Menus and text fitting

- **The standard menu geometry is `ItemH: 10, Spacing: 1, TextDY: 1`.**
  Default-height items can run off the 64px panel. The LED caps ink only
  rows 1-5 of their 8-row bitmap, so `TextDY: 1` (ui.Button/Menu) is the
  optical-centering nudge. PNG-dump every menu STATE and confirm all items
  fully visible with clearance.
- **`ui.Menu.H` enables real viewport scrolling** — it is OPT-IN and
  silently off otherwise. For ≤4 items, fit with geometry instead.
- **Label rhythm**: when 1P entries are "1P EASY/…", the 2P entry is
  "2P GAME" (or "2 PLAYER" where 1P has no prefix) — never a lone
  "2 PLAYERS" breaking the column.
- `ui.Font3x5` has no `&`, `/`, and other punctuation — glyphs render
  silently blank. Verify every literal string's characters exist, or
  hand-plot a cartridge-local mini-glyph.

### LCDs

- **Blank the idle seat in 1P** at match start AND on resume-from-confirm;
  restore both at the title.
- **The panel is playfield; the LCD is HUD.** Score/lives/wave belong
  on the LCD (hearts for lives). Never duplicate a panel affordance on the
  LCD.
- **Count glyphs per screen, not per cartridge**: eight CGRAM slots per
  LCD, shared between the engine's palette and your `DefineLCDGlyph`
  runes, so count what can be on ONE screen at ONE moment.

### Storage

- **Keys must match `[A-Za-z0-9_.-]+` — no slashes**; `StorageWrite`
  returns false and nothing else says so. One regression per cartridge
  asserts its real keys and a fresh-instance round trip
  ([patterns](../sdk/docs/patterns.md#storage-discipline)).

### Versions

- Version bumps live in TWO places (manifest.json and MANUAL.md's footer),
  and the compiled-in copy is regenerated at build time, so a bump without
  a rebuild ships the old number. Preflight's `manifest` check catches both.

### AI opponents

- **Difficulty is an execution model, not a think-timer.** AI pieces
  must ride the same gravity/pace physics a human gets, with assists
  (hard-drop etc.) as occasional per-piece dice rolls (~5/15/30% by tier).
- **"Dumber" must never mean suicidal.** Uniform-random legal moves
  top the AI out and read as broken; mistakes pick from the 2nd-4th
  BEST placements (weighted toward 2nd). Pin with sims: easy survives
  solo (multiples longer than a suicidal model) AND underperforms hard.

### Sound design

- **A cue is an `sfxlib.Patch` — data, not a loop.** The grammar is
  published in
  [sdk/docs/reference-sound.md](../sdk/docs/reference-sound.md#the-patch-params);
  do not restate it, read it. In `tools/gen-sfx`: build the patch,
  `p.Render(rate)`, assert the spectrum, then `sfxlib.WriteAssetFile`
  with `p.MarshalParams(name)` as the recipe node's `Params`, so soundlab
  reopens the cue with knobs. Life's and Snake's cues skip the generator
  entirely: each `.arkadesound.wav` is the source, edited and re-rendered
  in place with soundlab at arkade.build/dev/tools. Keep a `tools/gen-sfx`
  only for a cue computed from something that is not itself an asset. Do
  NOT write a new cue as a loop of `sfxlib.Sine` calls; when converting an
  existing hand-written one, `tools/assetdiff` says `SAME` or `DIFF`.
- **Start from a template.** `sfxlib.Tap`, `Impact`, `Sweep`, `Bonk`
  (`Bonk(BonkLowHz, BonkHighHz, WaveSquare)` is the usual refusal),
  `Scrape`, `Motif`+`Run`. A template is where a cue STARTS, not an
  obligation. There is deliberately no template for an engine drone or a
  character's voice.
- **A voice is an `sfxlib.Vocal`**: reach for it when a mouth would SAY the
  sound; a voice over other material is a vocal node beside a patch node in
  `compose`, never a layer.
  Spec: [the `vocal` params](../sdk/docs/reference-sound.md#the-vocal-params).
- **Words are `tools/speech`**: text read by an offline neural voice at
  build time. Use it when the sound must be UNDERSTOOD; set
  `HighpassHz: 150` for a man's voice or the spectrum assertion fails on
  his fundamental; commit the rendered asset.
  Spec: [the `speech` params](../sdk/docs/reference-sound.md#the-speech-params).
- **The spectrum rule applies to every cue**, patch, vocal, speech or
  hand-written: `sfxlib.AssertSpectrum` before it is written (<50% of
  energy below 150 Hz, >30% in 200–2000 Hz; fundamentals ~250–1500 Hz).
- Nothing is sampled. Prefer synthesizing a treasured sound to preserving
  a recording of it: data can be pitched, spectrum-checked and re-rendered
  to the same bytes.
- A sound's envelope `generator` says who owns its file. A cartridge's
  `gen-sfx` must not overwrite a soundlab-owned file; regenerate those by
  reopening the file in soundlab and exporting it over itself. A patch is
  the one relaxation: retune it in soundlab, then carry the numbers back
  into `gen-sfx` (Copy Go snippet).
- On a cabinet with a microphone, a game's own effects are **cancelled**
  out of what the detectors hear. If the lab shows one reaching a
  detector, check `/api/mic/status` for a stinger that was not heard
  rather than working around it in the game.
- **Perceptual rumble**: modulate a mid-band carrier's AMPLITUDE at
  30-60Hz (tremolo) for growl, with whole modulation cycles per loop so the
  seam doesn't click.
- Game-over deserves its own sting (a two-note descending "womp womp"),
  sequenced structurally after the death beat; pin plays-exactly-once.
- Telegraph hazards audibly (a ding before the pop) — the warning IS the
  fairness.
- Patterns that work: escalating pitch tiers by value with low-to-high
  70ms stagger for combos; one sound family with per-direction
  pitch/timbre tilts; doppler noise-sweeps for pass-bys; per-bounce
  diminishing thumps; a 90ms retrigger cooldown wherever physics can
  machine-gun a collision sound.

### Visual / LED

- LED panels bloom: near-white fields glare. Compare candidate palettes
  with contrast ratios for EVERY foreground/background pair, err dimmer
  than a monitor suggests, and pin contrast numerically with a minimum
  per-channel delta regression.
- **Review at 1x — the across-the-table test** — as well as zoomed.
- Blink/pulse cadences: ~1Hz cursor pulse; golden tests must land on a
  deterministic blink phase (derive, don't hope).
- Effect state reads best as environment color (snake's border tint
  per pickup, blinking near expiry) — not corner widgets; and a
  dimmed border for ghost-type effects so "walls still kill" stays
  legible.
- **Fixed pixel insets, not proportional spacing**, for pips/glyph
  internals — proportional gaps shrink to zero at small sizes. Derive and
  pin the minimum size that keeps 1px of body between elements.
- Judge choreography (climb-outs, transitions) as frame PAIRS in a
  filmstrip — a cut reads instantly even when each frame is fine.
- PNG dump dirs belong in a scratch directory, never inside the
  cartridge dir.

### Process

- **Reproduce before fixing**: scripted journeys + per-frame PNG dumps
  find the real mechanism where armchair analysis fails.
- **Fail-then-pass proof**: rebuild the pre-fix wasm (`git show HEAD:`)
  and demonstrate the new regression test failing against it before
  trusting the fix.
- **Test with real record shapes**: keydown+keypress+keyup triples,
  correct Physical bytes, FlagVirtual only for keyboard.
- When geometry/feel questions loop, get ONE empirical data point
  (a paired-pad probe, a screenshot, an engine-table probe test)
  instead of another theory round.
- When a note about color/layout is ambiguous, restate your concrete
  reading in the work order before building it.
- A hidden demo/sandbox mode (undocumented input sequence) pays for
  itself: autopilot, player-takeover, invincibility that BLINKS-and-
  continues, timeline markers with skip keys. Keep normal play
  byte-identical and pinned lethal by regression.
