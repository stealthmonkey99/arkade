// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/tools/manifestgen/gen"
)

const goodManifest = `{
  "schemaVersion": 1,
  "abi": 1,
  "slug": "fixture",
  "title": "Fixture Game",
  "publisher": "Årkade",
  "author": "Test",
  "version": "1.2.3",
  "description": "A fixture cartridge for preflight's own tests.",
  "tags": ["arcade"],
  "players": ["1p"],
  "audience": "all-ages",
  "capabilities": [],
  "visibility": "public",
  "icon": "assets/icon.png",
  "manual": "MANUAL.md",
  "links": {"source": null}
}`

const micModelBody = `{"arkade":1,"kind":"mic","name":"commands",` +
	`"generator":{"tool":"micmodel","version":"1.0.0"},` +
	`"features":{"sampleRateHz":14700,"frameLen":512,"hopLen":147,"melBands":26,"coeffs":2,"minHz":100,"maxHz":6000},` +
	`"segment":{"minMs":150,"maxMs":1200,"hangoverMs":200,"snr":4.0,"minRms":0.02},` +
	`"classes":[{"name":"come","cue":0,"threshold":0.42,"templates":[[[1.0,2.0],[3.0,4.0]]]}]}`

const goodManual = `<img src="manual-icon.png" width="128">

# Fixture Game

How to play the fixture.

*Manual for version 1.2.3*
`

const goodSource = `package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

type fixtureGame struct{}

func (fixtureGame) Manifest() []byte { return manifestBytes }

func main() {}
`

// goodGenerated is manifest_gen.go as manifestgen writes it for goodManifest.
func goodGenerated(t *testing.T) string {
	t.Helper()
	src, err := gen.Source([]byte(goodManifest))
	if err != nil {
		t.Fatal(err)
	}
	return string(src)
}

