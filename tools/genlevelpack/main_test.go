// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunFmtCanonicalizesAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.arkadelevels")
	src := "pack demo\nlegend\n  #  wall\n  \\s floor\nlevel L01\n  par  6\n  |###|\n  |# #|\n  |###|\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runFmt(path); err != nil {
		t.Fatalf("runFmt: %v", err)
	}
	once, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(once), "par  6") {
		t.Errorf("runFmt left the hand-aligned spacing in place:\n%s", once)
	}
	if !strings.Contains(string(once), "par 6") {
		t.Errorf("runFmt did not canonicalise the metadata:\n%s", once)
	}

	if err := runFmt(path); err != nil {
		t.Fatalf("second runFmt: %v", err)
	}
	twice, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(twice) != string(once) {
		t.Errorf("runFmt is not idempotent\n--- first\n%s\n--- second\n%s", once, twice)
	}
}

func TestRunFmtPreservesComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.arkadelevels")
	src := `// a header comment
pack demo

legend
  #  wall
  \s floor

level L01
  |###|
  |# #|
  |###|
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runFmt(path); err != nil {
		t.Fatalf("runFmt: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "// a header comment") {
		t.Errorf("runFmt dropped the header comment:\n%s", got)
	}
}

func TestRunFmtRefusesAnUnparseablePackAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.arkadelevels")
	before := "pack demo\nlegend\n  #  wall\nlevel L01\n  |###|\n  |####|\n"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runFmt(path)
	if err == nil {
		t.Fatal("runFmt accepted a ragged grid")
	}
	if !strings.Contains(err.Error(), "wide") {
		t.Errorf("error = %v, want the parser's own width complaint", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != before {
		t.Errorf("a refused format still changed the file on disk:\n%s", after)
	}
}

func TestCartridgeGeneratesChecksAndSkips(t *testing.T) {
	if err := cartridge(t.TempDir(), true); err != nil {
		t.Errorf("a cartridge without assets/ = %v, want it skipped", err)
	}

	cart := t.TempDir()
	if err := os.Mkdir(filepath.Join(cart, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	pack := filepath.Join(cart, "assets", "walls.arkadelevels")
	if err := os.WriteFile(pack, []byte("pack walls\nlegend\n  # wall\nlevel ONE\n  |#|\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cartridge(cart, true); err == nil {
		t.Error("-check passed a cartridge that has never been generated")
	}
	if err := cartridge(cart, false); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cart, "levels_gen.go")); err != nil {
		t.Fatalf("no levels_gen.go after generating: %v", err)
	}
	if err := cartridge(cart, true); err != nil {
		t.Errorf("-check right after generating = %v", err)
	}
	if err := os.WriteFile(pack, []byte("pack walls\nlegend\n  # wall\nlevel TWO\n  |#|\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := cartridge(cart, true)
	if err == nil || !strings.Contains(err.Error(), "assets/walls.arkadelevels") {
		t.Errorf("-check after a pack edit = %v, want a stale error naming the pack", err)
	}
}
