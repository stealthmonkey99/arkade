// SPDX-License-Identifier: GPL-2.0-or-later

package config

import (
	"fmt"

	"github.com/stealthmonkey99/arkade/apps/screensaver"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

type screensaversScene struct {
	a    *app
	menu *element.Menu
}

func (s *screensaversScene) Init() error {
	var buttons []*element.Button
	for _, entry := range screensaver.Entries() {
		buttons = append(buttons, element.NewButton(element.ButtonOptions{
			Label: entry.Name,
			Action: func(*element.Button, input.Event) {
				s.a.saverEditor.open(entry)
				_ = s.a.SceneMgr.Transition(modeScreensaverEdit)
			},
		}))
	}
	s.menu = element.NewMenu(element.MenuOptions{
		Options: element.Options{OffsetX: 2},
		Buttons: buttons,
		Spacing: 2,
	})
	s.a.Ctx.AddLayer(modeScreensavers, []display.Element{s.menu}, 0)
	return nil
}

func (s *screensaversScene) Enter() {
	s.a.Ctx.SetButtonLights(input.BothSeats, hardware.ButtonLights{"joya": true, "joyb": true}, true)
	s.a.Ctx.SetText(input.BothSeats, "", "")
}

func (s *screensaversScene) HandleInput(_ clock.FrameTime, ev input.Event) bool {
	if s.a.SceneMgr.State() != modeScreensavers {
		return false
	}
	return s.menu.HandleInput(ev)
}

func (s *screensaversScene) Update(stats clock.FrameTime) { s.menu.Update(stats) }

func (s *screensaversScene) Render(clock.FrameTime, scene.RenderReason) {}

func (s *screensaversScene) Teardown() {}

const (
	editRowTop   = 2
	editRowPitch = 8
	editLeft     = 3
	editRight    = 61
	editDayPitch = 6
)

var (
	editInk      = display.RGB(130, 130, 130)
	editFocusInk = display.RGB(178, 206, 234)
	editDayOff   = display.RGB(60, 60, 60)
)

// screensaverEditScene is the screensaver's own schedule editor, opened over
// its settings file from SETTINGS; red saves and backs out.
type screensaverEditScene struct {
	a        *app
	name     string
	settings screensaver.Settings
	editor   screensaver.Editor
	panel    *element.SpriteEl
}

func (s *screensaverEditScene) Init() error {
	s.panel = element.NewSprite(element.SpriteOptions{})
	s.a.Ctx.AddLayer(modeScreensaverEdit, []display.Element{s.panel}, 0)
	return nil
}

func (s *screensaverEditScene) open(entry screensaver.Entry) {
	s.name = entry.Name
	s.settings = entry.New()
	if err := s.settings.Load(s.a.Ctx.RT.Store); err != nil {
		s.a.Ctx.Log(fmt.Sprintf("%s settings read failed: %v", entry.Name, err))
	}
	s.editor = s.settings.Editor()
}

func (s *screensaverEditScene) Enter() { s.repaint() }

func (s *screensaverEditScene) repaint() {
	s.panel.SetData(s.draw())
	s.a.Ctx.SetButtonLights(input.BothSeats, s.editor.Lights(), true)
	hint := "RED SAVES"
	if s.editor.Row == screensaver.RowDays {
		hint = "GREEN DAY RED OK"
	}
	s.a.Ctx.SetText(input.BothSeats, s.name, hint)
}

func (s *screensaverEditScene) draw() display.Sprite {
	cv, sprite := display.NewSpriteCanvas(64, 64)
	font := ui.FontLED{}
	for row := 0; row < s.editor.Rows(); row++ {
		y := editRowTop + row*editRowPitch
		ink := editInk
		if row == s.editor.Row {
			ink = editFocusInk
			ui.VLine(cv, 0, y+2, y+4, display.ToUI(editFocusInk))
		}
		font.DrawText(cv, editLeft, y, s.editor.Label(row), 1, display.ToUI(ink))
		if row == screensaver.RowDays {
			s.drawDays(cv, y, row == s.editor.Row)
			continue
		}
		value := s.editor.Value(row)
		font.DrawText(cv, editRight-font.TextWidth(value, 1), y, value, 1, display.ToUI(ink))
	}
	return sprite
}

func (s *screensaverEditScene) drawDays(cv ui.Canvas, y int, focused bool) {
	font := ui.FontLED{}
	x := editRight - editDayPitch*len(screensaver.DayLetters) + 1
	for i := range len(screensaver.DayLetters) {
		ink := editDayOff
		if s.editor.Schedule.Days[i] {
			ink = editFocusInk
		}
		dx := x + i*editDayPitch
		font.DrawText(cv, dx, y, screensaver.DayLetters[i:i+1], 1, display.ToUI(ink))
		if focused && i == s.editor.Day {
			ui.HLine(cv, dx, dx+4, y+7, display.ToUI(editFocusInk))
		}
	}
}

func (s *screensaverEditScene) HandleInput(_ clock.FrameTime, ev input.Event) bool {
	if s.a.SceneMgr.State() != modeScreensaverEdit || ev.Kind != input.KindKeypress {
		return false
	}
	name := ev.Name
	if name == input.KeyEscape {
		name = input.KeyJoyB
	}
	switch s.editor.HandleKey(name) {
	case screensaver.Save:
		if err := s.settings.Save(s.a.Ctx.RT); err != nil {
			s.a.Ctx.Log(fmt.Sprintf("%s settings write failed: %v", s.name, err))
		}
		if err := s.a.SceneMgr.Transition("back"); err != nil {
			s.a.Ctx.Log(fmt.Sprintf("back transition failed: %v", err))
		}
		return true
	case screensaver.Changed:
		s.repaint()
		return true
	}
	return false
}

func (s *screensaverEditScene) Update(clock.FrameTime) {}

func (s *screensaverEditScene) Render(clock.FrameTime, scene.RenderReason) {}

func (s *screensaverEditScene) Teardown() {}
