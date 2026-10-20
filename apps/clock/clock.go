// SPDX-License-Identifier: GPL-2.0-or-later

// Package clock is the clock app: analog, digital and calendar faces,
// switched with left/right. Enter/joy-A toggles the analog tick sound and
// up/down page the calendar. The launch face and tick-sound preference
// persist. The display turns toward the player who launched the app, then
// toward whoever last pressed a control.
package clock

import (
	_ "embed"
	"encoding/json"
	"fmt"

	fclock "github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/fsm"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
)

//go:embed manifest.json
var manifestJSON []byte

// Clock modes double as FSM state names, scene and layer labels, and
// init-time FSM events.
const (
	modeAnalog   = "analog-face"
	modeDigital  = "digital-face"
	modeCalendar = "calendar-face"
)

// settings is the persisted clock preferences.
type settings struct {
	LaunchMode string `json:"launchMode"`
	TickSound  bool   `json:"tickSound"`
}

var defaultSettings = settings{LaunchMode: modeAnalog, TickSound: true}

type app struct {
	engine.BaseApp
	settings        settings
	lastDisplayTime string
	lastDisplayDate string
	// facing is the seat the display is rotated toward, NoSeat while upright.
	facing input.Seat
}

func init() {
	var m struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(manifestJSON, &m); err != nil {
		panic(fmt.Sprintf("clock: bad embedded manifest.json: %v", err))
	}
	engine.Register(engine.Manifest{
		Slug:        "apps/clock",
		Name:        m.Name,
		Description: engine.DescriptionFromManifestJSON(manifestJSON),
		Type:        engine.TypeApp,
		Icon:        engine.IconFromManifestJSON(manifestJSON),
		New:         newApp,
	})
}

func newApp(ctx *engine.AppContext) engine.App {
	machine := fsm.NewBuilder().
		AddTransition("init", modeAnalog, modeAnalog).
		AddTransition("init", modeDigital, modeDigital).
		AddTransition("init", modeCalendar, modeCalendar).
		AddTransition(modeAnalog, "next", modeDigital).
		AddTransition(modeDigital, "next", modeCalendar).
		AddTransition(modeCalendar, "next", modeAnalog).
		AddTransition(modeAnalog, "prev", modeCalendar).
		AddTransition(modeCalendar, "prev", modeDigital).
		AddTransition(modeDigital, "prev", modeAnalog).
		Build("init")
	return &app{BaseApp: engine.NewBaseApp(ctx, machine), settings: defaultSettings}
}

// Init loads settings, registers the face scenes and jumps to the launch face.
func (a *app) Init() error {
	if err := a.Ctx.ReadStorage("", &a.settings); err != nil {
		a.Ctx.Log(fmt.Sprintf("settings read failed: %v", err))
	}

	faces := []struct {
		label string
		impl  scene.Impl
	}{
		{modeAnalog, &analogFaceScene{a: a}},
		{modeDigital, &digitalFaceScene{a: a}},
		{modeCalendar, &calendarFaceScene{a: a}},
	}
	for _, f := range faces {
		sc, err := a.SceneMgr.Add(f.label, f.impl, scene.Options{})
		if err != nil {
			return err
		}
		sc.TrackLayer(f.label)
	}

	if err := a.SceneMgr.Dispatch(a.settings.LaunchMode); err != nil {
		// Corrupt storage falls back to the analog face.
		a.settings.LaunchMode = modeAnalog
		if err := a.SceneMgr.Dispatch(modeAnalog); err != nil {
			return err
		}
	}
	if cur := a.SceneMgr.Current(); cur != nil {
		cur.TransitionTo()
	}

	a.QueueKeys(input.KeyLeft, input.KeyRight, input.KeyUp, input.KeyDown,
		input.KeyEnter, input.KeyJoyA)

	// Keyboard launches leave it upright.
	a.faceSeat(a.Ctx.RT.LastControllerSeat())
	return nil
}

// faceSeat rotates the display toward a seat: P1 counter-clockwise, P2
// clockwise, anything else upright. The engine restores Upright on teardown.
func (a *app) faceSeat(seat input.Seat) {
	orient := display.Upright
	switch seat {
	case input.SeatP1:
		orient = display.CounterClockwise
	case input.SeatP2:
		orient = display.Clockwise
	default:
		seat = input.NoSeat
	}
	if seat == a.facing {
		return
	}
	a.facing = seat
	a.Ctx.RT.Renderer.VirtualRotate(orient)
}

// HandleInput switches faces on left/right and passes the rest to the scene.
func (a *app) HandleInput(stats fclock.FrameTime) {
	for _, ev := range a.Queue.Drain() {
		if ev.Source.Type == input.SourceController {
			a.faceSeat(ev.Source.Seat)
		}
		switch ev.Name {
		case input.KeyLeft, input.KeyRight:
			event := "prev"
			if ev.Name == input.KeyRight {
				event = "next"
			}
			if err := a.SceneMgr.Transition(event); err != nil {
				a.Ctx.Log(fmt.Sprintf("face transition failed: %v", err))
			}
		default:
			a.SceneMgr.HandleInput(stats, ev)
		}
	}
}

// Update mirrors the time and date onto the LCDs when the strings change.
func (a *app) Update(fclock.FrameTime) {
	now := a.Ctx.Now()
	dispTime := now.Format("3:04:05 PM")
	dispDate := now.Format("Mon Jan 02 2006")
	if a.lastDisplayTime != dispTime || a.lastDisplayDate != dispDate {
		a.lastDisplayTime = dispTime
		a.lastDisplayDate = dispDate
		a.Ctx.SetText(input.BothSeats, dispTime, dispDate)
	}
}

// Teardown remembers the current face as the next launch mode.
func (a *app) Teardown() error {
	a.settings.LaunchMode = a.SceneMgr.State()
	return a.Ctx.WriteStorage("", a.settings)
}
