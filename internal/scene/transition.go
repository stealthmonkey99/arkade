// SPDX-License-Identifier: GPL-2.0-or-later

// Package scene provides the scene state machine, scene manager and
// transition strategies. Time comes from the host's clock so
// transitions are deterministic under test.
package scene

import (
	"time"

	"github.com/stealthmonkey99/arkade/internal/display"
)

// Durations for the stock transition speeds.
const (
	DurInstant    = 0
	DurFast       = 500 * time.Millisecond
	DurMediumFast = 1000 * time.Millisecond
	DurMediumSlow = 3000 * time.Millisecond
	DurSlow       = 5000 * time.Millisecond
)

// Adjustment tells the scene how to render mid-transition.
type Adjustment struct {
	// Opacity applies to the scene's layers when HasOpacity is set.
	Opacity    float64
	HasOpacity bool
	// SuppressActivity skips the scene's update and render.
	SuppressActivity bool
	// Scrim, when set, fills the system scrim layer.
	Scrim    display.Pixel
	HasScrim bool
}

// Strategy computes per-frame adjustments for a running transition.
type Strategy interface {
	// Adjust computes the adjustment for the time elapsed since the transition began.
	Adjust(elapsed time.Duration, in bool) Adjustment
	// Duration is the total transition time.
	Duration() time.Duration
	// HideOnExit hides the scene's layers after transitioning out.
	HideOnExit() bool
}

func pct(elapsed, duration time.Duration) float64 {
	if duration <= 0 {
		return 1
	}
	p := float64(elapsed) / float64(duration)
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}

// JumpCut snaps between hidden and shown once the duration passes.
type JumpCut struct{ Dur time.Duration }

func (j JumpCut) Duration() time.Duration { return j.Dur }
func (j JumpCut) HideOnExit() bool        { return true }
func (j JumpCut) Adjust(elapsed time.Duration, in bool) Adjustment {
	done := pct(elapsed, j.Dur) >= 1
	opacity := 0.0
	if (done && in) || (!done && !in) {
		opacity = 1
	}
	return Adjustment{Opacity: opacity, HasOpacity: true, SuppressActivity: true}
}

// Overlay fades in over existing content; exiting stays opaque.
type Overlay struct{ Dur time.Duration }

func (o Overlay) Duration() time.Duration { return o.Dur }
func (o Overlay) HideOnExit() bool        { return false }
func (o Overlay) Adjust(elapsed time.Duration, in bool) Adjustment {
	opacity := 1.0
	if in {
		opacity = pct(elapsed, o.Dur)
	}
	return Adjustment{Opacity: opacity, HasOpacity: true}
}

// FadeThrough cross-fades: in ramps 0→1, out ramps 1→0.
type FadeThrough struct{ Dur time.Duration }

func (f FadeThrough) Duration() time.Duration { return f.Dur }
func (f FadeThrough) HideOnExit() bool        { return true }
func (f FadeThrough) Adjust(elapsed time.Duration, in bool) Adjustment {
	p := pct(elapsed, f.Dur)
	opacity := p
	if !in {
		opacity = 1 - p
	}
	return Adjustment{Opacity: opacity, HasOpacity: true}
}

// FadeToColor fades through a solid color using the system scrim.
type FadeToColor struct {
	Dur  time.Duration
	Fill display.Pixel
}

func (f FadeToColor) Duration() time.Duration { return f.Dur }
func (f FadeToColor) HideOnExit() bool        { return true }
func (f FadeToColor) Adjust(elapsed time.Duration, in bool) Adjustment {
	p := pct(elapsed, f.Dur)
	var opacity float64
	if in {
		opacity = max(0, (p-0.5)*2)
	} else {
		opacity = max(0, (1-p-0.5)*2)
	}
	scrim := f.Fill
	scrim.A = uint8((1 - opacity) * float64(scrim.A))
	layerOpacity := 1.0
	if opacity <= 0 {
		layerOpacity = 0
	}
	return Adjustment{
		Opacity:          layerOpacity,
		HasOpacity:       true,
		SuppressActivity: opacity <= 0,
		Scrim:            scrim,
		HasScrim:         true,
	}
}

// FadeFast is the default scene transition.
var FadeFast = FadeThrough{Dur: DurFast}
