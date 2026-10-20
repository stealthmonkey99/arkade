// SPDX-License-Identifier: GPL-2.0-or-later

package packfile_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/levels"
	"github.com/stealthmonkey99/arkade/sdk/levels/packfile"
)

// The hook is a BFS from the level holding the spawn across `exit` edges,
// refusing a pack with a room nothing can reach.
func TestPackHookRefusesAnUnreachableLevel(t *testing.T) {
	reachable := func(p *levels.Pack) error {
		start := -1
		for i := range p.Levels {
			if p.Levels[i].Record("spawn") != nil {
				start = i
			}
		}
		if start < 0 {
			return errors.New("no level holds a spawn")
		}
		seen := map[int]bool{start: true}
		queue := []int{start}
		for len(queue) > 0 {
			lv := &p.Levels[queue[0]]
			queue = queue[1:]
			for i := range lv.Exits {
				if t := lv.Exits[i].Target; !seen[t] {
					seen[t] = true
					queue = append(queue, t)
				}
			}
		}
		for i := range p.Levels {
			if !seen[i] {
				return fmt.Errorf("level %q is unreachable from the spawn", p.Levels[i].Name)
			}
		}
		return nil
	}

	const head = "pack rooms\nlegend\n  # wall\n"
	for _, tc := range []struct {
		name string
		src  string
		want string // "" means the pack must be accepted
	}{
		{
			name: "connected",
			src: head +
				"level A\n  |#|\n  ent spawn 0 0\n  exit right B\n" +
				"level B\n  |#|\n  exit left A\n",
		},
		{
			name: "island",
			src: head +
				"level A\n  |#|\n  ent spawn 0 0\n" +
				"level B\n  |#|\n",
			want: `level "B" is unreachable`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			out := filepath.Join(dir, "levels_gen.go")
			if err := os.WriteFile(filepath.Join(dir, "rooms"+packfile.Ext), []byte(tc.src), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := packfile.Generate(packfile.Config{
				Dir: dir, Out: out, Pkg: "main", PackHook: reachable,
			})
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				if _, err := os.Stat(out); err != nil {
					t.Errorf("a passing pack wrote nothing: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Generate accepted a pack the hook rejects")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Errorf("Generate wrote %s despite the PackHook refusing", out)
			}
		})
	}
}

// The per-level hook runs first, so the pack hook never sees a level that
// already failed.
func TestPackHookRunsAfterEveryLevelHook(t *testing.T) {
	dir := t.TempDir()
	src := "pack rooms\nlegend\n  # wall\nlevel A\n  |#|\nlevel B\n  |#|\n"
	if err := os.WriteFile(filepath.Join(dir, "rooms"+packfile.Ext), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	var order []string
	_, err := packfile.Generate(packfile.Config{
		Dir: dir, Out: filepath.Join(dir, "levels_gen.go"), Pkg: "main",
		Hook: func(lv *levels.Level) ([]levels.Entry, error) {
			order = append(order, "level "+lv.Name)
			return nil, nil
		},
		PackHook: func(p *levels.Pack) error {
			order = append(order, "pack "+p.Name)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := strings.Join(order, ","); got != "level A,level B,pack rooms" {
		t.Errorf("hook order = %q, want every level then the pack", got)
	}
}
