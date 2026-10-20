// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// fakeBus records every Tx; it is safe for concurrent use and delay simulates a
// slow bus.
type txRecord struct {
	addr uint16
	w    []byte
}

type fakeBus struct {
	mu    sync.Mutex
	recs  []txRecord
	delay time.Duration
	err   error
	devs  map[uint16]*fakeDevice // beyond the cabinet's fixed set
	gone  map[uint16]bool        // fixed devices a test has pulled off the bus
}

type fakeDevice struct{ regs [256]byte }

var fakeBusFixed = map[uint16]bool{muxAddr: true, lcdAddrs[0]: true, lcdAddrs[1]: true}

var (
	errFakeBus   = errors.New("fake bus: nobody home")
	errNoDevice  = errors.New("fake bus: no device at address")
	errShortRead = errors.New("fake bus: read past register file")
)

func (f *fakeBus) plug(addr uint16, reset map[byte]byte) *fakeDevice {
	d := &fakeDevice{}
	for reg, v := range reset {
		d.regs[reg] = v
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.devs == nil {
		f.devs = map[uint16]*fakeDevice{}
	}
	f.devs[addr] = d
	return d
}

func (f *fakeBus) unplug(addr uint16) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gone == nil {
		f.gone = map[uint16]bool{}
	}
	f.gone[addr] = true
}

func (f *fakeBus) Tx(addr uint16, w, r []byte) error {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	cp := append([]byte(nil), w...)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recs = append(f.recs, txRecord{addr: addr, w: cp})
	dev := f.devs[addr]
	if dev == nil && (f.gone[addr] || !fakeBusFixed[addr]) {
		return errNoDevice
	}
	if dev != nil && len(cp) > 0 {
		for i, b := range cp[1:] {
			dev.regs[int(cp[0])+i] = b
		}
		if len(r) > 0 {
			if int(cp[0])+len(r) > len(dev.regs) {
				return errShortRead
			}
			copy(r, dev.regs[cp[0]:])
		}
	}
	return f.err
}

func (f *fakeBus) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeBus) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recs = nil
}

func (f *fakeBus) forAddr(addr uint16) []txRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []txRecord
	for _, r := range f.recs {
		if r.addr == addr {
			out = append(out, r)
		}
	}
	return out
}

func noSleep(time.Duration) {}

