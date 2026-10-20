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
