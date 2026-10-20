// SPDX-License-Identifier: GPL-2.0-or-later

package scores

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/storage"
)

func fullTable(scoresDesc ...int) Table {
	t := Table{}
	for _, s := range scoresDesc {
		t.Entries = append(t.Entries, Entry{Score: s})
	}
	return t
}

func TestQualifiesEmptyTable(t *testing.T) {
	var tab Table
	if !tab.Qualifies(1) {
		t.Error("any positive score should qualify against an empty table")
	}
	if tab.Qualifies(0) {
		t.Error("a zero score must never qualify")
	}
	if tab.Qualifies(-5) {
		t.Error("a negative score must never qualify")
	}
}

func TestQualifiesPartialTable(t *testing.T) {
	tab := fullTable(100, 50)
	if !tab.Qualifies(1) {
		t.Error("with room left, even a score below the lowest qualifies")
	}
}

func TestQualifiesFullTable(t *testing.T) {
	tab := fullTable(100, 90, 80, 70, 60, 50, 40, 30, 20, 10)
	if !tab.Qualifies(11) {
		t.Error("beating the 10th place should qualify")
	}
	if tab.Qualifies(10) {
		t.Error("TYING the 10th place must NOT qualify (earlier record keeps its spot)")
	}
	if tab.Qualifies(9) {
		t.Error("below the 10th place must not qualify")
	}
	if !tab.Qualifies(1000) {
		t.Error("beating first place should qualify")
	}
}

func TestInsertOrdersDescending(t *testing.T) {
	var tab Table
	if rank := tab.Insert(50, "B"); rank != 1 {
		t.Errorf("first insert should rank 1, got %d", rank)
	}
	if rank := tab.Insert(100, "A"); rank != 1 {
		t.Errorf("a higher score should rank 1, got %d", rank)
	}
	if rank := tab.Insert(75, "C"); rank != 2 {
		t.Errorf("a middle score should rank 2, got %d", rank)
	}
	want := []int{100, 75, 50}
	for i, e := range tab.Entries {
		if e.Score != want[i] {
			t.Fatalf("entries not sorted descending: %+v", tab.Entries)
		}
	}
}

func TestInsertTieKeepsEarlierRecordAhead(t *testing.T) {
	var tab Table
	tab.Insert(50, "FIRST")
	rank := tab.Insert(50, "SECOND")
	if rank != 2 {
		t.Errorf("an equal score should insert AFTER the existing equal, got rank %d", rank)
	}
	if tab.Entries[0].Name != "FIRST" || tab.Entries[1].Name != "SECOND" {
		t.Errorf("tie ordering wrong: %+v", tab.Entries)
	}
}

func TestInsertCapEvictsLowest(t *testing.T) {
	tab := fullTable(100, 90, 80, 70, 60, 50, 40, 30, 20, 10)
	rank := tab.Insert(55, "NEW")
	if rank != 6 {
		t.Errorf("expected rank 6, got %d", rank)
	}
	if len(tab.Entries) != Max {
		t.Fatalf("table must stay capped at %d, got %d", Max, len(tab.Entries))
	}
	if tab.Entries[len(tab.Entries)-1].Score != 20 {
		t.Errorf("the old 10th (score 10) should be evicted, tail is %+v", tab.Entries[len(tab.Entries)-1])
	}
	if tab.Entries[5].Name != "NEW" || tab.Entries[5].Score != 55 {
		t.Errorf("new entry not at rank 6: %+v", tab.Entries)
	}
}

func TestInsertNonQualifyingIntoFullTableIsNoop(t *testing.T) {
	tab := fullTable(100, 90, 80, 70, 60, 50, 40, 30, 20, 10)
	if rank := tab.Insert(5, "NOPE"); rank != 0 {
		t.Errorf("a below-table insert ranked %d, want 0", rank)
	}
	if len(tab.Entries) != Max || tab.Entries[Max-1].Score != 10 {
		t.Errorf("a below-table insert must not change the table: %+v", tab.Entries)
	}
}

