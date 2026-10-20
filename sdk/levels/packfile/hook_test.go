// SPDX-License-Identifier: GPL-2.0-or-later

package packfile_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/levels"
	"github.com/stealthmonkey99/arkade/sdk/levels/packfile"
)

// crateSolverPar solves a single-player crate-pushing level by breadth-first
// search and returns its optimal move count as `par`. An unsolvable level, or
// one already solved before the player moves, is an error.
func crateSolverPar(lv *levels.Level) ([]levels.Entry, error) {
	px, okX := lv.Int("p1x")
	py, okY := lv.Int("p1y")
	if !okX || !okY {
		return nil, errors.New("no p1x/p1y player start")
	}
	if coop, ok := lv.Bool("coop"); ok && coop {
		return nil, errors.New("this hook only solves single-player levels")
	}

	var crates []int
	for y := 0; y < lv.H; y++ {
		for x := 0; x < lv.W; x++ {
			if c := lv.At(x, y); c == '$' || c == '*' {
				crates = append(crates, y*lv.W+x)
			}
		}
	}
	if len(crates) == 0 {
		return nil, errors.New("no crates")
	}

	// under a crate, '$' is floor and '*' is a target
	target := func(i int) bool { c := lv.Cells[i]; return c == '.' || c == '*' }
	wall := func(x, y int) bool { return lv.At(x, y) == '#' || lv.At(x, y) == 0 }

	solved := func(crates []int) bool {
		for _, c := range crates {
			if !target(c) {
				return false
			}
		}
		return true
	}
	key := func(p int, crates []int) string {
		b := make([]byte, 0, 2*(len(crates)+1))
		b = append(b, byte(p>>8), byte(p))
		for _, c := range crates {
			b = append(b, byte(c>>8), byte(c))
		}
		return string(b)
	}

	sort.Ints(crates)
	start := py*lv.W + px
	if solved(crates) {
		return nil, errors.New("already solved at the start — not a puzzle")
	}

	type state struct {
		player int
		crates []int
		dist   int
	}
	queue := []state{{start, crates, 0}}
	seen := map[string]bool{key(start, crates): true}
	steps := [4][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}

	for qi := 0; qi < len(queue); qi++ {
		cur := queue[qi]
		cx, cy := cur.player%lv.W, cur.player/lv.W
		for _, d := range steps {
			nx, ny := cx+d[0], cy+d[1]
			if wall(nx, ny) {
				continue
			}
			np := ny*lv.W + nx
			next := cur.crates
			if ci := indexOf(next, np); ci >= 0 {
				bx, by := nx+d[0], ny+d[1]
				if wall(bx, by) {
					continue
				}
				bp := by*lv.W + bx
				if indexOf(next, bp) >= 0 {
					continue
				}
				next = append([]int(nil), cur.crates...)
				next[ci] = bp
				sort.Ints(next)
			}
			k := key(np, next)
			if seen[k] {
				continue
			}
			seen[k] = true
			if solved(next) {
				return []levels.Entry{{Key: "par", Int: cur.dist + 1}}, nil
			}
			queue = append(queue, state{np, next, cur.dist + 1})
		}
	}
	return nil, fmt.Errorf("unsolvable (%d states explored)", len(seen))
}

func indexOf(s []int, v int) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func TestHookFailsTheBuild(t *testing.T) {
	dir := t.TempDir()
	// the crate is wedged in a corner
	src := `pack solo
legend
  #  wall
  \s floor
  .  target
  $  crate
  *  crate-on-target
level STUCK
  p1x 2
  p1y 2
  |#####|
  |#$  #|
  |#  .#|
  |#####|
`
	if err := os.WriteFile(filepath.Join(dir, "solo"+packfile.Ext), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "levels_gen.go")
	_, err := packfile.Generate(packfile.Config{Dir: dir, Out: out, Pkg: "main", Hook: crateSolverPar})
	if err == nil {
		t.Fatal("Generate accepted an unsolvable level")
	}
	if got := err.Error(); !strings.Contains(got, "STUCK") || !strings.Contains(got, "unsolvable") {
		t.Errorf("error = %v, want it to name the level and the reason", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Error("an unsolvable level still produced a generated file")
	}
}

// A hook's derived entry replaces a same-key entry from the pack file.
func TestHookDerivedMetadataMerges(t *testing.T) {
	dir := t.TempDir()
	src := `pack solo
legend
  #  wall
  \s floor
  .  target
  $  crate
  *  crate-on-target
level ONE
  p1x 3
  p1y 1
  |#####|
  |#.$ #|
  |#   #|
  |#####|
level PINNED
  p1x 3
  p1y 1
  par 99
  |#####|
  |#.$ #|
  |#   #|
  |#####|
`
	if err := os.WriteFile(filepath.Join(dir, "solo"+packfile.Ext), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := packfile.Generate(packfile.Config{
		Dir: dir, Out: filepath.Join(dir, "levels_gen.go"), Pkg: "main", Hook: crateSolverPar,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	lvs := files[0].Pack.Levels
	if par, ok := lvs[0].Int("par"); !ok || par != 1 {
		t.Errorf(`derived Int("par") = (%d, %v), want (1, true)`, par, ok)
	}
	if par, ok := lvs[1].Int("par"); !ok || par != 1 {
		t.Errorf(`Int("par") = (%d, %v), want the hook's 1 to replace the file's 99`, par, ok)
	}
	for i := 1; i < len(lvs[0].Meta); i++ {
		if lvs[0].Meta[i-1].Key > lvs[0].Meta[i].Key {
			t.Fatalf("metadata is not sorted after merging: %v", lvs[0].Meta)
		}
	}
}
