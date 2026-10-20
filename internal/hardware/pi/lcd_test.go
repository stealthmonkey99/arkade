// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"bytes"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

type hdNibble struct {
	nib byte
	rs  bool
}

func decodeNibbles(t *testing.T, recs []txRecord) []hdNibble {
	return decodeNibblesWith(t, recs, true)
}

func decodeNibblesWith(t *testing.T, recs []txRecord, lit bool) []hdNibble {
	t.Helper()
	var out []hdNibble
	for i := 0; i < len(recs); i++ {
		b := recs[i].w[0]
		if lit && b&pcfBacklight == 0 {
			t.Fatalf("write %d: backlight bit dropped (%#x)", i, b)
		}
		if b&pcfEnable == 0 {
			if !lit {
				continue
			}
			t.Fatalf("write %d: expected enable-high strobe first (%#x)", i, b)
		}
		if i+1 >= len(recs) || recs[i+1].w[0] != b&^pcfEnable {
			t.Fatalf("write %d: strobe %#x not followed by enable-low copy", i, b)
		}
		out = append(out, hdNibble{nib: b >> 4, rs: b&pcfRS != 0})
		i++ // skip the enable-low half
	}
	return out
}

type hdByte struct {
	b  byte
	rs bool
}

func pairBytes(t *testing.T, nibs []hdNibble) []hdByte {
	t.Helper()
	if len(nibs)%2 != 0 {
		t.Fatalf("odd nibble count %d", len(nibs))
	}
	out := make([]hdByte, 0, len(nibs)/2)
	for i := 0; i < len(nibs); i += 2 {
		if nibs[i].rs != nibs[i+1].rs {
			t.Fatalf("nibble pair %d mixes rs", i)
		}
		out = append(out, hdByte{b: nibs[i].nib<<4 | nibs[i+1].nib, rs: nibs[i].rs})
	}
	return out
}

func decodeData(t *testing.T, recs []txRecord) []byte {
	t.Helper()
	var out []byte
	for _, hb := range pairBytes(t, decodeNibblesWith(t, recs, false)) {
		if hb.rs {
			out = append(out, hb.b)
		}
	}
	return out
}

func newTestLCD(t *testing.T) (*hd44780, *fakeBus) {
	t.Helper()
	bus := &fakeBus{}
	dev, err := newHD44780(bus, 0x27, noSleep)
	if err != nil {
		t.Fatal(err)
	}
	return dev, bus
}

func TestHD44780InitSequence(t *testing.T) {
	_, bus := newTestLCD(t)
	nibs := decodeNibbles(t, bus.recs)

	wake := []byte{0x3, 0x3, 0x3, 0x2}
	for i, want := range wake {
		if nibs[i].rs || nibs[i].nib != want {
			t.Fatalf("wake nibble %d = %#x rs=%v, want %#x rs=false", i, nibs[i].nib, nibs[i].rs, want)
		}
	}
	cmds := pairBytes(t, nibs[len(wake):])
	want := []byte{lcdCmdFunctionSet, lcdCmdDisplayControl, lcdCmdClear, lcdCmdEntryMode}
	if len(cmds) != len(want) {
		t.Fatalf("init sends %d bytes, want %d", len(cmds), len(want))
	}
	for i, w := range want {
		if cmds[i].rs || cmds[i].b != w {
			t.Errorf("init byte %d = %#x rs=%v, want %#x rs=false", i, cmds[i].b, cmds[i].rs, w)
		}
	}
}

func TestPrintLineAddressesRows(t *testing.T) {
	dev, bus := newTestLCD(t)
	for row, addr := range map[int]byte{0: lcdCmdSetDDRAMRow0, 1: lcdCmdSetDDRAMRow1} {
		bus.reset()
		if err := dev.printLine(row, []byte("hi")); err != nil {
			t.Fatal(err)
		}
		got := pairBytes(t, decodeNibbles(t, bus.recs))
		if got[0].rs || got[0].b != addr {
			t.Errorf("row %d: first byte = %#x rs=%v, want DDRAM %#x", row, got[0].b, got[0].rs, addr)
		}
		if !got[1].rs || got[1].b != 'h' || !got[2].rs || got[2].b != 'i' {
			t.Errorf("row %d: data bytes = %+v, want 'h','i' with rs", row, got[1:])
		}
	}
}

