// SPDX-License-Identifier: GPL-2.0-or-later

// Package fsm is the guest-side screen-state machine: the state variable a
// cartridge already keeps, with the obligations that hang off every
// transition made structural. It does not dispatch: `switch machine.State()`
// stays in the cartridge, guards stay in the code that decides them, and
// nothing here routes input, updates a sim or draws.
//
// A [Machine] is a stack of [State]s at most [MaxDepth] deep; the top is what
// is on screen. It moves three ways, and the entry hook runs on all of them:
//
//	m.Go(s)    replace the top
//	m.Push(s)  layer over the top; the covered screen stays
//	m.Pop()    drop the top; the covered screen resumes
//
//	m.OnEnter(fn)  fn runs after every arrival, Go/Push/Pop/Reset alike
//
// The entry hook is the one place a screen asserts the peripherals it owns,
// including after a Pop back out from under an overlay.
//
//	const (
//	    stTitle fsm.State = iota
//	    stPlay
//	    stOver
//	    stConfirm
//	)
//
//	var machine fsm.Machine
//
//	func Init(s *cartlib.Setup) {
//	    machine.OnEnter(enterScreen)
//	    machine.Edges(legalEdges)
//	    machine.Reset(stTitle)
//	}
//
//	func enterScreen(prev, next fsm.State) {
//	    switch next {
//	    case stTitle:
//	        showTitlePeripherals()
//	    case stPlay:
//	        showPlayPeripherals()
//	    case stOver:
//	        showGameOverPeripherals()
//	    case stConfirm:
//	        showConfirmPeripherals()
//	    }
//	}
//
//	func update(dtMicros uint64) {
//	    machine.Tick(dtMicros)
//	    switch machine.State() {
//	    case stPlay:
//	        stepSim(dtMicros)
//	        if gameOver() {
//	            machine.Go(stOver)
//	        }
//	    case stOver:
//	        if machine.Elapsed() >= overHoldMicros {
//	            machine.Go(stTitle)
//	        }
//	    }
//	}
//
// A cartridge that adopts the package keeps no parallel state variable:
// [Machine.State] is the only copy. Run-wide modifiers that are not screens,
// such as a solo or versus mode, stay plain package variables.
//
// [Machine.Go] and [Machine.Push] take a constant destination, so a walk of
// the call sites recovers the complete edge set. Do not write
// `m.Go(nextScreenFor(x))`.
//
// The package obeys the TinyGo cartridge rules: no imports, no
// interfaces, no goroutines, no panics, and no allocation, including in
// [Machine.Reset]. A Machine is a value with no constructor, and hooks are
// plain top-level funcs. An illegal transition is refused and counted in
// [Machine.Blocked], never trapped, so a cartridge keeps drawing the screen
// it is on. A Machine costs 80 bytes on wasm32 and nothing per frame.
//
// Below roughly six states plus a modal, a plain uint8 and a switch is the
// better choice; nothing in the SDK requires this package.
package fsm

// State is a screen. A cartridge declares its own dense enum exactly as it
// already does, one constant per screen starting at 0:
//
//	const (
//	    stTitle fsm.State = iota
//	    stPlay
//	    ...
//	)
//
// State is a defined uint8, so an int variable cannot be passed as a screen.
type State uint8

// MaxDepth is how many screens the stack holds at once: one base screen
// plus MaxDepth-1 overlays. A Push past it is refused.
const MaxDepth = 4

// MaxStates is the largest state set an edge table can describe: one bit per
// destination in a uint16. A larger machine runs without a table.
const MaxStates = 16

// Machine holds one screen stack.
//
// The zero value is a usable machine at state 0 with no hooks and no edge
// table. Init should still call [Machine.Reset] so the initial screen's
// entry hook runs.
type Machine struct {
	stack [MaxDepth]State
	// per level: an overlay freezes the clock of the screen it covers
	clock [MaxDepth]uint64
	depth uint8 // live entries; 0 only before the first Reset/Go/Push
	enter func(prev, next State)
	exit  func(cur, next State)
	// edges[from] has bit `to` set when from->to is declared; nil is unchecked
	edges   []uint16
	blocked uint32
}

// OnEnter registers the hook that runs after every arrival: [Machine.Go],
// [Machine.Push], [Machine.Pop] and [Machine.Reset]. A screen re-asserts its
// LCD text and button lamps here, since nothing restores peripherals for it.
//
// prev is the screen departed, covered (Push) or dismissed (Pop). On the
// first Reset of a fresh machine prev == next.
//
// Call it once, in Init, with a plain top-level func; a closure allocates. A
// nil hook does nothing.
func (m *Machine) OnEnter(fn func(prev, next State)) { m.enter = fn }

// OnExit registers the hook that runs before a screen is left by
// [Machine.Go] or [Machine.Pop]. It does not run on [Machine.Push], since the
// covered screen is still on the stack, nor on [Machine.Reset]. Call it once,
// in Init, with a plain top-level func.
func (m *Machine) OnExit(fn func(cur, next State)) { m.exit = fn }

