// SPDX-License-Identifier: GPL-2.0-or-later

package input

import (
	"strings"
	"testing"
)

func TestMapKeyDirectionLeftController(t *testing.T) {
	got := MapKeyDirection(PositionLeft, Upright, KeyUp)
	if got != KeyLeft {
		t.Errorf("left controller up → left, got %s", got)
	}
	got = MapKeyDirection(PositionRight, Upright, KeyUp)
	if got != KeyRight {
		t.Errorf("right controller up → right, got %s", got)
	}
}

func TestMapKeyDirectionNonDirectionalPassthrough(t *testing.T) {
	if got := MapKeyDirection(PositionLeft, Upright, KeyJoyA); got != KeyJoyA {
		t.Errorf("non-directional key must pass through, got %s", got)
	}
}

func TestMapKeyDirectionComposesWithRotation(t *testing.T) {
	// The seat the display is rotated toward reads its stick unchanged: left
	// faces CounterClockwise, right faces Clockwise.
	if got := MapKeyDirection(PositionLeft, CounterClockwise, KeyUp); got != KeyUp {
		t.Errorf("faced left seat should be identity under CCW, got %s", got)
	}
	if got := MapKeyDirection(PositionRight, Clockwise, KeyUp); got != KeyUp {
		t.Errorf("faced right seat should be identity under CW, got %s", got)
	}
	// The opposite seat needs the 180° compensation.
	if got := MapKeyDirection(PositionRight, CounterClockwise, KeyUp); got != KeyDown {
		t.Errorf("across-the-table right seat under CCW: up → down, got %s", got)
	}
	if got := MapKeyDirection(PositionLeft, Clockwise, KeyUp); got != KeyDown {
		t.Errorf("across-the-table left seat under CW: up → down, got %s", got)
	}
	if got := MapKeyDirection(PositionUnknown, Clockwise, KeyUp); got != KeyUp {
		t.Errorf("unknown position must pass through, got %s", got)
	}
}

func TestMapJoyDirectionComposesWithRotation(t *testing.T) {
	if got := MapJoyDirection(PositionLeft, CounterClockwise, JoyUp); got != JoyUp {
		t.Errorf("faced left seat should be identity under CCW, got %s", got)
	}
	if got := MapJoyDirection(PositionRight, CounterClockwise, JoyUp); got != JoyDown {
		t.Errorf("across-the-table right seat under CCW: up → down, got %s", got)
	}
}

func TestMapJoyDirectionTwoStepsAroundRing(t *testing.T) {
	if got := MapJoyDirection(PositionRight, Upright, JoyUp); got != JoyRight {
		t.Errorf("right controller joystick up → right, got %s", got)
	}
	if got := MapJoyDirection(PositionLeft, Upright, JoyUp); got != JoyLeft {
		t.Errorf("left controller joystick up → left, got %s", got)
	}
}

func TestHandlerStackTopWins(t *testing.T) {
	h := NewHandler()
	var order []string
	bottom := []*Mapping{{
		Conditions: []Condition{Key(KeyEnter)},
		Trigger:    func(Event) { order = append(order, "bottom") },
	}}
	top := []*Mapping{{
		Conditions: []Condition{Key(KeyEnter)},
		Trigger:    func(Event) { order = append(order, "top") },
	}}
	h.AddMappings(bottom)
	h.AddMappings(top)

	h.Dispatch(Event{Kind: KindKeypress, Name: KeyEnter})
	if len(order) != 1 || order[0] != "top" {
		t.Errorf("top mapping should win without bubbling, got %v", order)
	}
}

