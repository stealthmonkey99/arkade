// SPDX-License-Identifier: GPL-2.0-or-later

package fsm

import "testing"

// The synthetic cartridge: a game, a pause menu over it, an options list over
// that, and a confirm over the options list, which fills MaxDepth.
const (
	stPlay State = iota
	stPause
	stOptions
	stResetConfirm
	stTitle
)

func stName(s State) string {
	switch s {
	case stPlay:
		return "play"
	case stPause:
		return "pause"
	case stOptions:
		return "options"
	case stResetConfirm:
		return "reset"
	case stTitle:
		return "title"
	}
	return "?"
}

// trace is package-level so the hooks stay plain top-level funcs.
var trace []string

func recEnter(prev, next State) { trace = append(trace, "enter "+stName(prev)+">"+stName(next)) }
func recExit(cur, next State)   { trace = append(trace, "exit "+stName(cur)+">"+stName(next)) }

func traced() *Machine {
	trace = trace[:0]
	m := &Machine{}
	m.OnEnter(recEnter)
	m.OnExit(recExit)
	return m
}

func wantTrace(t *testing.T, want ...string) {
	t.Helper()
	if len(trace) != len(want) {
		t.Fatalf("hook calls = %v, want %v", trace, want)
	}
	for i := range want {
		if trace[i] != want[i] {
			t.Fatalf("hook calls = %v, want %v", trace, want)
		}
	}
}

func TestZeroValueIsAUsableMachineAtStateZero(t *testing.T) {
	var m Machine
	if m.depth != 0 {
		t.Fatalf("a virgin machine's depth = %d, want 0", m.depth)
	}
	if got := m.State(); got != stPlay {
		t.Fatalf("State() = %v, want state 0", got)
	}
	if got := m.Under(); got != stPlay {
		t.Fatalf("Under() = %v, want state 0", got)
	}
	if m.Depth() != 1 {
		t.Fatalf("Depth() = %d, want 1 — the implicit single entry", m.Depth())
	}
	if !m.In(stPlay) {
		t.Fatal("In(state 0) is false on a virgin machine")
	}
	if m.Elapsed() != 0 || m.Blocked() != 0 {
		t.Fatalf("Elapsed()=%d Blocked()=%d, want 0/0", m.Elapsed(), m.Blocked())
	}
	m.Tick(100)
	if m.Elapsed() != 100 {
		t.Fatalf("Elapsed() after Tick on a virgin machine = %d, want 100", m.Elapsed())
	}
}

func TestNilHooksAreLegal(t *testing.T) {
	var m Machine
	m.Reset(stTitle)
	m.Go(stPlay)
	m.Push(stPause)
	m.Pop()
	if m.State() != stPlay {
		t.Fatalf("State() = %v, want play", m.State())
	}
}

func TestResetFiresTheEntryHookWithPrevEqualToNext(t *testing.T) {
	m := traced()
	m.Reset(stTitle)
	wantTrace(t, "enter title>title")
	if m.State() != stTitle || m.Depth() != 1 {
		t.Fatalf("State()=%v Depth()=%d, want title/1", m.State(), m.Depth())
	}
}

func TestResetCollapsesAStackAndIgnoresTheEdgeTable(t *testing.T) {
	m := traced()
	m.Edges([]uint16{stPlay: 1 << stPause})
	m.Reset(stPlay)
	m.Push(stPause)
	trace = trace[:0]
	// title is not in the table; Reset is a placement, not a transition
	m.Reset(stTitle)
	wantTrace(t, "enter pause>title")
	if m.Depth() != 1 || m.State() != stTitle {
		t.Fatalf("Depth()=%d State()=%v after Reset, want 1/title", m.Depth(), m.State())
	}
	if m.Blocked() != 0 {
		t.Fatalf("Blocked() = %d after a Reset past the edge table, want 0", m.Blocked())
	}
}

