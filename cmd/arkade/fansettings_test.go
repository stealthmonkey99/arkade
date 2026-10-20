// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"reflect"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/config"
	"github.com/stealthmonkey99/arkade/internal/hardware/pi"
	"github.com/stealthmonkey99/arkade/internal/storage"
)

// The cabinet's default threshold and the driver's own fallback must agree,
// or a fresh cabinet would cool differently before and after its first save.
func TestCabinetFanDefaultMatchesTheDriver(t *testing.T) {
	if float64(config.DefaultFanThresholdF) != pi.DefaultFanThresholdF {
		t.Fatalf("config default %d°F, pi default %v°F", config.DefaultFanThresholdF, pi.DefaultFanThresholdF)
	}
}

// TestLoadHardwareSettingsFanSection: the fan block merges on its own, with
// or without a pca9685 block beside it.
func TestLoadHardwareSettingsFanSection(t *testing.T) {
	dir := t.TempDir()
	if def := pi.LoadConfig(dir).Fan; !reflect.DeepEqual(def.ExpanderPins, []int{7}) || len(def.PCAChannels) != 0 {
		t.Fatalf("no file = %+v, want the expander's pin 7 alone", def)
	}

	raw := map[string]any{"fan": map[string]any{"pcaChannels": []int{7}, "invert": true}}
	if err := storage.New(dir).Write("hardware", "", raw); err != nil {
		t.Fatalf("seeding settings: %v", err)
	}
	got := pi.LoadConfig(dir)
	if !reflect.DeepEqual(got.Fan.PCAChannels, []int{7}) || !got.Fan.Invert {
		t.Errorf("fan = %+v, want channel 7 inverted", got.Fan)
	}
	if !reflect.DeepEqual(got.Fan.ExpanderPins, []int{7}) {
		t.Errorf("an omitted expanderPins should keep the default, got %v", got.Fan.ExpanderPins)
	}
	if got.PCA9685.Addr != 0x40 {
		t.Errorf("a fan-only file should leave the pca9685 defaults alone, got addr %#x", got.PCA9685.Addr)
	}
}
