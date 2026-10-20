// SPDX-License-Identifier: GPL-2.0-or-later

package shell

import (
	"sort"
	"strings"

	bluetoothapp "github.com/stealthmonkey99/arkade/apps/bluetooth"
	"github.com/stealthmonkey99/arkade/internal/cartridge/rcart"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
)

// Synthetic category ids. Real categories use the manifest tag itself as
// the id (the rcart schema's lowercase-hyphen vocabulary).
const (
	catRecent    = "recent"
	catFavorites = "favorites"
	catAll       = "all"
	catSystem    = "system"
)

// curatedTags fixes the shelf order for the manifest tag vocabulary; other tags follow alphabetically.
var curatedTags = []string{
	"puzzle", "arcade", "action", "strategy", "board", "cards", "shooter",
	"platformer", "racing", "sports", "party", "sim", "trivia", "classic",
}

var accents = map[string]display.Pixel{
	catRecent:    {R: 236, G: 236, B: 236, A: 255},
	catFavorites: {R: 240, G: 200, B: 40, A: 255}, // the yellow C button's color
	catAll:       {R: 168, G: 130, B: 255, A: 255},
	catSystem:    {R: 150, G: 160, B: 170, A: 255},
	"arcade":     {R: 255, G: 180, B: 40, A: 255},
	"action":     {R: 255, G: 90, B: 80, A: 255},
	"puzzle":     {R: 60, G: 220, B: 130, A: 255},
	"strategy":   {R: 80, G: 150, B: 255, A: 255},
	"board":      {R: 216, G: 160, B: 100, A: 255},
	"cards":      {R: 70, G: 200, B: 200, A: 255},
	"shooter":    {R: 255, G: 130, B: 40, A: 255},
	"platformer": {R: 170, G: 230, B: 60, A: 255},
	"racing":     {R: 60, G: 210, B: 255, A: 255},
	"sports":     {R: 240, G: 220, B: 60, A: 255},
	"sim":        {R: 190, G: 120, B: 255, A: 255},
	"party":      {R: 255, G: 110, B: 200, A: 255},
	"trivia":     {R: 255, G: 160, B: 170, A: 255},
	"classic":    {R: 220, G: 190, B: 90, A: 255},
}

var defaultAccent = display.Pixel{R: 200, G: 200, B: 200, A: 255}

func accentFor(id string) display.Pixel {
	if p, ok := accents[id]; ok {
		return p
	}
	return defaultAccent
}

// entryKind separates apps from the shell's own control tiles.
type entryKind int

const (
	entryApp entryKind = iota
	entrySearch
	entryVolume
	entryWifi
	entryBluetooth
	entrySleep
	entryShutdown
	entryTemp
	entryBrightness
)

// entry is one shelf tile: an app manifest, or a control.
type entry struct {
	kind  entryKind
	m     engine.Manifest // entryApp, and the app entryBluetooth opens
	label string          // control tiles; apps use m.Name
}

func appEntry(m engine.Manifest) entry { return entry{kind: entryApp, m: m} }

func (e entry) name() string {
	if e.kind == entryApp {
		return e.m.Name
	}
	return e.label
}

// interactable reports whether D does anything on this tile: only games have a detail card.
func (e entry) interactable() bool {
	return e.kind == entryApp && e.m.Type == engine.TypeGame
}

// systemControls are the SYSTEM shelf's built-in tiles. SLEEP leads because
// left/right on the first tile must flip shelves, which a slider would swallow.
// BLUETOOTH is a hidden app resolved by slug; without it registered there is no tile.
func systemControls() []entry {
	out := []entry{
		controlEntry(entrySleep),
		controlEntry(entryVolume),
		controlEntry(entryBrightness),
		controlEntry(entryWifi),
	}
	if m, err := engine.Lookup(bluetoothapp.Slug); err == nil {
		bt := controlEntry(entryBluetooth)
		bt.m = m
		out = append(out, bt)
	}
	return append(out, controlEntry(entryTemp), controlEntry(entryShutdown))
}

