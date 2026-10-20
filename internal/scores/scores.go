// SPDX-License-Identifier: GPL-2.0-or-later

// Package scores maintains the per-game top-10 high-score tables stored
// under each game's "scores" label. Load reads the legacy single
// {highScore, name} shape as a one-entry table; the table shape is written
// only on the next Save.
package scores

import "github.com/stealthmonkey99/arkade/internal/storage"

// Max is the table capacity: the classic arcade top ten.
const Max = 10

// Label is the storage label every game's table lives under.
const Label = "scores"

// Entry is one recorded score. Name may be empty (anonymous submit, or a
// migrated legacy save that predates name entry).
type Entry struct {
	Score int    `json:"score"`
	Name  string `json:"name,omitempty"`
}

// Table is a game's top scores, sorted descending; a tie keeps the earlier
// record ahead.
type Table struct {
	Entries []Entry `json:"entries"`
}

// Load reads a game's table from storage. A missing or unreadable file
// yields an empty table; a legacy {"highScore": N, "name": S} file (no
// "entries" key) yields a one-entry table.
func Load(store *storage.Store, slug string) Table {
	var raw struct {
		Entries   []Entry `json:"entries"`
		HighScore *int    `json:"highScore"`
		Name      string  `json:"name"`
	}
	if err := store.Read(slug, Label, &raw); err != nil {
		return Table{}
	}
	if raw.Entries != nil {
		return Table{Entries: raw.Entries}
	}
	if raw.HighScore != nil {
		return Table{Entries: []Entry{{Score: *raw.HighScore, Name: raw.Name}}}
	}
	return Table{}
}

// Save persists the table (always in the new {"entries": ...} shape).
func Save(store *storage.Store, slug string, t Table) error {
	return store.Write(slug, Label, t)
}

// Best returns the top entry, or ok=false for an empty table.
func (t Table) Best() (Entry, bool) {
	if len(t.Entries) == 0 {
		return Entry{}, false
	}
	return t.Entries[0], true
}

// Qualifies reports whether a run's score earns a spot: it must be
// positive, and either the table has room or the score strictly beats the
// lowest entry.
func (t Table) Qualifies(score int) bool {
	if score <= 0 {
		return false
	}
	if len(t.Entries) < Max {
		return true
	}
	return score > t.Entries[len(t.Entries)-1].Score
}

// Rank returns the 1-based rank the score would take right now; Insert
// assigns it when the score Qualifies.
func (t Table) Rank(score int) int {
	rank := 1
	for _, e := range t.Entries {
		if e.Score >= score {
			rank++
		}
	}
	return rank
}

// Insert places a qualifying score after existing equal scores, evicts past
// Max, and returns its 1-based rank. A score Qualifies refuses leaves the
// table untouched and returns 0.
func (t *Table) Insert(score int, name string) (rank int) {
	if !t.Qualifies(score) {
		return 0
	}
	idx := t.Rank(score) - 1
	t.Entries = append(t.Entries, Entry{})
	copy(t.Entries[idx+1:], t.Entries[idx:])
	t.Entries[idx] = Entry{Score: score, Name: name}
	if len(t.Entries) > Max {
		t.Entries = t.Entries[:Max]
	}
	return idx + 1
}
