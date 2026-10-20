// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureCart(t *testing.T, name string) *Cart {
	t.Helper()
	root := repoRoot(t)
	dir, err := filepath.Abs(filepath.Join("testdata", "carts", name))
	if err != nil {
		t.Fatalf("resolving fixture %s: %v", name, err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return &Cart{Slug: name, Dir: dir, Root: root}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root, err := findRoot(cwd)
	if err != nil {
		t.Fatalf("findRoot: %v", err)
	}
	return root
}

func run(t *testing.T, id string, c *Cart) *Result {
	t.Helper()
	for _, check := range registry {
		if check.ID != id {
			continue
		}
		res := &Result{ID: check.ID, Title: check.Title, Rule: check.Rule, Status: StatusPass}
		check.Run(c, res)
		return res
	}
	t.Fatalf("no check %q in the registry", id)
	return nil
}

func findings(r *Result) string {
	var b strings.Builder
	for _, f := range r.Findings {
		b.WriteString(string(f.Status))
		b.WriteString(" ")
		b.WriteString(f.Where)
		b.WriteString(" ")
		b.WriteString(f.Message)
		b.WriteString("\n")
	}
	return b.String()
}

func wantContains(t *testing.T, got, want, why string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("missing %q (%s)\ngot:\n%s", want, why, got)
	}
}

func wantNotContains(t *testing.T, got, want, why string) {
	t.Helper()
	if strings.Contains(got, want) {
		t.Errorf("unexpected %q (%s)\ngot:\n%s", want, why, got)
	}
}

func TestGoldenRulesPassesCleanCartridge(t *testing.T) {
	res := run(t, "golden-rules", fixtureCart(t, "tidy"))
	if res.Status != StatusPass {
		t.Errorf("tidy fixture: status %s, want pass\n%s", res.Status, findings(res))
	}
}

func TestGoldenRulesIgnoresNonCartridgeFiles(t *testing.T) {
	c := fixtureCart(t, "tidy")
	srcs, err := c.Sources()
	if err != nil {
		t.Fatalf("Sources: %v", err)
	}
	var got []string
	for _, s := range srcs {
		got = append(got, filepath.Base(s.Path))
	}
	joined := strings.Join(got, " ")
	for _, want := range []string{"main.go", "variant.go"} {
		if !strings.Contains(joined, want) {
			t.Errorf("%s missing from the TinyGo build set %v", want, got)
		}
	}
	for _, unwanted := range []string{"hoststub.go", "sim_test.go"} {
		if strings.Contains(joined, unwanted) {
			t.Errorf("%s is not in the TinyGo build but was parsed as cartridge source (%v)", unwanted, got)
		}
	}
}

func TestGoldenRulesCatchesEveryRule(t *testing.T) {
	res := run(t, "golden-rules", fixtureCart(t, "rulebreaker"))
	if res.Status != StatusFail {
		t.Fatalf("rulebreaker fixture: status %s, want fail\n%s", res.Status, findings(res))
	}
	got := findings(res)
	for _, tc := range []struct{ want, why string }{
		{"declares func init()", "rule 1: no init()"},
		{"func main() has", "rule 1: main must stay empty"},
		{"package-level var calls buildClock()", "rule 1: no non-trivial package-level initializer"},
		{`imports "time"`, "rules 3 and 5: no wall clock, no time package"},
		{"time.Since", "rule 3: no wall clock read"},
		{`imports "math/rand"`, "rule 4: RandomU64 only"},
		{`imports "crypto/rand"`, "rule 4: RandomU64 only"},
		{"rand.Intn", "rule 4: the call site, not just the import"},
		{`imports "sync"`, "rule 5: no sync"},
		{"go statement", "rule 5: no goroutines"},
		{"channel type", "rule 5: no channels"},
		{"select statement", "rule 5: no select"},
		{`imports "fmt"`, "rule 6: avoid fmt"},
		{`imports "encoding/json"`, "rule 6: avoid encoding/json"},
		{"libm transcendental call site", "rule 8: math.Sin counted, not condemned"},
	} {
		wantContains(t, got, tc.want, tc.why)
	}
}

func TestGoldenRulesDoesNotWarnPerLibmSite(t *testing.T) {
	res := run(t, "golden-rules", fixtureCart(t, "rulebreaker"))
	for _, f := range res.Findings {
		if f.Status != StatusPass && strings.Contains(f.Message, "libm") {
			t.Errorf("libm reported as %s, want an informational count: %s", f.Status, f.Message)
		}
	}
}

func TestLCDPassesFittingText(t *testing.T) {
	res := run(t, "lcd", fixtureCart(t, "lcdtidy"))
	if res.Status != StatusPass {
		t.Errorf("lcdtidy fixture: status %s, want pass\n%s", res.Status, findings(res))
	}
	got := findings(res)
	wantNotContains(t, got, "SIXTEEN", "16 columns is the inclusive limit")
	wantNotContains(t, got, "SCORE", "a short literal plus a runtime piece is undecidable, so silent")
}

