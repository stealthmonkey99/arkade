// SPDX-License-Identifier: GPL-2.0-or-later

// Package bluetooth is the wireless-controller screen: pairing and seat
// assignment. Seats are remembered by the pad's own identity so they survive
// the pad sleeping.
package bluetooth

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/fsm"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
)

//go:embed manifest.json
var manifestJSON []byte

// Slug is what the shell launches this app by.
const Slug = "apps/bluetooth"

const (
	modeDevices = "mode-devices"
	modeAdd     = "mode-add"
)

// settings is the persisted copy of the controller manager's seat memory,
// keyed by the pad's stable identity (its MAC).
type settings struct {
	Seats map[string]int `json:"seats,omitempty"`
}

type app struct {
	engine.BaseApp
	settings settings
	// shared by both screens; connecting blocks
	bus bus
}

func init() {
	var m struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(manifestJSON, &m); err != nil {
		panic(fmt.Sprintf("bluetooth: bad embedded manifest.json: %v", err))
	}
	engine.Register(engine.Manifest{
		Slug:        Slug,
		Name:        m.Name,
		Description: engine.DescriptionFromManifestJSON(manifestJSON),
		Type:        engine.TypeApp,
		Icon:        engine.IconFromManifestJSON(manifestJSON),
		// Hidden: the SYSTEM shelf places it; visible, it would be listed twice.
		Hidden: true,
		New:    newApp,
	})
}

func newApp(ctx *engine.AppContext) engine.App {
	machine := fsm.NewBuilder().
		AddTransition("init", "load", modeDevices).
		AddTransition(modeDevices, modeAdd, modeAdd).
		AddTransition(modeAdd, "back", modeDevices).
		Build("init")
	return &app{BaseApp: engine.NewBaseApp(ctx, machine)}
}

func (a *app) Init() error {
	a.settings.Seats = map[string]int{}
	if err := a.Ctx.ReadStorage("", &a.settings); err != nil {
		a.Ctx.Log(fmt.Sprintf("bluetooth: settings read failed: %v", err))
	}
	if a.settings.Seats == nil {
		a.settings.Seats = map[string]int{}
	}
	// Covers a pad that attached before its seat was written down.
	if m := a.Ctx.RT.Controllers; m != nil {
		for id, seat := range a.settings.Seats {
			m.RememberSeat(id, input.SeatOf(seat))
		}
		for _, at := range m.List() {
			if seat, ok := a.settings.Seats[at.StableID]; ok && at.Seat != input.SeatOf(seat) {
				m.AssignSeat(at.ID, input.SeatOf(seat))
			}
		}
	}

	a.bus.log = a.Ctx.Log
	a.bus.open()

	if err := a.addScene(modeDevices, &devicesScene{a: a, bus: &a.bus}); err != nil {
		return err
	}
	if err := a.addScene(modeAdd, &scanScene{a: a, bus: &a.bus}); err != nil {
		return err
	}
	if err := a.SceneMgr.Transition("load"); err != nil {
		return err
	}
	// A kind-only condition matches any key name, which intercepts
	// JOY_B/ESCAPE above the engine's default quit map.
	kinds := []input.EventKind{
		input.KindJoystickChange, input.KindKeydown,
		input.KindKeyup, input.KindKeypress,
	}
	conds := make([]input.Condition, len(kinds))
	for i := range kinds {
		conds[i] = input.Condition{Kind: &kinds[i]}
	}
	a.Ctx.AddKeyMap([]*input.Mapping{{Conditions: conds, Queue: &a.Queue}})
	return nil
}

func (a *app) HandleInput(stats clock.FrameTime) {
	for _, ev := range a.Queue.Drain() {
		if a.SceneMgr.HandleInput(stats, ev) {
			continue
		}
		if ev.Kind != input.KindKeypress {
			continue
		}
		switch ev.Name {
		case input.KeyJoyB, input.KeyEscape, input.KeyQuit:
			if a.SceneMgr.State() == modeDevices {
				a.Ctx.Exit()
			} else if err := a.SceneMgr.Transition("back"); err != nil {
				a.Ctx.Log(fmt.Sprintf("bluetooth: back failed: %v", err))
			}
		default:
			a.Ctx.RejectKey()
		}
	}
}

func (a *app) addScene(label string, impl scene.Impl) error {
	sc, err := a.SceneMgr.Add(label, impl, scene.Options{})
	if err != nil {
		return err
	}
	sc.TrackLayer(label)
	return nil
}

func (a *app) rememberSeat(stableID string, seat input.Seat) {
	if stableID == "" {
		return
	}
	if a.settings.Seats == nil {
		a.settings.Seats = map[string]int{}
	}
	a.settings.Seats[stableID] = seat.Index()
	if m := a.Ctx.RT.Controllers; m != nil {
		m.RememberSeat(stableID, seat)
	}
	a.save()
}

func (a *app) forgetSeat(stableID string) {
	delete(a.settings.Seats, stableID)
	if m := a.Ctx.RT.Controllers; m != nil {
		m.ForgetSeat(stableID)
	}
	a.save()
}

// save writes immediately rather than at Teardown so a power cut mid-game
// does not lose the seat.
func (a *app) save() {
	if err := a.Ctx.WriteStorage("", a.settings); err != nil {
		a.Ctx.Log(fmt.Sprintf("bluetooth: settings write failed: %v", err))
	}
}

func (a *app) Teardown() error {
	a.bus.close()
	a.save()
	return a.BaseApp.Teardown()
}
