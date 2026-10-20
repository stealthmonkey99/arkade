// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

import "testing"

// cheatTaintOff clears the one-way taint for one test, since a test binary
// runs many sessions in one process.
func cheatTaintOff(t *testing.T) {
	t.Helper()
	prev := cheatTainted
	cheatTainted = false
	t.Cleanup(func() { cheatTainted = prev })
}

// cheatKbdDir is a dev console arrow: a bare virtual keypress.
func cheatKbdDir(key uint8) InputRecord {
	return InputRecord{Kind: KindKeypress, Key: key, Flags: FlagVirtual, Index: uint8(SeatNone), Physical: SeatNone}
}

func cheatKbdKey(key uint8) InputRecord {
	return InputRecord{Kind: KindKeypress, Key: key, Flags: FlagVirtual, Index: uint8(SeatNone), Physical: SeatNone}
}

// cheatStickDir is a real stick push's joystick record.
func cheatStickDir(joy, seat uint8) InputRecord {
	return InputRecord{Kind: KindJoystick, Joy: joy, Source: SourceController, Physical: Seat(seat), Index: seat, Player: Slot(seat + 1)}
}

// cheatStickTwin is the non-virtual keypress a real stick emits alongside
// its joystick record for the same push.
func cheatStickTwin(key, seat uint8) InputRecord {
	return InputRecord{Kind: KindKeypress, Key: key, Source: SourceController, Physical: Seat(seat), Index: seat, Player: Slot(seat + 1)}
}

func cheatBtn(kind, key, seat uint8) InputRecord {
	return InputRecord{Kind: kind, Key: key, Source: SourceController, Physical: Seat(seat), Index: seat, Player: Slot(seat + 1)}
}

var cheatKbdSeq = [CheatSteps]uint8{KeyUp, KeyUp, KeyDown, KeyDown, KeyLeft, KeyRight, KeyLeft, KeyRight}
var cheatJoySeq = [CheatSteps]uint8{JoyN, JoyN, JoyS, JoyS, JoyW, JoyE, JoyW, JoyE}

func TestCheatCodeKeyboardShapeNeedsTheAux1Latch(t *testing.T) {
	cheatTaintOff(t)
	var c CheatCode

	// Without the modifier the eight directions do nothing.
	for _, k := range cheatKbdSeq {
		if _, ok := c.Feed(cheatKbdDir(k)); ok {
			t.Fatal("the bare sequence fired without Aux1")
		}
	}
	if cheatTainted {
		t.Fatal("the bare sequence tainted the session")
	}

	c.Feed(cheatKbdKey(KeyJoyAux1))
	for i, k := range cheatKbdSeq {
		seat, ok := c.Feed(cheatKbdDir(k))
		if ok != (i == CheatSteps-1) {
			t.Fatalf("step %d: fired = %v, want %v", i, ok, i == CheatSteps-1)
		}
		if ok && seat != SeatNone {
			t.Fatalf("keyboard seat = %d, want SeatNone", seat)
		}
	}
	if !cheatTainted {
		t.Fatal("a completed combo did not taint the session")
	}
}

func TestCheatCodeRearmsAfterFiring(t *testing.T) {
	cheatTaintOff(t)
	var c CheatCode
	for round := 0; round < 2; round++ {
		c.Feed(cheatKbdKey(KeyJoyAux1))
		fired := false
		for _, k := range cheatKbdSeq {
			if _, ok := c.Feed(cheatKbdDir(k)); ok {
				fired = true
			}
		}
		if !fired {
			t.Fatalf("round %d did not fire — the detector failed to rearm", round)
		}
	}
}

func TestCheatCodeRealStickHoldsAux1(t *testing.T) {
	cheatTaintOff(t)
	var c CheatCode
	const seat = 1

	c.Feed(cheatBtn(KindKeydown, KeyJoyAux1, seat)) // held, no keyup until the end
	got := SeatNone
	for i, j := range cheatJoySeq {
		// Both records of a real push, in the order the host sends them.
		seatOut, ok := c.Feed(cheatStickDir(j, seat))
		if ok {
			got = seatOut
		}
		if _, twin := c.Feed(cheatStickTwin(cheatKbdSeq[i], seat)); twin {
			t.Fatalf("step %d: the non-virtual keypress twin double-counted", i)
		}
		// The stick returning to centre must not break the combo.
		if _, ok := c.Feed(cheatStickDir(JoyCenter, seat)); ok {
			t.Fatalf("step %d: a re-centre record fired the combo", i)
		}
	}
	if got != seat {
		t.Fatalf("completing seat = %d, want %d", got, seat)
	}
	c.Feed(cheatBtn(KindKeyup, KeyJoyAux1, seat))
}

