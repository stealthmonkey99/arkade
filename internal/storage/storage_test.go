// SPDX-License-Identifier: GPL-2.0-or-later

package storage

import (
	"os"
	"path/filepath"
	"testing"
)

type scores struct {
	HighScore int      `json:"highScore"`
	Names     []string `json:"names,omitempty"`
}

func TestWriteReadRoundTrip(t *testing.T) {
	s := New(t.TempDir())
	in := scores{HighScore: 9001, Names: []string{"BOB"}}
	if err := s.Write("games/snake", "scores", in); err != nil {
		t.Fatal(err)
	}
	var out scores
	if err := s.Read("games/snake", "scores", &out); err != nil {
		t.Fatal(err)
	}
	if out.HighScore != 9001 || len(out.Names) != 1 {
		t.Errorf("round trip mismatch: %+v", out)
	}
}

func TestReadMissingLeavesDefaults(t *testing.T) {
	s := New(t.TempDir())
	out := scores{HighScore: 42}
	if err := s.Read("games/none", "", &out); err != nil {
		t.Fatal(err)
	}
	if out.HighScore != 42 {
		t.Error("missing file should leave defaults untouched")
	}
}

func TestDefaultLabelIsSettings(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if err := s.Write("shell", "", scores{HighScore: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "shell", "settings.json")); err != nil {
		t.Errorf("default label should be settings.json: %v", err)
	}
}

func TestWriteIsAtomicNoTmpLeftover(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if err := s.Write("a/b", "x", scores{}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "a", "b"))
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Error("tmp file left behind")
		}
	}
}

func TestDeleteMissingIsFine(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Delete("ghost", ""); err != nil {
		t.Errorf("deleting missing file should not error: %v", err)
	}
}