// Edges installs the legal-transition table: t[from] with bit `to` set for
// every declared from->to. It only validates; guards stay in the cartridge.
//
//	var legalEdges = []uint16{
//	    stTitle: 1 << stPlay,
//	    stPlay:  1<<stOver | 1<<stConfirm,
//	    stOver:  1 << stTitle,
//	}
//
// An indexed slice literal like that one allocates nothing. A nil table
// allows every transition. States at or above [MaxStates] are never declared.
func (m *Machine) Edges(t []uint16) { m.edges = t }

// Reset places the machine at s with an empty stack and fires the entry
// hook. Call it once, from Init. The edge table does not police it.
func (m *Machine) Reset(s State) {
	prev := s
	if m.depth > 0 {
		prev = m.stack[m.depth-1]
	}
	m.stack[0] = s
	m.clock[0] = 0
	m.depth = 1
	if m.enter != nil {
		m.enter(prev, s)
	}
}

// Go replaces the top of the stack with s: exit hook, clock zeroed, entry
// hook. s must be a constant.
//
// Returns false and does nothing if an edge table is installed and this edge
// is not in it; the refusal is counted in [Machine.Blocked].
func (m *Machine) Go(s State) bool {
	m.settle()
	cur := m.stack[m.depth-1]
	if !m.declared(cur, s) {
		m.blocked++
		return false
	}
	if m.exit != nil {
		m.exit(cur, s)
	}
	m.stack[m.depth-1] = s
	m.clock[m.depth-1] = 0
	if m.enter != nil {
		m.enter(cur, s)
	}
	return true
}

// Push layers s over the current screen without leaving it: the covered
// screen keeps its place on the stack and its clock stops, and [Machine.Pop]
// returns to both.
//
// Returns false, counted in [Machine.Blocked], past [MaxDepth] or on an
// undeclared edge. s must be a constant.
func (m *Machine) Push(s State) bool {
	m.settle()
	cur := m.stack[m.depth-1]
	if m.depth >= MaxDepth || !m.declared(cur, s) {
		m.blocked++
		return false
	}
	m.stack[m.depth] = s
	m.clock[m.depth] = 0
	m.depth++
	if m.enter != nil {
		m.enter(cur, s)
	}
	return true
}

// Pop leaves the top screen and re-enters the one beneath, running the entry
// hook so its peripherals are re-asserted.
//
// The resumed screen's clock continues from where the overlay froze it, so
// [Machine.Elapsed] reads as a resume while the hook fires as an arrival.
//
// Returns false, counted in [Machine.Blocked], at depth 1.
func (m *Machine) Pop() bool {
	m.settle()
	if m.depth == 1 {
		m.blocked++
		return false
	}
	cur := m.stack[m.depth-1]
	next := m.stack[m.depth-2]
	if m.exit != nil {
		m.exit(cur, next)
	}
	m.depth--
	if m.enter != nil {
		m.enter(cur, next)
	}
	return true
}

// State is the top of the stack: the screen to dispatch input, update and
// draw on.
func (m *Machine) State() State { return m.stack[m.live()-1] }

// Under is the screen beneath the top, or the top itself at depth 1.
func (m *Machine) Under() State {
	if d := m.live(); d > 1 {
		return m.stack[d-2]
	}
	return m.stack[0]
}

// In reports whether s is anywhere on the stack, covered screens included.
func (m *Machine) In(s State) bool {
	for i := uint8(0); i < m.live(); i++ {
		if m.stack[i] == s {
			return true
		}
	}
	return false
}

// Depth is the number of screens on the stack: 1 for a bare screen, 2 with
// one overlay.
func (m *Machine) Depth() uint8 { return m.live() }

// Tick adds dtMicros to the top screen's clock; covered screens stay frozen.
// Call it once per Update, before dispatching.
func (m *Machine) Tick(dtMicros uint64) {
	m.settle()
	m.clock[m.depth-1] += dtMicros
}

// Elapsed is the dtMicros accumulated since the top screen was entered. It is
// zeroed by [Machine.Go] and [Machine.Push] and continues across [Machine.Pop].
func (m *Machine) Elapsed() uint64 { return m.clock[m.live()-1] }

// Blocked is the number of refused operations: undeclared edges, pushes past
// [MaxDepth], and pops at depth 1. A host-side test can assert it is zero
// after a scripted journey.
func (m *Machine) Blocked() uint32 { return m.blocked }

// live reads the zero value (never Reset) as one entry at state 0.
func (m *Machine) live() uint8 {
	if m.depth == 0 {
		return 1
	}
	return m.depth
}

// settle makes the zero value's implicit single entry real before a write.
func (m *Machine) settle() {
	if m.depth == 0 {
		m.depth = 1
	}
}

// declared reports whether from->to is in the edge table; nil means unchecked.
func (m *Machine) declared(from, to State) bool {
	if m.edges == nil {
		return true
	}
	if int(from) >= len(m.edges) || to >= MaxStates {
		return false
	}
	return m.edges[from]&(1<<to) != 0
}
