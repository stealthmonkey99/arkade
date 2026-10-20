// SPDX-License-Identifier: GPL-2.0-or-later

// Package goldens implements the ASCII golden-frame file convention:
// testdata/golden/<name>.txt next to the owning test, rendered via
// render.ASCII and rewritten when UPDATE_GOLDENS=1 is set. Both internal/testkit
// and carttest wrap it, so every golden stays one format.
package goldens

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/render"
)

// Frame compares frame with testdata/golden/<name>.txt (relative to the
// calling test's directory, per go test's cwd convention). UPDATE_GOLDENS=1
// rewrites the file instead of comparing.
func Frame(t *testing.T, name string, frame display.Matrix) {
	t.Helper()
	got := render.ASCII(frame)
	path := filepath.Join("testdata", "golden", name+".txt")

	if os.Getenv("UPDATE_GOLDENS") != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("golden mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("golden write: %v", err)
		}
		t.Logf("golden updated: %s", path)
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s missing (run with UPDATE_GOLDENS=1 to create): %v", name, err)
	}
	if string(want) != got {
		t.Errorf("frame does not match golden %s\n--- got ---\n%s\n--- diff hint: run with UPDATE_GOLDENS=1 and `git diff testdata` ---", name, got)
	}
}
