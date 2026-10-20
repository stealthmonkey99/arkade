// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"math"
	"reflect"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// pcaReset is the chip's MODE1 power-on value (ALLCALL + SLEEP).
var pcaReset = map[byte]byte{pcaMode1: 0x11}

func pcaSuite(t *testing.T) (*Suite, *fakeBus, Config) {
	t.Helper()
	return pcaSuiteWith(t, DefaultConfig())
}

func pcaSuiteWith(t *testing.T, cfg Config) (*Suite, *fakeBus, Config) {
	t.Helper()
	bus := &fakeBus{}
	bus.plug(cfg.PCA9685.Addr, pcaReset)
	s := newSuite(bus, nil, noSleep, cfg)
	if s.pca == nil {
		t.Fatal("a PCA9685 on the bus should take over the lamps")
	}
	if got := s.LampDriver(); got != "pca9685" {
		t.Fatalf("LampDriver = %q, want pca9685", got)
	}
	return s, bus, cfg
}

func chanWrites(bus *fakeBus, addr uint16) []txRecord {
	var out []txRecord
	for _, r := range bus.forAddr(addr) {
		if len(r.w) == 5 && r.w[0] >= pcaLED0OnL && r.w[0] < pcaAllLEDOnL {
			out = append(out, r)
		}
	}
	return out
}

func TestPCA9685AbsentFallsBackToExpander(t *testing.T) {
	bus := &fakeBus{}
	s := newSuite(bus, nil, noSleep, DefaultConfig())
	if s.pca != nil {
		t.Fatal("nothing answers 0x40 on a bare fake bus")
	}
	if got := s.LampDriver(); got != "mcp23017" {
		t.Fatalf("LampDriver = %q, want mcp23017", got)
	}
	bus.reset()

	s.SetLights(0, true)
	if got := bus.forAddr(muxAddr); len(got) != 1 || !reflect.DeepEqual(got[0].w, []byte{regOLATA, 0x3f, 0x00}) {
		t.Fatalf("lamp write = %v, want one expander latch", got)
	}
}

// TestPCA9685InitSequence pins that PRE_SCALE is written while SLEEP is set; it
// is writable only then.
func TestPCA9685InitSequence(t *testing.T) {
	bus := &fakeBus{}
	bus.plug(pcaDefaultAddr, pcaReset)
	if _, err := newPCA9685(bus, pcaDefaultAddr, 1000, false, noSleep); err != nil {
		t.Fatal(err)
	}
	want := [][]byte{
		{pcaMode1},                    // the probe read
		{pcaMode1, 0x11},              // SLEEP held, RESTART not set
		{pcaPreScale, 0x05},           // 25 MHz / (4096 × 1000) − 1, rounded
		{pcaMode1, 0x20},              // auto-increment on, awake, no All-Call
		{pcaMode2, pcaTotemPole},      // totem-pole outputs, not inverted
		{pcaAllLEDOnL, 0, 0, 0, 0x10}, // every channel full-off
	}
	var got [][]byte
	for _, r := range bus.forAddr(pcaDefaultAddr) {
		got = append(got, r.w)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("init sequence =\n%v\nwant\n%v", got, want)
	}
}

func TestPCA9685InvertAndFrequency(t *testing.T) {
	bus := &fakeBus{}
	bus.plug(pcaDefaultAddr, pcaReset)
	if _, err := newPCA9685(bus, pcaDefaultAddr, 200, true, noSleep); err != nil {
		t.Fatal(err)
	}
	for _, r := range bus.forAddr(pcaDefaultAddr) {
		if len(r.w) == 2 && r.w[0] == pcaMode2 && r.w[1] != pcaTotemPole|pcaInvert {
			t.Errorf("MODE2 = %#x, want INVRT set", r.w[1])
		}
		if len(r.w) == 2 && r.w[0] == pcaPreScale && r.w[1] != 30 {
			t.Errorf("prescale for 200 Hz = %d, want 30", r.w[1])
		}
	}
	for _, freq := range []int{0, 1, 100000} {
		if p := pcaPrescale(freq); p < pcaMinPrescale {
			t.Errorf("prescale for %d Hz = %d, below the datasheet floor", freq, p)
		}
	}
}

