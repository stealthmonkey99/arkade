// SPDX-License-Identifier: GPL-2.0-or-later

package config

import (
	"fmt"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
)

const indicatorRadius = 6

type joyIndicator struct {
	indicator *element.Circle
	label     *element.Label
	pos       input.JoystickPosition
}

type inputTestScene struct {
	a       *app
	source  *element.Label
	message *element.Label
	joys    []joyIndicator
	coords  map[input.JoystickPosition][2]int
	// re-sent with every update: the hardware API sets both LCD lines at once
	line1       string
	backPressed bool
}

func (s *inputTestScene) Init() error {
	s.source = element.NewLabel(element.LabelOptions{
		Options: element.Options{OffsetX: 32, OffsetY: 28, Align: element.Center},
	})
	s.message = element.NewLabel(element.LabelOptions{
		Options: element.Options{OffsetX: 32, OffsetY: 36, Align: element.Center},
		Text:    "press any key",
	})

	borders := []display.Pixel{display.Orange, display.Green, display.Cyan, display.Red}
	darkGray := display.DarkGray
	s.joys = make([]joyIndicator, 2)
	for i := range s.joys {
		border := element.BorderOf(borders[i%4])
		s.joys[i] = joyIndicator{
			indicator: element.NewCircle(element.CircleOptions{
				Options: element.Options{OffsetX: i, OffsetY: i},
				Radius:  indicatorRadius,
				Border:  &border,
				Fill:    &darkGray,
			}),
			label: element.NewLabel(element.LabelOptions{
				Text: fmt.Sprintf("%d:%d", i, i),
			}),
			pos: input.JoyCenter,
		}
	}
	s.coords = map[input.JoystickPosition][2]int{
		input.JoyCenter:    {32, 32},
		input.JoyUp:        {32, 12},
		input.JoyUpLeft:    {12, 12},
		input.JoyUpRight:   {52, 12},
		input.JoyDown:      {32, 52},
		input.JoyDownLeft:  {12, 52},
		input.JoyDownRight: {52, 52},
		input.JoyLeft:      {12, 32},
		input.JoyRight:     {52, 32},
	}
	elements := []display.Element{s.source, s.message}
	for i := range s.joys {
		elements = append(elements, s.joys[i].indicator, s.joys[i].label)
	}
	s.a.Ctx.AddLayer(modeInputTest, elements, 0)
	return nil
}

func (s *inputTestScene) Enter() {
	s.a.Ctx.SetLights(input.BothSeats, false)
}

func (s *inputTestScene) HandleInput(_ clock.FrameTime, ev input.Event) bool {
	if s.a.SceneMgr.State() != modeInputTest {
		return false
	}
	slot, seat := ev.Source.Slot, ev.Source.Seat
	if ev.Source.Type == input.SourceKeyboard {
		s.source.SetText("keyboard")
		// the keyboard stands in for P1 on a desk with no cabinet
		slot, seat = input.SlotOf(0), input.SeatP1
	} else {
		s.source.SetText(fmt.Sprintf("controller %d", slot.Index()))
	}
	// a seatless device has no panel of its own
	lights := func(on bool) {
		s.a.Ctx.SetLights(input.SeatsOf(seat), on)
	}
	text := func(line1, line2 string) {
		s.a.Ctx.SetText(input.SeatsOf(seat), line1, line2)
	}

	switch ev.Kind {
	case input.KindJoystickChange:
		if seat.Valid() && seat.Index() < len(s.joys) {
			s.joys[seat.Index()].pos = ev.Position
		}
	case input.KindKeydown, input.KindKeypress:
		mods := ""
		if ev.Meta {
			mods += "*"
		}
		if ev.Ctrl {
			mods += "^"
		}
		if ev.Shift {
			mods += "+"
		}
		msg := fmt.Sprintf("%d:%d:%s%s", slot.Index(), seat.Index(), mods, ev.Name)
		s.message.SetText(msg)
		lights(true) // per-button lights aren't modeled
		switch ev.Name {
		case input.KeyJoyB, input.KeyEscape, input.KeyQuit:
			if s.backPressed {
				text(s.line1, msg)
				return false
			}
			s.line1 = "press RED again"
			s.backPressed = true
		default:
			s.line1 = ""
			s.backPressed = false
		}
		text(s.line1, msg)
	case input.KindKeyup:
		s.message.SetText("press any key")
		s.a.Ctx.SetLights(input.BothSeats, false)
		text(s.line1, "press any key")
	}
	return true
}

func (s *inputTestScene) Update(clock.FrameTime) {
	if s.a.SceneMgr.State() != modeInputTest {
		return
	}
	for i := range s.joys {
		joy := &s.joys[i]
		x := s.coords[joy.pos][0] + i
		y := s.coords[joy.pos][1] + i
		joy.indicator.SetOffsetX(x - joy.indicator.Radius())
		joy.indicator.SetOffsetY(y - joy.indicator.Radius())
		joy.label.SetOffsetX(x - joy.indicator.Radius() + indicatorRadius)
		joy.label.SetOffsetY(y - joy.indicator.Radius() + indicatorRadius)
		joy.indicator.SetVisible(joy.pos != input.JoyCenter)
		joy.label.SetVisible(joy.pos != input.JoyCenter)
	}
}

func (s *inputTestScene) Render(clock.FrameTime, scene.RenderReason) {}

func (s *inputTestScene) Teardown() {}
