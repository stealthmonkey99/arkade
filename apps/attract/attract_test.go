// SPDX-License-Identifier: GPL-2.0-or-later

package attract_test

import (
	"math"
	"testing"

	_ "github.com/stealthmonkey99/arkade/apps/attract"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/scores"
	"github.com/stealthmonkey99/arkade/internal/storage"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// testGameSlug is a TypeGame stand-in for readHighScores to scan; it is
// never loaded.
const testGameSlug = "test/game"

func init() {
	engine.Register(engine.Manifest{
		Slug: testGameSlug,
		Name: "Test Game",
		Type: engine.TypeGame,
		New:  func(ctx *engine.AppContext) engine.App { return nil },
	})
}

func TestAttractGoldenAmbient(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "attract"})
	rig.TickFor("3s")
	rig.Golden("attract-ambient")
}

func TestAttractGoldenFigure(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "attract"})
	rig.TickFor("9s") // past the 8s wordmark slot, into the figure slot
	rig.Golden("attract-figure")
}

func TestAttractGoldenShowcase(t *testing.T) {
	dir := t.TempDir()
	if err := storage.New(dir).Write(testGameSlug, "scores", struct {
		HighScore int `json:"highScore"`
	}{HighScore: 4242}); err != nil {
		t.Fatalf("seed score: %v", err)
	}

	rig := testkit.NewRig(t, testkit.Options{StartApp: "attract", DataDir: dir})
	rig.TickFor("16.1s") // past the two 8s ambient slots, into the showcase slot
	rig.Golden("attract-showcase")
}

func TestAttractGoldenShowcaseWithName(t *testing.T) {
	dir := t.TempDir()
	if err := storage.New(dir).Write(testGameSlug, "scores", struct {
		HighScore int    `json:"highScore"`
		Name      string `json:"name"`
	}{HighScore: 4242, Name: "ada"}); err != nil {
		t.Fatalf("seed score: %v", err)
	}

	rig := testkit.NewRig(t, testkit.Options{StartApp: "attract", DataDir: dir})
	rig.TickFor("16.1s")
	rig.Golden("attract-showcase-named")
}

// hasTrophyGold reports whether the trophy's gold is on screen, which doubles
// as a showcase-visible probe.
func hasTrophyGold(frame display.Matrix) bool {
	gold := display.Pixel{R: 252, G: 192, B: 32, A: 255}
	for _, row := range frame {
		for _, p := range row {
			if p == gold {
				return true
			}
		}
	}
	return false
}

func TestAttractLeaderboardTickerAndExtendedSlot(t *testing.T) {
	dir := t.TempDir()
	table := scores.Table{}
	table.Insert(4242, "matt")
	table.Insert(4210, "zoe")
	table.Insert(900, "") // anonymous entry: row renders rank+score only
	table.Insert(50, "rex")
	if err := scores.Save(storage.New(dir), testGameSlug, table); err != nil {
		t.Fatalf("seed table: %v", err)
	}

	rig := testkit.NewRig(t, testkit.Options{StartApp: "attract", DataDir: dir})
	// The showcase starts at 16s; 2.5s in, rank #2 is fully visible and #3
	// is rising in.
	rig.TickFor("18.5s")
	if !hasTrophyGold(rig.Frame()) {
		t.Fatal("expected the gold trophy beside the HIGH line on the showcase")
	}
	rig.Golden("attract-leaderboard-midscroll")

	// 3 ticker rows extend the slot to max(8s, 7.35s+2s) = 9.35s.
	rig.TickFor("6s") // t=24.5s: inside the extension
	if !hasTrophyGold(rig.Frame()) {
		t.Error("the showcase slot should extend past the classic 8s while the ticker crawls")
	}
	rig.TickFor("1s") // t=25.5s: past 16s+9.35s
	if hasTrophyGold(rig.Frame()) {
		t.Error("the extended slot should still end and hand back to the ambient view")
	}
}

func TestAttractSingleEntryShowsNoTicker(t *testing.T) {
	dir := t.TempDir()
	table := scores.Table{}
	table.Insert(4242, "zed")
	if err := scores.Save(storage.New(dir), testGameSlug, table); err != nil {
		t.Fatalf("seed table: %v", err)
	}

	rig := testkit.NewRig(t, testkit.Options{StartApp: "attract", DataDir: dir})
	rig.TickFor("18.5s")
	frame := rig.Frame()
	if !hasTrophyGold(frame) {
		t.Fatal("expected the trophy on a single-entry showcase")
	}
	// The ticker strip (y52-63) must hold no leaderboard lettering.
	dim := display.Pixel{R: 104, G: 104, B: 104, A: 255}
	for y := 52; y < 64; y++ {
		for x := range frame[y] {
			if frame[y][x] == dim {
				t.Fatalf("single-entry game must not render ticker rows, found one at (%d,%d)", x, y)
			}
		}
	}
	rig.TickFor("5.7s") // t=24.2s
	if hasTrophyGold(rig.Frame()) {
		t.Error("a single-entry game must keep the classic 8s slot")
	}
}

