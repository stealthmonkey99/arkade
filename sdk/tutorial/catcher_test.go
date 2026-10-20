// SPDX-License-Identifier: GPL-2.0-or-later

package main

// The sim reaches for no host call, so these tests need no wasm or fake host.

import (
	"encoding/json"
	"os"
	"testing"
)

// fixedRand stands in for cartlib.RandomU64.
func fixedRand(v uint64) func() uint64 { return func() uint64 { return v } }

// countingRand counts the sim's random draws, the budget a golden test's
// RandScript relies on.
type countingRand struct {
	n     int
	value uint64
}

func (c *countingRand) next() uint64 { c.n++; return c.value }

func runSteps(s *sim, n, dir int, rand func() uint64) stepResult {
	var total stepResult
	for i := 0; i < n; i++ {
		r := s.step(dir, rand)
		total.caught += r.caught
		total.missed += r.missed
	}
	return total
}

func newSim() *sim {
	var s sim
	s.reset()
	return &s
}

// newEndlessSim is a fresh run past the last authored wave, where the RNG
// spawner lives; the wave index is the mode.
func newEndlessSim() *sim {
	s := newSim()
	s.wave = len(wavesPack.Levels)
	return s
}

func TestResetStartsCentredWithFullLives(t *testing.T) {
	s := newSim()
	if want := (fbW - catcherW) / 2; s.x != want {
		t.Errorf("start x = %d, want %d (centred)", s.x, want)
	}
	if s.lives != startLives {
		t.Errorf("start lives = %d, want %d", s.lives, startLives)
	}
	if s.score != 0 || s.caught != 0 {
		t.Errorf("start score/caught = %d/%d, want 0/0", s.score, s.caught)
	}
	if s.over() {
		t.Error("a fresh run reports itself over")
	}
}

// TestFruitInTheCatchersColumnIsCaught: a fruit that falls where the catcher
// stands scores and costs nothing.
func TestFruitInTheCatchersColumnIsCaught(t *testing.T) {
	s := newEndlessSim()
	// the catcher does not move (dir 0), so this tests only collision
	res := runSteps(s, 400, 0, fixedRand(uint64(s.x)))
	if res.caught == 0 {
		t.Fatal("400 steps of fruit falling onto the catcher caught nothing")
	}
	if res.missed != 0 {
		t.Errorf("%d fruit were missed while falling straight into the basket", res.missed)
	}
	if s.lives != startLives {
		t.Errorf("lives = %d after a clean run, want %d", s.lives, startLives)
	}
	if s.score != res.caught*catchScore {
		t.Errorf("score = %d for %d catches, want %d", s.score, res.caught, res.caught*catchScore)
	}
}

// TestFruitOutOfReachIsMissedAndCostsALife is the rule that ends a run.
func TestFruitOutOfReachIsMissedAndCostsALife(t *testing.T) {
	s := newEndlessSim()
	// column 0 is the far left and the catcher starts centred
	res := runSteps(s, 400, 0, fixedRand(0))
	if res.caught != 0 {
		t.Errorf("caught %d fruit that fell out of reach", res.caught)
	}
	if res.missed == 0 {
		t.Fatal("400 steps of fruit falling out of reach missed nothing")
	}
	if s.score != 0 {
		t.Errorf("score = %d after catching nothing, want 0", s.score)
	}
	if s.lives != startLives-res.missed && s.lives > 0 {
		t.Errorf("lives = %d after %d misses, want %d", s.lives, res.missed, startLives-res.missed)
	}
	if !s.over() {
		t.Errorf("lives = %d after 400 steps of misses — the run should be over", s.lives)
	}
}

