# `sdk/levels` — authored levels, once, for every cartridge

Every cartridge with hand-authored levels tends to invent its own format:
generated `levelDef` literals, fixture string grids, baked solution
bitmaps. They all mean the same thing — **a
grid, a legend, and some numbers** — so this package says it once.

The pipeline has the same shape as the manifest's (`manifest.json` →
`manifest_gen.go`):

```
assets/*.arkadelevels  --[ make build ]-->  levels_gen.go
  hand-authored                              build output,
  source of truth                            checked in
```

A cartridge commits **both**. The text pack is the only file a human edits
and what a diff shows; the generated Go is build output, like
`manifest_gen.go`, and is what TinyGo compiles. Every cartridge build runs
`tools/genlevelpack` first and rewrites nothing that is up to date, and
preflight's `levels` check fails a `levels_gen.go` that is stale,
hand-edited or missing.

## Layout

```
sdk/levels/
  levels.go      the runtime half — Pack, Legend, Level, Layer, Entry, and
                 their accessors. Zero imports, no maps, no allocation:
                 this is the code that runs inside the cartridge
  packfile/      the host half — the pack file format, its parser, the
                 emitter, and Main + Hook. Never compiled into a
                 cartridge, so it uses maps and `fmt` freely
  example/       the worked example: example.arkadelevels, the levels_gen.go the
                 generator makes of it, and the few lines a cartridge
                 writes to read them
```

## The pack file

One file, one pack, `.arkadelevels` (a file ending `.levels` is read the
same way). The full spec (every directive, every error)
is `packfile`'s package doc — `go doc ./sdk/levels/packfile`. The shape:

```
// Comments are whole-line and start with //.
pack solo

legend
  #  wall
  \s floor
  .  target
  $  crate
  *  crate-on-target

level L01
  p1x 3
  p1y 1
  coop false
  |#####|
  |#.$ #|
  |#   #|
  |#####|
```

Grid rows are delimited by `|` on **both** sides. That is not decoration: a
floor cell is very often a space, and an editor that strips trailing
whitespace would otherwise silently narrow a row. Rows must all be the same
width — a ragged grid is an error with a line number, never a padded guess.

Metadata is an open set, not a fixed struct: `par` means something to a
crate-pusher, `budget` and `goal` to a tray-packing puzzle, and neither
cartridge should carry
the other's fields. A value's type comes from its spelling (`12` is an int,
`true` is a bool, anything else is a string) and is read back through the
matching accessor — `lv.Int("par")`, `lv.Bool("coop")`, `lv.Str("hint")`. A
key read at the wrong type reads as MISSING rather than as zero, so a typo
in a pack file cannot quietly become a default.

Levels needing a second grid over the same board — a puzzle's pre-placed
tray dividers — declare `layer NAME` after the main grid. A layer has its own
dimensions (an edge grid is a column wider than the cells it separates) and
shares the pack's legend unless it names another one.

## Beyond one byte per cell

A grid says where things are one byte at a time. Three kinds of authored
data do not fit in a byte, and each has its own construct — shaped by an
exploration game's room graph and a nonogram's indexed reveal art, and
general on purpose:

```
pack rooms

legend
  #  wall
  \s floor

// A second alphabet, for a layer whose cells are not terrain. Each entry
// may carry a PARAMETER, so the palette lives with the characters it names.
legend palette
  r red   #d04040
  g grass #40a040
  . none

level west-floor
  wing west
  |####|
  |#  #|
  |####|
  exit right west-pod
  exit up corridor-n2 mapwing
  ent terminal 12 48 file1
  ent npc 40 48 guard
  ent patrol 8 52 24 52 40 52
  layer reveal palette
  |rg..|
  |....|
  |....|
```

**`ent KIND X Y [ARG ...]`** is one `levels.Record` — a typed thing at a
position. `KIND` is the cartridge's own word, exactly as a legend name is;
this format never learns what "terminal" means. `X` and `Y` carry **no
unit**: a cell-unit record would round away a terminal authored at pixel
(12, 48) inside a 16×16 grid of 4-pixel tiles, and a pixel-unit one would
make a cell-grid game multiply by a tile size the format cannot know
either. They are two integers, and a `Hook` is
where "inside the panel" gets checked in units that mean something.

Arguments are positional and typed by spelling — the same rule metadata
follows — read back through `levels.Args`:

```go
term := lv.Record("terminal")
file, _ := term.Args.Str(0)
```

A **route** needs no list syntax: it is a record whose arguments are
alternating x and y, walked with `Args.Point(i)`. That is why the grammar
has one record form and not two.

**`exit DIR TARGET [LOCK ...]`** is one `levels.Exit`. `DIR` is a bare
token the cartridge gives meaning to (`left`, `next`), unique within the
level. `TARGET` is another level's name **in the same pack**, and it is
resolved at parse time: an exit to a level that does not exist is a build
error with a line number, and `Exit.Target` is an index into `Pack.Levels`
rather than a name the cartridge would have to look up at runtime. That
resolution is also the reason a room graph belongs in **one** pack file —
a reference the parser cannot see is a reference nothing later can check.

**`legend NAME`** declares an extra legend, and any legend entry may carry
one value after its name. That is what makes an indexed-colour layer
possible without a hand-maintained table beside the grid:

```go
lg := pack.LayerLegend(lv.Layer("reveal"))
rgb, ok := lg.Values.Color(lg.Ordinal(c))
```

`#d04040` is a colour, the fourth value kind alongside int, bool and
string. Every valued entry of one legend must be the same kind (a parallel
table with two types in it is not indexable), and an entry written without
a parameter reads as missing at *every* type rather than as a zero of one.

## Reading a pack from a cartridge

