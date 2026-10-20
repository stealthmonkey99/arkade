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
