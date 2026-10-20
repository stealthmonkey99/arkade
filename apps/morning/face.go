// SPDX-License-Identifier: GPL-2.0-or-later

package morning

import (
	"fmt"
	"strings"
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
	"github.com/stealthmonkey99/arkade/internal/weather"
)

const (
	panelW = 64
	panelH = 64

	timeY      = 6
	dateY      = 23
	ruleY      = 33
	ruleW      = 23
	condY      = 39
	outlookY   = 52
	staleY     = condY + condDrop
	staleRight = 62
)

const crossfade = 2 * time.Second

const outlookHold = 20 * time.Second

const staleAfter = 3 * time.Hour

var (
	amber      = display.RGB(255, 153, 51)
	sunInk     = dim(amber, 0.85)
	cloudInk   = dim(display.RGB(140, 158, 217), 0.6)
	moonInk    = dim(display.RGB(198, 212, 255), 0.7)
	tempInk    = dim(amber, 0.85)
	outlookInk = dim(amber, 0.6)
	lowInk     = dim(display.RGB(120, 165, 235), 0.7)
	highInk    = dim(display.RGB(255, 112, 72), 0.7)
	dateInk    = dim(amber, 0.55)
	ruleInk    = dim(amber, 0.3)
	horizonInk = dim(amber, 0.3)
	staleInk   = dim(amber, 0.45)
)

// dim scales a colour toward black, keeping the crossfade the only alpha on screen.
func dim(p display.Pixel, f float64) display.Pixel {
	return display.RGB(uint8(float64(p.R)*f), uint8(float64(p.G)*f), uint8(float64(p.B)*f))
}

type faceScene struct {
	a *app

	prev, cur *clockFace
	cond      *element.SpriteEl
	forecast  *element.SpriteEl
	sunrise   *element.SpriteEl
	stale     *element.SpriteEl
	rule      *element.SpriteEl
	date      *element.SpriteEl

	shownDigits   string
	shownMeridiem string
	fadeStart     time.Time
	fading        bool

	// Lower line rotation: which message is up, since when, and the current dissolve.
	sunriseUp     bool
	outlookSince  time.Time
	outlookFadeAt time.Time
	outlookFading bool

	lastForecast string
	lastSunrise  string
	lastDate     string
	lastCond     conditions
	haveCond     bool
}

func (s *faceScene) Init() error {
	timeOpts := element.Options{OffsetX: panelW / 2, OffsetY: timeY, Align: element.TopCenter}
	s.cur = newClockFace(timeOpts, amber)
	s.prev = newClockFace(timeOpts, amber)
	s.prev.SetVisible(false)

	centred := func(y int) element.Options {
		return element.Options{OffsetX: panelW / 2, OffsetY: y, Align: element.TopCenter, Hidden: true}
	}
	s.cond = element.NewSprite(element.SpriteOptions{Options: centred(condY)})
	s.forecast = element.NewSprite(element.SpriteOptions{Options: centred(outlookY)})
	s.sunrise = element.NewSprite(element.SpriteOptions{Options: centred(outlookY)})
	s.stale = element.NewSprite(element.SpriteOptions{
		Options: element.Options{OffsetX: staleRight, OffsetY: staleY, Align: element.TopRight, Hidden: true},
		Data:    smallSprite("OLD", staleInk),
	})
	s.rule = element.NewSprite(element.SpriteOptions{
		Options: centred(ruleY),
		Data:    dottedRule(ruleW, ruleInk),
	})
	s.date = element.NewSprite(element.SpriteOptions{
		Options: element.Options{OffsetX: panelW / 2, OffsetY: dateY, Align: element.TopCenter},
	})

	s.a.Ctx.AddLayer(modeFace, []display.Element{
		s.prev, s.cur, s.cond, s.forecast, s.sunrise, s.stale, s.rule, s.date,
	}, 0)
	return nil
}

func (s *faceScene) Enter() {
	c := s.a.Ctx
	c.SetPanelDim(s.a.settings.Brightness)
	c.SetLights(input.BothSeats, false)
	c.SetText(input.BothSeats, "", "")
	c.SetLCDBacklight(false)
	s.refresh(c.Now(), true)
}

func (s *faceScene) Update(clock.FrameTime) {
	if s.a.SceneMgr.State() != modeFace {
		return
	}
	s.refresh(s.a.Ctx.Now(), false)
}

// refresh repaints everything that may change. snap skips the crossfade, for the first frame after a load.
func (s *faceScene) refresh(now time.Time, snap bool) {
	s.refreshTime(now, snap)
	s.refreshWeather(now, snap)
	if d := formatDate(now); d != s.lastDate {
		s.lastDate = d
		s.date.SetData(smallSprite(d, dateInk))
	}
}

func (s *faceScene) refreshTime(now time.Time, snap bool) {
	digits, meridiem := formatClock(now)
	switch {
	case snap:
		s.cur.SetTime(digits, meridiem)
		s.cur.SetOpacity(1)
		s.prev.SetVisible(false)
		s.shownDigits, s.shownMeridiem = digits, meridiem
		s.fading = false
	case digits != s.shownDigits || meridiem != s.shownMeridiem:
		s.prev.SetTime(s.shownDigits, s.shownMeridiem)
		s.prev.SetVisible(true)
		s.cur.SetTime(digits, meridiem)
		s.shownDigits, s.shownMeridiem = digits, meridiem
		s.fadeStart, s.fading = now, true
	}
	if !s.fading {
		return
	}
	f := float64(now.Sub(s.fadeStart)) / float64(crossfade)
	if f >= 1 {
		s.fading = false
		s.cur.SetOpacity(1)
		s.prev.SetVisible(false)
		return
	}
	e := smoothstep(f)
	s.cur.SetOpacity(e)
	s.prev.SetOpacity(1 - e)
}

