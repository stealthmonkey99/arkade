// SPDX-License-Identifier: GPL-2.0-or-later

package cartridge

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/stealthmonkey99/arkade/internal/cartridge/rcart"
)

// VariantsDirName is the data-root subdirectory holding stored builds of one
// slug side by side, as variants/<variant>/<slug>.rcart. The registry never
// scans it, so a stored build is inert until SetVariant activates it.
const VariantsDirName = "variants"

// preferredVariantOrder leads the display order; other names sort after.
var preferredVariantOrder = []string{"public", "private"}

// hashEntry caches a content hash keyed on size plus mtime, so a dashboard
// refresh does not re-hash tens of megabytes of bundles.
type hashEntry struct {
	size int64
	mod  time.Time
	sum  [sha256.Size]byte
}

// Variants reports every cartridge slug with more than one stored build.
// available maps the engine slug to its variant names in display order;
// active maps it to the variant whose bytes are live, omitting slugs whose
// live file matches none. Both are nil when there is no variants tree. The
// plain map types let internal/net declare the interface without importing
// this package.
func (c *RemoteControl) Variants() (available map[string][]string, active map[string]string) {
	if c.variantsDir == "" {
		return nil, nil
	}
	dirs, err := os.ReadDir(c.variantsDir)
	if err != nil {
		return nil, nil
	}

	bySlug := map[string][]string{}
	for _, d := range dirs {
		if !d.IsDir() || !safeNameRE.MatchString(d.Name()) {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(c.variantsDir, d.Name()))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".rcart" {
				continue
			}
			slug := strings.TrimSuffix(e.Name(), ".rcart")
			if !safeNameRE.MatchString(slug) {
				continue
			}
			bySlug[slug] = append(bySlug[slug], d.Name())
		}
	}

	for slug, names := range bySlug {
		if len(names) < 2 {
			continue
		}
		sortVariantNames(names)
		if available == nil {
			available, active = map[string][]string{}, map[string]string{}
		}
		prefixed := prefixSlug(slug)
		available[prefixed] = names
		if a := c.activeVariant(slug, names); a != "" {
			active[prefixed] = a
		}
	}
	return available, active
}

// activeVariant reports which of names the live bundle is byte-identical to,
// or "" for none. Only same-size candidates are hashed.
func (c *RemoteControl) activeVariant(slug string, names []string) string {
	live := filepath.Join(c.dir, slug+".rcart")
	liveInfo, err := os.Stat(live)
	if err != nil {
		return ""
	}
	var liveSum [sha256.Size]byte
	haveLiveSum := false
	for _, name := range names {
		cand := filepath.Join(c.variantsDir, name, slug+".rcart")
		candInfo, err := os.Stat(cand)
		if err != nil || candInfo.Size() != liveInfo.Size() {
			continue
		}
		if !haveLiveSum {
			liveSum, err = c.fileSum(live, liveInfo)
			if err != nil {
				return ""
			}
			haveLiveSum = true
		}
		candSum, err := c.fileSum(cand, candInfo)
		if err != nil {
			continue
		}
		if candSum == liveSum {
			return name
		}
	}
	return ""
}

// fileSum returns path's SHA-256, from the cache when size and mtime are
// unchanged since it was last read.
func (c *RemoteControl) fileSum(path string, info os.FileInfo) ([sha256.Size]byte, error) {
	c.mu.Lock()
	if e, ok := c.hashes[path]; ok && e.size == info.Size() && e.mod.Equal(info.ModTime()) {
		c.mu.Unlock()
		return e.sum, nil
	}
	c.mu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [sha256.Size]byte{}, err
	}
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))

	c.mu.Lock()
	if c.hashes == nil {
		c.hashes = map[string]hashEntry{}
	}
	c.hashes[path] = hashEntry{size: info.Size(), mod: info.ModTime(), sum: sum}
	c.mu.Unlock()
	return sum, nil
}

// SetVariant installs the stored variant build of slug, prefixed or bare,
// into the live library, re-registers it and returns its display name.
// A build that does not parse, or whose manifest names a different slug, is
// refused before anything is written, so scores stay filed under one slug.
// A running instance keeps the build it started with; only the next launch
// sees the swap.
func (c *RemoteControl) SetVariant(slug, variant string) (name string, err error) {
	if c.variantsDir == "" {
		return "", fmt.Errorf("cartridge: no build-variant store configured")
	}
	slug = strings.TrimPrefix(slug, SlugPrefix)
	if !safeNameRE.MatchString(slug) {
		return "", fmt.Errorf("cartridge: %q is not a valid cartridge slug", slug)
	}
	if !safeNameRE.MatchString(variant) {
		return "", fmt.Errorf("cartridge: %q is not a valid variant name", variant)
	}

	src := filepath.Join(c.variantsDir, variant, slug+".rcart")
	bundle, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("cartridge: reading %s build of %s: %w", variant, slug, err)
	}
	b, err := rcart.Load(bytes.NewReader(bundle))
	if err != nil {
		return "", fmt.Errorf("cartridge: %s build of %s is not a valid bundle: %w", variant, slug, err)
	}
	if b.Manifest.Slug != slug {
		return "", fmt.Errorf("cartridge: %s build stored as %s.rcart declares slug %q; refusing to install it under a slug it does not claim", variant, slug, b.Manifest.Slug)
	}

	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return "", fmt.Errorf("cartridge: preparing library dir %s: %w", c.dir, err)
	}
	if err := c.checkRoom(len(bundle)); err != nil {
		return "", err
	}
	dest := filepath.Join(c.dir, slug+".rcart")
	if err := writeFileAtomic(dest, bundle); err != nil {
		return "", fmt.Errorf("cartridge: writing %s: %w", dest, err)
	}
	c.warnIfLowSpace()
	c.reg.Scan()
	return b.Manifest.DisplayTitle(), nil
}

func sortVariantNames(names []string) {
	rank := func(n string) int {
		for i, p := range preferredVariantOrder {
			if n == p {
				return i
			}
		}
		return len(preferredVariantOrder)
	}
	sort.SliceStable(names, func(i, j int) bool {
		ri, rj := rank(names[i]), rank(names[j])
		if ri != rj {
			return ri < rj
		}
		return names[i] < names[j]
	})
}
