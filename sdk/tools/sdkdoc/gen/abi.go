// SPDX-License-Identifier: GPL-2.0-or-later

package gen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/doc"
	"go/doc/comment"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// ABIPage is the hand-written ABI reference whose import and export tables
// sdkdoc generates, relative to sdk/docs.
const ABIPage = "abi.md"

// hostImportsDir holds the cabinet's import table, relative to the SDK root.
const hostImportsDir = "../internal/cartridge/wasm"

// importGroups titles the host's import table by the role variable that
// serves each group, in table order.
var importGroups = map[string]string{
	"draw":     "Display",
	"sheets":   "Sprite sheets",
	"sound":    "Audio",
	"store":    "Storage",
	"scores":   "Scores and recent names",
	"ctl":      "Players, feedback and lifecycle",
	"lcd":      "LCDs",
	"lamps":    "Button lamps",
	"channels": "Input channels",
	"session":  "Environment",
}

// exportContracts names the cartlib symbol whose doc is each export's
// contract. An export missing here takes its contract from the doc comment
// on its //export function.
var exportContracts = map[string]string{
	"cart_abi_version":  "AbiVersion",
	"cart_manifest":     "Game.Manifest",
	"cart_init":         "Game.Init",
	"cart_handle_input": "Game.HandleInput",
	"cart_update":       "Game.Update",
	"cart_render":       "Game.Render",
	"cart_teardown":     "Teardowner.Teardown",
}

// optionalExports are the exports a cartridge declares itself; cartlib
// supplies the dispatch, not the //export.
var optionalExports = []struct{ name, sig, symbol string }{
	{"cart_attract", "cart_attract(dtMicros u64)", "Attractor.Attract"},
}

// abiRow is one line of a generated table.
type abiRow struct {
	name, sig  string
	wrappers   []string // cartlib symbols, as Recv.Name or Name
	contract   string
	unwrapped  bool
	contractOf string // symbol the contract came from, for errors
}

// abiTables renders the generated export and import sections of abi.md.
func abiTables(sdkRoot string, cartlib *loaded, ix symbolIndex) (exports, imports string, err error) {
	dir := filepath.Join(sdkRoot, "cartlib")
	decls, err := parseDir(dir, func(name string) bool {
		return name == "abi_tinygo.go" || name == "host_tinygo.go" || name == "exports_tinygo.go"
	})
	if err != nil {
		return "", "", err
	}
	symDocs := symbolDocs(cartlib.doc)
	link := func(sym string) string {
		if target, ok := ix[cartlib.importPath][sym]; ok {
			return fmt.Sprintf("[%s](api/%s)", sym, target)
		}
		return "`" + sym + "`"
	}

	sentence := contracts{&pageWriter{l: cartlib, ix: ix, parser: cartlib.parser(ix), linkPrefix: "api/"}}.sentence
	exports, err = exportTable(decls, symDocs, link, sentence)
	if err != nil {
		return "", "", err
	}

	declared := wasmImports(decls)  // host_x -> hostX and its type
	viaMethod := hostMethods(decls) // hostX -> wasmHost method
	// go/doc dropped the bodies from cartlib's own AST, so parse it again.
	hostFiles, err := parseDir(dir, func(name string) bool {
		ok, _ := hostContext.MatchFile(dir, name)
		return ok && !strings.HasSuffix(name, "_test.go")
	})
	if err != nil {
		return "", "", err
	}
	wrappers := wrapperIndex(hostFiles, cartlib.doc)
	served, err := servedImports(filepath.Join(sdkRoot, filepath.FromSlash(hostImportsDir)))
	if err != nil {
		return "", "", err
	}
	servedSet := map[string]bool{}
	for _, s := range served {
		servedSet[s.name] = true
	}
	var missing []string
	for name := range declared {
		if !servedSet[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return "", "", fmt.Errorf("cartlib declares imports the cabinet does not serve: %s", strings.Join(missing, ", "))
	}

	var b strings.Builder
	group := ""
	for _, s := range served {
		if s.group != group {
			group = s.group
			title, ok := importGroups[group]
			if !ok {
				return "", "", fmt.Errorf("hostimports.go: role %q has no title in sdkdoc's importGroups", group)
			}
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "### %s\n\n| Import | Signature | cartlib | Contract |\n| --- | --- | --- | --- |\n", title)
		}
		row := abiRow{name: s.name}
		if d, ok := declared[s.name]; ok {
			row.sig = wireSignature(s.name, d.Type)
			row.wrappers = wrappers[viaMethod[d.Name.Name]]
			if len(row.wrappers) == 0 {
				return "", "", fmt.Errorf("%s: no exported cartlib function reaches it", s.name)
			}
			row.contractOf = row.wrappers[0]
			row.contract = symDocs[row.wrappers[0]]
		} else {
			row.unwrapped = true
			row.sig = wireSignature(s.name, s.handler.Type)
			row.contractOf = "the host handler " + s.handler.Name.Name
			if s.handler.Doc != nil {
				row.contract = s.handler.Doc.Text()
			}
		}
		line, err := row.render(sentence, link)
		if err != nil {
			return "", "", err
		}
		b.WriteString(line)
	}
	return exports, b.String(), nil
}

