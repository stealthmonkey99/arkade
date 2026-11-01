# Designing for the hardware

The cabinet is a 64×64 LED panel lying flat in a table, a joystick and
lit arcade buttons at each seat, and a 16×2 LCD in front of each player. These
are the lessons about each of them that every cartridge ends up
learning; where the players sit relative to all of it has
[a page of its own](seating.md).

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

## Button semantics

Platform-wide rule: **B is back/exit only; D is "interact"** where a
second action button is needed, with an eligibility light. Mid-game B
opens the `ui.HoldConfirm` flow (press → release arms → hold ~1 s →
confirm on release); title/menu/game-over B acts immediately — except
over a **qualifying name entry**, where the placement isn't in the table
yet and a fumbled B would throw it away, so B opens the confirm there too
(`cartridges/snake`'s `handleNameEntryInput` in `input.go` is the reference). This
applies to every cartridge with a name entry.
The confirm you raise is the one your cartridge *already* means by B, so
a completed hold goes wherever that overlay has always gone (out of the
cartridge for most; back to the title for a score-chasing arcade game or an artillery duel).
Hold-B exit must work from *every* state, including both seats of any 2P
phase. Don't invent per-game exit schemes.

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
