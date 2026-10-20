// SPDX-License-Identifier: GPL-2.0-or-later

package config

import (
	"fmt"

	"github.com/stealthmonkey99/arkade/internal/audio"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
)

// boopFile is a cache key for LoadBytes, not a real path.
const boopFile = "boop.wav"

type audioScene struct {
	a      *app
	volume *element.Label
}

func (s *audioScene) Init() error {
	s.volume = element.NewLabel(element.LabelOptions{
		Options: element.Options{OffsetX: 32, OffsetY: 32, Align: element.Center},
	})
	s.a.Ctx.AddLayer(modeAudio, []display.Element{s.volume}, 0)
	if err := s.a.Ctx.RT.Audio.LoadBytes(s.a.Ctx.Manifest.Slug, boopFile, audio.PreviewBoop()); err != nil {
		s.a.Ctx.Log(fmt.Sprintf("boop: %v", err))
	}
	return nil
}

func (s *audioScene) Enter() {
	s.a.Ctx.SetButtonLights(input.BothSeats, hardware.ButtonLights{"joyd": true}, true)
	s.updateDisplay()
}

func (s *audioScene) updateDisplay() {
	if s.a.settings.Muted {
		s.volume.SetText("- muted -")
	} else {
		s.volume.SetText(fmt.Sprintf("volume: %d%%", s.a.settings.Volume))
	}
	if vc, ok := s.a.Ctx.RT.Audio.(audio.VolumeControl); ok {
		vc.SetVolumePercent(s.a.settings.Volume)
		vc.SetMuted(s.a.settings.Muted)
	}
}

// stepVolume applies the new level before previewing it with the boop.
func (s *audioScene) stepVolume(volume int) {
	s.a.settings.Volume = volume
	s.a.settings.Muted = false
	s.updateDisplay()
	s.a.Ctx.PlaySound(boopFile)
}

func (s *audioScene) HandleInput(_ clock.FrameTime, ev input.Event) bool {
	if s.a.SceneMgr.State() != modeAudio {
		return false
	}
	if ev.Kind != input.KindKeypress {
		return false
	}
	switch ev.Name {
	case input.KeyUp:
		if s.a.settings.Volume < 100 {
			s.stepVolume(min(100, s.a.settings.Volume+5))
			return true
		}
	case input.KeyDown:
		if s.a.settings.Volume > 0 {
			s.stepVolume(max(0, s.a.settings.Volume-5))
			return true
		}
	case input.KeyJoyD:
		s.a.settings.Muted = !s.a.settings.Muted
		s.updateDisplay()
		return true
	}
	// UP at 100% and DOWN at 0% fall through and the app rejects them
	return false
}

func (s *audioScene) Update(clock.FrameTime) {}

func (s *audioScene) Render(clock.FrameTime, scene.RenderReason) {}

func (s *audioScene) Teardown() {}
