// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"encoding/json"
	"github.com/stealthmonkey99/arkade/tools/fleet"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistryIsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, check := range registry {
		switch {
		case check.ID == "":
			t.Error("a check has no ID")
		case seen[check.ID]:
			t.Errorf("duplicate check id %q — ids are the JSON keys and must be unique", check.ID)
		case check.Title == "":
			t.Errorf("%s: no Title", check.ID)
		case check.Rule == "":
			t.Errorf("%s: no Rule — every check cites the canon it enforces", check.ID)
		case check.Run == nil:
			t.Errorf("%s: no Run", check.ID)
		}
		seen[check.ID] = true
	}
	if len(registry) < 5 {
		t.Errorf("only %d checks registered", len(registry))
	}
}

func TestOnlyTheTestRunnerIsSlow(t *testing.T) {
	for _, check := range registry {
		if check.Slow && check.ID != "tests" {
			t.Errorf("%s is marked slow — is -fast's contract still right?", check.ID)
		}
	}
}

func TestStatusRollupTakesTheWorst(t *testing.T) {
	for _, tc := range []struct {
		name  string
		notes []Status
		want  Status
	}{
		{"nothing found", nil, StatusPass},
		{"one warning", []Status{StatusPass, StatusWarn}, StatusWarn},
		{"a warning and a failure", []Status{StatusWarn, StatusFail}, StatusFail},
		{"a failure then a pass", []Status{StatusFail, StatusPass}, StatusFail},
	} {
		r := &Result{Status: StatusPass}
		for _, s := range tc.notes {
			r.note(s, "rule", Loc{Path: "where"}, "message")
		}
		if r.Status != tc.want {
			t.Errorf("%s: status %s, want %s", tc.name, r.Status, tc.want)
		}
	}
}

func TestSkipDoesNotAffectVerdict(t *testing.T) {
	card := CartReport{Checks: []Result{{Status: StatusSkip}, {Status: StatusPass}}}
	if got := card.verdict(); got != StatusPass {
		t.Errorf("skip + pass = %s, want pass", got)
	}
	card = CartReport{Checks: []Result{{Status: StatusSkip}, {Status: StatusFail}}}
	if got := card.verdict(); got != StatusFail {
		t.Errorf("skip + fail = %s, want fail", got)
	}
}

func TestStrictPromotesWarningsToFailures(t *testing.T) {
	build := func(strict bool) Report {
		rep := Report{Strict: strict, Carts: []CartReport{
			{Slug: "warner", Checks: []Result{{Status: StatusWarn}}},
			{Slug: "clean", Checks: []Result{{Status: StatusPass}}},
		}}
		rep.finish()
		return rep
	}
	if got := build(false); got.Summary.Failed != 0 || got.Summary.Warned != 1 {
		t.Errorf("lenient: %d failed, %d warned; want 0 and 1", got.Summary.Failed, got.Summary.Warned)
	}
	if got := build(true); got.Summary.Failed != 1 || got.Summary.Warned != 0 {
		t.Errorf("strict: %d failed, %d warned; want 1 and 0", got.Summary.Failed, got.Summary.Warned)
	}
}

func TestTextReportHidesInfoUnlessVerbose(t *testing.T) {
	res := Result{ID: "demo", Status: StatusPass, Summary: "all good"}
	res.info(Loc{Path: "assets/blip.wav"}, "40%% sub-150Hz")
	rep := Report{Carts: []CartReport{{Slug: "demo", Checks: []Result{res}}}}
	rep.finish()

	if strings.Contains(rep.Text(false), "sub-150Hz") {
		t.Error("informational finding leaked into the default report")
	}
	if !strings.Contains(rep.Text(true), "sub-150Hz") {
		t.Error("-v did not print the informational finding")
	}
}

