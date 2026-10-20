// SPDX-License-Identifier: GPL-2.0-or-later

package packfile_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/levels"
	"github.com/stealthmonkey99/arkade/sdk/levels/packfile"
)

func TestIndexedHookSeesOrdinalPosition(t *testing.T) {
	dir := t.TempDir()
	// the index runs 0..3 across both packs, not restarting per file
	srcA := "pack a\nlegend\n  # wall\nlevel ONE\n  |#|\nlevel TWO\n  |#|\n"
	srcB := "pack b\nlegend\n  # wall\nlevel THREE\n  |#|\nlevel FOUR\n  |#|\n"
	if err := os.WriteFile(filepath.Join(dir, "a"+packfile.Ext), []byte(srcA), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b"+packfile.Ext), []byte(srcB), 0o644); err != nil {
		t.Fatal(err)
	}

	var seen []int
	var names []string
	hook := func(i int, lv *levels.Level) ([]levels.Entry, error) {
		seen = append(seen, i)
		names = append(names, lv.Name)
		return []levels.Entry{{Key: "ord", Int: i}}, nil
	}

	files, err := packfile.Generate(packfile.Config{
		Dir: dir, Out: filepath.Join(dir, "levels_gen.go"), Pkg: "main", IndexedHook: hook,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	wantIdx := []int{0, 1, 2, 3}
	wantNames := []string{"ONE", "TWO", "THREE", "FOUR"}
	if len(seen) != len(wantIdx) {
		t.Fatalf("hook ran %d time(s), want %d", len(seen), len(wantIdx))
	}
	for i := range wantIdx {
		if seen[i] != wantIdx[i] || names[i] != wantNames[i] {
			t.Errorf("call %d: (i=%d, name=%q), want (i=%d, name=%q)", i, seen[i], names[i], wantIdx[i], wantNames[i])
		}
	}

	all := append(append([]levels.Level{}, files[0].Pack.Levels...), files[1].Pack.Levels...)
	for i, lv := range all {
		if ord, ok := lv.Int("ord"); !ok || ord != i {
			t.Errorf("level %d: Int(\"ord\") = (%d, %v), want (%d, true)", i, ord, ok, i)
		}
	}
}

func TestIndexedHookFailsTheBuild(t *testing.T) {
	dir := t.TempDir()
	src := "pack a\nlegend\n  # wall\nlevel STUCK\n  |#|\n"
	if err := os.WriteFile(filepath.Join(dir, "a"+packfile.Ext), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "levels_gen.go")
	hook := func(i int, lv *levels.Level) ([]levels.Entry, error) {
		return nil, errors.New("out of order")
	}
	_, err := packfile.Generate(packfile.Config{Dir: dir, Out: out, Pkg: "main", IndexedHook: hook})
	if err == nil {
		t.Fatal("Generate accepted a failing IndexedHook")
	}
	if got := err.Error(); !strings.Contains(got, "STUCK") || !strings.Contains(got, "out of order") {
		t.Errorf("error = %v, want it to name the level and the reason", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Error("a failing IndexedHook still produced a generated file")
	}
}

func TestConfigRefusesBothHookKinds(t *testing.T) {
	dir := t.TempDir()
	src := "pack a\nlegend\n  # wall\nlevel ONE\n  |#|\n"
	if err := os.WriteFile(filepath.Join(dir, "a"+packfile.Ext), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	hook := func(lv *levels.Level) ([]levels.Entry, error) { return nil, nil }
	indexedHook := func(i int, lv *levels.Level) ([]levels.Entry, error) { return nil, nil }

	_, err := packfile.Generate(packfile.Config{
		Dir: dir, Out: filepath.Join(dir, "levels_gen.go"), Pkg: "main",
		Hook: hook, IndexedHook: indexedHook,
	})
	if err == nil {
		t.Fatal("Generate accepted a Config with both Hook and IndexedHook set")
	}
}
