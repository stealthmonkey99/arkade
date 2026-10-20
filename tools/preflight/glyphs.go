// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
)

// glyphTablePath is parsed at runtime because customChars is unexported and a
// copy would drift.
var glyphTablePath = filepath.Join("internal", "hardware", "pi", "glyphs.go")

// MaxCGRAMSlots is the HD44780 limit. The budget applies per screen: its two
// lines may use at most this many distinct custom runes.
const MaxCGRAMSlots = 8

// loadGlyphTable returns the rune keys of pi's customChars map. The caller
// skips the unknown-rune check on error.
func loadGlyphTable(root string) (map[rune]bool, error) {
	path := filepath.Join(root, glyphTablePath)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", glyphTablePath, err)
	}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || vs.Names[0].Name != "customChars" || len(vs.Values) != 1 {
				continue
			}
			lit, ok := vs.Values[0].(*ast.CompositeLit)
			if !ok {
				continue
			}
			out := map[rune]bool{}
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.BasicLit)
				if !ok || key.Kind != token.CHAR {
					continue
				}
				if r, _, _, err := strconv.UnquoteChar(key.Value[1:len(key.Value)-1], '\''); err == nil {
					out[r] = true
				}
			}
			if len(out) == 0 {
				return nil, fmt.Errorf("%s: customChars parsed to an empty rune set", glyphTablePath)
			}
			return out, nil
		}
	}
	return nil, fmt.Errorf("%s: no `var customChars = map[rune][8]byte{...}` found", glyphTablePath)
}