func TestSpawnDrawsExactlyOneRandomPerFruit(t *testing.T) {
	s := newEndlessSim()
	c := &countingRand{value: 0}
	var spawned int
	for i := 0; i < 300; i++ {
		before := c.n
		s.step(0, c.next)
		if c.n > before {
			spawned++
		}
		if c.n-before > 1 {
			t.Fatalf("step %d drew %d random numbers; the sim must draw exactly one per spawn "+
				"or a golden test's RandScript cannot describe a run", i, c.n-before)
		}
	}
	if spawned == 0 {
		t.Fatal("nothing spawned in 300 steps")
	}
	if c.n != spawned {
		t.Errorf("%d draws for %d spawns", c.n, spawned)
	}
}

// TestSpawnColumnAlwaysFitsOnThePanel guards the modulus: a fruit spawned at
// x = 60 would hang off the right edge.
func TestSpawnColumnAlwaysFitsOnThePanel(t *testing.T) {
	for _, v := range []uint64{0, 1, 59, 60, 63, 1 << 62, ^uint64(0)} {
		s := newEndlessSim()
		runSteps(s, spawnBase, 0, fixedRand(v))
		found := false
		for i := range s.fruit {
			f := s.fruit[i]
			if !f.live {
				continue
			}
			found = true
			if f.x < 0 || f.x > fbW-fruitW {
				t.Errorf("rand()=%d spawned a fruit at x=%d, outside 0..%d", v, f.x, fbW-fruitW)
			}
		}
		if !found {
			t.Errorf("rand()=%d: nothing spawned after %d steps", v, spawnBase)
		}
	}
}

func TestCatcherClampsToThePanelEdges(t *testing.T) {
	s := newSim()
	runSteps(s, 200, -1, fixedRand(0))
	if s.x != 0 {
		t.Errorf("x = %d after walking left forever, want 0", s.x)
	}
	s.reset()
	runSteps(s, 200, 1, fixedRand(0))
	if s.x != catcherMaxX {
		t.Errorf("x = %d after walking right forever, want %d", s.x, catcherMaxX)
	}
}

// TestDashIsFasterThenUnavailableThenBack pins the dash cycle, including the
// property peripherals.go lights the blue button on.
func TestDashIsFasterThenUnavailableThenBack(t *testing.T) {
	walk := newSim()
	runSteps(walk, dashSteps, 1, fixedRand(0))

	dash := newSim()
	if !dash.dashReady() {
		t.Fatal("a fresh run cannot dash")
	}
	if !dash.startDash() {
		t.Fatal("startDash refused on a fresh run")
	}
	runSteps(dash, dashSteps, 1, fixedRand(0))

	if dash.x <= walk.x {
		t.Errorf("dashing moved to x=%d, walking to x=%d — a dash must cover more ground", dash.x, walk.x)
	}
	if dash.dashReady() {
		t.Error("a second dash is available the instant the first one ends")
	}
	if dash.startDash() {
		t.Error("startDash succeeded during the cooldown")
	}

	runSteps(dash, dashCooldownSteps, 0, fixedRand(0))
	if !dash.dashReady() {
		t.Errorf("still not ready %d steps after the dash ended", dashCooldownSteps)
	}
}

// TestDifficultyRampsWithCatchesAndThenStops checks the curve and its
// ceiling: an unclamped ramp would move fruit past the basket in one step.
func TestDifficultyRampsWithCatchesAndThenStops(t *testing.T) {
	s := newEndlessSim()
	base, baseGap := s.fallRate(), s.spawnEvery()
	if base != fallBase || baseGap != spawnBase {
		t.Fatalf("fresh run: fallRate=%d spawnEvery=%d, want %d and %d", base, baseGap, fallBase, spawnBase)
	}

	s.caught = 10
	if s.fallRate() <= base {
		t.Errorf("fallRate did not rise after 10 catches (%d)", s.fallRate())
	}
	if s.spawnEvery() >= baseGap {
		t.Errorf("spawnEvery did not shrink after 10 catches (%d)", s.spawnEvery())
	}

	s.caught = 10_000
	if s.fallRate() != fallMax {
		t.Errorf("fallRate = %d at 10000 catches, want the %d ceiling", s.fallRate(), fallMax)
	}
	if s.spawnEvery() != spawnMin {
		t.Errorf("spawnEvery = %d at 10000 catches, want the %d floor", s.spawnEvery(), spawnMin)
	}
	// a fruit must not move further than the basket is deep in one step
	if fallMax/16 > basketH {
		t.Errorf("at full speed a fruit falls %dpx per step through a %dpx basket — it can tunnel", fallMax/16, basketH)
	}
}

