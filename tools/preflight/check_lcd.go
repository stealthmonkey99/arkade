// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"go/ast"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"unicode/utf8"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/lcdrune"
)

// LCDCols is the character LCD's width; longer lines are cut, not wrapped.
const LCDCols = 16

const (
	ruleLCDWidth       = "docs/CARTRIDGE.md: the panel LCD is 16x2 — text past column 16 is not shown at all"
	ruleCGRAM          = "internal/hardware/pi/glyphs.go: the HD44780 has 8 CGRAM slots, and the budget is per SCREEN and per MOMENT (each screen's two lines)"
	ruleGlyphSet       = "internal/hardware/pi/glyphs.go, sdk/cartlib/glyphs.go, docs/ASSETS.md: runes outside customChars, outside a Setup.DefineLCDGlyph definition, outside the manifest's assets.glyphs, and outside ASCII fall back to '?' on the cabinet"
	ruleBundledGlyph   = "docs/ASSETS.md: every path under the manifest's assets.glyphs must exist in the cartridge and pass assetfile validation — rcartpack refuses to build the bundle otherwise"
	ruleCartGlyphASCII = "sdk/cartlib/glyphs.go: DefineLCDGlyph rejects any code point below U+0080 — the HD44780 ROM already draws ASCII"
	ruleCartGlyphRune  = "internal/lcdrune: an LCD glyph rune must be a plain, single-width, monochrome character — never an emoji or an East Asian wide rune, which terminals draw in colour and two cells wide; both hosts refuse them"
	ruleCartGlyphCap   = "sdk/cartlib/glyphs.go: DefineLCDGlyph caps a cartridge at MaxCartGlyphs (32) distinct definitions"
)

// checkLCD checks every cartlib.SetText call for text that overflows 16
// columns or uses runes the chip cannot draw. Only statically known text is
// judged: for partly computed lines the literal floor still proves overflow.
// Fully runtime text is not visible here.
func checkLCD(c *Cart, r *Result) {
	srcs, err := c.Sources()
	if err != nil {
		r.fail("", Loc{}, "parsing cartridge sources: %v", err)
		return
	}
	if len(srcs) == 0 {
		r.skip("no TinyGo-compiled .go files in %s", c.Rel(c.Dir))
		return
	}

	engineGlyphs, glyphErr := loadGlyphTable(c.Root)
	consts := stringConsts(srcs)

	defineCalls := collectCartGlyphDefs(c, srcs, runeConsts(srcs))
	cartGlyphs := validateCartGlyphDefs(r, defineCalls)

	// Bundled glyphs reach the same table as DefineLCDGlyph, so they count as drawable.
	bundled := collectBundledGlyphs(c, r)
	for ru := range bundled {
		cartGlyphs[ru] = true
	}

	calls := 0
	cartRunes := map[rune]bool{}
	for _, src := range srcs {
		ast.Inspect(src.File, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isSetTextCall(call) || len(call.Args) < 3 {
				return true
			}
			calls++
			checkSetTextCall(c, r, call, consts, engineGlyphs, cartGlyphs, glyphErr == nil, cartRunes)
			return true
		})
	}

	if calls == 0 {
		if len(defineCalls) == 0 && len(bundled) == 0 {
			r.skip("no cartlib.SetText calls — this cartridge drives no LCD text")
			return
		}
		r.summarize("%d Setup.DefineLCDGlyph call(s) and %d bundled glyph(s), no cartlib.SetText calls", len(defineCalls), len(bundled))
		return
	}
	if glyphErr != nil {
		r.warn("", Loc{}, "could not read the cabinet glyph palette (%v) — unknown-rune checking skipped", glyphErr)
	}

	// Over eight custom runes across the cartridge is legal but makes eviction possible.
	if len(cartRunes) > MaxCGRAMSlots {
		r.warn(ruleCGRAM, Loc{}, "uses %d distinct custom glyphs across the cartridge (%s) — more than the %d CGRAM slots, so screens will page; confirm no single screen needs more than %d at once",
			len(cartRunes), runeList(sortedRunes(cartRunes)), MaxCGRAMSlots, MaxCGRAMSlots)
	}

	if fails, warns := r.counts(); fails+warns > 0 {
		r.summarize("%d SetText call(s), %d violation(s), %d to look at", calls, fails, warns)
	} else {
		r.summarize("%d SetText call(s) fit 16 columns and the CGRAM budget", calls)
	}
}

// isSetTextCall matches cartlib.SetText(player, line1, line2). SetTextBytes
// is not matched because it has no static text to measure.
func isSetTextCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "SetText" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "cartlib"
}

