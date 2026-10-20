// SPDX-License-Identifier: GPL-2.0-or-later

package scene

import (
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// State is the scene lifecycle state.
type State string

const (
	Initializing     State = "initializing"
	TransitioningIn  State = "transitioning-in"
	Active           State = "active"
	TransitioningOut State = "transitioning-out"
	Inactive         State = "inactive"
)

// RenderReason says why a scene is being rendered.
type RenderReason string

const (
	ReasonInit RenderReason = "init"
	ReasonTick RenderReason = "tick"
)

// Host is what a scene needs from its app: layer management scoped to the
// app's owner, audio helpers and time.
type Host interface {
	AddLayer(label string, elements []display.Element, opacity float64)
	RemoveLayer(label string)
	SetLayerAlpha(label string, value float64)
	AddElements(label string, elements ...display.Element)
	RemoveElements(label string, elements ...display.Element)
	SetScrim(p display.Pixel)
	PlaySound(file string)
	PlayMusic(file string)
	StopMusic()
	Log(msg string)
	Now() time.Time
}

// Impl is the game-specific part of a scene.
type Impl interface {
	Init() error
	// Enter runs when the scene starts transitioning in (enterScene).
	Enter()
	// HandleInput processes one event; returns true when consumed.
	HandleInput(stats clock.FrameTime, ev input.Event) bool
	Update(stats clock.FrameTime)
	// Render refreshes elements before the display composites.
	Render(stats clock.FrameTime, reason RenderReason)
	Teardown()
}

// Options configures a scene's transitions.
type Options struct {
	TransitionIn  Strategy
	TransitionOut Strategy
}

// Scene wraps an Impl with the lifecycle state machine and transition
// bookkeeping.
type Scene struct {
	Label string
	host  Host
	impl  Impl

	state       State
	inStrategy  Strategy
	outStrategy Strategy
	layerLabels []string
	transStart  time.Time
}

// NewScene wraps impl; fadeFast transitions by default.
func NewScene(label string, host Host, impl Impl, opts Options) *Scene {
	s := &Scene{Label: label, host: host, impl: impl, state: Initializing}
	s.inStrategy = opts.TransitionIn
	if s.inStrategy == nil {
		s.inStrategy = FadeFast
	}
	s.outStrategy = opts.TransitionOut
	if s.outStrategy == nil {
		s.outStrategy = FadeFast
	}
	return s
}

// State returns the scene's lifecycle state.
func (s *Scene) State() State { return s.state }

// IsTransitioning reports whether the scene is mid-transition.
func (s *Scene) IsTransitioning() bool {
	return s.state == TransitioningIn || s.state == TransitioningOut
}

// AddLayer adds a layer through the host and tracks it, so the scene's
// transitions fade it.
func (s *Scene) AddLayer(label string, elements []display.Element, opacity float64) {
	s.host.AddLayer(label, elements, opacity)
	s.TrackLayer(label)
}

// TrackLayer registers a layer the host already holds so transitions can
// adjust its alpha.
func (s *Scene) TrackLayer(label string) {
	s.layerLabels = append(s.layerLabels, label)
}

func (s *Scene) setLayerAlphas(v float64) {
	for _, label := range s.layerLabels {
		s.host.SetLayerAlpha(label, v)
	}
}

// TransitionTo begins showing the scene.
func (s *Scene) TransitionTo() {
	switch s.state {
	case Initializing, Inactive, TransitioningOut:
		s.state = TransitioningIn
		s.transStart = s.host.Now()
		s.impl.Enter()
	}
}

// TransitionAway begins hiding the scene.
func (s *Scene) TransitionAway() {
	switch s.state {
	case Active, TransitioningIn:
		s.state = TransitioningOut
		s.transStart = s.host.Now()
	}
}

func (s *Scene) strategy() (Strategy, bool) {
	if s.state == TransitioningOut {
		return s.outStrategy, false
	}
	return s.inStrategy, true
}

// finishTransition completes the in-flight transition if its time is up.
func (s *Scene) finishTransition() {
	strat, in := s.strategy()
	if s.host.Now().Sub(s.transStart) < strat.Duration() {
		return
	}
	if in {
		s.state = Active
		s.setLayerAlphas(1)
	} else {
		s.state = Inactive
		if strat.HideOnExit() {
			s.setLayerAlphas(0)
		}
	}
}

// HandleInput forwards to the impl only while active.
func (s *Scene) HandleInput(stats clock.FrameTime, ev input.Event) bool {
	if s.state != Active {
		return false
	}
	return s.impl.HandleInput(stats, ev)
}

// Update advances transitions and scene logic.
func (s *Scene) Update(stats clock.FrameTime) {
	switch s.state {
	case Active:
		s.impl.Update(stats)
	case TransitioningIn, TransitioningOut:
		strat, in := s.strategy()
		adj := strat.Adjust(s.host.Now().Sub(s.transStart), in)
		if adj.SuppressActivity {
			return
		}
		s.impl.Update(stats)
	}
}

// Render draws the scene, applying transition adjustments.
func (s *Scene) Render(stats clock.FrameTime, reason RenderReason) {
	switch s.state {
	case Active:
		s.impl.Render(stats, reason)
	case TransitioningIn, TransitioningOut:
		strat, in := s.strategy()
		adj := strat.Adjust(s.host.Now().Sub(s.transStart), in)
		if !adj.SuppressActivity {
			s.impl.Render(stats, reason)
		}
		if adj.HasOpacity {
			s.setLayerAlphas(adj.Opacity)
		}
		if adj.HasScrim {
			s.host.SetScrim(adj.Scrim)
		}
		s.finishTransition()
	}
}

// Teardown stops music and tears down the impl.
func (s *Scene) Teardown() {
	s.host.StopMusic()
	s.impl.Teardown()
}