func TestCreateCharWritesCGRAM(t *testing.T) {
	dev, bus := newTestLCD(t)
	pattern := customChars['♥']
	bus.reset()
	if err := dev.createChar(3, pattern); err != nil {
		t.Fatal(err)
	}
	got := pairBytes(t, decodeNibbles(t, bus.recs))
	if got[0].rs || got[0].b != lcdCmdSetCGRAM|3<<3 {
		t.Fatalf("CGRAM address byte = %#x rs=%v", got[0].b, got[0].rs)
	}
	if len(got) != 9 {
		t.Fatalf("createChar sends 9 bytes, got %d", len(got))
	}
	for i, row := range pattern {
		if !got[i+1].rs || got[i+1].b != row {
			t.Errorf("pattern row %d = %#x rs=%v, want %#x rs=true", i, got[i+1].b, got[i+1].rs, row)
		}
	}
}

func TestSetBacklightPinsBitAcrossWrites(t *testing.T) {
	dev, bus := newTestLCD(t)

	bus.reset()
	if err := dev.setBacklight(false); err != nil {
		t.Fatal(err)
	}
	if len(bus.recs) != 1 || bus.recs[0].w[0] != 0 {
		t.Fatalf("backlight off should write one dark byte, got %+v", bus.recs)
	}
	bus.reset()
	if err := dev.printLine(0, []byte("hi")); err != nil {
		t.Fatal(err)
	}
	for i, rec := range bus.recs {
		if rec.w[0]&pcfBacklight != 0 {
			t.Errorf("write %d relit the backlight while dark (%#x)", i, rec.w[0])
		}
	}

	bus.reset()
	if err := dev.setBacklight(true); err != nil {
		t.Fatal(err)
	}
	if len(bus.recs) != 1 || bus.recs[0].w[0] != pcfBacklight {
		t.Fatalf("backlight on should write one lit byte, got %+v", bus.recs)
	}
	bus.reset()
	if err := dev.printLine(0, []byte("hi")); err != nil {
		t.Fatal(err)
	}
	decodeNibbles(t, bus.recs)
}

func bareBytes(t *testing.T, recs []txRecord) []byte {
	t.Helper()
	var out []byte
	for i := 0; i < len(recs); i++ {
		b := recs[i].w[0]
		if b&pcfEnable == 0 {
			out = append(out, b)
			continue
		}
		if i+1 >= len(recs) || recs[i+1].w[0] != b&^pcfEnable {
			t.Fatalf("write %d: strobe %#x is not followed by its enable-low half", i, b)
		}
		i++
	}
	return out
}

func TestBacklightLevelWaveforms(t *testing.T) {
	dev, bus := newTestLCD(t)
	sc := newScreen(dev, nil, func(string, string) {})

	bus.reset()
	sc.setBacklightLevel(hardware.LampLevelFromABI(5))
	want := []txRecord{
		{0x27, []byte{pcfBacklight}},
		{0x27, []byte{0}},
		{0x27, []byte{pcfBacklight}},
		{0x27, []byte{0}},
	}
	if got := bus.forAddr(0x27); !reflect.DeepEqual(got, want) {
		t.Errorf("quantized level 5 frame = %v, want %v", got, want)
	}

	bus.reset()
	sc.setText("dim", "")
	for i, rec := range bus.forAddr(0x27) {
		if rec.w[0]&pcfBacklight != 0 {
			t.Fatalf("write %d lit the backlight while the level rounds dark (%#x)", i, rec.w[0])
		}
	}

	bus.reset()
	sc.setBacklightLevel(backlightFull)
	if got := bus.forAddr(0x27); len(got) != 1 || got[0].w[0] != pcfBacklight {
		t.Errorf("full level = %v, want one lit byte", got)
	}

	bus.reset()
	sc.setBacklightLevel(0)
	if got := bus.forAddr(0x27); len(got) != 1 || got[0].w[0] != 0 {
		t.Errorf("dark level = %v, want one dark byte", got)
	}
}

