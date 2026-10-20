// SPDX-License-Identifier: GPL-2.0-or-later

// Package rcart parses and validates .rcart cartridge bundles, a tar,
// optionally gzipped, holding manifest.json, game.wasm and an assets/ tree,
// and the bare .wasm files accepted as degenerate cartridges. A bundle is
// untrusted input from inserted media.
package rcart

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
)

// Well-known bundle entry names.
const (
	ManifestName = "manifest.json"
	WasmName     = "game.wasm"
)

// Hard caps that bound memory and CPU whatever a hostile tar claims.
const (
	// MaxWasmBytes caps game.wasm alone.
	MaxWasmBytes = 8 << 20
	// MaxManifestBytes caps manifest.json.
	MaxManifestBytes = 1 << 20
	// MaxTotalBytes caps the bundle's uncompressed content. Sized to fit the
	// largest shipped cartridge, about 39 MiB of WAV soundtrack.
	MaxTotalBytes = 64 << 20
	// MaxEntries caps the tar entry count against a many-tiny-files bomb.
	MaxEntries = 512
)

// Bundle is a parsed and validated cartridge, from a .rcart tar or a bare
// .wasm file.
type Bundle struct {
	Manifest Manifest
	Wasm     []byte
	// Assets holds every other regular file by bundle-relative path, e.g.
	// "assets/sprites/icon.png". Nil for a bare .wasm.
	Assets map[string][]byte
	// Icon and Logo are decoded from the manifest's PNG paths, or nil when
	// unset or broken. Logo is the larger title-card art.
	Icon display.Sprite
	Logo display.Sprite
	// Warnings are the non-fatal problems Load found, in a stable order.
	Warnings []Warning
}

// Load parses and validates a .rcart bundle from r, gunzipping it when it
// starts with the gzip magic.
//
// Each entry's declared size is checked against its cap before anything is
// read. That is a complete bomb defense because tar.Reader never yields more
// than the declared size. Absolute and escaping paths, duplicate names, and
// any entry other than a regular file or directory are rejected.
func Load(r io.Reader) (*Bundle, error) {
	br := bufio.NewReader(r)
	var tr *tar.Reader
	if magic, err := br.Peek(2); err == nil && len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		gz, err := gzip.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("rcart: gzip: %w", err)
		}
		defer gz.Close()
		tr = tar.NewReader(gz)
	} else {
		tr = tar.NewReader(br)
	}

	entries := map[string][]byte{}
	var total int64
	count := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("rcart: reading tar: %w", err)
		}

		count++
		if count > MaxEntries {
			return nil, fmt.Errorf("rcart: bundle has more than %d entries", MaxEntries)
		}

		name, ok := cleanEntryName(hdr.Name)
		if !ok {
			return nil, fmt.Errorf("rcart: illegal entry path %q (absolute paths and .. are not allowed)", hdr.Name)
		}
		if name == "." {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg, tar.TypeRegA: //nolint:staticcheck // older tar writers still emit TypeRegA
		default:
			return nil, fmt.Errorf("rcart: entry %q has an unsupported type (only regular files and directories are allowed)", hdr.Name)
		}

		if _, dup := entries[name]; dup {
			return nil, fmt.Errorf("rcart: duplicate entry %q", name)
		}
		if hdr.Size < 0 {
			return nil, fmt.Errorf("rcart: entry %q declares a negative size", name)
		}

		perFileCap := int64(MaxTotalBytes)
		switch name {
		case ManifestName:
			perFileCap = MaxManifestBytes
		case WasmName:
			perFileCap = MaxWasmBytes
		}
		if hdr.Size > perFileCap {
			return nil, fmt.Errorf("rcart: entry %q is %d bytes, over the %d byte cap", name, hdr.Size, perFileCap)
		}
		if hdr.Size > MaxTotalBytes-total {
			return nil, fmt.Errorf("rcart: bundle exceeds the %d byte total uncompressed cap", MaxTotalBytes)
		}

		buf := make([]byte, hdr.Size)
		if _, err := io.ReadFull(tr, buf); err != nil {
			return nil, fmt.Errorf("rcart: reading %q: %w", name, err)
		}
		entries[name] = buf
		total += hdr.Size
	}

	manifestRaw, ok := entries[ManifestName]
	if !ok {
		return nil, fmt.Errorf("rcart: bundle is missing %s", ManifestName)
	}
	delete(entries, ManifestName)

	var m Manifest
	if err := json.Unmarshal(manifestRaw, &m); err != nil {
		return nil, fmt.Errorf("rcart: parsing %s: %w", ManifestName, err)
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("rcart: %s: %w", ManifestName, err)
	}

	wasmBytes, ok := entries[WasmName]
	if !ok {
		return nil, fmt.Errorf("rcart: bundle is missing %s", WasmName)
	}
	delete(entries, WasmName)
	if len(wasmBytes) == 0 {
		return nil, fmt.Errorf("rcart: %s is empty", WasmName)
	}

	b := &Bundle{Manifest: m, Wasm: wasmBytes, Assets: entries}
	for _, ref := range [...]struct {
		field string
		path  string
		out   *display.Sprite
	}{{"icon", m.Icon, &b.Icon}, {"logo", m.Logo, &b.Logo}} {
		if ref.path == "" {
			continue
		}
		png, ok := entries[ref.path]
		if !ok {
			b.Warnings = append(b.Warnings, warnf(CategoryArt, Cosmetic, ref.path, "%s %q not found in bundle", ref.field, ref.path))
			continue
		}
		sprite, err := element.DecodePNG(png)
		if err != nil {
			b.Warnings = append(b.Warnings, warnf(CategoryArt, Cosmetic, ref.path, "%s %q: %v", ref.field, ref.path, err))
			continue
		}
		*ref.out = sprite
	}
	if m.Manual != "" {
		if _, ok := entries[m.Manual]; !ok {
			b.Warnings = append(b.Warnings, warnf(CategoryManual, Cosmetic, m.Manual, "manual %q not found in bundle", m.Manual))
		}
	}
	b.Warnings = append(b.Warnings, sheetWarnings(entries)...)
	b.Warnings = append(b.Warnings, soundWarnings(entries)...)
	for _, k := range listedAssetKinds {
		b.Warnings = append(b.Warnings, listedAssetWarnings(k, m, entries)...)
	}
	return b, nil
}

