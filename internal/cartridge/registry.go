// SPDX-License-Identifier: GPL-2.0-or-later

// Package cartridge discovers .rcart bundles and bare .wasm files on disk,
// registers each as an engine app, and polls for changes so a USB stick can be
// swapped while the cabinet runs. Every launch instantiates the cartridge
// fresh from a cold cart_init.
package cartridge

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/stealthmonkey99/arkade/internal/cartridge/rcart"
	"github.com/stealthmonkey99/arkade/internal/cartridge/wasm"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
)

// SlugPrefix namespaces every cartridge-derived engine.Manifest so it can
// never shadow a compiled-in app's slug.
const SlugPrefix = "cart/"

var cartridgeExtensions = []string{".rcart", ".wasm"}

// Options configures a Registry.
type Options struct {
	// Dirs are glob patterns scanned for cartridges, expanded fresh on every
	// Scan so a USB drive mounted later under an unpredictable label is
	// picked up.
	Dirs []string
	// Width and Height are the cart_init dimensions; zero means the
	// cabinet's display.DefaultWidth and DefaultHeight.
	Width, Height int
	// Logger receives level/message log lines; nil discards them.
	Logger func(level, msg string)
}

type trackedCart struct {
	path    string
	modTime time.Time
	size    int64
}

// Registry scans directories for cartridges and registers or unregisters
// them as engine apps.
type Registry struct {
	mu     sync.Mutex
	dirs   []string
	width  int
	height int
	logger func(level, msg string)

	tracked map[string]trackedCart // by unprefixed slug; only registrations this registry made

	// seenSkipped holds collision-skipped files by path, so a still-inserted
	// stick is not rediscovered every poll.
	seenSkipped map[string]trackedCart
}

// ScanResult summarizes one Scan call, for logging and tests.
type ScanResult struct {
	Added   []string // prefixed slugs newly registered
	Removed []string // prefixed slugs unregistered (file changed or disappeared)
	Skipped []string // "path: reason" for parse errors and collisions

	// Discovered lists every cleanly parsed cartridge at a previously
	// untracked path, collisions included: the physical-insert signal.
	// An in-place update to a tracked path is not a discovery.
	Discovered []string
}

func NewRegistry(opts Options) *Registry {
	width, height := opts.Width, opts.Height
	if width == 0 {
		width = display.DefaultWidth
	}
	if height == 0 {
		height = display.DefaultHeight
	}
	logger := opts.Logger
	if logger == nil {
		logger = func(string, string) {}
	}
	dirs := make([]string, len(opts.Dirs))
	copy(dirs, opts.Dirs)
	return &Registry{
		dirs:        dirs,
		width:       width,
		height:      height,
		logger:      logger,
		tracked:     map[string]trackedCart{},
		seenSkipped: map[string]trackedCart{},
	}
}

func prefixSlug(slug string) string { return SlugPrefix + slug }

// scans reports whether dir is one of the registry's scan patterns or a
// directory one of them matches.
func (r *Registry) scans(dir string) bool {
	dir = filepath.Clean(dir)
	for _, pattern := range r.dirs {
		if filepath.Clean(pattern) == dir {
			return true
		}
		if ok, err := filepath.Match(pattern, dir); err == nil && ok {
			return true
		}
	}
	return false
}