func TestHandlerBubbles(t *testing.T) {
	h := NewHandler()
	var order []string
	bottom := []*Mapping{{
		Conditions: []Condition{Key(KeyEnter)},
		Trigger:    func(Event) { order = append(order, "bottom") },
	}}
	top := []*Mapping{{
		Conditions: []Condition{Key(KeyEnter)},
		Trigger:    func(Event) { order = append(order, "top") },
		Bubbles:    true,
	}}
	h.AddMappings(bottom)
	h.AddMappings(top)

	h.Dispatch(Event{Kind: KindKeypress, Name: KeyEnter})
	if len(order) != 2 || order[0] != "top" || order[1] != "bottom" {
		t.Errorf("bubbling should reach lower mappings, got %v", order)
	}
}

func TestHandlerQueueCollects(t *testing.T) {
	h := NewHandler()
	var q Queue
	h.AddMappings([]*Mapping{{
		Conditions: []Condition{Key(KeyUp), Key(KeyDown)},
		Queue:      &q,
	}})
	h.Dispatch(Event{Kind: KindKeypress, Name: KeyUp})
	h.Dispatch(Event{Kind: KindKeypress, Name: KeyDown})
	h.Dispatch(Event{Kind: KindKeypress, Name: KeyLeft}) // unmatched

	events := q.Drain()
	if len(events) != 2 {
		t.Fatalf("expected 2 queued events, got %d", len(events))
	}
	if events[0].Name != KeyUp || events[1].Name != KeyDown {
		t.Errorf("queue order wrong: %v", events)
	}
	if q.Len() != 0 {
		t.Error("drain should clear the queue")
	}
}

func TestHandlerRemoveMappings(t *testing.T) {
	h := NewHandler()
	hits := 0
	set := []*Mapping{{
		Conditions: []Condition{Key(KeyEnter)},
		Trigger:    func(Event) { hits++ },
	}}
	handle := h.AddMappings(set)
	h.Dispatch(Event{Kind: KindKeypress, Name: KeyEnter})
	h.RemoveMappings(handle)
	h.Dispatch(Event{Kind: KindKeypress, Name: KeyEnter})
	if hits != 1 {
		t.Errorf("expected 1 hit after removal, got %d", hits)
	}
}

func TestRemoveMappingsTakesOffOnlyItsOwnPush(t *testing.T) {
	h := NewHandler()
	var hits []string
	keep := h.AddMappings([]*Mapping{{
		Conditions: []Condition{Key(KeyEnter)},
		Trigger:    func(Event) { hits = append(hits, "keep") },
	}})
	emptyA := h.AddMappings(nil)
	h.AddMappings([]*Mapping{})
	shared := &Mapping{Conditions: []Condition{Key(KeyUp)}, Trigger: func(Event) { hits = append(hits, "shared") }}
	first := h.AddMappings([]*Mapping{shared})
	h.AddMappings([]*Mapping{shared})

	h.RemoveMappings(emptyA)
	h.RemoveMappings(emptyA)
	h.RemoveMappings(first)
	h.Dispatch(Event{Kind: KindKeypress, Name: KeyEnter})
	h.Dispatch(Event{Kind: KindKeypress, Name: KeyUp})
	if len(h.mappings) != 3 {
		t.Errorf("stack has %d sets, want 3 (keep, the other empty set, the second shared push)", len(h.mappings))
	}
	if got := strings.Join(hits, ","); got != "keep,shared" {
		t.Errorf("hits %q, want keep,shared", got)
	}
	h.RemoveMappings(keep)
	h.Dispatch(Event{Kind: KindKeypress, Name: KeyEnter})
	if len(hits) != 2 {
		t.Errorf("removed set still fired: %v", hits)
	}
}

func TestConditionShiftMatch(t *testing.T) {
	shift := true
	cond := Condition{Name: ptrKey(KeyRotate), Shift: &shift}
	if cond.Matches(Event{Kind: KindKeypress, Name: KeyRotate}) {
		t.Error("shift condition should not match unshifted event")
	}
	if !cond.Matches(Event{Kind: KindKeypress, Name: KeyRotate, Shift: true}) {
		t.Error("shift condition should match shifted event")
	}
}

func ptrKey(k KeyName) *KeyName { return &k }
