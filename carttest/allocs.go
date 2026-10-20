// SPDX-License-Identifier: GPL-2.0-or-later

package carttest

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// allocSite matches one line of TinyGo's -print-allocs report.
var allocSite = regexp.MustCompile(`^(.+\.go):(\d+):\d+: object allocated on the heap: (.*)$`)

// RequireNoAllocsAfterInit builds srcDir with TinyGo's -print-allocs and
// fails on every heap allocation site in the cartridge's own source that
// sits outside the functions named in initOnly. Under -gc=leaking nothing is
// ever freed, so an allocation reachable from Update, Render, HandleInput
// or Attract leaks for the life of the cabinet process.
//
// initOnly names the functions that run only from Init, spelled "name" for
// a plain function and "Type.name" for a method. Package-level initializers
// run once at boot and are always allowed. Allocations inside the SDK and
// the TinyGo runtime are not judged here: the report cannot attribute them
// to a caller.
//
// Skips when TinyGo is not on PATH.
func RequireNoAllocsAfterInit(t *testing.T, srcDir string, initOnly ...string) {
	t.Helper()
	tinygoPath, err := exec.LookPath("tinygo")
	if err != nil {
		t.Skip("tinygo not on PATH; skipping the post-Init allocation check")
	}
	abs, err := filepath.Abs(srcDir)
	if err != nil {
		t.Fatal(err)
	}
	args := append([]string{"build"}, tinyGoCartridgeBuildArgs...)
	args = append(args, "-print-allocs=.", "-o", filepath.Join(t.TempDir(), "allocs.wasm"), ".")
	cmd := exec.Command(tinygoPath, args...)
	cmd.Dir = abs
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tinygo build -print-allocs in %s: %v\n%s", abs, err, out)
	}

	allowed := map[string]bool{}
	for _, name := range initOnly {
		allowed[name] = true
	}
	funcs := map[string]*fileFuncs{}
	seen := map[string]bool{}
	var bad []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		m := allocSite.FindStringSubmatch(strings.TrimSpace(sc.Text()))
		if m == nil {
			continue
		}
		file, reason := m[1], m[3]
		rel, err := filepath.Rel(abs, file)
		if err != nil || strings.HasPrefix(rel, "..") || strings.HasSuffix(file, "_test.go") {
			continue
		}
		line, _ := strconv.Atoi(m[2])
		ff, ok := funcs[file]
		if !ok {
			if ff, err = parseFileFuncs(file); err != nil {
				t.Fatalf("parse %s: %v", file, err)
			}
			funcs[file] = ff
		}
		fn := ff.enclosing(line)
		if fn == "" || allowed[fn] {
			continue
		}
		site := fmt.Sprintf("%s:%d in %s: %s", rel, line, fn, reason)
		if !seen[site] {
			seen[site] = true
			bad = append(bad, site)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		t.Errorf("%d heap allocation site(s) outside Init in %s; under -gc=leaking each one leaks for the life of the process:\n  %s\n"+
			"Move the value to package scope or reset it in place; name the function in initOnly only if it truly runs only from Init.",
			len(bad), abs, strings.Join(bad, "\n  "))
	}
}

type funcSpan struct {
	name       string
	start, end int
}

type fileFuncs struct{ spans []funcSpan }

func parseFileFuncs(path string) (*fileFuncs, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	ff := &fileFuncs{}
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		name := fd.Name.Name
		if fd.Recv != nil && len(fd.Recv.List) > 0 {
			name = receiverName(fd.Recv.List[0].Type) + "." + name
		}
		ff.spans = append(ff.spans, funcSpan{name, fset.Position(fd.Pos()).Line, fset.Position(fd.End()).Line})
	}
	return ff, nil
}

func receiverName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StarExpr:
		return receiverName(x.X)
	case *ast.IndexExpr:
		return receiverName(x.X)
	case *ast.Ident:
		return x.Name
	}
	return "?"
}

// enclosing names the function declared around line, or "" for package
// scope.
func (ff *fileFuncs) enclosing(line int) string {
	for _, s := range ff.spans {
		if line >= s.start && line <= s.end {
			return s.name
		}
	}
	return ""
}
