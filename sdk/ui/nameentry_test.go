// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

// tap simulates one full press and release of dir. Move is a level call, so
// two presses of the same direction in a row need the release between them.
func (n *NameEntry) tap(dir Direction) {
	n.Move(dir)
	n.Move(DirNone)
}

func TestNameEntryStartsBlank(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	if n.Name() != "" {
		t.Errorf("fresh entry should be blank, got %q", n.Name())
	}
	if n.Done() {
		t.Error("fresh entry should not be done")
	}
	if n.cursor != 0 {
		t.Errorf("fresh entry cursor should start at slot 0, got %d", n.cursor)
	}
}

func TestNameEntryDefaultMaxLenIsSeven(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	if len(n.slots) != DefaultNameEntryMaxLen {
		t.Errorf("default MaxLen should be %d, got %d", DefaultNameEntryMaxLen, len(n.slots))
	}
}

func TestNameEntryUpCyclesBlankThenAlphabet(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.tap(DirUp)
	if n.slots[0] != 'A' {
		t.Errorf("first Up from blank should land on A, got %q", n.slots[0])
	}
	n.tap(DirUp)
	if n.slots[0] != 'B' {
		t.Errorf("second Up should land on B, got %q", n.slots[0])
	}
}

func TestNameEntryLevelMoveDoesNotRepeatWithoutUpdate(t *testing.T) {
	// Repeated Move with the same held direction must not re-cycle; only Update's
	// hold-to-scroll advances while held.
	n := NewNameEntry(0, 0, 0)
	n.Move(DirUp)
	n.Move(DirUp)
	n.Move(DirUp)
	if n.slots[0] != 'A' {
		t.Errorf("repeated Move(DirUp) without release or Update should stay at A, got %q", n.slots[0])
	}
}

func TestNameEntryUpWrapsPastDigitsToBlank(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	// blank -> A..Z (26) -> 0..9 (10) -> blank: 37 taps back to blank.
	for i := 0; i < 37; i++ {
		n.tap(DirUp)
	}
	if n.slots[0] != ' ' {
		t.Errorf("37 taps of Up should wrap all the way back to blank, got %q", n.slots[0])
	}
}

func TestNameEntryDownFromBlankWrapsToNine(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.tap(DirDown)
	if n.slots[0] != '9' {
		t.Errorf("Down from blank should wrap backward to 9, got %q", n.slots[0])
	}
}

func TestNameEntryLeftRightMovesCursorAcrossAllCells(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.tap(DirLeft)
	if n.cursor != 0 {
		t.Errorf("Left at cursor 0 should stay clamped, got %d", n.cursor)
	}
	n.tap(DirRight)
	n.tap(DirRight)
	if n.cursor != 2 {
		t.Errorf("two Rights should move cursor to 2, got %d", n.cursor)
	}
	for i := 0; i < 10; i++ {
		n.tap(DirRight)
	}
	if n.cursor != n.maxLen {
		t.Errorf("Right should clamp at the OK cell (index %d), got cursor %d", n.maxLen, n.cursor)
	}
	n.tap(DirLeft)
	if n.cursor != n.maxLen-1 {
		t.Errorf("Left from the OK cell should return to the last letter slot, got %d", n.cursor)
	}
}

func TestNameEntryActivateOnFilledSlotAdvancesOntoOK(t *testing.T) {
	n := NewNameEntry(0, 0, 2)
	n.tap(DirUp) // slot 0 = A
	n.Do(ActionActivate)
	if n.cursor != 1 {
		t.Fatalf("Activate on a filled slot should advance one cell, got %d", n.cursor)
	}
	n.tap(DirUp) // slot 1 = A
	n.Do(ActionActivate)
	if n.cursor != n.maxLen {
		t.Errorf("Activate on the filled last slot should walk onto the OK cell, got %d", n.cursor)
	}
	if n.Done() {
		t.Error("reaching the OK cell must not submit by itself")
	}
}

