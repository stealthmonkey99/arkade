# Authoring patterns

The rules below hold across every cartridge — distilled to one
paragraph per lesson, each learned from a real cartridge rather than a
hypothetical. Three reference games are worth keeping open while you
read: `cartridges/life` (minimal) and `cartridges/snake` (full-featured)
from the [arkade-cartridges](https://github.com/stealthmonkey99/arkade-cartridges)
repo, and Catcher at `sdk/tutorial` (sprites, sound, levels and
goldens in a small codebase).

## Button semantics

Platform-wide rule: **B is back/exit only; D is "interact"** where a
second action button is needed, with an eligibility light. Mid-game B
opens the `ui.HoldConfirm` flow (press → release arms → hold ~1 s →
confirm on release); title/menu/game-over B acts immediately — except
over a **qualifying name entry**, where the placement isn't in the table
yet and a fumbled B would throw it away, so B opens the confirm there too
(`cartridges/snake`'s `handleGameOverInput` is the reference). This
applies to every cartridge with a name entry.
The confirm you raise is the one your cartridge *already* means by B, so
a completed hold goes wherever that overlay has always gone (out of the
cartridge for most; back to the title for a score-chasing arcade game or an artillery duel).
Hold-B exit must work from *every* state, including both seats of any 2P
phase. Don't invent per-game exit schemes.

## Designing for 64×64

- **Don't shrink the world — move a camera.** Keep the game's natural
  proportions and treat the panel as a window (overview for input
  phases, 1:1 follow for action). Shrinking a whole scene to fit has
  killed games; the camera saved them.
- **Spend pixels on the emotional beat, not sprite fidelity** — a
  screen-filling zoom at the climactic moment lands harder than a
  prettier 14×16 sprite ever will.
- **The per-seat LCD is a free second screen**: HUD, aim readouts,
  "WAIT YOUR TURN". Panel = playfield, LCD = HUD; never duplicate a
  panel affordance on the LCD.
- LED panels bloom — near-white fields glare. Check every
  foreground/background pair's contrast, err dimmer than a monitor
  suggests, and **review at 1× across the table**, not only zoomed.
  Pin hard-won contrast with a numeric regression test.
- Dense boards: compress the middle, keep the anchors legible (top
  item full-size, counts visible), add a cursor "lift" overlay, and
  let the LCD name the exact selection.

## Input truths

The ones you will hit first:

- Act on **keypress** (`r.IsPress()`), never also keydown (double-fire).
- Take direction edges from `r.PressDir()` — a real stick sends two
  records per push, and it counts one.
- Any held-state behavior must go through **`ui.HeldDirs`**
  (`held.Feed(r)`) — terminal keyboards never send keyup, so hand-rolled
  hold tracking is a recurring source of bugs.
- Route records to `ui.HoldConfirm.Feed` *before* any keypress-only
  early return, or the confirm can never arm on a real pad.
- The engine's seat-mount direction remap is ground truth — never add
  cartridge-side direction swaps on top of it. Egocentric schemes
  (tank steering) opt out via `cartlib.SetRawDirections(true)` at
  gameplay start, back to `false` on **every** path to the title, so
  menus keep the standard remapped feel.
- Give death/game-over screens a ~1.5–2 s input lockout — players are
  still mashing when they die.
- Hold-to-step a list or a cursor with `ui.HoldRepeat` (220 ms delay,
  then a step every 120 ms — the cabinet's own menus' cadence) rather
  than hand-rolled timers.

## Seats, strategy, and rotation

Affordances with sharp edges, ported to the browser host as well. The full doctrine — which
of three shapes your game is, and what each implies — is
[Seating & rotation](seating.md). Headlines:

- Physical seat 0 is the panel's **right** side (red stick); seat 1
  left (blue). So a sided board's territory belongs on the **x axis**:
  the top and bottom edges face nobody. Track identity from observed
  presses, never assumptions; a solo player's color/side follows their
  joystick.
- **A sided board never rotates**, in any mode — lay it out along x
  instead. If you are writing a point-of-view flip so the human's
  color lands at the bottom, the board is on the wrong axis. 1P still
  gates the idle seat, with `StrategyOnePlayerAgainstAI` (gates
  without rotating) rather than a `Solo` lock.
- `LockPlayerToSeat` only works with a one-player strategy on top of
  the stack — under `StrategyAnyPlayer` it silently does nothing, no
  rotation *and no gating* — and **never lock to a seat you haven't
  seen input from** (an unobserved lock gates the real player out of
  their own game).
- **Quit is never gated.** A locked strategy still passes B from
  either seat to your cartridge; the engine enforces it once, so
  nobody is ever trapped in a game they aren't playing. It does NOT
  pass anything else through — keep your own seat filter.
- **2P simultaneous play never locks the engine gate**: push once at
  match start, leave it unlocked, arbitrate cartridge-side with a slot
  filter that checks quit keys *first*.
- **2P alternating turns rotate toward the active player at each
  turn change** (left upright, a shared board ends up "played sideways" for
  the player on the far side). Use pop → `LockPlayerToSeat` → push an
  *unlocked* `AgainstAI` on top, so the waiting seat's B still
  reaches the shared confirm. The exception is a game whose waiting
  player actually watches the other's turn — an artillery game's shot arc.
- **Unlock before any screen that invites both seats.** A name-entry
  lock left on strands the other player at "PRESS A" with a lit button
  that does nothing.
- Re-assert LCDs and lights when resuming from any overlay — nothing
  restores peripheral state for you.

## Loading screens

No informational text — a loader is a short (2–3 s) animation that
hints the game's core mechanic, with a sound synced to its beats
(~0.5 peak). The title art must sit at exactly the post-load title's
position (pin it with an alignment test). Draw everything into the
`LoadFrame` framebuffer: it is the only way a pixel reaches the screen.

## Attract mode

The screensaver runs a **dedicated instance** that calls only
`cart_init`, `cart_attract`, and `cart_teardown` — no load phase, no
updates. Audio and peripherals are fully live in it, so the platform
convention is structural silence: attract plays zero sounds and
touches no LCDs/lights; any title sting is fired from a path attract
never runs (`LoadFrame`, not `Init`). Add a golden assertion counting
`host_play_sound` calls across init + a long attract run: zero.

## Sprite art as PNGs

Gameplay art lives as hand-editable PNGs, shipped in the bundle and
lifted with `Setup.LoadSheet` in `Init` into a guest-side `ui.Sprite`,
so goldens and parity hold. The alpha rule, `ui.OpaqueMask`'s: only
`A == 255` pixels enter the collision mask, so soft edges are automatically forgiving and a
"solid-looking but benign" region is authored at A=254. Keep animated
effects code-drawn on top; keep tiling/parallax placement in code with
the repeating unit as the PNG. The full pipeline, spritelab to
`Sheet.Cel`, is in [the tutorial](tutorial-assets.md#draw-the-fruit).

## Buttons and lights

Light a button **exactly when pressing it would do something** —
eligibility lighting. A statically lit button reads as a mystery; an
unlit synonym reads as a hidden control (two keys, one action: bind
one). Address lights per active seat; don't broadcast `SeatBoth`
out of convenience. Cabinet button colors players actually say out
loud: A green, B red, C yellow, D blue.

Nothing restores lamps or LCD text for you — **every path into a screen
owes them**, including the way back out from under a dismissed overlay.
That obligation is the single easiest thing to forget across the
codebase, and it is what [`sdk/fsm`](ui.md#screen-state-sdkfsm)'s entry hook exists to
make structural. A cartridge that keeps its plain mode enum instead
(most of them, correctly) has to remember by hand: grep for every
assignment to the mode variable and check each one re-asserts.

## Sound design

The cabinet's small speakers reproduce nothing below ~150 Hz —
synthesize fundamentals in the 250–1500 Hz band and make the asset
*generator* assert its spectrum (the shared synth
`tools/sfxlib` has oscillators, ADSR, noise, and Goertzel
band assertions built in). Perceptual rumble = amplitude-modulating a
mid-band carrier at 30–60 Hz. Add a ~90 ms retrigger cooldown wherever
physics can machine-gun a collision sound. Game-over deserves its own
sting, sequenced structurally after the death beat.

## LCD craft

Blank the idle seat's LCD in 1P (stale menu text reads as broken).
Custom glyphs are cheap: the driver pages ♥ ♠ ♦ ♣, arrows, dice, and
the ░▒▓█ shade ramp from a shared palette (8 distinct customs per
screen at once); both hosts draw the same 5×8 bitmap. Meters draw from
the shade ramp in Unicode order. Need a symbol the palette lacks?
Define it in your own cartridge —
[`Setup.DefineLCDGlyph`](cartlib.md#your-own-glyphs) — and leave
`internal/hardware/pi/glyphs.go` for runes every cartridge wants. The
eight slots are shared between the two sources either way.

## Storage discipline

Keys must match `[A-Za-z0-9_.-]+` — a slash means every write is
refused. `cartlib.StorageWrite` reports it by returning false, and
nothing else will: the call never reaches the host, and no log on the
cabinet says why. Give each cartridge one regression that (a) asserts
`cartlib.StorageKeyOK` for each real key, or `StorageWrite`'s result,
and (b) proves a write→read round trip on a fresh instance
(`sdk/cartlib/hosttest` keeps storage across a test). Version numbers live in two places (manifest.json and
the MANUAL.md footer) — bump both and rebuild, since the compiled-in
copy is generated from manifest.json; preflight's `manifest` check
catches the misses.

## Testing strategy

- **Golden test minimum**: determinism (two identical runs) + one
  correctness anchor, driving the checked-in `.wasm`. Keep the seeded
  input mash and the alloc soak *in* the test file — both classes of
  bug they catch (trap crashes, leak/corruption) resurface.
- **Host-Go render tests**: because draw code targets `ui.Canvas`, a
  plain `go test` can render any board state into a `ui.Layer` and
  assert pixels — no TinyGo, no wasm. Use it for worst-case layouts
  scripting a journey can't reach.
- **PNG dump hooks** gated on an env var inside the golden test are
  the pixel-art iteration loop; dump dirs go in the scratchpad, never
  the cartridge directory.
- **Use realistic record shapes** (keydown+keypress+keyup triples,
  correct `Physical` bytes, `FlagVirtual` only where true) — CLI-
  shaped test input has masked multiple real-pad bugs.
- Reproduce before fixing; prove regressions fail-then-pass against
  the pre-fix build.

## Debugging a wasm crash

TinyGo `-panic=trap` turns every runtime panic into a bare
`unreachable`. On the cabinet: frozen visuals + still-playing audio +
per-frame `wasm error: unreachable` journal spam = dead instance; the
engine logs a wasm stack trace on the first trap (function names
survive `-no-debug`). Audit slice indexes and divides on `Update`
paths — input record bytes are host data; never index by
`Player`/`Physical` unchecked (`Seat.Index` and `Slot.Index` return -1
when there is no single seat or slot).

## Before you call it done

Re-run the golden and host-render tests (see [Testing
strategy](#testing-strategy) above), and re-check the storage-key and
version-bump discipline in [Storage discipline](#storage-discipline).
If the game changes gameplay later, bump the version and re-pass the
manual.
