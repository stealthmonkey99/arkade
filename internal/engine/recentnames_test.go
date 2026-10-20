// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/storage"
)

// recentRig builds the minimal Runtime the name memory needs: a manual clock so tests drive expiry deterministically (recentnames.go must never touch time.Now), and no Store, so the MRU stays in RAM.
func recentRig() (*Runtime, *clock.ManualClock) {
	clk := clock.NewManualClock()
	return &Runtime{Clock: clk}, clk
}

// recentStoreRig adds persistence rooted at dir; pass the same dir twice to simulate a restart.
func recentStoreRig(dir string) (*Runtime, *clock.ManualClock) {
	clk := clock.NewManualClock()
	return &Runtime{Clock: clk, Store: storage.New(dir)}, clk
}

// mru drains a seat's whole MRU through the public walk API, the way the
// ABI bridge does.
func mru(rt *Runtime, seat int) []string {
	var out []string
	for i := 0; ; i++ {
		name, ok := rt.RecentNameAt(seat, i)
		if !ok {
			return out
		}
		out = append(out, name)
	}
}

func mruEquals(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestRecentNameRoundTrip(t *testing.T) {
	rt, _ := recentRig()
	rt.NoteRecentName(0, "KARL")
	got, ok := rt.RecentName(0)
	if !ok || got != "KARL" {
		t.Errorf("expected KARL for seat 0, got %q ok=%v", got, ok)
	}
	if _, ok := rt.RecentName(1); ok {
		t.Error("seat 1 never submitted; nothing should be remembered for it")
	}
}

func TestRecentNameSeatsAreIndependent(t *testing.T) {
	rt, _ := recentRig()
	rt.NoteRecentName(0, "KARL")
	rt.NoteRecentName(1, "ZOE")
	if got, _ := rt.RecentName(0); got != "KARL" {
		t.Errorf("seat 0 should keep its own name, got %q", got)
	}
	if got, _ := rt.RecentName(1); got != "ZOE" {
		t.Errorf("seat 1 should keep its own name, got %q", got)
	}
}

func TestRecentNameExpiresAtExactlyOneHour(t *testing.T) {
	rt, clk := recentRig()
	rt.NoteRecentName(0, "KARL")

	clk.Advance(time.Hour - time.Millisecond)
	if got, ok := rt.RecentName(0); !ok || got != "KARL" {
		t.Errorf("just inside the TTL the name should survive, got %q ok=%v", got, ok)
	}

	clk.Advance(time.Millisecond) // elapsed == exactly 1h
	if _, ok := rt.RecentName(0); ok {
		t.Error("at exactly the 1h TTL the name must be expired (elapsed >= TTL)")
	}
}

func TestRecentNameExpiresPastOneHour(t *testing.T) {
	rt, clk := recentRig()
	rt.NoteRecentName(1, "ZOE")
	clk.Advance(time.Hour + time.Minute)
	if _, ok := rt.RecentName(1); ok {
		t.Error("well past the TTL the name must be gone")
	}
}

func TestRecentNameOverwriteByNewerRestartsTTL(t *testing.T) {
	rt, clk := recentRig()
	rt.NoteRecentName(0, "OLD")
	clk.Advance(50 * time.Minute)
	rt.NoteRecentName(0, "NEW") // overwrite re-stamps the clock

	if got, _ := rt.RecentName(0); got != "NEW" {
		t.Fatalf("newer submit should overwrite, got %q", got)
	}
	clk.Advance(30 * time.Minute) // 80min after OLD, 30min after NEW
	if got, ok := rt.RecentName(0); !ok || got != "NEW" {
		t.Errorf("TTL should run from the newest submit, got %q ok=%v", got, ok)
	}
	clk.Advance(31 * time.Minute) // 61min after NEW
	if _, ok := rt.RecentName(0); ok {
		t.Error("the overwritten entry must expire on the new timestamp")
	}
}

func TestRecentNameEmptySubmitNotRemembered(t *testing.T) {
	rt, _ := recentRig()
	rt.NoteRecentName(0, "")
	if _, ok := rt.RecentName(0); ok {
		t.Error("an anonymous (empty) submit must not be remembered")
	}
	// ...and must not clobber an existing memory either.
	rt.NoteRecentName(0, "KARL")
	rt.NoteRecentName(0, "")
	if got, _ := rt.RecentName(0); got != "KARL" {
		t.Errorf("an empty submit must not clobber the stored name, got %q", got)
	}
}

func TestRecentNameOutOfRangeSeatsIgnored(t *testing.T) {
	rt, _ := recentRig()
	rt.NoteRecentName(-1, "SYS") // keyboard/system pseudo-seat: not remembered
	rt.NoteRecentName(2, "TWO")
	for _, seat := range []int{-1, 0, 1, 2} {
		if _, ok := rt.RecentName(seat); ok {
			t.Errorf("seat %d should have nothing remembered", seat)
		}
	}
}

// MRU half: per-seat, no TTL, persistent. The TTL covers active-play auto-suggest, the MRU covers regulars and never decays; the two must not collapse into one value.

func TestRecentNameMRUOrderIsNewestFirst(t *testing.T) {
	rt, _ := recentRig()
	for _, name := range []string{"ANN", "BOB", "CAL"} {
		rt.NoteRecentName(0, name)
	}
	if got := mru(rt, 0); !mruEquals(got, "CAL", "BOB", "ANN") {
		t.Errorf("MRU = %v, want [CAL BOB ANN] — newest first", got)
	}
}

func TestRecentNameMRUDedupesByPromoting(t *testing.T) {
	rt, _ := recentRig()
	for _, name := range []string{"ANN", "BOB", "CAL", "ANN"} {
		rt.NoteRecentName(0, name)
	}
	// ANN moves to the front rather than appearing twice: a regular who
	// plays every day must not push everyone else out with copies of
	// themselves.
	if got := mru(rt, 0); !mruEquals(got, "ANN", "CAL", "BOB") {
		t.Errorf("MRU = %v, want [ANN CAL BOB] — a re-used name is promoted, never duplicated", got)
	}
}

func TestRecentNameMRUEvictsOldestPastFive(t *testing.T) {
	rt, _ := recentRig()
	for _, name := range []string{"ONE", "TWO", "TRE", "FOR", "FIV", "SIX"} {
		rt.NoteRecentName(0, name)
	}
	got := mru(rt, 0)
	if len(got) != recentNameMRUMax {
		t.Fatalf("MRU holds %d names, want %d", len(got), recentNameMRUMax)
	}
	if !mruEquals(got, "SIX", "FIV", "FOR", "TRE", "TWO") {
		t.Errorf("MRU = %v, want [SIX FIV FOR TRE TWO] — ONE falls off the end", got)
	}
	if _, ok := rt.RecentNameAt(0, recentNameMRUMax); ok {
		t.Error("reading past the end must report ok=false, not a sixth name")
	}
}

func TestRecentNameMRUNeverDecays(t *testing.T) {
	rt, clk := recentRig()
	rt.NoteRecentName(0, "KARL")
	clk.Advance(365 * 24 * time.Hour) // a year between visits

	if _, ok := rt.RecentName(0); ok {
		t.Error("the PREFILL must still expire — only the MRU is TTL-free")
	}
	if got, ok := rt.RecentNameAt(0, 0); !ok || got != "KARL" {
		t.Errorf("MRU[0] = (%q, %v) a year later, want (KARL, true) — the MRU has no TTL", got, ok)
	}
}

func TestRecentNameMRUSeatsAreIndependent(t *testing.T) {
	rt, _ := recentRig()
	rt.NoteRecentName(0, "KARL")
	rt.NoteRecentName(1, "ZOE")
	rt.NoteRecentName(0, "ANN")

	if got := mru(rt, 0); !mruEquals(got, "ANN", "KARL") {
		t.Errorf("seat 0 MRU = %v, want [ANN KARL] — seat 1's submit leaked in", got)
	}
	if got := mru(rt, 1); !mruEquals(got, "ZOE") {
		t.Errorf("seat 1 MRU = %v, want [ZOE] — seat 0's submits leaked in", got)
	}
}

func TestRecentNameMRUSurvivesARestart(t *testing.T) {
	dir := t.TempDir()

	before, _ := recentStoreRig(dir)
	before.NoteRecentName(0, "KARL")
	before.NoteRecentName(1, "ZOE")
	before.NoteRecentName(0, "ANN")

	// A reboot or redeploy restarts the process but leaves the data root alone.
	after, clk := recentStoreRig(dir)
	if got := mru(after, 0); !mruEquals(got, "ANN", "KARL") {
		t.Errorf("seat 0 MRU after restart = %v, want [ANN KARL]", got)
	}
	if got := mru(after, 1); !mruEquals(got, "ZOE") {
		t.Errorf("seat 1 MRU after restart = %v, want [ZOE]", got)
	}
	// The PREFILL is deliberately NOT restored: a fresh boot has nobody
	// sitting down yet, so auto-filling a name would be a guess about a
	// person, not a memory of one.
	if _, ok := after.RecentName(0); ok {
		t.Error("the 1h prefill must not survive a restart — it is RAM-only by design")
	}
	// ...and a reloaded MRU still promotes correctly on top of what it read.
	after.NoteRecentName(0, "KARL")
	clk.Advance(time.Minute)
	if got := mru(after, 0); !mruEquals(got, "KARL", "ANN") {
		t.Errorf("MRU after a post-restart submit = %v, want [KARL ANN]", got)
	}
}

func TestRecentNameMRUWritesWhereTheCabinetLooks(t *testing.T) {
	dir := t.TempDir()
	rt, _ := recentStoreRig(dir)
	rt.NoteRecentName(0, "KARL")

	// Pinned as a path, not just a round-trip: the cabinet's data root is
	// rsynced and hand-inspected, so the file's location is part of the
	// contract (data/engine/recentnames.json).
	path := filepath.Join(dir, "engine", "recentnames.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if want := `{"seats":[["KARL"],[]]}`; string(raw) != want {
		t.Errorf("persisted file = %s, want %s", raw, want)
	}
}

func TestRecentNameMRUWithoutAStoreStaysInRAM(t *testing.T) {
	// Every bare test rig — and any embedder that never assigned a Store —
	// must get the feature minus persistence, never a nil-pointer panic.
	rt, _ := recentRig()
	rt.NoteRecentName(0, "KARL")
	if got := mru(rt, 0); !mruEquals(got, "KARL") {
		t.Errorf("store-less MRU = %v, want [KARL]", got)
	}
}

func TestRecentNameMRUIgnoresBlankAndAbandonedNames(t *testing.T) {
	rt, _ := recentRig()
	rt.NoteRecentName(0, "KARL")
	// A player who confirms an untouched entry submits "" (ui.NameEntry.Name
	// drops trailing blanks); a player who ABANDONS entry never calls this at
	// all. Neither may land in the list of regulars.
	rt.NoteRecentName(0, "")
	rt.NoteRecentName(0, "   ")
	if got := mru(rt, 0); !mruEquals(got, "KARL") {
		t.Errorf("MRU = %v, want [KARL] — a blank submit must not be remembered", got)
	}
	// Surrounding blanks are trimmed rather than making a second entry, so
	// " KARL" and "KARL" are the same regular.
	rt.NoteRecentName(0, " KARL ")
	if got := mru(rt, 0); !mruEquals(got, "KARL") {
		t.Errorf("MRU = %v, want [KARL] — a padded name must dedupe against the trimmed one", got)
	}
}

func TestRecentNameMRUOutOfRangeSeatsAndIndexes(t *testing.T) {
	rt, _ := recentRig()
	rt.NoteRecentName(-1, "SYS")
	rt.NoteRecentName(2, "TWO")
	for _, seat := range []int{-1, 0, 1, 2} {
		if _, ok := rt.RecentNameAt(seat, 0); ok {
			t.Errorf("seat %d should have an empty MRU", seat)
		}
	}
	rt.NoteRecentName(0, "KARL")
	if _, ok := rt.RecentNameAt(0, -1); ok {
		t.Error("a negative index must report ok=false")
	}
}

func TestRecentNameMRUToleratesAHandEditedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "engine"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Blanks, a duplicate, an over-long list and a missing second seat must degrade to a sane list, not a failed boot.
	raw := `{"seats":[["ANN","","ANN"," BOB ","CAL","DEE","EVE","FAY"]]}`
	if err := os.WriteFile(filepath.Join(dir, "engine", "recentnames.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, _ := recentStoreRig(dir)
	if got := mru(rt, 0); !mruEquals(got, "ANN", "BOB", "CAL", "DEE", "EVE") {
		t.Errorf("MRU = %v, want [ANN BOB CAL DEE EVE] — blanks and duplicates dropped, capped at five", got)
	}
	if got := mru(rt, 1); len(got) != 0 {
		t.Errorf("seat 1 MRU = %v, want empty — the file names only one seat", got)
	}
}
