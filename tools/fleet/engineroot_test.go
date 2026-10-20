// SPDX-License-Identifier: GPL-2.0-or-later

package fleet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEngineRootWalksThroughNestedModules(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(EngineModule+"\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(root, "sdk", "template")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner, "go.mod"), []byte("module arkade-cart-template\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := EngineRoot(inner)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(root)
	if g, _ := filepath.EvalSymlinks(got); g != want {
		t.Fatalf("EngineRoot(%q) = %q, want %q", inner, got, root)
	}
}

func TestEngineRootFailsOutsideTheEngine(t *testing.T) {
	loose := t.TempDir()
	if err := os.WriteFile(filepath.Join(loose, "go.mod"), []byte("module catcher\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := EngineRoot(loose); err == nil {
		t.Fatalf("EngineRoot(%q) = %q, want an error", loose, got)
	}
}
