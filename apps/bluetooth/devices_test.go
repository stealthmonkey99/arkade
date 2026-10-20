// SPDX-License-Identifier: GPL-2.0-or-later

package bluetooth

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	bt "github.com/stealthmonkey99/arkade/internal/bluetooth"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

func anyPixel(art display.Sprite, want display.Pixel) bool {
	for _, rowPx := range art {
		for _, px := range rowPx {
			if px == want {
				return true
			}
		}
	}
	return false
}

func TestRowArtBadgesTheSeatInItsOwnColour(t *testing.T) {
	p1 := rowArt(row{dev: bt.Device{Name: "Pad"}, seat: input.SeatP1, live: true}, false)
	if !anyPixel(p1, seatP1) {
		t.Error("a P1 row is not drawn in the P1 seat colour")
	}
	if anyPixel(p1, seatP2) {
		t.Error("a P1 row carries the P2 colour")
	}

	p2 := rowArt(row{dev: bt.Device{Name: "Pad"}, seat: input.SeatP2, live: true}, false)
	if !anyPixel(p2, seatP2) {
		t.Error("a P2 row is not drawn in the P2 seat colour")
	}
}

func TestRowArtDistinguishesLiveFromRemembered(t *testing.T) {
	live := rowArt(row{dev: bt.Device{Name: "Pad"}, seat: input.SeatP1, live: true}, false)
	asleep := rowArt(row{dev: bt.Device{Name: "Pad"}, seat: input.SeatP1}, false)
	if !anyPixel(asleep, unseat) {
		t.Error("a remembered-but-unreadable pad is not drawn differently from a live one")
	}
	if sameSprite(live, asleep) {
		t.Error("live and asleep rows render identically")
	}
}

func TestRowArtHighlightsTheSelection(t *testing.T) {
	plain := rowArt(row{dev: bt.Device{Name: "Pad"}, seat: input.SeatP1}, false)
	picked := rowArt(row{dev: bt.Device{Name: "Pad"}, seat: input.SeatP1}, true)
	if sameSprite(plain, picked) {
		t.Error("the selected row is drawn the same as an unselected one")
	}
}

func TestNormalizeIDBridgesBlueZAndHidapi(t *testing.T) {
	if normalizeID("02:1A:2B:3C:4D:03") != normalizeID("02:1a:2b:3c:4d:03") {
		t.Error("BlueZ's upper-case MAC does not match hidapi's lower-case one")
	}
}

func TestChordHintNamesBothSeatsAndFits(t *testing.T) {
	if len(chordHint) > 16 {
		t.Errorf("chordHint is %d chars, wider than the LCD", len(chordHint))
	}
	for _, want := range []string{"SEL", "R1", "L1", "P1", "P2"} {
		if !strings.Contains(chordHint, want) {
			t.Errorf("chordHint %q does not mention %q", chordHint, want)
		}
	}
}