func TestPCASetLightsWritesSixChannels(t *testing.T) {
	s, bus, cfg := pcaSuite(t)
	bus.reset()

	s.SetLights(0, true)
	got := chanWrites(bus, cfg.PCA9685.Addr)
	if len(got) != lightsPerPlayer {
		t.Fatalf("one seat = %d channel writes, got %d", lightsPerPlayer, len(got))
	}
	for i, ch := range cfg.PCA9685.Lamps[0] {
		want := []byte{byte(pcaLED0OnL + 4*ch), 0, pcaFullBit, 0, 0}
		if !reflect.DeepEqual(got[i].w, want) {
			t.Errorf("channel %d full-on = %v, want %v", ch, got[i].w, want)
		}
	}
	if n := len(bus.forAddr(muxAddr)); n != 0 {
		t.Errorf("expander should stay latched low while the PCA drives the lamps, got %d writes", n)
	}

	bus.reset()
	s.SetLights(-1, false)
	got = chanWrites(bus, cfg.PCA9685.Addr)
	if len(got) != lightsPerPlayer {
		t.Fatalf("both seats off = %d writes, got %d", lightsPerPlayer, len(got))
	}
	if want := []byte{byte(pcaLED0OnL + 4*cfg.PCA9685.Lamps[0][0]), 0, 0, 0, pcaFullBit}; !reflect.DeepEqual(got[0].w, want) {
		t.Errorf("channel 0 off = %v, want the full-off bit %v", got[0].w, want)
	}
}

func TestPCAGammaDuty(t *testing.T) {
	s, bus, cfg := pcaSuite(t)
	level := uint8((int(hardware.LampLevelMax) + 1) / 2)
	bus.reset()

	s.SetLampLevels(0, hardware.LampLevels{level})
	got := chanWrites(bus, cfg.PCA9685.Addr)
	if len(got) != 1 {
		t.Fatalf("one changed lamp = one write, got %d", len(got))
	}
	duty := uint16(got[0].w[3]) | uint16(got[0].w[4])<<8
	want := uint16(math.Round(float64(pcaSteps-1) * math.Pow(float64(level)/float64(hardware.LampLevelMax), pcaGammaExp)))
	if duty != want {
		t.Errorf("level %d duty = %d, want %d", level, duty, want)
	}
	if duty >= (pcaSteps-1)/3 {
		t.Errorf("level %d duty = %d; gamma %.1f should put half brightness well below a third", level, duty, pcaGammaExp)
	}
	if got[0].w[1] != 0 || got[0].w[2] != 0 {
		t.Errorf("a dimmed channel turns on at count 0, got ON=%#x%#x", got[0].w[2], got[0].w[1])
	}

	bus.reset()
	s.SetLampLevels(0, hardware.LampLevels{level})
	if n := len(chanWrites(bus, cfg.PCA9685.Addr)); n != 0 {
		t.Errorf("an unchanged level should write nothing, got %d writes", n)
	}
}

func TestPCAFanKeepsTheExpander(t *testing.T) {
	s, bus, _ := pcaSuite(t)
	bus.reset()

	s.fan.check(tempThreshold + 10)
	if !s.fan.isOn() {
		t.Fatal("a hot sample should engage the relay")
	}
	got := bus.forAddr(muxAddr)
	if len(got) != 1 || !reflect.DeepEqual(got[0].w, []byte{regOLATA, 1 << fanPin, 0x00}) {
		t.Fatalf("fan latch = %v, want port A bit %d set and the lamp bits low", got, fanPin)
	}
}

