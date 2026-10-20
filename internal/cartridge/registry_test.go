// SPDX-License-Identifier: GPL-2.0-or-later

package cartridge_test

import (
	"archive/tar"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/cartridge"
	"github.com/stealthmonkey99/arkade/internal/engine"
)

// writeRcart's wasm is a placeholder: Scan parses but never instantiates.
func writeRcart(t *testing.T, dir, slug, name, description string) string {
	t.Helper()
	manifest := `{"abi":1,"slug":"` + slug + `","name":"` + name + `","description":"` + description + `"}`
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range []struct{ name, data string }{
		{"manifest.json", manifest},
		{"game.wasm", "\x00asm-placeholder"},
	} {
		hdr := &tar.Header{Name: e.name, Typeflag: tar.TypeReg, Size: int64(len(e.data)), Mode: 0o644}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if _, err := tw.Write([]byte(e.data)); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	path := filepath.Join(dir, slug+".rcart")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func writeRcartWithIcon(t *testing.T, dir, slug, name string) string {
	t.Helper()
	manifest := `{"abi":1,"slug":"` + slug + `","name":"` + name + `","icon":"assets/icon.png"}`

	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if (x+y)%2 == 0 {
				img.SetNRGBA(x, y, color.NRGBA{R: 15, G: 188, B: 122, A: 255})
			}
		}
	}
	var iconBuf bytes.Buffer
	if err := png.Encode(&iconBuf, img); err != nil {
		t.Fatalf("encode icon PNG: %v", err)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range []struct {
		name string
		data []byte
	}{
		{"manifest.json", []byte(manifest)},
		{"game.wasm", []byte("\x00asm-placeholder")},
		{"assets/icon.png", iconBuf.Bytes()},
	} {
		hdr := &tar.Header{Name: e.name, Typeflag: tar.TypeReg, Size: int64(len(e.data)), Mode: 0o644}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if _, err := tw.Write(e.data); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	path := filepath.Join(dir, slug+".rcart")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func writeBareWasm(t *testing.T, dir, slug string) string {
	t.Helper()
	path := filepath.Join(dir, slug+".wasm")
	if err := os.WriteFile(path, []byte("\x00asm-placeholder-"+slug), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// cleanupRegistry unregisters slugs whatever Scan did, since the engine
// registry is process-global.
func cleanupRegistry(t *testing.T, slugs ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, s := range slugs {
			engine.Unregister(s)
		}
	})
}

func TestScanRegistersBareWasm(t *testing.T) {
	dir := t.TempDir()
	writeBareWasm(t, dir, "snake")
	cleanupRegistry(t, "cart/snake")

	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{dir}})
	res := reg.Scan()

	if len(res.Added) != 1 || res.Added[0] != "cart/snake" {
		t.Fatalf("Added = %v, want [cart/snake]", res.Added)
	}
	m, err := engine.Lookup("cart/snake")
	if err != nil {
		t.Fatalf("Lookup(cart/snake): %v", err)
	}
	if m.Name != "Snake" {
		t.Errorf("Name = %q, want Snake", m.Name)
	}
	if m.Type != engine.TypeGame {
		t.Errorf("Type = %v, want TypeGame", m.Type)
	}
	if m.New == nil {
		t.Error("New is nil")
	}
}

func TestScanRegistersRcartBundle(t *testing.T) {
	dir := t.TempDir()
	writeRcart(t, dir, "demo", "Demo Game", "a fine demo")
	cleanupRegistry(t, "cart/demo")

	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{dir}})
	res := reg.Scan()

	if len(res.Added) != 1 || res.Added[0] != "cart/demo" {
		t.Fatalf("Added = %v, want [cart/demo]", res.Added)
	}
	m, err := engine.Lookup("cart/demo")
	if err != nil {
		t.Fatalf("Lookup(cart/demo): %v", err)
	}
	if m.Name != "Demo Game" || m.Description != "a fine demo" {
		t.Errorf("manifest = %+v", m)
	}
}

