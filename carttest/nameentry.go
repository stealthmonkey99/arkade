// SPDX-License-Identifier: GPL-2.0-or-later

package carttest

import (
	"context"
	"testing"
)

// NameEntryB is the shared regression harness for the rule that a stray B
// press during a qualifying name entry must not discard the placement. A
// single B must neither leave the cartridge nor submit a score; the full
// press, release, hold gesture (ui.HoldConfirm) leaves without submitting.
// What the overlay looks like, and that any other button cancels back to
// typing, are left to each cartridge's own tests.
type NameEntryB struct {
	// Reach returns a fresh cartridge parked on a qualifying name entry, with
	// the FakeHost that recorded its journey. It is called more than once and
	// must build a new instance each time.
	Reach func(t *testing.T) (*Cartridge, *FakeHost)

	// Tick advances the cartridge n frames with the same dt and Update/Render
	// cadence as the cartridge's own golden suite.
	Tick func(t *testing.T, cart *Cartridge, n int)

	// Player is the record's Player byte (the pairing slot, 1 or 2) and
	// Physical its Physical byte (the cabinet port, 0 or 1). Both default to 0;
	// set them to match the cartridge's own press helper.
	Player   uint8
	Physical uint8

	// Flags is the Flags byte set on every record, for carts whose press
	// helpers stamp RecordFlagVirtual.
	Flags uint8

	// HoldTicks is how many frames the final B is held; 0 means 60. It must
	// cover HoldConfirm's 1 s fill plus its blink at the cartridge's dt.
	HoldTicks int
}

func (s NameEntryB) bRecord(kind uint8) InputRecord {
	return InputRecord{
		Kind:     kind,
		Key:      KeyJoyB,
		Player:   s.Player,
		Physical: s.Physical,
		Flags:    s.Flags,
	}
}

func (s NameEntryB) press(t *testing.T, cart *Cartridge, rec InputRecord) {
	t.Helper()
	if err := cart.DispatchInput(context.Background(), []InputRecord{rec}); err != nil {
		t.Fatalf("DispatchInput(B kind=%d): %v", rec.Kind, err)
	}
}

// Run asserts the contract in two phases: a bare B and then a mash of six
// must not leave the cartridge or submit a score; the full HoldB gesture must
// leave with nothing submitted.
func (s NameEntryB) Run(t *testing.T) {
	t.Helper()

	cart, host := s.Reach(t)
	if host.Exits != 0 || host.ScoreCount() != 0 {
		t.Fatalf("setup: reached name entry with Exits=%d ScoreCount=%d, want 0 and 0 — "+
			"Reach is not parked on an unsubmitted qualifying entry", host.Exits, host.ScoreCount())
	}
	for i := 0; i < 6; i++ {
		s.press(t, cart, s.bRecord(RecordKindKeydown))
		s.press(t, cart, s.bRecord(RecordKindKeypress))
		s.press(t, cart, s.bRecord(RecordKindKeyup))
		s.Tick(t, cart, 2)
		if host.Exits != 0 {
			t.Fatalf("bare B press #%d left the cartridge — a fumbled B must never discard a placement", i+1)
		}
	}
	if host.ScoreCount() != 0 {
		t.Errorf("score table has %d rows after a B mash during name entry, want 0 (nothing was confirmed)",
			host.ScoreCount())
	}

	holdCart, holdHost := s.Reach(t)
	s.HoldB(t, holdCart)
	if holdHost.Exits == 0 {
		t.Error("a full press-release-hold B during name entry did not leave the cartridge — " +
			"the deliberate quit gesture has to still work, or the modal is a trap")
	}
	if holdHost.ScoreCount() != 0 {
		t.Errorf("holding B out of name entry submitted %d rows, want 0 — quitting abandons the placement",
			holdHost.ScoreCount())
	}
}

// HoldB performs the full quit gesture on a cartridge in name entry: a press
// that opens the overlay, a release that arms it, then a second press held
// through the fill and confirm blink. Exported for a cartridge's own
// assertions.
func (s NameEntryB) HoldB(t *testing.T, cart *Cartridge) {
	t.Helper()
	s.press(t, cart, s.bRecord(RecordKindKeydown))
	s.press(t, cart, s.bRecord(RecordKindKeypress)) // opens the overlay
	s.press(t, cart, s.bRecord(RecordKindKeyup))    // arms the hold
	s.Tick(t, cart, 2)

	hold := s.HoldTicks
	if hold <= 0 {
		hold = 60
	}
	s.press(t, cart, s.bRecord(RecordKindKeydown)) // the hold proper
	s.Tick(t, cart, hold)
	s.press(t, cart, s.bRecord(RecordKindKeyup))
	s.Tick(t, cart, 2)
}
