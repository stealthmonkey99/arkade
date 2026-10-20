// SPDX-License-Identifier: GPL-2.0-or-later

package evening

import (
	"time"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
)

const (
	// gateStale is a polling gap long enough that the tracker's history is about a different evening.
	gateStale = 10 * time.Second
	// micLostAfter is how long the microphone must report nothing before the tracker resets.
	micLostAfter = 5 * time.Second
	// settingsTTL bounds how often the gate re-reads settings.json; the
	// settings screen drops the cache on save.
	settingsTTL = 30 * time.Second
)

// room is the gate's state. Engine goroutine only; unlocked.
var room = &gateState{tr: newTracker(SensMedium)}

// gateState answers the engine's once-a-second question of whether the
// equalizer should be up. Entry needs presence and a loud room; staying up
// needs only presence, so quiet scenes do not bounce the panel.
type gateState struct {
	tr *tracker

	set     settings
	setRead time.Time
	setOK   bool

	lastCall time.Time
	seq      uint64
	haveSeq  bool
	micLost  time.Time
	showing  bool
}

func gate(rt *engine.Runtime, now time.Time) bool { return room.evaluate(rt, now) }

func (g *gateState) evaluate(rt *engine.Runtime, now time.Time) bool {
	if !g.lastCall.IsZero() && (now.Sub(g.lastCall) > gateStale || now.Before(g.lastCall)) {
		g.forget()
	}
	g.lastCall = now

	set := g.settings(rt, now)
	g.tr.setSensitivity(set.Sensitivity)
	g.sample(rt, now)

	if !rt.PresenceWithin(set.presenceHold()) {
		g.showing = false
		return false
	}
	if !g.showing && g.tr.Wants() {
		g.showing = true
	}
	return g.showing
}

// sample feeds the tracker one level frame, de-duplicated by Seq.
func (g *gateState) sample(rt *engine.Runtime, now time.Time) {
	src, has := rt.Hardware.(hardware.MicSource)
	if !has {
		return
	}
	lvl, ok := src.MicLevel()
	if !ok {
		if g.micLost.IsZero() {
			g.micLost = now
		}
		if now.Sub(g.micLost) >= micLostAfter {
			g.tr.reset()
			g.haveSeq = false
		}
		return
	}
	g.micLost = time.Time{}
	if g.haveSeq && lvl.Seq == g.seq {
		return
	}
	g.seq, g.haveSeq = lvl.Seq, true
	g.tr.Push(now, lvl.RMS)
}

func (g *gateState) settings(rt *engine.Runtime, now time.Time) settings {
	if g.setOK && now.Sub(g.setRead) < settingsTTL {
		return g.set
	}
	s := defaultSettings()
	if err := rt.Store.Read(slug, "", &s); err != nil {
		s = defaultSettings()
	}
	s.normalize()
	g.set, g.setRead, g.setOK = s, now, true
	return s
}

func (g *gateState) invalidate() { g.setOK = false }

// forget clears the gate's view of the room, keeping the settings cache.
func (g *gateState) forget() {
	g.tr.reset()
	g.haveSeq = false
	g.micLost = time.Time{}
	g.showing = false
}
