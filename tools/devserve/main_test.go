// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/tools/fleet"
)

// t.TempDir() sits under /var, a symlink on macOS, and the walkers do not resolve it.
func sameDir(a, b string) bool {
	aAbs, _ := filepath.EvalSymlinks(a)
	bAbs, _ := filepath.EvalSymlinks(b)
	return aAbs == bAbs
}

// testing.T.Chdir needs Go 1.24; the module is on 1.23.
func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) })
}

func writeEngineGoMod(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(fleet.EngineModule+"\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindEngineRootFromTheWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	writeEngineGoMod(t, root)
	deep := filepath.Join(root, "tools", "devserve")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, deep)

	got, err := findEngineRoot()
	if err != nil {
		t.Fatalf("findEngineRoot: %v", err)
	}
	if !sameDir(got, root) {
		t.Fatalf("findEngineRoot() from %q = %q, want %q", deep, got, root)
	}
}

func TestFindEngineRootFallsBackToTheSourceTree(t *testing.T) {
	chdir(t, t.TempDir())

	got, err := findEngineRoot()
	if err != nil {
		t.Fatalf("findEngineRoot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(got, "cmd", "arkade")); err != nil {
		t.Fatalf("findEngineRoot() = %q, which does not contain cmd/arkade: %v", got, err)
	}
}

func TestReadManifestTakesSlugAndTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1,"slug":"catcher","title":"Catcher"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readManifest(path)
	if err != nil {
		t.Fatalf("readManifest: %v", err)
	}
	if want := (cartManifest{Slug: "catcher", Title: "Catcher"}); got != want {
		t.Fatalf("readManifest = %+v, want %+v", got, want)
	}
}

func TestReadManifestRejectsBadManifests(t *testing.T) {
	cases := map[string]string{
		"missing slug": `{"schemaVersion":1,"title":"x"}`,
		"invalid slug": `{"slug":"Not A Slug"}`,
		"not json":     `{`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "manifest.json")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if got, err := readManifest(path); err == nil {
				t.Fatalf("readManifest = %+v, want an error", got)
			}
		})
	}
}

func TestCheckCartridgeDirWantsTheTemplateShape(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"manifest.json", "go.mod"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkCartridgeDir(dir, false); err != nil {
		t.Fatalf("checkCartridgeDir with -build wanted no Makefile: %v", err)
	}
	err := checkCartridgeDir(dir, true)
	if err == nil {
		t.Fatal("checkCartridgeDir passed with no Makefile, want an error")
	}
	if !strings.Contains(err.Error(), "Makefile") {
		t.Fatalf("error = %q, want it to name the missing Makefile", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "Makefile"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkCartridgeDir(dir, true); err != nil {
		t.Fatalf("checkCartridgeDir on a complete cartridge: %v", err)
	}

	if err := checkCartridgeDir(filepath.Join(dir, "Makefile"), true); err == nil {
		t.Fatal("checkCartridgeDir accepted a file, want an error")
	}
}

func TestSlugPattern(t *testing.T) {
	ok := []string{"orbit", "8bit", "maze", "a"}
	bad := []string{"", "Orbit", "-orbit", "light_out", "light out", "../etc"}
	for _, s := range ok {
		if !slugPattern.MatchString(s) {
			t.Errorf("slugPattern rejected valid slug %q", s)
		}
	}
	for _, s := range bad {
		if slugPattern.MatchString(s) {
			t.Errorf("slugPattern accepted invalid slug %q", s)
		}
	}
}

func TestCartridgeArgTakesThePositionalOrTheAlias(t *testing.T) {
	cases := []struct {
		name       string
		cartFlag   string
		positional []string
		want       string
		wantErr    bool
	}{
		{"positional", "", []string{"catcher"}, "catcher", false},
		{"alias", "catcher", nil, "catcher", false},
		{"both agree", "catcher", []string{"catcher"}, "catcher", false},
		{"both disagree", "catcher", []string{"other"}, "", true},
		{"neither", "", nil, "", true},
		{"two dirs", "", []string{"a", "b"}, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := cartridgeArg(c.cartFlag, c.positional)
			if (err != nil) != c.wantErr || got != c.want {
				t.Fatalf("cartridgeArg(%q, %q) = %q, %v; want %q, error %v", c.cartFlag, c.positional, got, err, c.want, c.wantErr)
			}
		})
	}
}

func TestParseInterspersedAllowsFlagsAfterTheDirectory(t *testing.T) {
	fs := flag.NewFlagSet("devserve", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8134", "")
	var origins originList
	fs.Var(&origins, "origin", "")

	got := parseInterspersed(fs, []string{"-origin", "http://localhost:5173", "catcher", "-addr", "127.0.0.1:0"})
	if len(got) != 1 || got[0] != "catcher" {
		t.Fatalf("positional = %q, want [catcher]", got)
	}
	if *addr != "127.0.0.1:0" {
		t.Fatalf("-addr = %q, want 127.0.0.1:0", *addr)
	}
	if len(origins) != 1 || origins[0] != "http://localhost:5173" {
		t.Fatalf("-origin = %q", origins)
	}
}

func TestNormalizeOrigin(t *testing.T) {
	ok := map[string]string{
		"https://Example.org":    "https://example.org",
		"http://localhost:5173/": "http://localhost:5173",
	}
	for in, want := range ok {
		got, err := normalizeOrigin(in)
		if err != nil || got != want {
			t.Errorf("normalizeOrigin(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"*", "null", "example.org", "ftp://example.org", "https://example.org/play", "https://example.org?x=1"} {
		if got, err := normalizeOrigin(in); err == nil {
			t.Errorf("normalizeOrigin(%q) = %q, want an error", in, got)
		}
	}
}
