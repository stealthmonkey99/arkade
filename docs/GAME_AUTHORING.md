# Writing an Årkade game

Games are **cartridges**: one `.wasm` file, written in Go and built with
TinyGo, that runs on the cabinet and in a web browser from the same bytes.
A new game never needs an engine rebuild.

The step-by-step material lives in the SDK documentation set, `sdk/docs/`
(also rendered at arkade.build `/dev/docs`). Read it in this order:

1. [Getting started](../sdk/docs/getting-started.md): template to a packed
   `.rcart` playing in your browser and terminal.
2. [The tutorial](../sdk/docs/tutorial.md) and its sequels
   ([assets](../sdk/docs/tutorial-assets.md),
   [levels](../sdk/docs/tutorial-levels.md),
   [debugging](../sdk/docs/tutorial-debug.md)): one small complete game,
   Catcher, whose finished source is `sdk/tutorial`.
3. The references: [cartlib](../sdk/docs/cartlib.md), [ui](../sdk/docs/ui.md),
   [the ABI](../sdk/docs/abi.md), [packaging](../sdk/docs/packaging.md) and the
   per-asset pages starting at [assets](../sdk/docs/reference-assets.md).
4. Before you call a game finished: [Gotchas](../sdk/docs/gotchas.md),
   [Seating & rotation](../sdk/docs/seating.md) and
   [Authoring patterns](../sdk/docs/patterns.md).

Prerequisites (Go and TinyGo versions) are in the
[README](../README.md#requirements). [`GAME_AUTHORING_AI.md`](GAME_AUTHORING_AI.md)
is the same material as a dense checklist for an AI assistant, and
[`CARTRIDGE.md`](CARTRIDGE.md) is the ABI's design record.

The two games in
[arkade-cartridges](https://github.com/stealthmonkey99/arkade-cartridges)
are the best teachers: `cartridges/life` is the simplest complete game, and
`cartridges/snake` has a menu, two-player mode, high scores, sound and a
screensaver.

## Why the rules look the way they do

**One wasm, two hosts.** The cabinet runs a cartridge under wazero, the
browser under its own host. Golden tests compare their frames byte for
byte, so everything a frame shows must be a pure function of `dtMicros`,
the input records and `cartlib.RandomU64`. That is why there is no wall
clock, no `math/rand` and no libm on observable paths: each would make the
two hosts, or a replay, disagree.

**TinyGo with `-scheduler=none -gc=leaking`.** The runtime is small enough to
load instantly and never pauses to collect, but it never frees either. A
per-frame allocation leaks for the life of the cabinet process, so all
allocation happens in `Init`, and the calls that allocate are methods of the
`*cartlib.Setup` only `Init` receives. There is no scheduler, so no
goroutines. None of this fails at compile time, which is why preflight and
`carttest.RequireNoAllocsAfterInit` check it.

**Setup in `Init`, never `init`.** The host calls `Init` once, on purpose;
package initializers that call functions do not run reliably in the wasm
build, and keeping every cartridge's setup in one place makes it observable.

**Scores belong to the cabinet.** The shell's LCD readout, the attract
trophy and the leaderboard ticker read a game's table by slug from outside
the game, so a cartridge submits through the score ABI instead of keeping
its own.

**A seat is not a slot.** `r.Player` (a `cartlib.Slot`) says whose piece
moves; `r.Physical` (a `cartlib.Seat`) says which LCD and lamps face that
person. They diverge when someone picks up the other controller, and the
types keep them apart.

**Rotation is a cabinet nicety.** The cabinet can turn the panel toward a
player and gate input to them; neither host gates a keyboard record, so a
game decides correctness from the records themselves.

## Putting it on a cabinet

`make rcart` after every `make build` (the bundle is not rebuilt with the
`.wasm`), then install it as
[CABINET.md](CABINET.md#deploying-a-cartridge) describes, or upload it over
HTTP through the [dashboard](DASHBOARD.md).
