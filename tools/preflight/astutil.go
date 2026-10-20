// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"bytes"
	"go/ast"
	"go/printer"
	"go/token"
	"strconv"
	"strings"
)

// exprString renders an expression back to source for a finding's message.
func exprString(e ast.Expr) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, token.NewFileSet(), e); err != nil {
		return "?"
	}
	return buf.String()
}

// stringConsts collects every package-level const or var bound to a string
// literal, so a name passed to a check resolves one level deep. Anything
// computed or cross-package would need a type checker and is left out.
func stringConsts(srcs []*SourceFile) map[string]string {
	out := map[string]string{}
	for _, src := range srcs {
		for _, decl := range src.File.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || (gd.Tok != token.CONST && gd.Tok != token.VAR) {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || len(vs.Names) != len(vs.Values) {
					continue
				}
				for i, name := range vs.Names {
					if s, ok := literalString(vs.Values[i], nil); ok {
						out[name.Name] = s
					}
				}
			}
		}
	}
	return out
}

// literalString evaluates an expression to the string it definitely
// produces, resolving identifiers through consts when given. It reports
// ok=false as soon as any part is not statically known.
func literalString(e ast.Expr, consts map[string]string) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(v.Value)
		if err != nil {
			return "", false
		}
		return s, true
	case *ast.Ident:
		if consts == nil {
			return "", false
		}
		s, ok := consts[v.Name]
		return s, ok
	case *ast.ParenExpr:
		return literalString(v.X, consts)
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", false
		}
		l, lok := literalString(v.X, consts)
		r, rok := literalString(v.Y, consts)
		if !lok || !rok {
			return "", false
		}
		return l + r, true
	}
	return "", false
}

// literalFloor returns the text an expression certainly contains and whether
// any part is computed at runtime. Runtime pieces only make the string
// longer, so the floor is enough to prove an LCD line overflows.
func literalFloor(e ast.Expr, consts map[string]string) (text string, dynamic bool) {
	switch v := e.(type) {
	case *ast.ParenExpr:
		return literalFloor(v.X, consts)
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", true
		}
		l, ld := literalFloor(v.X, consts)
		r, rd := literalFloor(v.Y, consts)
		return l + r, ld || rd
	default:
		if s, ok := literalString(e, consts); ok {
			return s, false
		}
		return "", true
	}
}

// runeConsts collects every package-level const bound to a rune literal,
// resolving one level deep like stringConsts.
func runeConsts(srcs []*SourceFile) map[string]rune {
	out := map[string]rune{}
	for _, src := range srcs {
		for _, decl := range src.File.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || len(vs.Names) != len(vs.Values) {
					continue
				}
				for i, name := range vs.Names {
					if r, ok := literalRune(vs.Values[i], nil); ok {
						out[name.Name] = r
					}
				}
			}
		}
	}
	return out
}

// literalRune is literalString for a single rune: a character literal or an
// identifier bound to one in consts.
func literalRune(e ast.Expr, consts map[string]rune) (rune, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.CHAR {
			return 0, false
		}
		r, _, _, err := strconv.UnquoteChar(v.Value[1:len(v.Value)-1], '\'')
		if err != nil {
			return 0, false
		}
		return r, true
	case *ast.Ident:
		if consts == nil {
			return 0, false
		}
		r, ok := consts[v.Name]
		return r, ok
	case *ast.ParenExpr:
		return literalRune(v.X, consts)
	}
	return 0, false
}

// customRunes returns the non-ASCII runes in s, deduplicated, in first
// appearance order. The LCD pages these into its eight CGRAM slots; ASCII
// comes from the controller ROM.
func customRunes(s string) []rune {
	var out []rune
	seen := map[rune]bool{}
	for _, r := range s {
		if r < 0x80 || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}

// runeList renders a rune set for a finding's message.
func runeList(rs []rune) string {
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		parts = append(parts, strconv.QuoteRune(r))
	}
	return strings.Join(parts, " ")
}