func TestNameEntryActivateOnEmptySlotJumpsToOK(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	for i := 0; i < 4; i++ {
		n.tap(DirUp)         // cycle the current slot to 'A'
		n.Do(ActionActivate) // confirm it and advance to the next slot
	}
	// cursor now on slot 4, which is empty: confirm means "I'm done".
	n.Do(ActionActivate)
	if n.cursor != n.maxLen {
		t.Errorf("Activate on an empty slot should jump straight to the OK cell, got cursor %d", n.cursor)
	}
	if n.Done() {
		t.Error("the jump itself must not submit")
	}
}

func TestNameEntryActivateOnOKSubmitsTrimmed(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.SetName("MARK")
	n.Do(ActionActivate) // cursor already on OK (prefill start)
	if !n.Done() {
		t.Fatal("Activate on the OK cell should submit")
	}
	if got := n.Name(); got != "MARK" {
		t.Errorf("submit should trim trailing blanks, got %q", got)
	}
}

func TestNameEntryEmptyConfirmSubmitsAnonymous(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.Do(ActionActivate) // slot 0 empty -> jump to OK
	n.Do(ActionActivate) // OK -> submit
	if !n.Done() {
		t.Fatal("Activate Activate on a blank entry should submit")
	}
	if got := n.Name(); got != "" {
		t.Errorf("a blank entry should submit as \"\", got %q", got)
	}
}

func TestNameEntryEraseClearsAndMovesLeft(t *testing.T) {
	n := NewNameEntry(0, 0, 3)
	n.tap(DirUp) // slot 0 = A
	n.tap(DirRight)
	n.tap(DirUp) // slot 1 = A
	n.tap(DirRight)

	// cursor now on slot 2 (blank): erase should just move left.
	n.Do(ActionErase)
	if n.cursor != 1 || n.slots[1] != 'A' {
		t.Fatalf("erase on a blank slot should only move left, got cursor=%d slots=%q", n.cursor, n.slots)
	}

	// cursor on slot 1 (filled 'A'): erase clears it and moves left.
	n.Do(ActionErase)
	if n.cursor != 0 || n.slots[1] != ' ' {
		t.Errorf("erase on a filled slot should clear it and move left, got cursor=%d slots=%q", n.cursor, n.slots)
	}
}

func TestNameEntryEraseFromOKMovesBackWithoutClearing(t *testing.T) {
	n := NewNameEntry(0, 0, 2)
	n.SetName("AB") // fills both slots, cursor on OK
	if n.cursor != n.maxLen {
		t.Fatalf("SetName filling both slots should land cursor on OK, got %d", n.cursor)
	}
	n.Do(ActionErase)
	if n.cursor != n.maxLen-1 {
		t.Errorf("erase on the OK cell should move back to the last slot, got %d", n.cursor)
	}
	if n.Name() != "AB" {
		t.Errorf("erase on the OK cell must not clear a letter, got %q", n.Name())
	}
}

func TestNameEntryEraseAtSlotZeroStaysPut(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.tap(DirUp)
	n.Do(ActionErase)
	if n.cursor != 0 {
		t.Errorf("erase at slot 0 should not move past it, got cursor %d", n.cursor)
	}
	if n.slots[0] != ' ' {
		t.Errorf("erase should still clear slot 0, got %q", n.slots[0])
	}
}

// TestNameEntryBackIsInert pins that ActionBack does nothing, so a cartridge
// can treat B as back or the quit confirm without the widget swallowing it.
func TestNameEntryBackIsInert(t *testing.T) {
	n := NewNameEntry(0, 0, 3)
	n.tap(DirUp) // slot 0 = A
	n.tap(DirRight)
	n.tap(DirUp) // slot 1 = A
	before, beforeCursor := n.Name(), n.cursor

	n.Do(ActionBack)
	if n.Name() != before || n.cursor != beforeCursor {
		t.Errorf("ActionBack must not touch the entry (B belongs to the host as back/exit), got name=%q cursor=%d, want name=%q cursor=%d",
			n.Name(), n.cursor, before, beforeCursor)
	}
	if n.Done() {
		t.Error("ActionBack must not submit either")
	}
}

