// SPDX-License-Identifier: GPL-2.0-or-later

package screensaver

import (
	"sort"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/storage"
)

// Settings is one screensaver's settings file as its schedule editor sees it.
// The app's own green-button screen and SETTINGS both edit through it.
type Settings interface {
	// Editor returns an editor whose Schedule and Fields point into this value.
	Editor() Editor
	// Load replaces the value with the stored file, repaired; a missing file
	// loads the defaults. A read error is returned after falling back.
	Load(store *storage.Store) error
	// Save writes the value where the app keeps it and drops every cache that
	// would keep the old schedule in force.
	Save(rt *engine.Runtime) error
}

// Entry is a screensaver whose settings SETTINGS can open.
type Entry struct {
	Name string
	New  func() Settings // a value holding the app's defaults
}

var entries []Entry

// Register adds a screensaver to SETTINGS' list; call it from the app's init.
func Register(e Entry) { entries = append(entries, e) }

// Entries lists the registered screensavers in the order their default
// windows open through the day.
func Entries() []Entry {
	out := append([]Entry(nil), entries...)
	from := make(map[string]string, len(out))
	for _, e := range out {
		from[e.Name] = e.New().Editor().Schedule.From
	}
	sort.SliceStable(out, func(i, j int) bool { return from[out[i].Name] < from[out[j].Name] })
	return out
}
