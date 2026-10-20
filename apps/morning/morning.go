// SPDX-License-Identifier: GPL-2.0-or-later

// Package morning is the morning-mode screensaver: a bedside clock shown
// instead of the attract reel while its schedule window is open, with a
// weather row from Open-Meteo. Green reloads it as a settings screen
// (engine.AppContext.OpenedForSettings); manual coordinates are written by
// the operator dashboard, not edited there.
package morning

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/stealthmonkey99/arkade/apps/screensaver"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/fsm"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
	"github.com/stealthmonkey99/arkade/internal/weather"
)

// Slug is the app's registry slug, its storage owner and the dashboard route segment.
const Slug = "apps/morning"

const (
	weatherLabel  = "weather"
	locationLabel = "location"
)

// weatherInterval is the refetch period, and the cache age that triggers a refetch on load.
const weatherInterval = 10 * time.Minute

// presenceWindow is how recent room activity must be for a refetch to be worth making.
const presenceWindow = weatherInterval

const locationTTL = 24 * time.Hour

// weatherBaseURL is the forecast endpoint; tests point it at an httptest server.
var weatherBaseURL = weather.DefaultBaseURL

// Entry points: the face and the settings screen. The engine picks one at
// load; nothing navigates between them.
const (
	modeFace     = "face"
	modeSchedule = "schedule"
)

// bgLayer covers any suspended app's layers the engine keeps underneath.
const bgLayer = "bg"

const (
	locationAuto   = "auto"
	locationManual = "manual"
)

// settings is the app's settings.json; the engine itself reads only "schedule".
type settings struct {
	Schedule   engine.Schedule `json:"schedule"`
	Brightness int             `json:"brightness"`
	Units      string          `json:"units"`
	Location   string          `json:"location,omitempty"`
	// Lat/Lon are the manual coordinates written by the dashboard; nil in manual mode means no weather row.
	Lat *float64 `json:"lat,omitempty"`
	Lon *float64 `json:"lon,omitempty"`
}

func defaultSettings() settings {
	return settings{
		Schedule: engine.Schedule{
			Enabled:  true,
			From:     "06:00",
			To:       "09:00",
			Days:     engine.EveryDay(),
			Priority: 10,
		},
		Brightness: 15,
		Units:      "F",
		Location:   locationAuto,
	}
}

func (s settings) hasLocation() bool { return s.Lat != nil && s.Lon != nil }

// locationMode defaults an unset mode: stored coordinates mean manual, otherwise auto.
func locationMode(mode string, hasCoords bool) string {
	switch mode {
	case locationAuto, locationManual:
		return mode
	}
	if hasCoords {
		return locationManual
	}
	return locationAuto
}

type app struct {
	engine.BaseApp
	settings settings
	poller   *weather.Poller
	cachedAt time.Time

	fix     weather.Location
	haveFix bool
	// locating carries the automatic lookup's result to Update; nil once taken.
	locating     chan locateResult
	locateCancel context.CancelFunc
	locateWG     sync.WaitGroup
}

type locateResult struct {
	fix weather.Location
	err error
}

func init() {
	engine.Register(engine.Manifest{
		Slug:            Slug,
		Name:            "Morning",
		Description:     "A bedside clock for the hours after sunrise.",
		Type:            engine.TypeScreensaver,
		Hidden:          true,
		DefaultSchedule: defaultSettings().Schedule,
		New:             newApp,
	})
	screensaver.Register(screensaver.Entry{
		Name: "MORNING",
		New:  func() screensaver.Settings { s := defaultSettings(); return &s },
	})
}

func newApp(ctx *engine.AppContext) engine.App {
	machine := fsm.NewBuilder().
		AddTransition("init", modeFace, modeFace).
		AddTransition("init", modeSchedule, modeSchedule).
		Build("init")
	return &app{BaseApp: engine.NewBaseApp(ctx, machine)}
}

