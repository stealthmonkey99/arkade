// SPDX-License-Identifier: GPL-2.0-or-later

package rcart

import (
	"bytes"
	"strings"
	"testing"
)

const hidManifest = `{"abi":1,"slug":"demo","name":"Demo",` +
	`"capabilities":["hid"],` +
	`"assets":{"hid":["assets/dancepad.arkadehid.json"]}}`

const hidManifestNoCapability = `{"abi":1,"slug":"demo","name":"Demo",` +
	`"assets":{"hid":["assets/dancepad.arkadehid.json"]}}`

var goodHIDFile = []byte(`{"arkade":1,"kind":"hid","name":"dancepad",` +
	`"generator":{"tool":"editor","version":"1.0.0"},` +
	`"device":{"vendor":4660,"product":22136,"name":"Dance pad"},` +
	`"channels":[{"name":"pad.up","kind":"button","report":{"byte":0}}],` +
	`"standard":{"pad.up":"joy.north"}}`)

func hidEntries(manifest string, file []byte, path string) []tarEntry {
	entries := []tarEntry{
		{name: ManifestName, data: []byte(manifest)},
		{name: WasmName, data: []byte("\x00asm-fake-wasm-bytes")},
	}
	if file != nil {
		entries = append(entries, tarEntry{name: path, data: file})
	}
	return entries
}

func TestLoadReadsTheManifestHIDList(t *testing.T) {
	b, err := Load(bytes.NewReader(buildTar(t, false,
		hidEntries(hidManifest, goodHIDFile, "assets/dancepad.arkadehid.json"))))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := "assets/dancepad.arkadehid.json"
	if got := b.Manifest.HIDAssets(); len(got) != 1 || got[0] != want {
		t.Fatalf("HIDAssets() = %v, want [%s]", got, want)
	}
	if !b.Manifest.HasCapability(CapabilityHID) {
		t.Error(`the manifest declares "hid" but HasCapability says otherwise`)
	}
	if len(b.Warnings) != 0 {
		t.Fatalf("a valid profile should warn about nothing, got %v", b.Warnings)
	}
}

func TestLoadWarnsAboutHIDProfilesTheBundleCannotDeliver(t *testing.T) {
	broken := []byte(`{"arkade":1,"kind":"hid","name":"x",` +
		`"generator":{"tool":"t","version":"1"},"channels":[]}`)
	for _, tc := range []struct {
		name     string
		manifest string
		file     []byte
		path     string
		want     string
	}{
		{"missing from the bundle", hidManifest, nil, "", "not found in bundle"},
		{"wrong naming rule", hidManifest, goodHIDFile, "assets/dancepad.json", "not found in bundle"},
		{"invalid envelope", hidManifest, broken, "assets/dancepad.arkadehid.json", "no channels"},
		{"capability not declared", hidManifestNoCapability, goodHIDFile, "assets/dancepad.arkadehid.json", `does not declare the "hid" capability`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := Load(bytes.NewReader(buildTar(t, false, hidEntries(tc.manifest, tc.file, tc.path))))
			if err != nil {
				t.Fatalf("Load: %v — a broken profile must never fail the whole bundle", err)
			}
			var found bool
			for _, w := range b.Warnings {
				if strings.Contains(w.Message, tc.want) {
					found = true
				}
			}
			if !found {
				t.Errorf("warnings %v, want one mentioning %q", b.Warnings, tc.want)
			}
		})
	}
}

func TestManifestWithNoHIDListIsUntouched(t *testing.T) {
	b, err := Load(bytes.NewReader(buildTar(t, false, []tarEntry{
		{name: ManifestName, data: []byte(`{"abi":1,"slug":"demo","name":"Demo"}`)},
		{name: WasmName, data: []byte("\x00asm-fake-wasm-bytes")},
	})))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := b.Manifest.HIDAssets(); got != nil {
		t.Errorf("HIDAssets() = %v, want nil", got)
	}
	if len(b.Warnings) != 0 {
		t.Errorf("warnings %v, want none", b.Warnings)
	}
}

func TestMountAssetsDropsAListWhoseCapabilityIsMissing(t *testing.T) {
	m := Manifest{
		Capabilities: []string{CapabilityMic},
		Assets: &AssetRefs{
			Glyphs: []string{"assets/heart.arkadeglyph.json"},
			HID:    []string{"assets/dancepad.arkadehid.json"},
			Mic:    []string{"assets/words.arkademic.json"},
			Lights: []string{"assets/win.arkadelights.json"},
		},
	}
	got, warnings := m.MountAssets()
	if got.HID != nil {
		t.Errorf("HID = %v, want it dropped without the %q capability", got.HID, CapabilityHID)
	}
	if len(got.Glyphs) != 1 || len(got.Mic) != 1 || len(got.Lights) != 1 {
		t.Errorf("MountAssets() = %+v, want glyphs, mic and lights kept", got)
	}
	want := `assets.hid ignored — the manifest does not declare the "hid" capability`
	if len(warnings) != 1 || warnings[0] != want {
		t.Errorf("warnings = %q, want [%q]", warnings, want)
	}
}

func TestLoadWarnsWhenAListExceedsTheMountCap(t *testing.T) {
	paths := `"assets/a.arkademic.json","assets/b.arkademic.json","assets/c.arkademic.json","assets/d.arkademic.json","assets/e.arkademic.json"`
	manifest := `{"abi":1,"slug":"demo","name":"Demo","capabilities":["mic"],"assets":{"mic":[` + paths + `]}}`
	b, err := Load(bytes.NewReader(buildTar(t, false, hidEntries(manifest, nil, ""))))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := "assets.mic lists 5 sound model(s); a host mounts at most 4"
	for _, w := range b.Warnings {
		if w.Message == want && w.Category == CategoryMic && w.Severity == Undeliverable {
			return
		}
	}
	t.Errorf("warnings %q, want %q", b.Warnings, want)
}
