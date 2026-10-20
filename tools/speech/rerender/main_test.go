// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/tools/sfxlib"
	"github.com/stealthmonkey99/arkade/tools/speech"
)

func writeSound(t *testing.T, path string, recipe *assetfile.SoundRecipe) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := assetfile.SoundMeta{
		Name:      "line",
		Generator: assetfile.Generator{Tool: "test", Version: "1.0.0"},
		Recipe:    recipe,
	}
	if err := sfxlib.WriteAssetFile(path, []float64{0, 0.5, -0.5, 0}, 22050, meta); err != nil {
		t.Fatal(err)
	}
}

func speechNode(t *testing.T, text string) assetfile.SoundNode {
	t.Helper()
	raw, err := speech.Params{Text: text, Speed: 1.3}.MarshalParams("line")
	if err != nil {
		t.Fatal(err)
	}
	return assetfile.SoundNode{Sfx: "line", Params: raw}
}

func TestFindSpeechKeepsTheBlend(t *testing.T) {
	root := t.TempDir()
	line := speech.Params{Text: "fifteen two", Voice: "bm_george", BlendVoice: "bm_fable", BlendWeight: 0.4, Speed: 1.3}
	raw, err := line.MarshalParams("line")
	if err != nil {
		t.Fatal(err)
	}
	writeSound(t, filepath.Join(root, "blend.arkadesound.wav"),
		&assetfile.SoundRecipe{Compose: []assetfile.SoundNode{{Sfx: "line", Params: raw}}})
	got, err := findSpeech(root)
	if err != nil || len(got) != 1 || got[0].params != line {
		t.Fatalf("found %+v, %v; want the blended line %+v", got, err, line)
	}
}

func TestFindSpeechByEnvelopeNotName(t *testing.T) {
	root := t.TempDir()
	writeSound(t, filepath.Join(root, "cart", "assets", "count.arkadesound.wav"),
		&assetfile.SoundRecipe{Compose: []assetfile.SoundNode{speechNode(t, "fifteen two")}})
	writeSound(t, filepath.Join(root, "cart", "assets", "blip.arkadesound.wav"),
		&assetfile.SoundRecipe{Compose: []assetfile.SoundNode{{Sfx: "blip", Params: json.RawMessage(`{"kind":"patch"}`)}}})
	writeSound(t, filepath.Join(root, "cart", "assets", "mixed.arkadesound.wav"),
		&assetfile.SoundRecipe{Compose: []assetfile.SoundNode{speechNode(t, "a"), speechNode(t, "b")}, Mix: &assetfile.SoundMix{Peak: 0.9, JoinMs: 6}})
	writeSound(t, filepath.Join(root, "cart", "assets", "sampled.arkadesound.wav"), nil)
	writeSound(t, filepath.Join(root, "cart", "node_modules", "x", "hidden.arkadesound.wav"),
		&assetfile.SoundRecipe{Compose: []assetfile.SoundNode{speechNode(t, "skip me")}})
	if err := os.WriteFile(filepath.Join(root, "cart", "assets", "bare.wav"), sfxlib.Encode([]float64{0}, 22050), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := findSpeech(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("found %d speech assets, want 1: %+v", len(got), got)
	}
	a := got[0]
	if filepath.Base(a.path) != "count.arkadesound.wav" || a.rate != 22050 || a.params.Text != "fifteen two" || a.params.Speed != 1.3 {
		t.Fatalf("found %+v", a)
	}
}