func (r abiRow) render(sentence func(string) string, link func(string) string) (string, error) {
	contract := sentence(r.contract)
	if contract == "" {
		return "", fmt.Errorf("%s: %s has no doc comment to take its contract from", r.name, r.contractOf)
	}
	if strings.Contains(contract, "|") {
		return "", fmt.Errorf("%s: the contract sentence from %s contains a '|', which breaks the table", r.name, r.contractOf)
	}
	via := "not wrapped"
	if !r.unwrapped {
		var links []string
		for _, w := range r.wrappers {
			links = append(links, link(w))
		}
		via = strings.Join(links, ", ")
	}
	return fmt.Sprintf("| `%s` | `%s` | %s | %s |\n", r.name, r.sig, via, contract), nil
}

func exportTable(files []*ast.File, symDocs map[string]string, link, sentence func(string) string) (string, error) {
	var b strings.Builder
	b.WriteString("| Export | Signature | cartlib | Contract |\n| --- | --- | --- | --- |\n")
	type export struct {
		name string
		fn   *ast.FuncDecl
	}
	var exports []export
	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Doc == nil {
				continue
			}
			for _, c := range fn.Doc.List {
				if name, ok := strings.CutPrefix(c.Text, "//export "); ok {
					exports = append(exports, export{strings.TrimSpace(name), fn})
				}
			}
		}
	}
	if len(exports) == 0 {
		return "", fmt.Errorf("cartlib: no //export functions found")
	}
	for _, e := range exports {
		sym, contract := exportContracts[e.name], ""
		via := "—"
		if sym != "" {
			doc, ok := symDocs[sym]
			if !ok {
				return "", fmt.Errorf("%s: sdkdoc maps it to %s, which cartlib does not document", e.name, sym)
			}
			contract, via = doc, link(sym)
		} else {
			contract = docWithoutDirectives(e.fn.Doc)
		}
		contract = sentence(contract)
		if contract == "" {
			return "", fmt.Errorf("%s: no contract (map it in sdkdoc's exportContracts or document its //export function)", e.name)
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | %s | %s |\n", e.name, wireSignature(e.name, e.fn.Type), via, contract)
	}
	for _, o := range optionalExports {
		doc, ok := symDocs[o.symbol]
		if !ok {
			return "", fmt.Errorf("%s: sdkdoc maps it to %s, which cartlib does not document", o.name, o.symbol)
		}
		fmt.Fprintf(&b, "| `%s` (optional) | `%s` | %s | %s |\n", o.name, o.sig, link(o.symbol), sentence(doc))
	}
	return b.String(), nil
}

// contracts renders an ABI table's contract cell.
type contracts struct{ w *pageWriter }

