// SPDX-License-Identifier: GPL-2.0-or-later

package carttest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var letterHintPatterns = []*regexp.Regexp{
	// Case-insensitive on "press" only, or prose like "Press a light" would match.
	regexp.MustCompile(`(?i:\bPRESS)\s+[ABCD]\b`),
	regexp.MustCompile(`(^|[^A-Za-z0-9])[ABCD]\s*[=:]\s*[A-Za-z]`),
}

var spacedLegend = regexp.MustCompile(`(^|[^A-Za-z0-9])([ABCD]) [A-Z]{2,}`)

// isSpacedLegend needs two different letters: one reads as the English
// article ("FOUR IN A ROW"), and so does a repeat ("A CAT IS NOT A PET").
func isSpacedLegend(text string) bool {
	seen := map[string]bool{}
	for _, m := range spacedLegend.FindAllStringSubmatch(text, -1) {
		seen[m[2]] = true
		if len(seen) >= 2 {
			return true
		}
	}
	return false
}

func isLetteredHint(text string) bool {
	if isSpacedLegend(text) {
		return true
	}
	for _, re := range letterHintPatterns {
		if re.MatchString(text) {
			return true
		}
	}
	return false
}

// NoLetteredButtonHints fails t for every string literal, in the non-test Go
// files of each directory in dirs (the current directory when dirs is
// empty), that names a cabinet button by letter: "PRESS A", "A=PLAY",
// "B: BACK", "A SET  D SWAP". The cabinet's buttons carry colours, not
// letters, so such a prompt names a control the player cannot find. Scanning
// source rather than frames also covers screens no golden reaches. There is
// no exemption list; a string that needs one is the wrong pattern. A
// directory with no Go source is itself a failure, so a mistyped path cannot
// pass silently.
func NoLetteredButtonHints(t testing.TB, dirs ...string) {
	t.Helper()
	if len(dirs) == 0 {
		dirs = []string{"."}
	}
	for _, dir := range dirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("no Go source in %s (err=%v): this guard is only as good as its reach", dir, err)
			continue
		}
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			checkFileForLetterHints(t, path)
		}
	}
}

func checkFileForLetterHints(t testing.TB, path string) {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
		return
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
		return
	}
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		text, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		if isLetteredHint(text) {
			t.Errorf(`%s: lettered button hint in %q — the cabinet's buttons carry no letters. On the panel draw a disc (ui.ButtonDot); on the LCD name the action and light the lamp`,
				fset.Position(lit.Pos()), text)
		}
		return true
	})
}
