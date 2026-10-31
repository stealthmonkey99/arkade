// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
)

// watcher polls a cartridge directory with stat-only walks; the tools stay
// stdlib-only, so no fsnotify.
type watcher struct {
	dir          string
	slug         string // excludes <slug>.wasm and <slug>.rcart, or every build would retrigger itself
	pollInterval time.Duration
	quietPeriod  time.Duration // rapid saves coalesce into one rebuild
}

func newWatcher(dir, slug string) *watcher {
	return &watcher{
		dir:          dir,
		slug:         slug,
		pollInterval: 300 * time.Millisecond,
		quietPeriod:  400 * time.Millisecond,
	}
}

// snapshot is a cheap signature of a tree: file count, total size, latest mtime.
type snapshot struct {
	count   int
	size    int64
	maxMod  time.Time
	readErr bool // a transient stat failure never looks like unchanged
}

func (a snapshot) equal(b snapshot) bool {
	return a.count == b.count && a.size == b.size && a.maxMod.Equal(b.maxMod) && a.readErr == b.readErr
}

// skipName reports the build outputs and dotfiles, which are never source.
func (w *watcher) skipName(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch name {
	case w.slug + ".wasm", w.slug + ".rcart":
		return true
	}
	return false
}

func (w *watcher) take() snapshot {
	var s snapshot
	err := filepath.WalkDir(w.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			s.readErr = true
			return nil
		}
		if path != w.dir && w.skipName(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			s.readErr = true
			return nil
		}
		s.count++
		s.size += info.Size()
		if info.ModTime().After(s.maxMod) {
			s.maxMod = info.ModTime()
		}
		return nil
	})
	if err != nil {
		s.readErr = true
	}
	return s
}

// run polls until ctx is done, calling trigger once the tree has been stable for quietPeriod.
func (w *watcher) run(ctx context.Context, trigger func()) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	last := w.take()
	lastBuilt := last
	var changedAt time.Time
	pending := false

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cur := w.take()
			if !cur.equal(last) {
				last = cur
				changedAt = time.Now()
				pending = true
			}
			if pending && cur.equal(lastBuilt) {
				// Reverted to the last built state.
				pending = false
			}
			if pending && time.Since(changedAt) >= w.quietPeriod {
				pending = false
				lastBuilt = cur
				trigger()
			}
		}
	}
}