// sentence is the first sentence of a doc comment, its links resolved for
// abi.md. A comment on unexported source (a //export function, a host
// handler) loses its leading identifier, and "x is host_y:" with it.
func (c contracts) sentence(text string) string {
	text = strings.TrimSpace(text)
	if first, rest, ok := strings.Cut(text, " "); ok && !ast.IsExported(first) && token.IsIdentifier(first) {
		if _, after, ok := strings.Cut(rest, ": "); ok && strings.HasPrefix(rest, "is ") && !strings.Contains(rest[:len(rest)-len(after)], ".") {
			rest = after
		}
		if strings.HasPrefix(rest, "it ") {
			rest = strings.TrimPrefix(rest, "it ")
		}
		text = strings.ToUpper(rest[:1]) + rest[1:]
	}
	for _, b := range c.w.parser.Parse(text).Content {
		if p, ok := b.(*comment.Paragraph); ok {
			return firstSentence(c.w.text(p.Text))
		}
	}
	return ""
}

// firstSentence cuts s after its first period that is followed by a space
// and not preceded by a lone capital, as go/doc's Synopsis does.
func firstSentence(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] != '.' || (i+1 < len(s) && s[i+1] != ' ') {
			continue
		}
		if i >= 1 && s[i-1] >= 'A' && s[i-1] <= 'Z' && (i < 2 || s[i-2] == ' ') {
			continue
		}
		return s[:i+1]
	}
	return strings.TrimSpace(s)
}

func docWithoutDirectives(g *ast.CommentGroup) string {
	if g == nil {
		return ""
	}
	return g.Text() // Text drops //export and other directive lines
}

// wireSignature writes fn's parameters and results with wasm value types.
func wireSignature(name string, fn *ast.FuncType) string {
	var params []string
	for _, f := range fn.Params.List {
		t := wasmType(f.Type)
		if t == "" {
			continue // context.Context and api.Module on host handlers
		}
		var names []string
		for _, n := range f.Names {
			names = append(names, n.Name)
		}
		params = append(params, strings.Join(names, ", ")+" "+t)
	}
	sig := name + "(" + strings.Join(params, ", ") + ")"
	if fn.Results != nil {
		var results []string
		for _, f := range fn.Results.List {
			n := max(1, len(f.Names))
			for range n {
				results = append(results, wasmType(f.Type))
			}
		}
		if len(results) == 1 {
			sig += " -> " + results[0]
		} else {
			sig += " -> (" + strings.Join(results, ", ") + ")"
		}
	}
	return sig
}

func wasmType(e ast.Expr) string {
	id, ok := e.(*ast.Ident)
	if !ok {
		return ""
	}
	return map[string]string{"uint32": "u32", "int32": "i32", "uint64": "u64", "int64": "i64", "float32": "f32", "float64": "f64"}[id.Name]
}

func parseDir(dir string, keep func(name string) bool) ([]*ast.File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || !keep(e.Name()) {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

// wasmImports maps each //go:wasmimport arkade name to its declaration.
func wasmImports(files []*ast.File) map[string]*ast.FuncDecl {
	out := map[string]*ast.FuncDecl{}
	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Doc == nil {
				continue
			}
			for _, c := range fn.Doc.List {
				if rest, ok := strings.CutPrefix(c.Text, "//go:wasmimport arkade "); ok {
					out[strings.TrimSpace(rest)] = fn
				}
			}
		}
	}
	return out
}

// hostMethods maps each wasm import function to the wasmHost method that
// calls it.
func hostMethods(files []*ast.File) map[string]string {
	out := map[string]string{}
	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if id, ok := call.Fun.(*ast.Ident); ok && strings.HasPrefix(id.Name, "host") {
						out[id.Name] = fn.Name.Name
					}
				}
				return true
			})
		}
	}
	return out
}

