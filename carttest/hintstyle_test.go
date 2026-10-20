// SPDX-License-Identifier: GPL-2.0-or-later

package carttest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsLetteredHint(t *testing.T) {
	for text, want := range map[string]bool{
		"PRESS A":            true,
		"Press B to start":   true,
		"A=PLAY":             true,
		"D = GO":             true,
		"B: BACK":            true,
		"A SET  D SWAP":      true,
		"Press a light":      false,
		"FOUR IN A ROW":      false,
		"A CAT IS NOT A PET": false,
		"PLAY":               false,
	} {
		if got := isLetteredHint(text); got != want {
			t.Errorf("isLetteredHint(%q) = %v, want %v", text, got, want)
		}
	}
}

type recordingTB struct {
	testing.TB
	errors, fatals int
}

func (r *recordingTB) Helper()               {}
func (r *recordingTB) Errorf(string, ...any) { r.errors++ }
func (r *recordingTB) Fatalf(string, ...any) { r.fatals++ }

func TestNoLetteredButtonHintsScansSourceNotTests(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("game.go", "package main\n\nvar prompt = \"PRESS A\"\nvar ok = \"FOUR IN A ROW\"\n")
	write("game_test.go", "package main\n\nvar quoted = \"A=PLAY\"\n")

	rec := &recordingTB{TB: t}
	NoLetteredButtonHints(rec, dir)
	if rec.errors != 1 || rec.fatals != 0 {
		t.Errorf("got %d errors, %d fatals; want 1 error (game.go's prompt) and no fatal", rec.errors, rec.fatals)
	}
}

func TestNoLetteredButtonHintsRefusesAnEmptyDir(t *testing.T) {
	rec := &recordingTB{TB: t}
	NoLetteredButtonHints(rec, t.TempDir())
	if rec.fatals != 1 {
		t.Errorf("an empty directory gave %d fatals, want 1", rec.fatals)
	}
}
