// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, c *Cart, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(c.Dir, ConfigName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func scanFixture(t *testing.T, c *Cart, ids ...string) *CartReport {
	t.Helper()
	card := &CartReport{Slug: c.Slug}
	for _, id := range ids {
		card.Checks = append(card.Checks, *run(t, id, c))
	}
	if cfg, err := c.Config(); err == nil {
		applySuppressions(c, cfg, card)
	}
	card.Status = card.verdict()
	return card
}

func TestConfigCheckSkipsWithoutAFile(t *testing.T) {
	res := run(t, "config", tempCart(t, "plain"))
	if res.Status != StatusSkip {
		t.Errorf("status %s, want skip — most cartridges have no %s", res.Status, ConfigName)
	}
}

func TestConfigCheckRejectsMalformedExemptions(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{
			"no reason",
			`{"suppress":[{"check":"golden-rules","contains":"seededConfetti"}]}`,
			"no \"reason\"",
		},
		{
			"a whitespace reason is no reason",
			`{"suppress":[{"check":"golden-rules","contains":"x","reason":"   "}]}`,
			"no \"reason\"",
		},
		{
			"narrows nothing",
			`{"suppress":[{"check":"golden-rules","reason":"because"}]}`,
			"needs a \"where\" or a \"contains\"",
		},
		{
			"unknown check id",
			`{"suppress":[{"check":"goldenrules","contains":"x","reason":"because"}]}`,
			"no check \"goldenrules\" in the registry",
		},
		{
			"no check id",
			`{"suppress":[{"contains":"x","reason":"because"}]}`,
			"no \"check\"",
		},
		{
			"empty file",
			`{}`,
			"delete it rather than shipping an empty exemption file",
		},
		{
			"unknown field",
			`{"supress":[]}`,
			"unparseable",
		},
		{
			"wav floor with no reason",
			`{"wavFloor":{"hz":40}}`,
			"no \"reason\"",
		},
		{
			"wav floor out of range",
			`{"wavFloor":{"frac":1.5,"reason":"because"}}`,
			"frac in [0,1]",
		},
	} {
		c := tempCart(t, "bad")
		writeConfig(t, c, tc.body)
		res := run(t, "config", c)
		if res.Status != StatusFail {
			t.Errorf("%s: status %s, want fail\n%s", tc.name, res.Status, findings(res))
			continue
		}
		wantContains(t, findings(res), tc.want, tc.name)
	}
}

func TestBrokenConfigSuppressesNothing(t *testing.T) {
	c := tempCart(t, "broken")
	writeSprite(t, c, "handle.png", 8, 8, 251, true)
	writeConfig(t, c, `{"suppress":[{"check":"sprite-alpha","contains":"A=251"}]}`) // no reason
	card := scanFixture(t, c, "config", "sprite-alpha")
	if got := card.check("sprite-alpha").Status; got != StatusWarn {
		t.Errorf("sprite-alpha status %s, want warn — a broken config must not silence anything", got)
	}
	if got := card.check("config").Status; got != StatusFail {
		t.Errorf("config status %s, want fail", got)
	}
}

func TestSuppressionClearsTheVerdictButNotTheReport(t *testing.T) {
	const reason = "the kettle handle is solid-looking but benign by design"
	c := tempCart(t, "kettle")
	writeSprite(t, c, "handle.png", 8, 8, 250, true)
	writeConfig(t, c, `{"suppress":[{"check":"sprite-alpha","where":"cartridges/kettle/assets/sprites/handle.png","reason":"`+reason+`"}]}`)

	card := scanFixture(t, c, "config", "sprite-alpha")
	if card.Status != StatusPass {
		t.Fatalf("verdict %s, want pass — the finding is answered for\n%s", card.Status, findings(card.check("sprite-alpha")))
	}

	res := card.check("sprite-alpha")
	if len(res.Findings) != 1 {
		t.Fatalf("%d findings, want the original one kept", len(res.Findings))
	}
	f := res.Findings[0]
	if f.Status != StatusSuppressed {
		t.Errorf("finding status %s, want suppressed", f.Status)
	}
	if f.Severity != StatusWarn {
		t.Errorf("severity %s, want the warn it would have been — the JSON has to keep what was exempted", f.Severity)
	}
	if f.Reason != reason {
		t.Errorf("reason %q, want the config's", f.Reason)
	}

	rep := Report{Carts: []CartReport{*card}}
	rep.finish()
	text := rep.Text(false)
	wantContains(t, text, "[suppressed warn: "+reason+"]", "the reason is printed on the default report card")
	wantContains(t, text, "A=250", "the original message is printed too")
}

