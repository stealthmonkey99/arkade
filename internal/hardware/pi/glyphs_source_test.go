// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestCustomCharsSourceParsesLikeTheBrowserHost reads glyphs.go the way the
// browser host's LCD font parity test does (a line regex over the literal),
// so reformatting the table breaks here rather than silently there.
func TestCustomCharsSourceParsesLikeTheBrowserHost(t *testing.T) {
	src, err := os.ReadFile("glyphs.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	start := strings.Index(body, "customChars = map[rune]")
	if start < 0 {
		t.Fatal("glyphs.go no longer declares customChars = map[rune]...")
	}
	body = body[start:]
	end := strings.Index(body, "\n}")
	if end < 0 {
		t.Fatal("no closing \\n} after customChars")
	}
	entry := regexp.MustCompile(`(?m)^\s*'(.)':\s*\{([^}]*)\}`)
	parsed := map[rune][8]byte{}
	for _, m := range entry.FindAllStringSubmatch(body[:end], -1) {
		var rows [8]byte
		var n int
		for _, field := range strings.Split(m[2], ",") {
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			v, err := strconv.ParseUint(strings.TrimPrefix(field, "0x"), 16, 8)
			if err != nil || n >= len(rows) {
				t.Fatalf("rune %s: row %q is not one of eight hex bytes", m[1], field)
			}
			rows[n] = byte(v)
			n++
		}
		if n != len(rows) {
			t.Errorf("rune %s: parsed %d rows, want 8", m[1], n)
		}
		parsed[[]rune(m[1])[0]] = rows
	}
	if len(parsed) != len(customChars) {
		t.Errorf("line parser found %d runes, customChars has %d: keep one entry per line as 'r': {0x.., ...},", len(parsed), len(customChars))
	}
	for r, want := range customChars {
		if got, ok := parsed[r]; !ok || got != want {
			t.Errorf("rune %q: line parser read %v (found %v), want %v", r, got, ok, want)
		}
	}
}