func TestCheatCodeAux1ReleaseDisarms(t *testing.T) {
	cheatTaintOff(t)
	var c CheatCode
	c.Feed(cheatBtn(KindKeydown, KeyJoyAux1, 0))
	c.Feed(cheatKbdDir(KeyUp))
	c.Feed(cheatKbdDir(KeyUp))
	c.Feed(cheatBtn(KindKeyup, KeyJoyAux1, 0)) // let go halfway
	for _, k := range cheatKbdSeq[2:] {
		if _, ok := c.Feed(cheatKbdDir(k)); ok {
			t.Fatal("the combo completed after Aux1 was released")
		}
	}
	if c.Progress() != 0 {
		t.Fatalf("progress after release = %d, want 0", c.Progress())
	}
}

func TestCheatCodePartialMatchRestartsAtOne(t *testing.T) {
	cheatTaintOff(t)
	var c CheatCode
	c.Feed(cheatBtn(KindKeydown, KeyJoyAux1, 0))

	c.Feed(cheatKbdDir(KeyUp))
	c.Feed(cheatKbdDir(KeyUp))
	c.Feed(cheatKbdDir(KeyUp)) // wrong (wanted DOWN), but it IS sequence[0]
	if got := c.Progress(); got != 1 {
		t.Fatalf("progress after a first-element misfire = %d, want 1", got)
	}
	fired := false
	for _, k := range cheatKbdSeq[1:] {
		if _, ok := c.Feed(cheatKbdDir(k)); ok {
			fired = true
		}
	}
	if !fired {
		t.Fatal("the sequence did not complete from the restarted-at-1 state")
	}

	// A wrong press drops the leader to 1, not 0: every direction starts
	// one of the four rotations. No tick is emitted for it.
	var ticks int
	c.OnStep = func(int) { ticks++ }
	c.Feed(cheatKbdDir(KeyUp)) // leader -> 1
	c.Feed(cheatKbdDir(KeyUp)) // leader -> 2
	ticks = 0
	c.Feed(cheatKbdDir(KeyLeft)) // wrong for the run in progress
	if got := c.Progress(); got != 1 {
		t.Fatalf("progress after an unrelated misfire = %d, want 1 "+
			"(the rotation that begins with LEFT)", got)
	}
	if ticks != 0 {
		t.Fatalf("a misfire emitted %d entry ticks, want 0 — the tick must "+
			"mean 'you are getting closer', not 'a key moved'", ticks)
	}
}

func TestCheatCodeOnStepFiresPerAdvanceExceptTheLast(t *testing.T) {
	cheatTaintOff(t)
	var c CheatCode
	steps := make([]int, 0, CheatSteps)
	c.OnStep = func(n int) { steps = append(steps, n) }

	c.Feed(cheatBtn(KindKeydown, KeyJoyAux1, 0))
	for _, k := range cheatKbdSeq {
		c.Feed(cheatKbdDir(k))
	}
	want := []int{1, 2, 3, 4, 5, 6, 7} // no tick on the completing press
	if len(steps) != len(want) {
		t.Fatalf("OnStep calls = %v, want %v", steps, want)
	}
	for i := range want {
		if steps[i] != want[i] {
			t.Fatalf("OnStep calls = %v, want %v", steps, want)
		}
	}
}

func TestCheatCodeUnrelatedKeypressBreaksTheKeyboardLatch(t *testing.T) {
	cheatTaintOff(t)
	var c CheatCode
	c.Feed(cheatKbdKey(KeyJoyAux1))
	c.Feed(cheatKbdKey(KeyJoyA)) // did something else in between
	for _, k := range cheatKbdSeq {
		if _, ok := c.Feed(cheatKbdDir(k)); ok {
			t.Fatal("the combo fired through an unrelated keypress")
		}
	}
}

