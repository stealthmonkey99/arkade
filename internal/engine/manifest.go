// SPDX-License-Identifier: GPL-2.0-or-later

// Package engine owns the game loop, the app lifecycle and the app
// registry.
package engine

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/stealthmonkey99/arkade/internal/display"
)

// AppType categorizes registry entries.
type AppType string

const (
	TypeBoot    AppType = "boot"
	TypeShell   AppType = "shell"
	TypeApp     AppType = "app"
	TypeGame    AppType = "game"
	TypeAttract AppType = "attract"
)

// CapabilityMic is the bundle-manifest capability that opts an app in to microphone events
// (input.KeyMicWhistle and friends); withheld by default so older cartridges are unaffected.
const CapabilityMic = "mic"

// Manifest describes an installable app. Factory-produced apps receive an AppContext scoped to it.
type Manifest struct {
	// Slug is the unique id and storage/layer owner, e.g. "games/snake".
	Slug        string
	Name        string
	Description string
	Type        AppType
	// Version is the app's own release version; Author, Publisher and Audience likewise come from the
	// bundle manifest. All are "" for native apps.
	Version   string
	Author    string
	Publisher string
	Audience  string
	// WasmSHA256 is the lowercase hex SHA-256 of the .wasm bytes this manifest launches, "" for native
	// apps. A flight recording pins it and replay refuses a mismatch; it is also the engine's test for
	// "is this a cartridge?".
	WasmSHA256 string
	// Visibility is the bundle distribution tier ("public", "play-only", "private"); the shell links a
	// game's web play page only for public/play-only.
	Visibility string
	// Tags classify the app for the shell's category browser; untagged apps appear only in the catch-all shelf.
	Tags []string
	// Players lists supported modes ("1p", "2p-coop", "2p-versus", "alternating"). Empty means undeclared
	// and matches every player-count filter.
	Players []string
	// Icon is the shell menu's 8x8 sprite; Logo is optional larger art for the detail view.
	Icon display.Sprite
	Logo display.Sprite
	// Capabilities are the bundle manifest's permission requests. The engine acts only on "mic"; unknown
	// entries are ignored.
	Capabilities []string
	// Hidden apps (boot, shell) don't appear in the shell menu.
	Hidden bool
	// ExitsOnQuit exits the whole engine when the app unloads (shell).
	ExitsOnQuit bool
	// SortOrder positions the app in a fresh shell menu (lower = higher).
	SortOrder int
	// RawDirections opts out of remapEvent's seat-mount and display-orientation remapping of controller
	// directions, for egocentric schemes (tank rotate/thrust) where a direction is relative to the avatar,
	// not the panel. This is the initial state; Runtime.SetRawDirectionsOverride changes it at runtime.
	RawDirections bool
	// New constructs the app.
	New func(ctx *AppContext) App

	// NewAttract, if non-nil, builds a custom attract renderer bound to ctx (cartridges exporting
	// cart_attract). ok=false falls back to the default showcase.
	NewAttract func(ctx *AppContext) (r AttractRenderer, ok bool)

	// ScreensaverGate, on a TypeScreensaver manifest, must also return true for the screensaver to be
	// eligible. It runs on the tick goroutine about once a second, so it must be cheap and not block.
	// nil means the schedule alone decides.
	ScreensaverGate func(rt *Runtime, now time.Time) bool

	// DefaultSchedule, on a TypeScreensaver manifest, applies until the operator's settings file exists;
	// a stored schedule, even a disabled one, then wins.
	DefaultSchedule Schedule
}

// AttractRenderer draws one app's custom attract slot over the engine's starfield. Attract advances and
// draws a frame each tick into a layer the renderer owns (pixels with alpha < 255 show the stars);
// Close tears it down. Anything it leaves untouched, LCDs and lamps included, keeps attract's default.
type AttractRenderer interface {
	Attract(dtMicros uint64)
	Close()
}

var (
	regMu    sync.Mutex
	registry []Manifest
)

// ErrSlugTaken is Register's refusal of a slug already in the registry.
var ErrSlugTaken = errors.New("slug already registered")

// Register adds a manifest to the global registry, refusing a slug already there with ErrSlugTaken. It
// returns an error rather than panicking because cartridges register at runtime, where a collision is
// an expected skip. Safe from any goroutine.
func Register(m Manifest) error {
	regMu.Lock()
	defer regMu.Unlock()
	for _, existing := range registry {
		if existing.Slug == m.Slug {
			return fmt.Errorf("register %q: %w", m.Slug, ErrSlugTaken)
		}
	}
	registry = append(registry, m)
	return nil
}

// Unregister removes the manifest with the given slug and reports whether it found one. Safe from any
// goroutine.
func Unregister(slug string) bool {
	regMu.Lock()
	defer regMu.Unlock()
	for i, m := range registry {
		if m.Slug == slug {
			registry = append(registry[:i:i], registry[i+1:]...)
			return true
		}
	}
	return false
}

// Apps returns visible manifests sorted by SortOrder then name.
func Apps() []Manifest {
	regMu.Lock()
	defer regMu.Unlock()
	var out []Manifest
	for _, m := range registry {
		if !m.Hidden {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Lookup finds a manifest by slug.
func Lookup(slug string) (Manifest, error) {
	regMu.Lock()
	defer regMu.Unlock()
	for _, m := range registry {
		if m.Slug == slug {
			return m, nil
		}
	}
	return Manifest{}, fmt.Errorf("no app registered for slug %q", slug)
}

// lookupType finds the first manifest of a type (boot, shell).
func lookupType(t AppType) (Manifest, bool) {
	regMu.Lock()
	defer regMu.Unlock()
	for _, m := range registry {
		if m.Type == t {
			return m, true
		}
	}
	return Manifest{}, false
}
