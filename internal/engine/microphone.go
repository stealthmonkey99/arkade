// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/audio"
)

// MicModels is the part of the microphone the engine uses; Runtime.Mic holds
// one when the cabinet has a microphone.
type MicModels interface {
	// SetSessionModels installs the sound models a mounted cartridge brought
	// with it, replacing the whole set. It receives nil at teardown so a model
	// never outlives its cartridge.
	SetSessionModels(models []*assetfile.Envelope)
	// Calibrate marks that the stinger is about to play, so the echo
	// canceller aligns on a known sound.
	Calibrate()
}

// PlayStinger plays the cabinet's stinger and, with a microphone, calibrates
// it on that sound. It plays under the system owner, so it outlasts the app
// that asked for it.
func (rt *Runtime) PlayStinger() error {
	if rt.Mic != nil {
		rt.Mic.Calibrate()
	}
	return rt.Audio.PlayBytes("system", "stinger", audio.Stinger(), 1)
}
