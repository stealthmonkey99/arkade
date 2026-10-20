// SPDX-License-Identifier: GPL-2.0-or-later

// Package config is the cabinet settings app: a menu of settings scenes.
package config

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/stealthmonkey99/arkade/internal/clock"
	cabinet "github.com/stealthmonkey99/arkade/internal/config"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/fsm"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
)

//go:embed manifest.json
var manifestJSON []byte

// Mode values double as FSM states, events and layer labels.
const (
	modeInputTest        = "mode-input-test"
	modeWifiConnect      = "mode-wifi-connect"
	modeAudio            = "mode-audio"
	modeFan              = "mode-fan"
	modeRotation         = "mode-rotation"
	modeResetControllers = "mode-reset-controllers"
	modeSystemStats      = "mode-system-stats"
	modeShutdown         = "mode-shutdown"
	modeCrashTest        = "mode-crash-test"
	modeScreensavers     = "mode-screensavers"
	modeScreensaverEdit  = "mode-screensaver-edit"
)

type app struct {
	engine.BaseApp
	settings    cabinet.Cabinet
	saverEditor *screensaverEditScene
}

func init() {
	engine.Register(engine.Manifest{
		Slug:        cabinet.Owner,
		Name:        nameFromManifest(manifestJSON),
		Description: engine.DescriptionFromManifestJSON(manifestJSON),
		Type:        engine.TypeApp,
		Icon:        engine.IconFromManifestJSON(manifestJSON),
		New:         newApp,
	})
}

func nameFromManifest(raw []byte) string {
	var m struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		panic(fmt.Sprintf("bad embedded manifest.json: %v", err))
	}
	return m.Name
}

func newApp(ctx *engine.AppContext) engine.App {
	machine := fsm.NewBuilder().
		AddTransition("init", "load", "menu").
		AddTransition("menu", modeResetControllers, modeResetControllers).
		AddTransition(modeResetControllers, "back", "menu").
		AddTransition("menu", modeInputTest, modeInputTest).
		AddTransition(modeInputTest, "back", "menu").
		AddTransition("menu", modeAudio, modeAudio).
		AddTransition(modeAudio, "back", "menu").
		AddTransition("menu", modeFan, modeFan).
		AddTransition(modeFan, "back", "menu").
		AddTransition("menu", modeRotation, modeRotation).
		AddTransition(modeRotation, "back", "menu").
		AddTransition("menu", modeWifiConnect, modeWifiConnect).
		AddTransition(modeWifiConnect, "back", "menu").
		AddTransition("menu", modeSystemStats, modeSystemStats).
		AddTransition(modeSystemStats, "back", "menu").
		AddTransition("menu", modeShutdown, modeShutdown).
		AddTransition(modeShutdown, "back", "menu").
		AddTransition("menu", modeCrashTest, modeCrashTest).
		AddTransition(modeCrashTest, "back", "menu").
		AddTransition("menu", modeScreensavers, modeScreensavers).
		AddTransition(modeScreensavers, "back", "menu").
		AddTransition(modeScreensavers, modeScreensaverEdit, modeScreensaverEdit).
		AddTransition(modeScreensaverEdit, "back", modeScreensavers).
		Build("init")
	return &app{BaseApp: engine.NewBaseApp(ctx, machine)}
}

func (a *app) addScene(label string, impl scene.Impl, layer string) error {
	sc, err := a.SceneMgr.Add(label, impl, scene.Options{})
	if err != nil {
		return err
	}
	sc.TrackLayer(layer)
	return nil
}

func (a *app) Init() error {
	var err error
	if a.settings, err = cabinet.Load(a.Ctx.RT.Store); err != nil {
		a.Ctx.Log(fmt.Sprintf("settings read failed: %v", err))
	}

	if err := a.addScene("menu", &menuScene{a: a}, "menu"); err != nil {
		return err
	}
	if err := a.addScene(modeInputTest, &inputTestScene{a: a}, modeInputTest); err != nil {
		return err
	}
	if err := a.addScene(modeWifiConnect, &wifiConnectScene{a: a}, modeWifiConnect); err != nil {
		return err
	}
	if err := a.addScene(modeAudio, &audioScene{a: a}, modeAudio); err != nil {
		return err
	}
	if err := a.addScene(modeFan, &fanScene{a: a}, modeFan); err != nil {
		return err
	}
	if err := a.addScene(modeRotation, &rotationScene{a: a}, modeRotation); err != nil {
		return err
	}
	if err := a.addScene(modeResetControllers, &resetControllersScene{a: a}, modeResetControllers); err != nil {
		return err
	}
	if err := a.addScene(modeSystemStats, &systemStatsScene{a: a}, modeSystemStats); err != nil {
		return err
	}
	if err := a.addScene(modeShutdown, &shutdownScene{a: a}, modeShutdown); err != nil {
		return err
	}
	if err := a.addScene(modeCrashTest, &crashTestScene{a: a}, modeCrashTest); err != nil {
		return err
	}
	if err := a.addScene(modeScreensavers, &screensaversScene{a: a}, modeScreensavers); err != nil {
		return err
	}
	a.saverEditor = &screensaverEditScene{a: a}
	if err := a.addScene(modeScreensaverEdit, a.saverEditor, modeScreensaverEdit); err != nil {
		return err
	}
	if err := a.SceneMgr.Transition("load"); err != nil {
		return err
	}

	// A keypress condition matches any key name, so this also intercepts
	// JOY_B/ESCAPE/QUIT ahead of the engine's default quit map.
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

// HandleInput lets scenes consume events first; unconsumed back/quit pops the
// scene, or exits from the menu.
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
			if a.SceneMgr.State() == "menu" {
				a.Ctx.Exit()
			} else if err := a.SceneMgr.Transition("back"); err != nil {
				a.Ctx.Log(fmt.Sprintf("back transition failed: %v", err))
			}
		default:
			a.Ctx.RejectKey()
		}
	}
}

func (a *app) Teardown() error {
	return a.settings.Save(a.Ctx.RT.Store)
}
