// SPDX-License-Identifier: GPL-2.0-or-later

// Package storage persists app state as JSON files under a data root as
// data/<owner>/<label>.json, written atomically so a power cut cannot corrupt
// a file.
package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const defaultLabel = "settings"

// Store reads and writes JSON blobs keyed by owner + label.
type Store struct {
	root string
}

// New creates a store rooted at dir (e.g. "./data").
func New(dir string) *Store { return &Store{root: dir} }

func (s *Store) path(owner, label string) string {
	if label == "" {
		label = defaultLabel
	}
	return filepath.Join(s.root, filepath.FromSlash(owner), label+".json")
}

// Read unmarshals the stored JSON into out. A missing file leaves out
// untouched and returns nil, so callers can pre-fill defaults.
func (s *Store) Read(owner, label string, out any) error {
	raw, err := os.ReadFile(s.path(owner, label))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("storage read %s/%s: %w", owner, label, err)
	}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("storage parse %s/%s: %w", owner, label, err)
	}
	return nil
}

// Write marshals in and atomically replaces the stored file.
func (s *Store) Write(owner, label string, in any) error {
	path := s.path(owner, label)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("storage mkdir: %w", err)
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("storage marshal %s/%s: %w", owner, label, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("storage write %s/%s: %w", owner, label, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("storage rename %s/%s: %w", owner, label, err)
	}
	return nil
}

// Delete removes a stored file (missing files are fine).
func (s *Store) Delete(owner, label string) error {
	err := os.Remove(s.path(owner, label))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
