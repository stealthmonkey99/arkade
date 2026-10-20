// SPDX-License-Identifier: GPL-2.0-or-later

package gen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/doc"
	"go/doc/comment"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
)

// The pages are rendered by hand from the parsed comment rather than with
// comment.Printer.Markdown: that printer backslash-escapes backticks and
// emits tab-indented code blocks, and the docs site's renderer has neither
// escapes nor indented code, so its output shows up there as literal
// backslashes and run-together code.

// headingID is the anchor GitHub and the docs site both derive from a
// heading: lowercase, with only letters, digits, hyphens and spaces kept.
func headingID(heading string) string {
	heading = strings.NewReplacer("`", "", "*", "", "_", "").Replace(heading)
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-':
			b.WriteByte('-')
		}
	}
	return b.String()
}

func typeHeading(name string) string { return "type " + name }

func funcHeading(recv, name string) string {
	if recv == "" {
		return "func " + name
	}
	return "func (" + recv + ") " + name
}

// symbolIndex resolves a doc link to a page and anchor: import path, then
// "Name" or "Recv.Name", to "page.md#anchor".
type symbolIndex map[string]map[string]string

func (ix symbolIndex) add(l *loaded) error {
	syms := map[string]string{"": l.Page}
	seen := map[string]string{}
	put := func(key, anchor string) error {
		syms[key] = l.Page + "#" + anchor
		return nil
	}
	heading := func(h, key string) error {
		id := headingID(h)
		if prev, dup := seen[id]; dup {
			return fmt.Errorf("%s: %s and %s share the anchor #%s", l.importPath, prev, key, id)
		}
		seen[id] = key
		return put(key, id)
	}
	values := func(vs []*doc.Value, anchor string) {
		for _, v := range vs {
			for _, n := range v.Names {
				put(n, anchor)
			}
		}
	}
	values(l.doc.Consts, "constants")
	values(l.doc.Vars, "variables")
	for _, f := range l.doc.Funcs {
		if err := heading(funcHeading("", f.Name), f.Name); err != nil {
			return err
		}
	}
	for _, t := range l.doc.Types {
		if err := heading(typeHeading(t.Name), t.Name); err != nil {
			return err
		}
		anchor := headingID(typeHeading(t.Name))
		values(t.Consts, anchor)
		values(t.Vars, anchor)
		for _, name := range memberNames(t) {
			put(t.Name+"."+name, anchor)
		}
		for _, f := range t.Funcs {
			if err := heading(funcHeading("", f.Name), f.Name); err != nil {
				return err
			}
		}
		for _, m := range t.Methods {
			if err := heading(funcHeading(m.Recv, m.Name), t.Name+"."+m.Name); err != nil {
				return err
			}
		}
	}
	ix[l.importPath] = syms
	return nil
}

// memberNames lists a type's struct field and interface method names.
func memberNames(t *doc.Type) []string {
	var out []string
	for _, s := range t.Decl.Specs {
		ts, ok := s.(*ast.TypeSpec)
		if !ok {
			continue
		}
		var list *ast.FieldList
		switch x := ts.Type.(type) {
		case *ast.StructType:
			list = x.Fields
		case *ast.InterfaceType:
			list = x.Methods
		}
		if list == nil {
			continue
		}
		for _, f := range list.List {
			for _, n := range f.Names {
				out = append(out, n.Name)
			}
		}
	}
	return out
}

// parser is the package's doc-comment parser with [Type.Member] links to
// struct fields and interface methods resolved from the index. Go 1.23's
// go/doc does not resolve those itself and newer releases do; the pages
// must read the same whichever toolchain renders them.
func (l *loaded) parser(ix symbolIndex) *comment.Parser {
	p := l.doc.Parser()
	p.LookupSym = memberLookup(p.LookupSym, ix[l.importPath])
	return p
}

func memberLookup(base func(recv, name string) (ok bool), syms map[string]string) func(recv, name string) (ok bool) {
	return func(recv, name string) bool {
		if base(recv, name) {
			return true
		}
		_, ok := syms[recv+"."+name]
		return recv != "" && ok
	}
}

// pageWriter renders one package's page.
type pageWriter struct {
	l          *loaded
	ix         symbolIndex
	parser     *comment.Parser
	linkPrefix string // prepended to links into the reference
	buf        bytes.Buffer
}

func (w *pageWriter) printf(format string, args ...any) { fmt.Fprintf(&w.buf, format, args...) }

