// SPDX-License-Identifier: GPL-2.0-or-later

package packfile_test

import (
	"path/filepath"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/levels"
	"github.com/stealthmonkey99/arkade/sdk/levels/packfile"
)

// The two packs in testdata/ are checked cell for cell against hand-written Go
// transcriptions. pushing_solo.levels is one grid with coordinates as metadata
// and a `par` that hook_test.go's BFS derives. packing_puzzle.levels has names
// with spaces and two extra grids, one a column wider than the board.

func parseTestdata(t *testing.T, name string) *levels.Pack {
	t.Helper()
	f, err := packfile.ParseFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", name, err)
	}
	return f.Pack
}

// pushingCommitted is pushing_solo.levels as Go literals, padded with '#'.
var pushingCommitted = []struct {
	name     string
	w, h     int
	grid     [10]string // 12 characters per row
	p1x, p1y int
	par      int
}{
	{
		name: "L01", w: 6, h: 3, p1x: 3, p1y: 1, par: 1,
		grid: [10]string{
			"############",
			"#.$  #######",
			"############",
			"############",
			"############",
			"############",
			"############",
			"############",
			"############",
			"############",
		},
	},
	{
		name: "L02", w: 5, h: 6, p1x: 2, p1y: 4, par: 2,
		grid: [10]string{
			"############",
			"# . ########",
			"#   ########",
			"# $ ########",
			"#   ########",
			"############",
			"############",
			"############",
			"############",
			"############",
		},
	},
	{
		name: "L03", w: 6, h: 5, p1x: 1, p1y: 1, par: 6,
		grid: [10]string{
			"############",
			"#    #######",
			"# $  #######",
			"#   .#######",
			"############",
			"############",
			"############",
			"############",
			"############",
			"############",
		},
	},
	{
		name: "L04", w: 7, h: 5, p1x: 3, p1y: 3, par: 11,
		grid: [10]string{
			"############",
			"#.   .######",
			"# $ $ ######",
			"#     ######",
			"############",
			"############",
			"############",
			"############",
			"############",
			"############",
		},
	},
}

func TestFidelityPushing(t *testing.T) {
	pack := parseTestdata(t, "pushing_solo.levels")
	if len(pack.Levels) != len(pushingCommitted) {
		t.Fatalf("%d levels, want %d", len(pack.Levels), len(pushingCommitted))
	}

	for i, want := range pushingCommitted {
		lv := &pack.Levels[i]
		if lv.Name != want.name || lv.W != want.w || lv.H != want.h {
			t.Errorf("level %d = %q %dx%d, want %q %dx%d", i, lv.Name, lv.W, lv.H, want.name, want.w, want.h)
			continue
		}

		for y := 0; y < 10; y++ {
			for x := 0; x < 12; x++ {
				got := lv.At(x, y)
				if got == 0 {
					got = '#' // out-of-level padding
				}
				if got != want.grid[y][x] {
					t.Fatalf("%s cell (%d, %d) = %q, want %q", want.name, x, y, got, want.grid[y][x])
				}
			}
		}

		if px, ok := lv.Int("p1x"); !ok || px != want.p1x {
			t.Errorf("%s p1x = (%d, %v), want %d", want.name, px, ok, want.p1x)
		}
		if py, ok := lv.Int("p1y"); !ok || py != want.p1y {
			t.Errorf("%s p1y = (%d, %v), want %d", want.name, py, ok, want.p1y)
		}
		if coop, ok := lv.Bool("coop"); !ok || coop {
			t.Errorf("%s coop = (%v, %v), want (false, true)", want.name, coop, ok)
		}

		derived, err := crateSolverPar(lv)
		if err != nil {
			t.Fatalf("%s: solver hook: %v", want.name, err)
		}
		if len(derived) != 1 || derived[0].Key != "par" || derived[0].Int != want.par {
			t.Errorf("%s derived %v, want par = %d", want.name, derived, want.par)
		}
	}
}

