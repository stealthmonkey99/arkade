# Tutorial part 3: waves you draw by hand

[Part 1](tutorial.md) built the game and [part 2](tutorial-assets.md) gave
it art and a voice. Everything it does still comes out of the same place,
though: `cartlib.RandomU64` picks a column, a curve picks a speed, and
nobody ever *decided* anything. A run has no opening, no shape and no
moment that was designed.

This part gives it one. Four waves of fruit, authored in a grid editor,
checked by a generator that refuses the ones the cartridge cannot play, and
compiled in as ordinary data — with the endless random game kept exactly as
it is, waiting on the other side of them as the finale.

It is the platform's third asset pipeline, and it is the same shape as the two
in part 2:

```text
assets/*.arkadelevels  --[ tools/genlevels ]-->  levels_gen.go
  hand-authored                               checked in
  source of truth                             next to it
```

You commit both halves. The text pack is the only file you edit and what
a diff shows; the generated Go is build output, like `manifest_gen.go`,
and is what TinyGo compiles. Every `make build` regenerates it first and
leaves an up-to-date file alone, so there is no generator to run by hand
and preflight's `levels` check fails a `levels_gen.go` that is stale or
edited. `sdk/levels` is the runtime
half — `Pack`, `Level`, `Legend`, and their accessors — and it is built to
the cartridge's rules: no imports, and no allocation, not even at
load, because there is no load. The generated literal *is* the loaded form.

## What a wave is

The format ships no opinion about what a grid MEANS. A box-pushing
puzzle's level is a room; a nonogram is a clue grid. Catcher has no board at all, so its
grid has to encode the one thing it does have — a schedule:

- **A row is a moment.** Time runs down the page. Row 0 fires the instant
  the wave starts, row 1 one `cadence` later, and so on to the bottom.
- **A column is a drop lane**, spread evenly across the 64-pixel panel:
  lane 0 at the left edge, the last lane at the right. Twelve lanes is a
  comfortable number to draw and lands them about five pixels apart.
- **An `o` is a fruit**, falling down that lane, at that moment. Two on one
  row fall together.

Two numbers make a wave harder or easier, and they are metadata rather than
grid: `cadence` is sim steps between rows (a step is 25 ms) and `fall` is
the fall speed in sixteenths of a pixel per step. A wave gets faster by
dropping its rows closer together, by falling harder, or both.

That is a deliberately obvious encoding, and obvious is the requirement:
the whole point of putting levels in a grid editor is that the picture on
the screen is the thing you are designing. A row of `o`s reads as "these
fall together" at a glance, and a diagonal reads as a sweep.

## Draw one in levellab

A pack lives in the cartridge's `assets/`, beside the sprites and sounds
from part 2; the packer leaves it out of the `.rcart`, since the build
compiles it in. Open levellab at `arkade.build/dev/tools`; **Export** writes
packs back into `assets/`.

Press **New** under **Packs** on the left and call the file
`waves.arkadelevels`. It opens on a starter pack rather than a blank page: a
two-cell legend, one small level, and a `pack` name derived from the file
name — which matters, because that name becomes the generated Go variable.
`waves.arkadelevels` is what makes `wavesPack` exist.

The **Legend** on the right is the palette every grid in the pack paints
from, and the starter's two cells are for a game with walls. Edit them in
place. Catcher needs:

| Character | Name |
| --- | --- |
| `.` | `sky` |
| `o` | `fruit` |

The name is the cartridge's own word for that cell, and it rides into the
generated file so the data is self-describing. (Space is a legal cell
character — it is written `\s`, because the format's `|`-delimited rows
exist precisely so that an editor stripping trailing whitespace cannot
silently narrow a grid. A visible `.` is easier to draw against.)

Now the wave. **Rename** the starter level to `FIRST DROPS`, set **w** and
**h** to `12` in the toolbar and press **Resize**. Click `.` in the
**Palette** and fill the grid, then click `o` and place five fruit:

- Row 0, lane 5 — dead centre.
- Row 3, lane 3. Row 6, lane 7. Row 9, lane 1. Row 11, lane 10.

That first drop lands exactly where the catcher starts. It is the one
piece of level design in this whole pack that really matters: the first
thing a new player does is *nothing*, and a game should reward that once
before it starts asking for footwork.

Under **Metadata**, press **+ key** twice and add `cadence` `20` and
`fall` `14`. Values are typed by their spelling — `20` is an int, `true`
would be a bool, anything else is a string — and read back through the
matching accessor, so a key read at the wrong type reads as *missing*
rather than as zero.

Press **Text** at any point. That pane shows the exact bytes **Save**
would write, and it is worth knowing why it can be exact: the page holds
the pack's structure and never builds a line of text itself. The Go half
renders that structure and hands the result to the same `packfile.Parse`
the generator runs, on every change. Every error you see in the editor is
an error the build would have given you, at the same line, in the same
words — and a pack that will not parse cannot be saved at all.

**Save.** Then draw the other three the same way, or paste the finished
pack below into your cartridge's `assets/waves.arkadelevels` and reopen the
editor on it. Reading a pack and writing it straight back is
byte-identical, comments and all, which is what makes hand-editing and the
grid editor genuinely interchangeable rather than one of them being the
real one.

## The pack

Here is the whole file. Note what is *not* in it: `par`. The generator
derives that, and a hand-written one would be replaced by the solver's
anyway.