func TestPCAFanChannels(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Fan.PCAChannels = []int{7, 15}
	s, bus, _ := pcaSuiteWith(t, cfg)
	if s.FanDriver() != "pca9685" {
		t.Fatalf("FanDriver = %q, want pca9685", s.FanDriver())
	}
	bus.reset()

	s.fan.check(tempThreshold + 10)
	got := chanWrites(bus, cfg.PCA9685.Addr)
	if len(got) != 2 {
		t.Fatalf("a hot sample = one write per channel, got %d", len(got))
	}
	for i, ch := range cfg.Fan.PCAChannels {
		want := []byte{byte(pcaLED0OnL + 4*ch), 0, pcaFullBit, 0, 0}
		if !reflect.DeepEqual(got[i].w, want) {
			t.Errorf("channel %d = %v, want full-on %v", ch, got[i].w, want)
		}
	}
	if n := len(bus.forAddr(muxAddr)); n != 0 {
		t.Errorf("the expander should see no relay traffic, got %d writes", n)
	}

	bus.reset()
	s.fan.check(tempThreshold - 10)
	got = chanWrites(bus, cfg.PCA9685.Addr)
	if len(got) != 2 {
		t.Fatalf("a cool sample = one write per channel, got %d", len(got))
	}
	for i, ch := range cfg.Fan.PCAChannels {
		want := []byte{byte(pcaLED0OnL + 4*ch), 0, 0, 0, pcaFullBit}
		if !reflect.DeepEqual(got[i].w, want) {
			t.Errorf("channel %d = %v, want full-off %v", ch, got[i].w, want)
		}
	}
}

func TestPCAFanInverted(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Fan.PCAChannels = []int{7}
	cfg.Fan.Invert = true
	s, bus, _ := pcaSuiteWith(t, cfg)
	reg := byte(pcaLED0OnL + 4*cfg.Fan.PCAChannels[0])
	bus.reset()

	s.fan.check(tempThreshold + 10)
	got := chanWrites(bus, cfg.PCA9685.Addr)
	if len(got) != 1 || !reflect.DeepEqual(got[0].w, []byte{reg, 0, 0, 0, pcaFullBit}) {
		t.Fatalf("inverted relay on = %v, want the full-off pulse (output low = engaged)", got)
	}
	bus.reset()
	s.fan.check(tempThreshold - 10)
	got = chanWrites(bus, cfg.PCA9685.Addr)
	if len(got) != 1 || !reflect.DeepEqual(got[0].w, []byte{reg, 0, pcaFullBit, 0, 0}) {
		t.Fatalf("inverted relay off = %v, want full-on", got)
	}
}

func TestPCAFanModesOverrideTemperature(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Fan.PCAChannels = []int{7}
	s, bus, _ := pcaSuiteWith(t, cfg)
	reg := byte(pcaLED0OnL + 4*cfg.Fan.PCAChannels[0])
	bus.reset()

	s.SetFanConfig(tempThreshold, hardware.FanOn)
	got := chanWrites(bus, cfg.PCA9685.Addr)
	if len(got) != 1 || !reflect.DeepEqual(got[0].w, []byte{reg, 0, pcaFullBit, 0, 0}) {
		t.Fatalf("FanOn with no sample = %v, want the channel full-on", got)
	}
	if s.FanMode() != hardware.FanOn {
		t.Errorf("FanMode = %q, want on", s.FanMode())
	}
	bus.reset()
	s.fan.check(60)
	if n := len(chanWrites(bus, cfg.PCA9685.Addr)); n != 0 {
		t.Errorf("a cool sample must not drop a held-on relay, got %d writes", n)
	}

	s.SetFanConfig(tempThreshold, hardware.FanOff)
	bus.reset()
	s.fan.check(200)
	if n := len(chanWrites(bus, cfg.PCA9685.Addr)); n != 0 || s.FanOn() {
		t.Errorf("a hot sample must not raise a held-off relay: on=%v writes=%d", s.FanOn(), n)
	}
}

func TestSuiteWithoutExpander(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Fan.PCAChannels = []int{7}
	bus := &fakeBus{}
	bus.plug(cfg.PCA9685.Addr, pcaReset)
	bus.unplug(muxAddr)

	var errs []string
	s := newSuite(bus, func(level, msg string) {
		if level == "error" {
			errs = append(errs, msg)
		}
	}, noSleep, cfg)
	if len(errs) != 0 {
		t.Fatalf("a cabinet with no expander should log no errors, got %q", errs)
	}
	if s.mcp != nil {
		t.Fatal("no expander answered, so there should be no lamp writer")
	}
	if s.LampDriver() != "pca9685" || s.FanDriver() != "pca9685" {
		t.Fatalf("drivers = %q/%q, want pca9685 for both", s.LampDriver(), s.FanDriver())
	}
	bus.reset()

	s.SetLights(0, true)
	if n := len(chanWrites(bus, cfg.PCA9685.Addr)); n != lightsPerPlayer {
		t.Errorf("lamps = %d channel writes, want %d", n, lightsPerPlayer)
	}
	bus.reset()
	s.fan.check(tempThreshold + 10)
	got := chanWrites(bus, cfg.PCA9685.Addr)
	want := []byte{byte(pcaLED0OnL + 4*cfg.Fan.PCAChannels[0]), 0, pcaFullBit, 0, 0}
	if len(got) != 1 || !reflect.DeepEqual(got[0].w, want) {
		t.Errorf("fan = %v, want the relay channel full-on %v", got, want)
	}
	s.StatusLines()
	if err := s.Close(); err != nil {
		t.Errorf("Close = %v", err)
	}
}

