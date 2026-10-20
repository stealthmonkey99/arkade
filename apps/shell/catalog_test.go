// SPDX-License-Identifier: GPL-2.0-or-later

package shell

import (
	"testing"

	bluetoothapp "github.com/stealthmonkey99/arkade/apps/bluetooth"
	"github.com/stealthmonkey99/arkade/internal/engine"
)

func game(slug, name string, tags []string, players []string) engine.Manifest {
	return engine.Manifest{Slug: slug, Name: name, Type: engine.TypeGame, Tags: tags, Players: players}
}

func testApps() []engine.Manifest {
	return []engine.Manifest{
		{Slug: "clock", Name: "Clock", Type: engine.TypeApp},
		game("cart/solo", "Solo", []string{"puzzle"}, []string{"1p"}),
		game("cart/duel", "Duel", []string{"arcade", "zzz-custom"}, []string{"2p-versus"}),
		game("cart/both", "Both", []string{"puzzle", "arcade"}, []string{"1p", "2p-coop"}),
		game("cart/legacy", "Legacy", nil, nil),
	}
}

func catByID(t *testing.T, cats []category, id string) category {
	t.Helper()
	for _, c := range cats {
		if c.id == id {
			return c
		}
	}
	t.Fatalf("no %q shelf in %v", id, catIDs(cats))
	return category{}
}

func catIDs(cats []category) []string {
	ids := make([]string, len(cats))
	for i, c := range cats {
		ids[i] = c.id
	}
	return ids
}

// slugsOf lists the app slugs on a shelf, skipping control tiles.
func slugsOf(c category) []string {
	var out []string
	for _, e := range c.entries {
		if e.kind == entryApp {
			out = append(out, e.m.Slug)
		}
	}
	return out
}

func TestBuildCatalogShelves(t *testing.T) {
	cats := buildCatalog(testApps(), []string{"cart/duel", "gone"}, nil)

	want := []string{catRecent, catAll, "puzzle", "arcade", "zzz-custom", catSystem}
	got := catIDs(cats)
	if len(got) != len(want) {
		t.Fatalf("shelves = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("shelves = %v, want %v (curated order, unknown tags after, SYSTEM last)", got, want)
		}
	}

	if recent := slugsOf(catByID(t, cats, catRecent)); len(recent) != 1 || recent[0] != "cart/duel" {
		t.Errorf("RECENT = %v, want just cart/duel (MRU order, unknown slugs dropped)", recent)
	}
	all := catByID(t, cats, catAll)
	if len(slugsOf(all)) != 5 {
		t.Errorf("ALL holds %d apps, want 5 (games and system apps both)", len(slugsOf(all)))
	}
	if all.entries[0].kind != entrySearch {
		t.Error("ALL's first tile should be the search control")
	}
	if puzzle := slugsOf(catByID(t, cats, "puzzle")); len(puzzle) != 2 {
		t.Errorf("puzzle shelf holds %v, want 2 (solo + both)", puzzle)
	}
	system := catByID(t, cats, catSystem)
	if got := slugsOf(system); len(got) != 1 || got[0] != "clock" {
		t.Errorf("SYSTEM apps = %v, want just clock", got)
	}
	wantControls := []entryKind{entrySleep, entryVolume, entryBrightness, entryWifi, entryBluetooth, entryTemp, entryShutdown}
	for i, k := range wantControls {
		if system.entries[i].kind != k {
			t.Fatalf("SYSTEM tile %d kind = %v, want %v (the controls lead the shelf, sleep first)", i, system.entries[i].kind, k)
		}
	}
}

func TestBuildCatalogBluetoothTileCarriesTheApp(t *testing.T) {
	bluetoothTile := func() (entry, bool) {
		for _, e := range catByID(t, buildCatalog(testApps(), nil, nil), catSystem).entries {
			if e.kind == entryBluetooth {
				return e, true
			}
		}
		return entry{}, false
	}
	if e, ok := bluetoothTile(); !ok || e.m.Slug != bluetoothapp.Slug {
		t.Fatalf("BLUETOOTH tile = %+v (present %v), want one carrying the %s manifest", e, ok, bluetoothapp.Slug)
	}

	m, err := engine.Lookup(bluetoothapp.Slug)
	if err != nil {
		t.Fatal(err)
	}
	engine.Unregister(m.Slug)
	t.Cleanup(func() { engine.Register(m) })
	if _, ok := bluetoothTile(); ok {
		t.Error("BLUETOOTH tile built with no app registered to open")
	}
}

func TestBuildCatalogFavorites(t *testing.T) {
	for _, c := range buildCatalog(testApps(), nil, nil) {
		if c.id == catFavorites {
			t.Fatal("FAVORITES shelf present with nothing starred")
		}
	}

	// Unknown slugs (an unplugged cartridge) are dropped.
	cats := buildCatalog(testApps(), []string{"cart/solo"}, []string{"cart/duel", "cart/solo", "gone"})
	ids := catIDs(cats)
	if ids[0] != catRecent || ids[1] != catFavorites || ids[2] != catAll {
		t.Fatalf("shelf order = %v, want FAVORITES right after RECENT", ids)
	}
	favs := slugsOf(catByID(t, cats, catFavorites))
	if len(favs) != 2 || favs[0] != "cart/duel" || favs[1] != "cart/solo" {
		t.Fatalf("FAVORITES = %v, want [cart/duel cart/solo] in star order", favs)
	}
	if got := catByID(t, cats, catFavorites).accent; got != accentFor(catFavorites) || got.R != 240 {
		t.Fatalf("FAVORITES accent = %+v, want the yellow C-button color", got)
	}
}

func TestSearchMatches(t *testing.T) {
	apps := testApps()
	got := searchMatches(apps, "o")
	// Nothing starts with "o"; Clock, Solo and Both contain it.
	if len(got) != 3 {
		t.Fatalf("searchMatches(o) = %d entries, want 3", len(got))
	}
	got = searchMatches(apps, "SO")
	if len(got) != 1 || got[0].m.Slug != "cart/solo" {
		t.Fatalf("searchMatches(SO) = %v, want just cart/solo (case-insensitive prefix)", got)
	}
	if got := searchMatches(apps, ""); got != nil {
		t.Fatalf("empty query should match nothing, got %v", got)
	}
	got = searchMatches(apps, "l")
	if len(got) == 0 || got[0].m.Slug != "cart/legacy" {
		t.Fatalf("searchMatches(l) should rank the prefix match (Legacy) first, got %v", got)
	}
}

func TestPlayersBadgeAndLine(t *testing.T) {
	cases := []struct {
		modes []string
		badge string
		line  string
	}{
		{[]string{"1p"}, "1P", "1P"},
		{[]string{"2p-versus"}, "2P", "2P VS"},
		{[]string{"1p", "2p-coop"}, "1-2P", "1P + 2P CO-OP"},
		{[]string{"1p", "alternating"}, "1-2P", "1P + 2P TURNS"},
		{nil, "", ""},
	}
	for _, tc := range cases {
		if got := playersBadge(tc.modes); got != tc.badge {
			t.Errorf("playersBadge(%v) = %q, want %q", tc.modes, got, tc.badge)
		}
		if got := playersLine(tc.modes); got != tc.line {
			t.Errorf("playersLine(%v) = %q, want %q", tc.modes, got, tc.line)
		}
	}
}
