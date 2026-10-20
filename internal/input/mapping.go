// SPDX-License-Identifier: GPL-2.0-or-later

package input

import "sync"

// Condition is a partial match against an Event. Nil fields match anything.
type Condition struct {
	Kind     *EventKind
	Name     *KeyName
	Position *JoystickPosition
	Ctrl     *bool
	Shift    *bool
}

func Key(name KeyName) Condition { return Condition{Name: &name} }

func KeyShift(name KeyName, shift bool) Condition { return Condition{Name: &name, Shift: &shift} }

func Joy(pos JoystickPosition) Condition {
	kind := KindJoystickChange
	return Condition{Kind: &kind, Position: &pos}
}

// Matches reports whether the condition matches ev. An unset Kind defaults
// to keypress.
func (c Condition) Matches(ev Event) bool {
	kind := KindKeypress
	if c.Kind != nil {
		kind = *c.Kind
	}
	if ev.Kind != kind {
		return false
	}
	if c.Name != nil && ev.Name != *c.Name {
		return false
	}
	if c.Position != nil && ev.Position != *c.Position {
		return false
	}
	if c.Ctrl != nil && ev.Ctrl != *c.Ctrl {
		return false
	}
	if c.Shift != nil && ev.Shift != *c.Shift {
		return false
	}
	return true
}

// Mapping routes matching events to a trigger and/or a queue.
type Mapping struct {
	// Conditions: the event matches if ANY condition matches.
	Conditions []Condition
	// Trigger runs when a condition matches.
	Trigger func(ev Event)
	// Queue collects matching events for the app's HandleInput phase.
	Queue *Queue
	// Bubbles lets the event continue to lower mapping sets (default false).
	Bubbles bool
}

// Queue is a simple FIFO of events shared between the dispatcher and apps.
type Queue struct {
	mu     sync.Mutex
	events []Event
}

func (q *Queue) Push(ev Event) {
	q.mu.Lock()
	q.events = append(q.events, ev)
	q.mu.Unlock()
}

func (q *Queue) Drain() []Event {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := q.events
	q.events = nil
	return out
}

func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.events)
}

// Handler owns the stack of mapping sets. New sets go on top and see events
// first; a match without Bubbles stops propagation.
type Handler struct {
	mu       sync.Mutex
	mappings []*MappingSet
	ch       chan Event
	spies    []func()
}

func NewHandler() *Handler {
	return &Handler{ch: make(chan Event, 256)}
}

func (h *Handler) Events() chan<- Event { return h.ch }

// Push enqueues an event (non-blocking; drops if the buffer is full).
func (h *Handler) Push(ev Event) {
	select {
	case h.ch <- ev:
	default:
	}
}

func (h *Handler) Drain() []Event {
	var out []Event
	for {
		select {
		case ev := <-h.ch:
			out = append(out, ev)
		default:
			return out
		}
	}
}

// MappingSet is one entry on the handler's stack, as returned by AddMappings.
type MappingSet struct {
	mappings []*Mapping
}

// AddMappings pushes mappings on top of the stack and returns the handle
// that removes exactly this push.
func (h *Handler) AddMappings(mappings []*Mapping) *MappingSet {
	set := &MappingSet{mappings: mappings}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mappings = append([]*MappingSet{set}, h.mappings...)
	return set
}

// RemoveMappings takes a set off the stack; removing it twice is a no-op.
func (h *Handler) RemoveMappings(set *MappingSet) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, existing := range h.mappings {
		if existing == set {
			h.mappings = append(h.mappings[:i], h.mappings[i+1:]...)
			return
		}
	}
}

func (h *Handler) AddSpy(fn func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.spies = append(h.spies, fn)
}

// Dispatch walks the mapping stack top-down. Returns true if any mapping
// handled the event.
func (h *Handler) Dispatch(ev Event) bool {
	h.mu.Lock()
	sets := make([]*MappingSet, len(h.mappings))
	copy(sets, h.mappings)
	spies := append([]func(){}, h.spies...)
	h.mu.Unlock()

	for _, spy := range spies {
		spy()
	}

	handled := false
	for _, set := range sets {
		for _, m := range set.mappings {
			matched := false
			for _, cond := range m.Conditions {
				if cond.Matches(ev) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			handled = true
			if m.Trigger != nil {
				m.Trigger(ev)
			}
			if m.Queue != nil {
				m.Queue.Push(ev)
			}
			if !m.Bubbles {
				return true
			}
		}
	}
	return handled
}
