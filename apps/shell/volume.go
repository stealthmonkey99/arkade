// SPDX-License-Identifier: GPL-2.0-or-later

package shell

import (
	"fmt"

	"github.com/stealthmonkey99/arkade/internal/audio"
	"github.com/stealthmonkey99/arkade/internal/config"
)

const (
	volumeStep = 5
	// boopFile keys audio.PreviewBoop in the audio cache under the shell's slug.
	boopFile = "boop.wav"
)

var volumePanel = &levelPanel{
	adjust:  func(a *app, delta int) { a.bumpVolume(delta * volumeStep) },
	toggle:  (*app).toggleMute,
	paint:   drawVolume,
	readout: volumeLCD,
}

func (a *app) initAudio(cab config.Cabinet) {
	if err := a.ctx.RT.Audio.LoadBytes(a.ctx.Manifest.Slug, boopFile, audio.PreviewBoop()); err != nil {
		a.ctx.Log(fmt.Sprintf("boop: %v", err))
	}
	a.st.volume, a.st.muted = cab.Volume, cab.Muted
}

// bumpVolume steps the level, unmuting, and previews it with the boop. It
// rejects the key at a rail when already unmuted.
func (a *app) bumpVolume(delta int) {
	next := min(100, max(0, a.st.volume+delta))
	if next == a.st.volume && !a.st.muted {
		a.ctx.RejectKey()
		return
	}
	a.st.volume, a.st.muted = next, false
	a.applyAudio()
	a.saveAudio()
	a.ctx.PlaySound(boopFile)
}

func (a *app) toggleMute() {
	a.st.muted = !a.st.muted
	a.applyAudio()
	a.saveAudio()
	if !a.st.muted {
		a.ctx.PlaySound(boopFile)
	}
}

func (a *app) applyAudio() {
	if vc, ok := a.ctx.RT.Audio.(audio.VolumeControl); ok {
		vc.SetVolumePercent(a.st.volume)
		vc.SetMuted(a.st.muted)
	}
}

func (a *app) saveAudio() {
	a.saveCabinet(func(c *config.Cabinet) { c.Volume, c.Muted = a.st.volume, a.st.muted })
}

// volumeTileLCD is the readout while the VOLUME tile is focused on the shelf.
func volumeTileLCD(a *app) (string, string) {
	if a.st.muted {
		return "VOLUME", "- muted -"
	}
	return "VOLUME", fmt.Sprintf("level: %d%%", a.st.volume)
}

func volumeLCD(st *browseState) (lines [2][2]string) {
	lines[0][0] = "VOLUME"
	if st.muted {
		lines[0][1] = "- muted -"
	} else {
		lines[0][1] = lcdBar(st.volume)
	}
	lines[1][0] = "LEFT/RIGHT ADJ"
	lines[1][1] = "BLUE = MUTE"
	return lines
}