func testSuite(t *testing.T) (*Suite, *fakeBus) {
	t.Helper()
	bus := &fakeBus{}
	s := newSuite(bus, nil, noSleep, DefaultConfig())
	if s.lamps == nil || s.screens[0] == nil || s.screens[1] == nil {
		t.Fatal("suite should wire the lamp writer and both screens over the fake bus")
	}
	return s, bus
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestIsPiMatchesCpuinfoModel(t *testing.T) {
	orig := cpuInfoPath
	defer func() { cpuInfoPath = orig }()

	cpuInfoPath = writeFile(t, "cpuinfo", "Hardware\t: BCM2835\nModel\t\t: Raspberry Pi 4 Model B Rev 1.4\n")
	if !IsPi() {
		t.Error("Raspberry Pi model line should be detected")
	}
	cpuInfoPath = writeFile(t, "cpuinfo2", "model name\t: Apple M2\n")
	if IsPi() {
		t.Error("non-Pi cpuinfo should not be detected")
	}
	cpuInfoPath = filepath.Join(t.TempDir(), "missing")
	if IsPi() {
		t.Error("missing cpuinfo should not be detected")
	}
}

func TestCPUTempFConvertsMillidegrees(t *testing.T) {
	origInfo, origZone := cpuInfoPath, thermalZonePath
	defer func() { cpuInfoPath, thermalZonePath = origInfo, origZone }()

	cpuInfoPath = writeFile(t, "cpuinfo", "Model : Raspberry Pi 3\n")
	thermalZonePath = writeFile(t, "temp", "50000\n") // 50°C
	if got := CPUTempF(); got != 122 {
		t.Errorf("50000 mC = 122°F, got %v", got)
	}
	thermalZonePath = filepath.Join(t.TempDir(), "missing")
	if got := CPUTempF(); !math.IsInf(got, -1) {
		t.Errorf("missing thermal zone should read -Inf, got %v", got)
	}
	cpuInfoPath = writeFile(t, "cpuinfo2", "not a pi\n")
	if got := CPUTempF(); !math.IsInf(got, -1) {
		t.Errorf("off-Pi temp should read -Inf, got %v", got)
	}
}

func TestSetLightsLatchesPlayerPins(t *testing.T) {
	s, bus := testSuite(t)
	bus.reset()

	s.SetLights(-1, true)
	recs := bus.forAddr(muxAddr)
	if len(recs) != 2 {
		t.Fatalf("both players = one latch write per seat, got %d", len(recs))
	}
	if got := recs[1].w; !reflect.DeepEqual(got, []byte{regOLATA, 0x3f, 0x3f}) {
		t.Errorf("final latches = %v, want [%#x 0x3f 0x3f]", got, regOLATA)
	}

	bus.reset()
	s.SetLights(1, false)
	recs = bus.forAddr(muxAddr)
	if len(recs) != 1 {
		t.Fatalf("one seat = one latch write, got %d", len(recs))
	}
	if got := recs[0].w; !reflect.DeepEqual(got, []byte{regOLATA, 0x3f, 0x00}) {
		t.Errorf("player 2 off = %v, want port B dark and port A untouched", got)
	}
}

func TestSetButtonLightsDeltaKeepsLevels(t *testing.T) {
	s, bus := testSuite(t)
	s.SetLampLevels(0, hardware.LampLevels{hardware.LampLevelFromABI(3), 0, hardware.LampLevelMax, 0, 0, 0})
	bus.reset()

	s.SetButtonLights(0, hardware.ButtonLights{"joyb": true}, false)
	if got, want := s.levels[0], (hardware.LampLevels{hardware.LampLevelFromABI(3), hardware.LampLevelMax, hardware.LampLevelMax}); got != want {
		t.Errorf("delta levels = %v, want %v", got, want)
	}
	want := []txRecord{
		{muxAddr, []byte{regOLATA, 0x07, 0x00}},
		{muxAddr, []byte{regOLATA, 0x07, 0x00}},
		{muxAddr, []byte{regOLATA, 0x06, 0x00}},
		{muxAddr, []byte{regOLATA, 0x06, 0x00}},
	}
	if got := bus.forAddr(muxAddr); !reflect.DeepEqual(got, want) {
		t.Fatalf("frame = %v, want %v", got, want)
	}

	bus.reset()
	s.SetButtonLights(0, hardware.ButtonLights{"joyb": true}, true)
	if got, want := s.levels[0], (hardware.LampLevels{0, hardware.LampLevelMax}); got != want {
		t.Errorf("only=true levels = %v, want %v", got, want)
	}
	if got := bus.forAddr(muxAddr); len(got) != 1 || !reflect.DeepEqual(got[0].w, []byte{regOLATA, 0x02, 0x00}) {
		t.Errorf("binary state = %v, want a single latch write of 0x02", got)
	}
}

func TestSetTextTargetsOneScreen(t *testing.T) {
	s, bus := testSuite(t)
	bus.reset()

	s.SetText(0, "hi", "")
	if got := bus.forAddr(lcdAddrs[1]); len(got) != 0 {
		t.Errorf("player 1 text should not write to player 2 LCD, got %d writes", len(got))
	}
	if got := bus.forAddr(lcdAddrs[0]); len(got) == 0 {
		t.Error("player 1 LCD should receive writes")
	}
}

func TestSuiteBacklightLevels(t *testing.T) {
	s, bus := testSuite(t)

	for _, tc := range []struct {
		on   bool
		want byte
	}{{true, pcfBacklight}, {false, 0}} { // the suite starts dark
		bus.reset()
		s.SetBacklight(tc.on)
		for _, addr := range lcdAddrs {
			got := bus.forAddr(addr)
			if len(got) != 1 || got[0].w[0] != tc.want {
				t.Errorf("SetBacklight(%v) on 0x%x = %v, want one %#x byte", tc.on, addr, got, tc.want)
			}
		}
	}

	bus.reset()
	s.SetBacklightLevel(hardware.LampLevelFromABI(10))
	want := []byte{0, pcfBacklight, 0, pcfBacklight}
	for _, addr := range lcdAddrs {
		got := bus.forAddr(addr)
		if len(got) != len(want) {
			t.Fatalf("level 10 on 0x%x = %v, want one four-slot frame", addr, got)
		}
		for i, w := range want {
			if got[i].w[0] != w {
				t.Errorf("level 10 on 0x%x slot %d = %#x, want %#x", addr, i, got[i].w[0], w)
			}
		}
	}
}

func TestStatusLinesShape(t *testing.T) {
	s, _ := testSuite(t)
	s.SetText(-1, "hello", "world")
	s.SetLights(0, true)

	lines := s.StatusLines()
	if len(lines) != 2 {
		t.Fatalf("want 2 status lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "P1 ◉") || !strings.Contains(lines[0], "P2 ○") {
		t.Errorf("light indicators missing: %q", lines[0])
	}
	if !strings.Contains(lines[0], "|     hello      |") {
		t.Errorf("centered text missing: %q", lines[0])
	}
	if !strings.Contains(lines[1], "Fan ✻ ~°F") || !strings.Contains(lines[1], "Motion ⏾") {
		t.Errorf("idle fan/motion indicators missing: %q", lines[1])
	}
}

func TestCloseLeavesPowerOffMessage(t *testing.T) {
	s, bus := testSuite(t)
	s.SetText(-1, "playing", "games")
	bus.reset()

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, addr := range lcdAddrs {
		text := string(decodeData(t, bus.forAddr(addr)))
		if !strings.Contains(text, "Power OFF") || !strings.Contains(text, "under table") {
			t.Errorf("lcd 0x%x teardown text = %q", addr, text)
		}
	}
	if err := s.Close(); err != nil {
		t.Errorf("second Close should be a no-op, got %v", err)
	}
}

// powerOffDimmed is the level the power-off sequence leaves the backlight at,
// too low for a backpack bit to hold.
const powerOffDimmed = hardware.LampLevelMax * 30 / 100

func TestCloseKeepsTheSequencesGoodbye(t *testing.T) {
	s, bus := testSuite(t)
	s.SetText(-1, hardware.PowerOffLine1, hardware.PowerOffLine2)
	s.SetBacklightLevel(powerOffDimmed)
	bus.reset()

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, addr := range lcdAddrs {
		if text := decodeData(t, bus.forAddr(addr)); len(text) != 0 {
			t.Errorf("lcd 0x%x redrew %q on close, want the sequence's own text left alone", addr, text)
		}
		recs := bus.forAddr(addr)
		if len(recs) == 0 {
			t.Errorf("lcd 0x%x wrote nothing on close, want the backpack bit switched on", addr)
		}
		for _, r := range recs {
			if r.w[0]&pcfBacklight == 0 {
				t.Errorf("lcd 0x%x wrote %#x, want the backpack bit left high", addr, r.w[0])
			}
		}
	}
}

func TestClosePCABacklightKeepsItsLevel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PCA9685.Backlights = [2]int{6, 14}
	s, bus, _ := pcaSuiteWith(t, cfg)
	s.SetText(-1, hardware.PowerOffLine1, hardware.PowerOffLine2)
	s.SetBacklightLevel(powerOffDimmed)
	bus.reset()

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if got := chanWrites(bus, cfg.PCA9685.Addr); len(got) != 0 {
		t.Errorf("close rewrote %d PCA channels, want the backlight PWM held", len(got))
	}
	for _, addr := range lcdAddrs {
		if got := bus.forAddr(addr); len(got) != 0 {
			t.Errorf("lcd 0x%x wrote %d records on close, want it left exactly as it was", addr, len(got))
		}
	}
}

func testAsyncSuite(t *testing.T, delay time.Duration) (*Suite, *fakeBus) {
	t.Helper()
	bus := &fakeBus{delay: delay}
	s := newSuite(bus, nil, noSleep, DefaultConfig())
	s.startWorkers()
	t.Cleanup(func() {
		if s.mcp != nil {
			s.mcp.stop()
		}
		for _, sc := range s.screens {
			sc.stopWorker()
		}
	})
	return s, bus
}

func TestSetLightsReturnsFastEvenWithABusyScreen(t *testing.T) {
	s, _ := testAsyncSuite(t, 2*time.Millisecond)

	s.SetText(-1, strings.Repeat("x", 20), strings.Repeat("y", 20))

	for i := 0; i < 20; i++ {
		start := time.Now()
		s.SetLights(-1, i%2 == 0)
		if d := time.Since(start); d > 5*time.Millisecond {
			t.Fatalf("SetLights took %v, want it to return without waiting on the screen worker (a full redraw at this bus speed takes ~270ms)", d)
		}
	}
}

func TestCloseFlushesGoodbyeThroughWorker(t *testing.T) {
	s, bus := testAsyncSuite(t, time.Millisecond)

	s.SetText(-1, "playing", "games")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, addr := range lcdAddrs {
		text := string(decodeData(t, bus.forAddr(addr)))
		if !strings.Contains(text, "Power OFF") || !strings.Contains(text, "under table") {
			t.Errorf("lcd 0x%x teardown text = %q", addr, text)
		}
	}
}

func TestConcurrentSetTextAndButtonLightsRace(t *testing.T) {
	s, _ := testAsyncSuite(t, 0)

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				s.SetText(-1, fmt.Sprintf("g%d-%d", n, i), "line2")
				s.SetLights(n%2, i%2 == 0)
				s.SetButtonLights(-1, hardware.ButtonLights{"joya": i%2 == 0, "joyb": i%3 == 0}, i%5 == 0)
				s.SetBacklight(i%2 == 0)
				_ = s.StatusLines()
			}
		}(g)
	}
	wg.Wait()
}
