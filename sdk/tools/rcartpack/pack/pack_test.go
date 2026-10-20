// SPDX-License-Identifier: GPL-2.0-or-later

package pack

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/cartridge/rcart"
)

const testManifestJSON = `{
  "schemaVersion": 1,
  "abi": 1,
  "slug": "widget",
  "title": "Widget",
  "publisher": "arkade",
  "author": "Ada",
  "version": "1.0.0",
  "description": "A test cartridge.",
  "tags": ["puzzle"],
  "players": ["1p"],
  "audience": "all-ages",
  "visibility": "private",
  "icon": "assets/icon.png",
  "manual": "MANUAL.md",
  "links": {"source": null}
}`

func run(dir, wasm, out string) error {
	_, err := Pack(Options{Dir: dir, Wasm: wasm, Out: out})
	return err
}

func writeCart(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(testManifestJSON), 0o644))
	must(os.WriteFile(filepath.Join(dir, "widget.wasm"), []byte("\x00asm-widget"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "MANUAL.md"), []byte("# Widget\n"), 0o644))
	must(os.MkdirAll(filepath.Join(dir, "assets", "sprites"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "assets", "icon.png"), []byte("icon-bytes"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "assets", "ding.wav"), []byte("ding-bytes"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "assets", "sprites", "hero.png"), []byte("hero-bytes"), 0o644))
	return dir
}

func TestRunPacksNestedAssetsAndRoundTrips(t *testing.T) {
	dir := writeCart(t)
	out := filepath.Join(dir, "widget.rcart")
	if err := run(dir, filepath.Join(dir, "widget.wasm"), out); err != nil {
		t.Fatalf("run: %v", err)
	}

	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	b, err := rcart.Load(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("re-loading packed bundle: %v", err)
	}
	if b.Manifest.Slug != "widget" {
		t.Fatalf("slug = %q", b.Manifest.Slug)
	}
	if string(b.Wasm) != "\x00asm-widget" {
		t.Fatalf("wasm = %q", b.Wasm)
	}
	if _, ok := b.Assets["MANUAL.md"]; !ok {
		t.Error("MANUAL.md was not packed into the bundle")
	}
	if got, ok := b.Assets["assets/sprites/hero.png"]; !ok || string(got) != "hero-bytes" {
		t.Errorf("assets/sprites/hero.png = %q, ok=%v; want hero-bytes, true (nested assets/ subdirectory must be packed)", got, ok)
	}
	if got := string(b.Assets["assets/ding.wav"]); got != "ding-bytes" {
		t.Errorf("assets/ding.wav = %q", got)
	}
}

