// SPDX-License-Identifier: GPL-2.0-or-later

package clock

import (
	fclock "github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
)

// digitalFaceScene shows the local time in a gray frame.
type digitalFaceScene struct {
	a           *app
	digitalTime *element.Label
}

func (s *digitalFaceScene) Init() error {
	s.digitalTime = element.NewLabel(element.LabelOptions{
		Options: element.Options{Align: element.Center, OffsetX: 32, OffsetY: 32},
	})
	grayBorder := element.BorderOf(display.Gray)
	frame := element.NewRect(element.RectOptions{
		Options: element.Options{OffsetX: 2, OffsetY: 18},
		W:       60, H: 28, Border: &grayBorder,
	})
	s.a.Ctx.AddLayer(modeDigital, []display.Element{frame, s.digitalTime}, 0)
	return nil
}

func (s *digitalFaceScene) Enter() {
	s.a.Ctx.SetLights(input.BothSeats, false)
}

func (s *digitalFaceScene) HandleInput(fclock.FrameTime, input.Event) bool {
	return false
}

func (s *digitalFaceScene) Update(fclock.FrameTime) {
	s.digitalTime.SetText(s.a.Ctx.Now().Format("3:04:05 PM"))
}

func (s *digitalFaceScene) Render(fclock.FrameTime, scene.RenderReason) {}

func (s *digitalFaceScene) Teardown() {}