func TestFanWithoutExpanderOrChannels(t *testing.T) {
	bus := &fakeBus{}
	bus.plug(pcaDefaultAddr, pcaReset)
	bus.unplug(muxAddr)
	var errs []string
	s := newSuite(bus, func(level, msg string) {
		if level == "error" {
			errs = append(errs, msg)
		}
	}, noSleep, DefaultConfig())
	if s.FanDriver() != "none" {
		t.Fatalf("FanDriver = %q, want none", s.FanDriver())
	}
	if len(errs) != 2 {
		t.Fatalf("want one fan error and one mux error, got %q", errs)
	}
	s.fan.check(tempThreshold + 10)
	if !s.FanOn() {
		t.Error("the fan should still track the state it wanted with no driver")
	}
}

func TestPCABacklightChannels(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PCA9685.Backlights = [2]int{6, 14}
	s, bus, _ := pcaSuiteWith(t, cfg)
	bus.reset()

	level := uint8((int(hardware.LampLevelMax) + 1) / 2)
	s.SetBacklightLevel(level)
	got := chanWrites(bus, cfg.PCA9685.Addr)
	if len(got) != 2 {
		t.Fatalf("two configured backlights = two channel writes, got %d", len(got))
	}
	for i, ch := range cfg.PCA9685.Backlights {
		if got[i].w[0] != byte(pcaLED0OnL+4*ch) {
			t.Errorf("backlight %d wrote channel register %#x, want %#x", i, got[i].w[0], pcaLED0OnL+4*ch)
		}
	}
	bus.reset()
	s.SetBacklightLevel(0)
	got = chanWrites(bus, cfg.PCA9685.Addr)
	if len(got) != 2 || !reflect.DeepEqual(got[0].w[1:], []byte{0, 0, 0, pcaFullBit}) {
		t.Errorf("dark backlight = %v, want both channels full-off", got)
	}

	for _, addr := range lcdAddrs {
		for _, r := range bus.forAddr(addr) {
			if r.w[0] != pcfBacklight {
				t.Errorf("lcd 0x%x wrote %#x, want the backlight bit held high", addr, r.w[0])
			}
		}
	}
}

func TestPCABacklightInverted(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PCA9685.Backlights = [2]int{14, 15}
	cfg.PCA9685.BacklightsInvert = true
	s, bus, _ := pcaSuiteWith(t, cfg)

	got := chanWrites(bus, cfg.PCA9685.Addr)
	if len(got) != 2 || !reflect.DeepEqual(got[0].w[1:], []byte{0, pcaFullBit, 0, 0}) {
		t.Fatalf("inverted dark backlight = %v, want full-on (output high = lamp off)", got)
	}
	bus.reset()
	s.SetBacklightLevel(0)
	if got := chanWrites(bus, cfg.PCA9685.Addr); len(got) != 0 {
		t.Errorf("a dark backlight asked for dark again = %v, want no bus traffic", got)
	}
	bus.reset()
	s.SetBacklightLevel(hardware.LampLevelMax)
	got = chanWrites(bus, cfg.PCA9685.Addr)
	if len(got) != 2 || !reflect.DeepEqual(got[0].w[1:], []byte{0, 0, 0, pcaFullBit}) {
		t.Fatalf("inverted full backlight = %v, want full-off", got)
	}
	bus.reset()
	level := uint8((int(hardware.LampLevelMax) + 1) / 2)
	s.SetBacklightLevel(level)
	got = chanWrites(bus, cfg.PCA9685.Addr)
	want := pcaSteps - s.pca.duty[level]
	if len(got) != 2 || got[0].w[3] != byte(want) || got[0].w[4] != byte(want>>8) {
		t.Fatalf("inverted half backlight = %v, want OFF=%d (4096 minus the gamma duty)", got, want)
	}
}

