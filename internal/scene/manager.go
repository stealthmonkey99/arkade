// SPDX-License-Identifier: GPL-2.0-or-later

package scene

import (
	"fmt"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/fsm"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// Manager routes lifecycle calls to registered scenes and drives scene
// switching through an app-supplied FSM whose state names match scene
// labels.
type Manager struct {
	host      Host
	machine   *fsm.FSM
	scenes    map[string]*Scene
	order     []string
	lastScene *Scene
}

// NewManager builds a manager around an app's state machine.
func NewManager(host Host, machine *fsm.FSM) *Manager {
	return &Manager{host: host, machine: machine, scenes: map[string]*Scene{}}
}

// Add runs impl's Init and, only if it succeeds, registers the scene under
// label (usually an FSM state name).
func (m *Manager) Add(label string, impl Impl, opts Options) (*Scene, error) {
	if _, exists := m.scenes[label]; exists {
		return nil, fmt.Errorf("duplicate scene %q", label)
	}
	if err := impl.Init(); err != nil {
		return nil, fmt.Errorf("scene %q init: %w", label, err)
	}
	s := NewScene(label, m.host, impl, opts)
	m.scenes[label] = s
	m.order = append(m.order, label)
	return s, nil
}

// State returns the FSM's current state name.
func (m *Manager) State() string { return m.machine.State() }

// Data returns the payload attached to the last FSM transition.
func (m *Manager) Data() any { return m.machine.Data() }

// Current returns the scene for the FSM state (or the last active scene if
// the state has no scene).
func (m *Manager) Current() *Scene {
	if s, ok := m.scenes[m.machine.State()]; ok {
		m.lastScene = s
	}
	return m.lastScene
}

// Dispatch fires an FSM event without scene transitions.
func (m *Manager) Dispatch(event string, data ...any) error {
	return m.machine.Dispatch(event, data...)
}

// Transition fires an FSM event, transitioning away from the current scene
// and into the new one.
func (m *Manager) Transition(event string, data ...any) error {
	if cur := m.Current(); cur != nil {
		cur.TransitionAway()
	}
	if err := m.machine.Dispatch(event, data...); err != nil {
		return err
	}
	if next := m.Current(); next != nil {
		next.TransitionTo()
	}
	return nil
}

// HandleInput offers an event to every scene (active scenes consume it).
// Returns true if any scene consumed the event.
func (m *Manager) HandleInput(stats clock.FrameTime, ev input.Event) bool {
	handled := false
	for _, label := range m.order {
		if m.scenes[label].HandleInput(stats, ev) {
			handled = true
		}
	}
	return handled
}

// Update advances every scene.
func (m *Manager) Update(stats clock.FrameTime) {
	for _, label := range m.order {
		m.scenes[label].Update(stats)
	}
}

// Render draws every scene.
func (m *Manager) Render(stats clock.FrameTime, reason RenderReason) {
	for _, label := range m.order {
		m.scenes[label].Render(stats, reason)
	}
}

// Teardown tears down all scenes.
func (m *Manager) Teardown() {
	for _, label := range m.order {
		m.scenes[label].Teardown()
	}
	m.scenes = map[string]*Scene{}
	m.order = nil
}
