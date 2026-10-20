// SPDX-License-Identifier: GPL-2.0-or-later

package audio

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Resolver opens a sound file for an owner. Game code passes paths like
// "./assets/omnomnom.wav"; the resolver maps them to real files.
type Resolver func(owner, file string) (io.ReadCloser, error)

// Lister returns an owner's sound files (in the "./assets/foo.wav" form the
// resolver expects), so the mixer can preload them when the app loads.
type Lister func(owner string) []string

// DirResolver resolves sounds under root using the layout
// <root>/<owner>/<file>, so owner "games/snake" + "./assets/lost.wav"
// becomes <root>/games/snake/assets/lost.wav.
func DirResolver(root string) Resolver {
	return func(owner, file string) (io.ReadCloser, error) {
		path := filepath.Join(root, filepath.FromSlash(owner), filepath.Clean(filepath.FromSlash(file)))
		return os.Open(path)
	}
}

// DirLister lists an owner's .wav files under root, returned as owner-
// relative "./assets/..." paths so they feed straight back into a
// DirResolver-built resolver.
func DirLister(root string) Lister {
	return func(owner string) []string {
		dir := filepath.Join(root, filepath.FromSlash(owner))
		var out []string
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if strings.EqualFold(filepath.Ext(d.Name()), ".wav") {
				if rel, err := filepath.Rel(dir, path); err == nil {
					out = append(out, "./"+filepath.ToSlash(rel))
				}
			}
			return nil
		})
		return out
	}
}

// AssetOverlay serves sound bytes held in memory, such as a cartridge bundle's
// assets/ tree, which has no on-disk directory for DirResolver to address.
// Owners are registered per cartridge launch. Every method is nil-receiver-safe.
type AssetOverlay struct {
	mu      sync.RWMutex
	byOwner map[string]map[string][]byte
}

// NewAssetOverlay builds an empty overlay.
func NewAssetOverlay() *AssetOverlay {
	return &AssetOverlay{byOwner: map[string]map[string][]byte{}}
}

// Register makes owner's assets resolvable. A nil assets map is a no-op, so
// that owner's sounds fall through to the fallback resolver.
func (o *AssetOverlay) Register(owner string, assets map[string][]byte) {
	if o == nil || assets == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.byOwner[owner] = assets
}

// Unregister drops owner's assets.
func (o *AssetOverlay) Unregister(owner string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.byOwner, owner)
}

// Resolver wraps fallback with this overlay. A registered owner is served only
// from memory, and a miss is an error; an unregistered owner goes to fallback.
func (o *AssetOverlay) Resolver(fallback Resolver) Resolver {
	return func(owner, file string) (io.ReadCloser, error) {
		if o != nil {
			o.mu.RLock()
			m, ok := o.byOwner[owner]
			o.mu.RUnlock()
			if ok {
				if data, found := m[file]; found {
					return io.NopCloser(bytes.NewReader(data)), nil
				}
				return nil, fmt.Errorf("audio: %q not found in %s's cartridge bundle assets", file, owner)
			}
		}
		if fallback == nil {
			return nil, fmt.Errorf("audio: no resolver configured for owner %q", owner)
		}
		return fallback(owner, file)
	}
}
