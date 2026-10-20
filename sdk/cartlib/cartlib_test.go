// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

import "testing"

// Pure logic only: every host call panics under the plain Go toolchain.

func TestTextWidth(t *testing.T) {
	cases := []struct {
		n, scale, want int
	}{
		{0, 1, 0},
		{1, 1, glyphW},
		{5, 1, 5*glyphAdvance - 1},
		{3, 2, 3*glyphAdvance*2 - 2},
	}
	for _, c := range cases {
		if got := TextWidth(c.n, c.scale); got != c.want {
			t.Errorf("TextWidth(%d, %d) = %d, want %d", c.n, c.scale, got, c.want)
		}
	}
}

func TestNumDigits(t *testing.T) {
	cases := []struct {
		v    uint32
		want int
	}{
		{0, 1},
		{9, 1},
		{10, 2},
		{999, 3},
		{1000, 4},
		{4294967295, 10},
	}
	for _, c := range cases {
		if got := NumDigits(c.v); got != c.want {
			t.Errorf("NumDigits(%d) = %d, want %d", c.v, got, c.want)
		}
	}
}

func TestAppendUint(t *testing.T) {
	cases := []struct {
		buf     string
		pos     int
		v       uint32
		want    string
		wantEnd int
	}{
		{"........", 0, 0, "0.......", 1},
		{"........", 2, 407, "..407...", 5},
		{"............", 1, 4294967295, ".4294967295.", 11},
		{"........", 0, 4294967295, "94967295", 8},
		{".....", 3, 12345, "...45", 5},
		{"....", 4, 7, "....", 4},
		{"....", 6, 7, "....", 6},
	}
	for _, c := range cases {
		buf := []byte(c.buf)
		end := AppendUint(buf, c.pos, c.v)
		if string(buf) != c.want || end != c.wantEnd {
			t.Errorf("AppendUint(%q, %d, %d) = %d leaving %q, want %d leaving %q", c.buf, c.pos, c.v, end, buf, c.wantEnd, c.want)
		}
	}
}

func TestDrawTextIntoFramebuffer(t *testing.T) {
	var fb Framebuffer
	DrawText(&fb, 0, 0, "HI", 1, 255, 255, 255)

	lit := 0
	for i := 0; i < len(fb.Pix); i += 4 {
		if fb.Pix[i] != 0 || fb.Pix[i+1] != 0 || fb.Pix[i+2] != 0 {
			lit++
		}
	}
	if lit == 0 {
		t.Fatal("DrawText lit no pixels")
	}
}

func TestFramebufferClearAndSetPixelClipping(t *testing.T) {
	var fb Framebuffer
	fb.Clear(1, 2, 3)
	if fb.Pix[0] != 1 || fb.Pix[1] != 2 || fb.Pix[2] != 3 || fb.Pix[3] != 0xFF {
		t.Fatalf("Clear did not set the first pixel: %v", fb.Pix[:4])
	}

	fb.SetPixel(-1, -1, 9, 9, 9)
	fb.SetPixel(FBWidth, FBHeight, 9, 9, 9)
	fb.SetPixel(1000, 1000, 9, 9, 9)

	fb.SetPixel(0, 0, 10, 20, 30)
	if fb.Pix[0] != 10 || fb.Pix[1] != 20 || fb.Pix[2] != 30 || fb.Pix[3] != 0xFF {
		t.Fatalf("SetPixel(0,0,...) = %v", fb.Pix[:4])
	}
}

