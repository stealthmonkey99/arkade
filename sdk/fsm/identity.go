// SPDX-License-Identifier: GPL-2.0-or-later

package fsm

// IDMachine is [Machine] plus one small value: which member of an array of
// entities this instance is. [Machine.OnEnter]'s hook cannot tell which
// machine called it, so a hook shared by four jellies or a pool of cars needs
// an id.
//
// Give each entity its own IDMachine, record its id once, and register one
// OnEnterID hook shared by all of them:
//
//	type jelly struct {
//	    mode fsm.IDMachine
//	    fright bool
//	}
//	var jellies [4]jelly
//
//	var jellyEdges = []uint16{
//	    jellyChase:  1 << jellyFright,
//	    jellyFright: 1<<jellyChase | 1<<jellyEaten,
//	    jellyEaten:  1 << jellyChase,
//	}
//
//	func Init() {
//	    for i := range jellies {
//	        jellies[i].mode.SetID(uint8(i))
//	        jellies[i].mode.OnEnterID(enterJellyMode)
//	        jellies[i].mode.Edges(jellyEdges)
//	        jellies[i].mode.Reset(jellyChase)
//	    }
//	}
//
//	// enterJellyMode is shared by all four jellies; id says which.
//	func enterJellyMode(id uint8, prev, next fsm.State) {
//	    switch next {
//	    case jellyFright:
//	        jellies[id].fright = true
//	    case jellyEaten:
//	        jellies[id].fright = false
//	    }
//	}
//
// A lone screen machine does not need an id; use IDMachine only when one
// machine definition is shared by several instances at once.
//
// [IDMachine.Reset], [IDMachine.Go], [IDMachine.Push] and [IDMachine.Pop]
// behave like [Machine]'s, with the same refusal rules and [Machine.Blocked]
// accounting, and then also fire OnEnterID. The other [Machine] methods are
// promoted unchanged, including [Machine.OnEnter], which also still fires.
type IDMachine struct {
	Machine
	id      uint8
	enterID func(id uint8, prev, next State)
}

// SetID records which entity this instance is. Call it once, in Init, before
// the first Reset. The zero value's id is 0.
func (m *IDMachine) SetID(id uint8) { m.id = id }

// ID is the value [IDMachine.SetID] recorded, or 0 if it was never called.
func (m *IDMachine) ID() uint8 { return m.id }

// OnEnterID registers the hook that runs after every arrival, like
// [Machine.OnEnter] but also receiving the id [IDMachine.SetID] recorded.
// prev and next mean what they mean there.
//
// Call it once, in Init, with a plain top-level func; a closure allocates. A
// nil hook does nothing.
func (m *IDMachine) OnEnterID(fn func(id uint8, prev, next State)) { m.enterID = fn }

// Reset places the machine at s and fires OnEnterID. See [Machine.Reset].
func (m *IDMachine) Reset(s State) {
	prev := s
	if m.depth > 0 {
		prev = m.stack[m.depth-1]
	}
	m.Machine.Reset(s)
	if m.enterID != nil {
		m.enterID(m.id, prev, s)
	}
}

// Go replaces the top of the stack with s and fires OnEnterID. See
// [Machine.Go]; OnEnterID does not fire on a refusal.
func (m *IDMachine) Go(s State) bool {
	cur := m.State()
	if !m.Machine.Go(s) {
		return false
	}
	if m.enterID != nil {
		m.enterID(m.id, cur, s)
	}
	return true
}

// Push layers s over the current screen and fires OnEnterID. See
// [Machine.Push]; OnEnterID does not fire on a refusal.
func (m *IDMachine) Push(s State) bool {
	cur := m.State()
	if !m.Machine.Push(s) {
		return false
	}
	if m.enterID != nil {
		m.enterID(m.id, cur, s)
	}
	return true
}

// Pop leaves the top screen, re-enters the one beneath, and fires OnEnterID.
// See [Machine.Pop]; OnEnterID does not fire on a refusal.
func (m *IDMachine) Pop() bool {
	if m.Depth() < 2 {
		return m.Machine.Pop() // refused and counted, same as Machine's
	}
	cur := m.State()
	next := m.Under()
	if !m.Machine.Pop() {
		return false
	}
	if m.enterID != nil {
		m.enterID(m.id, cur, next)
	}
	return true
}