func (a *app) Init() error {
	a.loadSettings()
	a.Ctx.AddFilledLayer(bgLayer, display.Black, nil, 1)

	mode, impl := modeFace, scene.Impl(&faceScene{a: a})
	if a.Ctx.OpenedForSettings() {
		mode, impl = modeSchedule, &scheduleScene{a: a}
	} else {
		a.resolveLocation()
		a.startWeather()
	}
	sc, err := a.SceneMgr.Add(mode, impl, scene.Options{})
	if err != nil {
		return err
	}
	sc.TrackLayer(mode)
	if err := a.SceneMgr.Dispatch(mode); err != nil {
		return err
	}
	sc.TransitionTo()

	if mode == modeSchedule {
		a.QueueKeys(input.KeyUp, input.KeyDown, input.KeyLeft, input.KeyRight,
			input.KeyJoyA, input.KeyJoyB)
	}
	return nil
}

func (a *app) loadSettings() {
	if err := a.settings.Load(a.Ctx.RT.Store); err != nil {
		a.Ctx.Log(fmt.Sprintf("settings read failed: %v", err))
	}
}

// resolveLocation uses the dashboard's coordinates in manual mode; otherwise
// a cached fix, else a lookup on a goroutine that Update collects.
func (a *app) resolveLocation() {
	if a.settings.Location == locationManual {
		if a.settings.hasLocation() {
			a.fix = weather.Location{Lat: *a.settings.Lat, Lon: *a.settings.Lon}
			a.haveFix = true
		}
		return
	}

	var cached weather.Location
	if err := a.Ctx.ReadStorage(locationLabel, &cached); err != nil {
		a.Ctx.Log(fmt.Sprintf("location cache read failed: %v", err))
	}
	if !cached.At.IsZero() && a.Ctx.Now().Sub(cached.At) < locationTTL {
		a.fix, a.haveFix = cached, true
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.locateCancel = cancel
	a.locating = make(chan locateResult, 1)
	out := a.locating
	a.locateWG.Add(1)
	go func() {
		defer a.locateWG.Done()
		fix, err := weather.Locate(ctx, nil)
		out <- locateResult{fix: fix, err: err}
	}()
}

func (a *app) takeLocation() {
	if a.locating == nil {
		return
	}
	select {
	case res := <-a.locating:
		a.locating = nil
		if res.err != nil {
			a.Ctx.Log(fmt.Sprintf("location lookup failed, drawing the clock alone: %v", res.err))
			return
		}
		a.fix, a.haveFix = res.fix, true
		if err := a.Ctx.WriteStorage(locationLabel, res.fix); err != nil {
			a.Ctx.Log(fmt.Sprintf("location cache write failed: %v", err))
		}
		a.startWeather()
	default:
	}
}

func (a *app) startWeather() {
	if a.poller != nil || !a.haveFix {
		return
	}
	a.poller = weather.NewPoller(&weather.Client{BaseURL: weatherBaseURL},
		a.fix.Lat, a.fix.Lon, a.settings.Units, weatherInterval)
	var cached weather.Reading
	if err := a.Ctx.ReadStorage(weatherLabel, &cached); err != nil {
		a.Ctx.Log(fmt.Sprintf("weather cache read failed: %v", err))
	}
	// a cache in the other unit would draw wrong numbers until the first fetch
	if !cached.At.IsZero() && cached.Units == a.settings.Units {
		a.poller.Seed(cached)
		a.cachedAt = cached.At
	}
}

// Update polls the weather (at most one fetch, on its own goroutine) and caches new readings.
func (a *app) Update(clock.FrameTime) {
	a.takeLocation()
	if a.poller == nil {
		return
	}
	r, ok := a.poller.Latest()
	// the first reading is fetched regardless of presence
	if !ok || a.Ctx.RT.PresenceWithin(presenceWindow) {
		a.poller.Poll(a.Ctx.Now())
	}
	if !ok || !r.At.After(a.cachedAt) {
		return
	}
	a.cachedAt = r.At
	if err := a.Ctx.WriteStorage(weatherLabel, r); err != nil {
		a.Ctx.Log(fmt.Sprintf("weather cache write failed: %v", err))
	}
}

func (a *app) Teardown() error {
	if a.locateCancel != nil {
		a.locateCancel()
		a.locateCancel = nil
	}
	a.locateWG.Wait()
	if a.poller != nil {
		a.poller.Close()
		a.poller = nil
	}
	if a.SceneMgr.State() == modeSchedule {
		return a.settings.Save(a.Ctx.RT)
	}
	return nil
}