func TestNameEntryUpDownOnOKCellAreNoops(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	for i := 0; i < n.maxLen; i++ {
		n.tap(DirRight)
	}
	if n.cursor != n.maxLen {
		t.Fatalf("expected cursor on OK, got %d", n.cursor)
	}
	n.tap(DirUp)
	n.tap(DirDown)
	if n.Name() != "" {
		t.Errorf("Up/Down on the OK cell must not edit any slot, got %q", n.Name())
	}
}

func TestNameEntryDoneIgnoresFurtherInput(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.Do(ActionActivate) // jump to OK
	n.Do(ActionActivate) // submit
	if !n.Done() {
		t.Fatal("setup: entry should be done")
	}
	n.tap(DirUp)
	if n.slots[0] != ' ' {
		t.Error("a done entry should ignore Move")
	}
	n.Do(ActionErase)
	if n.cursor != n.maxLen {
		t.Error("a done entry should ignore Do")
	}
}

func TestNameEntryHoldRepeatTiming(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.Move(DirUp) // press-and-hold: fires immediately
	if n.slots[0] != 'A' {
		t.Fatalf("the press edge should cycle immediately, got %q", n.slots[0])
	}

	n.Update(100_000) // total 100ms: first Update latches the hold here
	n.Update(200_000) // total 300ms: inside the 350ms initial delay
	n.Update(149_000) // total 449ms: still inside
	if n.slots[0] != 'A' {
		t.Fatalf("no repeat should fire during the initial delay, got %q", n.slots[0])
	}
	n.Update(2_000) // total 451ms: first repeat
	if n.slots[0] != 'B' {
		t.Fatalf("first repeat should fire after the 350ms delay, got %q", n.slots[0])
	}
	n.Update(49_000) // total 500ms: inside the 90ms interval
	if n.slots[0] != 'B' {
		t.Fatalf("repeats should be 90ms apart, got %q", n.slots[0])
	}
	n.Update(45_000) // total 545ms: second repeat
	if n.slots[0] != 'C' {
		t.Fatalf("second repeat should fire 90ms after the first, got %q", n.slots[0])
	}
	n.Update(180_000) // total 725ms: two more repeats due (631, 721 equivalents)
	if n.slots[0] != 'E' {
		t.Errorf("catch-up should step once per 90ms elapsed, got %q", n.slots[0])
	}
}

func TestNameEntryHoldRepeatDown(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.Move(DirDown) // blank wraps back to '9'
	n.Update(0)
	n.Update(360_000) // one repeat: '9' -> '8'
	if n.slots[0] != '8' {
		t.Errorf("held Down should repeat backward through the charset, got %q", n.slots[0])
	}
}

func TestNameEntryReleaseStopsRepeat(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.Move(DirUp)
	n.Update(0)
	n.Update(360_000) // 'A' -> 'B'
	n.Move(DirNone)   // release
	n.Update(1_000_000)
	n.Update(1_000_000)
	if n.slots[0] != 'B' {
		t.Errorf("release must stop the repeat, got %q", n.slots[0])
	}
}

func TestNameEntryFreshPressReArmsAfterRelease(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.Move(DirUp) // 'A'
	n.Update(0)
	n.Move(DirNone)
	n.Update(5_000_000)
	if n.slots[0] != 'A' {
		t.Fatalf("release should stop the hold without further edits, got %q", n.slots[0])
	}
	// A fresh press re-arms with a fresh delay window.
	n.Move(DirUp) // wraps 'A' -> 'B'
	n.Update(0)
	n.Update(100_000) // 100ms held: inside the fresh 350ms delay
	if n.slots[0] != 'B' {
		t.Fatalf("re-armed hold should restart the initial delay, got %q", n.slots[0])
	}
	n.Update(260_000) // 360ms held: repeat fires
	if n.slots[0] != 'C' {
		t.Errorf("re-armed hold should repeat after the delay, got %q", n.slots[0])
	}
}