func TestJSONReportCarriesTheRollUp(t *testing.T) {
	rep := Report{Tool: "preflight", Carts: []CartReport{
		{Slug: "bad", Version: "1.0.0", Checks: []Result{{ID: "lcd", Status: StatusFail, Summary: "overflow"}}},
	}}
	rep.finish()
	raw, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Report
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Summary.Failed != 1 || len(back.Summary.FailedList) != 1 || back.Summary.FailedList[0] != "bad" {
		t.Errorf("round-tripped summary: %+v", back.Summary)
	}
	if back.Carts[0].Status != StatusFail {
		t.Errorf("cart status %s, want fail", back.Carts[0].Status)
	}
}

func TestSelectChecksFiltersAndValidates(t *testing.T) {
	all, err := selectChecks("", false)
	if err != nil {
		t.Fatalf("selectChecks: %v", err)
	}
	if len(all) != len(registry) {
		t.Errorf("selected %d of %d checks with no filter", len(all), len(registry))
	}

	fast, err := selectChecks("", true)
	if err != nil {
		t.Fatalf("selectChecks -fast: %v", err)
	}
	for _, check := range fast {
		if check.Slow {
			t.Errorf("-fast selected the slow check %q", check.ID)
		}
	}

	only, err := selectChecks("lcd, manifest", false)
	if err != nil {
		t.Fatalf("selectChecks -only: %v", err)
	}
	if len(only) != 2 || only[0].ID != "manifest" || only[1].ID != "lcd" {
		var ids []string
		for _, check := range only {
			ids = append(ids, check.ID)
		}
		t.Errorf("-only gave %v, want [manifest lcd] in registry order", ids)
	}

	if _, err := selectChecks("nosuchcheck", false); err == nil {
		t.Error("an unknown check id was accepted")
	}
}

func TestFindRootWalksUp(t *testing.T) {
	root := repoRoot(t)
	deep := filepath.Join(root, "cartridges", "tools", "preflight", "testdata")
	got, err := findRoot(deep)
	if err != nil {
		t.Fatalf("findRoot: %v", err)
	}
	if got != root {
		t.Errorf("findRoot(%s) = %s, want %s", deep, got, root)
	}
	if _, err := findRoot(t.TempDir()); err == nil {
		t.Error("findRoot succeeded outside any Årkade checkout")
	}
}

func requireFleet(t *testing.T, root string) {
	t.Helper()
	_, slugs, err := fleet.Dirs(root)
	if err != nil {
		t.Fatalf("listing cartridges: %v", err)
	}
	if len(slugs) < 40 {
		t.Skipf("no cartridges under %s (%d cartridges); set %s", root, len(slugs), fleet.RootsEnv)
	}
}

func TestDiscoverCartsFindsTheFleet(t *testing.T) {
	requireFleet(t, repoRoot(t))
	slugs, err := discoverCarts(repoRoot(t))
	if err != nil {
		t.Fatalf("discoverCarts: %v", err)
	}
	if len(slugs) < 40 {
		t.Errorf("found only %d cartridges — wrong directory?", len(slugs))
	}
	seen := map[string]bool{}
	for _, s := range slugs {
		if seen[s] {
			t.Errorf("duplicate slug %q", s)
		}
		seen[s] = true
	}
	if seen["tools"] {
		t.Error("tools/ was scanned as a cartridge")
	}
	for _, want := range []string{"life", "snake"} {
		if !seen[want] {
			t.Errorf("%s missing from the discovered fleet", want)
		}
	}
}

func TestSelectCartsRejectsUnknownSlug(t *testing.T) {
	root := repoRoot(t)
	requireFleet(t, root)
	if _, err := selectCarts(root, false, []string{"life"}); err != nil {
		t.Errorf("a real slug was rejected: %v", err)
	}
	if _, err := selectCarts(root, false, []string{"lifee"}); err == nil {
		t.Error("a typo'd slug produced an empty report instead of an error")
	}
	if _, err := selectCarts(root, true, []string{"life"}); err == nil {
		t.Error("-all with a slug argument was accepted")
	}
	if _, err := selectCarts(root, false, nil); err == nil {
		t.Error("no slug and no -all was accepted")
	}
}