// packingCommitted is packing_puzzle.levels as a Go literal. A divider row with
// no dividers is the empty string; the pack spells those rows out.
var packingCommitted = []struct {
	name          string
	grid          [9]string
	hfix, vfix    [9]string // "" means "no dividers on this row"
	budget, goal  int
	hasDividers   bool
	vfixRowLength int
}{
	{
		name: "ONE PIECE", budget: 2, goal: 1,
		grid: [9]string{
			"..........", "..........", "..........",
			"..........", "..........", "..........",
			"..........", "..........", ".r........",
		},
	},
	{
		name: "FILL THE ROW", budget: 6, goal: 4,
		grid: [9]string{
			"..........", "..........", "..........",
			"..........", "..........", "..........",
			"..........", "..........", "rgb.......",
		},
	},
	{
		name: "HIGH SHELF", budget: 4, goal: 2,
		grid: [9]string{
			"..........", "..........", "y.........",
			"p.........", "..........", "..........",
			"..........", "..........", "..........",
		},
	},
	{
		name: "WALLED IN", budget: 5, goal: 3, hasDividers: true, vfixRowLength: 11,
		grid: [9]string{
			"..........", "..........", "..........",
			"..........", "..........", ".....k....",
			"..........", "....sWs...", ".w...b...i",
		},
		hfix: [9]string{5: ".....X....", 8: ".X.......X"},
		vfix: [9]string{5: ".....XX....", 8: ".X...XX...X"},
	},
}

func TestFidelityPacking(t *testing.T) {
	pack := parseTestdata(t, "packing_puzzle.levels")
	if len(pack.Levels) != len(packingCommitted) {
		t.Fatalf("%d levels, want %d", len(pack.Levels), len(packingCommitted))
	}

	for i, want := range packingCommitted {
		lv := &pack.Levels[i]
		if lv.Name != want.name {
			t.Errorf("level %d is %q, want %q", i, lv.Name, want.name)
			continue
		}
		if lv.W != 10 || lv.H != 9 {
			t.Errorf("%s is %dx%d, want 10x9", want.name, lv.W, lv.H)
		}
		for y := 0; y < 9; y++ {
			if got := lv.Row(y); got != want.grid[y] {
				t.Errorf("%s row %d = %q, want %q", want.name, y, got, want.grid[y])
			}
		}
		if b, ok := lv.Int("budget"); !ok || b != want.budget {
			t.Errorf("%s budget = (%d, %v), want %d", want.name, b, ok, want.budget)
		}
		if g, ok := lv.Int("goal"); !ok || g != want.goal {
			t.Errorf("%s goal = (%d, %v), want %d", want.name, g, ok, want.goal)
		}

		if !want.hasDividers {
			if len(lv.Layers) != 0 {
				t.Errorf("%s has %d layers, want none", want.name, len(lv.Layers))
			}
			continue
		}
		checkFixLayer(t, lv, "hfix", want.name, want.hfix, 10)
		checkFixLayer(t, lv, "vfix", want.name, want.vfix, want.vfixRowLength)
	}
}

func checkFixLayer(t *testing.T, lv *levels.Level, layer, level string, want [9]string, width int) {
	t.Helper()
	ly := lv.Layer(layer)
	if ly == nil {
		t.Fatalf("%s has no %q layer", level, layer)
	}
	if ly.W != width || ly.H != 9 {
		t.Fatalf("%s layer %q is %dx%d, want %dx9", level, layer, ly.W, ly.H, width)
	}
	for y := 0; y < 9; y++ {
		for x := 0; x < width; x++ {
			divider := want[y] != "" && want[y][x] == 'X'
			if got := ly.At(x, y) == 'X'; got != divider {
				t.Errorf("%s layer %q (%d, %d) divider = %v, want %v", level, layer, x, y, got, divider)
			}
		}
	}
}

func TestFidelityPacksGenerate(t *testing.T) {
	files, err := packfile.Generate(packfile.Config{
		Dir: "testdata",
		Out: filepath.Join(t.TempDir(), "levels_gen.go"),
		Pkg: "main",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("%d packs, want 2", len(files))
	}
	if files[0].Pack.Name != "puzzle" || files[1].Pack.Name != "solo" {
		t.Errorf("packs = %q, %q, want puzzle then solo", files[0].Pack.Name, files[1].Pack.Name)
	}
}
