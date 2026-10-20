// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"strings"
	"sync"
	"time"

	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// recentNameTTL is how long a submitted name is offered back as a prefill for its seat. It governs
// RecentName only; the MRU never expires.
const recentNameTTL = time.Hour

// recentNameMRUMax is the names kept per seat: as many as a name entry offers.
const recentNameMRUMax = ui.NameEntryMaxRecents

// recentNamesOwner/recentNamesLabel place the MRU at <data>/engine/recentnames.json. The owner is
// "engine" because no single app owns cabinet-wide state.
const (
	recentNamesOwner = "engine"
	recentNamesLabel = "recentnames"
)

// recentNamesFile is the on-disk shape, one newest-first list per physical seat:
//
//	{"seats":[["KARL","ZOE"],["ZOE"]]}
type recentNamesFile struct {
	Seats [][]string `json:"seats"`
}

// recentSeat holds one seat's expiring prefill (name/at) and its MRU (mru[:mruN], newest first).
type recentSeat struct {
	name string
	at   time.Time

	mru  [recentNameMRUMax]string
	mruN int
}

// recentNames is the per-seat name memory, keyed on physical controller index 0/1; seats never share.
// Each seat has an expiring prefill (RecentName, measured on rt.Clock) and a persisted MRU with no
// expiry (RecentNameAt). The MRU loads lazily because Runtime.Store is assignable and bare test rigs
// never set one.
type recentNames struct {
	mu     sync.Mutex
	seats  [2]recentSeat
	loaded bool
}

// NoteRecentName records name for a physical seat (0 or 1) in both memories: it re-stamps the prefill,
// promotes the name in the MRU and writes the MRU to disk. Blank names and other seats are ignored.
func (rt *Runtime) NoteRecentName(player int, name string) {
	name = strings.TrimSpace(name)
	if name == "" || player < 0 || player >= len(rt.recent.seats) {
		return
	}
	rt.recent.mu.Lock()
	rt.recent.load(rt)
	seat := &rt.recent.seats[player]
	seat.name = name
	seat.at = rt.Clock.Now()
	seat.promote(name)
	snapshot := rt.recent.snapshot()
	rt.recent.mu.Unlock()

	rt.saveRecentNames(snapshot)
}

// RecentName returns the name a seat submitted within recentNameTTL (expired at exactly the TTL), or
// ok=false.
func (rt *Runtime) RecentName(player int) (string, bool) {
	if player < 0 || player >= len(rt.recent.seats) {
		return "", false
	}
	rt.recent.mu.Lock()
	defer rt.recent.mu.Unlock()
	seat := rt.recent.seats[player]
	if seat.name == "" || rt.Clock.Now().Sub(seat.at) >= recentNameTTL {
		return "", false
	}
	return seat.name, true
}

// RecentNameAt returns the index-th name in a seat's MRU, newest first, or ok=false past the end.
func (rt *Runtime) RecentNameAt(player, index int) (string, bool) {
	if player < 0 || player >= len(rt.recent.seats) || index < 0 || index >= recentNameMRUMax {
		return "", false
	}
	rt.recent.mu.Lock()
	defer rt.recent.mu.Unlock()
	rt.recent.load(rt)
	seat := &rt.recent.seats[player]
	if index >= seat.mruN {
		return "", false
	}
	return seat.mru[index], true
}

// promote moves name to the front of the seat's MRU, dropping a duplicate or else the oldest entry.
func (s *recentSeat) promote(name string) {
	cut := s.mruN
	for i := 0; i < s.mruN; i++ {
		if s.mru[i] == name {
			cut = i
			break
		}
	}
	if cut >= recentNameMRUMax {
		cut = recentNameMRUMax - 1 // list full and the name is new: evict the oldest
	}
	for i := cut; i > 0; i-- {
		s.mru[i] = s.mru[i-1]
	}
	s.mru[0] = name
	if cut == s.mruN && s.mruN < recentNameMRUMax {
		s.mruN++
	}
}

// load reads the persisted MRU on first touch; caller holds mu. Without a Store the MRU stays in RAM.
func (r *recentNames) load(rt *Runtime) {
	if r.loaded {
		return
	}
	r.loaded = true // one attempt, even if it fails: never retry per call
	if rt.Store == nil {
		return
	}
	var f recentNamesFile
	if err := rt.Store.Read(recentNamesOwner, recentNamesLabel, &f); err != nil {
		rt.logRecentNames("warn", "recent names: "+err.Error())
		return
	}
	for seat := range r.seats {
		if seat >= len(f.Seats) {
			break
		}
		for _, name := range f.Seats[seat] {
			// Appended in file order, not promoted, so a hand-edited file keeps its order.
			r.seats[seat].appendUnique(strings.TrimSpace(name))
		}
	}
}

// appendUnique adds name to the end of the MRU unless blank, present or full. Load-time only.
func (s *recentSeat) appendUnique(name string) {
	if name == "" || s.mruN >= recentNameMRUMax {
		return
	}
	for i := 0; i < s.mruN; i++ {
		if s.mru[i] == name {
			return
		}
	}
	s.mru[s.mruN] = name
	s.mruN++
}

// snapshot copies the MRUs into the on-disk shape; caller holds mu.
func (r *recentNames) snapshot() recentNamesFile {
	f := recentNamesFile{Seats: make([][]string, len(r.seats))}
	for seat := range r.seats {
		// make-then-append so an untouched seat marshals as [] rather than null.
		names := make([]string, 0, r.seats[seat].mruN)
		f.Seats[seat] = append(names, r.seats[seat].mru[:r.seats[seat].mruN]...)
	}
	return f
}

// saveRecentNames persists the snapshot; a write failure is logged, not allowed to fail a score submit.
func (rt *Runtime) saveRecentNames(f recentNamesFile) {
	if rt.Store == nil {
		return
	}
	if err := rt.Store.Write(recentNamesOwner, recentNamesLabel, f); err != nil {
		rt.logRecentNames("warn", "recent names: "+err.Error())
	}
}

// logRecentNames is Log with a nil-logger guard for bare test rigs.
func (rt *Runtime) logRecentNames(level, msg string) {
	if rt.logger == nil {
		return
	}
	rt.logger(level, msg)
}