func TestBacklightFramesNeverSplitANibble(t *testing.T) {
	bus := &fakeBus{}
	dev, err := newHD44780(bus, 0x27, time.Sleep)
	if err != nil {
		t.Fatal(err)
	}
	sc := newScreen(dev, nil, func(string, string) {})
	sc.startWorker(true)
	defer sc.stopWorker()

	bus.reset()
	sc.setBacklightLevel(hardware.LampLevelFromABI(7))
	sc.setText("dimmed", "backlight")

	// A level quantizing to 7 keeps nibble writes dark, so a lit byte can only be
	// a BAM slot: three per frame. Wait for two frames rather than a fixed time.
	// Strobe pairing is checked after the worker stops, since a live snapshot can
	// end between a nibble's halves.
	litSlots := func() int {
		lit := 0
		for _, r := range bus.forAddr(0x27) {
			if r.w[0] == pcfBacklight {
				lit++
			}
		}
		return lit
	}
	deadline := time.Now().Add(3 * time.Second)
	for litSlots() < 6 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	sc.testWorker().flush()
	sc.stopWorker()
	if lit := litSlots(); lit < 6 {
		t.Errorf("only %d lit slot writes in 3s, want at least two frames' worth", lit)
	}
	bareBytes(t, bus.forAddr(0x27))
	if got := sc.window(0); got != padRight(centerShort("dimmed"), lcdCols) {
		t.Errorf("text should still land between frames, window=%q", got)
	}
}

func TestSetTextCentersShortLines(t *testing.T) {
	dev, bus := newTestLCD(t)
	sc := newScreen(dev, nil, func(string, string) {})
	bus.reset()

	sc.setText("hi", "hello world!!!!!") // 2 chars centered, 16 chars as-is
	if sc.lastLines[0] != "       hi" {
		t.Errorf("center pads left only: %q", sc.lastLines[0])
	}
	if sc.lastLines[1] != "hello world!!!!!" {
		t.Errorf("full-width line should not be padded: %q", sc.lastLines[1])
	}
	if got := string(decodeData(t, bus.forAddr(0x27))); !strings.Contains(got, "       hi       ") {
		t.Errorf("printed line 1 = %q, want centered and padded to 16", got)
	}
}

func TestSetTextSkipsUnchangedLines(t *testing.T) {
	dev, bus := newTestLCD(t)
	sc := newScreen(dev, nil, func(string, string) {})
	sc.setText("same", "same")
	bus.reset()
	sc.setText("same", "same")
	if len(bus.recs) != 0 {
		t.Errorf("unchanged text should not rewrite, got %d writes", len(bus.recs))
	}
}

func TestScrollStepMath(t *testing.T) {
	long := strings.Repeat("x", 20) // longest+3 = 23

	idx, act := scrollStep(long, "", -scrollLeadIn)
	if idx != -scrollLeadIn+1 || act != scrollHold {
		t.Errorf("lead-in: got idx=%d act=%d", idx, act)
	}
	idx, act = scrollStep(long, "", 0)
	if idx != 1 || act != scrollDraw {
		t.Errorf("first visible shift: got idx=%d act=%d", idx, act)
	}
	idx, act = scrollStep(long, "", 22)
	if idx != -scrollLeadIn || act != scrollReset {
		t.Errorf("wrap at longest+3: got idx=%d act=%d", idx, act)
	}
	idx, act = scrollStep("short", "also short", 5)
	if idx != 5 || act != scrollStop {
		t.Errorf("no overflow should stop: got idx=%d act=%d", idx, act)
	}
}

func TestMarqueeScrollAndReset(t *testing.T) {
	dev, bus := newTestLCD(t)
	sc := newScreen(dev, nil, func(string, string) {})
	long := "ABCDEFGHIJKLMNOPQRST" // 20 chars, no centering
	sc.setText(long, "")
	if sc.idx != -scrollLeadIn {
		t.Fatalf("new text should reset index to -scrollLeadIn, got %d", sc.idx)
	}

	// 10 hold steps bring the index to 0 without redrawing.
	bus.reset()
	for i := 0; i < 10; i++ {
		sc.step()
	}
	if sc.idx != 0 || len(bus.recs) != 0 {
		t.Fatalf("lead-in should not draw: idx=%d writes=%d", sc.idx, len(bus.recs))
	}

	bus.reset()
	sc.step()
	if got := string(decodeData(t, bus.forAddr(0x27))); !strings.Contains(got, "BCDEFGHIJKLMNOPQ") {
		t.Errorf("scroll window = %q, want BCDEFGHIJKLMNOPQ", got)
	}

	for sc.idx != -scrollLeadIn {
		bus.reset()
		sc.step()
	}
	if got := string(decodeData(t, bus.forAddr(0x27))); !strings.Contains(got, "ABCDEFGHIJKLMNOP") {
		t.Errorf("wrap redraw = %q, want line start", got)
	}
}