type category struct {
	id      string
	label   string
	accent  display.Pixel
	entries []entry
}

// buildCatalog derives the shelves: RECENT, FAVORITES, ALL (search tile first),
// one per game tag, and SYSTEM (controls, then non-game apps). Empty shelves are dropped.
func buildCatalog(apps []engine.Manifest, mru, favorites []string) []category {
	var cats []category
	add := func(id, label string, list []entry) {
		if len(list) > 0 {
			cats = append(cats, category{id: id, label: label, accent: accentFor(id), entries: list})
		}
	}
	appEntries := func(ms []engine.Manifest) []entry {
		out := make([]entry, len(ms))
		for i, m := range ms {
			out[i] = appEntry(m)
		}
		return out
	}

	bySlug := map[string]engine.Manifest{}
	for _, m := range apps {
		bySlug[m.Slug] = m
	}
	var recent []engine.Manifest
	for _, slug := range mru {
		if m, ok := bySlug[slug]; ok && len(recent) < recentCap {
			recent = append(recent, m)
		}
	}
	add(catRecent, "RECENT", appEntries(recent))
	var faved []engine.Manifest
	for _, slug := range favorites {
		if m, ok := bySlug[slug]; ok {
			faved = append(faved, m)
		}
	}
	add(catFavorites, "FAVORITES", appEntries(faved))
	add(catAll, "ALL", append([]entry{{kind: entrySearch, label: "SEARCH"}}, appEntries(apps)...))

	byTag := map[string][]engine.Manifest{}
	var system []engine.Manifest
	for _, m := range apps {
		if m.Type != engine.TypeGame {
			system = append(system, m)
			continue
		}
		for _, tag := range m.Tags {
			byTag[tag] = append(byTag[tag], m)
		}
	}
	curated := map[string]bool{}
	for _, tag := range curatedTags {
		curated[tag] = true
		add(tag, strings.ToUpper(tag), appEntries(byTag[tag]))
	}
	var extras []string
	for tag := range byTag {
		if !curated[tag] {
			extras = append(extras, tag)
		}
	}
	sort.Strings(extras)
	for _, tag := range extras {
		add(tag, strings.ToUpper(tag), appEntries(byTag[tag]))
	}
	add(catSystem, "SYSTEM", append(systemControls(), appEntries(system)...))
	return cats
}

// searchMatches filters apps by a case-insensitive name query, prefix matches first.
func searchMatches(apps []engine.Manifest, query string) []entry {
	q := strings.ToLower(query)
	if q == "" {
		return nil
	}
	var prefix, within []entry
	for _, m := range apps {
		name := strings.ToLower(m.Name)
		switch {
		case strings.HasPrefix(name, q):
			prefix = append(prefix, appEntry(m))
		case strings.Contains(name, q):
			within = append(within, appEntry(m))
		}
	}
	return append(prefix, within...)
}

// recentCap bounds the RECENT shelf.
const recentCap = 10

// playersBadge returns "1P", "2P" or "1-2P", or "" when no modes are declared.
func playersBadge(modes []string) string {
	pm := rcart.PlayerModes(modes)
	solo, duo := pm.SupportsCount(1), pm.SupportsCount(2)
	switch {
	case solo && duo:
		return "1-2P"
	case duo:
		return "2P"
	case solo:
		return "1P"
	default:
		return ""
	}
}

// playersLine spells the modes out for the detail card, joined with " + ".
func playersLine(modes []string) string {
	names := map[string]string{
		rcart.PlayersSolo:        "1P",
		rcart.PlayersCoop:        "2P CO-OP",
		rcart.PlayersVersus:      "2P VS",
		rcart.PlayersAlternating: "2P TURNS",
		rcart.PlayersLegacyTwo:   "2P",
	}
	var parts []string
	for _, m := range modes {
		if n, ok := names[m]; ok {
			parts = append(parts, n)
		}
	}
	return strings.Join(parts, " + ")
}
