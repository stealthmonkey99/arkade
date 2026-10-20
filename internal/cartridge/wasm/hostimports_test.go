// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/api"
)

const cartlibDir = "../../../sdk/cartlib"

// cartlibImports reads every //go:wasmimport arkade declaration in
// sdk/cartlib, keyed by import name, as its Go parameter and result types.
func cartlibImports(t *testing.T) map[string][]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(cartlibDir, "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no Go files under %s: %v", cartlibDir, err)
	}
	out := map[string][]string{}
	fset := token.NewFileSet()
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, f, src, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Doc == nil {
				continue
			}
			for _, c := range fn.Doc.List {
				fields := strings.Fields(c.Text)
				if len(fields) != 3 || fields[0] != "//go:wasmimport" || fields[1] != "arkade" {
					continue
				}
				out[fields[2]] = signatureOf(t, fn.Type)
			}
		}
	}
	return out
}

func signatureOf(t *testing.T, ft *ast.FuncType) []string {
	var sig []string
	add := func(list *ast.FieldList, prefix string) {
		if list == nil {
			return
		}
		for _, field := range list.List {
			ident, ok := field.Type.(*ast.Ident)
			if !ok {
				t.Fatalf("wasmimport parameter type %T is not a plain integer type", field.Type)
			}
			n := max(len(field.Names), 1)
			for range n {
				sig = append(sig, prefix+ident.Name)
			}
		}
	}
	add(ft.Params, "")
	add(ft.Results, "-> ")
	return sig
}

var (
	contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
	moduleType  = reflect.TypeOf((*api.Module)(nil)).Elem()
)

// hostSignatureOf is a hostImport fn's wire signature: its parameters after
// the context and module, then its results.
func hostSignatureOf(fn any) []string {
	ft := reflect.TypeOf(fn)
	var sig []string
	for i := range ft.NumIn() {
		in := ft.In(i)
		if in == contextType || in == moduleType {
			continue
		}
		sig = append(sig, in.Kind().String())
	}
	for i := range ft.NumOut() {
		sig = append(sig, "-> "+ft.Out(i).Kind().String())
	}
	return sig
}

func TestHostImportsMatchCartlib(t *testing.T) {
	want := cartlibImports(t)
	if len(want) == 0 {
		t.Fatal("found no //go:wasmimport arkade declarations in sdk/cartlib")
	}
	br := newBridge(newStubChannelHost(), frameDims{}, Options{})
	got := map[string][]string{}
	for _, imp := range br.arkadeImports() {
		if _, dup := got[imp.name]; dup {
			t.Errorf("%s is registered twice", imp.name)
		}
		got[imp.name] = hostSignatureOf(imp.fn)
	}
	for name, sig := range want {
		host, ok := got[name]
		if !ok {
			t.Errorf("cartlib imports %s but the host does not register it", name)
			continue
		}
		if !reflect.DeepEqual(host, sig) {
			t.Errorf("%s: host signature %v, cartlib declares %v", name, host, sig)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok && !hostOnlyImports[name] {
			t.Errorf("the host registers %s but cartlib declares no import for it", name)
		}
	}
}

// hostOnlyImports are ABI 1 imports cartlib stopped declaring because its
// full-frame blit overwrites anything they draw. The host keeps them so a
// guest built without cartlib still links against the ABI the docs promise.
var hostOnlyImports = map[string]bool{
	"host_draw_pixel":  true,
	"host_fill_rect":   true,
	"host_load_sprite": true,
	"host_draw_sprite": true,
	"host_draw_cel":    true,
}
