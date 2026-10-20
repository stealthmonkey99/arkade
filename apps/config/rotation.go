// SPDX-License-Identifier: GPL-2.0-or-later

package config

import (
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// rotationScene sets the ROOM (idle) and SEATED (table play) default "top"
// edges. Edges are display.Orientation ordinals: Up=0, Right=1, Down=2, Left=3.
type rotationScene struct {
	a *app

	// 0 = room, 1 = seated
	editing int
	// aimed edge per row; written to settings only on apply
	staged  [2]int
	preview bool
	held    bool // this scene owns the engine's base orientation override

	frame *element.Rect
	bar   *element.Rect
	top   *element.Label
	name  *element.Label
	hint  *element.SpriteEl
}

const (
	rotCenter  = 32
	rotFrameHW = 23 // frame half-width/height → spans 9..55
	rotBarLong = 44
	rotBarThin = 3
)

func (s *rotationScene) Init() error {
	frameColor := display.DarkGray
	frameBorder := element.BorderOf(frameColor)
	s.frame = element.NewRect(element.RectOptions{
		Options: element.Options{OffsetX: rotCenter, OffsetY: rotCenter, Align: element.Center},
		W:       rotFrameHW * 2, H: rotFrameHW * 2,
		Fill:   &display.Transparent,
		Border: &frameBorder,
	})
	barFill := display.Cyan
	s.bar = element.NewRect(element.RectOptions{
		Options: element.Options{Align: element.Center},
		W:       rotBarLong, H: rotBarThin,
		Fill:   &barFill,
		Border: &element.NoBorder,
	})
	s.top = element.NewLabel(element.LabelOptions{
		Options: element.Options{Align: element.Center},
		Text:    "TOP",
	})
	s.name = element.NewLabel(element.LabelOptions{
		Options: element.Options{OffsetX: rotCenter, OffsetY: rotCenter, Align: element.Center},
	})
	s.hint = element.NewSprite(element.SpriteOptions{
		Options: element.Options{OffsetX: rotCenter, OffsetY: 60, Align: element.Center},
	})
	s.a.Ctx.AddLayer(modeRotation, []display.Element{s.frame, s.bar, s.top, s.name, s.hint}, 0)
	return nil
}

func (s *rotationScene) Enter() {
	s.a.Ctx.SetButtonLights(input.BothSeats, hardware.ButtonLights{"joya": true, "joyd": true}, true)
	s.editing = 0
	s.preview = false
	s.staged[0] = int(s.a.settings.RoomOrientation)
	s.staged[1] = int(s.a.settings.SeatedOrientation)
	s.a.Ctx.RT.SetBaseOrientationOverride(display.Upright)
	s.held = true
	s.updateDisplay()
}

func (s *rotationScene) placeMarker(edge int) {
	switch edge {
	case 1: // right
		s.bar.SetWidth(rotBarThin)
		s.bar.SetHeight(rotBarLong)
		s.bar.SetOffsetX(rotCenter + rotFrameHW - 1)
		s.bar.SetOffsetY(rotCenter)
		s.top.SetOffsetX(rotCenter + rotFrameHW - 9)
		s.top.SetOffsetY(rotCenter)
	case 2: // bottom
		s.bar.SetWidth(rotBarLong)
		s.bar.SetHeight(rotBarThin)
		s.bar.SetOffsetX(rotCenter)
		s.bar.SetOffsetY(rotCenter + rotFrameHW - 1)
		s.top.SetOffsetX(rotCenter)
		s.top.SetOffsetY(rotCenter + rotFrameHW - 7)
	case 3: // left
		s.bar.SetWidth(rotBarThin)
		s.bar.SetHeight(rotBarLong)
		s.bar.SetOffsetX(rotCenter - rotFrameHW + 1)
		s.bar.SetOffsetY(rotCenter)
		s.top.SetOffsetX(rotCenter - rotFrameHW + 9)
		s.top.SetOffsetY(rotCenter)
	default: // top
		s.bar.SetWidth(rotBarLong)
		s.bar.SetHeight(rotBarThin)
		s.bar.SetOffsetX(rotCenter)
		s.bar.SetOffsetY(rotCenter - rotFrameHW + 1)
		s.top.SetOffsetX(rotCenter)
		s.top.SetOffsetY(rotCenter - rotFrameHW + 7)
	}
	s.bar.MarkDirty()
	s.top.MarkDirty()
}

func (s *rotationScene) updateDisplay() {
	s.placeMarker(s.staged[s.editing])
	if s.editing == 1 {
		s.name.SetText("SEAT")
	} else {
		s.name.SetText("ROOM")
	}
	if s.preview {
		s.hint.SetData(display.TextSprite("APPLIED", display.White))
	} else {
		row := ui.HintRow{LabelColor: ui.White, Hints: []ui.Hint{
			{Text: "SET", Color: ui.ButtonGreen},
			{Text: "SWAP", Color: ui.ButtonBlue},
		}}
		s.hint.SetData(display.Draw(row.Width(), ui.HintRowHeight, row))
	}
	s.name.MarkDirty()
	s.hint.MarkDirty()
}

func edgeForDir(name input.KeyName) (int, bool) {
	switch name {
	case input.KeyUp:
		return 0, true
	case input.KeyRight:
		return 1, true
	case input.KeyDown:
		return 2, true
	case input.KeyLeft:
		return 3, true
	}
	return 0, false
}

func (s *rotationScene) HandleInput(_ clock.FrameTime, ev input.Event) bool {
	if s.a.SceneMgr.State() != modeRotation {
		return false
	}
	if ev.Kind != input.KindKeypress {
		return false
	}
	// consumed: its direction was remapped under the previewed rotation
	if s.preview {
		if _, ok := edgeForDir(ev.Name); ok || ev.Name == input.KeyJoyA {
			s.preview = false
			s.a.Ctx.RT.SetBaseOrientationOverride(display.Upright)
			s.updateDisplay()
			return true
		}
	}
	if edge, ok := edgeForDir(ev.Name); ok {
		s.staged[s.editing] = edge
		s.updateDisplay()
		return true
	}
	switch ev.Name {
	case input.KeyJoyA:
		if s.editing == 1 {
			s.a.settings.SeatedOrientation = display.Orientation(s.staged[1])
			s.a.Ctx.RT.SetSeatedOrientation(display.Orientation(s.staged[1]))
		} else {
			s.a.settings.RoomOrientation = display.Orientation(s.staged[0])
			s.a.Ctx.RT.SetRoomOrientation(display.Orientation(s.staged[0]))
		}
		s.preview = true
		s.a.Ctx.RT.SetBaseOrientationOverride(display.Orientation(s.staged[s.editing]))
		s.updateDisplay()
		return true
	case input.KeyJoyD:
		s.editing ^= 1
		s.preview = false
		s.a.Ctx.RT.SetBaseOrientationOverride(display.Upright)
		s.updateDisplay()
		return true
	}
	return false
}

// Update releases the override on leaving; the scene manager ticks inactive scenes too.
func (s *rotationScene) Update(clock.FrameTime) {
	if s.a.SceneMgr.State() == modeRotation {
		want := display.Upright
		if s.preview {
			want = display.Orientation(s.staged[s.editing])
		}
		s.a.Ctx.RT.SetBaseOrientationOverride(want)
		s.held = true
		return
	}
	if s.held {
		s.a.Ctx.RT.ClearBaseOrientationOverride()
		s.held = false
	}
}

func (s *rotationScene) Render(clock.FrameTime, scene.RenderReason) {}

func (s *rotationScene) Teardown() {
	if s.held {
		s.a.Ctx.RT.ClearBaseOrientationOverride()
		s.held = false
	}
}
