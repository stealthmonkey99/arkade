// SPDX-License-Identifier: GPL-2.0-or-later

// Package gen renders the SDK's API reference, sdk/docs/api/*.md, from the
// SDK packages' doc comments, and generates the import and export tables in
// sdk/docs/abi.md from cartlib's wasm declarations and the cabinet host's
// import table. It also holds the conformance rules the SDK's doc comments
// are held to ([Lint]). Command sdkdoc is its command line.
package gen

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A Package is one SDK package the reference covers.
type Package struct {
	// Dir is the package directory relative to the SDK root (sdk/).
	Dir string
	// Page is the generated page's file name under sdk/docs/api.
	Page string
}

// Packages lists the SDK packages in the reference, in sidebar order.
var Packages = []Package{
	{"cartlib", "cartlib.md"},
	{"cartlib/hosttest", "cartlib-hosttest.md"},
	{"ui", "ui.md"},
	{"ui/input", "ui-input.md"},
	{"fsm", "fsm.md"},
	{"levels", "levels.md"},
	{"levels/packfile", "levels-packfile.md"},
}

// Commands lists the SDK's command directories, relative to the SDK root.
// [Lint] holds their package docs to the rules; they get no page.
var Commands = []string{
	"tools/manifestgen",
	"tools/rcartpack",
	"tools/sdkdoc",
}

// loaded is one package parsed under one build configuration.
type loaded struct {
	Package
	importPath string
	fset       *token.FileSet
	files      []*ast.File // non-test files, kept for comment lookup
	doc        *doc.Package
}

var (
	hostContext = func() build.Context {
		c := build.Default
		c.BuildTags = nil
		c.GOOS, c.GOARCH = "linux", "amd64"
		return c
	}()
	tinygoContext = func() build.Context {
		c := build.Default
		c.BuildTags = []string{"tinygo"}
		c.GOOS, c.GOARCH = "wasip1", "wasm"
		return c
	}()
)

// load parses the package in sdkRoot/p.Dir under ctx, test files included
// so its examples attach to their symbols.
func load(sdkRoot string, p Package, ctx build.Context) (*loaded, error) {
	dir := filepath.Join(sdkRoot, filepath.FromSlash(p.Dir))
	importPath, err := importPathOf(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	var all, nonTest []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		if ok, err := ctx.MatchFile(dir, name); err != nil || !ok {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		all = append(all, f)
		if !strings.HasSuffix(name, "_test.go") {
			nonTest = append(nonTest, f)
		}
	}
	if len(nonTest) == 0 {
		return nil, fmt.Errorf("%s: no Go files", dir)
	}
	d, err := doc.NewFromFiles(fset, all, importPath)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	return &loaded{Package: p, importPath: importPath, fset: fset, files: nonTest, doc: d}, nil
}

// importPathOf derives dir's import path from the nearest go.mod above it.
func importPathOf(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for d := abs; ; d = filepath.Dir(d) {
		raw, err := os.ReadFile(filepath.Join(d, "go.mod"))
		if err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				if mod, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
					rel, err := filepath.Rel(d, abs)
					if err != nil {
						return "", err
					}
					if rel == "." {
						return mod, nil
					}
					return mod + "/" + filepath.ToSlash(rel), nil
				}
			}
			return "", fmt.Errorf("%s/go.mod has no module line", d)
		}
		if filepath.Dir(d) == d {
			return "", fmt.Errorf("no go.mod above %s", dir)
		}
	}
}

// exportedSurface lists every exported name d documents, methods as
// Recv.Name, sorted.
func exportedSurface(d *doc.Package) []string {
	var out []string
	values := func(vs []*doc.Value) {
		for _, v := range vs {
			out = append(out, v.Names...)
		}
	}
	values(d.Consts)
	values(d.Vars)
	for _, f := range d.Funcs {
		out = append(out, f.Name)
	}
	for _, t := range d.Types {
		out = append(out, t.Name)
		values(t.Consts)
		values(t.Vars)
		for _, f := range t.Funcs {
			out = append(out, f.Name)
		}
		for _, m := range t.Methods {
			out = append(out, t.Name+"."+m.Name)
		}
	}
	sort.Strings(out)
	return out
}

// onlyIn returns the names in a missing from b.
func onlyIn(a, b []string) []string {
	have := map[string]bool{}
	for _, n := range b {
		have[n] = true
	}
	var out []string
	for _, n := range a {
		if !have[n] {
			out = append(out, n)
		}
	}
	return out
}
