// SPDX-License-Identifier: GPL-2.0-or-later

package rcart

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func schema1Manifest() Manifest {
	src := "https://example.com/arkade-cartridges/tree/main/life"
	return Manifest{
		SchemaVersion: 1,
		ABI:           2,
		Slug:          "life",
		Title:         "LIFE",
		Publisher:     "arkade",
		Author:        "Ada",
		Version:       "1.0.0",
		Description:   "A cellular automaton.",
		Tags:          []string{"puzzle", "retro"},
		Players:       PlayerModes{PlayersSolo, PlayersVersus},
		Audience:      AudienceAllAges,
		Visibility:    VisibilityPublic,
		Icon:          "assets/icon.png",
		Manual:        "MANUAL.md",
		Links:         &Links{Source: &src},
	}
}

func TestValidateSchema1Accepts(t *testing.T) {
	m := schema1Manifest()
	if err := m.Validate(); err != nil {
		t.Fatalf("valid schema-1 manifest rejected: %v", err)
	}
	// links.source: null is the private-cartridge starting state.
	m.Links = &Links{Source: nil}
	if err := m.Validate(); err != nil {
		t.Fatalf("null links.source rejected: %v", err)
	}
}

func TestValidateSchema1Rejects(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Manifest)
		wantSub string
	}{
		{"missing title", func(m *Manifest) { m.Title = "" }, `"title" is required`},
		{"missing publisher", func(m *Manifest) { m.Publisher = "" }, `"publisher"`},
		{"missing author", func(m *Manifest) { m.Author = "" }, `"author"`},
		{"bad version", func(m *Manifest) { m.Version = "1.0" }, "MAJOR.MINOR.PATCH"},
		{"missing description", func(m *Manifest) { m.Description = "" }, `"description"`},
		{"long description", func(m *Manifest) { m.Description = strings.Repeat("x", maxDescriptionLen+1) }, "card text"},
		{"no tags", func(m *Manifest) { m.Tags = nil }, `"tags"`},
		{"bad tag", func(m *Manifest) { m.Tags = []string{"Puzzle Games"} }, "tag"},
		{"no players", func(m *Manifest) { m.Players = nil }, `"players"`},
		{"legacy 2p players", func(m *Manifest) { m.Players = PlayerModes{PlayersLegacyTwo} }, "ambiguous"},
		{"unknown player mode", func(m *Manifest) { m.Players = PlayerModes{"3p"} }, "vocabulary"},
		{"duplicate player mode", func(m *Manifest) { m.Players = PlayerModes{PlayersSolo, PlayersSolo} }, "duplicated"},
		{"bad audience", func(m *Manifest) { m.Audience = "E10+" }, "audience"},
		{"missing audience", func(m *Manifest) { m.Audience = "" }, "audience"},
		{"bad visibility", func(m *Manifest) { m.Visibility = "unlisted" }, "visibility"},
		{"missing visibility", func(m *Manifest) { m.Visibility = "" }, "visibility"},
		{"bad capability", func(m *Manifest) { m.Capabilities = []string{"Motion!"} }, "capability"},
		{"missing icon", func(m *Manifest) { m.Icon = "" }, `"icon"`},
		{"traversal icon", func(m *Manifest) { m.Icon = "../icon.png" }, "bundle-relative"},
		{"absolute manual", func(m *Manifest) { m.Manual = "/etc/passwd" }, "bundle-relative"},
		{"missing links", func(m *Manifest) { m.Links = nil }, `"links"`},
		{"non-https source", func(m *Manifest) { s := "ftp://x"; m.Links = &Links{Source: &s} }, "https"},
		{"bad slug", func(m *Manifest) { m.Slug = "My Game" }, "slug"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := schema1Manifest()
			tc.mutate(&m)
			err := m.Validate()
			if err == nil {
				t.Fatal("expected a validation error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error %q does not mention %q", err, tc.wantSub)
			}
		})
	}
}

