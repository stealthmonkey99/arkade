// SPDX-License-Identifier: GPL-2.0-or-later

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/storage"
)

// todaysFile is settings.json as the cabinet writes it after a visit to every
// setting: the key order and spelling are the on-disk contract.
const todaysFile = `{"volume":35,"muted":false,"fanThresholdF":130,"fanMode":"on","roomOrientation":1,"seatedOrientation":3,"brightness":70,"lamps":"off"}`

func seed(t *testing.T, raw string) (*storage.Store, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "apps", "config", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	return storage.New(dir), dir
}

func TestLoadReadsTodaysFileIdentically(t *testing.T) {
	store, dir := seed(t, todaysFile)
	got, err := Load(store)
	if err != nil {
		t.Fatal(err)
	}
	want := Cabinet{
		Volume: 35, FanThresholdF: 130, FanMode: hardware.FanOn,
		RoomOrientation: display.Orientation(1), SeatedOrientation: display.Orientation(3),
		Brightness: 70, Lamps: engine.LampsOff,
	}
	if got != want {
		t.Fatalf("Load = %+v, want %+v", got, want)
	}

	if err := got.Save(store); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "apps", "config", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != todaysFile {
		t.Fatalf("Save rewrote the file as\n%s\nwant\n%s", raw, todaysFile)
	}
}

func TestLoadWithoutAFileIsTheDefault(t *testing.T) {
	got, err := Load(storage.New(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if got != Default() {
		t.Fatalf("Load = %+v, want Default %+v", got, Default())
	}
	if got.PanelBrightness(60) != 60 {
		t.Errorf("PanelBrightness without a stored level = %d, want the fallback 60", got.PanelBrightness(60))
	}
	got.Brightness = 85
	if got.PanelBrightness(60) != 85 {
		t.Errorf("PanelBrightness with a stored level = %d, want 85", got.PanelBrightness(60))
	}
}

func TestLoadMigratesTheLegacyBooleans(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantLamp engine.LampPolicy
		wantFan  hardware.FanMode
	}{
		{"nothing said", `{"volume":50}`, engine.LampsMatchScreen, hardware.FanAuto},
		{"the old booleans", `{"lampsOff":true,"fanDisabled":true}`, engine.LampsOff, hardware.FanOff},
		{"the typed keys", `{"lamps":"on","fanMode":"on"}`, engine.LampsOn, hardware.FanOn},
		{"typed outranks boolean", `{"lamps":"match","lampsOff":true,"fanMode":"auto","fanDisabled":true}`, engine.LampsMatchScreen, hardware.FanAuto},
		{"unreadable falls back", `{"lamps":"sometimes","fanMode":"sometimes"}`, engine.LampsMatchScreen, hardware.FanAuto},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store, _ := seed(t, c.raw)
			got, err := Load(store)
			if err != nil {
				t.Fatal(err)
			}
			if got.Lamps != c.wantLamp || got.FanMode != c.wantFan {
				t.Errorf("lamps %q fan %q, want %q %q", got.Lamps, got.FanMode, c.wantLamp, c.wantFan)
			}
		})
	}
}

func TestSaveDropsTheLegacyKeysAndKeepsTheRest(t *testing.T) {
	store, _ := seed(t, `{"volume":40,"fanThresholdF":120,"fanDisabled":true,"lampsOff":true}`)
	c, err := Load(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Save(store); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := store.Read(Owner, "", &raw); err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []string{"fanDisabled", "lampsOff"} {
		if _, there := raw[legacy]; there {
			t.Errorf("%s survived a save", legacy)
		}
	}
	if raw["fanMode"] != "off" || raw["lamps"] != "off" || raw["volume"] != 40.0 {
		t.Errorf("saved %v, want the migrated modes beside the untouched volume", raw)
	}
	if _, there := raw["brightness"]; there {
		t.Error("a brightness nobody set was written, which would override --brightness")
	}
}

func TestLoadRepairsOutOfRangeValues(t *testing.T) {
	store, _ := seed(t, `{"volume":140,"fanThresholdF":300,"roomOrientation":6}`)
	got, err := Load(store)
	if err != nil {
		t.Fatal(err)
	}
	if got.Volume != 100 || got.FanThresholdF != FanThresholdMax || got.RoomOrientation != display.Orientation(2) {
		t.Errorf("Load = %+v, want volume 100, threshold %d, room orientation 2", got, FanThresholdMax)
	}
}

func TestClampFanThreshold(t *testing.T) {
	cases := map[int]int{
		DefaultFanThresholdF:      DefaultFanThresholdF,
		FanThresholdMin - 5:       FanThresholdMin,
		FanThresholdMax + 5:       FanThresholdMax,
		0:                         FanThresholdMin,
		FanThresholdMin:           FanThresholdMin,
		FanThresholdMax:           FanThresholdMax,
		FanThresholdMax + 100_000: FanThresholdMax,
	}
	for in, want := range cases {
		if got := ClampFanThreshold(in); got != want {
			t.Errorf("ClampFanThreshold(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestLoadReportsACorruptFileAndStillAnswers(t *testing.T) {
	store, _ := seed(t, `{"volume":`)
	got, err := Load(store)
	if err == nil {
		t.Fatal("a corrupt file loaded silently")
	}
	if got != Default() {
		t.Errorf("a corrupt file loaded as %+v, want Default", got)
	}
}