func TestGoExitsThenEntersAndZeroesTheClock(t *testing.T) {
	m := traced()
	m.Reset(stTitle)
	m.Tick(500)
	trace = trace[:0]

	if !m.Go(stPlay) {
		t.Fatal("Go(play) refused with no edge table installed")
	}
	wantTrace(t, "exit title>play", "enter title>play")
	if m.State() != stPlay {
		t.Fatalf("State() = %v, want play", m.State())
	}
	if m.Elapsed() != 0 {
		t.Fatalf("Elapsed() after Go = %d, want 0 — a fresh screen starts a fresh clock", m.Elapsed())
	}
	if m.Depth() != 1 {
		t.Fatalf("Depth() after Go = %d, want 1 — Go replaces, it does not stack", m.Depth())
	}
}

func TestGoReplacesTheTopOfAStackWithoutDisturbingWhatIsUnder(t *testing.T) {
	m := traced()
	m.Reset(stPlay)
	m.Push(stPause)
	m.Go(stOptions)
	if m.State() != stOptions {
		t.Fatalf("State() = %v, want options", m.State())
	}
	if m.Under() != stPlay {
		t.Fatalf("Under() = %v, want play", m.Under())
	}
	if m.Depth() != 2 {
		t.Fatalf("Depth() = %d, want 2", m.Depth())
	}
	if m.In(stPause) {
		t.Fatal("In(pause) is still true after Go replaced it")
	}
}

func TestPushCoversTheScreenWithoutLeavingIt(t *testing.T) {
	m := traced()
	m.Reset(stPlay)
	m.Tick(500)
	trace = trace[:0]

	if !m.Push(stPause) {
		t.Fatal("Push(pause) refused")
	}
	// play is covered, not left
	wantTrace(t, "enter play>pause")
	if m.State() != stPause || m.Under() != stPlay || m.Depth() != 2 {
		t.Fatalf("State()=%v Under()=%v Depth()=%d, want pause/play/2", m.State(), m.Under(), m.Depth())
	}
	if !m.In(stPlay) {
		t.Fatal("In(play) is false under a modal — a frozen sim must know it is only frozen")
	}
	if m.Elapsed() != 0 {
		t.Fatalf("the modal's own Elapsed() = %d, want 0", m.Elapsed())
	}
}

func TestPopReEntersTheCoveredScreenAndResumesItsClock(t *testing.T) {
	m := traced()
	m.Reset(stPlay)
	m.Tick(500)
	m.Push(stPause)
	m.Tick(9_000) // time spent in the modal
	trace = trace[:0]

	if !m.Pop() {
		t.Fatal("Pop refused at depth 2")
	}
	// the covered screen re-enters
	wantTrace(t, "exit pause>play", "enter pause>play")
	if m.State() != stPlay || m.Depth() != 1 {
		t.Fatalf("State()=%v Depth()=%d, want play/1", m.State(), m.Depth())
	}
	if m.Elapsed() != 500 {
		t.Fatalf("Elapsed() after Pop = %d, want 500 — the overlay froze the clock, it did not destroy it", m.Elapsed())
	}
}

func TestTickFeedsOnlyTheTopOfTheStack(t *testing.T) {
	var m Machine
	m.Reset(stPlay)
	m.Tick(100)
	m.Push(stPause)
	m.Tick(70)
	m.Push(stOptions)
	m.Tick(3)
	if m.Elapsed() != 3 {
		t.Fatalf("options Elapsed() = %d, want 3", m.Elapsed())
	}
	m.Pop()
	if m.Elapsed() != 70 {
		t.Fatalf("pause Elapsed() after Pop = %d, want 70", m.Elapsed())
	}
	m.Pop()
	if m.Elapsed() != 100 {
		t.Fatalf("play Elapsed() after Pop = %d, want 100", m.Elapsed())
	}
}

