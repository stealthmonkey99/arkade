// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

func TestAttractZeroValueIsSafe(t *testing.T) {
	var a Attract
	if got := a.Update(1_000_000); got {
		t.Errorf("Update on empty Attract = true, want false")
	}
	a.Draw(newByteCanvas(4, 4)) // must not panic
	a.Next()
	a.Goto(5)
	if p := a.Phase(); p != 0 {
		t.Errorf("Phase() = %d, want 0", p)
	}
	if e := a.Elapsed(); e != 0 {
		t.Errorf("Elapsed() = %d, want 0", e)
	}
	if pr := a.Progress(); pr != 0 {
		t.Errorf("Progress() = %v, want 0", pr)
	}
}

func TestAttractAutoAdvanceWraps(t *testing.T) {
	var drew []int
	a := Attract{Phases: []AttractPhase{
		{DurationMicros: 100, Draw: func(uint64, Canvas) { drew = append(drew, 0) }},
		{DurationMicros: 200, Draw: func(uint64, Canvas) { drew = append(drew, 1) }},
	}}
	cv := newByteCanvas(1, 1)

	if a.Update(50); a.Phase() != 0 {
		t.Fatalf("after 50us: phase = %d, want 0 (not yet due)", a.Phase())
	}
	if transitioned := a.Update(50); !transitioned || a.Phase() != 1 {
		t.Fatalf("after 100us total: transitioned=%v phase=%d, want true/1", transitioned, a.Phase())
	}
	if e := a.Elapsed(); e != 0 {
		t.Fatalf("Elapsed() right after transition = %d, want 0 (zeroed)", e)
	}
	if a.Update(199); a.Phase() != 1 {
		t.Fatalf("phase = %d, want still 1 (199 < 200)", a.Phase())
	}
	if transitioned := a.Update(1); !transitioned || a.Phase() != 0 {
		t.Fatalf("after phase 1's duration: transitioned=%v phase=%d, want true/0 (wrapped)", transitioned, a.Phase())
	}

	a.Draw(cv)
	if len(drew) == 0 || drew[len(drew)-1] != 0 {
		t.Fatalf("Draw dispatched to phase %v, want the current phase (0)", drew)
	}
}

func TestAttractZeroDurationNeverAutoAdvances(t *testing.T) {
	a := Attract{Phases: []AttractPhase{
		{DurationMicros: 0},
		{DurationMicros: 100},
	}}
	if a.Update(1_000_000_000); a.Phase() != 0 {
		t.Fatalf("phase = %d, want 0 (DurationMicros==0 never auto-advances)", a.Phase())
	}
}

// TestAttractOnEnterFiresOnceForTheFirstPhase pins that the first phase's
// OnEnter fires exactly once, lazily, from whichever of Update or Draw runs
// first.
func TestAttractOnEnterFiresOnceForTheFirstPhase(t *testing.T) {
	enters := 0
	a := Attract{Phases: []AttractPhase{
		{DurationMicros: 1_000_000, OnEnter: func() { enters++ }},
	}}
	a.Draw(newByteCanvas(1, 1))
	a.Update(1)
	a.Update(1)
	a.Draw(newByteCanvas(1, 1))
	if enters != 1 {
		t.Errorf("OnEnter fired %d times before any transition, want exactly 1", enters)
	}
}

// TestAttractOnEnterFiresOnEveryTransition pins that OnEnter fires each time
// its phase becomes current, by timeout, Next or Goto.
func TestAttractOnEnterFiresOnEveryTransition(t *testing.T) {
	var log []int
	a := Attract{Phases: []AttractPhase{
		{DurationMicros: 10, OnEnter: func() { log = append(log, 0) }},
		{DurationMicros: 0, OnEnter: func() { log = append(log, 1) }},
	}}
	a.Update(10) // 0 -> 1 (timeout)
	a.Next()     // 1 -> 0 (cartridge-decided end)
	a.Goto(1)    // explicit jump
	want := []int{0, 1, 0, 1}
	if len(log) != len(want) {
		t.Fatalf("OnEnter log = %v, want %v", log, want)
	}
	for i := range want {
		if log[i] != want[i] {
			t.Fatalf("OnEnter log = %v, want %v", log, want)
		}
	}
}

func TestAttractGotoWrapsNegativeAndOutOfRange(t *testing.T) {
	a := Attract{Phases: []AttractPhase{{}, {}, {}}}
	a.Goto(-1)
	if p := a.Phase(); p != 2 {
		t.Errorf("Goto(-1) with 3 phases = %d, want 2", p)
	}
	a.Goto(7)
	if p := a.Phase(); p != 1 {
		t.Errorf("Goto(7) with 3 phases = %d, want 1", p)
	}
}

// TestAttractDataDependentBranch pins a phase with DurationMicros 0 that picks
// its next phase from data with Elapsed and Goto.
func TestAttractDataDependentBranch(t *testing.T) {
	const (
		phaseGap = iota
		phaseLit
		phaseGrow
	)
	remaining := 2 // two more LIT beats before the branch goes to GROW
	a := Attract{Phases: []AttractPhase{
		{DurationMicros: 100}, // GAP: fixed, sequential
		{DurationMicros: 0},   // LIT: data-dependent next phase
		{DurationMicros: 50},  // GROW: fixed, sequential
	}}
	a.Update(100)
	if a.Phase() != phaseLit {
		t.Fatalf("phase = %d, want phaseLit", a.Phase())
	}
	// LIT's duration is a local constant the cartridge checks itself.
	for i := 0; i < 3; i++ {
		a.Update(30)
		if a.Elapsed() < 30 {
			continue
		}
		remaining--
		if remaining > 0 {
			a.Goto(phaseGap)
		} else {
			a.Goto(phaseGrow)
		}
	}
	if a.Phase() != phaseGrow {
		t.Fatalf("phase = %d, want phaseGrow after the branch resolves", a.Phase())
	}
}

// TestAttractProgress pins the [0,1] clamp and the DurationMicros == 0 case.
func TestAttractProgress(t *testing.T) {
	a := Attract{Phases: []AttractPhase{
		{DurationMicros: 200},
		{DurationMicros: 0},
	}}
	a.Update(50)
	if got := a.Progress(); got != 0.25 {
		t.Errorf("Progress() at 50/200 = %v, want 0.25", got)
	}
	a.Update(1000) // overshoots; Update transitions away first
	if a.Phase() != 1 {
		t.Fatalf("phase = %d, want 1 (timed out)", a.Phase())
	}
	if got := a.Progress(); got != 0 {
		t.Errorf("Progress() on a DurationMicros==0 phase = %v, want 0", got)
	}
}

func TestAttractElapsedPassedToDraw(t *testing.T) {
	var got uint64
	a := Attract{Phases: []AttractPhase{
		{DurationMicros: 1_000_000, Draw: func(elapsed uint64, _ Canvas) { got = elapsed }},
	}}
	a.Update(12345)
	a.Draw(newByteCanvas(1, 1))
	if got != a.Elapsed() || got != 12345 {
		t.Errorf("Draw saw elapsed=%d, want %d", got, a.Elapsed())
	}
}
