// SPDX-License-Identifier: GPL-2.0-or-later

// Package config owns the cabinet's settings.json: the schema, the defaults,
// the migrations from older keys, and the one way to load and save it. The
// settings app, the shell's SYSTEM shelf and cmd/arkade all read and write
// the file through Cabinet.
package config

import (
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/storage"
)

// Owner is the storage owner the file lives under: the settings app's slug.
const Owner = "apps/config"

const (
	DefaultVolume        = 50
	DefaultFanThresholdF = 120

	// The fan threshold is clamped to a range the Pi's hysteresis can act on.
	FanThresholdMin = 80
	FanThresholdMax = 180
)

// Cabinet is the operator's standing settings.
type Cabinet struct {
	Volume            int
	Muted             bool
	FanThresholdF     int
	FanMode           hardware.FanMode
	RoomOrientation   display.Orientation
	SeatedOrientation display.Orientation
	// Brightness is the panel percent, or 0 while the operator has never set
	// one; PanelBrightness resolves that against the command line.
	Brightness int
	Lamps      engine.LampPolicy
}

// Default is a cabinet nobody has configured yet.
func Default() Cabinet {
	return Cabinet{
		Volume:        DefaultVolume,
		FanThresholdF: DefaultFanThresholdF,
		FanMode:       hardware.FanAuto,
		Lamps:         engine.LampsMatchScreen,
	}
}

// file is the on-disk shape, plus the two legacy keys Load migrates.
type file struct {
	Volume            *int   `json:"volume,omitempty"`
	Muted             bool   `json:"muted"`
	FanThresholdF     *int   `json:"fanThresholdF,omitempty"`
	FanMode           string `json:"fanMode,omitempty"`
	RoomOrientation   int    `json:"roomOrientation"`
	SeatedOrientation int    `json:"seatedOrientation"`
	Brightness        *int   `json:"brightness,omitempty"`
	Lamps             string `json:"lamps,omitempty"`
	LampsOff          bool   `json:"lampsOff,omitempty"`
	FanDisabled       bool   `json:"fanDisabled,omitempty"`
}

// Load reads the file, filling in defaults for anything it does not say and
// migrating the legacy lampsOff and fanDisabled booleans. A missing file is
// Default with no error; an unreadable one is Default with the error.
func Load(store *storage.Store) (Cabinet, error) {
	var f file
	err := store.Read(Owner, "", &f)
	c := Default()
	if err != nil {
		return c, err
	}
	if f.Volume != nil {
		c.Volume = min(100, max(0, *f.Volume))
	}
	c.Muted = f.Muted
	if f.FanThresholdF != nil && *f.FanThresholdF > 0 {
		c.FanThresholdF = ClampFanThreshold(*f.FanThresholdF)
	}
	if mode, ok := hardware.ParseFanMode(f.FanMode); ok {
		c.FanMode = mode
	} else if f.FanDisabled {
		c.FanMode = hardware.FanOff
	}
	c.RoomOrientation = display.Orientation(f.RoomOrientation & 3)
	c.SeatedOrientation = display.Orientation(f.SeatedOrientation & 3)
	if f.Brightness != nil {
		c.Brightness = *f.Brightness
	}
	if p, ok := engine.ParseLampPolicy(f.Lamps); ok {
		c.Lamps = p
	} else if f.LampsOff {
		c.Lamps = engine.LampsOff
	}
	return c, nil
}

// Save writes the whole file, dropping the legacy keys.
func (c Cabinet) Save(store *storage.Store) error {
	f := file{
		Volume:            &c.Volume,
		Muted:             c.Muted,
		FanThresholdF:     &c.FanThresholdF,
		FanMode:           string(c.FanMode),
		RoomOrientation:   int(c.RoomOrientation),
		SeatedOrientation: int(c.SeatedOrientation),
		Lamps:             string(c.Lamps),
	}
	if c.Brightness > 0 {
		f.Brightness = &c.Brightness
	}
	return store.Write(Owner, "", f)
}

// PanelBrightness is the stored panel percent, or fallback (the command
// line's) while none has been stored.
func (c Cabinet) PanelBrightness(fallback int) int {
	if c.Brightness <= 0 {
		return fallback
	}
	return c.Brightness
}

// ClampFanThreshold bounds a threshold to FanThresholdMin..FanThresholdMax.
func ClampFanThreshold(degF int) int {
	return min(FanThresholdMax, max(FanThresholdMin, degF))
}
