// SPDX-License-Identifier: GPL-2.0-or-later

package rcart

import (
	"bytes"
	"strings"
	"testing"
)

const micManifest = `{"abi":1,"slug":"demo","name":"Demo",` +
	`"capabilities":["mic"],` +
	`"assets":{"mic":["assets/commands.arkademic.json"]}}`

const micManifestNoCapability = `{"abi":1,"slug":"demo","name":"Demo",` +
	`"assets":{"mic":["assets/commands.arkademic.json"]}}`

var goodMicFile = []byte(`{"arkade":1,"kind":"mic","name":"commands",` +
	`"generator":{"tool":"micmodel","version":"1.0.0"},` +
	`"features":{"sampleRateHz":14700,"frameLen":512,"hopLen":147,"melBands":26,"coeffs":2,"minHz":100,"maxHz":6000},` +
	`"segment":{"minMs":150,"maxMs":1200,"hangoverMs":200,"snr":4.0,"minRms":0.02},` +
	`"classes":[{"name":"come","cue":0,"threshold":0.42,"templates":[[[1.0,2.0],[3.0,4.0]]]}]}`)

func TestLoadReadsTheManifestMicList(t *testing.T) {
	b, err := Load(bytes.NewReader(buildTar(t, false,
		hidEntries(micManifest, goodMicFile, "assets/commands.arkademic.json"))))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := "assets/commands.arkademic.json"
	if got := b.Manifest.MicAssets(); len(got) != 1 || got[0] != want {
		t.Fatalf("MicAssets() = %v, want [%s]", got, want)
	}
	if !b.Manifest.HasCapability(CapabilityMic) {
		t.Error(`the manifest declares "mic" but HasCapability says otherwise`)
	}
	if len(b.Warnings) != 0 {
		t.Fatalf("a valid model should warn about nothing, got %v", b.Warnings)
	}
}

func TestLoadWarnsAboutMicModelsTheBundleCannotDeliver(t *testing.T) {
	broken := []byte(`{"arkade":1,"kind":"mic","name":"x",` +
		`"generator":{"tool":"t","version":"1"}}`)
	for _, tc := range []struct {
		name     string
		manifest string
		file     []byte
		path     string
		want     string
	}{
		{"missing from the bundle", micManifest, nil, "", "not found in bundle"},
		{"wrong naming rule", micManifest, goodMicFile, "assets/commands.json", "not found in bundle"},
		{"invalid envelope", micManifest, broken, "assets/commands.arkademic.json", "no features block"},
		{"capability not declared", micManifestNoCapability, goodMicFile, "assets/commands.arkademic.json", `does not declare the "mic" capability`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := Load(bytes.NewReader(buildTar(t, false, hidEntries(tc.manifest, tc.file, tc.path))))
			if err != nil {
				t.Fatalf("Load: %v — a broken model must never fail the whole bundle", err)
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
