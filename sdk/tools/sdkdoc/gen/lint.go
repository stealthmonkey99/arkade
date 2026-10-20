// SPDX-License-Identifier: GPL-2.0-or-later

package gen

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/token"
	"path"
	"sort"
	"strings"
)

// Lint checks the SDK's doc comments against the conventions in
// sdk/docs/AUTHORING.md and returns one "file:line: problem" string per
// miss, sorted; none means the SDK conforms. Both builds of every
// [Packages] entry are checked, so a symbol that exists only under TinyGo
// is held to the same rules. The rules:
//
//   - every package has a package doc beginning "Package <name>", or
//     "Command <name>" for a [Commands] entry;
//   - every exported function, method, type and interface method has a doc
//     comment whose first word is its name ("A", "An" or "The" may precede
//     a type's name);
//   - an exported const or var declared alone has a doc that begins with
//     its name, and a group declaring several has a doc on the group or on
//     every exported name in it;
//   - an exported struct field has a doc or a trailing comment.
func Lint(sdkRoot string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	report := func(fset *token.FileSet, pos token.Pos, format string, args ...any) {
		p := fset.Position(pos)
		msg := fmt.Sprintf("%s:%d: %s", relPath(sdkRoot, p.Filename), p.Line, fmt.Sprintf(format, args...))
		if !seen[msg] {
			seen[msg] = true
			out = append(out, msg)
		}
	}
	for _, p := range Packages {
		for _, ctx := range []bool{false, true} {
			c := hostContext
			if ctx {
				c = tinygoContext
			}
			l, err := load(sdkRoot, p, c)
			if err != nil {
				return nil, err
			}
			lintPackage(l, "Package "+l.doc.Name, report)
		}
	}
	for _, dir := range Commands {
		l, err := load(sdkRoot, Package{Dir: dir}, hostContext)
		if err != nil {
			return nil, err
		}
		want := "Command " + path.Base(dir)
		if !strings.HasPrefix(l.doc.Doc, want+" ") {
			report(l.fset, l.files[0].Package, "the package doc must begin %q", want)
		}
	}
	sort.Strings(out)
	return out, nil
}

type reporter func(fset *token.FileSet, pos token.Pos, format string, args ...any)

func lintPackage(l *loaded, want string, report reporter) {
	d, fset := l.doc, l.fset
	if !strings.HasPrefix(d.Doc, want+" ") {
		report(fset, packagePos(l), "the package doc must begin %q", want)
	}
	lintValues(fset, d.Consts, report)
	lintValues(fset, d.Vars, report)
	for _, f := range d.Funcs {
		lintFunc(fset, f, report)
	}
	for _, t := range d.Types {
		if !startsWithName(t.Doc, t.Name, true) {
			report(fset, t.Decl.Pos(), "type %s: %s", t.Name, docProblem(t.Doc, t.Name))
		}
		lintValues(fset, t.Consts, report)
		lintValues(fset, t.Vars, report)
		for _, f := range t.Funcs {
			lintFunc(fset, f, report)
		}
		for _, m := range t.Methods {
			if m.Level == 0 { // promoted methods are documented where they are declared
				lintFunc(fset, m, report)
			}
		}
		lintMembers(fset, t, report)
	}
}

// packagePos is where the package doc belongs: the package clause of the
// file that has the doc, or of the first file.
func packagePos(l *loaded) token.Pos {
	for _, f := range l.files {
		if f.Doc != nil {
			return f.Package
		}
	}
	return l.files[0].Package
}

func lintFunc(fset *token.FileSet, f *doc.Func, report reporter) {
	name := f.Name
	if f.Recv != "" {
		name = strings.TrimPrefix(f.Recv, "*") + "." + f.Name
	}
	if !startsWithName(f.Doc, f.Name, false) {
		report(fset, f.Decl.Pos(), "func %s: %s", name, docProblem(f.Doc, f.Name))
	}
}

func lintValues(fset *token.FileSet, vs []*doc.Value, report reporter) {
	for _, v := range vs {
		var exported []*ast.Ident
		allDocumented := true
		for _, s := range v.Decl.Specs {
			spec := s.(*ast.ValueSpec)
			for _, n := range spec.Names {
				if n.IsExported() {
					exported = append(exported, n)
					if spec.Doc == nil && spec.Comment == nil {
						allDocumented = false
					}
				}
			}
		}
		if len(exported) == 0 {
			continue
		}
		kind := v.Decl.Tok.String()
		if len(exported) == 1 && !v.Decl.Lparen.IsValid() {
			if n := exported[0].Name; !startsWithName(v.Doc, n, false) {
				report(fset, v.Decl.Pos(), "%s %s: %s", kind, n, docProblem(v.Doc, n))
			}
			continue
		}
		if strings.TrimSpace(v.Doc) == "" && !allDocumented {
			var names []string
			for _, n := range exported {
				names = append(names, n.Name)
			}
			report(fset, v.Decl.Pos(), "%s group (%s): needs a doc on the group or on each name", kind, strings.Join(names, ", "))
		}
	}
}

// lintMembers checks an exported type's interface methods and struct
// fields.
func lintMembers(fset *token.FileSet, t *doc.Type, report reporter) {
	for _, s := range t.Decl.Specs {
		ts, ok := s.(*ast.TypeSpec)
		if !ok {
			continue
		}
		switch x := ts.Type.(type) {
		case *ast.InterfaceType:
			for _, m := range x.Methods.List {
				for _, n := range m.Names {
					if n.IsExported() && !startsWithName(m.Doc.Text(), n.Name, false) {
						report(fset, n.Pos(), "interface method %s.%s: %s", t.Name, n.Name, docProblem(m.Doc.Text(), n.Name))
					}
				}
			}
		case *ast.StructType:
			for _, f := range x.Fields.List {
				if f.Doc != nil || f.Comment != nil {
					continue
				}
				for _, n := range f.Names {
					if n.IsExported() {
						report(fset, n.Pos(), "field %s.%s: no doc or trailing comment", t.Name, n.Name)
					}
				}
			}
		}
	}
}

func startsWithName(text, name string, article bool) bool {
	text = strings.TrimSpace(text)
	if article {
		for _, a := range []string{"A ", "An ", "The "} {
			if rest, ok := strings.CutPrefix(text, a); ok {
				text = rest
				break
			}
		}
	}
	rest, ok := strings.CutPrefix(text, name)
	return ok && (rest == "" || !isIdentRune(rest[0]))
}

func isIdentRune(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func docProblem(text, name string) string {
	if strings.TrimSpace(text) == "" {
		return "no doc comment"
	}
	first, _, _ := strings.Cut(strings.TrimSpace(text), " ")
	return fmt.Sprintf("the doc must begin with %q, not %q", name, first)
}

func relPath(root, file string) string {
	if i := strings.Index(file, "/sdk/"); i >= 0 {
		return file[i+1:]
	}
	return file
}