func (s *faceScene) refreshWeather(now time.Time, snap bool) {
	if s.a.poller == nil {
		return // no location configured: the clock stands alone
	}
	s.rule.SetVisible(true)

	r, ok := s.a.poller.Latest()
	if !ok {
		s.setConditions(conditions{value: "--"})
		s.stale.SetVisible(false)
		s.setForecast("--", "--")
		s.refreshOutlook(now, time.Time{}, false, snap)
		return
	}
	s.setConditions(conditions{
		sky:   r.Sky(),
		night: r.Night(now),
		known: true,
		value: fmt.Sprintf("%.0f", r.TempNow),
		unit:  r.Units,
	})
	s.stale.SetVisible(now.Sub(r.At) > staleAfter)
	s.setForecast(fmt.Sprintf("%.0f", r.Low), fmt.Sprintf("%.0f", r.High))

	// sunrise shows only while it is dark and still to come
	rise, haveRise := r.NextSunrise(now)
	s.refreshOutlook(now, rise, haveRise && r.Night(now), snap)
}

// conditions is everything the upper weather line draws. known is false
// before any reading lands, when only dashes are shown.
type conditions struct {
	sky   weather.Sky
	night bool
	known bool
	value string
	unit  string
}

// setConditions redraws only on change, since SpriteEl.SetData always dirties the element.
func (s *faceScene) setConditions(c conditions) {
	if s.haveCond && c == s.lastCond {
		return
	}
	s.haveCond, s.lastCond = true, c
	var icon display.Sprite
	if c.known {
		icon = weatherIcon(c.sky, c.night, sunInk, cloudInk, moonInk)
	}
	s.cond.SetData(conditionsSprite(icon, c.value, c.unit, tempInk))
	s.cond.SetVisible(true)
}

// setForecast draws "L51° H68°", with cool and warm inks on the numbers.
func (s *faceScene) setForecast(low, high string) {
	text := "L" + low + " H" + high
	if text == s.lastForecast {
		return
	}
	s.lastForecast = text
	s.forecast.SetData(smallRuns(
		inkRun{text: "L", ink: outlookInk}, inkRun{text: low, ink: lowInk, degree: true},
		inkRun{text: " H", ink: outlookInk}, inkRun{text: high, ink: highInk, degree: true},
	))
}

// refreshOutlook rotates the lower line between forecast and sunrise. Without
// haveRise the forecast keeps the line; snap lands on it with no dissolve.
func (s *faceScene) refreshOutlook(now, rise time.Time, haveRise, snap bool) {
	if haveRise {
		if at, _ := formatClock(rise); at != s.lastSunrise {
			s.lastSunrise = at
			s.sunrise.SetData(sunriseSprite(at, sunInk, horizonInk, outlookInk))
		}
	}
	if snap {
		s.sunriseUp, s.outlookFading, s.outlookSince = false, false, now
		s.forecast.SetOpacity(1)
		s.forecast.SetVisible(true)
		s.sunrise.SetVisible(false)
		return
	}
	if !s.outlookFading {
		// also brings the line up when the weather arrives after Enter
		s.forecast.SetVisible(!s.sunriseUp)
		s.sunrise.SetVisible(s.sunriseUp)
	}

	want := s.sunriseUp
	switch {
	case !haveRise:
		want, s.outlookSince = false, now
	case now.Sub(s.outlookSince) >= outlookHold:
		want = !s.sunriseUp
	}
	if want != s.sunriseUp {
		s.sunriseUp = want
		s.outlookSince, s.outlookFadeAt, s.outlookFading = now, now, true
	}
	s.driveOutlookFade(now)
}

// driveOutlookFade advances the handover. It times off outlookFadeAt because
// outlookSince restarts every tick while there is no sunrise to rotate to.
func (s *faceScene) driveOutlookFade(now time.Time) {
	if !s.outlookFading {
		return
	}
	in, out := s.forecast, s.sunrise
	if s.sunriseUp {
		in, out = s.sunrise, s.forecast
	}
	in.SetVisible(true)
	f := float64(now.Sub(s.outlookFadeAt)) / float64(crossfade)
	if f >= 1 {
		s.outlookFading = false
		in.SetOpacity(1)
		out.SetVisible(false)
		return
	}
	e := smoothstep(f)
	in.SetOpacity(e)
	out.SetOpacity(1 - e)
}

// HandleInput is a no-op: the engine swallows presses while this screensaver shows.
func (s *faceScene) HandleInput(clock.FrameTime, input.Event) bool { return false }

func (s *faceScene) Render(clock.FrameTime, scene.RenderReason) {}

func (s *faceScene) Teardown() {}

func formatClock(now time.Time) (digits, meridiem string) {
	h := now.Hour() % 12
	if h == 0 {
		h = 12
	}
	meridiem = "AM"
	if now.Hour() >= 12 {
		meridiem = "PM"
	}
	return fmt.Sprintf("%d:%02d", h, now.Minute()), meridiem
}

func formatDate(now time.Time) string { return strings.ToUpper(now.Format("Mon Jan 2")) }

func smoothstep(t float64) float64 {
	switch {
	case t <= 0:
		return 0
	case t >= 1:
		return 1
	}
	return t * t * (3 - 2*t)
}

// dottedRule is the hairline under the weather row: every other pixel lit.
func dottedRule(w int, c display.Pixel) display.Sprite {
	row := make([]display.Pixel, w)
	for x := range row {
		row[x] = display.Transparent
		if x%2 == 0 {
			row[x] = c
		}
	}
	return display.Sprite{row}
}