// manifestFixture writes a valid cartridge. An override with an empty value
// deletes that file.
func manifestFixture(t *testing.T, overrides map[string]string) *Cart {
	t.Helper()
	c := tempCart(t, "fixture")
	files := map[string]string{
		"manifest.json":   goodManifest,
		"MANUAL.md":       goodManual,
		"main.go":         goodSource,
		"manifest_gen.go": goodGenerated(t),
		"fixture.wasm":    "\x00asm\x01\x00\x00\x00",
	}
	for name, body := range overrides {
		files[name] = body
	}
	for name, body := range files {
		if body == "" {
			continue
		}
		path := filepath.Join(c.Dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writePNG(t, filepath.Join(c.Dir, "assets", "icon.png"), 8, 8)
	writePNG(t, filepath.Join(c.Dir, "manual-icon.png"), 16, 16)
	return c
}

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 90, G: 140, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestManifestPassesCompleteCartridge(t *testing.T) {
	res := run(t, "manifest", manifestFixture(t, nil))
	if res.Status != StatusPass {
		t.Errorf("status %s, want pass\n%s", res.Status, findings(res))
	}
}

func TestManifestDelegatesToTheRealSchema(t *testing.T) {
	broken := strings.Replace(goodManifest, `"audience": "all-ages"`, `"audience": "everyone"`, 1)
	res := run(t, "manifest", manifestFixture(t, map[string]string{"manifest.json": broken}))
	if res.Status != StatusFail {
		t.Fatalf("status %s, want fail\n%s", res.Status, findings(res))
	}
	wantContains(t, findings(res), "not in the vocabulary", "rcart's own error text comes through verbatim")
}

func TestManifestCatchesSlugDirectoryMismatch(t *testing.T) {
	renamed := strings.Replace(goodManifest, `"slug": "fixture"`, `"slug": "elsewhere"`, 1)
	res := run(t, "manifest", manifestFixture(t, map[string]string{"manifest.json": renamed}))
	wantContains(t, findings(res), "does not match the directory name", "a renamed slug orphans scores and MRU")
}

func TestManifestCatchesStaleManualFooter(t *testing.T) {
	stale := strings.Replace(goodManual, "1.2.3", "1.1.0", 1)
	res := run(t, "manifest", manifestFixture(t, map[string]string{"MANUAL.md": stale}))
	if res.Status != StatusFail {
		t.Fatalf("status %s, want fail\n%s", res.Status, findings(res))
	}
	wantContains(t, findings(res), "footer does not quote manifest version", "manual footer rule")
}

func TestManifestCatchesMissingManual(t *testing.T) {
	res := run(t, "manifest", manifestFixture(t, map[string]string{"MANUAL.md": ""}))
	wantContains(t, findings(res), "no MANUAL.md", "every cartridge ships a player manual")
}

func TestManifestCatchesMissingWasm(t *testing.T) {
	res := run(t, "manifest", manifestFixture(t, map[string]string{"fixture.wasm": ""}))
	wantContains(t, findings(res), "no built fixture.wasm", "the built cartridge is committed")
}

func TestManifestCatchesAStaleGeneratedManifest(t *testing.T) {
	bumped := strings.Replace(goodManifest, `"version": "1.2.3"`, `"version": "1.3.0"`, 1)
	manual := strings.Replace(goodManual, "1.2.3", "1.3.0", 1)
	res := run(t, "manifest", manifestFixture(t, map[string]string{"manifest.json": bumped, "MANUAL.md": manual}))
	if res.Status != StatusFail {
		t.Fatalf("status %s, want fail\n%s", res.Status, findings(res))
	}
	wantContains(t, findings(res), "manifest_gen.go is stale against manifest.json", "the wasm would report the old version")
	wantContains(t, findings(res), "sdk/tools/manifestgen", "the message names the mechanical fix")
}

func TestManifestCatchesAMissingGeneratedManifest(t *testing.T) {
	res := run(t, "manifest", manifestFixture(t, map[string]string{"manifest_gen.go": ""}))
	if res.Status != StatusFail {
		t.Fatalf("status %s, want fail\n%s", res.Status, findings(res))
	}
	wantContains(t, findings(res), "no manifest_gen.go", "a Go cartridge without the generated file was never built by its Makefile")
}

func TestManifestSkipsTheGeneratedFileForACCartridge(t *testing.T) {
	res := run(t, "manifest", manifestFixture(t, map[string]string{"main.go": "", "manifest_gen.go": ""}))
	if res.Status != StatusPass {
		t.Errorf("status %s, want pass — no TinyGo sources, nothing to generate\n%s", res.Status, findings(res))
	}
}

func TestBundlePacksAndLoads(t *testing.T) {
	res := run(t, "bundle", manifestFixture(t, nil))
	if res.Status != StatusPass {
		t.Errorf("status %s, want pass\n%s", res.Status, findings(res))
	}
	wantContains(t, res.Summary, "packs to a loadable .rcart", "the summary states what was proved")
}

func TestBundleFailsOnAnInvalidManifest(t *testing.T) {
	broken := strings.Replace(goodManifest, `"visibility": "public"`, `"visibility": "secret"`, 1)
	res := run(t, "bundle", manifestFixture(t, map[string]string{"manifest.json": broken}))
	if res.Status != StatusFail {
		t.Errorf("status %s, want fail — rcart.Load rejects this at mount time\n%s", res.Status, findings(res))
	}
}

func TestBundleSkipsWithoutWasm(t *testing.T) {
	res := run(t, "bundle", manifestFixture(t, map[string]string{"fixture.wasm": ""}))
	if res.Status != StatusSkip {
		t.Errorf("status %s, want skip — the manifest check already reported the missing wasm", res.Status)
	}
}

func TestManifestCatchesUndeliverableBundledAssets(t *testing.T) {
	withAssets := func(block string) string {
		return strings.Replace(goodManifest, `"icon": "assets/icon.png"`,
			block+"\n  \"icon\": \"assets/icon.png\"", 1)
	}
	withMic := func(block string) string {
		return strings.Replace(withAssets(block), `"capabilities": []`, `"capabilities": ["mic"]`, 1)
	}
	tests := []struct {
		name     string
		manifest string
		files    map[string]string
		want     string
	}{
		{
			name:     "a lamp sequence that is not there",
			manifest: withAssets(`"assets": {"lights": ["assets/lights/win.arkadelights.json"]},`),
			want:     "is not there",
		},
		{
			name:     "a lamp sequence that does not validate",
			manifest: withAssets(`"assets": {"lights": ["assets/lights/win.arkadelights.json"]},`),
			files: map[string]string{
				"assets/lights/win.arkadelights.json": `{"arkade":1,"kind":"lights","name":"x","generator":{"tool":"t","version":"1"},"frames":[]}`,
			},
			want: "has no frames",
		},
		{
			name:     "a lamp sequence under the wrong name",
			manifest: withAssets(`"assets": {"lights": ["assets/lights/win.json"]},`),
			files:    map[string]string{"assets/lights/win.json": `{}`},
			want:     "not a <name>.arkadelights.json file",
		},
		{
			name:     "a HID profile without the capability",
			manifest: withAssets(`"assets": {"hid": ["assets/pad.arkadehid.json"]},`),
			files: map[string]string{
				"assets/pad.arkadehid.json": `{"arkade":1,"kind":"hid","name":"pad","generator":{"tool":"t","version":"1"},` +
					`"channels":[{"name":"pad.up","kind":"button","report":{"byte":3,"bit":4}}]}`,
			},
			want: `does not declare the "hid" capability`,
		},
		{
			name:     "a sound model without the capability",
			manifest: withAssets(`"assets": {"mic": ["assets/commands.arkademic.json"]},`),
			files: map[string]string{
				"assets/commands.arkademic.json": micModelBody,
			},
			want: `does not declare the "mic" capability`,
		},
		{
			name:     "a sound model that does not validate",
			manifest: withMic(`"assets": {"mic": ["assets/commands.arkademic.json"]},`),
			files: map[string]string{
				"assets/commands.arkademic.json": `{"arkade":1,"kind":"mic","name":"x","generator":{"tool":"t","version":"1"}}`,
			},
			want: "no features block",
		},
		{
			name:     "a sound model under the wrong name",
			manifest: withMic(`"assets": {"mic": ["assets/commands.json"]},`),
			files:    map[string]string{"assets/commands.json": `{}`},
			want:     "not a <name>.arkademic.json file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrides := map[string]string{"manifest.json": tt.manifest}
			for name, body := range tt.files {
				overrides[name] = body
			}
			res := run(t, "manifest", manifestFixture(t, overrides))
			if res.Status != StatusFail {
				t.Fatalf("status %s, want fail\n%s", res.Status, findings(res))
			}
			if !strings.Contains(findings(res), tt.want) {
				t.Errorf("findings do not mention %q:\n%s", tt.want, findings(res))
			}
		})
	}
}

func TestManifestAcceptsADeliverableLampSequence(t *testing.T) {
	seq := `{
  "arkade": 1,
  "kind": "lights",
  "name": "win",
  "generator": {"tool": "t", "version": "1.0.0"},
  "frames": [{"lamps": {"p1": 63}, "holdMs": 90}, {"lamps": {"p1": 0}, "holdMs": 90}],
  "loop": "once"
}`
	manifest := strings.Replace(goodManifest, `"icon": "assets/icon.png"`,
		"\"assets\": {\"lights\": [\"assets/lights/win.arkadelights.json\"]},\n  \"icon\": \"assets/icon.png\"", 1)
	res := run(t, "manifest", manifestFixture(t, map[string]string{
		"manifest.json":                       manifest,
		"assets/lights/win.arkadelights.json": seq,
	}))
	if res.Status != StatusPass {
		t.Errorf("status %s, want pass\n%s", res.Status, findings(res))
	}
}