// linkURL is the href for a doc link: a page and anchor inside the
// reference, pkg.go.dev for anything outside it.
func (w *pageWriter) linkURL(l *comment.DocLink) string {
	path := l.ImportPath
	if path == "" {
		path = w.l.importPath
	}
	key := l.Name
	if l.Recv != "" {
		key = l.Recv + "." + l.Name
	}
	if syms, ok := w.ix[path]; ok {
		if target, ok := syms[key]; ok {
			return w.linkPrefix + target
		}
		return ""
	}
	url := "https://pkg.go.dev/" + path
	if key != "" {
		url += "#" + key
	}
	return url
}

// comment writes a doc comment, its own headings at headingLevel.
func (w *pageWriter) comment(text string, headingLevel int) {
	d := w.parser.Parse(text)
	for _, b := range d.Content {
		w.block(b, headingLevel)
		w.buf.WriteByte('\n')
	}
}

func (w *pageWriter) block(b comment.Block, headingLevel int) {
	switch b := b.(type) {
	case *comment.Paragraph:
		w.buf.WriteString(w.text(b.Text))
		w.buf.WriteByte('\n')
	case *comment.Heading:
		w.printf("%s %s\n", strings.Repeat("#", headingLevel), w.text(b.Text))
	case *comment.Code:
		w.fence(codeTag(b.Text), b.Text)
	case *comment.List:
		for i, item := range b.Items {
			if i > 0 && b.BlankBetween() {
				w.buf.WriteByte('\n')
			}
			marker := "-"
			if item.Number != "" {
				marker = item.Number + "."
			}
			var parts []string
			for _, c := range item.Content {
				parts = append(parts, w.text(c.(*comment.Paragraph).Text))
			}
			w.printf("%s %s\n", marker, strings.Join(parts, " "))
		}
	}
}

func (w *pageWriter) text(xs []comment.Text) string {
	var b strings.Builder
	for _, x := range xs {
		switch x := x.(type) {
		case comment.Plain:
			b.WriteString(strings.ReplaceAll(string(x), "\n", " "))
		case comment.Italic:
			b.WriteString("*" + strings.ReplaceAll(string(x), "\n", " ") + "*")
		case *comment.Link:
			fmt.Fprintf(&b, "[%s](%s)", w.text(x.Text), x.URL)
		case *comment.DocLink:
			label := w.text(x.Text)
			if url := w.linkURL(x); url != "" {
				fmt.Fprintf(&b, "[%s](%s)", label, url)
			} else {
				b.WriteString(label)
			}
		}
	}
	return b.String()
}

func (w *pageWriter) fence(tag, body string) {
	w.printf("```%s\n%s\n```\n", tag, strings.TrimRight(body, "\n"))
}

// codeTag is the fence tag for a comment's code block: Go when it parses as
// Go, text otherwise. Neither is ever copied or compiled as a whole file.
func codeTag(src string) string {
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "", "package p\n"+src, 0); err == nil {
		return "go fragment"
	}
	if _, err := parser.ParseFile(fset, "", "package p\nfunc _() {\n"+src+"\n}", 0); err == nil {
		return "go fragment"
	}
	return "text"
}

// node prints a declaration with the comments inside it.
func (w *pageWriter) node(n ast.Node) string {
	var comments []*ast.CommentGroup
	for _, f := range w.l.files {
		for _, c := range f.Comments {
			if c.Pos() >= n.Pos() && c.End() <= n.End() {
				comments = append(comments, c)
			}
		}
	}
	var b bytes.Buffer
	cfg := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}
	if err := cfg.Fprint(&b, w.l.fset, &printer.CommentedNode{Node: n, Comments: comments}); err != nil {
		return fmt.Sprintf("// %v", err)
	}
	return b.String()
}

func (w *pageWriter) decl(n ast.Node) { w.fence("go fragment", w.node(n)) }

// examples writes each example's doc, code and expected output.
func (w *pageWriter) examples(exs []*doc.Example) {
	for _, ex := range exs {
		title := "Example"
		if ex.Suffix != "" {
			title += " (" + strings.ReplaceAll(ex.Suffix, "_", " ") + ")"
		}
		w.printf("**%s**\n\n", title)
		if ex.Doc != "" {
			w.comment(ex.Doc, 6)
		}
		w.fence("go", exampleCode(w.l.fset, ex))
		w.buf.WriteByte('\n')
		if ex.Output != "" || ex.EmptyOutput {
			out := "Output:"
			if ex.Unordered {
				out = "Unordered output:"
			}
			w.printf("%s\n\n", out)
			w.fence("text", ex.Output)
			w.buf.WriteByte('\n')
		}
	}
}