func TestDecodeInputRecords(t *testing.T) {
	wire := []byte{
		KindKeypress, KeyJoyA, 1, JoyCenter, 0, 0, 0, 0,
		KindJoystick, 0, 2, JoyS, 0, 0, 0, 0,
	}
	got := DecodeInputRecords(wire)
	want := []InputRecord{
		{Kind: KindKeypress, Key: KeyJoyA, Player: 1, Joy: JoyCenter},
		{Kind: KindJoystick, Key: 0, Player: 2, Joy: JoyS},
	}
	if len(got) != len(want) {
		t.Fatalf("DecodeInputRecords: got %d records, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("record %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDecodeInputRecordsIntoCapsAtOutputLen(t *testing.T) {
	wire := make([]byte, 8*5)
	for i := range wire {
		wire[i] = byte(i)
	}
	var out [2]InputRecord
	n := DecodeInputRecordsInto(wire, out[:])
	if n != 2 {
		t.Fatalf("DecodeInputRecordsInto returned n=%d, want 2 (capped by len(out))", n)
	}
}

type gameStub struct {
	inited bool
	setup  *Setup
}

func (g *gameStub) Init(s *Setup)             { g.inited = true; g.setup = s }
func (g *gameStub) HandleInput([]InputRecord) {}
func (g *gameStub) Update(uint64)             {}
func (g *gameStub) Render(*Framebuffer)       {}
func (g *gameStub) Manifest() []byte          { return []byte(`{"abi":1,"slug":"stub","name":"Stub"}`) }

var _ Game = (*gameStub)(nil)

func TestRunRegistersGame(t *testing.T) {
	g := &gameStub{}
	if got := Run(g); got != 0 {
		t.Fatalf("Run returned %d, want 0", got)
	}
	if currentGame != Game(g) {
		t.Fatal("Run did not register the game as currentGame")
	}
	dispatchInit()
	if !g.inited || g.setup != &setup {
		t.Fatalf("dispatchInit did not reach the registered Game with the live Setup: %+v", g)
	}
}

// TestPointerPassingAllocatesNothing pins ptrOfString, which every wrapper
// passes strings through, since the wrappers themselves panic under go test.
func TestPointerPassingAllocatesNothing(t *testing.T) {
	const constant = "READY"
	built := string(append([]byte("Gen: "), '4', '2')) // heap-backed, not a constant
	buf := [16]byte{'P', 'o', 'p', ':'}

	cases := []struct {
		name string
		fn   func()
	}{
		{"constant", func() { sinkPtr = ptrOfString(constant) }},
		{"heap string", func() { sinkPtr = ptrOfString(built) }},
		{"empty string", func() { sinkPtr = ptrOfString("") }},
		{"byte slice", func() { sinkPtr = ptrOf(buf[:4]) }},
		{"empty byte slice", func() { sinkPtr = ptrOf(nil) }},
	}
	for _, c := range cases {
		if n := testing.AllocsPerRun(100, c.fn); n != 0 {
			t.Errorf("%s: %v allocations per run, want 0", c.name, n)
		}
	}

	// A copy would give a fresh address per call. The u32 is never
	// dereferenced: it truncates on this 64-bit host.
	if a, b := ptrOfString(built), ptrOfString(built); a != b {
		t.Errorf("ptrOfString returned %#x then %#x for the same string — it is copying, not handing over", a, b)
	}
	if a, b := ptrOf(buf[:4]), ptrOf(buf[:4]); a != b {
		t.Errorf("ptrOf returned %#x then %#x for the same slice — it is copying, not handing over", a, b)
	}
	if ptrOfString("") != ptrOf(nil) {
		t.Error("the empty string and the empty slice must land on the same emptyBytes backing byte")
	}
}

// sinkPtr keeps the AllocsPerRun closures from being optimized away.
var sinkPtr uint32

func TestPackLampLevelsKeepsEachLevelInItsOwnNibble(t *testing.T) {
	cases := []struct {
		name   string
		levels LampLevels
		want   uint32
	}{
		{"dark", LampLevels{}, 0},
		{"full", LampLevels{15, 15, 15, 15, 15, 15}, 0x00ffffff},
		{"ascending", LampLevels{1, 2, 3, 4, 5, 6}, 0x00654321},
		{"joya alone", LampLevels{LampLevelMax}, 0x0000000f},
		{"over max wraps within its own field", LampLevels{0x1f, 1}, 0x0000001f},
	}
	for _, c := range cases {
		if got := PackLampLevels(c.levels); got != c.want {
			t.Errorf("%s: PackLampLevels(%v) = %#08x, want %#08x", c.name, c.levels, got, c.want)
		}
	}
}
