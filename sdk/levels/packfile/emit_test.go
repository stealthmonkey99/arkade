// SPDX-License-Identifier: GPL-2.0-or-later

package packfile_test

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/levels/packfile"
)

func TestVarName(t *testing.T) {
	for _, tc := range []struct{ pkg, pack, want string }{
		{"main", "solo", "soloPack"},
		{"main", "bonus-rooms", "bonusRoomsPack"},
		{"main", "bonus_rooms", "bonusRoomsPack"},
		{"example", "demo", "DemoPack"},
		{"example", "night-shift", "NightShiftPack"},
	} {
		if got := packfile.VarName(tc.pkg, tc.pack); got != tc.want {
			t.Errorf("VarName(%q, %q) = %q, want %q", tc.pkg, tc.pack, got, tc.want)
		}
	}
}

func generate(t *testing.T, dir, pkg string) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "levels_gen.go")
	if _, err := packfile.Generate(packfile.Config{Dir: dir, Out: out, Pkg: pkg}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	src, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func TestExamplePackIsUpToDate(t *testing.T) {
	got := generate(t, filepath.Join("..", "example"), "example")
	want, err := os.ReadFile(filepath.Join("..", "example", "levels_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("sdk/levels/example/levels_gen.go is stale — regenerate it:\n"+
			"\tgo run ./tools/genlevelpack -dir sdk/levels/example -out sdk/levels/example/levels_gen.go -pkg example\n\ngot:\n%s", got)
	}
}

func TestEmitIsDeterministic(t *testing.T) {
	a := generate(t, "testdata", "main")
	b := generate(t, "testdata", "main")
	if !bytes.Equal(a, b) {
		t.Error("two runs over the same directory produced different output")
	}
}

func TestGeneratedSourceIsValidGo(t *testing.T) {
	src := generate(t, "testdata", "main")
	if _, err := parser.ParseFile(token.NewFileSet(), "levels_gen.go", src, parser.AllErrors); err != nil {
		t.Fatalf("generated source does not parse: %v\n%s", err, src)
	}
	text := string(src)
	puzzle, solo := strings.Index(text, "puzzlePack"), strings.Index(text, "soloPack")
	if puzzle < 0 || solo < 0 || puzzle > solo {
		t.Errorf("packs are not in filename order (packing_puzzle before pushing_solo): %d, %d", puzzle, solo)
	}
	if !strings.Contains(text, "\"#.$  #\" +") {
		t.Error("grid rows are not emitted one per line")
	}
}

func TestGenerateRejectsDuplicatePackNames(t *testing.T) {
	dir := t.TempDir()
	src := "pack solo\nlegend\n  # wall\nlevel ONE\n  |#|\n"
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(dir, name+packfile.Ext), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, err := packfile.Generate(packfile.Config{Dir: dir, Out: filepath.Join(dir, "levels_gen.go"), Pkg: "main"})
	if err == nil {
		t.Fatal("Generate accepted two packs with the same name")
	}
	if !strings.Contains(err.Error(), "already declared by a.levels") {
		t.Errorf("error = %v, want it to name the file that declared it first", err)
	}
}

func TestGenerateRejectsEmptyDir(t *testing.T) {
	if _, err := packfile.Generate(packfile.Config{Dir: t.TempDir(), Out: filepath.Join(t.TempDir(), "x.go")}); err == nil {
		t.Fatal("Generate succeeded on a directory with no pack files")
	}
}

func TestGenerateRequiresDirAndOut(t *testing.T) {
	if _, err := packfile.Generate(packfile.Config{Out: "x.go"}); err == nil {
		t.Error("Generate succeeded with no Dir")
	}
	if _, err := packfile.Generate(packfile.Config{Dir: "testdata"}); err == nil {
		t.Error("Generate succeeded with no Out")
	}
}

func TestGenerateWritesNothingOnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad"+packfile.Ext), []byte("pack a\nlegend\n  # wall\nlevel ONE\n  |#Q#|\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "levels_gen.go")
	if err := os.WriteFile(out, []byte("// previous\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := packfile.Generate(packfile.Config{Dir: dir, Out: out, Pkg: "main"}); err == nil {
		t.Fatal("Generate succeeded on an unparseable pack")
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "// previous\n" {
		t.Errorf("output file was overwritten despite the error:\n%s", got)
	}
}