func TestScanRegistersRcartBundleIcon(t *testing.T) {
	dir := t.TempDir()
	writeRcartWithIcon(t, dir, "iconic", "Iconic Game")
	cleanupRegistry(t, "cart/iconic")

	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{dir}})
	res := reg.Scan()

	if len(res.Added) != 1 || res.Added[0] != "cart/iconic" {
		t.Fatalf("Added = %v, want [cart/iconic]", res.Added)
	}
	m, err := engine.Lookup("cart/iconic")
	if err != nil {
		t.Fatalf("Lookup(cart/iconic): %v", err)
	}
	if m.Icon == nil {
		t.Fatal("Manifest.Icon is nil, want the bundle's 8x8 icon")
	}
	if len(m.Icon) != 8 {
		t.Fatalf("Manifest.Icon has %d rows, want 8", len(m.Icon))
	}
	for y, row := range m.Icon {
		if len(row) != 8 {
			t.Fatalf("Manifest.Icon row %d has %d cols, want 8", y, len(row))
		}
	}
}

func TestScanIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	writeBareWasm(t, dir, "snake")
	cleanupRegistry(t, "cart/snake")

	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{dir}})
	first := reg.Scan()
	if len(first.Added) != 1 {
		t.Fatalf("first scan Added = %v, want 1 entry", first.Added)
	}

	second := reg.Scan()
	if len(second.Added) != 0 || len(second.Removed) != 0 {
		t.Fatalf("rescan of an unchanged directory: Added=%v Removed=%v, want both empty", second.Added, second.Removed)
	}
	if _, err := engine.Lookup("cart/snake"); err != nil {
		t.Fatalf("Lookup(cart/snake) after idempotent rescan: %v", err)
	}
}

func TestScanDetectsRemoval(t *testing.T) {
	dir := t.TempDir()
	path := writeBareWasm(t, dir, "snake")
	cleanupRegistry(t, "cart/snake")

	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{dir}})
	reg.Scan()
	if _, err := engine.Lookup("cart/snake"); err != nil {
		t.Fatalf("Lookup(cart/snake) after first scan: %v", err)
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("os.Remove: %v", err)
	}
	res := reg.Scan()
	if len(res.Removed) != 1 || res.Removed[0] != "cart/snake" {
		t.Fatalf("Removed = %v, want [cart/snake]", res.Removed)
	}
	if _, err := engine.Lookup("cart/snake"); err == nil {
		t.Fatal("Lookup(cart/snake) after removal: expected an error")
	}
}

func TestScanDetectsChange(t *testing.T) {
	dir := t.TempDir()
	path := writeRcart(t, dir, "demo", "Demo V1", "")
	cleanupRegistry(t, "cart/demo")

	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{dir}})
	reg.Scan()
	m1, err := engine.Lookup("cart/demo")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if m1.Name != "Demo V1" {
		t.Fatalf("Name = %q, want Demo V1", m1.Name)
	}

	writeRcart(t, dir, "demo", "Demo V2", "")
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	res := reg.Scan()
	if len(res.Removed) != 1 || res.Removed[0] != "cart/demo" {
		t.Fatalf("Removed = %v, want [cart/demo] (old registration must go before the new one lands)", res.Removed)
	}
	if len(res.Added) != 1 || res.Added[0] != "cart/demo" {
		t.Fatalf("Added = %v, want [cart/demo]", res.Added)
	}
	m2, err := engine.Lookup("cart/demo")
	if err != nil {
		t.Fatalf("Lookup after change: %v", err)
	}
	if m2.Name != "Demo V2" {
		t.Fatalf("Name = %q, want Demo V2", m2.Name)
	}
}

func TestScanSkipsSlugCollisionBetweenTwoCartridges(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	writeBareWasm(t, dirA, "snake")
	writeBareWasm(t, dirB, "snake")
	cleanupRegistry(t, "cart/snake")

	// Which directory wins depends on temp-dir naming, so assert counts only.
	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{dirA, dirB}})
	res := reg.Scan()

	if len(res.Added) != 1 {
		t.Fatalf("Added = %v, want exactly one registration", res.Added)
	}
	if len(res.Skipped) != 1 {
		t.Fatalf("Skipped = %v, want exactly one collision skip", res.Skipped)
	}
	if _, err := engine.Lookup("cart/snake"); err != nil {
		t.Fatalf("Lookup(cart/snake): %v", err)
	}
}