func TestLCDCatchesOverflowAndGlyphBudget(t *testing.T) {
	res := run(t, "lcd", fixtureCart(t, "lcdbroken"))
	if res.Status != StatusFail {
		t.Fatalf("lcdbroken fixture: status %s, want fail\n%s", res.Status, findings(res))
	}
	got := findings(res)
	for _, tc := range []struct{ want, why string }{
		{"PRESS ANY KEY TO CONTINUE", "a 25-column literal overflows"},
		{"press to play again", "a 19-column literal reached through a const overflows"},
		{"literal text alone is", "literal parts alone overflowing is decidable despite the runtime piece"},
		{"is not in the cabinet's CGRAM palette", "an unmapped rune renders '?'"},
		{"distinct custom glyphs at once", "nine customs on one screen exceeds the 8 CGRAM slots"},
	} {
		wantContains(t, got, tc.want, tc.why)
	}
}

func TestLCDSkipsWithoutSetText(t *testing.T) {
	res := run(t, "lcd", fixtureCart(t, "tidy"))
	if res.Status != StatusSkip {
		t.Errorf("status %s, want skip for a cartridge with no SetText calls", res.Status)
	}
}

func TestLCDPassesCartDefinedGlyph(t *testing.T) {
	res := run(t, "lcd", fixtureCart(t, "lcdcartglyph"))
	if res.Status != StatusPass {
		t.Errorf("lcdcartglyph fixture: status %s, want pass\n%s", res.Status, findings(res))
	}
}

func TestLCDStillFailsUndefinedRuneAlongsideCartGlyph(t *testing.T) {
	res := run(t, "lcd", fixtureCart(t, "lcdglyphmissing"))
	if res.Status != StatusFail {
		t.Fatalf("lcdglyphmissing fixture: status %s, want fail\n%s", res.Status, findings(res))
	}
	got := findings(res)
	wantContains(t, got, "not in the cabinet's CGRAM palette", "☠ is in neither table")
	wantNotContains(t, got, `"֍"`, "the cartridge's own defined glyph must not be reported as unmapped")
}

func TestLCDFlagsASCIICodepointGlyphDefinition(t *testing.T) {
	res := run(t, "lcd", fixtureCart(t, "lcdglyphascii"))
	if res.Status != StatusFail {
		t.Fatalf("lcdglyphascii fixture: status %s, want fail\n%s", res.Status, findings(res))
	}
	wantContains(t, findings(res), "code points below U+0080 are rejected", "rule 1 of DefineLCDGlyph")
}

func TestLCDFlagsEmojiAndWideGlyphDefinitions(t *testing.T) {
	res := run(t, "lcd", fixtureCart(t, "lcdglyphemoji"))
	if res.Status != StatusFail {
		t.Fatalf("lcdglyphemoji fixture: status %s, want fail\n%s", res.Status, findings(res))
	}
	got := findings(res)
	wantContains(t, got, `DefineLCDGlyph('🧺', ...) — '🧺' is emoji`, "the emoji definition")
	wantContains(t, got, `DefineLCDGlyph('一', ...) — '一' is wide`, "the CJK definition")
	wantContains(t, got, `uses '🧺', which is emoji`, "the SetText line printing the emoji")
	wantNotContains(t, got, `'֍'`, "the narrow monochrome symbol is exactly what the rule wants")
	if n := strings.Count(got, "both hosts refuse it at runtime"); n != 2 {
		t.Errorf("got %d refused-definition findings, want exactly 2 (the emoji and the ideograph)\n%s", n, got)
	}
}

func TestLCDFlagsGlyphDefinitionOverTheCap(t *testing.T) {
	res := run(t, "lcd", fixtureCart(t, "lcdglyphcap"))
	if res.Status != StatusFail {
		t.Fatalf("lcdglyphcap fixture: status %s, want fail\n%s", res.Status, findings(res))
	}
	got := findings(res)
	wantContains(t, got, "32 distinct definitions already accepted", "the 33rd rune is the one that is rejected")
	n := strings.Count(got, "distinct definitions already accepted")
	if n != 1 {
		t.Errorf("got %d over-cap findings, want exactly 1 (only the 33rd rune is rejected)\n%s", n, got)
	}
}

func TestGlyphTableReadsTheRealPalette(t *testing.T) {
	glyphs, err := loadGlyphTable(repoRoot(t))
	if err != nil {
		t.Fatalf("loadGlyphTable: %v", err)
	}
	for _, r := range []rune{'Å', '█', '░', '♥', '⚅', '⏶'} {
		if !glyphs[r] {
			t.Errorf("palette is missing %q, which internal/hardware/pi/glyphs.go defines", r)
		}
	}
	if glyphs['☠'] {
		t.Error("palette claims to carry ☠, which glyphs.go does not define")
	}
	if len(glyphs) < 30 {
		t.Errorf("palette parsed to only %d runes — the parse probably matched the wrong thing", len(glyphs))
	}
}

func TestGlyphTableErrorsOffRepo(t *testing.T) {
	if _, err := loadGlyphTable(t.TempDir()); err == nil {
		t.Error("want an error when glyphs.go is absent, so the LCD check can degrade instead of inventing a palette")
	}
}
