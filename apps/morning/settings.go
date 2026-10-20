// SPDX-License-Identifier: GPL-2.0-or-later

package morning

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/stealthmonkey99/arkade/apps/screensaver"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/storage"
)

const brightStep = 5

var _ screensaver.Settings = (*settings)(nil)

func (s *settings) Editor() screensaver.Editor {
	return screensaver.Editor{
		Schedule: &s.Schedule,
		Fields: []screensaver.Field{
			{
				Label: "BRIGHT",
				Value: func() string { return strconv.Itoa(s.Brightness) },
				Adjust: func(d int) {
					s.Brightness = min(100, max(1, s.Brightness+d*brightStep))
				},
			},
			{
				Label: "UNITS",
				Value: func() string { return s.Units },
				Adjust: func(int) {
					if s.Units == "F" {
						s.Units = "C"
					} else {
						s.Units = "F"
					}
				},
			},
			{
				Label: "LOCATION",
				Value: func() string { return strings.ToUpper(s.Location) },
				Adjust: func(int) {
					if s.Location == locationAuto {
						s.Location = locationManual
					} else {
						s.Location = locationAuto
					}
				},
			},
		},
	}
}

func (s *settings) Load(store *storage.Store) error {
	var stored *settings
	err := store.Read(Slug, "", &stored)
	if stored == nil {
		*s = defaultSettings()
		return err
	}
	*s = *stored
	if s.Brightness < 1 || s.Brightness > 100 {
		s.Brightness = defaultSettings().Brightness
	}
	if s.Units != "C" {
		s.Units = "F"
	}
	s.Location = locationMode(s.Location, s.hasLocation())
	return err
}

// Save re-reads lat/lon first so a location the dashboard wrote meanwhile survives.
func (s *settings) Save(rt *engine.Runtime) error {
	var onDisk settings
	if err := rt.Store.Read(Slug, "", &onDisk); err != nil {
		rt.Log("log", fmt.Sprintf("[Morning] settings re-read failed: %v", err))
	} else {
		s.Lat, s.Lon = onDisk.Lat, onDisk.Lon
	}
	rt.InvalidateSchedules()
	return rt.Store.Write(Slug, "", *s)
}
