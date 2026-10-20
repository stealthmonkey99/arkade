// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/cartridge/rcart"
	"github.com/stealthmonkey99/arkade/sdk/tools/manifestgen/gen"
	"github.com/stealthmonkey99/arkade/sdk/tools/rcartpack/pack"
)

const (
	ruleBundledAsset = "docs/ASSETS.md, \"Packaging\": every path under the manifest's assets.* must exist in the cartridge and pass assetfile validation — rcartpack refuses to build the bundle otherwise"
	ruleSchema       = "internal/cartridge/rcart.Manifest.Validate — the schema-1 contract every packed bundle must satisfy"
	ruleManual       = "manual footer rule: the manual's footer quotes the manifest's CURRENT version, so a gameplay change that bumps the version forces a manual pass in the same commit"
	ruleGenerated    = "manifest.json is the source of truth: manifest_gen.go, the compiled-in runtime manifest, is generated from it by `go run ./sdk/tools/manifestgen <dir>` (every cartridge build runs it) and must be current"
)

// checkManifest validates the manifest with rcart, then checks the files it
// references, the manual, and the generated runtime manifest.
func checkManifest(c *Cart, r *Result) {
	m, _, err := c.Manifest()
	if err != nil {
		r.fail(ruleSchema, c.At(filepath.Join(c.Dir, ManifestName)), "%v", err)
		return
	}
	where := c.At(filepath.Join(c.Dir, ManifestName))

	if err := m.Validate(); err != nil {
		r.fail(ruleSchema, where, "%v", err)
	}
	if m.SchemaVersion != rcart.CurrentSchemaVersion {
		r.fail(ruleSchema, where, `schemaVersion is %d; new bundles must declare %d (sdk/tools/rcartpack refuses to pack a legacy manifest)`,
			m.SchemaVersion, rcart.CurrentSchemaVersion)
	}
	if m.Slug != c.Slug {
		r.fail(ruleSchema, where, "slug %q does not match the directory name %q — the cabinet keys scores, MRU and settings by slug, so the two must agree", m.Slug, c.Slug)
	}

	for _, ref := range []struct{ field, path string }{
		{"icon", m.Icon}, {"logo", m.Logo}, {"manual", m.Manual},
	} {
		if ref.path == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(c.Dir, filepath.FromSlash(ref.path))); err != nil {
			r.fail(ruleSchema, where, "%s points at %q, which is not in the cartridge directory", ref.field, ref.path)
		}
	}

	checkBundledAssets(c, r, m, where)
	checkManual(c, r, m)
	checkGeneratedManifest(c, r)

	wasm := filepath.Join(c.Dir, c.Slug+".wasm")
	if _, err := os.Stat(wasm); err != nil {
		r.fail(ruleSchema, c.At(wasm), "no built %s.wasm — it is built from the cartridge source and committed", c.Slug)
	}

	if fails, warns := r.counts(); fails == 0 && warns == 0 {
		r.summarize("schema-%d valid, %s, manual current", m.SchemaVersion, m.Version)
	} else {
		r.summarize("%d violation(s), %d to look at", fails, warns)
	}
}

// checkBundledAssets applies rcartpack's pack-time checks to the files listed
// under assets.hid, assets.mic and assets.lights: present, correctly named,
// valid envelope. assets.glyphs is read by collectBundledGlyphs instead.
func checkBundledAssets(c *Cart, r *Result, m *rcart.Manifest, where Loc) {
	if paths := m.HIDAssets(); len(paths) > 0 && !m.HasCapability(rcart.CapabilityHID) {
		// Without the capability the host mounts the profiles but ignores them.
		r.fail(ruleBundledAsset, where, "assets.hid lists %d profile(s) but the manifest does not declare the %q capability", len(paths), rcart.CapabilityHID)
	}
	if paths := m.MicAssets(); len(paths) > 0 && !m.HasCapability(rcart.CapabilityMic) {
		r.fail(ruleBundledAsset, where, "assets.mic lists %d sound model(s) but the manifest does not declare the %q capability", len(paths), rcart.CapabilityMic)
	}
	for _, listed := range []struct {
		field string
		kind  assetfile.Kind
		paths []string
	}{
		{"assets.hid", assetfile.KindHID, m.HIDAssets()},
		{"assets.mic", assetfile.KindMic, m.MicAssets()},
		{"assets.lights", assetfile.KindLights, m.LightsAssets()},
	} {
		for _, rel := range listed.paths {
			path := filepath.Join(c.Dir, filepath.FromSlash(rel))
			data, err := os.ReadFile(path)
			if err != nil {
				r.fail(ruleBundledAsset, where, "manifest lists %q under %s, but the file is not there", rel, listed.field)
				continue
			}
			if kind, ok := assetfile.KindFromFilename(rel); !ok || kind != listed.kind {
				r.fail(ruleBundledAsset, c.At(path), "%s lists %q, which is not a <name>.arkade%s.json file", listed.field, rel, listed.kind)
				continue
			}
			if _, err := assetfile.ReadBytes(rel, data); err != nil {
				r.fail(ruleBundledAsset, c.At(path), "%v", err)
			}
		}
	}
}

