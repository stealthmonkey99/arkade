// SPDX-License-Identifier: GPL-2.0-or-later

package rcart

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func encode8x8PNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			// A checkerboard, so a decoder returning blank cannot pass.
			if (x+y)%2 == 0 {
				img.SetNRGBA(x, y, color.NRGBA{R: 15, G: 188, B: 122, A: 255})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode 8x8 PNG: %v", err)
	}
	return buf.Bytes()
}

type tarEntry struct {
	name     string
	data     []byte
	typeflag byte
	// size overrides len(data) in the header.
	size    int64
	sizeSet bool
}

func buildTar(t *testing.T, gzipped bool, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	var tw *tar.Writer
	var gz *gzip.Writer
	if gzipped {
		gz = gzip.NewWriter(&buf)
		tw = tar.NewWriter(gz)
	} else {
		tw = tar.NewWriter(&buf)
	}
	for _, e := range entries {
		flag := e.typeflag
		if flag == 0 {
			flag = tar.TypeReg
		}
		size := int64(len(e.data))
		if e.sizeSet {
			size = e.size
		}
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: flag,
			Size:     size,
			Mode:     0o644,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%q): %v", e.name, err)
		}
		if flag == tar.TypeReg {
			if _, err := tw.Write(e.data); err != nil {
				t.Fatalf("Write(%q): %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	if gzipped {
		if err := gz.Close(); err != nil {
			t.Fatalf("gzip Close: %v", err)
		}
	}
	return buf.Bytes()
}

const goodManifest = `{"abi":1,"slug":"demo","name":"Demo","description":"a test cart","version":"1.0.0","players":[1,2],"icon":"assets/icon.png"}`

var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func goodEntries() []tarEntry {
	return []tarEntry{
		{name: ManifestName, data: []byte(goodManifest)},
		{name: WasmName, data: []byte("\x00asm-fake-wasm-bytes")},
		{name: "assets/icon.png", data: tinyPNG},
	}
}

func TestLoadGoodBundle(t *testing.T) {
	for _, gzipped := range []bool{false, true} {
		data := buildTar(t, gzipped, goodEntries())
		b, err := Load(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("gzipped=%v: Load: %v", gzipped, err)
		}
		if b.Manifest.Slug != "demo" || b.Manifest.Name != "Demo" {
			t.Fatalf("gzipped=%v: manifest = %+v", gzipped, b.Manifest)
		}
		if b.Manifest.Description != "a test cart" {
			t.Fatalf("gzipped=%v: description = %q", gzipped, b.Manifest.Description)
		}
		if string(b.Wasm) != "\x00asm-fake-wasm-bytes" {
			t.Fatalf("gzipped=%v: wasm bytes = %q", gzipped, b.Wasm)
		}
		if b.Icon == nil {
			t.Fatalf("gzipped=%v: icon not decoded, warnings=%v", gzipped, b.Warnings)
		}
		if len(b.Warnings) != 0 {
			t.Fatalf("gzipped=%v: unexpected warnings %v", gzipped, b.Warnings)
		}
		if _, ok := b.Assets[ManifestName]; ok {
			t.Fatalf("gzipped=%v: manifest.json leaked into Assets", gzipped)
		}
		if _, ok := b.Assets[WasmName]; ok {
			t.Fatalf("gzipped=%v: game.wasm leaked into Assets", gzipped)
		}
	}
}

func TestLoadIconIs8x8(t *testing.T) {
	entries := []tarEntry{
		{name: ManifestName, data: []byte(`{"slug":"demo","name":"Demo","icon":"assets/icon.png"}`)},
		{name: WasmName, data: []byte("wasm")},
		{name: "assets/icon.png", data: encode8x8PNG(t)},
	}
	b, err := Load(bytes.NewReader(buildTar(t, false, entries)))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(b.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", b.Warnings)
	}
	if len(b.Icon) != 8 {
		t.Fatalf("icon has %d rows, want 8", len(b.Icon))
	}
	for y, row := range b.Icon {
		if len(row) != 8 {
			t.Fatalf("icon row %d has %d cols, want 8", y, len(row))
		}
	}
}

func TestLoadMissingIconWarns(t *testing.T) {
	entries := []tarEntry{
		{name: ManifestName, data: []byte(`{"slug":"demo","name":"Demo","icon":"assets/nope.png"}`)},
		{name: WasmName, data: []byte("wasm")},
	}
	b, err := Load(bytes.NewReader(buildTar(t, false, entries)))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if b.Icon != nil {
		t.Fatalf("expected no icon, got %v", b.Icon)
	}
	if len(b.Warnings) != 1 {
		t.Fatalf("expected one warning, got %v", b.Warnings)
	}
}

func TestLoadMissingManifest(t *testing.T) {
	entries := []tarEntry{{name: WasmName, data: []byte("wasm")}}
	_, err := Load(bytes.NewReader(buildTar(t, false, entries)))
	if err == nil || !strings.Contains(err.Error(), "missing manifest.json") {
		t.Fatalf("Load: got %v, want a missing-manifest error", err)
	}
}

func TestLoadMissingWasm(t *testing.T) {
	entries := []tarEntry{{name: ManifestName, data: []byte(goodManifest)}}
	_, err := Load(bytes.NewReader(buildTar(t, false, entries)))
	if err == nil || !strings.Contains(err.Error(), "missing game.wasm") {
		t.Fatalf("Load: got %v, want a missing-wasm error", err)
	}
}

func TestLoadEmptyWasm(t *testing.T) {
	entries := []tarEntry{
		{name: ManifestName, data: []byte(goodManifest)},
		{name: WasmName, data: []byte{}},
	}
	_, err := Load(bytes.NewReader(buildTar(t, false, entries)))
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("Load: got %v, want an empty-wasm error", err)
	}
}

