// SPDX-License-Identifier: GPL-2.0-or-later

package config

import (
	"fmt"
	"strings"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
)

// No heap-MB line: runtime.MemStats is nondeterministic and would flake the golden.

type systemStatsScene struct {
	a *app

	cpu     *element.Label
	fan     *element.Label
	motion  *element.Label
	seen    *element.Label
	standby *element.Label
	idle    *element.Label
}

func (s *systemStatsScene) Init() error {
	newLine := func(y int) *element.Label {
		return element.NewLabel(element.LabelOptions{Options: element.Options{OffsetX: 2, OffsetY: y}})
	}
	title := element.NewLabel(element.LabelOptions{
		Options: element.Options{OffsetX: 2, OffsetY: 0},
		Text:    "SYSTEM STATS",
	})
	s.cpu = newLine(8)
	s.fan = newLine(16)
	s.motion = newLine(24)
	s.seen = newLine(32)
	s.standby = newLine(40)
	s.idle = newLine(48)

	s.a.Ctx.AddLayer(modeSystemStats, []display.Element{
		title, s.cpu, s.fan, s.motion, s.seen, s.standby, s.idle,
	}, 0)
	return nil
}

func (s *systemStatsScene) Enter() {
	s.a.Ctx.SetButtonLights(input.BothSeats, hardware.ButtonLights{"joyb": true}, true)
	s.refresh()
}

func (s *systemStatsScene) HandleInput(clock.FrameTime, input.Event) bool { return false }

func (s *systemStatsScene) Update(clock.FrameTime) {
	if s.a.SceneMgr.State() != modeSystemStats {
		return
	}
	s.refresh()
}

func (s *systemStatsScene) refresh() {
	rt := s.a.Ctx.RT

	if tc, ok := rt.Hardware.(hardware.ThermalSensor); ok {
		if degF, ok2 := tc.CPUTempF(); ok2 {
			s.cpu.SetText(fmt.Sprintf("CPU %.0fF", degF))
		} else {
			s.cpu.SetText("CPU ~")
		}
	} else {
		s.cpu.SetText("CPU ~")
	}

	fanOn := false
	if fc, ok := rt.Hardware.(hardware.CoolingFan); ok {
		fanOn = fc.FanOn()
	}
	if fanOn {
		s.fan.SetText("FAN ON")
	} else {
		s.fan.SetText("FAN OFF")
	}

	// engine clock, not hardware, so it stays deterministic under the test clock
	presence := rt.PresenceFor()
	if presence < engine.MotionActiveWindow {
		s.motion.SetText("MOTION ACTIVE")
	} else {
		s.motion.SetText("MOTION IDLE")
	}
	s.seen.SetText(fmt.Sprintf("SEEN %ds", int(presence.Seconds())))

	s.standby.SetText("STANDBY " + strings.ToUpper(rt.StandbyState().String()))
	s.idle.SetText(fmt.Sprintf("IDLE %ds", int(rt.IdleFor().Seconds())))
}

func (s *systemStatsScene) Render(clock.FrameTime, scene.RenderReason) {}

func (s *systemStatsScene) Teardown() {}
