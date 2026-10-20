// SPDX-License-Identifier: GPL-2.0-or-later

package packfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/stealthmonkey99/arkade/sdk/levels"
)

// A Hook inspects one parsed level at generation time. A returned error fails
// the generation, and the returned entries are metadata the checker derived,
// such as a solver's `par`. A returned entry replaces a same-key entry from the
// pack file.
//
// The level is already decoded, so a Hook uses the same accessors as the
// cartridge. It runs on the host and may allocate and use maps freely.
type Hook func(lv *levels.Level) ([]levels.Entry, error)

// An IndexedHook is a [Hook] that also receives the level's ordinal position,
// counting from 0 across the whole run in the pack-then-level order [Generate]
// walks. It suits checks that depend on where a level sits in the sequence.
type IndexedHook func(i int, lv *levels.Level) ([]levels.Entry, error)

// A PackHook inspects one whole parsed pack at generation time and can only
// refuse it. It suits checks across levels, such as whether every room is
// reachable through the exits. A pack has no metadata to derive into, so a
// check that wants to write something does it per level from a [Hook].
type PackHook func(p *levels.Pack) error

// Config drives [Render], [Generate] and [Check].
type Config struct {
	Dir string // directory holding the pack files
	Out string // path of the generated .go file; Render does not need it
	Pkg string // package clause for the generated file; empty means "main"

	// Hook and IndexedHook are optional and mutually exclusive.
	Hook        Hook        // runs once per level; see [Hook]
	IndexedHook IndexedHook // like Hook, with the level's index; see [IndexedHook]

	// PackHook is optional and independent of the two above. It runs once per
	// pack, after every level of that pack has passed its hook.
	PackHook PackHook
}

// Generate is [Render] followed by writing cfg.Out. An up-to-date cfg.Out is
// left untouched, so its timestamp moves only when its content does. It
// returns the parsed packs in output order.
func Generate(cfg Config) ([]*File, error) {
	if cfg.Out == "" {
		return nil, errors.New("packfile.Generate: Dir and Out are required")
	}
	files, src, err := Render(cfg)
	if err != nil {
		return nil, err
	}
	if err := Sync(false, Output{Path: cfg.Out, Src: src}); err != nil {
		return nil, err
	}
	return files, nil
}

// Render parses every pack file in cfg.Dir in name order, runs cfg.Hook or
// cfg.IndexedHook over every level and cfg.PackHook over every pack, and
// returns the parsed packs with the Go source [Emit] makes of them. It writes
// nothing. cfg.Out is ignored.
//
// Errors from all files and levels are reported together.
func Render(cfg Config) ([]*File, []byte, error) {
	if cfg.Dir == "" {
		return nil, nil, errors.New("packfile.Render: Dir is required")
	}
	if cfg.Hook != nil && cfg.IndexedHook != nil {
		return nil, nil, errors.New("packfile: Config.Hook and Config.IndexedHook are mutually exclusive")
	}
	if cfg.Pkg == "" {
		cfg.Pkg = "main"
	}

	entries, err := os.ReadDir(cfg.Dir)
	if err != nil {
		return nil, nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && HasExt(e.Name()) {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return nil, nil, fmt.Errorf("no *%s or *%s files in %s", ArkadeExt, Ext, cfg.Dir)
	}
	sort.Strings(names)

	var (
		files []*File
		errs  []error
		idx   int // this level's position across the whole run, for IndexedHook
	)
	for _, name := range names {
		f, err := ParseFile(filepath.Join(cfg.Dir, name))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if cfg.Hook != nil || cfg.IndexedHook != nil {
			for i := range f.Pack.Levels {
				lv := &f.Pack.Levels[i]
				var (
					derived []levels.Entry
					hookErr error
				)
				if cfg.Hook != nil {
					derived, hookErr = cfg.Hook(lv)
				} else {
					derived, hookErr = cfg.IndexedHook(idx, lv)
				}
				idx++
				if hookErr != nil {
					errs = append(errs, fmt.Errorf("%s: level %q: %w", name, lv.Name, hookErr))
					continue
				}
				if err := merge(lv, derived); err != nil {
					errs = append(errs, fmt.Errorf("%s: level %q: %w", name, lv.Name, err))
				}
			}
		}
		if cfg.PackHook != nil {
			if err := cfg.PackHook(f.Pack); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", name, err))
			}
		}
		// a repeated name would emit the same variable twice
		for _, prev := range files {
			if prev.Pack.Name == f.Pack.Name {
				errs = append(errs, fmt.Errorf("%s: pack name %q is already declared by %s",
					name, f.Pack.Name, filepath.Base(prev.Path)))
			}
		}
		files = append(files, f)
	}
	if len(errs) > 0 {
		return nil, nil, errors.Join(errs...)
	}

	src, err := Emit(cfg.Pkg, files)
	if err != nil {
		return nil, nil, err
	}
	return files, src, nil
}

func merge(lv *levels.Level, derived []levels.Entry) error {
	for _, d := range derived {
		if !isIdent(d.Key) {
			return fmt.Errorf("derived metadata key %q is not [A-Za-z][A-Za-z0-9_-]*", d.Key)
		}
		replaced := false
		for i := range lv.Meta {
			if lv.Meta[i].Key == d.Key {
				lv.Meta[i], replaced = d, true
				break
			}
		}
		if !replaced {
			lv.Meta = append(lv.Meta, d)
		}
	}
	sortEntries(lv.Meta)
	return nil
}