// checkManual enforces the manual's mechanical contract, which fleetlint's
// TestManualsCurrent also owns.
func checkManual(c *Cart, r *Result, m *rcart.Manifest) {
	path := filepath.Join(c.Dir, "MANUAL.md")
	where := c.At(path)
	raw, err := os.ReadFile(path)
	if err != nil {
		r.fail(ruleManual, where, "no MANUAL.md — every cartridge ships a player manual")
		return
	}
	manual := string(raw)

	if title := m.DisplayTitle(); title != "" && !strings.Contains(manual, "# "+title) {
		r.fail(ruleManual, where, "no '# %s' heading matching the manifest title", title)
	}
	if want := "*Manual for version " + m.Version; !strings.Contains(manual, want) {
		r.fail(ruleManual, where, "footer does not quote manifest version %s — if a change bumped the version, reread the manual against that change and only THEN update the footer", m.Version)
	}
	if !strings.Contains(manual, `src="manual-icon.png"`) {
		r.fail(ruleManual, where, "header does not use manual-icon.png")
	}
	if _, err := os.Stat(filepath.Join(c.Dir, "manual-icon.png")); err != nil {
		r.fail(ruleManual, where, "manual-icon.png missing — regenerate with `ARKADE_CARTRIDGE_ROOTS=<library>/cartridges go run ./tools/manualicon` from the engine checkout")
	}

	index := c.manualsIndex()
	idx, err := os.ReadFile(index)
	if err != nil {
		r.info(c.At(index), "no library table beside this cartridge's root, so its link there is not checked")
		return
	}
	if !strings.Contains(string(idx), "cartridges/"+c.Slug+"/MANUAL.md") {
		r.fail(ruleManual, c.At(index), "%s is not linked from the library table", c.Slug)
	}
}

// manualsIndex is the library table of the repository holding this
// cartridge: <repo>/cartridges/<slug> -> <repo>/docs/MANUALS.md, wherever
// that repository sits relative to the scan root.
func (c *Cart) manualsIndex() string {
	abs, err := filepath.Abs(c.Dir)
	if err != nil {
		abs = c.Dir
	}
	return filepath.Join(filepath.Dir(filepath.Dir(abs)), "docs", "MANUALS.md")
}

// checkGeneratedManifest requires manifest_gen.go to be what manifest.json
// generates. A cartridge with no TinyGo sources, such as a C cartridge, has
// no generated file; its golden tests pin cart_manifest instead.
func checkGeneratedManifest(c *Cart, r *Result) {
	srcs, err := c.Sources()
	if err != nil || len(srcs) == 0 {
		return
	}
	if err := gen.Check(c.Dir); err != nil {
		r.fail(ruleGenerated, c.At(filepath.Join(c.Dir, gen.FileName)), "%v — regenerate with `go run ./sdk/tools/manifestgen %s` and rebuild the wasm", err, c.Rel(c.Dir))
	}
}

// checkBundle builds the bundle in memory exactly as rcartpack would, which
// loads it back with rcart.Load, the function the cabinet uses at mount time.
func checkBundle(c *Cart, r *Result) {
	if _, _, err := c.Manifest(); err != nil {
		r.skip("manifest unreadable — see the manifest check")
		return
	}
	wasmPath := filepath.Join(c.Dir, c.Slug+".wasm")
	if _, err := os.Stat(wasmPath); err != nil {
		r.skip("no built %s.wasm to pack", c.Slug)
		return
	}
	built, err := pack.Build(pack.Options{Dir: c.Dir, Wasm: wasmPath})
	if err != nil {
		r.fail(ruleBundle, c.At(c.Dir), "rcartpack would refuse this cartridge: %v", err)
		return
	}
	bundle, tarBytes, wasm := built.Bundle, built.Tar, built.Bundle.Wasm
	for _, w := range built.Cosmetic {
		at := c.At(c.Dir)
		if w.Path != "" {
			at = c.At(filepath.Join(c.Dir, filepath.FromSlash(w.Path)))
		}
		r.warn(ruleBundle, at, "rcart.Load warning: %s", w)
	}
	r.summarize("packs to a loadable .rcart: %s, %d asset(s), %s wasm",
		bundle.Manifest.Slug, len(bundle.Assets), humanBytes(len(wasm)))
	r.info(c.At(c.Dir), "bundle tar is %s across %d entries", humanBytes(len(tarBytes)), len(bundle.Assets)+2)
}

const ruleBundle = "sdk/tools/rcartpack/pack.Build — what rcartpack refuses, including rcart.Load's mount-time contract (size caps, entry count, clean paths, manifest validity, deliverable assets)"

func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