func TestAttractFrameIsFullyOpaque(t *testing.T) {
	assertOpaque := func(t *testing.T, frame display.Matrix) {
		t.Helper()
		for y, row := range frame {
			for x, p := range row {
				if p.A != 255 {
					t.Fatalf("pixel (%d,%d) not fully opaque: %+v", x, y, p)
				}
			}
		}
	}

	t.Run("wordmark slot", func(t *testing.T) {
		rig := testkit.NewRig(t, testkit.Options{StartApp: "attract"})
		rig.TickFor("3s")
		assertOpaque(t, rig.Frame())
	})

	t.Run("figure slot", func(t *testing.T) {
		rig := testkit.NewRig(t, testkit.Options{StartApp: "attract"})
		rig.TickFor("9s")
		assertOpaque(t, rig.Frame())
	})

	t.Run("showcase slot", func(t *testing.T) {
		dir := t.TempDir()
		if err := storage.New(dir).Write(testGameSlug, "scores", struct {
			HighScore int `json:"highScore"`
		}{HighScore: 4242}); err != nil {
			t.Fatalf("seed score: %v", err)
		}
		rig := testkit.NewRig(t, testkit.Options{StartApp: "attract", DataDir: dir})
		rig.TickFor("16.1s")
		assertOpaque(t, rig.Frame())
	})
}

// litPositions returns every pixel that isn't opaque black.
func litPositions(frame display.Matrix) [][2]int {
	var out [][2]int
	for y, row := range frame {
		for x, p := range row {
			if p != display.Black {
				out = append(out, [2]int{x, y})
			}
		}
	}
	return out
}

// minMaxDist returns the nearest and farthest distance from the canvas
// centre among positions.
func minMaxDist(positions [][2]int) (min, max float64) {
	min = math.Inf(1)
	for _, p := range positions {
		dx := float64(p[0]) - 32
		dy := float64(p[1]) - 32
		d := math.Hypot(dx, dy)
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}
	return min, max
}

func TestAttractStarfieldWarpsOutward(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "attract"})

	rig.TickFor("500ms")
	early := litPositions(rig.Frame())
	if len(early) == 0 {
		t.Fatal("expected lit star pixels early on")
	}

	rig.TickFor("1s")
	later := litPositions(rig.Frame())
	if len(later) == 0 {
		t.Fatal("expected lit star pixels later on")
	}

	if positionsEqual(early, later) {
		t.Fatal("star positions did not change between frames a second apart; expected continuous warp motion")
	}

	const (
		innerBand = 10.0 // a fresh star sits at about 3px
		outerBand = 15.0 // long-travelled stars reach 20-40px
	)
	minDist, maxDist := minMaxDist(later)
	if minDist >= innerBand {
		t.Errorf("expected some stars still near the spawn point at center, closest lit pixel is %.1fpx out", minDist)
	}
	if maxDist <= outerBand {
		t.Errorf("expected some stars swept out toward the edge, farthest lit pixel is only %.1fpx out", maxDist)
	}
}

func positionsEqual(a, b [][2]int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAttractNeverAppearsInShellMenu(t *testing.T) {
	for _, m := range engine.Apps() {
		if m.Slug == "attract" {
			t.Fatalf("attract must be Hidden (excluded from engine.Apps()), found: %+v", m)
		}
	}
}

func TestAttractLightsOffAndChases(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "attract"})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	lit := 0
	for player := 0; player < 2; player++ {
		for _, name := range hardware.LightButtons {
			if hw.ButtonLit(player, name) {
				lit++
			}
		}
	}
	if lit != 0 {
		t.Fatalf("expected all lights off right after Init, got %d lit", lit)
	}

	rig.TickFor("1.5s")
	for player := 0; player < 2; player++ {
		lit = 0
		for _, name := range hardware.LightButtons {
			if hw.ButtonLit(player, name) {
				lit++
			}
		}
		if lit != 1 {
			t.Errorf("player %d: expected exactly one chased light, got %d", player, lit)
		}
	}
}

func TestAttractChaseHopsRandomlyPerPlayer(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "attract"})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	litButton := func(player int) string {
		t.Helper()
		found := ""
		for _, name := range hardware.LightButtons {
			if hw.ButtonLit(player, name) {
				if found != "" {
					t.Fatalf("player %d: more than one light lit", player)
				}
				found = name
			}
		}
		if found == "" {
			t.Fatalf("player %d: no light lit", player)
		}
		return found
	}

	rig.TickFor("1.1s") // first hop has landed
	prev := [2]string{litButton(0), litButton(1)}
	diverged := false
	for step := 0; step < 12; step++ {
		rig.TickFor("1s")
		cur := [2]string{litButton(0), litButton(1)}
		for p := 0; p < 2; p++ {
			if cur[p] == prev[p] {
				t.Errorf("step %d: player %d stayed on %q; every hop must move", step, p, cur[p])
			}
		}
		if cur[0] != cur[1] {
			diverged = true
		}
		prev = cur
	}
	if !diverged {
		t.Error("players tracked identically for 12 steps; rolls should be independent per player")
	}
}
