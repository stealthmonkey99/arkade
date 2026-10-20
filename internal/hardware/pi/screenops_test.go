// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

func (s *screen) testWorker() *screenWorker {
	w, ok := s.currentOps().(*screenWorker)
	if !ok {
		panic("screen has no worker")
	}
	return w
}

func TestNewScreenIsSynchronous(t *testing.T) {
	dev, bus := newTestLCD(t)
	sc := newScreen(dev, nil, func(string, string) {})
	if _, ok := sc.currentOps().(syncOps); !ok {
		t.Fatalf("a new screen's ops = %T, want syncOps", sc.currentOps())
	}
	bus.reset()
	sc.setText("hello", "")
	if got := sc.window(0); strings.TrimSpace(got) != "hello" {
		t.Errorf("row 0 = %q right after setText", got)
	}
	if len(bus.recs) == 0 {
		t.Error("synchronous setText wrote nothing")
	}
}

func TestStopWorkerReturnsToSynchronous(t *testing.T) {
	dev, _ := newTestLCD(t)
	sc := newScreen(dev, nil, func(string, string) {})
	sc.startWorker(true)
	sc.startWorker(true)
	sc.testWorker()
	sc.stopWorker()
	if _, ok := sc.currentOps().(syncOps); !ok {
		t.Fatalf("after stopWorker ops = %T, want syncOps", sc.currentOps())
	}
	sc.setText("after", "")
	if got := sc.window(0); strings.TrimSpace(got) != "after" {
		t.Errorf("row 0 = %q, want the text applied inline", got)
	}
}

func TestSyncBacklightNeverLandsInsideANibble(t *testing.T) {
	dev, bus := newTestLCD(t)
	sc := newScreen(dev, nil, func(string, string) {})
	bus.reset()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 50 {
			sc.setText(strings.Repeat("x", i%16), "y")
		}
	}()
	go func() {
		defer wg.Done()
		for i := range 200 {
			sc.setBacklightLevel(uint8(i%2) * backlightFull)
		}
	}()
	wg.Wait()
	bareBytes(t, bus.forAddr(0x27))
}

// The same requests produce the same bus traffic whichever ops carry them.
func TestWorkerAndSyncOpsWriteTheSameBytes(t *testing.T) {
	steps := []func(*screen){
		func(sc *screen) { sc.setText("Å score ♥", "line two") },
		func(sc *screen) { sc.setText("Å score ♥", "line 2 ♪") },
		func(sc *screen) { sc.setBacklightLevel(0) },
		func(sc *screen) { sc.setBacklightLevel(backlightFull) },
	}
	run := func(worker bool) []txRecord {
		dev, bus := newTestLCD(t)
		sc := newScreen(dev, nil, func(string, string) {})
		if worker {
			sc.startWorker(false)
			defer sc.stopWorker()
		}
		bus.reset()
		for _, step := range steps {
			step(sc)
			if worker {
				sc.testWorker().flush()
			}
		}
		bus.mu.Lock()
		defer bus.mu.Unlock()
		return append([]txRecord(nil), bus.recs...)
	}
	direct, queued := run(false), run(true)
	if len(direct) == 0 {
		t.Fatal("the script wrote nothing")
	}
	if !reflect.DeepEqual(direct, queued) {
		t.Errorf("worker traffic differs from synchronous: %d vs %d writes", len(queued), len(direct))
	}
}

func TestGlyphRedefinitionRedrawsBothScreensOfASuite(t *testing.T) {
	s, bus := testSuite(t)
	s.SetText(0, "♥", "")
	s.SetText(1, "♥", "")

	redesign := hardware.GlyphRows{0x1f, 0x11, 0x11, 0x11, 0x0a, 0x0a, 0x04, 0x00}
	s.SetCartGlyphs(map[rune]hardware.GlyphRows{'♥': redesign})
	bus.reset()
	s.SetText(0, "♥", "")
	s.SetText(1, "♥", "")

	for _, addr := range lcdAddrs {
		got := pairBytes(t, decodeNibblesWith(t, bus.forAddr(addr), false))
		if len(got) < 9 || got[0].b != lcdCmdSetCGRAM {
			t.Fatalf("lcd %#x: unchanged text after a redefinition must re-upload the glyph; writes = %v", addr, got)
		}
		for i, row := range redesign {
			if got[i+1].b != row {
				t.Errorf("lcd %#x row %d = %#x, want %#x", addr, i, got[i+1].b, row)
			}
		}
	}
}

func TestGlyphTablesBelongToTheirSuite(t *testing.T) {
	a, _ := testSuite(t)
	b, busB := testSuite(t)

	const coin = '¤'
	a.SetCartGlyphs(map[rune]hardware.GlyphRows{coin: {0x0e, 0x11, 0x15, 0x15, 0x15, 0x11, 0x0e, 0x00}})
	busB.reset()
	b.SetText(0, string(coin), "")
	if data := decodeData(t, busB.forAddr(lcdAddrs[0])); !containsByte(data, '?') {
		t.Errorf("another suite's cartridge glyph leaked in: %q", data)
	}
}

func TestUnchangedTextIsNotRedrawnWithoutARedefinition(t *testing.T) {
	s, bus := testSuite(t)
	s.SetText(0, "♥", "")
	s.SetCartGlyphs(map[rune]hardware.GlyphRows{'♥': {}})
	s.SetText(0, "♥", "")
	bus.reset()
	s.SetText(0, "♥", "")
	if n := len(bus.forAddr(lcdAddrs[0])); n != 0 {
		t.Errorf("a repeat after the redraw wrote %d times, want 0", n)
	}
}

func TestMarqueeTickPicksUpARedefinition(t *testing.T) {
	dev, bus := newTestLCD(t)
	glyphs := &glyphTable{}
	sc := newScreen(dev, glyphs, func(string, string) {})
	sc.setText("♥ "+strings.Repeat("x", lcdCols+4), "")
	for i := 0; i < scrollLeadIn; i++ {
		sc.step()
	}

	redesign := hardware.GlyphRows{0x1f, 0, 0x1f, 0, 0x1f, 0, 0x1f, 0}
	glyphs.setPreview(map[rune]hardware.GlyphRows{'♥': redesign})
	bus.reset()
	sc.step() // scrolls ♥ out of view; the next wrap brings it back
	for i := 0; i < lcdCols+8 && !hasCGRAMUpload(t, bus, redesign); i++ {
		sc.step()
	}
	if !hasCGRAMUpload(t, bus, redesign) {
		t.Error("the marquee never re-uploaded the redefined glyph")
	}
}

func hasCGRAMUpload(t *testing.T, bus *fakeBus, rows hardware.GlyphRows) bool {
	t.Helper()
	got := pairBytes(t, decodeNibbles(t, bus.forAddr(0x27)))
	for i := 0; i+8 < len(got); i++ {
		if got[i].rs || got[i].b&0xf8 != lcdCmdSetCGRAM {
			continue
		}
		match := true
		for j, r := range rows {
			if got[i+1+j].b != r {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