// TestFruitCountIsBounded: the sim holds a fixed array, so a long run cannot
// grow it.
func TestFruitCountIsBounded(t *testing.T) {
	s := newEndlessSim()
	runSteps(s, 5_000, 0, fixedRand(0))
	live := 0
	for i := range s.fruit {
		if s.fruit[i].live {
			live++
		}
	}
	if live > maxFruit {
		t.Errorf("%d live fruit, want at most %d", live, maxFruit)
	}
}

// autopilot steers toward the lowest live fruit; good enough to walk a wave,
// since tools/genlevels proves every drop reachable on foot.
func autopilot(s *sim) int {
	target, lowest := -1, -1
	for i := range s.fruit {
		if f := s.fruit[i]; f.live && f.y16 > lowest {
			lowest, target = f.y16, f.x
		}
	}
	if target < 0 {
		return 0
	}
	switch want := target + fruitW/2 - catcherW/2; {
	case s.x < want:
		return 1
	case s.x > want:
		return -1
	}
	return 0
}

// TestWavesPlayInOrderThenHandOverToTheRNG: the pack plays front to back
// drawing no random numbers, and walking off its end starts the endless
// finale. Lives are raised because the subject is the mode change.
func TestWavesPlayInOrderThenHandOverToTheRNG(t *testing.T) {
	s := newSim()
	s.lives = 1_000
	c := &countingRand{}

	for want := 0; want < len(wavesPack.Levels); want++ {
		if s.wave != want {
			t.Fatalf("wave index = %d, want %d — the waves must play in pack order", s.wave, want)
		}
		for i := 0; s.wave == want; i++ {
			if i > 5_000 {
				t.Fatalf("wave %d (%q) never finished", want, wavesPack.Levels[want].Name)
			}
			s.step(0, c.next)
		}
	}
	if c.n != 0 {
		t.Errorf("the authored waves drew %d random numbers; a wave is a schedule, not a roll", c.n)
	}
	if s.waveLevel() != nil {
		t.Fatal("the pack ran out but the sim still thinks it is in a wave")
	}

	// the RNG spawner owns the game from here
	runSteps(s, spawnBase, 0, c.next)
	if c.n == 0 {
		t.Errorf("no random draw in the %d steps after the last wave — the endless finale never started", spawnBase)
	}
}

// TestClearingAWaveAtParPaysTheBonus checks where the generator's derived
// metadata reaches the score.
func TestClearingAWaveAtParPaysTheBonus(t *testing.T) {
	first := &wavesPack.Levels[0]
	par, ok := first.Int("par")
	if !ok {
		t.Fatalf("%q carries no derived par — run `make build`", first.Name)
	}

	s := newSim()
	for i := 0; s.wave == 0; i++ {
		if i > 5_000 {
			t.Fatalf("%q never finished", first.Name)
		}
		s.step(autopilot(s), fixedRand(0))
	}
	if s.caught != par {
		t.Fatalf("caught %d of a par-%d wave; the autopilot is meant to clear it", s.caught, par)
	}
	if want := par*catchScore + waveBonus; s.score != want {
		t.Errorf("score after clearing %q at par = %d, want %d (%d catches plus the %d bonus)",
			first.Name, s.score, want, par, waveBonus)
	}
	if s.lives != startLives {
		t.Errorf("lives = %d after a clean wave, want %d", s.lives, startLives)
	}
}