func TestNameEntrySubmitClearsHold(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.Move(DirUp)        // 'A'
	n.Do(ActionActivate) // filled slot -> advance to slot 1
	n.Do(ActionActivate) // slot 1 blank -> jump to OK
	n.Do(ActionActivate) // OK -> submit
	if !n.Done() {
		t.Fatal("expected the entry to be submitted")
	}
	if n.pendingHold != DirNone || n.hold.Held() != DirNone {
		t.Error("submit should clear the held direction")
	}
	n.Update(1_000_000)
	n.Update(1_000_000)
	if n.slots[0] != 'A' {
		t.Errorf("no further repeats should fire after submit, got %q", n.slots[0])
	}
}

func TestNameEntryHoldOnOKCellDoesNotEdit(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.Do(ActionActivate) // empty slot 0 -> jump to OK
	n.Move(DirUp)        // no-op on OK
	n.Update(0)
	n.Update(1_000_000)
	if n.Name() != "" {
		t.Errorf("held repeat on the OK cell must not edit slots, got %q", n.Name())
	}
}

func TestNameEntryPrefillOnePressAccept(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.SetName("MARK")
	if n.cursor != n.maxLen {
		t.Fatalf("a prefilled entry should start on the OK cell, got cursor %d", n.cursor)
	}
	if n.Name() != "MARK" {
		t.Fatalf("slots should start prefilled, got %q", n.Name())
	}
	n.Do(ActionActivate) // ONE press: cursor is already on OK
	if !n.Done() || n.Name() != "MARK" {
		t.Errorf("a returning player should accept with a single Activate, got done=%v name=%q", n.Done(), n.Name())
	}
}

func TestNameEntryPrefillEditAfter(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.SetName("MARK")
	// LEFT steps back off OK into the slots as usual.
	n.tap(DirLeft)
	if n.cursor != n.maxLen-1 {
		t.Fatalf("Left from OK should land on the last slot, got %d", n.cursor)
	}
	// Walk to slot 3 (the final 'T') and back it out.
	for n.cursor > 3 {
		n.tap(DirLeft)
	}
	n.Do(ActionErase)
	if n.Name() != "MAR" {
		t.Errorf("erase should edit the prefilled letters, got %q", n.Name())
	}
	// Cycling still works on a prefilled slot.
	n.tap(DirUp) // slot 2: 'R' -> 'S'
	if n.Name() != "MAS" {
		t.Errorf("cycling should edit the prefilled letters, got %q", n.Name())
	}
}

func TestNameEntryPrefillSanitized(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.SetName("ma-rk!5 ")
	// lowercase uppercased, '-'/'!' dropped, trailing space trimmed.
	if n.Name() != "MARK5" {
		t.Errorf("prefill should sanitize to the widget charset, got %q", n.Name())
	}
	long := NewNameEntry(0, 0, 0)
	long.SetName("ABCDEFGHIJ")
	if long.Name() != "ABCDEFG" {
		t.Errorf("prefill should truncate to MaxLen, got %q", long.Name())
	}
	inner := NewNameEntry(0, 0, 0)
	inner.SetName("MARK B")
	if inner.Name() != "MARK B" {
		t.Errorf("inner spaces are part of the charset and must survive, got %q", inner.Name())
	}
}

func TestNameEntryPrefillFullyInvalidActsBlank(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.SetName("!!!")
	if n.cursor != 0 || n.Name() != "" {
		t.Errorf("a prefill that sanitizes to nothing must behave exactly like no prefill, got cursor=%d name=%q", n.cursor, n.Name())
	}
}