// TestFourDeepModalStack fills the stack to MaxDepth: the base screen plus
// three overlays, so the fifth push overflows.
func TestFourDeepModalStack(t *testing.T) {
	m := traced()
	m.Reset(stPlay)
	m.Tick(1_000)

	if !m.Push(stPause) {
		t.Fatal("Push(pause) over the game refused")
	}
	m.Tick(20)
	if !m.Push(stOptions) {
		t.Fatal("Push(options) over the pause menu refused")
	}
	m.Tick(3)
	trace = trace[:0]
	if !m.Push(stResetConfirm) {
		t.Fatal("Push(reset) over the options list refused — this is the chain MaxDepth 4 exists for")
	}
	wantTrace(t, "enter options>reset")
	m.Tick(7)

	if m.Depth() != MaxDepth {
		t.Fatalf("Depth() = %d, want the full %d", m.Depth(), MaxDepth)
	}
	if m.State() != stResetConfirm || m.Under() != stOptions {
		t.Fatalf("State()=%v Under()=%v, want reset/options", m.State(), m.Under())
	}
	if !m.In(stPlay) || !m.In(stPause) || !m.In(stOptions) || !m.In(stResetConfirm) {
		t.Fatal("In() lost one of the four live screens — the game underneath must still know it is only frozen")
	}

	// a fifth level is refused and counted
	trace = trace[:0]
	if m.Push(stTitle) {
		t.Fatal("Push past MaxDepth succeeded")
	}
	wantTrace(t)
	if m.State() != stResetConfirm || m.Depth() != MaxDepth {
		t.Fatalf("a refused Push moved the machine: State()=%v Depth()=%d", m.State(), m.Depth())
	}
	if m.Blocked() != 1 {
		t.Fatalf("Blocked() = %d after one overflow, want 1", m.Blocked())
	}

	// Go replaces rather than stacks, so it works on a full stack
	trace = trace[:0]
	if !m.Go(stTitle) {
		t.Fatal("Go at the top of a full stack refused")
	}
	wantTrace(t, "exit reset>title", "enter reset>title")
	if m.Depth() != MaxDepth || m.Under() != stOptions {
		t.Fatalf("Depth()=%d Under()=%v, want %d/options", m.Depth(), m.Under(), MaxDepth)
	}

	// unwinding re-enters every level in order with its own clock intact
	trace = trace[:0]
	m.Pop()
	if m.State() != stOptions || m.Elapsed() != 3 {
		t.Fatalf("after one Pop State()=%v Elapsed()=%d, want options/3", m.State(), m.Elapsed())
	}
	m.Pop()
	if m.State() != stPause || m.Elapsed() != 20 {
		t.Fatalf("after two Pops State()=%v Elapsed()=%d, want pause/20", m.State(), m.Elapsed())
	}
	m.Pop()
	if m.State() != stPlay || m.Elapsed() != 1_000 {
		t.Fatalf("after three Pops State()=%v Elapsed()=%d, want play/1000", m.State(), m.Elapsed())
	}
	wantTrace(t,
		"exit title>options", "enter title>options",
		"exit options>pause", "enter options>pause",
		"exit pause>play", "enter pause>play")
	if m.Depth() != 1 {
		t.Fatalf("Depth() = %d after unwinding, want 1", m.Depth())
	}
}

func TestPopAtDepthOneIsRefusedAndCounted(t *testing.T) {
	m := traced()
	m.Reset(stPlay)
	trace = trace[:0]
	if m.Pop() {
		t.Fatal("Pop at depth 1 succeeded — there is nothing under the base screen")
	}
	wantTrace(t)
	if m.State() != stPlay {
		t.Fatalf("State() = %v after a refused Pop, want play", m.State())
	}
	if m.Blocked() != 1 {
		t.Fatalf("Blocked() = %d, want 1 — a stray Pop is a bug worth counting, not a silent no-op", m.Blocked())
	}
}

var exampleEdges = []uint16{
	stPlay:         1<<stPause | 1<<stTitle,
	stPause:        1<<stOptions | 1<<stTitle,
	stOptions:      1 << stResetConfirm,
	stResetConfirm: 1 << stTitle,
	stTitle:        1 << stPlay,
}

func TestEdgeTableRefusesAnUndeclaredGo(t *testing.T) {
	m := traced()
	m.Edges(exampleEdges)
	m.Reset(stPlay)
	trace = trace[:0]

	if m.Go(stOptions) { // play->options is not in the table
		t.Fatal("an undeclared Go succeeded")
	}
	wantTrace(t)
	if m.State() != stPlay {
		t.Fatalf("State() = %v after a refused Go, want play — a refusal never trapped and never moved", m.State())
	}
	if m.Blocked() != 1 {
		t.Fatalf("Blocked() = %d, want 1", m.Blocked())
	}
	if !m.Go(stTitle) {
		t.Fatal("a DECLARED Go was refused")
	}
}

