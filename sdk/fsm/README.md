# `sdk/fsm` — the state variable, with its obligations attached

Every cartridge with more than one screen keeps a dense `uint8` enum and
dispatches on it. **That part is good and this package does not touch it.**

What this package owns is the part that hangs off *entering* a state, which
a hand-rolled cartridge re-implements at every transition and gets wrong
the moment one path into a screen forgets a step:

| the hand-rolled thing | what replaces it |
|---|---|
| `show*Peripherals(...)` called from every path into a screen | one `OnEnter` hook |
| `confirmReturnState` / `returnSt` + its restore switch | `Push` / `Pop` |
| `nameEntryActive` and friends — a screen spelled as a bool | a state |
| `stateT` zeroed at each transition, by hand | `Elapsed()` |

It deliberately refuses to be a scene graph, a dispatcher, an event
system, a source of timed auto-transitions or a generic type.

## The whole API

```go
type State uint8
const MaxDepth = 4   // live stack entries: a base screen + three overlays
const MaxStates = 16 // states an Edges table can describe; a larger machine runs without one

m.OnEnter(fn)  // fn(prev, next) runs after EVERY arrival — Go, Push, Pop, Reset
m.OnExit(fn)   // fn(cur, next) runs before a screen is actually left — Go, Pop
m.Edges(t)     // optional []uint16 legal-transition table: validation + editor data

m.Reset(s)     // Init places the machine and fires the entry hook
m.Go(s)        // replace the top of the stack        (s must be a CONSTANT)
m.Push(s)      // layer a modal over it               (s must be a CONSTANT)
m.Pop()        // dismiss it; the covered screen re-enters and resumes

m.State()      // what is on screen — the one copy of the state variable
m.Under()      // what a modal draws over
m.In(s)        // is s anywhere on the stack (a frozen sim knows it is only frozen)
m.Depth()      // live entries
m.Tick(dt)     // once per Update, before the dispatch switch
m.Elapsed()    // dt since the top was entered — the old stateT
m.Blocked()    // refusals; a host-side test pins this at 0
```

No maps, no imports, no allocation ever (`Reset` included), no interfaces,
no generics, no closures required. 80 bytes per machine on wasm32.

## Arrays of entities: `IDMachine`

`OnEnter`'s hook is `func(prev, next State)` — no argument says WHICH
machine called it. That is fine for a screen (there is only one) and
broken for an array: four jellies, a pool of cars or `players[]` sharing
one machine definition can't tell each other apart in a shared hook. Every
workaround is bad — copy-pasted
per-entity hooks, a closure per entity (allocates), a package-level
"current entity" set before each call (the implicit coupling this package
exists to remove).

`IDMachine` is `Machine` plus a recorded id and an `OnEnterID` hook:

```go
type IDMachine struct{ Machine } // + SetID/ID, OnEnterID

m.SetID(id)     // once, when the array is built — identity, not state
m.OnEnterID(fn) // fn(id, prev, next) — ONE hook, told apart by id
```

Everything else — `Go`, `Push`, `Pop`, `Reset`, `Edges`, `Tick`,
`Elapsed`, `State`, `Blocked`, `MaxDepth` — behaves exactly like
`Machine`'s, because `IDMachine` embeds `Machine` and calls straight
through to it before firing `OnEnterID`. `OnExit` needs no identity and is
inherited unchanged.

**Singleton machines (a screen, a scene) don't need this — use `Machine`
and `OnEnter`.** Reach for `IDMachine` only when the same machine
definition is shared by more than one live instance at once. It costs
nothing to the carts that don't: `Machine`'s own definition is untouched,
so a cartridge that never declares an `IDMachine` compiles exactly as it
did before this type existed.

## Adopting it

1. Retype the enum: `const ( stTitle fsm.State = iota; ... )`. Every screen
   currently hiding in a boolean (`nameEntryActive`, `paused`) becomes a real
   member. Whole-run modifiers that are genuinely *not* screens — a game
   mode, a fade — stay plain vars.
2. Delete `var state uint8` and `var stateT uint64`. **The machine owns the
   state variable**; a shadow copy is the drift
   this package exists to prevent. Reads become `machine.State()`, timers
   `machine.Elapsed()`.
3. Move every `show*Peripherals` call out of the `enter*` functions and into
   one `enter(prev, next)` hook, switched on `next`.
4. Delete `confirmReturnState` and its restore switch. `enterConfirm` becomes
   `machine.Push(stConfirm)`, `resumeFromConfirm` becomes `machine.Pop()`,
   and the restore switch is the entry hook you already wrote.
5. Add the edge table and a test that asserts `machine.Blocked() == 0` after
   the golden journey. That is what makes an undeclared transition a test
   failure instead of a diagram that lies.

Destinations must be **constants** — `machine.Go(nextScreenFor(x))` compiles
and works, but a computed destination is invisible to any tool that reads
the transitions out of the source, so the edge table and the editor drawn
from it would silently miss it. That is why destinations are literals.

## When NOT to adopt it

Below roughly six states plus a modal it is a wash, and a plain `uint8` enum
and a `switch` is a perfectly good answer. Nothing in the SDK requires
adoption, and a cartridge shipping the hand-rolled pattern is not broken.
The tutorial's Catcher adopts it at five screens to teach the pattern, not
because it pays there.