func TestLoadRejectsMissingRequiredFields(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
	}{
		{"no slug", `{"name":"Demo"}`},
		{"no name", `{"slug":"demo"}`},
		{"invalid json", `{not json`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entries := []tarEntry{
				{name: ManifestName, data: []byte(c.manifest)},
				{name: WasmName, data: []byte("wasm")},
			}
			_, err := Load(bytes.NewReader(buildTar(t, false, entries)))
			if err == nil {
				t.Fatalf("Load: expected an error for manifest %q", c.manifest)
			}
		})
	}
}

func TestLoadRejectsPathTraversal(t *testing.T) {
	cases := []string{
		"../escape.txt",
		"assets/../../escape.txt",
		"/etc/passwd",
		"assets/../../../etc/passwd",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			entries := append(goodEntries(), tarEntry{name: name, data: []byte("evil")})
			_, err := Load(bytes.NewReader(buildTar(t, false, entries)))
			if err == nil {
				t.Fatalf("Load: expected a path-traversal rejection for %q", name)
			}
			if !strings.Contains(err.Error(), "illegal entry path") {
				t.Fatalf("Load(%q): got %v, want an illegal-path error", name, err)
			}
		})
	}
}

func TestLoadRejectsDuplicateEntries(t *testing.T) {
	entries := []tarEntry{
		{name: ManifestName, data: []byte(goodManifest)},
		{name: WasmName, data: []byte("wasm-one")},
		{name: WasmName, data: []byte("wasm-two")},
	}
	_, err := Load(bytes.NewReader(buildTar(t, false, entries)))
	if err == nil || !strings.Contains(err.Error(), "duplicate entry") {
		t.Fatalf("Load: got %v, want a duplicate-entry error", err)
	}
}

// buildTarWithDangerousHeader appends a header claiming dangerousSize with
// no body and no final padding, which tar.Writer.Close would refuse to emit.
func buildTarWithDangerousHeader(t *testing.T, normalEntries []tarEntry, dangerousName string, dangerousSize int64) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range normalEntries {
		flag := e.typeflag
		if flag == 0 {
			flag = tar.TypeReg
		}
		hdr := &tar.Header{Name: e.name, Typeflag: flag, Size: int64(len(e.data)), Mode: 0o644}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%q): %v", e.name, err)
		}
		if _, err := tw.Write(e.data); err != nil {
			t.Fatalf("Write(%q): %v", e.name, err)
		}
	}
	hdr := &tar.Header{Name: dangerousName, Typeflag: tar.TypeReg, Size: dangerousSize, Mode: 0o644}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("WriteHeader(%q): %v", dangerousName, err)
	}
	return buf.Bytes()
}