func TestNameEntryResetRestoresPrefill(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.SetName("MARK")
	// Mangle the entry (back out through the blank tail into the last
	// 'T'), then Reset: back to the prefilled state, not blank.
	n.tap(DirLeft)
	for i := 0; i < 4; i++ {
		n.Do(ActionErase)
	}
	if n.Name() == "MARK" {
		t.Fatal("expected the edits to have changed the name")
	}
	n.Reset()
	if n.Name() != "MARK" || n.cursor != n.maxLen || n.Done() {
		t.Errorf("Reset should restore the prefilled state (cursor on OK), got name=%q cursor=%d done=%v", n.Name(), n.cursor, n.Done())
	}
}

func TestNameEntrySetNameLaterAndCleared(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	if n.cursor != 0 {
		t.Fatal("unprefilled entry should start at slot 0")
	}
	n.SetName("zoe")
	if n.Name() != "ZOE" || n.cursor != n.maxLen {
		t.Errorf("SetName should sanitize, fill and park on OK, got name=%q cursor=%d", n.Name(), n.cursor)
	}
	n.SetName("")
	if n.Name() != "" || n.cursor != 0 {
		t.Errorf("clearing the prefill should reset blank at slot 0, got name=%q cursor=%d", n.Name(), n.cursor)
	}
}

func TestNameEntryBlankPrefillIsExactlyClassicBehavior(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.SetName("")
	if n.cursor != 0 || n.Name() != "" {
		t.Errorf("blank prefill must match the classic start, got cursor=%d name=%q", n.cursor, n.Name())
	}
}

func recentEntry(t *testing.T, names ...string) *NameEntry {
	t.Helper()
	n := NewNameEntry(0, 0, 0)
	n.SetRecents(names)
	return n
}

func TestNameEntryNoRecentsIsExactlyTheClassicRing(t *testing.T) {
	// With nothing offered, Down from the opening blank still wraps to '9'.
	n := NewNameEntry(0, 0, 0)
	n.tap(DirDown)
	if n.slots[0] != '9' {
		t.Errorf("Down from blank with no recents should wrap to 9, got %q", n.slots[0])
	}
	n.SetRecents(nil)
	n.Reset()
	n.tap(DirDown)
	if n.slots[0] != '9' {
		t.Errorf("SetRecents(nil) must leave the classic wrap alone, got %q", n.slots[0])
	}
}

func TestNameEntryDownFromBlankWalksTheRecents(t *testing.T) {
	n := recentEntry(t, "MARK", "ZOE")
	n.tap(DirDown)
	if n.Name() != "MARK" {
		t.Fatalf("first Down past blank should preview the top recent, got %q", n.Name())
	}
	n.tap(DirDown)
	if n.Name() != "ZOE" {
		t.Fatalf("second Down should preview the next recent, got %q", n.Name())
	}
	// Off the bottom of the list the charset resumes, so the ring stays a ring.
	n.tap(DirDown)
	if n.Name() != "9" {
		t.Fatalf("Down past the last recent should land on 9, got %q", n.Name())
	}
	n.tap(DirDown)
	if n.Name() != "8" {
		t.Errorf("the charset should keep cycling below the list, got %q", n.Name())
	}
}

func TestNameEntryUpOutOfTheRecentsRestoresWhatWasTyped(t *testing.T) {
	n := recentEntry(t, "MARK")
	// Type "XY", walk back to slot 0 and blank it: the state a player is
	// in when they go looking for the list mid-name.
	n.slots[0], n.slots[1] = 'X', 'Y'
	n.tap(DirDown)
	if n.slots[0] != 'W' {
		t.Fatalf("Down on a non-blank slot 0 must cycle the charset, got %q", n.slots[0])
	}
	n.slots[0] = ' '
	n.tap(DirDown)
	if n.Name() != "MARK" {
		t.Fatalf("expected the recent preview, got %q", n.Name())
	}
	n.tap(DirUp)
	if n.Name() != " Y" {
		t.Errorf("backing up out of the list should restore the typed letters, got %q", n.Name())
	}
	if n.recentIdx != -1 {
		t.Error("backing out should leave the list")
	}
}