// exampleCode is an example's body with its braces and one level of
// indentation removed, or the whole file for a whole-file example.
func exampleCode(fset *token.FileSet, ex *doc.Example) string {
	var b bytes.Buffer
	cfg := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}
	cfg.Fprint(&b, fset, &printer.CommentedNode{Node: ex.Code, Comments: ex.Comments})
	src := b.String()
	if _, whole := ex.Code.(*ast.File); whole {
		var keep []string
		inOutput := false
		for _, line := range strings.Split(src, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "// Output:" || trimmed == "// Unordered output:" {
				inOutput = true
				continue
			}
			if inOutput && strings.HasPrefix(trimmed, "//") {
				continue
			}
			inOutput = false
			if strings.HasPrefix(line, "// SPDX-License-Identifier:") {
				continue
			}
			keep = append(keep, line)
		}
		return strings.TrimSpace(strings.Join(keep, "\n"))
	}
	src = strings.TrimSpace(src)
	src = strings.TrimSuffix(strings.TrimPrefix(src, "{"), "}")
	var lines []string
	for _, line := range strings.Split(strings.Trim(src, "\n"), "\n") {
		lines = append(lines, strings.TrimPrefix(line, "\t"))
	}
	out := strings.Join(lines, "\n")
	if i := strings.Index(out, "\n// Output:"); i >= 0 {
		out = out[:i]
	} else if strings.HasPrefix(out, "// Output:") {
		out = ""
	}
	if i := strings.Index(out, "\n// Unordered output:"); i >= 0 {
		out = out[:i]
	}
	return strings.TrimSpace(out)
}

// page renders the whole reference page for l. hostOnly and tinygoOnly
// name the exported symbols present in only one build of the package.
func (w *pageWriter) page(hostOnly, tinygoOnly []string) []byte {
	d := w.l.doc
	w.printf("# Package %s\n\n", w.l.Dir)
	w.printf("```go fragment\nimport %q\n```\n\n", w.l.importPath)
	w.printf("Generated by `sdkdoc` from the package's doc comments, the same text `go doc` and pkg.go.dev show. Edit the Go source, then run `make sdk-docs`.\n\n")
	if len(hostOnly) > 0 || len(tinygoOnly) > 0 {
		w.printf("This page documents the plain-Go (`!tinygo`) build, which a cartridge's `go test` compiles.")
		if len(hostOnly) > 0 {
			w.printf(" Only in that build: %s.", w.symbolList(hostOnly))
		}
		if len(tinygoOnly) > 0 {
			w.printf(" Only in the TinyGo build: %s.", strings.Join(tinygoOnly, ", "))
		}
		w.printf("\n\n")
	}

	w.printf("## Overview\n\n")
	w.comment(d.Doc, 3)
	w.examples(d.Examples)

	if len(d.Consts) > 0 {
		w.printf("## Constants\n\n")
		w.values(d.Consts)
	}
	if len(d.Vars) > 0 {
		w.printf("## Variables\n\n")
		w.values(d.Vars)
	}
	if len(d.Funcs) > 0 {
		w.printf("## Functions\n\n")
		for _, f := range d.Funcs {
			w.function(f, 3)
		}
	}
	if len(d.Types) > 0 {
		w.printf("## Types\n\n")
		for _, t := range d.Types {
			w.printf("### %s\n\n", typeHeading(t.Name))
			w.decl(t.Decl)
			w.buf.WriteByte('\n')
			w.comment(t.Doc, 5)
			w.examples(t.Examples)
			w.values(t.Consts)
			w.values(t.Vars)
			for _, f := range t.Funcs {
				w.function(f, 4)
			}
			for _, m := range t.Methods {
				w.function(m, 4)
			}
		}
	}
	return bytes.TrimRight(w.buf.Bytes(), "\n")
}

func (w *pageWriter) symbolList(names []string) string {
	var out []string
	for _, n := range names {
		recv, name, isMethod := strings.Cut(n, ".")
		link := &comment.DocLink{Name: n}
		if isMethod {
			link = &comment.DocLink{Recv: recv, Name: name}
		}
		if url := w.linkURL(link); url != "" {
			out = append(out, fmt.Sprintf("[%s](%s)", n, url))
		} else {
			out = append(out, "`"+n+"`")
		}
	}
	return strings.Join(out, ", ")
}

func (w *pageWriter) values(vs []*doc.Value) {
	for _, v := range vs {
		w.decl(v.Decl)
		w.buf.WriteByte('\n')
		w.comment(v.Doc, 5)
	}
}

func (w *pageWriter) function(f *doc.Func, level int) {
	w.printf("%s %s\n\n", strings.Repeat("#", level), funcHeading(f.Recv, f.Name))
	w.decl(f.Decl)
	w.buf.WriteByte('\n')
	w.comment(f.Doc, 5)
	w.examples(f.Examples)
}