func TestSetTextResetsScrollIndex(t *testing.T) {
	dev, _ := newTestLCD(t)
	sc := newScreen(dev, nil, func(string, string) {})
	sc.setText(strings.Repeat("y", 20), "")
	for i := 0; i < 15; i++ {
		sc.step()
	}
	if sc.idx <= 0 {
		t.Fatalf("expected mid-scroll, idx=%d", sc.idx)
	}
	sc.setText(strings.Repeat("z", 20), "")
	if sc.idx != -scrollLeadIn {
		t.Errorf("changed text should restart the marquee, idx=%d", sc.idx)
	}
}

func TestSubstituteFallsBackForUnknownRunes(t *testing.T) {
	dev, bus := newTestLCD(t)
	sc := newScreen(dev, nil, func(string, string) {})
	bus.reset()
	sc.setText("héllo", "")
	got := decodeData(t, bus.forAddr(0x27))
	if !bytes.Contains(got, []byte("h?llo")) {
		t.Errorf("non-ASCII without a glyph should print '?': %q", got)
	}
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition never became true within %v", timeout)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestSetTextAsyncReturnsFast(t *testing.T) {
	bus := &fakeBus{delay: 200 * time.Microsecond}
	dev, err := newHD44780(bus, 0x27, noSleep)
	if err != nil {
		t.Fatal(err)
	}
	sc := newScreen(dev, nil, func(string, string) {})
	sc.startWorker(true)
	defer sc.stopWorker()

	start := time.Now()
	sc.setText("hello", "world")
	if d := time.Since(start); d > time.Millisecond {
		t.Fatalf("setText took %v with a worker running, want <1ms", d)
	}

	waitUntil(t, 2*time.Second, func() bool {
		return sc.window(0) == padRight(centerShort("hello"), lcdCols)
	})
}

func TestSetTextAsyncCoalescesLatestWins(t *testing.T) {
	bus := &fakeBus{delay: time.Millisecond}
	dev, err := newHD44780(bus, 0x27, noSleep)
	if err != nil {
		t.Fatal(err)
	}
	sc := newScreen(dev, nil, func(string, string) {})
	sc.startWorker(true)
	defer sc.stopWorker()

	sc.setText("one", "")
	time.Sleep(5 * time.Millisecond)
	sc.setText("two", "")
	start := time.Now()
	sc.setText("three", "")
	if d := time.Since(start); d > time.Millisecond {
		t.Fatalf("setText took %v while the worker was busy, want <1ms", d)
	}

	// flush, not a window poll, proves the bytes landed: lastLines updates before
	// the device write.
	sc.testWorker().flush()
	if got := sc.window(0); got != padRight(centerShort("three"), lcdCols) {
		t.Fatalf("worker should have settled on the latest text, window=%q", got)
	}

	got := string(decodeData(t, bus.forAddr(0x27)))
	if strings.Contains(got, "two") {
		t.Errorf("coalesced intermediate text should never reach the device, got %q", got)
	}
	if !strings.Contains(got, "one") {
		t.Errorf("the in-flight redraw when the newer calls arrived should still complete, got %q", got)
	}
}

func TestScreenWorkerMarqueeKeepsAnimatingDuringWork(t *testing.T) {
	bus := &fakeBus{}
	dev, err := newHD44780(bus, 0x27, noSleep)
	if err != nil {
		t.Fatal(err)
	}
	sc := newScreen(dev, nil, func(string, string) {})
	sc.startWorker(true)
	defer sc.stopWorker()

	sc.setText(strings.Repeat("m", 20), "") // overflow -> marquee starts
	waitUntil(t, time.Second, func() bool {
		return sc.window(0) != padRight("", lcdCols) // initial draw landed
	})

	if !sc.testWorker().isAnimating() {
		t.Error("overflowing text should mark the worker as animating")
	}

	sc.setText("short", "") // no longer overflows
	waitUntil(t, time.Second, func() bool {
		return !sc.testWorker().isAnimating()
	})
}

