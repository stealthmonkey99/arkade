// SPDX-License-Identifier: GPL-2.0-or-later

package morning

import (
	"fmt"

	"github.com/stealthmonkey99/arkade/apps/screensaver"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

const (
	titleY    = 2
	rowTop    = 11
	rowPitch  = 7
	labelX    = 3
	valueX    = 37
	footerY   = 59
	dayCursor = 5 // rows below a day letter's top where its cursor bar sits
)

var (
	rowInk     = dim(amber, 0.5)
	selectFill = display.RGBA(255, 153, 51, 48)
	dayOnInk   = display.RGB(60, 200, 90)
	dayOffInk  = display.RGB(70, 70, 70)
)

type scheduleScene struct {
	a      *app
	editor screensaver.Editor
	panel  *element.SpriteEl
}

func (s *scheduleScene) Init() error {
	s.editor = s.a.settings.Editor()
	s.panel = element.NewSprite(element.SpriteOptions{})
	s.a.Ctx.AddLayer(modeSchedule, []display.Element{s.panel}, 0)
	return nil
}

func (s *scheduleScene) Enter() {
	s.editor.Row, s.editor.Day = 0, 0
	s.a.Ctx.SetText(input.BothSeats, "Morning", "red saves")
	s.repaint()
}

// repaint lights only the useful buttons: red saves, green toggles a day.
func (s *scheduleScene) repaint() {
	s.panel.SetData(s.draw())
	s.a.Ctx.SetButtonLights(input.BothSeats, s.editor.Lights(), true)
}

func (s *scheduleScene) draw() display.Sprite {
	cv, sprite := display.NewSpriteCanvas(panelW, panelH)
	font := ui.Font3x5{}

	title := "MORNING"
	font.DrawText(cv, (panelW-smallWidth(len(title)))/2, titleY, title, 1, display.ToUI(amber))

	for i := 0; i < s.editor.Rows(); i++ {
		y := rowTop + i*rowPitch
		ink := rowInk
		if i == s.editor.Row {
			ui.FillRect(cv, 0, y-2, panelW, rowPitch, display.ToUI(selectFill))
			ink = amber
		}
		font.DrawText(cv, labelX, y, s.editor.Label(i), 1, display.ToUI(ink))
		if i == screensaver.RowDays {
			s.drawDays(cv, y, i == s.editor.Row)
			continue
		}
		font.DrawText(cv, valueX, y, s.editor.Value(i), 1, display.ToUI(ink))
	}

	footer := "RED SAVES"
	font.DrawText(cv, (panelW-smallWidth(len(footer)))/2, footerY, footer, 1, display.ToUI(rowInk))
	return sprite
}

func (s *scheduleScene) drawDays(cv ui.Canvas, y int, active bool) {
	font := ui.Font3x5{}
	for i := 0; i < len(screensaver.DayLetters); i++ {
		x := valueX + i*smallAdvance
		ink := dayOffInk
		if s.a.settings.Schedule.Days[i] {
			ink = dayOnInk
		}
		font.DrawText(cv, x, y, screensaver.DayLetters[i:i+1], 1, display.ToUI(ink))
		if active && i == s.editor.Day {
			ui.HLine(cv, x, x+2, y+dayCursor, display.ToUI(amber))
		}
	}
}

func (s *scheduleScene) HandleInput(_ clock.FrameTime, ev input.Event) bool {
	if s.a.SceneMgr.State() != modeSchedule || ev.Kind != input.KindKeypress {
		return false
	}
	switch s.editor.HandleKey(ev.Name) {
	case screensaver.Save:
		if err := s.a.settings.Save(s.a.Ctx.RT); err != nil {
			s.a.Ctx.Log(fmt.Sprintf("settings write failed: %v", err))
		}
		s.a.Ctx.ExitToScreensaver()
		return true
	case screensaver.Changed:
		s.repaint()
		return true
	}
	return false
}

func (s *scheduleScene) Update(clock.FrameTime) {}

func (s *scheduleScene) Render(clock.FrameTime, scene.RenderReason) {}

func (s *scheduleScene) Teardown() {}
