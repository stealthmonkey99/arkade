// SPDX-License-Identifier: GPL-2.0-or-later

package input

import "testing"

func TestPressDirCountsAStickPushOnce(t *testing.T) {
	stick := Record{Kind: KindJoystick, Joy: JoyS, Source: SourceController}
	twin := Record{Kind: KindKeypress, Key: KeyDown, Source: SourceController}
	keyboard := Record{Kind: KindKeypress, Key: KeyDown, Flags: FlagVirtual}

	if got := stick.PressDir(); got != DirDown {
		t.Errorf("stick record PressDir = %v, want DirDown", got)
	}
	if got := twin.PressDir(); got != DirNone {
		t.Errorf("real stick's keypress twin PressDir = %v, want DirNone", got)
	}
	if got := keyboard.PressDir(); got != DirDown {
		t.Errorf("keyboard arrow PressDir = %v, want DirDown", got)
	}
	if got := twin.Dir(); got != DirDown {
		t.Errorf("twin Dir = %v, want DirDown (the level decode keeps it)", got)
	}
}

func TestDirIgnoresDiagonalsAndCentre(t *testing.T) {
	for _, j := range []uint8{JoyCenter, JoyNE, JoySE, JoySW, JoyNW} {
		if got := (Record{Kind: KindJoystick, Joy: j}).Dir(); got != DirNone {
			t.Errorf("Joy %d Dir = %v, want DirNone", j, got)
		}
	}
	for _, k := range []uint8{KeyUp, KeyDown, KeyLeft, KeyRight} {
		for _, kind := range []uint8{KindKeydown, KindKeyup} {
			r := Record{Kind: kind, Key: k, Flags: FlagVirtual}
			if r.Dir() == DirNone {
				t.Errorf("kind %d key %d: Dir is DirNone, want a direction", kind, k)
			}
			if r.PressDir() != DirNone {
				t.Errorf("kind %d key %d: PressDir is a direction, want DirNone", kind, k)
			}
		}
	}
}

func TestIsQuit(t *testing.T) {
	for _, k := range []uint8{KeyQuit, KeyEscape, KeyJoyB} {
		for _, kind := range []uint8{KindKeydown, KindKeyup, KindKeypress} {
			if !(Record{Kind: kind, Key: k}).IsQuit() {
				t.Errorf("kind %d key %d: IsQuit false", kind, k)
			}
		}
	}
	if (Record{Kind: KindKeypress, Key: KeyJoyA}).IsQuit() {
		t.Error("JoyA reported as a quit key")
	}
	if (Record{Kind: KindJoystick, Key: KeyJoyB}).IsQuit() {
		t.Error("a joystick record reported as a quit key")
	}
}

func TestSeatOther(t *testing.T) {
	cases := map[Seat]Seat{SeatP1: SeatP2, SeatP2: SeatP1, SeatBoth: SeatBoth, SeatNone: SeatNone}
	for in, want := range cases {
		if got := in.Other(); got != want {
			t.Errorf("%d.Other() = %d, want %d", in, got, want)
		}
	}
	if SeatBoth.Single() || SeatNone.Single() || !SeatP1.Single() || !SeatP2.Single() {
		t.Error("Single disagrees with its doc")
	}
}