// A stick carrying a slug the library already has is skipped but still
// discovered, so inserting it auto-launches the installed copy.
func TestScanDiscoversCollidingStickCartridge(t *testing.T) {
	library := t.TempDir()
	writeBareWasm(t, library, "snake")
	cleanupRegistry(t, "cart/snake")

	stick := t.TempDir()
	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{library, stick}})

	first := reg.Scan()
	if len(first.Discovered) != 1 || first.Discovered[0] != "cart/snake" {
		t.Fatalf("boot scan Discovered = %v, want [cart/snake]", first.Discovered)
	}

	if res := reg.Scan(); len(res.Discovered) != 0 {
		t.Fatalf("idle rescan Discovered = %v, want none", res.Discovered)
	}

	writeBareWasm(t, stick, "snake")
	res := reg.Scan()
	if len(res.Added) != 0 {
		t.Fatalf("Added = %v, want none (collision keeps the library copy)", res.Added)
	}
	if len(res.Skipped) != 1 {
		t.Fatalf("Skipped = %v, want the collision skip", res.Skipped)
	}
	if len(res.Discovered) != 1 || res.Discovered[0] != "cart/snake" {
		t.Fatalf("Discovered = %v, want [cart/snake]", res.Discovered)
	}

	if res := reg.Scan(); len(res.Discovered) != 0 {
		t.Fatalf("second rescan Discovered = %v, want none (stick still tracked)", res.Discovered)
	}
}

func TestScanSkipsCollisionWithExistingApp(t *testing.T) {
	const slug = "cart/builtin-lookalike"
	engine.Register(engine.Manifest{
		Slug: slug, Name: "Built-in", Type: engine.TypeGame, Hidden: true,
		New: func(ctx *engine.AppContext) engine.App { return nil },
	})
	cleanupRegistry(t, slug)

	dir := t.TempDir()
	writeBareWasm(t, dir, "builtin-lookalike")

	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{dir}})
	res := reg.Scan()

	if len(res.Added) != 0 {
		t.Fatalf("Added = %v, want none (the app registration must win)", res.Added)
	}
	if len(res.Skipped) != 1 {
		t.Fatalf("Skipped = %v, want one collision skip", res.Skipped)
	}
	m, err := engine.Lookup(slug)
	if err != nil {
		t.Fatalf("Lookup(%q): %v", slug, err)
	}
	if m.Name != "Built-in" {
		t.Fatalf("Lookup(%q).Name = %q, the pre-existing app must not have been replaced", slug, m.Name)
	}
}

func TestScanSkipsHostileBundleWithoutCrashing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.rcart"), []byte("not a tar file"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	writeBareWasm(t, dir, "good")
	cleanupRegistry(t, "cart/good")

	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{dir}})
	res := reg.Scan()

	if len(res.Skipped) != 1 {
		t.Fatalf("Skipped = %v, want one parse-error skip", res.Skipped)
	}
	if len(res.Added) != 1 || res.Added[0] != "cart/good" {
		t.Fatalf("Added = %v, want [cart/good] (the bad neighbor must not block a good cartridge)", res.Added)
	}
}

func TestScanExpandsGlobDirs(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "usb1")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	writeBareWasm(t, sub, "snake")
	cleanupRegistry(t, "cart/snake")

	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{filepath.Join(root, "*")}})
	res := reg.Scan()

	if len(res.Added) != 1 || res.Added[0] != "cart/snake" {
		t.Fatalf("Added = %v, want [cart/snake] via glob expansion", res.Added)
	}
}

func TestScanIgnoresMissingDirs(t *testing.T) {
	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{filepath.Join(t.TempDir(), "does-not-exist")}})
	res := reg.Scan()
	if len(res.Added) != 0 || len(res.Skipped) != 0 {
		t.Fatalf("scanning a missing dir: Added=%v Skipped=%v, want both empty", res.Added, res.Skipped)
	}
}
