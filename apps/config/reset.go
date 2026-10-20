// SPDX-License-Identifier: GPL-2.0-or-later

package config

import (
	"fmt"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
)

type resetControllersScene struct {
	a *app
}

func (s *resetControllersScene) Init() error {
	label := element.NewLabel(element.LabelOptions{
		Options: element.Options{OffsetX: 32, OffsetY: 32, Align: element.Center},
		Text:    "are you sure?",
	})
	s.a.Ctx.AddLayer(modeResetControllers, []display.Element{label}, 0)
	return nil
}

func (s *resetControllersScene) Enter() {
	s.a.Ctx.SetButtonLights(input.BothSeats, hardware.ButtonLights{"joya": true, "joyb": true}, true)
}

func (s *resetControllersScene) HandleInput(_ clock.FrameTime, ev input.Event) bool {
	if s.a.SceneMgr.State() != modeResetControllers {
		return false
	}
	if ev.Kind != input.KindKeypress {
		return false
	}
	switch ev.Name {
	case input.KeyEnter, input.KeySpace, input.KeyJoyA:
		if err := s.a.Ctx.RT.Store.Delete("boot", ""); err != nil {
			s.a.Ctx.Log(fmt.Sprintf("clearing controller pairings failed: %v", err))
		}
		if err := s.a.Ctx.Launch("boot"); err != nil {
			// headless builds and tests register no boot app
			s.a.Ctx.Exit()
		}
		return true
	}
	return false
}

func (s *resetControllersScene) Update(clock.FrameTime) {}

func (s *resetControllersScene) Render(clock.FrameTime, scene.RenderReason) {}

func (s *resetControllersScene) Teardown() {}
