// SPDX-License-Identifier: GPL-2.0-or-later

package config

import (
	"errors"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
)

// crashTestScene aborts the engine with a non-zero exit to exercise systemd's
// Restart=on-failure.
type crashTestScene struct {
	a *app
}

func (s *crashTestScene) Init() error {
	label := element.NewLabel(element.LabelOptions{
		Options: element.Options{OffsetX: 32, OffsetY: 32, Align: element.Center},
		Text:    "crash test?",
	})
	s.a.Ctx.AddLayer(modeCrashTest, []display.Element{label}, 0)
	return nil
}

func (s *crashTestScene) Enter() {
	s.a.Ctx.SetButtonLights(input.BothSeats, hardware.ButtonLights{"joya": true, "joyb": true}, true)
}

func (s *crashTestScene) HandleInput(_ clock.FrameTime, ev input.Event) bool {
	if s.a.SceneMgr.State() != modeCrashTest {
		return false
	}
	if ev.Kind != input.KindKeypress {
		return false
	}
	switch ev.Name {
	case input.KeyEnter, input.KeySpace, input.KeyJoyA:
		s.a.Ctx.RT.Abort(errors.New("crash test requested from config app"))
		return true
	}
	return false
}

func (s *crashTestScene) Update(clock.FrameTime) {}

func (s *crashTestScene) Render(clock.FrameTime, scene.RenderReason) {}

func (s *crashTestScene) Teardown() {}
