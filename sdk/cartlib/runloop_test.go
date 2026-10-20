// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

import (
	"strings"
	"testing"
)

type quitOwnerStub struct {
	gameStub
	owns bool
}

func (g *quitOwnerStub) OwnsQuitKeys() bool { return g.owns }

func TestQuitKeysBelongToCartlibUnlessTheGameOwnsThem(t *testing.T) {
	if ownsQuitKeys(&gameStub{}) {
		t.Error("a plain Game was treated as owning its quit keys")
	}
	if ownsQuitKeys(&quitOwnerStub{owns: false}) {
		t.Error("a QuitOwner returning false was treated as owning its quit keys")
	}
	if !ownsQuitKeys(&quitOwnerStub{owns: true}) {
		t.Error("a QuitOwner returning true was overridden")
	}
}

func TestQuitPressedOnlyOnAQuitKeypress(t *testing.T) {
	cases := []struct {
		name string
		recs []InputRecord
		want bool
	}{
		{"empty", nil, false},
		{"B press", []InputRecord{{Kind: KindKeypress, Key: KeyJoyB}}, true},
		{"escape press among others", []InputRecord{{Kind: KindKeydown, Key: KeyEscape}, {Kind: KindKeypress, Key: KeyEscape}}, true},
		{"q press", []InputRecord{{Kind: KindKeypress, Key: KeyQuit, Flags: FlagVirtual}}, true},
		{"B keydown and keyup only", []InputRecord{{Kind: KindKeydown, Key: KeyJoyB}, {Kind: KindKeyup, Key: KeyJoyB}}, false},
		{"A press", []InputRecord{{Kind: KindKeypress, Key: KeyJoyA}}, false},
		{"stick", []InputRecord{{Kind: KindJoystick, Key: KeyJoyB, Joy: JoyS}}, false},
	}
	for _, c := range cases {
		if got := quitPressed(c.recs); got != c.want {
			t.Errorf("%s: quitPressed = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestStorageWriteRefusesWithoutCallingTheHost relies on host_stub.go: with
// no host installed any host call panics, so a false return that did not
// panic is a write that never left the guest.
func TestStorageWriteRefusesWithoutCallingTheHost(t *testing.T) {
	cases := map[string][]byte{
		"":                                      nil,
		".":                                     nil,
		"..":                                    nil,
		"a/b":                                   nil,
		"spaced key":                            nil,
		strings.Repeat("k", MaxStorageKeyLen+1): nil,
		"ok-key":                                make([]byte, MaxStorageValueLen+1),
	}
	for key, val := range cases {
		if StorageWrite(key, val) {
			t.Errorf("StorageWrite(%q, %d bytes) = true, want false", key, len(val))
		}
	}
	if !StorageKeyOK(strings.Repeat("k", MaxStorageKeyLen)) || !StorageKeyOK("Life_v2.seeds-1") {
		t.Error("a legal key was refused")
	}
}

func TestSeatlessPeripheralCallsNeverReachTheHost(t *testing.T) {
	SetText(SeatNone, "A", "B")
	SetTextBytes(SeatNone, nil, nil)
	SetButtonLights(SeatNone, MaskJoyA, true)
	SetLights(SeatNone, true)
	SetLampLevels(SeatNone, LampLevels{}, MaskJoyA, 0)
	StopLights(SeatNone)
	LockPlayerToSeat(SeatBoth)
	NoteRecentName(SeatNone, "ADA")
	if PlayLights("assets/x.arkadelights.json", SeatNone, LightsLoopFile) {
		t.Error("PlayLights(SeatNone) reported playing")
	}
	if _, ok := RecentName(SeatBoth, make([]byte, MaxNameLen)); ok {
		t.Error("RecentName(SeatBoth) reported a name")
	}
}

func TestSeatArg(t *testing.T) {
	cases := []struct {
		seat Seat
		arg  int32
		ok   bool
	}{
		{SeatP1, 0, true}, {SeatP2, 1, true}, {SeatBoth, -1, true}, {SeatNone, 0, false}, {Seat(7), 0, false},
	}
	for _, c := range cases {
		if arg, ok := seatArg(c.seat); arg != c.arg || ok != c.ok {
			t.Errorf("seatArg(%d) = %d, %v; want %d, %v", c.seat, arg, ok, c.arg, c.ok)
		}
	}
}

func TestAddRecentNameDedupesWithoutAllocating(t *testing.T) {
	n := 0
	n = addRecentName(n, []byte("ADA"))
	n = addRecentName(n, []byte(""))
	n = addRecentName(n, []byte("ADA"))
	n = addRecentName(n, []byte("BOB"))
	if n != 2 || recentNameList[0] != "ADA" || recentNameList[1] != "BOB" {
		t.Fatalf("list = %q, want [ADA BOB]", recentNameList[:n])
	}
	name := []byte("CYD")
	if allocs := testing.AllocsPerRun(50, func() { addRecentName(2, name) }); allocs != 0 {
		t.Errorf("addRecentName allocated %v times per call", allocs)
	}
}

type loaderStub struct {
	gameStub
	ready bool
	seen  uint64
}

func (g *loaderStub) LoadFrame(elapsed uint64, fb *Framebuffer) bool {
	g.seen = elapsed
	return g.ready
}

func TestLoadPhaseHoldsTheGameForTheSplash(t *testing.T) {
	g := &loaderStub{ready: true}
	var p loadPhase
	var fb Framebuffer

	if p.tick(300_000) {
		t.Fatal("the game ticked during the load phase")
	}
	if !p.draw(g, &fb) || g.seen != 300_000 {
		t.Fatalf("draw did not show the loader at 300ms (saw %d)", g.seen)
	}
	if p.tick(300_000) {
		t.Fatal("a ready loader ended the phase before MinSplashMicros")
	}
	if !p.draw(g, &fb) {
		t.Fatal("the frame that ends the phase is still a splash frame")
	}
	if !p.tick(16_000) || p.draw(g, &fb) {
		t.Fatal("the load phase did not end once ready past MinSplashMicros")
	}

	g.ready = false
	p = loadPhase{}
	p.tick(MinSplashMicros * 4)
	p.draw(g, &fb)
	if p.tick(0) {
		t.Fatal("a loader that is not ready was cut short")
	}
}