```levels file=assets/waves.arkadelevels
// Catcher's authored waves — the opening minute of a run, drawn by hand
// instead of drawn from the RNG. Edit them here or in levellab at
// arkade.build/dev/tools (import this file, export it back); `make build`
// compiles it into the Go the cartridge builds in.
//
// # How to read a grid
//
// A row is a MOMENT, not a place. Time runs down the page: row 0 fires the
// instant the wave starts, row 1 one `cadence` later, and so on. A column is
// a drop lane spread evenly across the 64-pixel panel — lane 0 at the left
// edge, lane 11 at the right — so an `o` says "a fruit falls here, now", and
// two `o`s on one row fall together.
//
// `cadence` is sim steps between rows (one step is 25 ms) and `fall` is the
// fall speed in sixteenths of a pixel per step, so a wave gets harder by
// dropping its rows closer together, by falling faster, or both.
//
// `par` is deliberately absent: tools/genlevels DERIVES it — the most
// fruit a player who only WALKS can bring in — and a hand-written value
// would be replaced by the solver's anyway. Catch par and the wave pays a
// clear bonus.
//
// The generator also refuses waves the cartridge cannot run: one holding a
// drop no walking player can reach (a miss costs one of three lives, so an
// unreachable fruit is a life on a timer), and one whose rows land so close
// together that more than six fruit are airborne at once, which is the size
// of the sim's fixed fruit array.

pack waves

legend
  .  sky
  o  fruit

// Wave 1 opens with a fruit straight over the catcher's starting spot: the
// first thing a new player does is nothing, and the game should reward it
// once before it starts asking for footwork.
level "FIRST DROPS"
  cadence 20
  fall    14
  |.....o......|
  |............|
  |............|
  |...o........|
  |............|
  |............|
  |.......o....|
  |............|
  |............|
  |.o..........|
  |............|
  |..........o.|

// Two on a row fall together, and the pair is always close enough that
// one basket covers both — which is the whole lesson: line the middle of
// the basket up, do not pick a side. The generator refuses a pair wider
// than that, because a fruit nobody can reach is a life on a timer.
level DOUBLES
  cadence 16
  fall    16
  |.....oo.....|
  |............|
  |............|
  |..oo........|
  |............|
  |............|
  |........oo..|
  |............|
  |............|
  |...oo.......|
  |............|
  |......oo....|

// Alternating edges at a cadence a walk can only just keep up with. Every
// drop is still reachable on foot — the solver insists — but this is where
// the dash starts paying for itself.
level "THE ZIGZAG"
  cadence 14
  fall    20
  |.o..........|
  |............|
  |..........o.|
  |............|
  |..o.........|
  |............|
  |.........o..|
  |............|
  |...o........|
  |............|
  |........o...|
  |............|

// The finale: one fruit every quarter second, sweeping out to the left
// edge and back across to the right. Nothing in it is out of reach and
// there is not one spare moment in it either.
level DOWNPOUR
  cadence 10
  fall    24
  |.....o......|
  |....o.......|
  |...o........|
  |..o.........|
  |.o..........|
  |o...........|
  |..o.........|
  |.....o......|
  |........o...|
  |..........o.|
  |...........o|
  |.........o..|
```