func TestSeatSelectionIsNotADirection(t *testing.T) {
	src, err := readSource("devices.go")
	if err != nil {
		t.Fatal(err)
	}
	handler := src[strings.Index(src, "func (s *devicesScene) handleKey"):]
	handler = handler[:strings.Index(handler, "\n}")]
	for _, dir := range []string{"KeyLeft:", "KeyRight:"} {
		if i := strings.Index(handler, dir); i >= 0 {
			after := handler[i:]
			if strings.Contains(after[:min2(len(after), 120)], "setSeat") || strings.Contains(after[:min2(len(after), 120)], "cycleSeat") {
				t.Errorf("%s changes the seat; seat selection must be a button", dir)
			}
		}
	}
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func readSource(name string) (string, error) {
	b, err := os.ReadFile(name)
	return string(b), err
}

func TestHintRowFitsThePanel(t *testing.T) {
	s := &devicesScene{
		list:   []row{{dev: bt.Device{Name: "Pad"}, seat: input.SeatP1}, {isAdd: true}},
		cursor: 0,
	}
	for _, tc := range []struct {
		name   string
		cursor int
	}{{"on a device", 0}, {"on the add tile", 1}} {
		s.cursor = tc.cursor
		art := s.hintRow()
		if len(art) == 0 {
			t.Fatalf("%s: empty hint row", tc.name)
		}
		for y, rowPx := range art {
			if len(rowPx) > rowW {
				t.Errorf("%s: hint row line %d is %dpx, wider than the %dpx panel", tc.name, y, len(rowPx), rowW)
			}
		}
	}
}

func TestHintColoursMatchTheirKeys(t *testing.T) {
	src, err := readSource("devices.go")
	if err != nil {
		t.Fatal(err)
	}
	handler := src[strings.Index(src, "func (s *devicesScene) handleKey"):]
	handler = handler[:strings.Index(handler, "\n}")]

	seatKey := "input.KeyJoyC"
	dropKey := "input.KeyJoyD"
	if !strings.Contains(handler, seatKey) || !strings.Contains(handler, dropKey) {
		t.Fatalf("expected the seat and drop actions on %s and %s", seatKey, dropKey)
	}
	seatIdx, dropIdx := strings.Index(handler, seatKey), strings.Index(handler, dropKey)
	seatIsC := seatIdx < dropIdx

	hints := src[strings.Index(src, "func (s *devicesScene) hintRow"):]
	hints = hints[:strings.Index(hints, "\n}")]
	wantSeat, wantDrop := "ButtonYellow", "ButtonBlue" // C, D
	if !seatIsC {
		wantSeat, wantDrop = "ButtonBlue", "ButtonYellow"
	}
	if !strings.Contains(hints, `Text: "SEAT", Color: ui.`+wantSeat) {
		t.Errorf("the SEAT hint is not drawn in its own button's colour (want %s)", wantSeat)
	}
	if !strings.Contains(hints, `Text: "DROP", Color: ui.`+wantDrop) {
		t.Errorf("the DROP hint is not drawn in its own button's colour (want %s)", wantDrop)
	}
}

func TestHoldingADirectionScrolls(t *testing.T) {
	s := &devicesScene{list: make([]row, 8)}
	s.hold.Hold(ui.DirDown)

	s.tickHold(clock.FrameTime{ElapsedMs: ui.HoldRepeatDelayMicros / 1000})
	if s.cursor != 1 {
		t.Fatalf("cursor %d after the initial hold delay, want 1", s.cursor)
	}
	for i := 0; i < 3; i++ {
		s.tickHold(clock.FrameTime{ElapsedMs: ui.HoldRepeatIntervalMicros / 1000})
	}
	if s.cursor != 4 {
		t.Errorf("cursor %d after three repeat intervals, want 4", s.cursor)
	}

	for i := 0; i < 20; i++ {
		s.tickHold(clock.FrameTime{ElapsedMs: ui.HoldRepeatIntervalMicros / 1000})
	}
	if s.cursor != len(s.list)-1 {
		t.Errorf("cursor %d, want it clamped at the last row %d", s.cursor, len(s.list)-1)
	}
}

func TestReleasingTheStickStopsTheScroll(t *testing.T) {
	s := &devicesScene{list: make([]row, 8)}
	s.hold.Hold(ui.DirDown)
	s.tickHold(clock.FrameTime{ElapsedMs: ui.HoldRepeatDelayMicros / 1000})
	at := s.cursor
	s.hold.Reset()
	for i := 0; i < 10; i++ {
		s.tickHold(clock.FrameTime{ElapsedMs: ui.HoldRepeatIntervalMicros / 1000})
	}
	if s.cursor != at {
		t.Errorf("cursor moved from %d to %d after the stick was released", at, s.cursor)
	}
}

func TestScanReturnsToTheListOnItsOwn(t *testing.T) {
	if doneLingerMs <= 0 {
		t.Fatal("a finished pairing never returns to the list")
	}
	if doneLingerMs > 4*time.Second {
		t.Errorf("the outcome lingers %v; long enough to read is not long enough to doubt", doneLingerMs)
	}
}

func TestDropAsksOnScreenWithGreenAndRed(t *testing.T) {
	s := &devicesScene{
		list:   []row{{dev: bt.Device{Name: "Wireless Controller", Address: "aa:bb"}, seat: input.SeatP1}, {isAdd: true}},
		cursor: 0,
	}

	s.askDrop()
	if s.confirm == "" {
		t.Fatal("the drop was not raised for confirmation")
	}

	art := confirmArt(bt.DisplayName(s.list[0].dev), confirmW, confirmH)
	if len(art) != confirmH || len(art[0]) != confirmW {
		t.Fatalf("confirmation is %dx%d, want %dx%d", len(art[0]), len(art), confirmW, confirmH)
	}
	if confirmW > rowW+8 || confirmH > 40 {
		t.Errorf("the confirmation does not fit the panel (%dx%d)", confirmW, confirmH)
	}
	if !anyPixel(art, display.FromUI(ui.ButtonGreen)) {
		t.Error("no green YES on the confirmation")
	}
	if !anyPixel(art, display.FromUI(ui.ButtonRed)) {
		t.Error("no red NO on the confirmation")
	}
}

func TestRedCancelsTheDropButStillBacksOut(t *testing.T) {
	s := &devicesScene{
		a:      &app{},
		list:   []row{{dev: bt.Device{Name: "Pad", Address: "aa:bb"}, seat: input.SeatP1}},
		cursor: 0,
	}
	s.confirm = "aa:bb"
	if !s.handleKey(input.KeyJoyB) {
		t.Error("red did not cancel the confirmation")
	}
	if s.confirm != "" {
		t.Error("the confirmation survived a red press")
	}
	if s.handleKey(input.KeyJoyB) {
		t.Error("red was swallowed with nothing to cancel; back must reach the app")
	}
}

func TestHoldDoesNotScrollBehindTheConfirmation(t *testing.T) {
	s := &devicesScene{list: make([]row, 8)}
	s.confirm = "aa:bb"
	s.hold.Hold(ui.DirDown)
	for i := 0; i < 8; i++ {
		s.tickHold(clock.FrameTime{ElapsedMs: ui.HoldRepeatDelayMicros / 1000})
	}
	if s.cursor != 0 {
		t.Errorf("the list scrolled to %d while a confirmation was up", s.cursor)
	}
}

func TestUnsupportedPairerReportsDisconnect(t *testing.T) {
	p := bt.Unavailable("no radio")
	n, err := p.DisconnectAll(context.Background())
	if n != 0 {
		t.Errorf("disconnected %d devices on a cabinet with no radio", n)
	}
	if err == nil {
		t.Error("DisconnectAll reported success with no radio present")
	}
}

func TestAMacAliasRowIsDimmedLikeAnyAnonymousOne(t *testing.T) {
	anon := scanRowArt(bt.Device{Address: "0A:1B:2C:3D:4E:5F", Name: "0A-1B-2C-3D-4E-5F"}, false)
	bare := scanRowArt(bt.Device{Address: "0A:1B:2C:3D:4E:5F"}, false)
	named := scanRowArt(bt.Device{Address: "AA:01", Name: "Speaker"}, false)

	bright := display.Pixel{R: 200, G: 200, B: 200, A: 255}
	if anyPixel(anon, bright) {
		t.Error("a MAC-shaped alias is drawn as brightly as a real name")
	}
	if !anyPixel(named, bright) {
		t.Error("a real name is not drawn brightly")
	}
	dim := display.Pixel{R: 110, G: 110, B: 110, A: 255}
	if !anyPixel(anon, dim) || !anyPixel(bare, dim) {
		t.Error("an anonymous row is not dimmed")
	}
}

func TestTheListFitsOnThePanel(t *testing.T) {
	lastRowBottom := listTop + listSlots*rowH // exclusive
	if lastRowBottom > hintTop {
		t.Errorf("the list ends at %d and the hint starts at %d; the last row is under it", lastRowBottom, hintTop)
	}
	if hintTop+hintH > panelH {
		t.Errorf("the hint ends at %d on a %dpx panel", hintTop+hintH, panelH)
	}
	if listSlots < 4 {
		t.Errorf("only %d rows fit; the list is too short to be worth scrolling", listSlots)
	}
	if btRows != listSlots {
		t.Errorf("the scan screen draws %d rows and the device list %d", btRows, listSlots)
	}
}

func TestEveryRowSlotIsOnScreen(t *testing.T) {
	for i := 0; i < listSlots; i++ {
		top := listTop + i*rowH
		if bottom := top + rowH; bottom > panelH {
			t.Errorf("row %d spans %d..%d, past the %dpx panel", i, top, bottom-1, panelH)
		}
	}
}

func TestLampsMatchWhatEachStateAccepts(t *testing.T) {
	live := row{dev: bt.Device{Name: "Pad", Address: "aa:bb"}, seat: input.SeatP1, live: true}
	asleep := row{dev: bt.Device{Name: "Pad", Address: "aa:bb"}, seat: input.SeatP1}
	add := row{isAdd: true}

	cases := []struct {
		name string
		set  func(*devicesScene)
		want lampSet
		why  string
	}{
		{
			"drop confirmation",
			func(s *devicesScene) { s.list = []row{live}; s.confirm = "aa:bb" },
			lampSet{a: true, b: true},
			"only green (yes) and red (no) are answered while the question is up",
		},
		{
			"the add tile",
			func(s *devicesScene) { s.list = []row{add} },
			lampSet{a: true, b: true},
			"green scans and red backs out; there is no device to seat or drop",
		},
		{
			"a live pad",
			func(s *devicesScene) { s.list = []row{live} },
			lampSet{b: true, c: true, d: true},
			"green wakes a sleeping pad, and this one is already awake",
		},
		{
			"a sleeping pad",
			func(s *devicesScene) { s.list = []row{asleep} },
			lampSet{a: true, b: true, c: true, d: true},
			"green wakes it, yellow seats it, blue drops it, red backs out",
		},
	}
	for _, tc := range cases {
		s := &devicesScene{}
		tc.set(s)
		if got := s.wantLamps(); got != tc.want {
			t.Errorf("%s: lamps %+v, want %+v — %s", tc.name, got, tc.want, tc.why)
		}
	}
}

func TestEveryLitLampHasAHandler(t *testing.T) {
	src, err := readSource("devices.go")
	if err != nil {
		t.Fatal(err)
	}
	handler := src[strings.Index(src, "func (s *devicesScene) handleKey"):]
	handler = handler[:strings.Index(handler, "\n}")]

	for _, want := range []struct{ lamp, key string }{
		{"green", "input.KeyJoyA"},
		{"red", "input.KeyJoyB"},
		{"yellow", "input.KeyJoyC"},
		{"blue", "input.KeyJoyD"},
	} {
		if !strings.Contains(handler, want.key) {
			t.Errorf("the %s lamp is lit somewhere but handleKey has no %s case", want.lamp, want.key)
		}
	}
}

func TestScanLampsFollowItsPhase(t *testing.T) {
	s := &scanScene{}
	devs := []bt.Device{{Address: "aa:bb", Name: "Pad", Input: true}}

	if got := s.wantLamps(btScanning, nil, 0); got != (lampSet{b: true}) {
		t.Errorf("scanning with nothing found: %+v, want red only", got)
	}
	if got := s.wantLamps(btListing, devs, 0); got != (lampSet{a: true, b: true}) {
		t.Errorf("a device under the cursor: %+v, want green and red", got)
	}
	if got := s.wantLamps(btPairing, devs, 0); got != (lampSet{b: true}) {
		t.Errorf("mid-pairing: %+v, want red only — the screen is locked", got)
	}
}
