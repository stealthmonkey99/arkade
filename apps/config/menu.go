// SPDX-License-Identifier: GPL-2.0-or-later

package config

import (
	"strings"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
)

var menuItems = []struct {
	key   string
	title string
}{
	{modeWifiConnect, "Join wifi"},
	{modeAudio, "volume"},
	{modeInputTest, "test inputs"},
	{modeResetControllers, "reset controls"},
	{modeSystemStats, "system stats"},
	{modeShutdown, "shutdown"},
	{modeCrashTest, "crash test"},
	{modeFan, "fan"},
	{modeRotation, "rotation"},
	{modeScreensavers, "screensavers"},
}

type menuScene struct {
	a    *app
	menu *element.Menu
}

func (s *menuScene) Init() error {
	buttons := make([]*element.Button, 0, len(menuItems))
	for _, item := range menuItems {
		key := item.key
		buttons = append(buttons, element.NewButton(element.ButtonOptions{
			Label: strings.ToUpper(item.title),
			Action: func(*element.Button, input.Event) {
				_ = s.a.SceneMgr.Transition(key)
			},
		}))
	}
	s.menu = element.NewMenu(element.MenuOptions{
		Options: element.Options{OffsetX: 2},
		Buttons: buttons,
		Spacing: 2,
	})
	s.a.Ctx.AddLayer("menu", []display.Element{s.menu}, 0)
	return nil
}

func (s *menuScene) Enter() {
	s.a.Ctx.SetButtonLights(input.BothSeats, hardware.ButtonLights{"joya": true, "joyb": true}, true)
}

func (s *menuScene) HandleInput(_ clock.FrameTime, ev input.Event) bool {
	if s.a.SceneMgr.State() != "menu" {
		return false
	}
	return s.menu.HandleInput(ev)
}

func (s *menuScene) Update(stats clock.FrameTime) {
	s.menu.Update(stats)
}

func (s *menuScene) Render(clock.FrameTime, scene.RenderReason) {}

func (s *menuScene) Teardown() {}
