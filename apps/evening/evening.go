// SPDX-License-Identifier: GPL-2.0-or-later

// Package evening is the evening screensaver: within its scheduled hours, when
// someone is about and the room is loud, a dimmed equalizer and small clock
// replace the attract reel. Green opens its settings screen.
package evening

import (
	"fmt"
	"strconv"
	"time"

	"github.com/stealthmonkey99/arkade/apps/screensaver"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/storage"
)

const slug = "apps/evening"

// schedulePriority ranks the evening window above a plain clock where windows overlap.
const schedulePriority = 20

const (
	defaultBrightness  = 40
	defaultPresenceMin = 5
	minBrightness      = 5
	maxBrightness      = 100
	brightnessStep     = 5
	minPresenceMin     = 1
	maxPresenceMin     = 30
)

// settings is the whole settings.json; the engine itself decodes only Schedule.
type settings struct {
	Schedule    engine.Schedule `json:"schedule"`
	Brightness  int             `json:"brightness"`
	Sensitivity Sensitivity     `json:"sensitivity"`
	// minutes after the last motion that the room still counts as occupied
	PresenceHoldMin int `json:"presenceHoldMinutes"`
}

func defaultSettings() settings {
	return settings{
		Schedule: engine.Schedule{
			Enabled:  true,
			From:     "19:00",
			To:       "23:30",
			Days:     engine.EveryDay(),
			Priority: schedulePriority,
		},
		Brightness:      defaultBrightness,
		Sensitivity:     SensMedium,
		PresenceHoldMin: defaultPresenceMin,
	}
}

// normalize repairs a settings file written by an older build, or by hand.
func (s *settings) normalize() {
	d := defaultSettings()
	if s.Brightness < minBrightness || s.Brightness > maxBrightness {
		s.Brightness = d.Brightness
	}
	switch s.Sensitivity {
	case SensLow, SensMedium, SensHigh:
	default:
		s.Sensitivity = d.Sensitivity
	}
	if s.PresenceHoldMin < minPresenceMin || s.PresenceHoldMin > maxPresenceMin {
		s.PresenceHoldMin = d.PresenceHoldMin
	}
	if s.Schedule == (engine.Schedule{}) {
		s.Schedule = d.Schedule
	}
}

func (s settings) presenceHold() time.Duration {
	return time.Duration(s.PresenceHoldMin) * time.Minute
}

var _ screensaver.Settings = (*settings)(nil)

func (st *settings) Editor() screensaver.Editor {
	return screensaver.Editor{
		Schedule: &st.Schedule,
		On:       "YES",
		Off:      "NO",
		Fields: []screensaver.Field{
			{
				Label: "BRIGHT",
				Value: func() string { return strconv.Itoa(st.Brightness) },
				Adjust: func(d int) {
					st.Brightness = min(maxBrightness, max(minBrightness, st.Brightness+d*brightnessStep))
				},
			},
			{
				Label: "SENS",
				Value: func() string {
					switch st.Sensitivity {
					case SensLow:
						return "LOW"
					case SensHigh:
						return "HIGH"
					}
					return "MED"
				},
				Adjust: func(d int) {
					if d > 0 {
						st.Sensitivity = st.Sensitivity.next()
					} else {
						st.Sensitivity = st.Sensitivity.prev()
					}
				},
			},
			{
				Label: "HOLD",
				Value: func() string { return strconv.Itoa(st.PresenceHoldMin) + "M" },
				Adjust: func(d int) {
					st.PresenceHoldMin = min(maxPresenceMin, max(minPresenceMin, st.PresenceHoldMin+d))
				},
			},
		},
	}
}

func (st *settings) Load(store *storage.Store) error {
	*st = defaultSettings()
	err := store.Read(slug, "", st)
	if err != nil {
		*st = defaultSettings()
	}
	st.normalize()
	return err
}

// Save also drops the gate's settings cache, which the engine's invalidation does not reach.
func (st *settings) Save(rt *engine.Runtime) error {
	st.Schedule.Priority = schedulePriority
	rt.InvalidateSchedules()
	err := rt.Store.Write(slug, "", *st)
	room.invalidate()
	return err
}

func init() {
	d := defaultSettings()
	engine.Register(engine.Manifest{
		Slug:            slug,
		Name:            "Evening",
		Description:     "A dimmed equalizer for a loud evening room.",
		Type:            engine.TypeScreensaver,
		Hidden:          true,
		DefaultSchedule: d.Schedule,
		ScreensaverGate: gate,
		New:             newApp,
	})
	screensaver.Register(screensaver.Entry{
		Name: "EVENING",
		New:  func() screensaver.Settings { s := defaultSettings(); return &s },
	})
}

type app struct {
	ctx      *engine.AppContext
	settings settings

	bars   *barsEl
	clock  *element.Label
	view   *settingsView
	queue  input.Queue
	editor screensaver.Editor
}

func newApp(ctx *engine.AppContext) engine.App {
	a := &app{ctx: ctx, settings: defaultSettings()}
	a.editor = a.settings.Editor()
	return a
}

func (a *app) Init() error {
	a.load()
	if a.ctx.OpenedForSettings() {
		return a.initSettings()
	}
	return a.initScreensaver()
}

func (a *app) load() {
	if err := a.settings.Load(a.ctx.RT.Store); err != nil {
		a.ctx.Log(fmt.Sprintf("settings read failed: %v", err))
	}
}

func (a *app) initScreensaver() error {
	a.ctx.SetPanelDim(a.settings.Brightness)
	a.ctx.SetLights(input.BothSeats, false)
	a.ctx.SetText(input.BothSeats, "", "")
	a.ctx.SetLCDBacklight(false)

	a.ctx.AddFilledLayer("bg", display.Black, nil, 1)
	a.bars = newBars()
	lavender := display.RGBA(140, 115, 191, 128)
	a.clock = element.NewLabel(element.LabelOptions{
		Options: element.Options{OffsetX: 62, OffsetY: 2, Align: element.TopRight},
		Color:   &lavender,
	})
	a.clock.SetText(clockText(a.ctx.Now()))
	a.ctx.AddLayer("bars", []display.Element{a.bars, a.clock}, 1)
	return nil
}

// clockText is the corner clock: 12-hour, no leading zero, no meridiem.
func clockText(now time.Time) string {
	h := now.Hour() % 12
	if h == 0 {
		h = 12
	}
	return fmt.Sprintf("%d:%02d", h, now.Minute())
}

func (a *app) HandleInput(stats clock.FrameTime) {
	for _, ev := range a.queue.Drain() {
		if ev.Kind == input.KindKeypress {
			a.handleSettingsKey(ev.Name)
		}
	}
}

func (a *app) Update(stats clock.FrameTime) {
	if a.view != nil {
		return
	}
	bands, ok := a.spectrum()
	a.bars.advance(stats, bands, ok)
	a.clock.SetText(clockText(a.ctx.Now()))
}

// spectrum is the latest band reading; ok is false without a spectrum-reporting microphone.
func (a *app) spectrum() ([hardware.MicBandCount]float64, bool) {
	if b, ok := a.ctx.RT.Hardware.(hardware.MicBander); ok {
		return b.MicBands()
	}
	return [hardware.MicBandCount]float64{}, false
}

func (a *app) Render(clock.FrameTime, engine.RenderReason) {}

func (a *app) Teardown() error { return nil }

var _ engine.App = (*app)(nil)
