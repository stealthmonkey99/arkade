// SPDX-License-Identifier: GPL-2.0-or-later

package clock

import (
	"math"

	fclock "github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// analogFaceScene is a round face with hour, minute and second hands.
type analogFaceScene struct {
	a          *app
	hourHand   *element.Line
	minuteHand *element.Line
	secondHand *element.Line
	lastSec    int // -1 = unset
}

func (s *analogFaceScene) Init() error {
	zero := 0.0
	white := display.White
	lightGray := display.LightGray
	red := display.Red
	s.hourHand = element.NewLine(element.LineOptions{
		Options: element.Options{OffsetX: 32, OffsetY: 32},
		Color:   &white, Angle: &zero, Length: 16,
	})
	s.minuteHand = element.NewLine(element.LineOptions{
		Options: element.Options{OffsetX: 32, OffsetY: 32},
		Color:   &lightGray, Angle: &zero, Length: 28,
	})
	s.secondHand = element.NewLine(element.LineOptions{
		Options: element.Options{OffsetX: 32, OffsetY: 32},
		Color:   &red, Angle: &zero, Length: 28,
	})
	grayBorder := element.BorderOf(display.Gray)
	face := element.NewCircle(element.CircleOptions{Border: &grayBorder, Radius: 32})
	s.lastSec = -1
	elements := []display.Element{face}
	elements = append(elements, hourMarks()...)
	elements = append(elements, s.hourHand, s.minuteHand, s.secondHand)
	s.a.Ctx.AddLayer(modeAnalog, elements, 0)
	return nil
}

// hourMarks builds the 12 rim notches (long and white at the quarters) and the
// 12/3/6/9 numerals, which show which way the face is turned.
func hourMarks() []display.Element {
	white := display.White
	lightGray := display.LightGray
	marks := make([]display.Element, 0, 16)
	for h := range 12 {
		angle := float64(h) * 30
		quarter := h%3 == 0
		color, inner, length := &lightGray, 29.0, 3
		if quarter {
			color, inner, length = &white, 27.0, 5
		}
		// Lines draw outward from their origin; angle 0 is up, clockwise.
		rad := angle * math.Pi / 180
		marks = append(marks, element.NewLine(element.LineOptions{
			Options: element.Options{
				OffsetX: 32 + int(math.Round(inner*math.Sin(rad))),
				OffsetY: 32 - int(math.Round(inner*math.Cos(rad))),
			},
			Color: color, Angle: &angle, Length: length,
		}))
	}
	for _, n := range []struct {
		text   string
		cx, cy int // numeral centre, on a radius-21 ring
	}{
		{"12", 32, 11},
		{"3", 53, 32},
		{"6", 32, 53},
		{"9", 11, 32},
	} {
		// TextWidth includes a trailing blank column; drop it to centre the ink.
		marks = append(marks, element.NewLabel(element.LabelOptions{
			Options: element.Options{
				OffsetX: n.cx - (ui.FontLED{}.TextWidth(n.text, 1)-1)/2,
				OffsetY: n.cy - 4,
			},
			Text: n.text, Color: &white,
		}))
	}
	return marks
}

func (s *analogFaceScene) Enter() {
	// The hardware has one light per player, not per button.
	s.a.Ctx.SetLights(input.BothSeats, true)
}

func (s *analogFaceScene) HandleInput(_ fclock.FrameTime, ev input.Event) bool {
	if ev.Name == input.KeyEnter || ev.Name == input.KeyJoyA {
		s.a.settings.TickSound = !s.a.settings.TickSound
		return true
	}
	return false
}

func (s *analogFaceScene) Update(fclock.FrameTime) {
	now := s.a.Ctx.Now()
	h, m, sec := now.Hour(), now.Minute(), now.Second()
	s.hourHand.SetAngle(float64(h%12)/12*360 + float64(m)/(60*12)*360)
	s.minuteHand.SetAngle(float64(m)/60*360 + float64(sec)/(60*60)*360)
	s.secondHand.SetAngle(float64(sec) / 60 * 360)

	if s.a.settings.TickSound && s.lastSec != sec {
		if s.lastSec != -1 {
			s.a.Ctx.PlaySound("./assets/tick.wav")
		}
		s.lastSec = sec
	}
}

func (s *analogFaceScene) Render(fclock.FrameTime, scene.RenderReason) {}

func (s *analogFaceScene) Teardown() {}