func TestLoadRejectsOversizedWasm(t *testing.T) {
	data := buildTarWithDangerousHeader(t, []tarEntry{
		{name: ManifestName, data: []byte(goodManifest)},
	}, WasmName, MaxWasmBytes+1)
	_, err := Load(bytes.NewReader(data))
	if err == nil || !strings.Contains(err.Error(), "byte cap") {
		t.Fatalf("Load: got %v, want an over-cap rejection", err)
	}
}

func TestLoadRejectsDecompressionBomb(t *testing.T) {
	raw := buildTarWithDangerousHeader(t, []tarEntry{
		{name: ManifestName, data: []byte(goodManifest)},
		{name: WasmName, data: []byte("wasm")},
	}, "assets/huge.bin", 1<<30)

	var gzbuf bytes.Buffer
	gz := gzip.NewWriter(&gzbuf)
	if _, err := gz.Write(raw); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	if gzbuf.Len() > 1<<20 {
		t.Fatalf("test fixture itself is %d bytes, something's wrong with the bomb setup", gzbuf.Len())
	}

	_, err := Load(bytes.NewReader(gzbuf.Bytes()))
	if err == nil || !strings.Contains(err.Error(), "byte cap") {
		t.Fatalf("Load: got %v, want a total-cap rejection", err)
	}
}

func TestLoadRejectsTooManyEntries(t *testing.T) {
	entries := goodEntries()
	for i := 0; i < MaxEntries+1; i++ {
		entries = append(entries, tarEntry{name: filepath.Join("assets", "many", strconv(i)), data: []byte("x")})
	}
	_, err := Load(bytes.NewReader(buildTar(t, false, entries)))
	if err == nil || !strings.Contains(err.Error(), "entries") {
		t.Fatalf("Load: got %v, want a too-many-entries rejection", err)
	}
}

func TestLoadRejectsSymlink(t *testing.T) {
	entries := append(goodEntries(), tarEntry{name: "assets/link", typeflag: tar.TypeSymlink, data: nil})
	_, err := Load(bytes.NewReader(buildTar(t, false, entries)))
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("Load: got %v, want an unsupported-entry-type rejection", err)
	}
}

func strconv(i int) string {
	if i == 0 {
		return "0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

func TestLoadBareWasm(t *testing.T) {
	b, err := LoadBareWasm("snake", []byte("wasmbytes"))
	if err != nil {
		t.Fatalf("LoadBareWasm: %v", err)
	}
	if b.Manifest.Slug != "snake" || b.Manifest.Name != "Snake" {
		t.Fatalf("manifest = %+v", b.Manifest)
	}
	if string(b.Wasm) != "wasmbytes" {
		t.Fatalf("wasm = %q", b.Wasm)
	}
}

func TestLoadBareWasmTitleCase(t *testing.T) {
	b, err := LoadBareWasm("my-cool_game", []byte("x"))
	if err != nil {
		t.Fatalf("LoadBareWasm: %v", err)
	}
	if b.Manifest.Name != "My Cool Game" {
		t.Fatalf("name = %q, want %q", b.Manifest.Name, "My Cool Game")
	}
}

func TestLoadBareWasmRejectsEmpty(t *testing.T) {
	if _, err := LoadBareWasm("snake", nil); err == nil {
		t.Fatal("LoadBareWasm: expected an error for empty wasm")
	}
}

func TestLoadBareWasmRejectsOversized(t *testing.T) {
	if _, err := LoadBareWasm("snake", make([]byte, MaxWasmBytes+1)); err == nil {
		t.Fatal("LoadBareWasm: expected an error for oversized wasm")
	}
}

func TestLoadFileDispatchesByExtension(t *testing.T) {
	dir := t.TempDir()

	rcartPath := filepath.Join(dir, "demo.rcart")
	if err := os.WriteFile(rcartPath, buildTar(t, false, goodEntries()), 0o644); err != nil {
		t.Fatalf("write .rcart: %v", err)
	}
	b, err := LoadFile(rcartPath)
	if err != nil {
		t.Fatalf("LoadFile(.rcart): %v", err)
	}
	if b.Manifest.Slug != "demo" {
		t.Fatalf("slug = %q", b.Manifest.Slug)
	}

	wasmPath := filepath.Join(dir, "snake.wasm")
	if err := os.WriteFile(wasmPath, []byte("wasmbytes"), 0o644); err != nil {
		t.Fatalf("write .wasm: %v", err)
	}
	b2, err := LoadFile(wasmPath)
	if err != nil {
		t.Fatalf("LoadFile(.wasm): %v", err)
	}
	if b2.Manifest.Slug != "snake" || b2.Manifest.Name != "Snake" {
		t.Fatalf("manifest = %+v", b2.Manifest)
	}

	txtPath := filepath.Join(dir, "readme.txt")
	if err := os.WriteFile(txtPath, []byte("hi"), 0o644); err != nil {
		t.Fatalf("write .txt: %v", err)
	}
	if _, err := LoadFile(txtPath); err == nil {
		t.Fatal("LoadFile(.txt): expected an unsupported-extension error")
	}
}

func TestLoadFileRejectsSlugMismatch(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "not-demo.rcart")
	if err := os.WriteFile(p, buildTar(t, false, goodEntries()), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := LoadFile(p)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("LoadFile: got %v, want a slug-mismatch error", err)
	}
}

const glyphManifest = `{"abi":1,"slug":"demo","name":"Demo","assets":{"glyphs":["assets/glyphs/coin.arkadeglyph.json"]}}`

var goodGlyphFile = []byte(`{"arkade":1,"kind":"glyph","name":"coin",` +
	`"generator":{"tool":"editor","version":"1.0.0"},` +
	`"codepoint":164,"rows":[14,17,21,21,21,17,14,0],"category":"tetromino"}`)

func glyphEntries(file []byte) []tarEntry {
	return []tarEntry{
		{name: ManifestName, data: []byte(glyphManifest)},
		{name: WasmName, data: []byte("\x00asm-fake-wasm-bytes")},
		{name: "assets/glyphs/coin.arkadeglyph.json", data: file},
	}
}

func TestLoadReadsTheManifestGlyphList(t *testing.T) {
	b, err := Load(bytes.NewReader(buildTar(t, false, glyphEntries(goodGlyphFile))))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"assets/glyphs/coin.arkadeglyph.json"}
	if got := b.Manifest.GlyphAssets(); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("GlyphAssets() = %v, want %v", got, want)
	}
	if len(b.Warnings) != 0 {
		t.Fatalf("a valid glyph should warn about nothing, got %v", b.Warnings)
	}
}