// Scan registers new and changed cartridges and unregisters vanished ones.
// An unchanged file (same mtime and size) is left alone, and a prefixed slug
// already registered elsewhere is logged and skipped. Safe to call
// repeatedly and from any goroutine.
func (r *Registry) Scan() ScanResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	var res ScanResult
	found := r.discover()

	// A skipped file that reappears counts as a fresh discovery.
	for p := range r.seenSkipped {
		if _, ok := found[p]; !ok {
			delete(r.seenSkipped, p)
		}
	}

	for slug, tc := range r.tracked {
		if _, ok := found[tc.path]; ok {
			continue
		}
		prefixed := prefixSlug(slug)
		engine.Unregister(prefixed)
		delete(r.tracked, slug)
		res.Removed = append(res.Removed, prefixed)
		r.logger("info", fmt.Sprintf("cartridge: %s removed (%s no longer present)", prefixed, tc.path))
	}

	// Sorted, so between two new files the first path wins a collision.
	paths := make([]string, 0, len(found))
	for p := range found {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		info := found[p]
		if sk, ok := r.seenSkipped[p]; ok {
			if sk.modTime.Equal(info.ModTime()) && sk.size == info.Size() {
				continue
			}
			delete(r.seenSkipped, p)
		}
		prevSlug, wasTracked := r.slugForPath(p)
		if wasTracked {
			tc := r.tracked[prevSlug]
			if tc.modTime.Equal(info.ModTime()) && tc.size == info.Size() {
				continue
			}
		}

		bundle, err := rcart.LoadFile(p)
		if err != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s: %v", p, err))
			r.logger("warn", fmt.Sprintf("cartridge: %s: %v", p, err))
			continue
		}
		for _, w := range bundle.Warnings {
			r.logger("warn", fmt.Sprintf("cartridge: %s: %s", p, w))
		}
		if !wasTracked {
			res.Discovered = append(res.Discovered, prefixSlug(bundle.Manifest.Slug))
		}

		if wasTracked {
			oldSlug := prefixSlug(prevSlug)
			engine.Unregister(oldSlug)
			delete(r.tracked, prevSlug)
			res.Removed = append(res.Removed, oldSlug)
			r.logger("info", fmt.Sprintf("cartridge: %s unregistered (content of %s changed)", oldSlug, p))
		}

		slug := prefixSlug(bundle.Manifest.Slug)
		if err := engine.Register(r.buildManifest(slug, bundle)); err != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s: slug %q already registered, skipped", p, slug))
			r.seenSkipped[p] = trackedCart{path: p, modTime: info.ModTime(), size: info.Size()}
			r.logger("info", fmt.Sprintf("cartridge: %s: slug %q already registered, playing the installed copy", p, slug))
			continue
		}
		r.tracked[bundle.Manifest.Slug] = trackedCart{path: p, modTime: info.ModTime(), size: info.Size()}
		res.Added = append(res.Added, slug)
		r.logger("info", fmt.Sprintf("cartridge: %s registered from %s", slug, p))
	}

	return res
}

func (r *Registry) slugForPath(path string) (string, bool) {
	for slug, tc := range r.tracked {
		if tc.path == path {
			return slug, true
		}
	}
	return "", false
}

// discover ignores unreadable directories: an unmounted USB path is the
// normal state between insertions.
func (r *Registry) discover() map[string]os.FileInfo {
	found := map[string]os.FileInfo{}
	for _, pattern := range r.dirs {
		dirs, err := filepath.Glob(pattern)
		if err != nil || len(dirs) == 0 {
			continue
		}
		for _, dir := range dirs {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				if !hasCartridgeExtension(e.Name()) {
					continue
				}
				full := filepath.Join(dir, e.Name())
				info, err := os.Stat(full)
				if err != nil {
					continue
				}
				found[full] = info
			}
		}
	}
	return found
}

func hasCartridgeExtension(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, e := range cartridgeExtensions {
		if ext == e {
			return true
		}
	}
	return false
}

// buildManifest captures only the parsed bytes, so every New builds a fresh
// cartridge instance.
func (r *Registry) buildManifest(slug string, bundle *rcart.Bundle) engine.Manifest {
	wasmBytes := bundle.Wasm
	assets := bundle.Assets // nil for a bare .wasm
	mount, ignored := bundle.Manifest.MountAssets()
	for _, w := range ignored {
		r.logger("warn", fmt.Sprintf("cartridge: %s: %s", slug, w))
	}
	width, height := r.width, r.height

	m := engine.Manifest{
		Slug:        slug,
		Name:        bundle.Manifest.DisplayTitle(),
		Description: bundle.Manifest.Description,
		Type:        engine.TypeGame,
		Version:     bundle.Manifest.Version,
		// Pins a flight recording to a build; the engine also uses it to tell
		// a cartridge manifest from a native one.
		WasmSHA256:    fmt.Sprintf("%x", sha256.Sum256(wasmBytes)),
		Author:        bundle.Manifest.Author,
		Publisher:     bundle.Manifest.Publisher,
		Audience:      bundle.Manifest.Audience,
		Visibility:    bundle.Manifest.Visibility,
		Tags:          bundle.Manifest.Tags,
		Players:       bundle.Manifest.Players,
		Icon:          bundle.Icon,
		Logo:          bundle.Logo,
		Capabilities:  bundle.Manifest.Capabilities,
		RawDirections: bundle.Manifest.RawDirections,
		New: func(ctx *engine.AppContext) engine.App {
			return wasm.NewApp(ctx, wasmBytes, assets, wasm.Options{
				Width: width, Height: height,
				BundleGlyphs: mount.Glyphs,
				BundleHID:    mount.HID,
				BundleMic:    mount.Mic,
				BundleLights: mount.Lights,
			})
		},
	}
	// A nil NewAttract keeps a cartridge without cart_attract out of the
	// screensaver entirely.
	if wasm.ExportsAttract(context.Background(), wasmBytes) {
		m.NewAttract = func(ctx *engine.AppContext) (engine.AttractRenderer, bool) {
			return newAttractRenderer(ctx, slug, wasmBytes, width, height, assets, mount)
		}
	}
	return m
}
