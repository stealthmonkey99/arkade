// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// Choreography bounds, tighter than a lab preview's since a pattern is judged in a few seconds.
const (
	// MaxLightShowSteps caps a choreography.
	MaxLightShowSteps = 64
	// DefaultLightShowTimeout is how long a show holds the lamps when the request does not say.
	DefaultLightShowTimeout = 10 * time.Second
	// MinLightShowTimeout floors a request's timeout.
	MinLightShowTimeout = 250 * time.Millisecond
	// MaxLightShowTimeout caps it.
	MaxLightShowTimeout = 30 * time.Second
	// DefaultLightShowStep is a step's hold when it carries none: about a semiquaver at 125bpm, where a
	// pattern starts reading as rhythm.
	DefaultLightShowStep = 120 * time.Millisecond
	// MinLightShowStep floors a step. Steps advance on engine ticks (33ms at 30fps), so below one tick
	// means a new step every tick.
	MinLightShowStep = 10 * time.Millisecond
	// MaxLightShowStep caps one step at the longest show.
	MaxLightShowStep = MaxLightShowTimeout
)

// LightStep is one frame of a choreography: each seat's lamps and how long they hold.
type LightStep struct {
	// Seats is [player]mask. Both seats are always written so the game never shows through half a pattern.
	Seats [2]hardware.LightMask
	// Hold is how long this step stays up; zero means DefaultLightShowStep, others are clamped.
	Hold time.Duration
	// Fade is how long this step cross-fades in, linearly per lamp from its previous level to this step's
	// mask (LampLevelMax for a set bit, 0 for a clear one). Hold's clock starts at the same moment. Zero is a hard cut.
	Fade time.Duration
}

// LightShow is one choreography request. It overlays the lamps only: no input, no standby effects, and
// it always ends on an engine-enforced deadline. It is independent of LabPreview so lights and sound can
// play together.
type LightShow struct {
	// Steps is the pattern, 1 to MaxLightShowSteps.
	Steps []LightStep
	// Loop repeats the pattern until the timeout instead of holding the last step.
	Loop bool
	// Timeout is how long the show owns the lamps; zero means DefaultLightShowTimeout, others are clamped.
	Timeout time.Duration
}

// lightShowState is the live show, owned by the engine goroutine.
type lightShowState struct {
	steps []LightStep
	loop  bool
	// idx is the step on the lamps; advanceAt is when it is due to be replaced.
	idx       int
	advanceAt time.Time
	deadline  time.Time

	// stepStart is when idx was entered; fadeFrom the per-seat levels it fades from; fadeDur the clamped
	// Fade, zeroed once complete; current what is on the lamps now, so an interrupted fade hands the next
	// step an exact start.
	stepStart time.Time
	fadeFrom  [2]hardware.LampLevels
	fadeDur   time.Duration
	current   [2]hardware.LampLevels
}

// RequestLightShow queues a choreography for the top of the next tick. Safe from any goroutine. The
// latest request wins: applying one ends the show already playing. The engine keeps its own copy of
// Steps, so the caller may reuse it at once.
func (rt *Runtime) RequestLightShow(s LightShow) {
	s.Steps = slices.Clone(s.Steps)
	rt.requests.lightShow.Set(&s)
}

// CancelLightShow queues an immediate end to any show, applied at the top of the next tick. Safe from
// any goroutine; harmless when nothing is playing.
func (rt *Runtime) CancelLightShow() { rt.requests.lightShow.Set(nil) }

// applyLightShow applies one drained request; nil is a cancel.
func (rt *Runtime) applyLightShow(now time.Time, s *LightShow) {
	if s == nil {
		rt.endLightShow()
		rt.Log("info", "lab: light choreography cancelled, buttons restored")
		return
	}
	rt.enterLightShow(now, *s)
}

// LightShowActive reports whether a choreography owns the lamps. Engine-goroutine only; HTTP callers
// read Status().LightShowActive.
func (rt *Runtime) LightShowActive() bool { return rt.lightShow != nil }

// ClampLightShowTimeout applies the show's timeout bounds; zero means the default. Exported so
// internal/net can echo the timeout the engine will use.
func ClampLightShowTimeout(d time.Duration) time.Duration {
	if d == 0 {
		return DefaultLightShowTimeout
	}
	return max(MinLightShowTimeout, min(MaxLightShowTimeout, d))
}

// clampLightShowStep applies a step's hold bounds; zero means the default.
func clampLightShowStep(d time.Duration) time.Duration {
	if d == 0 {
		return DefaultLightShowStep
	}
	return max(MinLightShowStep, min(MaxLightShowStep, d))
}

// clampLightFade caps a fade at the step ceiling; zero or negative stays zero, there is no default.
func clampLightFade(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	return min(MaxLightShowStep, d)
}

