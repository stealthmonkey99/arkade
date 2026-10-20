// SPDX-License-Identifier: GPL-2.0-or-later

package packfile_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/sdk/levels/packfile"
)

const tinyPack = "pack walls\nlegend\n  # wall\nlevel ONE\n  |#|\n"

func newCart(t *testing.T) string {
	t.Helper()
	cart := t.TempDir()
	if err := os.Mkdir(filepath.Join(cart, packfile.AssetsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(cart, packfile.AssetsDir, "walls"+packfile.ArkadeExt), tinyPack)
	writeFile(t, filepath.Join(cart, packfile.AssetsDir, "pop.arkadesound.wav"), "not a pack")
	return cart
}

func writeFile(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateLeavesAnUpToDateFileUntouched(t *testing.T) {
	cart := newCart(t)
	cfg := packfile.CartridgeConfig(cart)
	if _, err := packfile.Generate(cfg); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(cfg.Out, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := packfile.Generate(cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(cfg.Out)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(old) {
		t.Errorf("an unchanged regeneration rewrote %s (mtime %v, want %v)", packfile.GenFile, info.ModTime(), old)
	}
}

func TestCheckSeesMissingStaleAndHandEditedFiles(t *testing.T) {
	cart := newCart(t)
	cfg := packfile.CartridgeConfig(cart)

	err := packfile.Check(cfg)
	var stale *packfile.StaleError
	if !errors.As(err, &stale) || !strings.Contains(err.Error(), "no levels_gen.go") {
		t.Fatalf("Check with no generated file = %v, want a StaleError naming the missing file", err)
	}

	if _, err := packfile.Generate(cfg); err != nil {
		t.Fatal(err)
	}
	if err := packfile.Check(cfg); err != nil {
		t.Fatalf("Check right after Generate = %v", err)
	}

	writeFile(t, filepath.Join(cart, packfile.AssetsDir, "walls"+packfile.ArkadeExt), strings.Replace(tinyPack, "|#|", "|##|", 1))
	err = packfile.Check(cfg)
	if !errors.As(err, &stale) {
		t.Fatalf("Check after a pack edit = %v, want a StaleError", err)
	}
	if want := "assets/walls.arkadelevels"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not name the source %s", err, want)
	}

	if _, err := packfile.Generate(cfg); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(cfg.Out)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, cfg.Out, strings.Replace(string(src), `"ONE"`, `"TWO"`, 1))
	if err := packfile.Check(cfg); !errors.As(err, &stale) {
		t.Errorf("Check after a hand edit of the Go = %v, want a StaleError", err)
	}
}

func TestCheckCoversExtras(t *testing.T) {
	cart := newCart(t)
	cfg := packfile.CartridgeConfig(cart)
	if _, err := packfile.Generate(cfg); err != nil {
		t.Fatal(err)
	}
	ids := filepath.Join(cart, "ids_gen.go")
	extra := func(files []*packfile.File) (packfile.Output, error) {
		return packfile.Output{Path: ids, Src: []byte("package main\n\nconst numLevels = " + string(rune('0'+len(files[0].Pack.Levels))) + "\n")}, nil
	}
	if err := packfile.Check(cfg, extra); err == nil || !strings.Contains(err.Error(), "no ids_gen.go") {
		t.Errorf("Check with a missing extra = %v, want it named", err)
	}
	if _, err := os.Stat(ids); !os.IsNotExist(err) {
		t.Errorf("Check wrote the extra (stat err %v)", err)
	}
}

func TestHasPacks(t *testing.T) {
	cart := newCart(t)
	if ok, err := packfile.HasPacks(cart); !ok || err != nil {
		t.Errorf("HasPacks(cart with a pack) = %v, %v", ok, err)
	}
	if ok, err := packfile.HasPacks(t.TempDir()); ok || err != nil {
		t.Errorf("HasPacks(cart without assets/) = %v, %v, want false, nil", ok, err)
	}
	spritesOnly := t.TempDir()
	if err := os.Mkdir(filepath.Join(spritesOnly, packfile.AssetsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(spritesOnly, packfile.AssetsDir, "hero.arkadesprite.png"), "png")
	if ok, err := packfile.HasPacks(spritesOnly); ok || err != nil {
		t.Errorf("HasPacks(assets/ with no pack) = %v, %v, want false, nil", ok, err)
	}
	legacy := t.TempDir()
	if err := os.Mkdir(filepath.Join(legacy, packfile.AssetsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(legacy, packfile.AssetsDir, "old"+packfile.Ext), tinyPack)
	if ok, _ := packfile.HasPacks(legacy); !ok {
		t.Error("HasPacks ignores a pack under the older .levels extension")
	}
}