// listedAssetWarnings checks one assets.<field> list: capability, mount cap,
// presence, file name and envelope. A missing capability is reported even
// when every file is fine, since the host would silently ignore the list.
func listedAssetWarnings(k listedAssetKind, m Manifest, entries map[string][]byte) []Warning {
	paths := k.paths(&m)
	if len(paths) == 0 {
		return nil
	}
	var out []Warning
	warn := func(path, format string, args ...any) {
		out = append(out, warnf(k.category, Undeliverable, path, format, args...))
	}
	if k.capability != "" && !m.HasCapability(k.capability) {
		warn("", "assets.%s lists %d %s(s) but the manifest does not declare the %q capability", k.field, len(paths), k.noun, k.capability)
	}
	if k.cap > 0 && len(paths) > k.cap {
		warn("", "assets.%s lists %d %s(s); a host mounts at most %d", k.field, len(paths), k.noun, k.cap)
	}
	for _, p := range paths {
		data, ok := entries[p]
		if !ok {
			warn(p, "%s %q not found in bundle", k.label, p)
			continue
		}
		if kind, ok := assetfile.KindFromFilename(p); !ok || kind != k.kind {
			warn(p, "%s %q is not a <name>.arkade%s.json file", k.label, p, k.kind)
			continue
		}
		if _, err := assetfile.ReadBytes(p, data); err != nil {
			warn(p, "%s %q: %v", k.label, p, err)
		}
	}
	return out
}