// enterLightShow applies one drained request, on the engine goroutine.
func (rt *Runtime) enterLightShow(now time.Time, s LightShow) {
	// Replace rather than stack, so the hold on the lamps is taken once and released once.
	rt.endLightShow()

	if len(s.Steps) == 0 {
		rt.Log("warn", "lab: light choreography has no steps, ignored")
		return
	}
	if len(s.Steps) > MaxLightShowSteps {
		// internal/net refuses oversize shows; truncate rather than refuse if one arrives in-process.
		rt.Log("warn", fmt.Sprintf("lab: light choreography truncated from %d steps to %d",
			len(s.Steps), MaxLightShowSteps))
		s.Steps = s.Steps[:MaxLightShowSteps]
	}

	st := &lightShowState{
		steps:    s.Steps,
		loop:     s.Loop,
		deadline: now.Add(ClampLightShowTimeout(s.Timeout)),
	}

	// Hold before writing so an app lighting buttons later this tick cannot land on the first step.
	rt.hwShadow.holdLights()
	rt.lightShow = st
	rt.enterLightStep(now, st, 0, rt.hwShadow.lampLevels())

	rt.Log("info", fmt.Sprintf("lab: light choreography, %d step(s)%s for %s",
		len(st.steps), loopNote(st.loop), st.deadline.Sub(now).Round(time.Millisecond)))
}

func loopNote(loop bool) string {
	if loop {
		return " looping"
	}
	return ""
}

// stepLightShow advances the pattern, rides any fade, and expires the show; called once per tick before
// anything paints.
func (rt *Runtime) stepLightShow(now time.Time) {
	st := rt.lightShow
	if st == nil {
		return
	}
	if !now.Before(st.deadline) {
		rt.endLightShow()
		rt.Log("info", "lab: light choreography ended, buttons restored")
		return
	}
	rt.advanceLightFade(now, st)
	if len(st.steps) < 2 || now.Before(st.advanceAt) {
		return
	}
	next := st.idx + 1
	if next >= len(st.steps) {
		if !st.loop {
			// Hold the last step for the rest of the timeout; a one-shot pattern ends on its final state.
			st.advanceAt = st.deadline
			return
		}
		next = 0
	}
	rt.enterLightStep(now, st, next, st.current)
}

// enterLightStep installs step idx: its hold deadline, then either a hard cut (Fade 0) or a cross-fade
// from the levels now on the lamps.
func (rt *Runtime) enterLightStep(now time.Time, st *lightShowState, idx int, from [2]hardware.LampLevels) {
	st.idx = idx
	st.advanceAt = now.Add(clampLightShowStep(st.steps[idx].Hold))
	if fade := clampLightFade(st.steps[idx].Fade); fade > 0 {
		st.stepStart = now
		st.fadeFrom = from
		st.fadeDur = fade
		st.current = from
		return
	}
	st.fadeDur = 0
	st.current = lightLevelsOf(st.steps[idx].Seats)
	rt.hwShadow.overrideLights(st.steps[idx].Seats)
}

// advanceLightFade recomputes the step's lamp levels for now and pushes them when a rounded level moved.
func (rt *Runtime) advanceLightFade(now time.Time, st *lightShowState) {
	if st.fadeDur == 0 {
		return
	}
	target := lightLevelsOf(st.steps[st.idx].Seats)
	elapsed := now.Sub(st.stepStart)
	if elapsed >= st.fadeDur {
		st.fadeDur = 0
		if st.current != target {
			st.current = target
			rt.hwShadow.overrideLightLevels(target)
		}
		return
	}
	next := lerpLampLevels(st.fadeFrom, target, float64(elapsed)/float64(st.fadeDur))
	if next != st.current {
		st.current = next
		rt.hwShadow.overrideLightLevels(next)
	}
}

// lightLevelsOf expands a step's seat masks into full-brightness levels.
func lightLevelsOf(seats [2]hardware.LightMask) [2]hardware.LampLevels {
	return [2]hardware.LampLevels{hardware.LampLevelsOf(seats[0]), hardware.LampLevelsOf(seats[1])}
}

// lerpLampLevels blends two seat pairs of levels linearly, per lamp, and
// rounds each to the nearest level.
func lerpLampLevels(from, to [2]hardware.LampLevels, frac float64) [2]hardware.LampLevels {
	var out [2]hardware.LampLevels
	for p := range out {
		for i := range out[p] {
			v := float64(from[p][i]) + (float64(to[p][i])-float64(from[p][i]))*frac
			out[p][i] = uint8(math.Round(v))
		}
	}
	return out
}

// endLightShow gives the lamps back and clears the state; a no-op when nothing is playing.
func (rt *Runtime) endLightShow() {
	if rt.lightShow == nil {
		return
	}
	rt.lightShow = nil
	// releaseLights replays the mirror, so buttons return to what the app believes is lit, not the last step.
	rt.hwShadow.releaseLights()
}
