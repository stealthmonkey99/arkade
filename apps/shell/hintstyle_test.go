// SPDX-License-Identifier: GPL-2.0-or-later

package shell_test

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

// The cabinet's buttons carry no letters, so a string literal that labels a
// button by letter fails. Scanning source catches screens with no render test.
var letterHintPatterns = []*regexp.Regexp{
	// "A=PLAY", "D = GO", "B: BACK"
	regexp.MustCompile(`(^|[^A-Za-z0-9])[ABCD]\s*[=:]\s*[A-Za-z]`),
	// "A SET  D SWAP"
	regexp.MustCompile(`(^|[^A-Za-z0-9])[ABCD] [A-Z]{2,}`),
}

// hintStyleDirs are the apps that draw cabinet UI chrome.
var hintStyleDirs = []string{".", "../config"}

func TestNoLetteredButtonHints(t *testing.T) {
	for _, dir := range hintStyleDirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("no Go source found in %s (err=%v) — this guard is only as good as its reach", dir, err)
		}
		for _, path := range files {
			checkFileForLetterHints(t, path)
		}
	}
}

func checkFileForLetterHints(t *testing.T, path string) {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
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
		where := fset.Position(lit.Pos())
		if len(text) == 1 && strings.ContainsAny(text, "ABCD") {
			t.Errorf(`%s: bare button letter %q — draw the button's colour instead (ui.Hint / ui.ButtonDot)`, where, text)
			return true
		}
		for _, re := range letterHintPatterns {
			if re.MatchString(text) {
				t.Errorf(`%s: lettered button hint in %q — the physical buttons carry no letters; use a coloured disc on the panel, or name the colour on the LCD`, where, text)
				return true
			}
		}
		return true
	})
}
