// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/levels/packfile"
)

const fixturePack = "pack walls\nlegend\n  # wall\n  . floor\nlevel ONE\n  |###|\n  |#.#|\n  |###|\n"

func levelsFixture(t *testing.T) *Cart {
	t.Helper()
	c := tempCart(t, "fixture")
	if err := os.MkdirAll(filepath.Join(c.Dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeLevels(t, c, fixturePack)
	if _, err := packfile.Generate(packfile.CartridgeConfig(c.Dir)); err != nil {
		t.Fatal(err)
	}
	return c
}

func writeLevels(t *testing.T, c *Cart, src string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(c.Dir, "assets", "walls.arkadelevels"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLevelsPassesACurrentPack(t *testing.T) {
	res := run(t, "levels", levelsFixture(t))
	if res.Status != StatusPass {
		t.Errorf("status %s, want pass\n%s", res.Status, findings(res))
	}
}

func TestLevelsCatchesAStalePack(t *testing.T) {
	c := levelsFixture(t)
	writeLevels(t, c, strings.Replace(fixturePack, "|#.#|", "|#..|", 1))
	res := run(t, "levels", c)
	if res.Status != StatusFail {
		t.Fatalf("status %s, want fail\n%s", res.Status, findings(res))
	}
	wantContains(t, findings(res), "assets/walls.arkadelevels", "the message names the pack the Go fell behind")
	wantContains(t, findings(res), "make build", "the message names the mechanical fix")
}

func TestLevelsCatchesAHandEditedFile(t *testing.T) {
	c := levelsFixture(t)
	gen := filepath.Join(c.Dir, packfile.GenFile)
	src, err := os.ReadFile(gen)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gen, []byte(strings.Replace(string(src), `"ONE"`, `"TWO"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	res := run(t, "levels", c)
	if res.Status != StatusFail {
		t.Fatalf("status %s, want fail\n%s", res.Status, findings(res))
	}
	wantContains(t, findings(res), "hand-edited", "an edit to the build output is named as such")
}

func TestLevelsCatchesAMissingFile(t *testing.T) {
	c := levelsFixture(t)
	if err := os.Remove(filepath.Join(c.Dir, packfile.GenFile)); err != nil {
		t.Fatal(err)
	}
	res := run(t, "levels", c)
	if res.Status != StatusFail {
		t.Fatalf("status %s, want fail\n%s", res.Status, findings(res))
	}
	wantContains(t, findings(res), "no levels_gen.go", "a cartridge with packs was never built")
}

func TestLevelsPassesACartridgeWithoutLevels(t *testing.T) {
	res := run(t, "levels", tempCart(t, "fixture"))
	if res.Status != StatusPass {
		t.Errorf("status %s, want pass — nothing to generate\n%s", res.Status, findings(res))
	}
}