// sheetWarnings reports each broken .arkadesprite.png, in sorted order. At
// mount the game still starts and host_load_sheet refuses just that sheet.
func sheetWarnings(entries map[string][]byte) []Warning {
	paths := make([]string, 0, len(entries))
	for p := range entries {
		if kind, ok := assetfile.KindFromFilename(p); ok && kind == assetfile.KindSprite {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	var out []Warning
	for _, p := range paths {
		// DecodeSheet checks cels against decoded bounds, as host_load_sheet does.
		if _, err := assetfile.DecodeSheet(p, entries[p]); err != nil {
			out = append(out, warnf(CategorySprite, Undeliverable, p, "sprite sheet %q: %v", p, err))
		}
	}
	return out
}

// soundWarnings reports each broken .arkadesound.wav, in sorted order,
// including a WAV header that disagrees with its render block. A plain .wav
// carries no envelope and is not checked.
func soundWarnings(entries map[string][]byte) []Warning {
	paths := make([]string, 0, len(entries))
	for p := range entries {
		if kind, ok := assetfile.KindFromFilename(p); ok && kind == assetfile.KindSound {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	var out []Warning
	for _, p := range paths {
		if _, err := assetfile.DecodeSound(p, entries[p]); err != nil {
			out = append(out, warnf(CategorySound, Undeliverable, p, "sound %q: %v", p, err))
		}
	}
	return out
}

// cleanEntryName normalizes a tar entry name and reports whether it's safe:
// not absolute, and never escapes the bundle root via a ".." segment.
func cleanEntryName(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	if path.IsAbs(raw) {
		return "", false
	}
	clean := path.Clean(raw)
	if clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", false
	}
	return clean, true
}

// LoadBareWasm builds a manifest-less Bundle from raw wasm bytes, deriving
// the slug and display name from slug, typically a filename stem.
func LoadBareWasm(slug string, wasmBytes []byte) (*Bundle, error) {
	if slug == "" {
		return nil, fmt.Errorf("rcart: empty slug")
	}
	if len(wasmBytes) == 0 {
		return nil, fmt.Errorf("rcart: empty wasm file")
	}
	if len(wasmBytes) > MaxWasmBytes {
		return nil, fmt.Errorf("rcart: wasm file is %d bytes, over the %d byte cap", len(wasmBytes), MaxWasmBytes)
	}
	return &Bundle{
		Manifest: Manifest{Slug: slug, Name: titleCase(slug)},
		Wasm:     wasmBytes,
	}, nil
}

func titleCase(slug string) string {
	words := strings.FieldsFunc(slug, func(r rune) bool { return r == '-' || r == '_' })
	if len(words) == 0 {
		return slug
	}
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// LoadFile loads a .rcart, whose manifest slug must match the file stem, or a
// .wasm, which loads as a full bundle when a manifest.json sits beside it and
// as a bare cartridge otherwise. Any other extension is an error.
func LoadFile(p string) (*Bundle, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(p))
	stem := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))

	switch ext {
	case ".rcart":
		b, err := Load(f)
		if err != nil {
			return nil, err
		}
		if b.Manifest.Slug != stem {
			return nil, fmt.Errorf("rcart: manifest slug %q does not match bundle filename %q.rcart (docs/CARTRIDGE.md: slug must match the .rcart filename stem)", b.Manifest.Slug, stem)
		}
		return b, nil
	case ".wasm":
		// +1 detects an oversized file without trusting a racy Stat.
		data, err := io.ReadAll(io.LimitReader(f, MaxWasmBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > MaxWasmBytes {
			return nil, fmt.Errorf("rcart: %s is over the %d byte cap", filepath.Base(p), MaxWasmBytes)
		}
		if _, statErr := os.Stat(filepath.Join(filepath.Dir(p), ManifestName)); statErr == nil {
			return loadSidecar(filepath.Dir(p), stem, data)
		}
		return LoadBareWasm(stem, data)
	default:
		return nil, fmt.Errorf("rcart: unsupported cartridge file extension %q", ext)
	}
}

// loadSidecar packs a cartridge project directory exactly as rcartpack would
// and loads it, so a source tree mounts like packed bundles. The registry
// fingerprints only the .wasm, so a manifest-only edit needs a restart.
func loadSidecar(dir, stem string, wasmBytes []byte) (*Bundle, error) {
	manifestRaw, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return nil, fmt.Errorf("rcart: sidecar %s: %w", ManifestName, err)
	}
	var quick struct {
		Manual string `json:"manual"`
	}
	_ = json.Unmarshal(manifestRaw, &quick) // Load below is the real validation

	tarBytes, err := BuildDirTar(dir, manifestRaw, wasmBytes, quick.Manual)
	if err != nil {
		return nil, err
	}
	b, err := Load(bytes.NewReader(tarBytes))
	if err != nil {
		return nil, err
	}
	if b.Manifest.Slug != stem {
		return nil, fmt.Errorf("rcart: sidecar manifest slug %q does not match %q.wasm (slug must match the file stem, like a bundle's)", b.Manifest.Slug, stem)
	}
	return b, nil
}

// BuildDirTar stages a project directory as a bundle tar: manifest.json,
// game.wasm, every file under dir/assets/ but the level packs, and the
// referenced manual. Shared
// by rcartpack and the mount path so they cannot drift. It enforces no caps;
// Load on the result does.
func BuildDirTar(dir string, manifestRaw, wasmBytes []byte, manual string) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	writeEntry := func(name string, data []byte) error {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
		if _, err := tw.Write(data); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
		return nil
	}
	if err := writeEntry(ManifestName, manifestRaw); err != nil {
		return nil, err
	}
	if err := writeEntry(WasmName, wasmBytes); err != nil {
		return nil, err
	}

	// A manual under assets/ is walked below; a missing one is Load's warning.
	if manual != "" && !strings.HasPrefix(manual, "assets/") {
		if data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(manual))); err == nil {
			if err := writeEntry(manual, data); err != nil {
				return nil, err
			}
		}
	}

	assetsDir := filepath.Join(dir, "assets")
	if info, statErr := os.Stat(assetsDir); statErr == nil && info.IsDir() {
		walkErr := filepath.WalkDir(assetsDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || isLevelPack(d.Name()) {
				return nil
			}
			rel, err := filepath.Rel(dir, p)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return writeEntry(filepath.ToSlash(rel), data)
		})
		if walkErr != nil {
			return nil, fmt.Errorf("walking %s: %w", assetsDir, walkErr)
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("finalizing tar: %w", err)
	}
	return buf.Bytes(), nil
}

// isLevelPack matches sdk/levels/packfile.HasExt. A pack is compiled into the
// wasm as levels_gen.go, so nothing reads one from a bundle.
func isLevelPack(name string) bool {
	return strings.HasSuffix(name, ".arkadelevels") || strings.HasSuffix(name, ".levels")
}