func TestLoadWarnsAboutGlyphsTheBundleCannotDeliver(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []tarEntry
		want    string
	}{
		{
			name:    "listed but absent",
			entries: glyphEntries(nil)[:2], // manifest and wasm only
			want:    "not found in bundle",
		},
		{
			name: "listed under a name that is not a glyph file",
			entries: []tarEntry{
				{name: ManifestName, data: []byte(`{"abi":1,"slug":"demo","name":"Demo","assets":{"glyphs":["assets/coin.json"]}}`)},
				{name: WasmName, data: []byte("\x00asm-fake-wasm-bytes")},
				{name: "assets/coin.json", data: goodGlyphFile},
			},
			want: "not a <name>.arkadeglyph.json file",
		},
		{
			name:    "ASCII code point",
			entries: glyphEntries([]byte(`{"arkade":1,"kind":"glyph","name":"a","generator":{"tool":"t","version":"1"},"codepoint":65,"rows":[1,1,1,1,1,1,1,1]}`)),
			want:    "below U+0080",
		},
		{
			name:    "seven rows",
			entries: glyphEntries([]byte(`{"arkade":1,"kind":"glyph","name":"a","generator":{"tool":"t","version":"1"},"codepoint":164,"rows":[1,1,1,1,1,1,1]}`)),
			want:    "rows, want 8",
		},
		{
			name:    "not JSON at all",
			entries: glyphEntries([]byte("not json")),
			want:    "invalid character",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := Load(bytes.NewReader(buildTar(t, false, tc.entries)))
			if err != nil {
				t.Fatalf("a broken glyph must never fail the LOAD (it is a mount-time warning): %v", err)
			}
			if len(b.Warnings) != 1 || !strings.Contains(b.Warnings[0].Message, tc.want) {
				t.Fatalf("warnings = %v, want exactly one mentioning %q", b.Warnings, tc.want)
			}
			if w := b.Warnings[0]; w.Category != CategoryGlyph || w.Severity != Undeliverable {
				t.Errorf("warning %+v, want an undeliverable glyph warning, which rcartpack refuses", w)
			}
		})
	}
}
