// SPDX-License-Identifier: GPL-2.0-or-later

package config

import (
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
)

// shutdownScene exits with code 0 so systemd's Restart=on-failure leaves the
// cabinet down.
type shutdownScene struct {
	a *app
}

func (s *shutdownScene) Init() error {
	label := element.NewLabel(element.LabelOptions{
		Options: element.Options{OffsetX: 32, OffsetY: 32, Align: element.Center},
		Text:    "shut down?",
	})
	s.a.Ctx.AddLayer(modeShutdown, []display.Element{label}, 0)
	return nil
}

func (s *shutdownScene) Enter() {
	s.a.Ctx.SetButtonLights(input.BothSeats, hardware.ButtonLights{"joya": true, "joyb": true}, true)
}

func (s *shutdownScene) HandleInput(_ clock.FrameTime, ev input.Event) bool {
	if s.a.SceneMgr.State() != modeShutdown {
		return false
	}
	if ev.Kind != input.KindKeypress {
		return false
	}
	switch ev.Name {
	case input.KeyEnter, input.KeySpace, input.KeyJoyA:
		s.a.Ctx.RT.PowerOff()
		return true
	}
	return false
}

func (s *shutdownScene) Update(clock.FrameTime) {}

func (s *shutdownScene) Render(clock.FrameTime, scene.RenderReason) {}

func (s *shutdownScene) Teardown() {}
