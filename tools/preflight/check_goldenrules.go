// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"go/ast"
	"go/token"
	"strconv"
)

const (
	ruleNoInit      = "GOLDEN RULE 1: no func init() and no non-trivial package-level initializers — setup goes in Game.Init"
	ruleTimeFromDt  = "GOLDEN RULE 3: time comes only from Update's dtMicros, never a wall clock"
	ruleRandomU64   = "GOLDEN RULE 4: randomness comes only from cartlib.RandomU64 — anything else breaks replay determinism"
	ruleNoScheduler = "GOLDEN RULE 5: no goroutines, channels, time or sync — -scheduler=none makes a goroutine a build failure"
	ruleNoReflect   = "GOLDEN RULE 6: avoid fmt/reflect/encoding/json on any path (binary size + reflection under TinyGo)"
	ruleNoLibm      = "GOLDEN RULE 8: libm transcendentals risk cross-host divergence — keep them off observable per-frame paths"
)

// bannedImports maps import paths that fail the build, trap at runtime or
// break replay determinism to the rule forbidding them.
var bannedImports = map[string]string{
	"time":         ruleNoScheduler,
	"sync":         ruleNoScheduler,
	"sync/atomic":  ruleNoScheduler,
	"math/rand":    ruleRandomU64,
	"math/rand/v2": ruleRandomU64,
	"crypto/rand":  ruleRandomU64,
}

// discouragedImports is rule 6's set: costly but not fatal, so they warn.
var discouragedImports = map[string]string{
	"fmt":           ruleNoReflect,
	"reflect":       ruleNoReflect,
	"encoding/json": ruleNoReflect,
}

// libmTranscendentals lists math functions not guaranteed bit-identical
// between TinyGo's libm and host Go. Sqrt and the exact functions (Abs,
// Floor, Ceil, Trunc, Mod, Max, Min) are left out.
var libmTranscendentals = map[string]bool{
	"Pow": true, "Sin": true, "Cos": true, "Tan": true,
	"Asin": true, "Acos": true, "Atan": true, "Atan2": true,
	"Exp": true, "Exp2": true, "Log": true, "Log2": true, "Log10": true,
	"Sinh": true, "Cosh": true, "Tanh": true, "Cbrt": true, "Hypot": true,
}

// checkGoldenRules applies every golden rule that syntax can decide to the
// files TinyGo compiles. Rule 7 (no allocation after Init) is not checked.
func checkGoldenRules(c *Cart, r *Result) {
	srcs, err := c.Sources()
	if err != nil {
		r.fail("", Loc{}, "parsing cartridge sources: %v", err)
		return
	}
	if len(srcs) == 0 {
		r.skip("no TinyGo-compiled .go files in %s", c.Rel(c.Dir))
		return
	}

	syms := packageSymbols(srcs)
	libmSites := 0
	for _, src := range srcs {
		checkImports(c, r, src)
		checkTopLevelDecls(c, r, src, syms)
		libmSites += checkExpressions(c, r, src)
	}
	if libmSites > 0 {
		// Whether a call is per-frame is not visible syntactically, so report a count.
		r.info(Loc{}, "%d libm transcendental call site(s) — tolerable one-off, not per-frame (%s)", libmSites, ruleNoLibm)
	}

	fails, warns := r.counts()
	switch {
	case fails > 0:
		r.summarize("%d violation(s) across %d TinyGo-compiled files", fails, len(srcs))
	case warns > 0:
		r.summarize("%d TinyGo-compiled files, %d to look at", len(srcs), warns)
	default:
		r.summarize("%d TinyGo-compiled files clean", len(srcs))
	}
}

// checkImports enforces the rules decidable from the import list.
func checkImports(c *Cart, r *Result, src *SourceFile) {
	for _, spec := range src.File.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		if rule, banned := bannedImports[path]; banned {
			r.fail(rule, c.Pos(spec.Pos()), "imports %q", path)
			continue
		}
		if rule, discouraged := discouragedImports[path]; discouraged {
			r.warn(rule, c.Pos(spec.Pos()), "imports %q — confirm it is off the steady-state path", path)
		}
	}
}

// checkTopLevelDecls enforces rule 1: no init(), an empty main(), and no
// package-level var whose initializer does work.
func checkTopLevelDecls(c *Cart, r *Result, src *SourceFile, syms packageScope) {
	for _, decl := range src.File.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv != nil {
				continue
			}
			switch d.Name.Name {
			case "init":
				r.fail(ruleNoInit, c.Pos(d.Pos()), "declares func init() — move this into Game.Init, which the host calls explicitly")
			case "main":
				if d.Body != nil && len(d.Body.List) > 0 {
					r.fail(ruleNoInit, c.Pos(d.Body.Pos()),
						"func main() has %d statement(s); it must stay empty — cartlib.Run registers the game at package level", len(d.Body.List))
				}
			}
		case *ast.GenDecl:
			checkPackageVars(c, r, d, syms)
		}
	}
}