func TestNameEntryRecentsOnlyOpenFromSlotZero(t *testing.T) {
	n := recentEntry(t, "MARK")
	n.tap(DirRight)
	n.tap(DirDown)
	if n.slots[1] != '9' {
		t.Errorf("the list hangs off slot 0 only; slot 1 should wrap to 9, got %q", n.slots[1])
	}
}

func TestNameEntryActivateTakesTheRecentAndParksOnOK(t *testing.T) {
	n := recentEntry(t, "MARK", "ZOE")
	n.tap(DirDown)
	n.Do(ActionActivate)
	if n.cursor != n.maxLen {
		t.Fatalf("Activate on a previewed recent should jump to OK, got cursor %d", n.cursor)
	}
	if n.Name() != "MARK" {
		t.Fatalf("the previewed name should stay, got %q", n.Name())
	}
	n.Do(ActionActivate)
	if !n.Done() || n.Name() != "MARK" {
		t.Errorf("one more press should submit it, got done=%v name=%q", n.Done(), n.Name())
	}
}

func TestNameEntryEraseCancelsThePreview(t *testing.T) {
	n := recentEntry(t, "MARK")
	n.slots[0] = 'Q'
	n.Do(ActionErase) // clears slot 0 back to blank, stays put
	n.tap(DirDown)
	if n.Name() != "MARK" {
		t.Fatalf("expected the preview, got %q", n.Name())
	}
	n.Do(ActionErase)
	if n.Name() != "" || n.recentIdx != -1 || n.cursor != 0 {
		t.Errorf("erase should cancel the preview in place, got name=%q idx=%d cursor=%d", n.Name(), n.recentIdx, n.cursor)
	}
}

func TestNameEntrySidewaysOutOfAPreviewKeepsIt(t *testing.T) {
	n := recentEntry(t, "MARK")
	n.tap(DirDown)
	n.tap(DirRight)
	if n.recentIdx != -1 {
		t.Fatal("Right should end the preview")
	}
	if n.Name() != "MARK" || n.cursor != 1 {
		t.Errorf("Right should keep the shown name and move on, got name=%q cursor=%d", n.Name(), n.cursor)
	}
	// And it is now ordinary text: cycling edits it in place.
	n.tap(DirUp)
	if n.Name() != "MBRK" {
		t.Errorf("a kept recent should be editable like anything else, got %q", n.Name())
	}
}

func TestNameEntryHeldDownScrollsThroughTheList(t *testing.T) {
	n := recentEntry(t, "MARK", "ZOE")
	n.Move(DirDown) // press edge: MARK
	n.Update(0)
	n.Update(360_000) // first repeat: ZOE
	if n.Name() != "ZOE" {
		t.Errorf("hold-to-scroll should walk the list like the charset, got %q", n.Name())
	}
}

func TestNameEntrySetRecentsSanitizesTruncatesAndDedupes(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.SetRecents([]string{"mark", "!!!", "", "MARK", "abcdefghij", "zoe", "bea", "kim", "pat"})
	want := []string{"MARK", "ABCDEFG", "ZOE", "BEA", "KIM"}
	if n.recentN != len(want) {
		t.Fatalf("kept %d recents, want %d (%q)", n.recentN, len(want), n.recents)
	}
	for i, w := range want {
		if n.recents[i] != w {
			t.Errorf("recent %d = %q, want %q", i, n.recents[i], w)
		}
	}
}