func TestCheatCodeResetKeepsOnStep(t *testing.T) {
	cheatTaintOff(t)
	var c CheatCode
	calls := 0
	c.OnStep = func(int) { calls++ }
	c.Feed(cheatBtn(KindKeydown, KeyJoyAux1, 0))
	c.Feed(cheatKbdDir(KeyUp))
	c.Reset()
	if c.Progress() != 0 || c.Seat() != SeatNone {
		t.Fatalf("after Reset: progress=%d seat=%d, want 0/SeatNone", c.Progress(), c.Seat())
	}
	c.Feed(cheatBtn(KindKeydown, KeyJoyAux1, 0))
	c.Feed(cheatKbdDir(KeyUp))
	if calls != 2 {
		t.Fatalf("OnStep calls = %d, want 2 (Reset must not clear the callback)", calls)
	}
}

// TestScoreSubmitReachesTheHostWhenClean is the control for
// TestCheatedSessionCannotSubmitAScore: if this stops panicking, that test
// proves nothing.
func TestScoreSubmitReachesTheHostWhenClean(t *testing.T) {
	cheatTaintOff(t)
	for _, tc := range []struct {
		name string
		call func()
	}{
		{"ScoreSubmit", func() { ScoreSubmit(100, "AAA") }},
		{"ScoreQualifies", func() { ScoreQualifies(100) }},
		{"ScoreRank", func() { ScoreRank(100) }},
		{"NoteRecentName", func() { NoteRecentName(0, "AAA") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s did not reach the host stub — the chokepoint test proves nothing", tc.name)
				}
			}()
			tc.call()
		})
	}
}

func TestCheatedSessionCannotSubmitAScore(t *testing.T) {
	cheatTaintOff(t)
	var c CheatCode
	c.Feed(cheatBtn(KindKeydown, KeyJoyAux1, 0))
	for _, k := range cheatKbdSeq {
		c.Feed(cheatKbdDir(k))
	}
	if !CheatTainted() {
		t.Fatal("the combo did not taint the session")
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("a cheated session reached a host_score_* import: %v", r)
		}
	}()
	if rank := ScoreSubmit(999999, "CHEAT"); rank != 0 {
		t.Fatalf("ScoreSubmit in a cheated session = %d, want 0", rank)
	}
	if ScoreQualifies(999999) {
		t.Fatal("ScoreQualifies in a cheated session = true, want false")
	}
	if rank := ScoreRank(999999); rank != 0 {
		t.Fatalf("ScoreRank in a cheated session = %d, want 0", rank)
	}
	NoteRecentName(0, "CHEAT")
}

func TestMarkCheatedShutsTheSameDoor(t *testing.T) {
	cheatTaintOff(t)
	MarkCheated()
	if !CheatTainted() {
		t.Fatal("MarkCheated did not taint the session")
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("MarkCheated left the score path open: %v", r)
		}
	}()
	if rank := ScoreSubmit(1, "X"); rank != 0 {
		t.Fatalf("ScoreSubmit = %d, want 0", rank)
	}
}

// TestCheatCodeSurvivesTheSeatRemap: on an upright panel the cabinet turns
// the left seat's stick -90 degrees and the right seat's +90, so a physical
// UP arrives as left at seat 0 and right at seat 1.
func TestCheatCodeSurvivesTheSeatRemap(t *testing.T) {
	// Measured on an upright panel. Index is the physical push; value is
	// what arrives.
	seatRemap := map[uint8][2]uint8{
		//  physical      seat 0   seat 1
		JoyN: {JoyW, JoyE}, // up    -> left  / right
		JoyS: {JoyE, JoyW}, // down  -> right / left
		JoyW: {JoyS, JoyN}, // left  -> down  / up
		JoyE: {JoyN, JoyS}, // right -> up    / down
	}

	for seat := uint8(0); seat < 2; seat++ {
		cheatTaintOff(t)
		var c CheatCode
		// Hold Aux1 on that seat's own pad.
		c.Feed(cheatBtn(KindKeydown, KeyJoyAux1, seat))

		fired := false
		for _, physical := range cheatJoySeq { // the PHYSICAL Konami gesture
			arrives := seatRemap[physical][seat]
			if _, ok := c.Feed(cheatStickDir(arrives, seat)); ok {
				fired = true
			}
		}
		if !fired {
			t.Errorf("seat %d: the physical Konami code did not fire — "+
				"the mount remap is still eating it", seat)
		}
	}
}