func TestSuppressionWhereMatchesTheFileWithoutTheLine(t *testing.T) {
	for _, tc := range []struct {
		want  string
		got   Loc
		match bool
	}{
		{"a/b.go", Loc{Path: "a/b.go"}, true},
		{"a/b.go", Loc{Path: "a/b.go", Line: 264}, true},
		{"a/b.go", Loc{Path: "a/b.go", Line: 1}, true},
		{"a/b.go", Loc{Path: "a/bb.go", Line: 1}, false},
		{"a/b.go:264", Loc{Path: "a/b.go", Line: 264}, true},
		{"a/b.go:264", Loc{Path: "a/b.go", Line: 265}, false},
		{"b.go:3", Loc{Path: "x/a/b.go", Cart: "b.go", Line: 3}, true},
		{"a/b.go", Loc{Path: "x/a/b.go", Cart: "b.go"}, true},
		{"z/b.go", Loc{Path: "x/a/b.go", Cart: "b.go"}, false},
		{"b.go", Loc{Path: "b.go/../c.go"}, false},
	} {
		if got := parseWhere(tc.want).covers(tc.got, "a/"); got != tc.match {
			t.Errorf("%q covers %+v = %v, want %v", tc.want, tc.got, got, tc.match)
		}
	}
}

func TestSuppressionDoesNotBleedOntoOtherFindings(t *testing.T) {
	c := tempCart(t, "twins")
	writeSprite(t, c, "handle.png", 8, 8, 250, true)
	writeSprite(t, c, "lid.png", 8, 8, 251, true)
	writeConfig(t, c, `{"suppress":[{"check":"sprite-alpha","where":"cartridges/twins/assets/sprites/handle.png","reason":"benign by design"}]}`)

	card := scanFixture(t, c, "config", "sprite-alpha")
	if got := card.check("sprite-alpha").Status; got != StatusWarn {
		t.Errorf("status %s, want warn — lid.png is still unanswered", got)
	}
}

func tempCartElsewhere(t *testing.T, slug string) *Cart {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/stealthmonkey99/arkade\n\ngo 1.23.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "cartridges", slug)
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &Cart{Slug: slug, Dir: dir, Root: root}
}

func TestSuppressionWhereSurvivesTheCartridgeLivingOutsideTheRepoRoot(t *testing.T) {
	c := tempCartElsewhere(t, "kettle")
	writeSprite(t, c, "handle.png", 8, 8, 250, true)
	writeConfig(t, c, `{"suppress":[{"check":"sprite-alpha","where":"cartridges/kettle/assets/sprites/handle.png","reason":"benign by design"}]}`)

	card := scanFixture(t, c, "config", "sprite-alpha")
	if card.Status != StatusPass {
		t.Fatalf("verdict %s, want pass — the waiver should still match\n%s", card.Status, findings(card.check("sprite-alpha")))
	}
	if findings := findings(card.check("config")); strings.Contains(findings, "matched nothing") {
		t.Errorf("config findings say the waiver matched nothing:\n%s", findings)
	}
}

func TestSuppressionWhereAcceptsTheBareCartridgeRelativeForm(t *testing.T) {
	c := tempCartElsewhere(t, "kettle")
	writeSprite(t, c, "handle.png", 8, 8, 250, true)
	writeConfig(t, c, `{"suppress":[{"check":"sprite-alpha","where":"assets/sprites/handle.png","reason":"benign by design"}]}`)

	card := scanFixture(t, c, "config", "sprite-alpha")
	if card.Status != StatusPass {
		t.Fatalf("verdict %s, want pass\n%s", card.Status, findings(card.check("sprite-alpha")))
	}
}