func TestNameEntryPeekDrawsOnlyWhereTheListIsOneScrollAway(t *testing.T) {
	peeked := func(n *NameEntry) bool {
		cv := newByteCanvas((n.maxLen+1)*nameEntrySlotWidth, ledGlyphRows+8)
		n.Draw(cv)
		for y := ledGlyphRows; y < ledGlyphRows+8; y++ {
			for x := 0; x < (n.maxLen+1)*nameEntrySlotWidth; x++ {
				if cv.at(x, y) == NameEntryPeekColor {
					return true
				}
			}
		}
		return false
	}

	if peeked(NewNameEntry(0, 0, 0)) {
		t.Error("no recents offered: nothing may be drawn under the row")
	}
	n := recentEntry(t, "MARK", "ZOE")
	if !peeked(n) {
		t.Error("a fresh entry with recents should hint at them from the first frame")
	}
	n.tap(DirUp) // slot 0 is 'A' now: Down goes to blank, not the list
	if peeked(n) {
		t.Error("the hint must not promise a list the next Down won't reach")
	}
	n.tap(DirDown) // back to blank
	n.tap(DirDown) // previewing MARK, ZOE still below
	if !peeked(n) {
		t.Error("mid-list, the next name down should still be hinted")
	}
	n.tap(DirDown) // previewing ZOE, nothing below
	if peeked(n) {
		t.Error("nothing left in the list: nothing left to peek at")
	}
}

func TestNameEntrySubmitTrimsTrailingSpaces(t *testing.T) {
	n := NewNameEntry(0, 0, 3)
	n.tap(DirUp)         // slot 0 = 'A'
	n.Do(ActionActivate) // confirm, advance to slot 1
	n.tap(DirUp)         // slot 1 = 'A'
	n.Do(ActionActivate) // confirm, advance to slot 2
	if n.Done() {
		t.Fatal("advancing should not submit yet")
	}
	n.Do(ActionActivate) // slot 2 blank -> jump to OK
	n.Do(ActionActivate) // OK -> submit
	if !n.Done() {
		t.Fatal("Activate on OK should submit")
	}
	if n.Name() != "AA" {
		t.Errorf("submitted name should be trimmed, got %q", n.Name())
	}
}

func TestNameEntryResetClearsEverything(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.tap(DirUp)
	n.Do(ActionActivate)
	n.Do(ActionActivate)
	n.Reset()
	if n.Name() != "" || n.Done() || n.cursor != 0 {
		t.Errorf("Reset should blank the entry, got name=%q done=%v cursor=%d", n.Name(), n.Done(), n.cursor)
	}
}

func TestNameEntryBlinkTogglesAtAccumulatedThreshold(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	if !n.blinkOn {
		t.Fatal("a fresh entry should start in the blink-on phase")
	}
	n.Update(300_000) // total 300ms: still inside the first 500ms phase
	if !n.blinkOn {
		t.Error("blink should still be on before 500ms accumulate")
	}
	n.Update(250_000) // total 550ms: crossed into the second phase
	if n.blinkOn {
		t.Error("blink should flip off after crossing 500ms")
	}
	n.Update(500_000) // total 1,050,000us: back to phase 0 (1050/500=2, even)
	if !n.blinkOn {
		t.Error("blink should flip erase on after crossing 1000ms")
	}
}

func nameEntryCellColors(cv *byteCanvas, cell int) map[Color]bool {
	out := map[Color]bool{}
	for y := 0; y < ledGlyphRows; y++ {
		for x := cell * nameEntrySlotWidth; x < (cell+1)*nameEntrySlotWidth; x++ {
			if c := cv.at(x, y); c != Black {
				out[c] = true
			}
		}
	}
	return out
}

