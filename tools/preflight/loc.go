// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"fmt"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
)

// Loc is where a finding points: one file, and a line in it when the
// finding has one. The zero Loc is a finding about the cartridge as a whole.
type Loc struct {
	// Path is the file relative to the scan root, slash-separated, as the
	// report prints it.
	Path string
	// Cart is the same file relative to the cartridge directory, or "" when
	// the file lies outside it.
	Cart string
	// Line is 1-based; 0 for a finding about the whole file.
	Line int
}

func (l Loc) String() string {
	if l.Line == 0 {
		return l.Path
	}
	return fmt.Sprintf("%s:%d", l.Path, l.Line)
}

// At locates a whole file by its absolute path.
func (c *Cart) At(abs string) Loc {
	l := Loc{Path: c.Rel(abs)}
	if rel, err := filepath.Rel(c.Dir, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		l.Cart = filepath.ToSlash(rel)
	}
	return l
}

// Pos locates a parsed source position, or returns the zero Loc if it is
// unknown.
func (c *Cart) Pos(p token.Pos) Loc {
	if c.fset == nil || !p.IsValid() {
		return Loc{}
	}
	pos := c.fset.Position(p)
	l := c.At(pos.Filename)
	l.Line = pos.Line
	return l
}

// suppressWhere is a suppression's "where" parsed once: a file, either
// relative to the scan root or to the cartridge (optionally under a
// "<root dir>/<slug>/" prefix), and an optional line.
type suppressWhere struct {
	file string
	line int
}

func parseWhere(s string) suppressWhere {
	if i := strings.LastIndex(s, ":"); i >= 0 {
		if n, err := strconv.Atoi(s[i+1:]); err == nil && n > 0 {
			return suppressWhere{file: s[:i], line: n}
		}
	}
	return suppressWhere{file: s}
}

// covers reports whether w names l's file, and l's line when w gives one.
// cartPrefix is the "<root dir>/<slug>/" a cartridge-relative where may
// carry.
func (w suppressWhere) covers(l Loc, cartPrefix string) bool {
	if w.line != 0 && w.line != l.Line {
		return false
	}
	if w.file == l.Path {
		return true
	}
	return l.Cart != "" && strings.TrimPrefix(w.file, cartPrefix) == l.Cart
}
