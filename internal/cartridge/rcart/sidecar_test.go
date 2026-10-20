// SPDX-License-Identifier: GPL-2.0-or-later

package rcart

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSidecarProject(t *testing.T, slug string) string {
	t.Helper()
	dir := t.TempDir()
	m := schema1Manifest()
	m.Slug = slug
	m.Logo = "assets/logo.png"
	raw, err := json.Marshal(&m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestName), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, slug+".wasm"), []byte("\x00asm"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "MANUAL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	png := encode8x8PNG(t)
	for _, name := range []string{"icon.png", "logo.png"} {
		if err := os.WriteFile(filepath.Join(dir, "assets", name), png, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "blip.wav"), []byte("RIFFxxxx"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadFileSidecarManifest(t *testing.T) {
	dir := writeSidecarProject(t, "demo")
	b, err := LoadFile(filepath.Join(dir, "demo.wasm"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if b.Manifest.Title != "LIFE" || b.Manifest.SchemaVersion != 1 {
		t.Errorf("sidecar manifest not honored: %+v", b.Manifest)
	}
	if len(b.Manifest.Tags) == 0 {
		t.Error("sidecar tags lost")
	}
	if b.Icon == nil || b.Logo == nil {
		t.Errorf("icon/logo not decoded from the assets dir (icon=%v logo=%v)", b.Icon != nil, b.Logo != nil)
	}
	for _, want := range []string{"MANUAL.md", "assets/blip.wav"} {
		if _, ok := b.Assets[want]; !ok {
			t.Errorf("%s missing from directory-backed bundle assets", want)
		}
	}
	if len(b.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", b.Warnings)
	}
}

func TestLoadFileSidecarSlugMismatch(t *testing.T) {
	dir := writeSidecarProject(t, "demo")
	if err := os.Rename(filepath.Join(dir, "demo.wasm"), filepath.Join(dir, "other.wasm")); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFile(filepath.Join(dir, "other.wasm"))
	if err == nil || !strings.Contains(err.Error(), "slug") {
		t.Fatalf("slug/stem mismatch not rejected: %v", err)
	}
}

func TestLoadFileSidecarInvalidManifestFailsLoud(t *testing.T) {
	dir := writeSidecarProject(t, "demo")
	if err := os.WriteFile(filepath.Join(dir, ManifestName), []byte(`{"schemaVersion":1,"slug":"demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// No silent fallback to a bare cartridge.
	_, err := LoadFile(filepath.Join(dir, "demo.wasm"))
	if err == nil || !strings.Contains(err.Error(), "title") {
		t.Fatalf("invalid sidecar manifest not rejected: %v", err)
	}
}