// checkPackageVars flags a package-level var whose initializer calls a
// function. A cartridge is one package, so a conversion can be told from a
// call by the package's own declarations (packageScope). A cross-package
// call could be either and warns; `var _ = cartlib.Run(game)` is allowed.
func checkPackageVars(c *Cart, r *Result, d *ast.GenDecl, syms packageScope) {
	if d.Tok != token.VAR {
		return
	}
	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, val := range vs.Values {
			call, ok := val.(*ast.CallExpr)
			if !ok {
				continue
			}
			switch classifyPackageInit(call, syms) {
			case initFolds:
				continue
			case initRuns:
				r.fail(ruleNoInit, c.Pos(call.Pos()),
					"package-level var calls %s() at init time — %s is a function in this package, so this is setup; move it into Game.Init",
					exprString(call.Fun), exprString(call.Fun))
			default:
				r.warn(ruleNoInit, c.Pos(call.Pos()),
					"package-level var evaluates %s() at init time — if that is not a compile-time-foldable conversion, move it into Game.Init",
					exprString(call.Fun))
			}
		}
	}
}

type initKind int

const (
	initUnknown initKind = iota // cross-package: could be either
	initFolds                   // a conversion or the registration line
	initRuns                    // a function call in this package
)

func classifyPackageInit(call *ast.CallExpr, syms packageScope) initKind {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		if pkg, ok := fun.X.(*ast.Ident); ok && pkg.Name == "cartlib" && fun.Sel.Name == "Run" {
			return initFolds
		}
		return initUnknown
	case *ast.ArrayType, *ast.StarExpr, *ast.ParenExpr, *ast.MapType, *ast.InterfaceType, *ast.StructType, *ast.ChanType, *ast.FuncType:
		return initFolds
	case *ast.Ident:
		switch {
		case syms.funcs[fun.Name] || runtimeBuiltins[fun.Name]:
			return initRuns
		case syms.types[fun.Name] || builtinTypes[fun.Name]:
			return initFolds
		}
		return initUnknown
	}
	return initUnknown
}

// runtimeBuiltins are the builtins that do work at init time. len and cap
// are left out because they fold over arrays and constants.
var runtimeBuiltins = map[string]bool{
	"make": true, "new": true, "append": true, "panic": true, "recover": true,
}

var builtinTypes = map[string]bool{
	"bool": true, "string": true, "byte": true, "rune": true, "error": true, "any": true,
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	"float32": true, "float64": true, "complex64": true, "complex128": true,
}

// packageScope is the cartridge package's top-level function and type names.
type packageScope struct {
	funcs map[string]bool
	types map[string]bool
}

func packageSymbols(srcs []*SourceFile) packageScope {
	scope := packageScope{funcs: map[string]bool{}, types: map[string]bool{}}
	for _, src := range srcs {
		for _, decl := range src.File.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					scope.funcs[d.Name.Name] = true
				}
			case *ast.GenDecl:
				if d.Tok != token.TYPE {
					continue
				}
				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						scope.types[ts.Name.Name] = true
					}
				}
			}
		}
	}
	return scope
}

// checkExpressions flags goroutines, channels and clock or rand
// calls, and returns the number of libm transcendental call sites.
func checkExpressions(c *Cart, r *Result, src *SourceFile) int {
	libm := 0
	seen := map[Loc]bool{}
	once := func(where Loc) bool {
		if seen[where] {
			return false
		}
		seen[where] = true
		return true
	}
	ast.Inspect(src.File, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.GoStmt:
			r.fail(ruleNoScheduler, c.Pos(node.Pos()), "go statement — a goroutine fails the -scheduler=none build")
		case *ast.ChanType:
			if where := c.Pos(node.Pos()); once(where) {
				r.fail(ruleNoScheduler, where, "channel type — channels need the scheduler that -scheduler=none removes")
			}
		case *ast.SelectStmt:
			r.fail(ruleNoScheduler, c.Pos(node.Pos()), "select statement — needs the scheduler that -scheduler=none removes")
		case *ast.CallExpr:
			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			switch pkg.Name {
			case "time":
				r.fail(ruleTimeFromDt, c.Pos(node.Pos()), "time.%s — accumulate Update's dtMicros instead", sel.Sel.Name)
			case "rand":
				r.fail(ruleRandomU64, c.Pos(node.Pos()), "rand.%s — use cartlib.RandomU64 (bounded: RandomU64() %% n)", sel.Sel.Name)
			case "math":
				if libmTranscendentals[sel.Sel.Name] {
					libm++
				}
			}
		}
		return true
	})
	return libm
}