// checkSetTextCall judges one call's two line arguments. A custom rune
// passes if it is in the engine table or the cartridge's own glyphs.
func checkSetTextCall(c *Cart, r *Result, call *ast.CallExpr, consts map[string]string, engineGlyphs, cartGlyphs map[rune]bool, haveGlyphs bool, cartRunes map[rune]bool) {
	callRunes := map[rune]bool{}
	for i, arg := range call.Args[1:3] {
		text, dynamic := literalFloor(arg, consts)
		if text == "" {
			continue
		}
		where := c.Pos(arg.Pos())
		if n := utf8.RuneCountInString(text); n > LCDCols {
			if dynamic {
				r.fail(ruleLCDWidth, where, "line %d's literal text alone is %d columns (%q) before anything computed is appended — the tail is invisible on the cabinet", i+1, n, text)
			} else {
				r.fail(ruleLCDWidth, where, "line %d is %d columns: %q — everything past column %d is invisible on the cabinet", i+1, n, text, LCDCols)
			}
		}
		for _, ru := range customRunes(text) {
			callRunes[ru] = true
			cartRunes[ru] = true
			if haveGlyphs && !engineGlyphs[ru] && !cartGlyphs[ru] {
				if why := lcdrune.Reason(ru); why != "" {
					r.fail(ruleCartGlyphRune, where, "line %d uses %q, which is %s — no LCD glyph can be defined on it, so it renders as '?' on both hosts; pick a plain, single-width, monochrome character", i+1, ru, why)
					continue
				}
				r.fail(ruleGlyphSet, where, "line %d uses %q, which is not in the cabinet's CGRAM palette or this cartridge's own Setup.DefineLCDGlyph definitions — it renders as '?' (define it with Setup.DefineLCDGlyph, add it to internal/hardware/pi/glyphs.go, or pick a mapped rune)", i+1, ru)
			}
		}
	}
	// Both lines are on screen together, so their runes share the CGRAM budget.
	if len(callRunes) > MaxCGRAMSlots {
		r.fail(ruleCGRAM, c.Pos(call.Pos()), "this screen needs %d distinct custom glyphs at once (%s) — only %d CGRAM slots exist, so the LRU will evict and some cells will show the wrong glyph",
			len(callRunes), runeList(sortedRunes(callRunes)), MaxCGRAMSlots)
	}
}

// sortedRunes returns the set in ascending order.
func sortedRunes(set map[rune]bool) []rune {
	out := make([]rune, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// isDefineLCDGlyphCall matches a DefineLCDGlyph(r, rows) call on whatever
// name the cartridge gave the *cartlib.Setup its Init received.
func isDefineLCDGlyphCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "DefineLCDGlyph"
}

// cartGlyphDefCall is one Setup.DefineLCDGlyph call site; dynamic is set
// when the rune argument is not statically known.
type cartGlyphDefCall struct {
	where   Loc
	r       rune
	dynamic bool
}

// collectCartGlyphDefs finds every Setup.DefineLCDGlyph call, resolving the
// rune argument from a literal or a package-level const.
func collectCartGlyphDefs(c *Cart, srcs []*SourceFile, runeTable map[string]rune) []cartGlyphDefCall {
	var out []cartGlyphDefCall
	for _, src := range srcs {
		ast.Inspect(src.File, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isDefineLCDGlyphCall(call) || len(call.Args) < 1 {
				return true
			}
			where := c.Pos(call.Pos())
			if ru, ok := literalRune(call.Args[0], runeTable); ok {
				out = append(out, cartGlyphDefCall{where: where, r: ru})
			} else {
				out = append(out, cartGlyphDefCall{where: where, dynamic: true})
			}
			return true
		})
	}
	return out
}

// validateCartGlyphDefs checks each DefineLCDGlyph call against the runtime
// rules: no ASCII, no emoji or wide runes, at most lcdrune.MaxCartGlyphs distinct
// runes. It returns the runes provably defined. A dynamic call is reported as
// info and contributes nothing, so a SetText rune only it explains may be
// reported as unmapped.
func validateCartGlyphDefs(r *Result, calls []cartGlyphDefCall) map[rune]bool {
	accepted := map[rune]bool{}
	for _, dc := range calls {
		if dc.dynamic {
			r.info(dc.where, "Setup.DefineLCDGlyph's rune argument is not a literal (or a const bound to one) — this call is invisible to the checks below; a SetText rune it alone would explain may be reported as unmapped, and it does not count toward the %d-rune cap", lcdrune.MaxCartGlyphs)
			continue
		}
		if dc.r < 0x80 {
			r.fail(ruleCartGlyphASCII, dc.where, "Setup.DefineLCDGlyph(%s, ...) — code points below U+0080 are rejected at runtime; this definition never reaches the cartridge's table", strconv.QuoteRune(dc.r))
			continue
		}
		if why := lcdrune.Reason(dc.r); why != "" {
			r.fail(ruleCartGlyphRune, dc.where, "Setup.DefineLCDGlyph(%s, ...) — %s is %s, and both hosts refuse it at runtime; this definition never reaches the cartridge's table. Pick a plain, single-width, monochrome character (֍ U+058D is the tutorial's basket)", strconv.QuoteRune(dc.r), strconv.QuoteRune(dc.r), why)
			continue
		}
		if accepted[dc.r] {
			continue // redefining an accepted rune always succeeds
		}
		if len(accepted) >= lcdrune.MaxCartGlyphs {
			r.fail(ruleCartGlyphCap, dc.where, "Setup.DefineLCDGlyph(%s, ...) is a new rune after %d distinct definitions already accepted — the runtime caps a cartridge's table at %d and rejects this one", strconv.QuoteRune(dc.r), len(accepted), lcdrune.MaxCartGlyphs)
			continue
		}
		accepted[dc.r] = true
	}
	return accepted
}

// collectBundledGlyphs returns the runes of the manifest's assets.glyphs
// files. The files are read because the host registers a glyph only if it
// validates.
func collectBundledGlyphs(c *Cart, r *Result) map[rune]bool {
	out := map[rune]bool{}
	m, _, err := c.Manifest()
	if err != nil || m == nil {
		return out // reported by the manifest check
	}
	for _, rel := range m.GlyphAssets() {
		path := filepath.Join(c.Dir, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			r.fail(ruleBundledGlyph, c.At(path), "manifest lists %q under assets.glyphs, but the file is not there", rel)
			continue
		}
		env, err := assetfile.ReadBytes(rel, data)
		if err != nil {
			r.fail(ruleBundledGlyph, c.At(path), "%v", err)
			continue
		}
		out[rune(env.Codepoint)] = true
	}
	return out
}
