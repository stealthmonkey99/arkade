// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import (
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/ui/input"
)

func rec(kind, key uint8, flags uint8) input.Record {
	return input.Record{Kind: kind, Key: key, Flags: flags}
}

func stick(joy uint8) input.Record {
	return input.Record{Kind: input.KindJoystick, Joy: joy, Source: input.SourceController}
}

// Feed must leave the same held state as the hand-written
// KeyDown/KeyUp/Pulse/SetJoy glue after every record of one stream.
func TestHeldDirsFeedMatchesHandGlue(t *testing.T) {
	stream := []input.Record{
		rec(input.KindKeydown, input.KeyLeft, 0),
		rec(input.KindKeypress, input.KeyLeft, 0),
		rec(input.KindKeyup, input.KeyLeft, 0),
		rec(input.KindKeydown, input.KeyUp, 0),
		rec(input.KindKeypress, input.KeyUp, input.FlagVirtual),
		rec(input.KindKeyup, input.KeyUp, 0),
		rec(input.KindKeypress, input.KeyRight, input.FlagVirtual),
		stick(input.JoyS),
		stick(input.JoyNE),
		stick(input.JoyW),
		stick(input.JoyCenter),
		rec(input.KindKeypress, input.KeyJoyA, 0),
		rec(input.KindKeydown, input.KeyJoyB, 0),
	}
	var fed, glued HeldDirs
	dirs := []Direction{DirUp, DirDown, DirLeft, DirRight}
	for i, r := range stream {
		fed.Feed(r)
		handGlue(&glued, r)
		fed.Update(50_000)
		glued.Update(50_000)
		for _, d := range dirs {
			if fed.Held(d) != glued.Held(d) {
				t.Fatalf("record %d: Held(%v) fed=%v glued=%v", i, d, fed.Held(d), glued.Held(d))
			}
		}
		if fed.Current() != glued.Current() {
			t.Fatalf("record %d: Current fed=%v glued=%v", i, fed.Current(), glued.Current())
		}
	}
}

func handGlue(h *HeldDirs, r input.Record) {
	keyDir := map[uint8]Direction{input.KeyUp: DirUp, input.KeyDown: DirDown, input.KeyLeft: DirLeft, input.KeyRight: DirRight}
	joyDir := map[uint8]Direction{input.JoyN: DirUp, input.JoyS: DirDown, input.JoyW: DirLeft, input.JoyE: DirRight}
	if d, ok := keyDir[r.Key]; ok && r.Kind != input.KindJoystick {
		switch r.Kind {
		case input.KindKeydown:
			h.KeyDown(d)
		case input.KindKeyup:
			h.KeyUp(d)
		case input.KindKeypress:
			if r.Flags&input.FlagVirtual != 0 {
				h.Pulse(d)
			}
		}
	}
	if r.Kind == input.KindJoystick {
		h.SetJoy(joyDir[r.Joy])
	}
}

func TestHoldConfirmFeedRealButton(t *testing.T) {
	var h HoldConfirm
	if h.Feed(rec(input.KindKeydown, input.KeyJoyB, 0)) {
		t.Fatal("Feed on a closed overlay reported true")
	}
	h.Open()
	if !h.Feed(rec(input.KindKeypress, input.KeyJoyB, 0)) {
		t.Fatal("opening press not consumed")
	}
	h.Feed(rec(input.KindKeyup, input.KeyJoyB, 0))
	if !h.Armed() {
		t.Fatal("keyup of the opening press did not arm the hold")
	}
	h.Feed(rec(input.KindKeydown, input.KeyJoyB, 0))
	if !h.Holding() {
		t.Fatal("keydown after arming is not a hold")
	}
	if h.Feed(rec(input.KindKeypress, input.KeyJoyA, 0)) {
		t.Error("a non-quit press was consumed; the caller could not cancel")
	}
}

func TestHoldConfirmFeedKeyboardPulses(t *testing.T) {
	var h HoldConfirm
	h.Open()
	h.Feed(rec(input.KindKeypress, input.KeyEscape, input.FlagVirtual))
	if !h.Holding() {
		t.Fatal("a virtual keypress granted no credit")
	}
	h.Update(700_000) // credit lapses: the keyboard's release, which arms
	if !h.Armed() {
		t.Fatal("credit expiry did not arm the hold")
	}
	h.Feed(rec(input.KindKeypress, input.KeyQuit, input.FlagVirtual))
	if !h.Holding() {
		t.Fatal("KeyQuit pulse is not a hold")
	}
	h.Feed(rec(input.KindKeypress, input.KeyJoyB, 0))
	if h.virtualLeft == 0 {
		t.Error("a non-virtual keypress dropped virtual credit")
	}
}
