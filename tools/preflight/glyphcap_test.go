// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/lcdrune"
)

// TestCartlibGlyphCapIsLcdrunes pins the one copy of the cap that cannot
// import lcdrune: sdk/cartlib is TinyGo-only and its own module.
func TestCartlibGlyphCapIsLcdrunes(t *testing.T) {
	if n := intConst(t, "../../sdk/cartlib/glyphs.go", "MaxCartGlyphs"); n != lcdrune.MaxCartGlyphs {
		t.Errorf("cartlib.MaxCartGlyphs = %d, lcdrune.MaxCartGlyphs = %d", n, lcdrune.MaxCartGlyphs)
	}
}

func TestCGRAMSlotsMatchTheLCDDriver(t *testing.T) {
	path := filepath.Join(repoRoot(t), glyphTablePath)
	if n := intConst(t, path, "maxCGRAMSlots"); n != MaxCGRAMSlots {
		t.Errorf("pi maxCGRAMSlots = %d, preflight MaxCGRAMSlots = %d", n, MaxCGRAMSlots)
	}
}

// intConst reads the integer literal a top-level const in path is bound to.
func intConst(t *testing.T, path, name string) int {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, id := range vs.Names {
				if id.Name != name || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok {
					t.Fatalf("%s in %s is not a literal", name, path)
				}
				n, err := strconv.Atoi(lit.Value)
				if err != nil {
					t.Fatalf("%s in %s: %v", name, path, err)
				}
				return n
			}
		}
	}
	t.Fatalf("no %s const in %s", name, path)
	return 0
}
