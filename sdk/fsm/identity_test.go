// SPDX-License-Identifier: GPL-2.0-or-later

package fsm

import "testing"

// idTrace records OnEnterID calls with their ids; the state set and stName
// come from fsm_test.go.
var idTrace []string

func recEnterID(id uint8, prev, next State) {
	idTrace = append(idTrace, itoa(id)+" "+stName(prev)+">"+stName(next))
}

func itoa(id uint8) string {
	if id >= 10 {
		return "??"
	}
	return string(rune('0' + id))
}

func idTraced(id uint8) *IDMachine {
	m := &IDMachine{}
	m.SetID(id)
	m.OnEnterID(recEnterID)
	return m
}

func wantIDTrace(t *testing.T, want ...string) {
	t.Helper()
	if len(idTrace) != len(want) {
		t.Fatalf("OnEnterID calls = %v, want %v", idTrace, want)
	}
	for i := range want {
		if idTrace[i] != want[i] {
			t.Fatalf("OnEnterID calls = %v, want %v", idTrace, want)
		}
	}
}

func TestIDMachineZeroValueIDIsZero(t *testing.T) {
	var m IDMachine
	if m.ID() != 0 {
		t.Fatalf("ID() on a virgin IDMachine = %d, want 0 — the first entity in an array needs no SetID call", m.ID())
	}
	m.SetID(3)
	if m.ID() != 3 {
		t.Fatalf("ID() after SetID(3) = %d, want 3", m.ID())
	}
}

func TestOnEnterIDFiresOnResetGoPushPop(t *testing.T) {
	idTrace = idTrace[:0]
	m := idTraced(7)

	m.Reset(stTitle)
	m.Go(stPlay)
	m.Push(stPause)
	m.Pop()
	wantIDTrace(t,
		"7 title>title", // virgin machine: prev == next
		"7 title>play",
		"7 play>pause",
		"7 pause>play",
	)
}

// TestOneHookToldApartOnlyByID shares one OnEnterID hook across an array of
// entities that differ only by id.
func TestOneHookToldApartOnlyByID(t *testing.T) {
	idTrace = idTrace[:0]
	var jellies [4]IDMachine
	for i := range jellies {
		jellies[i].SetID(uint8(i))
		jellies[i].OnEnterID(recEnterID)
		jellies[i].Reset(stPlay)
	}
	idTrace = idTrace[:0]

	jellies[0].Go(stPause)
	jellies[3].Go(stOptions)
	jellies[1].Go(stTitle)
	wantIDTrace(t, "0 play>pause", "3 play>options", "1 play>title")

	// a transition does not leak into a sibling
	if jellies[2].State() != stPlay {
		t.Fatalf("jellies[2].State() = %v, want play — a sibling's Go moved it", jellies[2].State())
	}
	if jellies[0].State() != stPause || jellies[3].State() != stOptions || jellies[1].State() != stTitle {
		t.Fatalf("states = %v/%v/%v/%v, want pause/play/play/options/title",
			jellies[0].State(), jellies[1].State(), jellies[2].State(), jellies[3].State())
	}
}

func TestOnEnterIDDoesNotFireOnARefusedGo(t *testing.T) {
	idTrace = idTrace[:0]
	m := idTraced(1)
	m.Edges([]uint16{stPlay: 1 << stPause})
	m.Reset(stPlay)
	idTrace = idTrace[:0]

	if m.Go(stOptions) { // undeclared
		t.Fatal("an undeclared Go succeeded")
	}
	wantIDTrace(t)
	if m.Blocked() != 1 {
		t.Fatalf("Blocked() = %d, want 1", m.Blocked())
	}
	if m.State() != stPlay {
		t.Fatalf("State() = %v after a refused Go, want play", m.State())
	}
}

func TestOnEnterIDDoesNotFireOnAPopAtDepthOne(t *testing.T) {
	idTrace = idTrace[:0]
	m := idTraced(1)
	m.Reset(stPlay)
	idTrace = idTrace[:0]

	if m.Pop() {
		t.Fatal("Pop at depth 1 succeeded")
	}
	wantIDTrace(t)
	if m.Blocked() != 1 {
		t.Fatalf("Blocked() = %d, want 1", m.Blocked())
	}
}

func TestOnEnterIDDoesNotFireOnAPushPastMaxDepth(t *testing.T) {
	idTrace = idTrace[:0]
	m := idTraced(1)
	m.Reset(stPlay)
	m.Push(stPause)
	m.Push(stOptions)
	m.Push(stResetConfirm) // depth 4, full
	idTrace = idTrace[:0]

	if m.Push(stTitle) {
		t.Fatal("Push past MaxDepth succeeded")
	}
	wantIDTrace(t)
	if m.Depth() != MaxDepth {
		t.Fatalf("Depth() = %d, want %d", m.Depth(), MaxDepth)
	}
	if m.Blocked() != 1 {
		t.Fatalf("Blocked() = %d, want 1", m.Blocked())
	}
}

func TestIDMachinePromotesEverythingElseUnchanged(t *testing.T) {
	m := idTraced(2)
	m.Edges(exampleEdges)
	m.Reset(stPlay)
	m.Tick(500)

	if m.Elapsed() != 500 {
		t.Fatalf("Elapsed() = %d, want 500", m.Elapsed())
	}
	m.Push(stPause)
	if !m.In(stPlay) {
		t.Fatal("In(play) is false under a modal")
	}
	if m.Under() != stPlay {
		t.Fatalf("Under() = %v, want play", m.Under())
	}
	if m.Depth() != 2 {
		t.Fatalf("Depth() = %d, want 2", m.Depth())
	}

	var exited []string
	m.OnExit(func(cur, next State) { exited = append(exited, stName(cur)+">"+stName(next)) })
	m.Pop()
	if len(exited) != 1 || exited[0] != "pause>play" {
		t.Fatalf("OnExit trace = %v, want [pause>play] — OnExit needs no identity and is not overridden", exited)
	}
}

func TestIDMachineFourDeepStackAndUnwind(t *testing.T) {
	idTrace = idTrace[:0]
	m := idTraced(9)
	m.Reset(stPlay)
	m.Push(stPause)
	m.Push(stOptions)
	m.Push(stResetConfirm)
	if m.Depth() != MaxDepth {
		t.Fatalf("Depth() = %d, want %d", m.Depth(), MaxDepth)
	}
	idTrace = idTrace[:0]
	m.Pop()
	m.Pop()
	m.Pop()
	wantIDTrace(t, "9 reset>options", "9 options>pause", "9 pause>play")
	if m.Depth() != 1 || m.State() != stPlay {
		t.Fatalf("Depth()=%d State()=%v after unwinding, want 1/play", m.Depth(), m.State())
	}
}

var idAllocSink uint32

func countEnterID(id uint8, prev, next State) {
	idAllocSink += uint32(id) + uint32(prev) + uint32(next)
}

// TestIDMachineNothingAllocates extends the no-allocation pin to the id path.
func TestIDMachineNothingAllocates(t *testing.T) {
	m := &IDMachine{}
	m.SetID(5)
	m.OnEnterID(countEnterID)
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
		idAllocSink += uint32(m.State()) + uint32(m.Under()) + uint32(m.Depth()) + uint32(m.Elapsed()) + m.Blocked() + uint32(m.ID())
		if m.In(stPlay) {
			idAllocSink++
		}
	})
	if n != 0 {
		t.Fatalf("allocations per full transition cycle = %v, want 0", n)
	}
}