func TestSuppressionWhereFallbackDoesNotBleedOntoOtherFindings(t *testing.T) {
	c := tempCartElsewhere(t, "twins")
	writeSprite(t, c, "handle.png", 8, 8, 250, true)
	writeSprite(t, c, "lid.png", 8, 8, 251, true)
	writeConfig(t, c, `{"suppress":[{"check":"sprite-alpha","where":"cartridges/twins/assets/sprites/handle.png","reason":"benign by design"}]}`)

	card := scanFixture(t, c, "config", "sprite-alpha")
	if got := card.check("sprite-alpha").Status; got != StatusWarn {
		t.Errorf("status %s, want warn — lid.png is still unanswered", got)
	}
}

func TestSuppressionWhereFallbackNeverReachesOutsideTheCartridge(t *testing.T) {
	c := tempCartElsewhere(t, "kettle")
	writeSprite(t, c, "handle.png", 8, 8, 250, true)
	writeConfig(t, c, `{"suppress":[{"check":"sprite-alpha","where":"cartridges/other/assets/sprites/handle.png","reason":"wrong cartridge on purpose"}]}`)

	card := scanFixture(t, c, "config", "sprite-alpha")
	if got := card.check("sprite-alpha").Status; got != StatusWarn {
		t.Errorf("status %s, want warn — the waiver names a different cartridge's file", got)
	}
}

func TestSuppressionIsScopedToItsCheck(t *testing.T) {
	c := tempCart(t, "misaimed")
	writeSprite(t, c, "handle.png", 8, 8, 250, true)
	writeConfig(t, c, `{"suppress":[{"check":"lcd","contains":"A=250","reason":"wrong check on purpose"}]}`)
	card := scanFixture(t, c, "config", "sprite-alpha")
	if got := card.check("sprite-alpha").Status; got != StatusWarn {
		t.Errorf("status %s, want warn — the suppression named a different check", got)
	}
}

func TestStaleSuppressionWarnsOnTheConfigCheck(t *testing.T) {
	c := tempCart(t, "stale")
	writeSprite(t, c, "clean.png", 8, 8, 255, true)
	writeConfig(t, c, `{"suppress":[{"check":"sprite-alpha","contains":"A=250","reason":"fixed long ago"}]}`)
	card := scanFixture(t, c, "config", "sprite-alpha")
	res := card.check("config")
	if res.Status != StatusWarn {
		t.Fatalf("config status %s, want warn\n%s", res.Status, findings(res))
	}
	wantContains(t, findings(res), "matched nothing", "a stale suppression says so")
}

func TestRestatusRecomputesFromFindingsAndPreservesSkip(t *testing.T) {
	r := &Result{Status: StatusFail, Findings: []Finding{
		{Status: StatusSuppressed}, {Status: StatusPass},
	}}
	r.restatus()
	if r.Status != StatusPass {
		t.Errorf("status %s, want pass once the only failure is suppressed", r.Status)
	}

	skipped := &Result{Status: StatusSkip}
	skipped.restatus()
	if skipped.Status != StatusSkip {
		t.Errorf("status %s, want skip preserved", skipped.Status)
	}
}

func TestFleetConfigsAreValid(t *testing.T) {
	root := repoRoot(t)
	slugs, err := discoverCarts(root)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, slug := range slugs {
		path := filepath.Join(root, "cartridges", slug, ConfigName)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		found++
		cfg, err := LoadConfig(path, registryIDs)
		if err != nil {
			t.Errorf("%s: %v", strings.TrimPrefix(path, root+"/"), err)
			continue
		}
		if cfg == nil {
			t.Errorf("%s: exists but loaded as nil", path)
		}
	}
	t.Logf("%d of %d cartridges carry a %s", found, len(slugs), ConfigName)
}
