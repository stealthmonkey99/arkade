// SPDX-License-Identifier: GPL-2.0-or-later

package flight

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DefaultKeep is how many dumps a Store retains. It is small because the
// cabinet shares its card with the game library.
const DefaultKeep = 8

// Store is the bounded on-disk home for flight dumps: one flat directory,
// rotated by file count after every write. Count suffices because the
// engine's ring already bounds each recording's size.
type Store struct {
	dir  string
	keep int
}

// NewStore builds a Store writing into dir (created on first write). keep
// <= 0 uses DefaultKeep.
func NewStore(dir string, keep int) *Store {
	if keep <= 0 {
		keep = DefaultKeep
	}
	return &Store{dir: dir, keep: keep}
}

// Dir reports the directory this Store writes into.
func (s *Store) Dir() string { return s.dir }

// Write encodes rec into a new file named for its instant, slug and reason,
// such as "20260905T141312Z-cart_snake-crash.arkflt", then rotates the
// directory down to Keep files. It returns the path written.
//
// It writes a temp file and renames it so a crash never leaves a partial
// file that looks like a recording.
func (s *Store) Write(rec *Recording, now time.Time) (string, error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return "", fmt.Errorf("flight: creating %s: %w", s.dir, err)
	}
	final := s.uniquePath(Filename(now, rec.Header.Slug, rec.Header.Reason))

	tmp, err := os.CreateTemp(s.dir, ".flight-*.tmp")
	if err != nil {
		return "", fmt.Errorf("flight: creating temp dump: %w", err)
	}
	tmpName := tmp.Name()
	if err := Encode(tmp, rec); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("flight: closing temp dump: %w", err)
	}
	if err := os.Rename(tmpName, final); err != nil {
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("flight: naming dump: %w", err)
	}
	if err := s.rotate(); err != nil {
		// The dump is on disk; a failed rotation must not fail it.
		return final, err
	}
	return final, nil
}

// uniquePath suffixes every dump after the first in a given second with its
// sequence number, so none overwrites another and List keeps the order they
// were taken.
func (s *Store) uniquePath(name string) string {
	stamp, _ := dumpOrder(name)
	seq := 0
	if entries, err := os.ReadDir(s.dir); err == nil {
		for _, e := range entries {
			if st, n := dumpOrder(e.Name()); st == stamp && n > seq {
				seq = n
			}
		}
	}
	if seq == 0 {
		return filepath.Join(s.dir, name)
	}
	return filepath.Join(s.dir, fmt.Sprintf("%s%s%d%s", strings.TrimSuffix(name, Extension), seqSep, seq+1, Extension))
}

// seqSep introduces a same-second sequence number; sanitize never emits it.
const seqSep = "."

// stampLen is the length of Filename's leading timestamp.
const stampLen = len("20060102T150405Z")

// dumpOrder splits a dump's filename into its timestamp and its sequence
// within that second (1 for an unsuffixed name). A name that is not a dump
// has an empty stamp.
func dumpOrder(name string) (stamp string, seq int) {
	if !strings.HasSuffix(name, Extension) || len(name) < stampLen {
		return "", 0
	}
	stamp, seq = name[:stampLen], 1
	rest := strings.TrimSuffix(name[stampLen:], Extension)
	if i := strings.LastIndex(rest, seqSep); i >= 0 {
		if n, err := strconv.Atoi(rest[i+len(seqSep):]); err == nil {
			seq = n
		}
	}
	return stamp, seq
}

// Filename builds a dump's name: sortable timestamp first, then the slug
// with its slash flattened, then the reason.
func Filename(now time.Time, slug, reason string) string {
	if slug == "" {
		slug = "unknown"
	}
	if reason == "" {
		reason = "manual"
	}
	return fmt.Sprintf("%s-%s-%s%s",
		now.UTC().Format("20060102T150405Z"), sanitize(slug), sanitize(reason), Extension)
}

// sanitize flattens anything that is not a safe filename character, such as
// the slash in a slug.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

// List returns the dumps in dir, oldest first: by timestamp, then by
// sequence within the second. A missing directory is an empty list, not an
// error: no dumps have ever been taken.
func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), Extension) {
			continue
		}
		out = append(out, filepath.Join(s.dir, e.Name()))
	}
	sort.Slice(out, func(i, j int) bool {
		si, ni := dumpOrder(filepath.Base(out[i]))
		sj, nj := dumpOrder(filepath.Base(out[j]))
		if si != sj {
			return si < sj
		}
		if ni != nj {
			return ni < nj
		}
		return out[i] < out[j]
	})
	return out, nil
}

// rotate deletes the oldest dumps until at most Keep remain.
func (s *Store) rotate() error {
	files, err := s.List()
	if err != nil {
		return err
	}
	if len(files) <= s.keep {
		return nil
	}
	for _, p := range files[:len(files)-s.keep] {
		if err := os.Remove(p); err != nil {
			return fmt.Errorf("flight: rotating out %s: %w", p, err)
		}
	}
	return nil
}