func TestEdgeTablePolicesPushToo(t *testing.T) {
	m := traced()
	m.Edges(exampleEdges)
	m.Reset(stPlay)
	if m.Push(stResetConfirm) { // play->reset is not declared
		t.Fatal("an undeclared Push succeeded")
	}
	if !m.Push(stPause) {
		t.Fatal("a declared Push was refused")
	}
	if m.Blocked() != 1 {
		t.Fatalf("Blocked() = %d, want 1", m.Blocked())
	}
}

func TestEdgeTableDoesNotPolicePop(t *testing.T) {
	// pause->play is not in exampleEdges: Pop's destination is the stack
	m := traced()
	m.Edges(exampleEdges)
	m.Reset(stPlay)
	m.Push(stPause)
	if !m.Pop() {
		t.Fatal("Pop was refused by the edge table — its destination is the stack, not an edge")
	}
	if m.Blocked() != 0 {
		t.Fatalf("Blocked() = %d, want 0", m.Blocked())
	}
}

func TestAStateOffTheEndOfTheTableIsUndeclared(t *testing.T) {
	m := &Machine{}
	m.Edges([]uint16{stPlay: 1 << stPause}) // len 2: nothing declared for pause onward
	m.Reset(stPause)
	if m.Go(stPlay) {
		t.Fatal("a transition from a state past the end of the table succeeded")
	}
	if m.Blocked() != 1 {
		t.Fatalf("Blocked() = %d, want 1", m.Blocked())
	}
}

func TestANilTableAllowsEverything(t *testing.T) {
	var m Machine
	m.Reset(stPlay)
	if !m.Go(stResetConfirm) || !m.Push(stTitle) {
		t.Fatal("a machine with no edge table refused something")
	}
	if m.Blocked() != 0 {
		t.Fatalf("Blocked() = %d, want 0", m.Blocked())
	}
}

func TestBlockedCountsEveryClassOfRefusal(t *testing.T) {
	var m Machine
	m.Edges(exampleEdges)
	m.Reset(stPlay)
	m.Go(stOptions)        // undeclared edge
	m.Pop()                // depth 1
	m.Push(stPause)        //   ok, depth 2
	m.Push(stOptions)      //   ok, depth 3
	m.Push(stResetConfirm) //   ok, depth 4 — now full
	m.Push(stTitle)        // overflow (a DECLARED edge, so this is depth alone)
	if m.Blocked() != 3 {
		t.Fatalf("Blocked() = %d, want 3 (undeclared edge, stray Pop, overflow)", m.Blocked())
	}
	if m.Depth() != MaxDepth || m.State() != stResetConfirm {
		t.Fatalf("Depth()=%d State()=%v after the refusals, want %d/reset — nothing moved", m.Depth(), m.State(), MaxDepth)
	}
}

var allocSink uint32

func countEnter(prev, next State) { allocSink += uint32(prev) + uint32(next) }
func countExit(cur, next State)   { allocSink += uint32(cur) + uint32(next) }

// TestNothingAllocates pins the no-allocation contract, Reset included.
func TestNothingAllocates(t *testing.T) {
	m := &Machine{}
	m.OnEnter(countEnter)
	m.OnExit(countExit)
	m.Edges(exampleEdges)
	m.Reset(stPlay)

	n := testing.AllocsPerRun(200, func() {
		m.Reset(stPlay)
		m.Tick(16_667)
		m.Push(stPause)
		m.Tick(16_667)
		m.Push(stOptions)
		m.Pop()
		m.Pop()
		m.Go(stTitle)
		m.Go(stPlay)
		allocSink += uint32(m.State()) + uint32(m.Under()) + uint32(m.Depth()) + uint32(m.Elapsed()) + m.Blocked()
		if m.In(stPlay) {
			allocSink++
		}
	})
	if n != 0 {
		t.Fatalf("allocations per full transition cycle = %v, want 0", n)
	}
}