func TestPCABacklightBackpackLow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PCA9685.Backlights = [2]int{14, 15}
	cfg.PCA9685.BacklightsBackpackLow = true
	s, bus, _ := pcaSuiteWith(t, cfg)

	bus.reset()
	s.SetBacklightLevel(hardware.LampLevelMax)
	if got := chanWrites(bus, cfg.PCA9685.Addr); len(got) != 2 {
		t.Fatalf("two backlight channel writes, got %d", len(got))
	}
	for _, addr := range lcdAddrs {
		for _, r := range bus.forAddr(addr) {
			if r.w[0]&pcfBacklight != 0 {
				t.Errorf("lcd 0x%x wrote %#x, want the backlight bit held low", addr, r.w[0])
			}
		}
	}
}

func (f *fakeBus) records() []txRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]txRecord(nil), f.recs...)
}

func gateOpened(recs []txRecord, addr uint16) int {
	for i, r := range recs {
		if r.addr == addr && r.w[0]&pcfBacklight != 0 {
			return i
		}
	}
	return -1
}

func channelWritten(recs []txRecord, addr uint16, ch int) int {
	last := -1
	for i, r := range recs {
		if r.addr == addr && len(r.w) == 5 && r.w[0] == byte(pcaLED0OnL+4*ch) {
			last = i
		}
	}
	return last
}

func firstWrite(recs []txRecord, addr uint16) int {
	for i, r := range recs {
		if r.addr == addr {
			return i
		}
	}
	return -1
}

// TestPCABacklightWriteOrder pins that the channel is driven dark before either
// backpack gate opens: both power up conducting, so the reverse order flashes
// the panel at full.
func TestPCABacklightWriteOrder(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PCA9685.Backlights = [2]int{6, 14}
	cfg.PCA9685.BacklightsInvert = true
	s, bus, _ := pcaSuiteWith(t, cfg)
	pca := cfg.PCA9685.Addr

	recs := bus.records()
	for i, addr := range lcdAddrs {
		ch := cfg.PCA9685.Backlights[i]
		dark := channelWritten(recs, pca, ch)
		if dark < 0 {
			t.Fatalf("panel %d: initPWM never wrote channel %d", i, ch)
		}
		if recs[dark].w[2]&pcaFullBit == 0 {
			t.Errorf("panel %d: channel %d left at %v, want full-on (output high = dark)", i, ch, recs[dark].w[1:])
		}
		if first := firstWrite(recs, addr); first >= 0 && dark > first {
			t.Errorf("panel %d: channel darkened at write %d, after the LCD's first write at %d", i, dark, first)
		}
	}

	bus.reset()
	s.SetBacklightLevel(0)
	recs = bus.records()
	for i, addr := range lcdAddrs {
		gate := gateOpened(recs, addr)
		if gate < 0 {
			t.Fatalf("panel %d: the first set never opened the gate", i)
		}
		if wrote := channelWritten(recs, pca, cfg.PCA9685.Backlights[i]); wrote > gate {
			t.Errorf("panel %d: gate opened at write %d, before the channel at %d", i, gate, wrote)
		}
	}

	for _, step := range []struct {
		name  string
		level uint8
	}{
		{"0 → 30%", hardware.LampLevelMax * 30 / 100},
		{"30% → 0", 0},
	} {
		bus.reset()
		s.SetBacklightLevel(step.level)
		if got := chanWrites(bus, pca); len(got) != 2 {
			t.Errorf("%s: %d channel writes, want one per panel", step.name, len(got))
		}
		for _, addr := range lcdAddrs {
			if n := len(bus.forAddr(addr)); n != 0 {
				t.Errorf("%s: %d writes to backpack 0x%x, want the gate left alone", step.name, n, addr)
			}
		}
	}
}
