// SPDX-License-Identifier: GPL-2.0-or-later

package main

import "testing"

func TestTinyGoSourceEvaluatesConstraints(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want bool
		why  string
	}{
		{"plain.go", "package main\n", true, "no constraint means it is in the build"},
		{"plain_test.go", "package main\n", false, "test files run under host Go, never TinyGo"},
		{"host.go", "//go:build !tinygo\n\npackage main\n", false, "a host-side rules file's shape"},
		{"guest.go", "//go:build tinygo\n\npackage main\n", true, "explicitly TinyGo-only"},
		{"branding.go", "//go:build !variant\n\npackage main\n", true, "a variant-build split stays in"},
		{"variant.go", "//go:build variant\n\npackage main\n", false, "the variant build is a separate build"},
		{"late.go", "package main\n\n//go:build !tinygo\n", true, "a constraint after the package clause is just a comment"},
	} {
		if got := TinyGoSource(tc.name, []byte(tc.src)); got != tc.want {
			t.Errorf("%s: got %v, want %v (%s)", tc.name, got, tc.want, tc.why)
		}
	}
}
