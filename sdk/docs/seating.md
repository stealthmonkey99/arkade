# Seating, rotation, and who gets to play

The cabinet is a table. The 64×64 panel lies flat in it, and a player sits
on each of two opposite sides. That one physical fact decides more about a
cartridge's layout than any other, and getting it wrong is not subtle —
it puts a player's own pieces on the far side of the table from them.

This page is the doctrine: which of three shapes your game is, what that
implies, and which of the strategy calls to make. Every rule here has a
cartridge behind it — a shipped, working example, not a hypothetical.

## The geometry, which is the whole thing

**Physical seat 0 is the panel's RIGHT edge, and it is the RED joystick.
Physical seat 1 is the LEFT edge, and it is BLUE.**

So in an unrotated framebuffer, seat 0's territory belongs at **high x** and
seat 1's at **low x**. Not top and bottom — the top and bottom edges face
*nobody*. A card game puts P1's hand at `x = 50..59`; pong puts `paddles[0]` at
`x = 63`. Both are right, and for the same reason.

The engine can also *rotate* the panel toward one seat
(`LockPlayerToSeat`), which puts the framebuffer's logical bottom on that
seat's near side. That is a real tool, but it is not a substitute for
laying the board out correctly, and reaching for it first is the single
most common mistake in this area — see [the trap](#the-trap-rotation-is-not-a-layout).

The browser host mirrors all of this: the same strategy calls gate input
there, and rotation is applied to the rendered frame with the matching
direction remap, the way the engine's renderer does it. But strategy remains an *affordance* on every host — a
cartridge must still demux on the input record's `Player` byte and never
lean on the gate for correctness.

## Which shape is your game?

Ask one question: **does the screen contain territory that belongs to a
particular player?** A home row, a tinted half, a goal, a battery, a hand
of cards.

| | Shape | 1P | 2P | Examples |
|---|---|---|---|---|
| **1** | **Fixed sides** — per-player territory | never rotates | never rotates | card games, board games, table-hockey games, artillery duels |
| **2** | **Shared field** — no owned territory | faces whoever started | shared, upright | snake, sliding-tile puzzles, box-pushing puzzles, drop-a-disc games |
| **3** | **Opposing views** — a per-player half each | faces whoever started | each half drawn toward its own seat | a head-to-head falling-block or drop game |

A 1P-only cartridge is shape 2 by default: there is nothing to keep fixed,
so facing the player is simply right.

## What the calls actually do

Three constants, and the difference between the first two is the difference
between "the board turns" and "the board never turns":

| Call | Rotates? | Gates input? |
|---|---|---|
| `StrategyOnePlayerSolo` + lock | **yes**, toward the locked seat | yes |
| `StrategyOnePlayerAgainstAI` + lock | **no**, stays upright | yes |
| `StrategyAnyPlayer` | no | no |

`LockPlayerToSeat` is a **silent no-op unless a one-player strategy is on
top of the stack**. Under `StrategyAnyPlayer` it does nothing at all — no
rotation *and no gating* — and it does not complain. This is easy to trip
over silently (see [the traps](#four-traps-to-avoid)).

`StrategyTwoPlayerTurnBased` does not rotate either; `SwitchPlayer` only
flips the input gate. Without rotation the panel never faces the
active player and 2P has to be played sideways.

### Quit is never gated

**A locked strategy does not block B.** Whatever the gate, a quit key from
*either* seat always reaches your cartridge — you will be asked, and you
still decide what to do with it (a hold-to-confirm game still gets its
confirm). This is enforced once, in the engine's `gateInput`, because
"either seat can always walk away from a game" is a property of the
cabinet rather than of any one cartridge.

Both hosts apply the same rule. Keyboard-sourced records are never
gated. A controller's B, Quit and Escape always pass, as keydown, keyup
and keypress alike, so a hold-to-confirm can arm and complete; a quit
from a seat the strategy refused does not make that seat the primary.

A per-cartridge quit filter is easy to get wrong here specifically: a
locked one-player strategy naturally gates *everything*, including the
idle seat's B, unless something explicitly carves out an exception for
it. The engine carves out that exception exactly once, so every
cartridge inherits "either seat can always walk away" for free instead
of re-deriving it under a locked gate.

What this does **not** do is let the idle seat play: everything except a
quit key is still gated. Do not read the guarantee as permission to skip
your own seat filter.

## Shape 1 — fixed sides

**Lay the territory out along x, and never rotate.** Red/white/P1 at high
x, black/blue/P2 at low x, in every mode. Then a 1P player at either seat
sees their own side naturally, and the same board works unchanged for 2P.

For 1P you still want the idle seat locked out. Use
`StrategyOnePlayerAgainstAI`, which gates without rotating:

```go fragment
// on the title: any seat may start
PushPlayerStrategy(StrategyOnePlayerSolo)   // unlocked — permissive

// when the match begins
PushPlayerStrategy(StrategyOnePlayerAgainstAI)
```

A turn-based card game does exactly this and does its idle-seat
filtering in the cartridge, so a quit key from the other seat still
lands. That is the reference shape.

### The trap: rotation is not a layout

A board game that draws its board with ranks running along
**y** puts red's home on the framebuffer's bottom edge — nobody's
side — and then has to rotate the panel 90° to drag it around to seat 0.
That looks right on the cabinet and wrong everywhere else, and it needs a
second 180° render flip to handle a seat-1 human, plus an inverted cursor
delta to cancel *that*. Three transforms stack up to compensate for one
wrong axis.

If you find yourself writing a point-of-view flip so the human's colour
lands at the bottom, stop: the board is on the wrong axis. Rotate the
layout, not the panel.

## Shape 2 — shared field

**1P:** push `StrategyOnePlayerSolo` and `LockPlayerToSeat(observed seat)`
the moment the mode is chosen, so the "get ready" screen already faces
them. Lock to a seat you have actually seen a press from — never a
hardcoded `0`, and never a seat you have not observed, which gates the
real player out of their own game.

**2P simultaneous** (both players acting at once — snake, a maze
shooter, table hockey's 2P): never lock. The panel stays upright and both seats stay
live. Arbitrate nothing; both are playing.

**2P alternating turns** (one plays, the other waits — a hopping or
stacking game played in turns, say): **rotate toward the active player at each turn change.**
A shared board with
screen-spatial controls, left upright, forces one of the two players to
play their entire turn sideways.

Do it with a lock → push sequence, which rotates *without* gating the
waiting player out. Push `StrategyOnePlayerSolo` once, when the mode is
chosen. On the first turn, lock and then push the unlocked layer; on every
later turn, pop first, which removes only the `AgainstAI` layer:

```text
PushPlayerStrategy(StrategyOnePlayerSolo)      // once, when the mode is chosen

PopPlayerStrategy()                            // later turns only: drop the AgainstAI layer
LockPlayerToSeat(activeSeat)                   // rotates the Solo strategy underneath
PushPlayerStrategy(StrategyOnePlayerAgainstAI) // permissive gate, LEFT UNLOCKED
```

`PopPlayerStrategy` removes the top strategy unconditionally. Popping on
the first turn would remove Solo itself, and the lock would land on
`AnyPlayer` as a no-op.

The unlocked layer on top is load-bearing: it means the idle seat's B
still reaches the shared quit confirm. Keep arbitrating turns
cartridge-side, checking quit keys *before* the turn filter. The engine
gate is belt-and-braces, not the mechanism.

**The exception:** a game where both players *watch* the other's turn does
not rotate. An artillery game is the case — you follow the arc of the other
player's shot, so turning the panel away mid-flight takes the game from
the person currently looking at it. The test is not "are turns
alternating," it is **"does the waiting player look away?"**

## Shape 3 — opposing views

Draw each player's half toward their own seat **in render code**, and do
not lock in 2P. The engine can only rotate the whole panel; a split view
is by definition two orientations at once, so it is yours to draw. A head-to-head falling-block game is
the typical case. 1P collapses to shape 2: lock and face the one player.

## Name entry, in every shape

Locking to the entrant's seat is correct everywhere, including shape 1 —
it is a full-screen form, not the board, and only one person is typing.

Two rules:

1. **Make sure a one-player strategy is actually on top before you lock**,
   or the lock is a no-op and both sticks drive the same letter-picker.
   If you pushed `StrategyAnyPlayer` for 2P, pop it or push over it first.
2. **Unlock before any screen that invites both seats.** A "GAME OVER —
   PRESS A" screen that lights both players' buttons while the gate is
   still locked to one of them leaves the other player pressing a lit
   button that does nothing. Unlock the moment the name-entry
   queue drains.

## Four traps to avoid

- **`LockPlayerToSeat` under `StrategyAnyPlayer`** — silently does nothing.
  Easy to miss because a test that only exercises a 1P run never
  triggers the failure, even though the identical code path breaks
  under 2P name entry.
- **A missing unlock after name entry** — strands the other player on the
  both-seats game-over screen.
- **Rotating in a fixed-sides game** — 1P rotates the board and red stops
  being on P1's side, defeating the fixed-sides promise entirely.
- **A point-of-view flip to compensate for a rotation** — a sign the
  board is laid out on the wrong axis.

## Before you call it done

- Play 1P from **both** seats. A bug that only appears when seat 1 starts
  is the single most common shape of failure here.
- Look at an **unrotated frame** — the carttest goldens capture the raw
  framebuffer, before any strategy rotation. If the layout only makes
  sense after the panel turns, it is leaning on rotation. (The browser
  player rotates and remaps input like the cabinet, so it cannot show
  you this.)
- From every screen that shows both players something, press B **from the
  seat that is not active**. It must work.
- Check that returning to the title unlocks, on every path out — including
  the ones that call `Exit()` directly rather than going through your own
  `returnToTitle`.

See also: [Authoring patterns](patterns.md#seats-strategy-and-rotation) for
the short form, and [cartlib reference](cartlib.md) for the call
signatures.