type panelModel struct {
	ddram [0x80]byte
	cgram [maxCGRAMSlots][8]byte
	addr  int
	cg    bool
}

func newPanelModel() *panelModel {
	m := &panelModel{}
	for i := range m.ddram {
		m.ddram[i] = ' '
	}
	return m
}

func (m *panelModel) feed(t *testing.T, recs []txRecord) {
	t.Helper()
	for _, hb := range pairBytes(t, decodeNibbles(t, recs)) {
		switch {
		case hb.rs && m.cg:
			m.cgram[m.addr/8][m.addr%8] = hb.b
			m.addr = (m.addr + 1) % (maxCGRAMSlots * 8)
		case hb.rs:
			m.ddram[m.addr&0x7f] = hb.b
			m.addr++
		case hb.b&0x80 != 0:
			m.addr, m.cg = int(hb.b&0x7f), false
		case hb.b&0x40 != 0:
			m.addr, m.cg = int(hb.b&0x3f), true
		case hb.b == lcdCmdClear:
			for i := range m.ddram {
				m.ddram[i] = ' '
			}
			m.addr, m.cg = 0, false
		}
	}
}

func (m *panelModel) shows(row, col int, want rune) bool {
	b := m.ddram[row*0x40+col]
	if pattern, custom := customChars[want]; custom {
		return b < maxCGRAMSlots && m.cgram[b] == pattern
	}
	return b == byte(want)
}

func distinctGlyphRunes(t *testing.T, n int) []rune {
	t.Helper()
	var keys []rune
	for r := range customChars {
		keys = append(keys, r)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	seen := map[[8]byte]bool{}
	var out []rune
	for _, r := range keys {
		if p := customChars[r]; !seen[p] {
			seen[p] = true
			out = append(out, r)
		}
		if len(out) == n {
			return out
		}
	}
	t.Fatalf("only %d distinct glyph bitmaps, want %d", len(out), n)
	return nil
}

// The two rows share one CGRAM: row 1's uploads must not overwrite a slot row 0
// still shows, and row 1 falls back to '?' once the budget is spent.
func TestTwoRowsNeverShowAnEvictedGlyph(t *testing.T) {
	runes := distinctGlyphRunes(t, 16)
	for _, tc := range []struct {
		name         string
		line1, line2 []rune
	}{
		{"eight and eight", runes[:8], runes[8:16]},
		{"five and five", runes[:5], runes[5:10]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dev, bus := newTestLCD(t)
			sc := newScreen(dev, nil, func(string, string) {})
			bus.reset()
			sc.setText(string(tc.line1), string(tc.line2))

			m := newPanelModel()
			m.feed(t, bus.forAddr(0x27))
			for row, line := range [2]string{sc.lastLines[0], sc.lastLines[1]} {
				for col, want := range []rune(line) {
					if m.shows(row, col, want) {
						continue
					}
					if row == 1 && m.ddram[0x40+col] == '?' {
						continue
					}
					t.Errorf("row %d col %d shows byte %#x, want %q", row, col, m.ddram[row*0x40+col], want)
				}
			}
		})
	}
}

func TestShortLine2ChangeKeepsLine1Scrolling(t *testing.T) {
	dev, bus := newTestLCD(t)
	sc := newScreen(dev, nil, func(string, string) {})
	sc.startWorker(true)
	defer sc.stopWorker()

	long := "ABCDEFGHIJKLMNOPQRST"
	sc.setText(long, "")
	sc.testWorker().flush()
	if !sc.testWorker().isAnimating() {
		t.Fatal("an overflowing line 1 should start the marquee")
	}

	bus.reset()
	sc.setText(long, "hi")
	sc.testWorker().flush()
	if got := string(decodeData(t, bus.forAddr(0x27))); got != padRight(centerShort("hi"), lcdCols) {
		t.Errorf("a line-2 change sent %q, want only row 1's 16 columns", got)
	}
	if !sc.testWorker().isAnimating() {
		t.Error("changing only line 2 stopped line 1's marquee")
	}

	sc.setText(long, "hi")
	sc.testWorker().flush()
	if !sc.testWorker().isAnimating() {
		t.Error("re-sending the same overflowing text stopped the marquee")
	}
}