```go
lv := &soloPack.Levels[n]
for y := 0; y < lv.H; y++ {
    for x := 0; x < lv.W; x++ {
        switch lv.At(x, y) {
        case '#':
            drawWall(x, y)
        case '$':
            spawnCrate(x, y)
        }
    }
}
par, _ := lv.Int("par")
```

`lv.Row(y)` gives a whole row as a string without copying, `lv.Count(c)` and
`lv.Find(c)` scan the grid, and `soloPack.Legend.Ordinal(c)` turns a cell
character into a dense index for cartridges that would rather have parallel
tables than a `switch`. That is the entire runtime surface.

**Golden rules** (`docs/GAME_AUTHORING_AI.md`) hold by construction: no maps
(the legend is a string scanned linearly, metadata a short sorted slice), no
imports at all, and no allocation — not even at load, because there is no
load. A grid is one string constant, emitted one row per line so the
generated file still reads as a picture:

```go
Cells: "#####" +
	"#.$ #" +
	"#   #" +
	"#####",
```

Go folds that to a single compile-time constant. Everything the generator
emits is a foldable composite literal — no `init`, no function calls,
nothing package-level that has to *run*. That matters: a table built from a
helper function is silently **absent** in the wasm build while every
host-Go test passes.

## Generating

One convention, no flags: packs in `assets/`, beside the cartridge's other
assets, output `levels_gen.go`. A
cartridge's `make build` runs, from the engine checkout,

```sh
go run ./tools/genlevelpack <cartridge dir>
```

before TinyGo (the template's and the tutorial's Makefiles call this step
`make levels`). Every `*.arkadelevels` in `assets/` becomes one
`var <name>Pack`, sorted by filename so the output is deterministic — the
generated file changes only when a pack file does. `genlevelpack -check
<dir>` writes nothing and exits 1 naming each stale file. `-dir`/`-out`/`-pkg`
remain only for a package that is not a cartridge, such as `sdk/levels/example`.
The packer leaves every pack out of the `.rcart`: it is compile-time input,
and nothing reads one at run time.

## Validation hooks: unsolvable fails the build, not the player

`genlevelpack` runs no per-game checks; it cannot know what "solvable" means
for your game. A cartridge that wants them writes its own three-line
generator in `tools/genlevels` instead of forking anything, whose `main` is
just:

```go
func main() {
    packfile.Main(packfile.Config{
        Hook: crateSolverPar, // func(*levels.Level) ([]levels.Entry, error)
    })
}
```

`genlevelpack` hands a cartridge with a `tools/genlevels` to it, so the
build, `make levels` and `go run ./tools/genlevels -check` all run the
same hooks. `packfile.Extra` functions passed to `Main` write sidecar
generated files (an id table, a derived attract script) from the validated
packs.

A `Hook` runs once per level, on the host, as part of the build — it may
use maps, recurse and allocate as freely as any other tool. It does two
jobs:

1. **Refuse.** A crate-pusher can BFS-solve every level and refuse to write
   its data file if one is unsolvable; a puzzle can replay each level's
   intended solution. Return an error and `Main` writes nothing at all —
   the cartridge's checked-in level data is never left half-valid.
2. **Derive.** Return `[]levels.Entry{{Key: "par", Int: n}}` and `par`
   becomes checked-in data no human maintains. A returned entry replaces a
   same-key entry from the pack file: what the checker computed is the truth
   about the level, so a stale hand-written value cannot survive it.

`packfile/hook_test.go` carries a complete worked one — a crate-pushing
solver, written against the decoded level.

A `PackHook` is the check a `Hook` structurally cannot make: `func(*levels.
Pack) error`, run once per pack after every level of it has passed. `exit`
is why it exists — the moment a level can name another one, the interesting
questions stop being per-level. A world-completability proof is exactly
that shape: a BFS of every room under an ability/quota unlock lattice,
checking that every room opens at the point the story needs it and that no
wing opens early. Refuse-only, because a pack has no metadata of its own to
derive into.

An `IndexedHook` is the same shape plus the level's ordinal position —
`func(i int, lv *levels.Level) ([]levels.Entry, error)` — for a check that
depends on WHERE a level sits rather than what it contains: a shooter's
shield ramp that names a wave from its stencil's position in the pack, or a
brick-breaker that refuses a brick type appearing before the level that
introduces it. Without the index, that number lives in a package-level
counter incremented by hand on every hook call; `Config.Hook`
and `Config.IndexedHook` are mutually exclusive, so a generator picks
whichever shape it needs and `Main` supplies the index itself.

## Canonicalizing a pack

```sh
go run ./tools/genlevelpack -fmt <cartridge dir>/assets/foo.arkadelevels
```

Run from the engine checkout, like the build step above.

Rewrites one pack file in place in the format's canonical shape —
single-space legend columns, a blank line before each level, comments kept
exactly where they were (`packfile.Format`) — the same shape
levellab's editor produces when a pack is opened and
saved untouched. Idempotent, and refuses an unparseable pack with the real
parser's own errors, writing nothing.

## Does this fit real levels?

`packfile/fidelity_test.go` checks the format against the two shapes it has
to fit, cell for cell against the same levels written as hard-coded Go: a
crate-pusher's solo levels (including a solver-derived `par`, re-derived by
the hook and compared) and a tray-packing puzzle's levels (names with
spaces, its own metadata vocabulary, and both divider layers). Packs are
editable in levellab, the hosted level editor at `arkade.build/dev/tools`.

## Testing

`sdk/levels` is its own Go module (like `sdk/cartlib` and `sdk/ui`), so the
root `go test ./...` does not descend into it:

```sh
go -C sdk/levels test ./...   # or: make sdk-levels-test
```
