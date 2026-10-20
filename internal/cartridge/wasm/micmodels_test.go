// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"testing"

	"github.com/stealthmonkey99/arkade/assetfile"
)

// micModelFile is the smallest model assetfile accepts.
const micModelFile = `{
  "arkade": 1, "kind": "mic", "name": "commands",
  "generator": { "tool": "micmodel", "version": "1.0.0" },
  "features": { "sampleRateHz": 14700, "frameLen": 512, "hopLen": 147,
                "melBands": 26, "coeffs": 2, "minHz": 100, "maxHz": 6000 },
  "segment": { "minMs": 150, "maxMs": 1200, "hangoverMs": 200, "snr": 4.0, "minRms": 0.02 },
  "classes": [
    { "name": "come", "cue": 0, "threshold": 0.42,
      "templates": [ [[1.0, 2.0], [3.0, 4.0]] ] },
    { "name": "sit", "cue": 1, "threshold": 0.5,
      "templates": [ [[-1.0, 0.0], [-2.0, 0.5]] ] }
  ]
}`

// stubBundleHost is deliberately not a MicModelSink.
type stubBundleHost struct {
	*stubChannelHost
	assets map[string][]byte
}

func newStubBundleHost() *stubBundleHost {
	return &stubBundleHost{stubChannelHost: newStubChannelHost(), assets: map[string][]byte{}}
}

func (h *stubBundleHost) BundleAsset(p string) ([]byte, bool) {
	data, ok := h.assets[p]
	return data, ok
}

type stubMicHost struct {
	*stubBundleHost
	models []*assetfile.Envelope
	calls  int
}

func newStubMicHost() *stubMicHost { return &stubMicHost{stubBundleHost: newStubBundleHost()} }

func (h *stubMicHost) SetSessionMicModels(models []*assetfile.Envelope) {
	h.models = models
	h.calls++
}

func TestMicBundleInstalledAtMountAndDroppedAtTeardown(t *testing.T) {
	host := newStubMicHost()
	host.assets["assets/commands.arkademic.json"] = []byte(micModelFile)
	reg := newMicRegistry(host)

	reg.loadBundle([]string{"assets/commands.arkademic.json"})
	if len(host.models) != 1 {
		t.Fatalf("the bundle's model was not installed at mount (%d models)", len(host.models))
	}
	env := host.models[0]
	if env.Name != "commands" || len(env.Classes) != 2 || env.Features.Coeffs != 2 {
		t.Fatalf("installed model = %+v, want the two-class commands model", env)
	}
	if got := env.Classes[1].Cue; got != 1 {
		t.Errorf("class %q fires cue %d, want 1", env.Classes[1].Name, got)
	}

	reg.dropBundle()
	if host.models != nil {
		t.Errorf("the bundle's models outlived the cartridge (%d still installed)", len(host.models))
	}
}

func TestMicBundleFailuresAreWarningsAndSkips(t *testing.T) {
	host := newStubMicHost()
	host.assets["assets/good.arkademic.json"] = []byte(micModelFile)
	host.assets["assets/wrongname.json"] = []byte(micModelFile)
	host.assets["assets/profile.arkadehid.json"] = []byte(micModelFile)
	host.assets["assets/broken.arkademic.json"] = []byte(
		`{"arkade":1,"kind":"mic","name":"x","generator":{"tool":"t","version":"1"}}`)
	reg := newMicRegistry(host)

	reg.loadBundle([]string{
		"assets/missing.arkademic.json",
		"assets/wrongname.json",
		"assets/profile.arkadehid.json",
		"assets/broken.arkademic.json",
		"assets/good.arkademic.json",
	})

	if len(host.models) != 1 {
		t.Fatalf("installed %d models, want only the good one", len(host.models))
	}
	for _, want := range []string{
		"no such asset",
		"not a <name>.arkademic.json file",
		"no features block",
	} {
		if !host.logged(want) {
			t.Errorf("no warning mentioning %q; logs: %v", want, host.logs)
		}
	}
}

func TestMicBundleCapIsFourModels(t *testing.T) {
	host := newStubMicHost()
	paths := make([]string, 0, maxBundleMicModels+2)
	for i := 0; i < maxBundleMicModels+2; i++ {
		p := "assets/m" + string(rune('a'+i)) + ".arkademic.json"
		host.assets[p] = []byte(micModelFile)
		paths = append(paths, p)
	}
	reg := newMicRegistry(host)
	reg.loadBundle(paths)

	if len(host.models) != maxBundleMicModels {
		t.Fatalf("installed %d models, want the cap to stop at %d", len(host.models), maxBundleMicModels)
	}
	if !host.logged("already holding 4 models") {
		t.Errorf("the cap bit without a warning; logs: %v", host.logs)
	}
}

func TestMicBundleIgnoresAHostThatIsNotASink(t *testing.T) {
	host := newStubBundleHost()
	host.assets["assets/commands.arkademic.json"] = []byte(micModelFile)
	reg := newMicRegistry(host)

	reg.loadBundle([]string{"assets/commands.arkademic.json"})
	if len(reg.models) != 1 {
		t.Fatalf("the registry holds %d models, want 1", len(reg.models))
	}
	if len(host.logs) != 0 {
		t.Errorf("a host with no microphone was warned at: %v", host.logs)
	}
	reg.dropBundle()
}

// TestMicCueOrdinalsAreAppended pins the numbering sdk/cartlib copies, which
// no compiler checks across the module boundary.
func TestMicCueOrdinalsAreAppended(t *testing.T) {
	for ord, want := range map[uint8]string{
		28: "mic.whistle", 29: "mic.clap", 30: "mic.blow",
		31: "mic.cue0", 34: "mic.cue3", 38: "mic.cue7",
	} {
		got, ok := OrdinalToKey(ord)
		if !ok || string(got) != want {
			t.Errorf("ordinal %d is %q (%v), want %q — sdk/cartlib/input.go hardcodes it", ord, got, ok, want)
		}
	}
	if _, ok := OrdinalToKey(39); ok {
		t.Error("the key table grew past mic.cue7; sdk/cartlib/input.go must grow with it")
	}
}
