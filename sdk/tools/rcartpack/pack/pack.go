// SPDX-License-Identifier: GPL-2.0-or-later

// Package pack builds a .rcart cartridge bundle from a project directory
// and refuses any bundle the cabinet would reject or could not fully
// deliver. The rcartpack command is a thin wrapper over Pack.
//
// The tar is built by rcart.BuildDirTar, never by a system tar: macOS bsdtar
// writes PAX headers and AppleDouble sidecar entries, which the browser
// host's strict reader rejects. The built bytes are then fed back through
// rcart.Load, the cabinet's own mount path, before anything is written.
package pack

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stealthmonkey99/arkade/internal/cartridge/rcart"
)

// Options names the inputs and output of one Pack.
type Options struct {
	// Dir is the project directory. It must hold manifest.json; an assets/
	// tree and the manifest's manual are packed when present.
	Dir string
	// Wasm is the compiled cartridge. Empty means <Dir>/game.wasm, else
	// <Dir>/<manifest slug>.wasm.
	Wasm string
	// Out is the bundle path. Empty means <Dir>/<manifest slug>.rcart. A
	// .rcart path's stem must equal the manifest slug.
	Out string
}

// Result describes a bundle Pack wrote.
type Result struct {
	// Path is where the bundle was written.
	Path string
	// Size is the bundle's length in bytes.
	Size int
	// Slug is the manifest slug.
	Slug string
	// Assets counts the bundle's files other than manifest.json and
	// game.wasm.
	Assets int
	// Cosmetic holds the loader's warnings that do not stop a pack, such as
	// missing title art. The caller decides whether to print them.
	Cosmetic []rcart.Warning
}

// String is the one-line summary rcartpack prints.
func (r Result) String() string {
	return fmt.Sprintf("wrote %s (%d bytes, slug %q, %d asset file(s))", r.Path, r.Size, r.Slug, r.Assets)
}

// Built is a bundle Build assembled and the loader accepted.
type Built struct {
	// Tar is the bundle's bytes, exactly as Pack writes them.
	Tar []byte
	// Bundle is rcart.Load's reading of Tar.
	Bundle *rcart.Bundle
	// Cosmetic holds the loader's warnings that do not stop a pack.
	Cosmetic []rcart.Warning
}

// Build assembles and validates the bundle opts describes without writing
// it; opts.Out is ignored. It returns an error when:
//
//   - the manifest or the wasm cannot be read;
//   - assets/ holds macOS Finder or AppleDouble junk;
//   - rcart.Load would reject the bundle;
//   - the manifest predates schemaVersion (the loader still mounts such
//     bundles, but none may be built);
//   - rcart.Load reports any rcart.Undeliverable warning, such as a broken
//     asset envelope or a listed asset the bundle does not carry.
func Build(opts Options) (Built, error) {
	manifestPath := filepath.Join(opts.Dir, rcart.ManifestName)
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		return Built{}, fmt.Errorf("read %s: %w (every .rcart needs a manifest.json — see docs/CARTRIDGE.md's \"Manifest\")", manifestPath, err)
	}
	var quick struct {
		Slug   string `json:"slug"`
		Manual string `json:"manual"`
	}
	_ = json.Unmarshal(manifestRaw, &quick) // best-effort; rcart.Load below is the real validation

	wasmPath, err := resolveWasm(opts.Dir, opts.Wasm, quick.Slug)
	if err != nil {
		return Built{}, err
	}
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return Built{}, fmt.Errorf("read %s: %w", wasmPath, err)
	}

	if err := refuseJunk(filepath.Join(opts.Dir, "assets")); err != nil {
		return Built{}, err
	}
	tarBytes, err := rcart.BuildDirTar(opts.Dir, manifestRaw, wasmBytes, quick.Manual)
	if err != nil {
		return Built{}, err
	}

	bundle, err := rcart.Load(bytes.NewReader(tarBytes))
	if err != nil {
		return Built{}, fmt.Errorf("built bundle would be rejected by the cabinet's .rcart loader: %w", err)
	}
	if bundle.Manifest.SchemaVersion == 0 {
		return Built{}, fmt.Errorf("%s is a pre-schema manifest — new bundles must declare \"schemaVersion\": %d (see sdk/README.md's \"Manifest schema\")", rcart.ManifestName, rcart.CurrentSchemaVersion)
	}
	cosmetic, undeliverable := splitWarnings(bundle.Warnings)
	if len(undeliverable) > 0 {
		return Built{}, undeliverableError(undeliverable)
	}
	return Built{Tar: tarBytes, Bundle: bundle, Cosmetic: cosmetic}, nil
}

