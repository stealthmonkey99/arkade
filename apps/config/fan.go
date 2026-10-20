// SPDX-License-Identifier: GPL-2.0-or-later

package config

import (
	"fmt"

	"github.com/stealthmonkey99/arkade/internal/clock"
	cabinet "github.com/stealthmonkey99/arkade/internal/config"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
)

const fanCPURefreshMs = 1000

type fanScene struct {
	a   *app
	fan *element.Label
	cpu *element.Label

	cpuPrimed        bool
	lastCPURefreshMs float64
}

func (s *fanScene) Init() error {
	s.fan = element.NewLabel(element.LabelOptions{
		Options: element.Options{OffsetX: 32, OffsetY: 24, Align: element.Center},
	})
	s.cpu = element.NewLabel(element.LabelOptions{
		Options: element.Options{OffsetX: 32, OffsetY: 40, Align: element.Center},
	})
	s.a.Ctx.AddLayer(modeFan, []display.Element{s.fan, s.cpu}, 0)
	return nil
}

func (s *fanScene) Enter() {
	s.a.Ctx.SetButtonLights(input.BothSeats, hardware.ButtonLights{"joyd": true}, true)
	s.cpuPrimed = false
	s.updateDisplay()
}

func nextFanMode(m hardware.FanMode) hardware.FanMode {
	switch m {
	case hardware.FanAuto:
		return hardware.FanOn
	case hardware.FanOn:
		return hardware.FanOff
	default:
		return hardware.FanAuto
	}
}

// updateDisplay also applies the setting to the hardware live.
func (s *fanScene) updateDisplay() {
	mode := s.a.settings.FanMode
	switch mode {
	case hardware.FanOff:
		s.fan.SetText("- fans off -")
	case hardware.FanOn:
		s.fan.SetText("- fans on -")
	default:
		s.fan.SetText(fmt.Sprintf("fan: >=%d°F", s.a.settings.FanThresholdF))
	}
	s.a.Ctx.RT.Hardware.SetFanConfig(float64(s.a.settings.FanThresholdF), mode)
}

func (s *fanScene) HandleInput(_ clock.FrameTime, ev input.Event) bool {
	if s.a.SceneMgr.State() != modeFan {
		return false
	}
	if ev.Kind != input.KindKeypress {
		return false
	}
	switch ev.Name {
	case input.KeyUp:
		if s.a.settings.FanThresholdF < cabinet.FanThresholdMax {
			s.a.settings.FanThresholdF = cabinet.ClampFanThreshold(s.a.settings.FanThresholdF + 5)
			s.updateDisplay()
			return true
		}
	case input.KeyDown:
		if s.a.settings.FanThresholdF > cabinet.FanThresholdMin {
			s.a.settings.FanThresholdF = cabinet.ClampFanThreshold(s.a.settings.FanThresholdF - 5)
			s.updateDisplay()
			return true
		}
	case input.KeyJoyD:
		s.a.settings.FanMode = nextFanMode(s.a.settings.FanMode)
		s.updateDisplay()
		return true
	}
	// UP at the max and DOWN at the min fall through and are rejected by the app.
	return false
}

func (s *fanScene) Update(stats clock.FrameTime) {
	if s.a.SceneMgr.State() != modeFan {
		return
	}
	if s.cpuPrimed && stats.DurationMs-s.lastCPURefreshMs < fanCPURefreshMs {
		return
	}
	s.cpuPrimed = true
	s.lastCPURefreshMs = stats.DurationMs
	if tc, ok := s.a.Ctx.RT.Hardware.(hardware.ThermalSensor); ok {
		if degF, ok2 := tc.CPUTempF(); ok2 {
			s.cpu.SetText(fmt.Sprintf("cpu: %.0f°F", degF))
			return
		}
	}
	s.cpu.SetText("cpu: ~")
}

func (s *fanScene) Render(clock.FrameTime, scene.RenderReason) {}

func (s *fanScene) Teardown() {}