One more thing the file *could* carry and does not: **front matter**. A
pack may open with an optional asset envelope — a name, a description,
tags, and the tool that wrote it — fenced between two lines spelled
exactly `// --- arkade ---`, every line of it in the format's own
comment syntax. That is the whole trick: the pack parser drops comment
lines before it reads anything, so `packfile.Parse` (and with it
`genlevels`) never sees the block, and a pack with it and the same pack
without it decode to the identical `levels.Pack`. levellab's **Asset
info** panel writes it — a brand-new pack gets one from its first save,
an existing pack only once you fill the panel in — and Snake's
`assets/walls.arkadelevels` ships with one. Nothing in this tutorial needs it,
which is why `waves.arkadelevels` has none. See
[Where the envelope lives](reference-levels.md#where-the-envelope-lives-front-matter-not-the-file).

## The generator, and the two waves it refuses

`tools/genlevelpack` is the generic generator and would turn that file
into Go without another line of code. But it runs no per-game checks,
because it cannot know what "playable" means for your game. So Catcher
carries its own generator in `tools/genlevels`: a `main` that is just
`packfile.Main` with a hook, which `genlevelpack` runs in its place
whenever a cartridge has one.

For Catcher it means two things, and neither is visible in the picture:

**More than six fruit airborne at once.** `catcher.go` holds its fruit in a
fixed array of `maxFruit`, because `-gc=leaking` never gives memory back and
a growing slice would leak forever. A wave whose rows land close enough to
overflow it does not crash — it silently drops fruit on the floor, which
reads as a bug in the collision code and is a bug in the level. Whether it
overflows depends on the cadence and the fall speed *together*.

**A drop nobody can reach.** Every miss costs a life and there are three of
them for the whole run, so a wave holding a fruit no player can get to does
not read as hard; it reads as the game taking a life on a schedule. And
whether a drop is reachable depends on where the *previous* one left you,
which is a question about the whole wave rather than about any row of it.

So the generator simulates. `packfile.Main` runs a `Hook` once per
level, on the host, as part of the build, so it may allocate, recurse
and use maps as freely as any other tool — and it does two jobs. It
**refuses**: return an error and `Main` writes nothing at all, so the
checked-in level data is never left half-valid. And it **derives**: the
same walk-reachability solver that proves a wave clearable knows how many
fruit a walking player can bring in, so it returns that as the wave's
`par`, and par becomes checked-in data no human maintains.

Refusing and deriving are one computation seen from two sides here, which
is worth being honest about: par comes out equal to the drop count for
every wave that survives. What the cartridge gains is that the number it
pays a bonus on is the *solver's*, so it cannot go stale the next time
somebody drags a fruit two lanes over.

`tools/genlevels/main.go`, in the cartridge:

```go file=tools/genlevels/main.go step=6
// SPDX-License-Identifier: GPL-2.0-or-later

// Command genlevels is Catcher's level generator: packfile.Main with the
// wave hook below. `make build` runs it, so the pack under assets/ is all
// anyone edits.
//
// It reads every .arkadelevels file in ../assets, checks each wave against
// the rules the cartridge cannot bend, and writes levels_gen.go beside the
// cartridge's other sources — refusing to write anything at all if one wave
// is broken. `go run ./tools/genlevels -check` reports a stale levels_gen.go
// instead of rewriting it.
//
// This is plain HOST Go. It is never compiled into the .wasm, so it may
// allocate, recurse and use maps as freely as any other tool; the golden
// rules that govern the cartridge do not reach here.
//
// # What the hook refuses, and why the build is the right place
//
// A wave is a picture of a schedule, and two of the ways it can be wrong
// are invisible in that picture:
//
//   - MORE THAN SIX FRUIT AIRBORNE AT ONCE. catcher.go holds its fruit in a
//     fixed array of maxFruit, because -gc=leaking never gives memory back
//     and a growing slice would leak forever. A wave whose rows land close
//     enough to overflow it does not crash — it silently drops fruit on the
//     floor, which reads as a bug in the collision code and is a bug in the
//     level. Whether it overflows depends on the cadence AND the fall speed
//     together, which is not something you can see by looking at the grid.
//   - A DROP NOBODY CAN REACH. Every miss costs a life, and there are three
//     of them for the whole run, so a wave holding a fruit no player can get
//     to does not read as "hard" — it reads as the game taking a life on a
//     schedule. Whether a drop is reachable depends on where the PREVIOUS
//     one put you, which is a question about the whole wave rather than
//     about any row of it.
//
// So [wavesHook] simulates each wave. The capacity check replays the drop
// schedule with nothing caught (catches only ever FREE a slot, so that is
// the worst case) and fails on the first row that would overflow. The
// reachability check is a small dynamic program over (step, catcher
// position) that finds the most fruit a player who only WALKS can bring in,
// and the wave has to pass unless that is every one of them.
//
// The hook then DERIVES that same number as the wave's `par`, which is the
// shape a puzzle generator uses for the length of the BFS solve that
// proved a level solvable (see sdk/levels/packfile's Hook). Refusing and
// deriving are one computation seen from two sides: par comes out equal to
// the drop count for every wave that survives, and what the cartridge reads
// when it pays a clear bonus is the solver's number rather than a hand-typed
// one that could go stale the next time somebody moves a fruit.
//
// # Why this cannot just import the cartridge's own constants
//
// The wall every cartridge's tools/genlevels hits: this package is itself
// `package main`, and Go cannot import another `main` package. The panel
// geometry, the walk speed and the lane spacing below are therefore a
// second copy of catcher.go's, kept in sync by hand — small, stable, and
// pinned from the other side by the cartridge's own unit tests.
package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/stealthmonkey99/arkade/sdk/levels"
	"github.com/stealthmonkey99/arkade/sdk/levels/packfile"
)

// The cartridge's geometry and rules, duplicated for the reason the package
// doc gives. Every one of these is catcher.go's.
const (
	fbW    = 64
	fbH    = 64
	fruitW = 5
	fruitH = 5

	catcherW    = 7
	catcherY    = fbH - 12 // 52
	catcherMaxX = fbW - catcherW
	basketH     = 3

	maxFruit = 6
	walkStep = 2
	fallMax  = 40

	// startX is where reset() parks the catcher, and therefore where the
	// solver's player begins the wave.
	startX = (fbW - catcherW) / 2
)

// cellFruit is the legend character that means "a fruit falls here". Sky is
// anything else, so a pack may name its empty cell whatever reads best.
const cellFruit = 'o'

// maxLanes keeps every lane a distinct pixel column. Beyond this two
// columns of the grid would spawn fruit at the same x, which is a grid
// lying about what it draws.
const maxLanes = fbW - fruitW + 1

// laneX is the pixel column lane i of a w-lane grid drops down: the lanes
// spread evenly from the left edge to the right, endpoints included. This
// mirrors catcher.go's laneX exactly.
func laneX(i, w int) int {
	if w <= 1 {
		return (fbW - fruitW) / 2
	}
	return i * (fbW - fruitW) / (w - 1)
}

// drop is one authored fruit: the grid cell it came from, the pixel column
// it falls down, and the sim step it appears on.
type drop struct {
	row, lane int
	x         int
	spawn     int
}

// schedule flattens a wave's grid into its drops, in fire order.
func schedule(lv *levels.Level, cadence int) []drop {
	var out []drop
	for y := 0; y < lv.H; y++ {
		row := lv.Row(y)
		for x := 0; x < len(row); x++ {
			if row[x] == cellFruit {
				out = append(out, drop{row: y, lane: x, x: laneX(x, lv.W), spawn: y * cadence})
			}
		}
	}
	return out
}

// fruitY is a drop's top edge, in whole pixels, at sim step t. It mirrors
// catcher.go's integer fall exactly: a fruit spawns with y16 = 0 and the
// same step's fall phase already advances it once.
func fruitY(d drop, t, fall int) int {
	if t < d.spawn {
		return -1
	}
	return (t - d.spawn + 1) * fall / 16
}

// The two halves of the sim's collision, split so the solver can ask them
// separately. inBand is "this fruit is at a height where the basket could
// take it"; inReach is "the basket is at an x where it would".
func inBand(y int) bool { return y >= catcherY-fruitH+1 && y <= catcherY+basketH-1 }
func gone(y int) bool   { return y >= fbH-fruitH }
func inReach(bx, fx int) bool {
	return fx < bx+catcherW && bx < fx+fruitW
}

// lastStep is one step past everything the wave can still be doing.
func lastStep(drops []drop, fall int) int {
	last := 0
	for _, d := range drops {
		if d.spawn > last {
			last = d.spawn
		}
	}
	return last + (fbH-fruitH)*16/fall + 2
}

// checkCapacity replays the schedule with nothing ever caught — the worst
// case, since a catch only frees a slot early — and reports the first row
// that would need a seventh fruit. It also returns the peak, which is the
// number worth printing even when a wave passes.
func checkCapacity(lv *levels.Level, drops []drop, cadence, fall int) (peak int, err error) {
	live := make([]int, 0, 2*maxFruit) // spawn steps of the airborne fruit
	next := 0
	for t := 0; t <= lastStep(drops, fall); t++ {
		// Spawn: every drop scheduled for this step, in grid order.
		for next < len(drops) && drops[next].spawn == t {
			if len(live) >= maxFruit {
				return 0, fmt.Errorf("row %d needs fruit slot %d, and the cartridge's array holds %d — "+
					"widen the cadence (%d) or slow the fall (%d)",
					drops[next].row, len(live)+1, maxFruit, cadence, fall)
			}
			live = append(live, t)
			next++
		}
		if len(live) > peak {
			peak = len(live)
		}
		// Fall, and retire whatever reached the ground.
		kept := live[:0]
		for _, spawn := range live {
			if !gone((t - spawn + 1) * fall / 16) {
				kept = append(kept, spawn)
			}
		}
		live = kept
	}
	return peak, nil
}

// state is one node of the reachability DP: where the catcher is, and which
// of the fruit CURRENTLY in the catch band it has already taken. The mask
// is what keeps the count honest — a basket can leave a fruit's reach and
// come back within two steps, and without it that fruit would be counted
// twice. Bits are cleared the moment a fruit leaves the band, so the state
// space collapses back to "position" between drops instead of growing with
// the length of the wave.
type state struct {
	x    int
	mask uint32
}

// par is the most fruit a player who only WALKS can catch. The dash can
// only ever beat it, which is why the cartridge pays its clear bonus on
// `caught >= par` rather than on equality.
//
// The DP ignores lives: it answers "how much of this wave is reachable",
// not "can one run survive it".
func par(drops []drop, fall int) int {
	if len(drops) > 32 {
		// The mask is a uint32. No hand-authored wave is close, and a
		// silent wrong answer would be worse than this.
		return -1
	}
	cur := map[state]int{{x: startX}: 0}
	best := 0
	for t := 0; t <= lastStep(drops, fall); t++ {
		next := make(map[state]int, len(cur)*3)
		for st, caught := range cur {
			for _, dir := range [3]int{-1, 0, 1} {
				nx := st.x + dir*walkStep
				if nx < 0 {
					nx = 0
				}
				if nx > catcherMaxX {
					nx = catcherMaxX
				}
				ns := state{x: nx, mask: st.mask}
				got := caught
				for i, d := range drops {
					bit := uint32(1) << uint(i)
					if ns.mask&bit != 0 || !inBand(fruitY(d, t, fall)) || !inReach(nx, d.x) {
						continue
					}
					ns.mask |= bit
					got++
				}
				if prev, seen := next[ns]; !seen || got > prev {
					next[ns] = got
				}
			}
		}
		// Retire the fruit that have left the band: their bits can never
		// mean anything again, and clearing them merges states that are
		// now genuinely the same.
		var live uint32
		for i, d := range drops {
			if y := fruitY(d, t, fall); inBand(y) {
				live |= uint32(1) << uint(i)
			}
		}
		cur = make(map[state]int, len(next))
		for st, got := range next {
			st.mask &= live
			if prev, seen := cur[st]; !seen || got > prev {
				cur[st] = got
			}
			if got > best {
				best = got
			}
		}
	}
	return best
}

// wavesHook is the packfile.Hook: refuse what the cartridge cannot run or
// the player cannot reach, and derive `par` for what survives.
func wavesHook(lv *levels.Level) ([]levels.Entry, error) {
	if lv.W < 2 || lv.W > maxLanes {
		return nil, fmt.Errorf("%d lane(s) wide; a wave needs 2 to %d so every lane is its own pixel column", lv.W, maxLanes)
	}
	cadence, ok := lv.Int("cadence")
	if !ok || cadence < 1 {
		return nil, errors.New("no `cadence` (sim steps between rows, 1 or more)")
	}
	fall, ok := lv.Int("fall")
	if !ok || fall < 1 || fall > fallMax {
		return nil, fmt.Errorf("no usable `fall` (sixteenths of a pixel per step, 1 to %d)", fallMax)
	}
	// A fruit that moves further in one step than the catch band is tall
	// would jump straight through the basket. fallMax is already inside
	// this, but a pack file is where the number is easy to get wrong.
	if band := fruitH + basketH - 1; fall/16 >= band {
		return nil, fmt.Errorf("fall %d moves a fruit %dpx a step through a %dpx catch band — it would tunnel",
			fall, fall/16, band)
	}

	drops := schedule(lv, cadence)
	if len(drops) == 0 {
		return nil, fmt.Errorf("no %q cells — a wave with nothing to catch is not a wave", cellFruit)
	}
	peak, err := checkCapacity(lv, drops, cadence, fall)
	if err != nil {
		return nil, err
	}
	n := par(drops, fall)
	if n < 0 {
		return nil, fmt.Errorf("%d drops; the reachability solver handles 32", len(drops))
	}
	if n < len(drops) {
		return nil, fmt.Errorf("%d of %d drops cannot be reached by walking — every miss costs one of three "+
			"lives, so a wave has to be clearable; move them closer together or widen the cadence (%d)",
			len(drops)-n, len(drops), cadence)
	}

	fmt.Printf("%-14s %2d drop(s), %d airborne at the peak, par=%d\n", lv.Name, len(drops), peak, n)
	return []levels.Entry{{Key: "par", Int: n}}, nil
}

func main() {
	_, thisFile, _, _ := runtime.Caller(0)
	cartDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	packfile.Main(packfile.Config{
		Dir:  filepath.Join(cartDir, "assets"),
		Out:  filepath.Join(cartDir, "levels_gen.go"),
		Pkg:  "main",
		Hook: wavesHook,
	})
}
```

Nothing needs wiring into the `Makefile`: the template's `build` target
already runs `genlevelpack` before TinyGo, finds `tools/genlevels`, and
hands the cartridge to it. The generator solves every wave first and writes
nothing if one of them is unplayable, so a broken wave fails the build
rather than a cabinet.

...and require `sdk/levels` in `go.mod`; its `replace` is already there
from part 2. One require covers both halves of the pipeline — the
cartridge compiles the runtime half, the generator runs the host-side
`packfile` subpackage of the same module:

```go-mod fragment
require github.com/stealthmonkey99/arkade/sdk/levels v0.1.0
```

Now build:

```sh
make build
```

The generator reports each wave it accepted, ahead of the compile:

```text
FIRST DROPS     5 drop(s), 2 airborne at the peak, par=5
DOUBLES        10 drop(s), 4 airborne at the peak, par=10
THE ZIGZAG      6 drop(s), 2 airborne at the peak, par=6
DOWNPOUR       12 drop(s), 4 airborne at the peak, par=12
```

### Break one on purpose

This is the part worth doing rather than reading. Open `DOWNPOUR` and pull
its `cadence` from `10` down to `4`:

```text
FIRST DROPS     5 drop(s), 2 airborne at the peak, par=5
DOUBLES        10 drop(s), 4 airborne at the peak, par=10
THE ZIGZAG      6 drop(s), 2 airborne at the peak, par=6
genlevels: waves.arkadelevels: level "DOWNPOUR": row 6 needs fruit slot 7, and the cartridge's array holds 6 — widen the cadence (4) or slow the fall (24)
```

Now put `DOUBLES`' first pair on opposite edges — `|o..........o|` — and
watch the other check fire:

```text
FIRST DROPS     5 drop(s), 2 airborne at the peak, par=5
THE ZIGZAG      6 drop(s), 2 airborne at the peak, par=6
DOWNPOUR       12 drop(s), 4 airborne at the peak, par=12
genlevels: waves.arkadelevels: level "DOUBLES": 1 of 10 drops cannot be reached by walking — every miss costs one of three lives, so a wave has to be clearable; move them closer together or widen the cadence (16)
```

Three things in those two transcripts are the whole argument for a hook:

1. **The build failed, not the player.** `levels_gen.go` on disk is
   untouched — you are still holding the last good one.
2. **The waves that passed still reported.** the generator collects every
   file's and every level's errors rather than stopping at the first, so
   one bad wave does not hide a second one behind it.
3. **The message is about the game**, not about the format. "row 6 needs
   fruit slot 7" is a sentence the person who drew row 6 can act on.

Put both waves back the way they were and rebuild. Then commit the pack and
the generated Go together, and do not hand-edit `levels_gen.go` — edit the
grid and run `make build`, which is a zero-line diff when nothing changed.

## Step 6 — waves in the simulation

Three files change, and `catcher.go` is most of it. The shape of the
change is small on purpose: **the wave index is the mode.** While
`s.wave` addresses a level in the pack, spawning comes from the schedule
and the fall speed comes from the wave; the moment it walks off the end of
the pack, both come from where they always did. There is no second flag to
disagree with it, and no state machine — a run crosses that line exactly
once, on the step the last fruit of the last wave resolves.

```go file=catcher.go step=6
// SPDX-License-Identifier: GPL-2.0-or-later

package main

// The simulation. It reaches for no host call, so plain `go test` covers
// every rule in it; sdk/levels, its one import, is pure data. step takes
// its randomness as a parameter: the cartridge passes cartlib.RandomU64,
// a test passes a counter.

import "github.com/stealthmonkey99/arkade/sdk/levels"

const (
	// The panel. cartlib.FBWidth/FBHeight say the same thing, but this
	// file does not import cartlib.
	fbW = 64
	fbH = 64

	catcherW    = 7        // one cel of the bundle's sprite sheet
	catcherH    = 9        //
	catcherY    = fbH - 12 // 52: the catcher's top edge, feet at 60
	basketH     = 3        // the open basket is the sprite's top 3 rows
	catcherMaxX = fbW - catcherW

	fruitW = 5 // assets/sprites/fruit.png
	fruitH = 5

	// a fixed array rather than a growing slice: -gc=leaking never
	// collects (GOLDEN RULE 7)
	maxFruit   = 6
	startLives = 3

	// One sim step is 25ms (40 Hz), independent of the host's ~30 fps
	// frame rate. Update accumulates dtMicros and runs whole steps, so
	// the game plays identically on a host that stutters — and, more
	// importantly, reproducibly, which is what the golden tests pin.
	stepMicros = 25_000

	walkStep = 2 // pixels per step walking
	dashStep = 5 // pixels per step dashing
	// A dash is short and its cooldown is long: it is the answer to one
	// fruit you cannot walk to, not a faster way to move.
	dashSteps         = 8  // 200ms
	dashCooldownSteps = 56 // 1.4s after it ends

	// Difficulty in the ENDLESS finale is a function of fruit caught, so
	// it ramps with skill rather than with elapsed time. fallRate is in
	// sixteenths of a pixel per step: 14 is ~35 px/s (a little under two
	// seconds to fall), 40 is 100 px/s. During an authored wave both
	// numbers come from the wave instead.
	fallBase  = 14
	fallMax   = 40
	spawnBase = 44 // steps between spawns at the start (1.1s)
	spawnMin  = 16 // ...and at full tilt (0.4s)

	catchScore = 10

	// waveBonus is what clearing a wave at its par is worth — par being
	// the most fruit tools/genlevels could prove a walking player able
	// to bring in. It is deliberately more than one catch and less than
	// a wave: a reason to chase the hard drop, not a reason to ignore
	// the score.
	waveBonus = 25

	// waveNameSteps holds the wave's name across the panel for the first
	// second and a half of it. It is the only place the name a human
	// typed in the level editor reaches the player.
	waveNameSteps = 60

	// cellFruit is the pack's "a fruit falls here" character. Every
	// other legend character is sky.
	cellFruit = 'o'
)

// fruit is one falling piece. y16 is the top edge in sixteenths of a
// pixel: integer arithmetic, because two hosts must agree on the answer
// bit for bit and floats invite a divergence the golden tests would
// only find later (see the ABI reference's determinism rules).
type fruit struct {
	live bool
	x    int // left edge, 0..fbW-fruitW
	y16  int // top edge, in 1/16 px
}

// y is the fruit's top edge in whole pixels.
func (f fruit) y() int { return f.y16 / 16 }

// sim is the whole game state. Everything the player can see is derived
// from these fields, and nothing else — no clock, no randomness held
// back, no pointers into anywhere.
type sim struct {
	x        int // the catcher's left edge
	dashLeft int // steps of dash remaining
	dashCool int // steps until the next dash is allowed
	fruit    [maxFruit]fruit
	spawnIn  int // steps until the next spawn, in the endless finale
	caught   int
	lives    int
	score    int

	// Where the run is in the authored waves. wave indexes
	// wavesPack.Levels and walks off the end exactly once, which is what
	// hands the run over to the endless finale for good.
	wave     int
	waveStep int // steps since this wave began
	waveRow  int // the next grid row to fire
	waveHits int // fruit caught in this wave, measured against its par
}

// reset puts a fresh run on the board. Assigning a whole struct literal
// (rather than zeroing fields one at a time) means a field added later
// cannot be forgotten here.
func (s *sim) reset() {
	*s = sim{
		x:       (fbW - catcherW) / 2,
		lives:   startLives,
		spawnIn: spawnBase,
	}
}

func (s *sim) over() bool { return s.lives <= 0 }

// waveLevel returns the wave being played, or nil once the pack has run out
// and the endless finale has taken over. It is the one test for "which
// mode is this run in", so the two modes can never disagree about it.
func (s *sim) waveLevel() *levels.Level {
	if s.wave >= len(wavesPack.Levels) {
		return nil
	}
	return &wavesPack.Levels[s.wave]
}

// waveMeta reads one of a wave's authored numbers, with a floor for the
// case tools/genlevels already refuses (a missing or nonsense value):
// levels_gen.go is generated, but it is also ordinary checked-in Go that
// somebody could hand-edit, and a zero cadence would fire every row of
// the wave on the same step.
func waveMeta(lv *levels.Level, key string, min int) int {
	if v, ok := lv.Int(key); ok && v >= min {
		return v
	}
	return min
}

// laneX is the pixel column lane i of a w-lane grid drops down: the lanes
// spread evenly from the left edge to the right, endpoints included, so a
// twelve-column grid reaches both edges of a 64-pixel panel. Integer
// arithmetic, like everything else here — tools/genlevels carries the
// same three lines and the two must agree exactly.
func laneX(i, w int) int {
	if w <= 1 {
		return (fbW - fruitW) / 2
	}
	return i * (fbW - fruitW) / (w - 1)
}

// waveTitle is the name to draw across the panel right now: the current
// wave's, for its first waveNameSteps, and "" the rest of the time.
func (s *sim) waveTitle() string {
	lv := s.waveLevel()
	if lv == nil || s.waveStep >= waveNameSteps {
		return ""
	}
	return lv.Name
}

// waveSpawn fires the wave's next grid row when its clock reaches it. A
// row is a MOMENT, not a place: every cellFruit on it drops at the same
// step, each down its own lane.
func (s *sim) waveSpawn(lv *levels.Level) {
	if s.waveRow < lv.H && s.waveStep >= s.waveRow*waveMeta(lv, "cadence", 1) {
		row := lv.Row(s.waveRow)
		for i := 0; i < len(row); i++ {
			if row[i] == cellFruit {
				s.spawn(laneX(i, lv.W))
			}
		}
		s.waveRow++
	}
	s.waveStep++
}

// finishWave hands the run on once the last row has fired and the last
// fruit of it has resolved. Waves therefore never overlap, which is why
// nothing here has to think about a fruit outliving the wave that
// authored it.
func (s *sim) finishWave(lv *levels.Level) {
	if s.waveRow < lv.H || s.anyLive() {
		return
	}
	// par is the generator's, derived by solving the wave rather than
	// typed by hand. A dash can beat it, hence >= rather than ==.
	if s.waveHits >= waveMeta(lv, "par", 1) {
		s.score += waveBonus
	}
	s.wave++
	s.waveStep, s.waveRow, s.waveHits = 0, 0, 0
}

func (s *sim) anyLive() bool {
	for i := range s.fruit {
		if s.fruit[i].live {
			return true
		}
	}
	return false
}

// spawn puts a fruit at the top of column x, or drops it on the floor if
// every slot in the fixed array is taken. The endless mode cannot
// overflow (it spawns one at a time, slower than they fall) and an
// authored wave that could is refused by tools/genlevels, so the silent
// branch here is the one nothing is supposed to reach.
func (s *sim) spawn(x int) {
	for i := range s.fruit {
		if !s.fruit[i].live {
			s.fruit[i] = fruit{live: true, x: x}
			return
		}
	}
}

// fallRate is the current fall speed in 1/16 px per step: the wave's own
// number while one is playing, and the skill ramp afterwards.
func (s *sim) fallRate() int {
	if lv := s.waveLevel(); lv != nil {
		return waveMeta(lv, "fall", 1)
	}
	r := fallBase + s.caught/2
	if r > fallMax {
		r = fallMax
	}
	return r
}

// spawnEvery is the current gap between spawns, in steps.
func (s *sim) spawnEvery() int {
	n := spawnBase - s.caught
	if n < spawnMin {
		n = spawnMin
	}
	return n
}

// dashReady reports whether pressing the blue button would do something
// right now. peripherals.go lights the button on exactly this answer —
// the eligibility-lighting rule (see Authoring patterns).
func (s *sim) dashReady() bool { return s.dashLeft == 0 && s.dashCool == 0 }

// startDash begins a dash if one is available, reporting whether it did
// so the caller can play a sound (or ring the reject bell) accordingly.
func (s *sim) startDash() bool {
	if !s.dashReady() {
		return false
	}
	s.dashLeft = dashSteps
	return true
}

// stepResult is what one step made happen. The sim returns events rather
// than playing sounds itself: sound is a host call, and keeping host
// calls out of this file is what keeps it testable.
type stepResult struct {
	caught int
	missed int
}

// step advances the simulation by exactly one stepMicros tick. dir is
// -1, 0 or +1 (the held direction, resolved by the caller); rand is the
// randomness source, called exactly once per RANDOM spawn and never
// otherwise — an authored wave draws no numbers at all.
func (s *sim) step(dir int, rand func() uint64) stepResult {
	var out stepResult

	// 1. Move. A dash overrides the walk speed and, when it ends, starts
	// its own cooldown; the cooldown only ticks down while not dashing.
	move := walkStep
	if s.dashLeft > 0 {
		move = dashStep
		s.dashLeft--
		if s.dashLeft == 0 {
			s.dashCool = dashCooldownSteps
		}
	} else if s.dashCool > 0 {
		s.dashCool--
	}
	s.x += dir * move
	if s.x < 0 {
		s.x = 0
	}
	if s.x > catcherMaxX {
		s.x = catcherMaxX
	}

	// 2. Spawn — from the authored schedule while a wave is playing, and
	// from the RNG once they run out. The single RandomU64 draw happens
	// on that second path and nowhere else, which is what lets a golden
	// test script the endless finale's fruit by listing a few numbers.
	wave := s.waveLevel()
	if wave != nil {
		s.waveSpawn(wave)
	} else {
		if s.spawnIn > 0 {
			s.spawnIn--
		}
		if s.spawnIn == 0 {
			s.spawn(int(rand() % uint64(fbW-fruitW+1)))
			s.spawnIn = s.spawnEvery()
		}
	}

	// 3. Fall, and resolve each fruit against the basket and the ground.
	rate := s.fallRate()
	for i := range s.fruit {
		f := &s.fruit[i]
		if !f.live {
			continue
		}
		f.y16 += rate
		if overlaps(f.x, f.y(), fruitW, fruitH, s.x, catcherY, catcherW, basketH) {
			f.live = false
			s.score += catchScore
			s.caught++
			s.waveHits++
			out.caught++
			continue
		}
		if f.y() >= fbH-fruitH {
			f.live = false
			s.lives--
			out.missed++
		}
	}

	// 4. Hand the run on if that emptied the last row of the wave.
	if wave != nil {
		s.finishWave(wave)
	}
	return out
}

// overlaps is axis-aligned rectangle intersection.
//
// Catcher collides rectangles because its catching surface IS a
// rectangle — the open mouth of the basket, the sprite's top three rows,
// the same shape in every walk cel. Art with an irregular silhouette
// wants pixel-perfect mask collision instead: Sheet.Cel has already
// built a ui.Mask for every sprite it lifts, and ui.Mask.Overlaps
// compares two of them directly. See the ui reference's "Sprites and
// masks" and Authoring patterns' "Sprite art as PNGs".
func overlaps(ax, ay, aw, ah, bx, by, bw, bh int) bool {
	return ax < bx+bw && bx < ax+aw && ay < by+bh && by < ay+ah
}
```

The one import is worth a pause, because part 1 made a point of this file
importing *nothing*. `sdk/levels` does not spoil that: its runtime half has
no imports of its own and reaches for no host call, so it is exactly as
testable as the rest of the file. `catcher_test.go` reads the real shipped
pack — `wavesPack.Levels[0]`, not a fixture standing in for it — and still
runs in a millisecond without TinyGo, wasm or a fake host.

Two smaller decisions in there are worth naming:

- **`waveMeta` has a floor.** `levels_gen.go` is generated, and the
  generator has already refused a missing `cadence` — but it is also
  ordinary checked-in Go that somebody could hand-edit, and a zero cadence
  would fire every row of the wave on the same step. A generated file is
  not a trusted one.
- **The bonus is `>=` par, not `== par`.** The solver only proves what a
  *walking* player can reach. A dash can beat it, and a game that punished
  you for playing well would be a strange thing to ship.

`render.go` gains eight lines: the wave's name, held across the panel for
its first second and a half.

```go file=render.go step=6
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

	g.drawWaveTitle(fb)

	for i := range g.sim.fruit {
		f := g.sim.fruit[i]
		if f.live {
			spriteFruit.Draw(fb, f.x, f.y())
		}
	}
	playerWalk.cel().Draw(fb, g.sim.x, catcherY)
	g.drawHUD(fb)
}

// drawWaveTitle flashes the wave's name across the panel as it starts.
// That name is the level's own — the token a human typed on the `level`
// line of assets/waves.arkadelevels, or into the level editor's rename box — so
// the authored data reaches the player as words and not only as falling
// fruit. It draws BEFORE the fruit, not after: a wave always opens with
// something falling through this band, and a fruit that passes in front of
// the lettering reads as a backdrop while one that the lettering punches
// through reads as a glitch.
func (g *catcherGame) drawWaveTitle(fb *cartlib.Framebuffer) {
	name := g.sim.waveTitle()
	if name == "" {
		return
	}
	small := ui.Font3x5{}
	w := small.TextWidth(len(name), 1)
	small.DrawText(fb, (cartlib.FBWidth-w)/2, 14, name, 1, colAccent)
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

That name is the level's own — the token you typed on the `level` line, or
into levellab's rename box — and it is the only place authored data reaches
the player as *words*. It draws before the fruit rather than after, which
is not a detail: a wave always opens with something falling through that
band, and a fruit passing in front of the lettering reads as a backdrop
while one the lettering punches through reads as a glitch.

Last, `main.go`, whose header now names `levels_gen.go`. Waves are a
player-visible feature, which is a **minor** version bump — in your copy,
`0.1.0` to `0.2.0` — and the version lives in two places that move
together: `manifest.json` and the `*Manual for version <v>*` footer in
`MANUAL.md` (the compiled-in copy is regenerated from the first at the
next `make build`). That footer is the mechanism, not paperwork: it is
what forces a manual pass in the same commit as the gameplay change, and
the finished cartridge's manual gained a section on the waves and the
clear bonus at the same time.

```go file=main.go step=6
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
//   - catcher.go: the simulation. Reaches for no host call, so
//     catcher_test.go covers the rules under plain `go test`.
//   - game.go: the cartlib.Game implementation — the sdk/fsm screen
//     machine and the transitions between its five screens.
//   - input.go: input decode, dispatch, and the hold-B confirm.
//   - render.go: every screen, drawn through sdk/ui onto the framebuffer.
//   - peripherals.go: sound, the per-seat LCD, and the button lights.
//   - attract.go: the cabinet's screensaver slot.
//   - sprites.go: the bundle's sprite sheet, lifted into ui.Sprites at Init.
//   - levels_gen.go: generated by tools/genlevels from assets/*.arkadelevels.
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

## Play it

```sh
make build      # in ../my-cartridges/catcher
```

Then run the tests, and expect your step-2 `catcher_test.go` to fail: it
spawns fruit from a fixed random column, and every run now opens with
four authored waves that draw no random numbers at all. The rules are
finished, so take the finished suite — copy
`../../arkade/sdk/tutorial/catcher_test.go` over yours. It starts each
rule test past the last wave (`newEndlessSim`) and adds tests for the
waves themselves: every authored drop spawns, every lane lands on the
panel, a clean wave pays its bonus.

```sh
go test ./...
```

Drag `catcher.wasm` onto the player at [arkade.build](https://arkade.build),
or run `./arkade --cartridges ../my-cartridges/catcher` from the engine
checkout ([part 4](tutorial-debug.md#iterate-from-the-terminal) has the
loop).

Four authored waves, then the game you already had. Watch for three
things:

- **The opening reads as designed**, because it is. The same first fruit,
  in the same lane, every single run — a wave draws no random numbers at
  all, so its determinism is structural rather than seeded.
- **The bonus fires** on a clean wave: the score jumps twenty-five past
  where the catches put it, which is the generator's `par` reaching the
  scoreboard.
- **The handover is invisible.** When `DOWNPOUR` empties, fruit simply
  keep coming — from `cartlib.RandomU64` now, at the difficulty your catch
  count has already earned. The high-score table and the attract screen
  never knew about waves in the first place, so nothing about the framing
  of a run changed.

## Where you are

Catcher now uses every pipeline the SDK has: sprites from PNGs, sound from
synthesis, levels from grids. All three commit their source *and* their
generated output, and all three fail the build rather than the player. The
levels, like the manifest, are regenerated by every `make build`; the sprite
sheet and sounds stay a deliberate `make assets`.

**Next: [play, record, replay, ship](tutorial-debug.md)** — the dev
server, the flight recorder, the golden tests that pin what all of this
looks like, and preflight.
