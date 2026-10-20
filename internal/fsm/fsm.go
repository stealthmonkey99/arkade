// SPDX-License-Identifier: GPL-2.0-or-later

// Package fsm is a tiny finite state machine. States and events are strings;
// a dispatch may carry a data payload that rides along with the new state.
package fsm

import "fmt"

// Builder accumulates states and transitions before building a machine.
type Builder struct {
	transitions map[string]map[string]string
}

func NewBuilder() *Builder {
	return &Builder{transitions: map[string]map[string]string{}}
}

// AddTransition allows `event` to move the machine from `from` to `to`.
func (b *Builder) AddTransition(from, event, to string) *Builder {
	if b.transitions[from] == nil {
		b.transitions[from] = map[string]string{}
	}
	b.transitions[from][event] = to
	return b
}

// Build creates a machine positioned at the initial state.
func (b *Builder) Build(initial string) *FSM {
	return &FSM{transitions: b.transitions, state: initial}
}

// FSM is a running state machine.
type FSM struct {
	transitions map[string]map[string]string
	state       string
	data        any
}

// State returns the current state name.
func (m *FSM) State() string { return m.state }

// Data returns the payload attached to the most recent transition.
func (m *FSM) Data() any { return m.data }

// Can reports whether event is legal from the current state.
func (m *FSM) Can(event string) bool {
	_, ok := m.transitions[m.state][event]
	return ok
}

// Dispatch fires an event, optionally attaching a data payload.
func (m *FSM) Dispatch(event string, data ...any) error {
	next, ok := m.transitions[m.state][event]
	if !ok {
		return fmt.Errorf("fsm: no transition for event %q from state %q", event, m.state)
	}
	m.state = next
	if len(data) > 0 {
		m.data = data[0]
	} else {
		m.data = nil
	}
	return nil
}