func parseExpr(t *testing.T, src string) ast.Expr {
	t.Helper()
	e, err := parser.ParseExpr(src)
	if err != nil {
		t.Fatalf("parsing %q: %v", src, err)
	}
	return e
}

func TestLiteralStringResolvesWhatIsKnown(t *testing.T) {
	consts := map[string]string{"title": "ORBIT"}
	for _, tc := range []struct {
		src  string
		want string
		ok   bool
	}{
		{`"HELLO"`, "HELLO", true},
		{`"HEL" + "LO"`, "HELLO", true},
		{`title`, "ORBIT", true},
		{`title + " MAZE"`, "ORBIT MAZE", true},
		{`(title)`, "ORBIT", true},
		{`unknown`, "", false},
		{`itoa(n)`, "", false},
		{`"SCORE " + itoa(n)`, "", false},
		{`42`, "", false},
	} {
		got, ok := literalString(parseExpr(t, tc.src), consts)
		if ok != tc.ok || got != tc.want {
			t.Errorf("literalString(%s) = %q,%v; want %q,%v", tc.src, got, ok, tc.want, tc.ok)
		}
	}
}

func TestLiteralFloorKeepsOnlyTheCertainText(t *testing.T) {
	consts := map[string]string{"unit": " PTS"}
	for _, tc := range []struct {
		src         string
		wantText    string
		wantDynamic bool
	}{
		{`"FINAL"`, "FINAL", false},
		{`"SCORE " + itoa(n)`, "SCORE ", true},
		{`"SCORE " + itoa(n) + unit`, "SCORE  PTS", true},
		{`itoa(n)`, "", true},
	} {
		text, dynamic := literalFloor(parseExpr(t, tc.src), consts)
		if text != tc.wantText || dynamic != tc.wantDynamic {
			t.Errorf("literalFloor(%s) = %q,%v; want %q,%v", tc.src, text, dynamic, tc.wantText, tc.wantDynamic)
		}
	}
}

func TestCustomRunesFindsNonASCIIOnly(t *testing.T) {
	got := customRunes("HP ♥♥♥ 100%")
	if len(got) != 1 || got[0] != '♥' {
		t.Errorf("customRunes = %q, want one '♥' (deduplicated, ASCII excluded)", string(got))
	}
	if len(customRunes("PLAIN ASCII 123")) != 0 {
		t.Error("ASCII text reported custom runes")
	}
}

func TestPackageSymbolsSplitsFunctionsFromTypes(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", `package main
type coord struct{ x, y int }
func helper() int { return 1 }
func (c coord) method() {}
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	scope := packageSymbols([]*SourceFile{{Path: "x.go", File: f}})
	if !scope.funcs["helper"] {
		t.Error("helper not recorded as a function")
	}
	if !scope.types["coord"] {
		t.Error("coord not recorded as a type")
	}
	if scope.funcs["method"] {
		t.Error("a method was recorded as a package-level function")
	}
}

func TestAssetFilesWalksNestedDirectories(t *testing.T) {
	c := tempCart(t, "assets")
	for _, rel := range []string{"a.wav", "sprites/b.png", "sprites/deep/c.png", "notes.txt"} {
		path := filepath.Join(c.Dir, "assets", rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pngs, err := c.AssetFiles(".png")
	if err != nil {
		t.Fatal(err)
	}
	if len(pngs) != 2 {
		t.Errorf("found %d PNGs, want 2 (nested included, .txt and .wav excluded)", len(pngs))
	}
	wavs, err := c.AssetFiles(".wav")
	if err != nil {
		t.Fatal(err)
	}
	if len(wavs) != 1 {
		t.Errorf("found %d WAVs, want 1", len(wavs))
	}
}

func TestAssetFilesIsEmptyWithoutAnAssetsDir(t *testing.T) {
	c := &Cart{Slug: "bare", Dir: t.TempDir(), Root: t.TempDir()}
	got, err := c.AssetFiles(".png")
	if err != nil || len(got) != 0 {
		t.Errorf("AssetFiles = %v, %v; want empty and no error", got, err)
	}
}