// wrapperIndex maps each host method (host.M) to the exported cartlib
// functions that call it, directly or through one unexported helper, in
// source order.
func wrapperIndex(files []*ast.File, d *doc.Package) map[string][]string {
	documented := symbolDocs(d)
	type fnInfo struct {
		key   string
		calls []string // host methods called directly
		uses  []string // package functions called
	}
	var fns []fnInfo
	for _, f := range files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			info := fnInfo{key: funcKey(fn)}
			recvName, recvType := "", ""
			if fn.Recv != nil && len(fn.Recv.List[0].Names) > 0 {
				recvName = fn.Recv.List[0].Names[0].Name
				recvType, _, _ = strings.Cut(info.key, ".")
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch fun := call.Fun.(type) {
				case *ast.SelectorExpr:
					x, ok := fun.X.(*ast.Ident)
					switch {
					case ok && x.Name == "host":
						info.calls = append(info.calls, fun.Sel.Name)
					case ok && recvName != "" && x.Name == recvName:
						info.uses = append(info.uses, recvType+"."+fun.Sel.Name)
					}
				case *ast.Ident:
					info.uses = append(info.uses, fun.Name)
				}
				return true
			})
			fns = append(fns, info)
		}
	}
	direct := map[string][]string{} // unexported func -> host methods
	for _, fn := range fns {
		if !ast.IsExported(lastPart(fn.key)) {
			direct[fn.key] = fn.calls
		}
	}
	out, indirect := map[string][]string{}, map[string][]string{}
	add := func(into map[string][]string, method, key string) {
		if !slices.Contains(into[method], key) {
			into[method] = append(into[method], key)
		}
	}
	for _, fn := range fns {
		if _, ok := documented[fn.key]; !ok {
			continue
		}
		for _, m := range fn.calls {
			add(out, m, fn.key)
		}
		for _, u := range fn.uses {
			for _, m := range direct[u] {
				add(indirect, m, fn.key)
			}
		}
	}
	for m, keys := range indirect {
		if len(out[m]) == 0 {
			out[m] = keys
		}
	}
	// A host call made only from an unexported method, such as the frame
	// blit, is attributed to the method's exported type.
	for _, fn := range fns {
		recv, _, isMethod := strings.Cut(fn.key, ".")
		if _, ok := documented[recv]; !isMethod || !ok {
			continue
		}
		for _, m := range fn.calls {
			if len(out[m]) == 0 {
				out[m] = []string{recv}
			}
		}
	}
	return out
}

func lastPart(key string) string {
	if i := strings.LastIndexByte(key, '.'); i >= 0 {
		return key[i+1:]
	}
	return key
}

