// SPDX-License-Identifier: GPL-2.0-or-later

package evening

import (
	"fmt"
	"strings"

	"github.com/stealthmonkey99/arkade/apps/screensaver"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

const (
	rowTop   = 2
	rowPitch = 8
	rowLeft  = 3
	rowRight = 61
)

var (
	rowNameOn  = display.RGB(210, 205, 225)
	rowNameOff = display.RGB(96, 92, 116)
	rowValueOn = display.RGB(191, 150, 235)
	rowValOff  = display.RGB(120, 110, 150)
	caretColor = display.RGB(191, 77, 140)
	dayOn      = display.RGB(191, 150, 235)
	dayOff     = display.RGB(70, 68, 88)
)

func (a *app) initSettings() error {
	a.view = &settingsView{a: a}
	w, h := 64, 64
	a.view.InitBase(element.Options{Width: &w, Height: &h})
	a.view.Impl = a.view.renderImpl
	a.view.MarkDirty()

	a.ctx.AddFilledLayer("bg", display.Black, []display.Element{a.view}, 1)
	a.ctx.AddKeyMap([]*input.Mapping{{
		Conditions: []input.Condition{
			input.Key(input.KeyUp), input.Key(input.KeyDown),
			input.Key(input.KeyLeft), input.Key(input.KeyRight),
			input.Key(input.KeyJoyA), input.Key(input.KeyJoyB),
		},
		Queue: &a.queue,
	}})
	a.ctx.SetLCDBacklight(true)
	a.applyPeripherals()
	return nil
}

// applyPeripherals lights only the useful buttons: red saves, green acts on DAYS.
func (a *app) applyPeripherals() {
	a.ctx.SetButtonLights(input.BothSeats, a.editor.Lights(), true)
	hint := "RED = SAVE"
	if a.editor.Row == screensaver.RowDays {
		hint = "GREEN DAY RED OK"
	}
	a.ctx.SetText(input.BothSeats, "EVENING", hint)
}

func (a *app) handleSettingsKey(name input.KeyName) {
	if a.view == nil {
		return
	}
	switch a.editor.HandleKey(name) {
	case screensaver.Save:
		a.save()
		return
	case screensaver.Changed:
		a.applyPeripherals()
		a.view.MarkDirty()
	default:
		if name == input.KeyJoyA {
			a.ctx.RejectKey()
		}
	}
}

func (a *app) save() {
	if err := a.settings.Save(a.ctx.RT); err != nil {
		a.ctx.Log(fmt.Sprintf("settings write failed: %v", err))
	}
	a.ctx.ExitToScreensaver()
}

type settingsView struct {
	element.Base
	a *app
}

func (v *settingsView) renderImpl(clock.FrameTime) display.Sprite {
	sprite := display.NewMatrix(64, 64, display.Transparent)
	for row := 0; row < v.a.editor.Rows(); row++ {
		y := rowTop + row*rowPitch
		sel := row == v.a.editor.Row
		name, value := rowNameOff, rowValOff
		if sel {
			name, value = rowNameOn, rowValueOn
			sprite[y+2][0] = caretColor
			sprite[y+3][0] = caretColor
			sprite[y+4][0] = caretColor
		}
		blit(sprite, display.TextSprite(v.a.editor.Label(row), name), rowLeft, y)
		if row == screensaver.RowDays {
			v.drawDays(sprite, y, sel)
			continue
		}
		text := v.a.editor.Value(row)
		blit(sprite, display.TextSprite(text, value), rowRight-ui.FontLED{}.TextWidth(text, 1), y)
	}
	return sprite
}

// drawDays paints the week, lit for active days, with a cursor under the one green toggles.
func (v *settingsView) drawDays(sprite display.Sprite, y int, sel bool) {
	const pitch = 6
	x := rowRight - pitch*7 + 1
	for i, letter := range strings.Split(screensaver.DayLetters, "") {
		c := dayOff
		if v.a.settings.Schedule.Days[i] {
			c = dayOn
		}
		blit(sprite, display.TextSprite(letter, c), x+i*pitch, y)
		if sel && i == v.a.editor.Day {
			for dx := 0; dx < 5; dx++ {
				sprite[y+7][x+i*pitch+dx] = caretColor
			}
		}
	}
}

// blit clips to the panel and skips fully transparent pixels.
func blit(dst, src display.Sprite, x, y int) {
	for sy, row := range src {
		dy := y + sy
		if dy < 0 || dy >= len(dst) {
			continue
		}
		for sx, px := range row {
			dx := x + sx
			if px.A == 0 || dx < 0 || dx >= len(dst[dy]) {
				continue
			}
			dst[dy][dx] = px
		}
	}
}