func TestValidateLegacyManifest(t *testing.T) {
	m := Manifest{ABI: 2, Slug: "snake", Name: "Snake"}
	if err := m.Validate(); err != nil {
		t.Fatalf("legacy manifest rejected: %v", err)
	}
	m.Name = ""
	if err := m.Validate(); err == nil {
		t.Fatal("legacy manifest with no name accepted")
	}
	m = Manifest{SchemaVersion: 2, Slug: "future", Title: "Future"}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "schemaVersion 2") {
		t.Fatalf("unknown schemaVersion not rejected: %v", err)
	}
}

func TestPlayerModesDecodeLegacyIntegers(t *testing.T) {
	var m Manifest
	if err := json.Unmarshal([]byte(`{"slug":"x","name":"X","players":[1,2]}`), &m); err != nil {
		t.Fatalf("legacy players failed to decode: %v", err)
	}
	if len(m.Players) != 2 || m.Players[0] != PlayersSolo || m.Players[1] != PlayersLegacyTwo {
		t.Fatalf("players = %v, want [%s %s]", m.Players, PlayersSolo, PlayersLegacyTwo)
	}
	if err := json.Unmarshal([]byte(`{"players":[3]}`), &m); err == nil {
		t.Fatal("players:[3] decoded, want an error")
	}
	if err := json.Unmarshal([]byte(`{"players":[true]}`), &m); err == nil {
		t.Fatal("players:[true] decoded, want an error")
	}
}

func TestPlayerModesCounts(t *testing.T) {
	cases := []struct {
		modes    PlayerModes
		max      int
		one, two bool
	}{
		{PlayerModes{PlayersSolo}, 1, true, false},
		{PlayerModes{PlayersSolo, PlayersVersus}, 2, true, true},
		{PlayerModes{PlayersAlternating}, 2, false, true},
		{PlayerModes{PlayersLegacyTwo}, 2, false, true},
		{PlayerModes{}, 0, false, false},
	}
	for _, tc := range cases {
		if got := tc.modes.MaxPlayers(); got != tc.max {
			t.Errorf("%v.MaxPlayers() = %d, want %d", tc.modes, got, tc.max)
		}
		if got := tc.modes.SupportsCount(1); got != tc.one {
			t.Errorf("%v.SupportsCount(1) = %v, want %v", tc.modes, got, tc.one)
		}
		if got := tc.modes.SupportsCount(2); got != tc.two {
			t.Errorf("%v.SupportsCount(2) = %v, want %v", tc.modes, got, tc.two)
		}
	}
}

func TestLoadSchema1Bundle(t *testing.T) {
	manifest := schema1Manifest()
	manifest.Slug = "demo"
	manifest.Logo = "assets/logo.png"
	raw, err := json.Marshal(&manifest)
	if err != nil {
		t.Fatal(err)
	}
	tarBytes := buildTar(t, false, []tarEntry{
		{name: ManifestName, data: raw},
		{name: WasmName, data: []byte("\x00asm")},
		{name: "assets/icon.png", data: encode8x8PNG(t)},
		{name: "MANUAL.md", data: []byte("# Demo\n")},
	})
	b, err := Load(bytes.NewReader(tarBytes))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if b.Manifest.DisplayTitle() != "LIFE" {
		t.Errorf("DisplayTitle = %q, want LIFE", b.Manifest.DisplayTitle())
	}
	if _, ok := b.Assets["MANUAL.md"]; !ok {
		t.Error("manual not carried through as a bundle asset")
	}
	found := false
	for _, w := range b.Warnings {
		if strings.Contains(w.Message, `logo "assets/logo.png" not found`) {
			found = true
		}
	}
	if !found {
		t.Errorf("missing-logo warning absent; warnings = %v", b.Warnings)
	}

	manifest.Audience = "E"
	raw, _ = json.Marshal(&manifest)
	bad := buildTar(t, false, []tarEntry{
		{name: ManifestName, data: raw},
		{name: WasmName, data: []byte("\x00asm")},
	})
	if _, err := Load(bytes.NewReader(bad)); err == nil || !strings.Contains(err.Error(), "audience") {
		t.Fatalf("schema-invalid bundle loaded; err = %v", err)
	}
}