// funcKey is Name or Recv.Name, pointer and type parameters dropped.
func funcKey(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	t := fn.Recv.List[0].Type
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	switch x := t.(type) {
	case *ast.IndexExpr:
		t = x.X
	case *ast.IndexListExpr:
		t = x.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

// symbolDocs maps every documented exported symbol of d, interface methods
// included, to its doc text.
func symbolDocs(d *doc.Package) map[string]string {
	out := map[string]string{}
	values := func(vs []*doc.Value) {
		for _, v := range vs {
			for _, n := range v.Names {
				out[n] = valueDoc(v, n)
			}
		}
	}
	values(d.Consts)
	values(d.Vars)
	for _, f := range d.Funcs {
		out[f.Name] = f.Doc
	}
	for _, t := range d.Types {
		out[t.Name] = t.Doc
		values(t.Consts)
		values(t.Vars)
		for _, f := range t.Funcs {
			out[f.Name] = f.Doc
		}
		for _, m := range t.Methods {
			out[t.Name+"."+m.Name] = m.Doc
		}
		for _, s := range t.Decl.Specs {
			ts, ok := s.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if it, ok := ts.Type.(*ast.InterfaceType); ok {
				for _, m := range it.Methods.List {
					for _, n := range m.Names {
						out[t.Name+"."+n.Name] = m.Doc.Text()
					}
				}
			}
		}
	}
	return out
}

// valueDoc is the doc for one name of a const or var group: the spec's own
// comment, else the group's.
func valueDoc(v *doc.Value, name string) string {
	for _, s := range v.Decl.Specs {
		vs := s.(*ast.ValueSpec)
		for _, n := range vs.Names {
			if n.Name == name && vs.Doc != nil {
				return vs.Doc.Text()
			}
		}
	}
	return v.Doc
}

// servedImport is one entry of the cabinet's import table.
type servedImport struct {
	name, group string
	handler     *ast.FuncDecl
}

// servedImports reads the cabinet's "arkade" import table in order, with
// each import's handler method.
func servedImports(dir string) ([]servedImport, error) {
	files, err := parseDir(dir, func(name string) bool { return !strings.HasSuffix(name, "_test.go") })
	if err != nil {
		return nil, err
	}
	methods := map[string]*ast.FuncDecl{} // Type.method
	fields := map[string]string{}         // bridge field -> type
	var table *ast.FuncDecl
	for _, f := range files {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Name.Name == "arkadeImports" {
					table = d
				}
				if d.Recv != nil {
					methods[funcKey(d)] = d
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					ts, ok := s.(*ast.TypeSpec)
					if !ok || ts.Name.Name != "bridge" {
						continue
					}
					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						continue
					}
					for _, fl := range st.Fields.List {
						for _, n := range fl.Names {
							fields[n.Name] = typeName(fl.Type)
						}
					}
				}
			}
		}
	}
	if table == nil {
		return nil, fmt.Errorf("%s: no arkadeImports table", dir)
	}
	roleType := map[string]string{}
	var out []servedImport
	ast.Inspect(table.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range n.Lhs {
				id, ok := lhs.(*ast.Ident)
				if !ok || i >= len(n.Rhs) {
					continue
				}
				switch r := n.Rhs[i].(type) {
				case *ast.CompositeLit:
					roleType[id.Name] = typeName(r.Type)
				case *ast.SelectorExpr:
					roleType[id.Name] = fields[r.Sel.Name]
				}
			}
		case *ast.CompositeLit:
			if len(n.Elts) != 2 {
				return true
			}
			lit, ok := n.Elts[0].(*ast.BasicLit)
			sel, ok2 := n.Elts[1].(*ast.SelectorExpr)
			if !ok || !ok2 || lit.Kind != token.STRING {
				return true
			}
			role, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			name, _ := strconv.Unquote(lit.Value)
			out = append(out, servedImport{name: name, group: role.Name, handler: methods[roleType[role.Name]+"."+sel.Sel.Name]})
		}
		return true
	})
	for _, s := range out {
		if s.handler == nil {
			return nil, fmt.Errorf("%s: cannot find the handler method for %s", dir, s.name)
		}
	}
	if len(out) < 40 {
		return nil, fmt.Errorf("%s: read only %d imports from arkadeImports; has the table changed shape?", dir, len(out))
	}
	return out, nil
}

func typeName(e ast.Expr) string {
	if s, ok := e.(*ast.StarExpr); ok {
		e = s.X
	}
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// spliceABI replaces each generated section of abi.md, between the
// <!-- sdkdoc:NAME --> and <!-- /sdkdoc:NAME --> markers, with its body.
func spliceABI(page []byte, sections map[string]string) ([]byte, error) {
	out := page
	names := make([]string, 0, len(sections))
	for n := range sections {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		open := []byte("<!-- sdkdoc:" + name + " -->\n")
		close := []byte("<!-- /sdkdoc:" + name + " -->")
		i := bytes.Index(out, open)
		j := bytes.Index(out, close)
		if i < 0 || j < i {
			return nil, fmt.Errorf("%s: no %s ... %s markers", ABIPage, strings.TrimSpace(string(open)), close)
		}
		var b bytes.Buffer
		b.Write(out[:i+len(open)])
		b.WriteString("\n" + sections[name] + "\n")
		b.Write(out[j:])
		out = b.Bytes()
	}
	return out, nil
}