func TestRunLeavesLevelPacksOutOfTheBundle(t *testing.T) {
	dir := writeCart(t)
	for _, name := range []string{"walls.arkadelevels", "old.levels", "sprites/nested.arkadelevels"} {
		if err := os.WriteFile(filepath.Join(dir, "assets", filepath.FromSlash(name)), []byte("pack walls\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "widget.rcart")
	res, err := Pack(Options{Dir: dir, Wasm: filepath.Join(dir, "widget.wasm"), Out: out})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	b, err := rcart.Load(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	for name := range b.Assets {
		if strings.HasSuffix(name, "levels") {
			t.Errorf("bundle carries level pack %q", name)
		}
	}
	if res.Assets != 4 {
		t.Errorf("Assets = %d, want 4 (manual, icon, ding, hero)", res.Assets)
	}
}

func TestRunDefaultsWasmPathToDirGameWasm(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(testManifestJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "game.wasm"), []byte("\x00asm"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "MANUAL.md"), []byte("# Widget\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "widget.rcart")
	if err := run(dir, "", out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}
}

func TestRunRejectsAppleDoubleJunkInAssets(t *testing.T) {
	dir := writeCart(t)
	if err := os.WriteFile(filepath.Join(dir, "assets", "._icon.png"), []byte("resource-fork-junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "widget.rcart")
	err := run(dir, filepath.Join(dir, "widget.wasm"), out)
	if err == nil {
		t.Fatal("run succeeded despite AppleDouble junk in assets/")
	}
	if !strings.Contains(err.Error(), "._icon.png") {
		t.Fatalf("err = %v, want it to name the offending file", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Error("output file written despite junk-refusal failure")
	}
}

func TestRunRejectsDSStoreInAssets(t *testing.T) {
	dir := writeCart(t)
	if err := os.WriteFile(filepath.Join(dir, "assets", ".DS_Store"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "widget.rcart")
	err := run(dir, filepath.Join(dir, "widget.wasm"), out)
	if err == nil || !strings.Contains(err.Error(), ".DS_Store") {
		t.Fatalf("err = %v, want a .DS_Store complaint", err)
	}
}

func TestRunRejectsSlugFilenameMismatch(t *testing.T) {
	dir := writeCart(t)
	out := filepath.Join(dir, "not-widget.rcart")
	err := run(dir, filepath.Join(dir, "widget.wasm"), out)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("err = %v, want a slug/filename mismatch complaint", err)
	}
}

func TestRunDeterministic(t *testing.T) {
	dir := writeCart(t)
	out1 := filepath.Join(dir, "widget.rcart")
	if err := run(dir, filepath.Join(dir, "widget.wasm"), out1); err != nil {
		t.Fatalf("run: %v", err)
	}
	dir2 := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	must(err)
	must(os.WriteFile(filepath.Join(dir2, "manifest.json"), data, 0o644))
	data, err = os.ReadFile(filepath.Join(dir, "widget.wasm"))
	must(err)
	must(os.WriteFile(filepath.Join(dir2, "widget.wasm"), data, 0o644))
	data, err = os.ReadFile(filepath.Join(dir, "MANUAL.md"))
	must(err)
	must(os.WriteFile(filepath.Join(dir2, "MANUAL.md"), data, 0o644))
	must(os.MkdirAll(filepath.Join(dir2, "assets", "sprites"), 0o755))
	for _, rel := range []string{filepath.Join("assets", "icon.png"), filepath.Join("assets", "ding.wav"), filepath.Join("assets", "sprites", "hero.png")} {
		data, err = os.ReadFile(filepath.Join(dir, rel))
		must(err)
		must(os.WriteFile(filepath.Join(dir2, rel), data, 0o644))
	}
	out2 := filepath.Join(dir2, "widget.rcart")
	if err := run(dir2, filepath.Join(dir2, "widget.wasm"), out2); err != nil {
		t.Fatalf("run (second copy): %v", err)
	}

	b1, err := os.ReadFile(out1)
	must(err)
	b2, err := os.ReadFile(out2)
	must(err)
	if !bytes.Equal(b1, b2) {
		t.Fatal("packing the same cartridge content from two directories produced different bytes")
	}
}

// assets/ding.wav from writeCart is not a WAV and is the control: only a name
// claiming an envelope is held to one.
func TestRunRefusesABrokenSoundAssetFile(t *testing.T) {
	dir := writeCart(t)
	if err := os.WriteFile(filepath.Join(dir, "assets", "blip.arkadesound.wav"), []byte("RIFF....WAVEfmt "), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "widget.rcart")
	err := run(dir, filepath.Join(dir, "widget.wasm"), out)
	if err == nil {
		t.Fatal("packing a bundle with an unreadable sound asset must fail")
	}
	if !strings.Contains(err.Error(), "blip.arkadesound.wav") {
		t.Errorf("error = %v, want it to name the file", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Error("a refused pack must not leave a bundle behind")
	}
}

func TestRunPacksAGoodSoundAssetFile(t *testing.T) {
	dir := writeCart(t)
	name := "blip.arkadesound.wav"
	wav := goodSoundAsset(t, name)
	if err := os.WriteFile(filepath.Join(dir, "assets", name), wav, 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "widget.rcart")
	if err := run(dir, filepath.Join(dir, "widget.wasm"), out); err != nil {
		t.Fatalf("run: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	b, err := rcart.Load(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := b.Assets["assets/"+name]
	if !ok {
		t.Fatalf("%s was not packed", name)
	}
	if !bytes.Equal(got, wav) {
		t.Error("the packed sound is not byte-identical to the file on disk")
	}
	if _, err := assetfile.DecodeSound("assets/"+name, got); err != nil {
		t.Errorf("the packed sound no longer validates: %v", err)
	}
}

func goodSoundAsset(t *testing.T, name string) []byte {
	t.Helper()
	const rate, channels, bits, frames = 22050, 1, 16, 16
	blockAlign := channels * bits / 8
	fmtChunk := make([]byte, 16)
	binary.LittleEndian.PutUint16(fmtChunk[0:2], 1)
	binary.LittleEndian.PutUint16(fmtChunk[2:4], channels)
	binary.LittleEndian.PutUint32(fmtChunk[4:8], rate)
	binary.LittleEndian.PutUint32(fmtChunk[8:12], uint32(rate*blockAlign))
	binary.LittleEndian.PutUint16(fmtChunk[12:14], uint16(blockAlign))
	binary.LittleEndian.PutUint16(fmtChunk[14:16], bits)
	pcm := make([]byte, frames*blockAlign)

	var body bytes.Buffer
	body.WriteString("WAVE")
	writeChunk := func(id string, data []byte) {
		body.WriteString(id)
		var n [4]byte
		binary.LittleEndian.PutUint32(n[:], uint32(len(data)))
		body.Write(n[:])
		body.Write(data)
	}
	writeChunk("fmt ", fmtChunk)
	writeChunk("data", pcm)
	var file bytes.Buffer
	file.WriteString("RIFF")
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(body.Len()))
	file.Write(n[:])
	file.Write(body.Bytes())

	env, err := assetfile.NewSoundEnvelope(assetfile.SoundMeta{
		Name:      "blip",
		Generator: assetfile.Generator{Tool: "rcartpack-test", Version: "1.0.0"},
		Recipe:    &assetfile.SoundRecipe{Compose: []assetfile.SoundNode{{Sfx: "blip"}}},
	}, assetfile.Render{SampleRate: rate, Channels: channels, Bits: bits})
	if err != nil {
		t.Fatal(err)
	}
	out, err := assetfile.WriteBytes(name, env, file.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRunRefusesAnUndeliverableLampSequence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		write func(dir string)
		want  string
	}{
		{
			name:  "listed but not in the bundle",
			write: func(string) {},
			want:  "not found in bundle",
		},
		{
			name: "listed and broken",
			write: func(dir string) {
				bad := `{"arkade":1,"kind":"lights","name":"x","generator":{"tool":"t","version":"1"},"frames":[]}`
				if err := os.WriteFile(filepath.Join(dir, "assets", "win.arkadelights.json"), []byte(bad), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: "has no frames",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeCart(t)
			withLightsManifest(t, dir)
			tc.write(dir)
			out := filepath.Join(dir, "widget.rcart")
			err := run(dir, filepath.Join(dir, "widget.wasm"), out)
			if err == nil {
				t.Fatal("packing a bundle whose assets.lights cannot be delivered must fail")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", err, tc.want)
			}
			if _, statErr := os.Stat(out); statErr == nil {
				t.Error("a refused pack must not leave a bundle behind")
			}
		})
	}
}

func TestRunPacksAGoodLampSequence(t *testing.T) {
	dir := writeCart(t)
	withLightsManifest(t, dir)
	env := &assetfile.Envelope{
		Format:    assetfile.FormatVersion,
		Kind:      assetfile.KindLights,
		Name:      "win",
		Generator: assetfile.Generator{Tool: "rcartpack_test", Version: "1.0.0"},
		LightsPayload: assetfile.LightsPayload{
			Frames: []assetfile.LightFrame{
				{Lamps: assetfile.NewLightLamps(assetfile.LightMaskPtr(assetfile.MaxLightMask), nil), HoldMs: 90},
				{Lamps: assetfile.NewLightLamps(assetfile.LightMaskPtr(0), nil), HoldMs: 90},
			},
			Loop: assetfile.LoopOnce,
		},
	}
	seq, err := assetfile.WriteBytes("win.arkadelights.json", env, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "win.arkadelights.json"), seq, 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "widget.rcart")
	if err := run(dir, filepath.Join(dir, "widget.wasm"), out); err != nil {
		t.Fatalf("run: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	b, err := rcart.Load(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := b.Assets["assets/win.arkadelights.json"]
	if !ok {
		t.Fatal("the sequence was not packed")
	}
	if !bytes.Equal(got, seq) {
		t.Error("the packed sequence is not byte-identical to the file on disk")
	}
	if _, err := assetfile.ReadBytes("assets/win.arkadelights.json", got); err != nil {
		t.Errorf("the packed sequence no longer validates: %v", err)
	}
}

// No capability is added: unlike assets.hid, a lamp sequence needs none.
func withLightsManifest(t *testing.T, dir string) {
	t.Helper()
	m := strings.Replace(testManifestJSON,
		`"links": {"source": null}`,
		`"assets": {"lights": ["assets/win.arkadelights.json"]},
  "links": {"source": null}`, 1)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(m), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRunListsEveryUndeliverableAsset pins that one refusal names every
// problem, grouped by kind, rather than stopping at the first group.
func TestRunListsEveryUndeliverableAsset(t *testing.T) {
	dir := writeCart(t)
	withLightsManifest(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "assets", "blip.arkadesound.wav"), []byte("RIFF....WAVEfmt "), 0o644); err != nil {
		t.Fatal(err)
	}
	err := run(dir, filepath.Join(dir, "widget.wasm"), filepath.Join(dir, "widget.rcart"))
	if err == nil {
		t.Fatal("packing a bundle with two undeliverable assets must fail")
	}
	for _, want := range []string{"1 unreadable sound file(s)", "blip.arkadesound.wav", "1 lamp sequence problem(s)", "win.arkadelights.json"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
}

func writeProject(t *testing.T, manifest string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "game.wasm"), []byte("\x00asm"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "MANUAL.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestRunFallsBackToTheSlugWasm pins the SDK template's layout, where the
// build writes <slug>.wasm and no game.wasm exists.
func TestRunFallsBackToTheSlugWasm(t *testing.T) {
	dir := writeCart(t)
	out := filepath.Join(dir, "widget.rcart")
	if err := run(dir, "", out); err != nil {
		t.Fatalf("run: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	b, err := rcart.Load(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if string(b.Wasm) != "\x00asm-widget" {
		t.Errorf("wasm = %q, want widget.wasm's bytes", b.Wasm)
	}
}

// TestRunRejectsPreSchemaManifest pins the ratchet: the loader still
// mounts legacy manifests, but no new bundle is built from one.
func TestRunRejectsPreSchemaManifest(t *testing.T) {
	dir := writeProject(t, `{"abi":1,"slug":"demo","name":"Demo","version":"1.0.0","players":[1]}`)
	err := run(dir, "", filepath.Join(dir, "demo.rcart"))
	if err == nil || !strings.Contains(err.Error(), "schemaVersion") {
		t.Fatalf("pre-schema manifest packed; err = %v", err)
	}
}

// TestRunRejectsSchemaInvalidManifest pins that pack-time validation is
// the loader's own: a vocabulary-invalid manifest fails with the mount
// error, before anything is written.
func TestRunRejectsSchemaInvalidManifest(t *testing.T) {
	dir := writeProject(t, strings.Replace(testManifestJSON, `"all-ages"`, `"E10+"`, 1))
	out := filepath.Join(dir, "widget.rcart")
	err := run(dir, "", out)
	if err == nil || !strings.Contains(err.Error(), "audience") {
		t.Fatalf("schema-invalid manifest packed; err = %v", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Error("output file written despite validation failure")
	}
}