func TestNameEntryDrawPalette(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.tap(DirUp)         // slot 0 = A
	n.Do(ActionActivate) // confirm slot 0, cursor -> slot 1 (blank, becomes active)

	cv := newByteCanvas((n.maxLen+1)*nameEntrySlotWidth, ledGlyphRows)
	n.blinkOn = true
	n.Draw(cv)

	if colors := nameEntryCellColors(cv, 0); !colors[NameEntryFilledColor] || len(colors) != 1 {
		t.Errorf("confirmed letter should render pure white, got %v", colors)
	}
	if colors := nameEntryCellColors(cv, 1); !colors[NameEntryActiveColor] || len(colors) != 1 {
		t.Errorf("active cell should render bright yellow on the blink-on phase, got %v", colors)
	}
	if colors := nameEntryCellColors(cv, 2); !colors[NameEntryEmptyColor] || len(colors) != 1 {
		t.Errorf("empty slot should render a dim gray underscore, got %v", colors)
	}
	if colors := nameEntryCellColors(cv, n.maxLen); !colors[NameEntryOKColor] || len(colors) != 1 {
		t.Errorf("OK cell should render a green checkmark, got %v", colors)
	}

	cv2 := newByteCanvas((n.maxLen+1)*nameEntrySlotWidth, ledGlyphRows)
	n.blinkOn = false
	n.Draw(cv2)
	if colors := nameEntryCellColors(cv2, 1); !colors[NameEntryFilledColor] || len(colors) != 1 {
		t.Errorf("active empty slot should render WHITE on the blink-off phase (never dim gray), got %v", colors)
	}
}

func TestNameEntryActiveBlankNeverGray(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.tap(DirUp)         // slot 0 = 'A'
	n.Do(ActionActivate) // confirm, cursor -> slot 1 (blank)

	for _, blink := range []bool{true, false} {
		n.blinkOn = blink
		cv := newByteCanvas((n.maxLen+1)*nameEntrySlotWidth, ledGlyphRows)
		n.Draw(cv)
		if colors := nameEntryCellColors(cv, 1); colors[NameEntryEmptyColor] {
			t.Errorf("blinkOn=%v: the active blank cell rendered dim gray — invisible cursor", blink)
		}
	}
}

// TestNameEntryGlyphsAreCenteredInTheirCells pins that every charset glyph sits
// centered in its cell, never straying into a neighbour's, with left and right
// padding within a pixel of each other.
func TestNameEntryGlyphsAreCenteredInTheirCells(t *testing.T) {
	for _, r := range nameEntryCharset {
		if r == ' ' {
			continue // no ink to center
		}
		n := NewNameEntry(0, 0, 1)
		n.slots[0] = r
		n.cursor = n.maxLen // keep the blink out of it
		cv := newByteCanvas(nameEntrySlotWidth, ledGlyphRows)
		n.Draw(cv)

		lo, hi := nameEntrySlotWidth, 0
		for y := 0; y < ledGlyphRows; y++ {
			for x := 0; x < nameEntrySlotWidth; x++ {
				if cv.at(x, y) == Black {
					continue
				}
				if x < lo {
					lo = x
				}
				if x >= hi {
					hi = x + 1
				}
			}
		}
		if lo >= hi {
			t.Errorf("%q rendered no pixels inside its cell", r)
			continue
		}
		left, right := lo, nameEntrySlotWidth-hi
		if right < 0 {
			t.Errorf("%q spills past its cell (ink columns %d..%d)", r, lo, hi-1)
			continue
		}
		if d := left - right; d > 1 || d < -1 {
			t.Errorf("%q is not centered in its cell: %d px of padding left, %d right", r, left, right)
		}
	}
}

func TestNameEntryCursorBlinkChangesPixels(t *testing.T) {
	n := NewNameEntry(0, 0, 0)
	n.tap(DirUp) // slot 0 = A, so the cursor pixels are lit

	on := newByteCanvas((n.maxLen+1)*nameEntrySlotWidth, ledGlyphRows)
	n.blinkOn = true
	n.Draw(on)

	off := newByteCanvas((n.maxLen+1)*nameEntrySlotWidth, ledGlyphRows)
	n.blinkOn = false
	n.Draw(off)

	same := true
	for y := 0; y < ledGlyphRows && same; y++ {
		for x := 0; x < nameEntrySlotWidth; x++ {
			if on.at(x, y) != off.at(x, y) {
				same = false
				break
			}
		}
	}
	if same {
		t.Error("the cursor cell should visibly differ between blink phases")
	}
}
