// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stealthmonkey99/arkade/internal/cartridge/rcart"
	"github.com/stealthmonkey99/arkade/tools/fleet"
)

// Cart is one cartridge directory under inspection, with the manifest,
// parsed sources and config loaded lazily and cached for all checks.
type Cart struct {
	Slug string
	Dir  string // absolute
	Root string // absolute repo root

	Options Options

	manifest    *rcart.Manifest
	manifestRaw []byte
	manifestErr error
	manifestGot bool

	fset       *token.FileSet
	sources    []*SourceFile
	sourcesErr error
	sourcesGot bool

	config    *Config
	configErr error
	configGot bool
}

// SourceFile is one parsed cartridge source file that the TinyGo build compiles.
type SourceFile struct {
	Path string // repo-relative, slash-separated
	File *ast.File
}

// ManifestName is the per-cartridge metadata file.
const ManifestName = "manifest.json"

// Manifest parses manifest.json once, returning the decoded manifest and its
// raw bytes. The error is cached too.
func (c *Cart) Manifest() (*rcart.Manifest, []byte, error) {
	if !c.manifestGot {
		c.manifestGot = true
		raw, err := os.ReadFile(filepath.Join(c.Dir, ManifestName))
		if err != nil {
			c.manifestErr = err
		} else {
			c.manifestRaw = raw
			var m rcart.Manifest
			if err := json.Unmarshal(raw, &m); err != nil {
				c.manifestErr = fmt.Errorf("unparseable %s: %w", ManifestName, err)
			} else {
				c.manifest = &m
			}
		}
	}
	return c.manifest, c.manifestRaw, c.manifestErr
}

// Rel returns abs as a repo-relative, slash-separated path.
func (c *Cart) Rel(abs string) string {
	rel, err := filepath.Rel(c.Root, abs)
	if err != nil {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(rel)
}

// Sources parses every .go file in the cartridge directory that
// TinyGoSource accepts. Subdirectories are host-only and never entered.
func (c *Cart) Sources() ([]*SourceFile, error) {
	if c.sourcesGot {
		return c.sources, c.sourcesErr
	}
	c.sourcesGot = true
	c.fset = token.NewFileSet()

	entries, err := os.ReadDir(c.Dir)
	if err != nil {
		c.sourcesErr = err
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	var errs []error
	for _, name := range names {
		path := filepath.Join(c.Dir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !TinyGoSource(name, raw) {
			continue
		}
		f, err := parser.ParseFile(c.fset, path, raw, parser.ParseComments)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		c.sources = append(c.sources, &SourceFile{Path: c.Rel(path), File: f})
	}
	c.sourcesErr = errors.Join(errs...)
	return c.sources, c.sourcesErr
}

// AssetFiles returns the absolute paths of files under assets/ whose
// extension matches ext (with the dot, case-insensitive), sorted.
func (c *Cart) AssetFiles(ext string) ([]string, error) {
	assets := filepath.Join(c.Dir, "assets")
	info, err := os.Stat(assets)
	if err != nil || !info.IsDir() {
		return nil, nil
	}
	var out []string
	err = filepath.WalkDir(assets, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(p), ext) {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

func findRoot(start string) (string, error) {
	return fleet.EngineRoot(start)
}

// discoverCarts lists every cartridge slug under the fleet's cartridge roots.
func discoverCarts(root string) ([]string, error) {
	_, slugs, err := fleet.Dirs(root)
	return slugs, err
}

// Options are the scan-wide settings from the flags.
type Options struct {
	AudioFloor audioFloorModel // zero is the reference cabinet
}

// NewCart binds a slug to its directory without reading anything yet.
func NewCart(root, slug string, opts Options) *Cart {
	dir := fleet.Dir(root, slug)
	if dir == "" {
		dir = filepath.Join(root, "cartridges", slug)
	}
	return &Cart{Slug: slug, Root: root, Dir: dir, Options: opts}
}