// Pack is Build, then writes the bundle to opts.Out. It also fails when Out
// is a .rcart path whose stem is not the slug. Nothing is written on error.
func Pack(opts Options) (Result, error) {
	built, err := Build(opts)
	if err != nil {
		return Result{}, err
	}
	slug := built.Bundle.Manifest.Slug
	out := opts.Out
	if out == "" {
		if slug == "" {
			return Result{}, fmt.Errorf("%s has no \"slug\" and no output path was given", rcart.ManifestName)
		}
		out = filepath.Join(opts.Dir, slug+".rcart")
	}
	if strings.EqualFold(filepath.Ext(out), ".rcart") {
		stem := strings.TrimSuffix(filepath.Base(out), filepath.Ext(out))
		if slug != stem {
			return Result{}, fmt.Errorf("manifest slug %q does not match output filename %q.rcart (docs/CARTRIDGE.md: slug must match the .rcart filename stem)", slug, stem)
		}
	}
	if err := os.WriteFile(out, built.Tar, 0o644); err != nil {
		return Result{}, fmt.Errorf("write %s: %w", out, err)
	}
	return Result{
		Path:     out,
		Size:     len(built.Tar),
		Slug:     slug,
		Assets:   len(built.Bundle.Assets),
		Cosmetic: built.Cosmetic,
	}, nil
}

func resolveWasm(dir, explicit, slug string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	candidates := []string{filepath.Join(dir, rcart.WasmName)}
	if slug != "" {
		candidates = append(candidates, filepath.Join(dir, slug+".wasm"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("no compiled cartridge .wasm found (tried %s; pass the path explicitly, or did you run the project's `make build` — tinygo + wasmfix — first?)", strings.Join(candidates, ", "))
}

func splitWarnings(ws []rcart.Warning) (cosmetic, undeliverable []rcart.Warning) {
	for _, w := range ws {
		if w.Severity == rcart.Undeliverable {
			undeliverable = append(undeliverable, w)
		} else {
			cosmetic = append(cosmetic, w)
		}
	}
	return cosmetic, undeliverable
}

func heading(c rcart.Category) string {
	switch c {
	case rcart.CategorySprite:
		return "unreadable sprite sheet(s); fix or regenerate them (docs/ASSETS.md)"
	case rcart.CategorySound:
		return "unreadable sound file(s); fix or regenerate them (docs/ASSETS.md)"
	case rcart.CategoryGlyph:
		return "glyph problem(s) in assets.glyphs (docs/ASSETS.md, sdk/docs/reference-glyph.md)"
	case rcart.CategoryHID:
		return "hid profile problem(s) in assets.hid (docs/ASSETS.md, sdk/docs/reference-hid.md)"
	case rcart.CategoryMic:
		return "mic model problem(s) in assets.mic (docs/ASSETS.md, \"mic\")"
	case rcart.CategoryLights:
		return "lamp sequence problem(s) in assets.lights (docs/ASSETS.md, sdk/docs/reference-lights.md)"
	default:
		return "asset problem(s) (docs/ASSETS.md)"
	}
}

func undeliverableError(ws []rcart.Warning) error {
	byCategory := map[rcart.Category][]string{}
	var order []rcart.Category
	for _, w := range ws {
		if _, seen := byCategory[w.Category]; !seen {
			order = append(order, w.Category)
		}
		byCategory[w.Category] = append(byCategory[w.Category], w.Message)
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	var b strings.Builder
	fmt.Fprintf(&b, "the bundle cannot deliver %d asset(s) it names or carries; the cabinet would mount it degraded:", len(ws))
	for _, c := range order {
		group := byCategory[c]
		fmt.Fprintf(&b, "\n%d %s:\n  %s", len(group), heading(c), strings.Join(group, "\n  "))
	}
	return errors.New(b.String())
}

// refuseJunk fails on the first .DS_Store or ._ AppleDouble entry under
// dir, which BuildDirTar would otherwise pack. A missing dir is a no-op.
func refuseJunk(dir string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if name == ".DS_Store" || strings.HasPrefix(name, "._") {
			return fmt.Errorf("%s looks like macOS Finder/AppleDouble junk, not a cartridge asset — remove it (COPYFILE_DISABLE=1 in the environment avoids creating it in the first place)", p)
		}
		return nil
	})
}
