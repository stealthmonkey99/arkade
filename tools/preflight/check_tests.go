// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"os/exec"
	"strings"
)

// checkTests runs the cartridge's own test suite, which already covers
// determinism and wasm-matches-source. The wasm rebuild depends on the local
// TinyGo version, so failure output is quoted verbatim.
func checkTests(c *Cart, r *Result) {
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = c.Dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		r.summarize("`go -C %s test ./...` passes", c.Rel(c.Dir))
		r.info(Loc{}, "%s", firstLines(string(out), 3))
		return
	}
	r.fail("docs/GAME_AUTHORING_AI.md 'Determinism & cross-host goldens'", c.At(c.Dir),
		"`go -C %s test ./...` failed: %v", c.Rel(c.Dir), err)
	for _, line := range failureLines(string(out)) {
		r.note(StatusFail, "", Loc{}, "  %s", line)
	}
	r.summarize("cartridge test suite FAILED")
}

// firstLines returns at most n non-empty lines of output.
func firstLines(s string, n int) string {
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
		if len(out) == n {
			break
		}
	}
	return strings.Join(out, "; ")
}

// maxTestOutputLines bounds how much failing test output goes on the card.
const maxTestOutputLines = 20

// failureLines drops passing-test chatter from `go test` output.
func failureLines(s string) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "ok ") || strings.HasPrefix(trimmed, "=== RUN") || strings.HasPrefix(trimmed, "--- PASS") || trimmed == "PASS" {
			continue
		}
		out = append(out, trimmed)
		if len(out) == maxTestOutputLines {
			out = append(out, "... (truncated; run the suite directly for the rest)")
			break
		}
	}
	return out
}