func TestInsertRefusesWhatQualifiesRefuses(t *testing.T) {
	for _, score := range []int{0, -5} {
		var tab Table
		if rank := tab.Insert(score, "ZERO"); rank != 0 {
			t.Errorf("Insert(%d) into an empty table ranked %d, want 0", score, rank)
		}
		if len(tab.Entries) != 0 {
			t.Errorf("Insert(%d) into an empty table stored %+v", score, tab.Entries)
		}
	}
	tab := fullTable(100, 90, 80, 70, 60, 50, 40, 30, 20, 10)
	if rank := tab.Insert(10, "TIE"); rank != 0 || tab.Entries[Max-1].Name == "TIE" {
		t.Errorf("a tie with the lowest entry of a full table ranked %d: %+v", rank, tab.Entries)
	}
}

func TestBest(t *testing.T) {
	var empty Table
	if _, ok := empty.Best(); ok {
		t.Error("empty table should report no best entry")
	}
	tab := Table{Entries: []Entry{{Score: 42, Name: "ZOE"}, {Score: 7}}}
	best, ok := tab.Best()
	if !ok || best.Score != 42 || best.Name != "ZOE" {
		t.Errorf("Best should return the first entry, got %+v ok=%v", best, ok)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	store := storage.New(t.TempDir())
	tab := Load(store, "games/none")
	if len(tab.Entries) != 0 {
		t.Errorf("missing file should load as empty table, got %+v", tab)
	}
}

func TestLoadLegacyWithName(t *testing.T) {
	store := storage.New(t.TempDir())
	if err := store.Write("games/x", Label, map[string]any{"highScore": 4242, "name": "ADA"}); err != nil {
		t.Fatal(err)
	}
	tab := Load(store, "games/x")
	if len(tab.Entries) != 1 || tab.Entries[0].Score != 4242 || tab.Entries[0].Name != "ADA" {
		t.Errorf("legacy {highScore,name} should migrate to a one-entry table, got %+v", tab)
	}
}

func TestLoadLegacyWithoutName(t *testing.T) {
	store := storage.New(t.TempDir())
	if err := store.Write("games/x", Label, map[string]any{"highScore": 7}); err != nil {
		t.Fatal(err)
	}
	tab := Load(store, "games/x")
	if len(tab.Entries) != 1 || tab.Entries[0].Score != 7 || tab.Entries[0].Name != "" {
		t.Errorf("legacy nameless save should migrate to one anonymous entry, got %+v", tab)
	}
}

func TestLoadIsReadSideOnly(t *testing.T) {
	dir := t.TempDir()
	store := storage.New(dir)
	if err := store.Write("games/x", Label, map[string]any{"highScore": 7, "name": "REX"}); err != nil {
		t.Fatal(err)
	}
	_ = Load(store, "games/x")
	raw, err := os.ReadFile(filepath.Join(dir, "games", "x", Label+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "entries") || !strings.Contains(string(raw), "highScore") {
		t.Errorf("Load must never rewrite the file; got %s", raw)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	store := storage.New(t.TempDir())
	var tab Table
	tab.Insert(300, "AAA")
	tab.Insert(100, "CCC")
	tab.Insert(200, "")
	if err := Save(store, "games/x", tab); err != nil {
		t.Fatal(err)
	}
	got := Load(store, "games/x")
	if len(got.Entries) != 3 {
		t.Fatalf("round trip lost entries: %+v", got)
	}
	for i, e := range got.Entries {
		if e != tab.Entries[i] {
			t.Errorf("entry %d changed in round trip: %+v vs %+v", i, e, tab.Entries[i])
		}
	}
}

func TestRankMatchesInsert(t *testing.T) {
	tab := fullTable(100, 50, 50, 10)
	cases := []struct{ score, want int }{
		{200, 1}, {100, 2}, {50, 4}, {30, 4}, {10, 5}, {5, 5},
	}
	for _, c := range cases {
		if got := tab.Rank(c.score); got != c.want {
			t.Errorf("Rank(%d) = %d, want %d", c.score, got, c.want)
		}
		cp := Table{Entries: append([]Entry(nil), tab.Entries...)}
		if got := cp.Insert(c.score, "X"); got != c.want {
			t.Errorf("Insert(%d) rank = %d, want %d (must match Rank)", c.score, got, c.want)
		}
	}
}
