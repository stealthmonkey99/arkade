// SPDX-License-Identifier: GPL-2.0-or-later

package clock

import (
	"fmt"
	"strconv"
	"time"

	fclock "github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
)

var monthLabels = [...]string{
	"JANUARY", "FEBRUARY", "MARCH", "APRIL", "MAY", "JUNE",
	"JULY", "AUGUST", "SEPTEMBER", "OCTOBER", "NOVEMBER", "DECEMBER",
}

var dayLabels = [...]string{"SU", "M", "TU", "W", "TH", "FR", "SA"}

type calendarCell struct {
	bg    *element.Rect
	label *element.Label
}

// calendarFaceScene is a month grid with the current day focused; up/down
// page months.
type calendarFaceScene struct {
	a             *app
	title         *element.Label
	cells         []calendarCell
	selectedMonth *time.Time // nil until the user pages
}

func (s *calendarFaceScene) Init() error {
	white := display.White
	red := display.Red
	black := display.Black
	lightGrayBorder := element.BorderOf(display.LightGray)

	s.title = element.NewLabel(element.LabelOptions{
		Options: element.Options{Align: element.TopCenter, OffsetX: 32, OffsetY: 0},
		Color:   &white,
	})
	var days []calendarCell
	for x := 6; x >= 0; x-- {
		days = append(days, calendarCell{
			bg: element.NewRect(element.RectOptions{
				Options: element.Options{OffsetX: x * 9, OffsetY: 7},
				W:       10, H: 7, Fill: &red, Border: &element.EdgeBorder,
			}),
			label: element.NewLabel(element.LabelOptions{
				Options: element.Options{Align: element.Center, OffsetX: x*9 + 5, OffsetY: 11},
				Text:    dayLabels[x], Color: &white,
			}),
		})
	}
	for y := 0; y < 6; y++ {
		for x := 0; x < 7; x++ {
			s.cells = append(s.cells, calendarCell{
				bg: element.NewRect(element.RectOptions{
					Options: element.Options{OffsetX: x * 9, OffsetY: y*8 + 14},
					W:       10, H: 9, Fill: &white, Border: &lightGrayBorder,
				}),
				label: element.NewLabel(element.LabelOptions{
					Options: element.Options{Align: element.TopRight, OffsetX: x*9 + 9, OffsetY: y*8 + 15},
					Color:   &black,
				}),
			})
		}
	}

	els := []display.Element{s.title}
	for _, cell := range days {
		els = append(els, cell.bg, cell.label)
	}
	for _, cell := range s.cells {
		els = append(els, cell.bg, cell.label)
	}
	s.a.Ctx.AddLayer(modeCalendar, els, 0)
	return nil
}

func (s *calendarFaceScene) Enter() {
	s.a.Ctx.SetLights(input.BothSeats, false)
}

func (s *calendarFaceScene) HandleInput(_ fclock.FrameTime, ev input.Event) bool {
	if ev.Kind == input.KindKeypress {
		switch ev.Name {
		case input.KeyUp:
			s.shiftMonth(-1)
			return true
		case input.KeyDown:
			s.shiftMonth(1)
			return true
		}
	}
	return false
}

// shiftMonth moves the viewed month by delta; time.Date normalizes day
// overflow at month ends.
func (s *calendarFaceScene) shiftMonth(delta int) {
	base := s.a.Ctx.Now()
	if s.selectedMonth != nil {
		base = *s.selectedMonth
	}
	t := time.Date(base.Year(), base.Month()+time.Month(delta), base.Day(),
		base.Hour(), base.Minute(), base.Second(), base.Nanosecond(), base.Location())
	s.selectedMonth = &t
}

func (s *calendarFaceScene) Update(fclock.FrameTime) {
	real := s.a.Ctx.Now()
	now := real
	if s.selectedMonth != nil {
		now = *s.selectedMonth
	}
	// today is the focused day, or -1 when viewing another month.
	today := -1
	if now.Year() == real.Year() && now.Month() == real.Month() {
		today = now.Day()
	}
	offset := int(time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Weekday())
	// Day 0 of next month is the last day of this one.
	lastDay := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Day()
	rows := (offset + lastDay + 6) / 7
	count := 1
	s.title.SetText(fmt.Sprintf("%s %d", monthLabels[int(now.Month())-1], now.Year()))
	for i, cell := range s.cells {
		excess := i >= rows*7
		cell.bg.SetVisible(!excess)
		cell.label.SetVisible(!excess)
		// count is compared before it advances, so when today is the 1st the
		// leading blank cells are focused too.
		fill := display.White
		if count == today {
			fill = display.Focus
		}
		cell.bg.SetFill(fill)
		if i < offset || count > lastDay {
			cell.label.SetText("")
		} else {
			cell.label.SetText(strconv.Itoa(count))
			count++
		}
	}
}

func (s *calendarFaceScene) Render(fclock.FrameTime, scene.RenderReason) {}

func (s *calendarFaceScene) Teardown() {}