// TestEveryAuthoredDropActuallySpawns is the runtime half of the generator's
// capacity check: spawn() silently drops a fruit when every slot is taken,
// so the shipped pack must fit the fixed array.
func TestEveryAuthoredDropActuallySpawns(t *testing.T) {
	for i := range wavesPack.Levels {
		lv := &wavesPack.Levels[i]
		want := lv.Count(cellFruit)

		s := newSim()
		s.wave = i
		s.lives = 1_000
		var got stepResult
		for n := 0; s.wave == i; n++ {
			if n > 5_000 {
				t.Fatalf("%q never finished", lv.Name)
			}
			r := s.step(autopilot(s), fixedRand(0))
			got.caught += r.caught
			got.missed += r.missed
		}
		if got.caught+got.missed != want {
			t.Errorf("%q: %d of its %d drops resolved (%d caught, %d missed) — the rest never reached the array",
				lv.Name, got.caught+got.missed, want, got.caught, got.missed)
		}
	}
}

// TestWaveLanesLandOnThePanel: the leftmost lane sits on the left edge, the
// rightmost is the last column a whole fruit fits in, and nothing escapes
// either.
func TestWaveLanesLandOnThePanel(t *testing.T) {
	for i := range wavesPack.Levels {
		lv := &wavesPack.Levels[i]
		for lane := 0; lane < lv.W; lane++ {
			if x := laneX(lane, lv.W); x < 0 || x > fbW-fruitW {
				t.Errorf("%q lane %d of %d drops at x=%d, outside 0..%d", lv.Name, lane, lv.W, x, fbW-fruitW)
			}
		}
		if x := laneX(0, lv.W); x != 0 {
			t.Errorf("%q lane 0 drops at x=%d, want the left edge", lv.Name, x)
		}
		if x := laneX(lv.W-1, lv.W); x != fbW-fruitW {
			t.Errorf("%q last lane drops at x=%d, want %d", lv.Name, x, fbW-fruitW)
		}
	}
}

// TestWaveNamesFitThePanel: render.go centres the name in ui.Font3x5, which
// draws only capitals, digits and some punctuation. The 4-pixel advance is
// that font's, restated because this file does not import sdk/ui.
func TestWaveNamesFitThePanel(t *testing.T) {
	for i := range wavesPack.Levels {
		name := wavesPack.Levels[i].Name
		if w := len(name)*4 - 1; w > fbW {
			t.Errorf("%q is %dpx wide, wider than the %dpx panel", name, w, fbW)
		}
		for j := 0; j < len(name); j++ {
			c := name[j]
			if c == ' ' || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
				continue
			}
			t.Errorf("%q: the 3x5 font has no glyph for %q", name, string(c))
		}
	}
}

func TestOverlaps(t *testing.T) {
	cases := []struct {
		name           string
		ax, ay, bx, by int
		want           bool
	}{
		{"identical", 0, 0, 0, 0, true},
		{"touching left edge", 0, 0, 5, 0, false},
		{"one px overlap", 0, 0, 4, 0, true},
		{"above", 0, 0, 0, 5, false},
		{"far away", 0, 0, 40, 40, false},
	}
	for _, c := range cases {
		if got := overlaps(c.ax, c.ay, 5, 5, c.bx, c.by, 5, 5); got != c.want {
			t.Errorf("%s: overlaps = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestLampCuePathIsTheOneTheManifestLists ties peripherals.go's
// lightsGameOver to the path in the manifest's assets.lights. The cabinet
// honours only listed paths, so a mismatch would fail silently.
func TestLampCuePathIsTheOneTheManifestLists(t *testing.T) {
	raw, err := os.ReadFile("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Assets struct {
			Lights []string `json:"lights"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, p := range m.Assets.Lights {
		if p == lightsGameOver {
			return
		}
	}
	t.Fatalf("peripherals.go plays %q, which manifest.json's assets.lights (%v) does not list", lightsGameOver, m.Assets.Lights)
}
