// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"go/build/constraint"
	"strings"
)

// TinyGoSource reports whether a cartridge source file is compiled into
// its wasm: not a test, and either unconstrained or with a //go:build line
// that holds when only the tinygo tag is set.
func TinyGoSource(name string, src []byte) bool {
	if strings.HasSuffix(name, "_test.go") {
		return false
	}
	expr, ok := buildConstraint(src)
	if !ok {
		return true
	}
	return expr.Eval(func(tag string) bool { return tag == "tinygo" })
}

// buildConstraint extracts a file's //go:build expression, if it has one
// before the package clause.
func buildConstraint(src []byte) (constraint.Expr, bool) {
	for _, line := range strings.Split(string(src), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "package ") {
			return nil, false
		}
		if !constraint.IsGoBuild(trimmed) {
			continue
		}
		expr, err := constraint.Parse(trimmed)
		if err != nil {
			return nil, false
		}
		return expr, true
	}
	return nil, false
}
