// SPDX-License-Identifier: GPL-2.0-or-later

// Package pi drives the cabinet peripherals on a Raspberry Pi: two HD44780
// LCDs on PCF8574 backpacks, button lamps and a fan relay on an MCP23017 or a
// PCA9685, and a PIR motion sensor. It is the hardware counterpart of
// hardware.Virtual; New fails off-Pi so callers can fall back. Device logic
// runs over I2CBus so tests drive a fake bus.
package pi

import (
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/host/v3"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

const lcdCols = hardware.LcdCols

// I2CBus is the minimal bus contract, matching periph.io's i2c.Bus.Tx. The
// devices here only write, so r is always nil.
type I2CBus interface {
	Tx(addr uint16, w, r []byte) error
}

// ErrNotPi is returned by New when /proc/cpuinfo does not identify a
// Raspberry Pi; callers should fall back to hardware.Virtual.
var ErrNotPi = errors.New("hardware/pi: not running on a Raspberry Pi")

var (
	cpuInfoPath     = "/proc/cpuinfo"
	thermalZonePath = "/sys/class/thermal/thermal_zone0/temp"
)

var piModelRE = regexp.MustCompile(`Model\s*:\s*Raspberry Pi`)

func IsPi() bool {
	b, err := os.ReadFile(cpuInfoPath)
	if err != nil {
		return false
	}
	return piModelRE.Match(b)
}

// CPUTempF reads the SoC temperature in °F, or -Inf off-Pi or on a read or
// parse failure.
func CPUTempF() float64 {
	if !IsPi() {
		return math.Inf(-1)
	}
	b, err := os.ReadFile(thermalZonePath)
	if err != nil {
		return math.Inf(-1)
	}
	milli, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
	if err != nil {
		return math.Inf(-1)
	}
	return milli/1000*1.8 + 32
}

// lcdAddrs are the PCF8574 backpack addresses, player 1 then player 2.
var lcdAddrs = [2]uint16{0x27, 0x23}

type lampSink interface {
	setLamps(seat int, levels hardware.LampLevels)
	stop()
}

type Config struct {
	PCA9685 PCA9685Config
	Fan     FanConfig
}

// FanConfig says where the cooling relay's input hangs. PCAChannels win when
// the board answered its probe; otherwise ExpanderPins drive it. Extra
// entries take the same value at the same moment.
type FanConfig struct {
	ExpanderPins []int
	PCAChannels  []int
	// Invert flips the PCA channels for a relay board that engages on a LOW
	// input; expander pins are always active-high.
	Invert bool
}

// PCA9685Config describes the optional PWM board and which channel each lamp
// and backlight uses. A channel of -1 is not wired to it.
type PCA9685Config struct {
	Enabled     *bool
	Addr        uint16
	FrequencyHz int
	Invert      bool
	Lamps       [2][lightsPerPlayer]int
	Backlights  [2]int
	// BacklightsInvert flips the backlight channels alone, for a backpack
	// whose backlight transistor switches on with a LOW base.
	BacklightsInvert bool
	// BacklightsBackpackLow holds the backpack's backlight bit low while the
	// PCA owns the backlight, for low-side wiring with an external transistor.
	// The default holds it high.
	BacklightsBackpackLow bool
}

// DefaultConfig is the cabinet's wiring: PCA9685 probed at 0x40 and 1 kHz,
// lamps on channels 0-5 and 8-13, backlights on the backpacks, fan relay on
// expander port A pin 7.
func DefaultConfig() Config {
	return Config{
		PCA9685: PCA9685Config{
			Enabled:     boolPtr(true),
			Addr:        pcaDefaultAddr,
			FrequencyHz: pcaDefaultFreqHz,
			Lamps:       [2][lightsPerPlayer]int{{0, 1, 2, 3, 4, 5}, {8, 9, 10, 11, 12, 13}},
			Backlights:  [2]int{-1, -1},
		},
		Fan: FanConfig{ExpanderPins: []int{fanPin}},
	}
}

func boolPtr(b bool) *bool { return &b }

func (c PCA9685Config) enabled() bool { return c.Enabled == nil || *c.Enabled }

type Suite struct {
	mu                    sync.Mutex
	logger                func(level, msg string)
	closer                interface{ Close() error } // real I2C bus; nil under test
	lamps                 lampSink                   // nil when no lamp driver came up
	mcp                   *lampWriter                // nil when the expander failed init, or is gone
	pca                   *pca9685                   // nil unless the PWM board answered
	fanDriver             string
	backlights            [2]int
	backlightsInvert      bool
	backlightsBackpackLow bool
	backlightLevel        uint8
	screens               [2]*screen // entries nil when an LCD failed init
	glyphs                *glyphTable
	fan                   *fan
	motion                *motionSensor // nil off-GPIO
	levels                [2]hardware.LampLevels
	closed                bool
	hardware.MicSlot      // internal/mic's capture on the cabinet
}

var _ hardware.Suite = (*Suite)(nil)

// New probes the Pi, opens I2C bus 1 and wires every device. An error means no
// usable hardware; device failures after the bus opens are logged and that
// device is left nil.
func New(logger func(level, msg string), cfg Config) (*Suite, error) {
	if !IsPi() {
		return nil, ErrNotPi
	}
	if _, err := host.Init(); err != nil {
		return nil, fmt.Errorf("hardware/pi: host init: %w", err)
	}
	bus, err := i2creg.Open("1")
	if err != nil {
		return nil, fmt.Errorf("hardware/pi: open i2c bus 1: %w", err)
	}
	s := newSuite(bus, logger, time.Sleep, cfg)
	s.closer = bus
	s.startWorkers()
	s.fan.start()
	s.initMotion()
	return s, nil
}

// startWorkers launches the screen and lamp writer goroutines so setters
// return without waiting on the bus. newSuite stays synchronous for tests.
func (s *Suite) startWorkers() {
	for _, sc := range s.screens {
		if sc != nil {
			sc.startWorker(true)
		}
	}
	if s.mcp != nil {
		s.mcp.start()
	}
}

func newSuite(bus I2CBus, logger func(level, msg string), sleep func(time.Duration), cfg Config) *Suite {
	if logger == nil {
		logger = func(string, string) {}
	}
	s := &Suite{logger: logger, glyphs: &glyphTable{}, backlights: cfg.PCA9685.Backlights, backlightsInvert: cfg.PCA9685.BacklightsInvert, backlightsBackpackLow: cfg.PCA9685.BacklightsBackpackLow}
	mux, muxErr := newMCP23017(bus, muxAddr)
	if muxErr == nil {
		s.mcp = newLampWriter(mux, s.logger, sleep)
		s.lamps = s.mcp
	}
	s.initPWM(bus, sleep, cfg.PCA9685)
	for i, addr := range lcdAddrs {
		dev, err := newHD44780At(bus, addr, sleep, 0)
		if err != nil {
			s.log("error", fmt.Sprintf("lcd failed at 0x%x: %v", addr, err))
			continue
		}
		s.screens[i] = newScreen(dev, s.glyphs, logger)
	}
	s.fan = newFan(s.initFanSink(cfg.Fan), logger)
	// logged last: a missing expander is expected once the PCA9685 has both
	if muxErr != nil {
		if s.pca != nil && s.fanDriver == "pca9685" {
			s.log("info", fmt.Sprintf("no MCP23017 at 0x%x: the PCA9685 has the lamps and the fan", muxAddr))
		} else {
			s.log("error", fmt.Sprintf("mux failed at 0x%x: %v", muxAddr, muxErr))
		}
	}
	return s
}

// initFanSink picks the relay's driver: the PCA9685 when configured and
// present, else the expander. With neither the fan is a logged no-op.
func (s *Suite) initFanSink(cfg FanConfig) fanSink {
	if len(cfg.PCAChannels) > 0 && s.pca != nil {
		s.fanDriver = "pca9685"
		s.log("info", fmt.Sprintf("fan: PCA9685 channels %s (inverted=%v)", joinInts(cfg.PCAChannels), cfg.Invert))
		return &pcaFan{dev: s.pca, chans: cfg.PCAChannels, invert: cfg.Invert}
	}
	if len(cfg.ExpanderPins) > 0 && s.mcp != nil {
		s.fanDriver = "mcp23017"
		s.log("info", fmt.Sprintf("fan: MCP23017 port A pins %s", joinInts(cfg.ExpanderPins)))
		return &expanderFan{w: s.mcp, pins: cfg.ExpanderPins}
	}
	s.fanDriver = "none"
	s.log("error", "fan: no relay driver (no PCA9685 channels configured and no MCP23017)")
	return nil
}

func joinInts(v []int) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ", ")
}

// initPWM hands the lamps to the PCA9685 when one answers the probe.
//
// The backlight channels are driven dark here, before either LCD wakes:
// newPCA9685 leaves channels full-off, which is the lit end of an inverted
// backlight channel, and the PCF8574 gate may already be high.
func (s *Suite) initPWM(bus I2CBus, sleep func(time.Duration), cfg PCA9685Config) {
	if !cfg.enabled() {
		return
	}
	dev, err := newPCA9685(bus, cfg.Addr, cfg.FrequencyHz, cfg.Invert, sleep)
	if err != nil {
		s.log("info", fmt.Sprintf("lamps: MCP23017 bit-angle (no PCA9685 at %#x)", cfg.Addr))
		return
	}
	s.pca = dev
	for _, ch := range cfg.Backlights {
		s.writeBacklightChannel(ch, 0)
	}
	s.lamps = newPCALamps(dev, cfg.Lamps, s.logger)
	s.log("info", fmt.Sprintf("lamps: PCA9685 at %#x, %d Hz", cfg.Addr, cfg.FrequencyHz))
	if cfg.Backlights[0] >= 0 || cfg.Backlights[1] >= 0 {
		s.log("info", fmt.Sprintf("backlights: PCA9685 channels %d and %d (inverted=%v, backpack bit low=%v)",
			cfg.Backlights[0], cfg.Backlights[1], cfg.BacklightsInvert, cfg.BacklightsBackpackLow))
	}
}

// LampDriver names the lamp back-end: "pca9685", "mcp23017" or "none".
func (s *Suite) LampDriver() string {
	switch {
	case s.pca != nil:
		return "pca9685"
	case s.mcp != nil:
		return "mcp23017"
	}
	return "none"
}

// FanDriver names the relay back-end: "pca9685", "mcp23017" or "none".
func (s *Suite) FanDriver() string { return s.fanDriver }

func (s *Suite) log(level, msg string) { s.logger(level, msg) }

// initMotion arms the PIR on GPIO5; failure only logs.
func (s *Suite) initMotion() {
	pin := gpioreg.ByName("GPIO5")
	if pin == nil {
		s.log("warn", "motion: GPIO5 not found, sensor disabled")
		return
	}
	if err := pin.In(gpio.PullDown, gpio.BothEdges); err != nil {
		s.log("warn", fmt.Sprintf("motion: GPIO5 setup failed: %v", err))
		return
	}
	s.motion = newMotionSensor(pin, s.logger)
	go s.motion.run()
}

func (s *Suite) SetText(index int, line1, line2 string) {
	for i, sc := range s.screens {
		if index >= 0 && index != i {
			continue
		}
		if sc != nil {
			sc.setText(line1, line2)
		}
	}
}

func (s *Suite) SetLights(index int, on bool) {
	var levels hardware.LampLevels
	if on {
		levels = hardware.LampLevelsOf(hardware.AllLights)
	}
	s.setSeatLevels(index, func(hardware.LampLevels) hardware.LampLevels { return levels })
}

// setSeatLevels replaces each addressed seat's levels with next's result and
// hands them to the lamp sink.
func (s *Suite) setSeatLevels(index int, next func(hardware.LampLevels) hardware.LampLevels) {
	s.mu.Lock()
	var touched [2]bool
	for i := range s.levels {
		if index >= 0 && index != i {
			continue
		}
		s.levels[i] = next(s.levels[i]).Clamp()
		touched[i] = true
	}
	levels := s.levels
	s.mu.Unlock()
	if s.lamps == nil {
		return
	}
	for i, t := range touched {
		if t {
			s.lamps.setLamps(i, levels[i])
		}
	}
}

// StatusLines mirrors the Virtual header line and appends temperature, fan and
// motion indicators.
func (s *Suite) StatusLines() []string {
	s.mu.Lock()
	levels := s.levels
	s.mu.Unlock()
	light := func(l hardware.LampLevels) string {
		if l.Mask() != 0 {
			return "◉"
		}
		return "○"
	}
	window := func(i, row int) string {
		if s.screens[i] == nil {
			return padRight("", lcdCols)
		}
		return s.screens[i].window(row)
	}
	motion := "⏾"
	if s.motion != nil {
		motion = s.motion.glyph()
	}
	fanGlyph, temp := s.fan.status()
	return []string{
		fmt.Sprintf("P1 %s |%s| |%s|   P2 %s |%s| |%s|",
			light(levels[0]), window(0, 0), window(0, 1),
			light(levels[1]), window(1, 0), window(1, 1)),
		fmt.Sprintf("Fan %s %s   Motion %s", fanGlyph, temp, motion),
	}
}

// Close stops the tickers and edge loop, then leaves each LCD reading the
// power-off message, unless the engine's sequence already did. The lamps keep
// what they showed, rounded to on/off.
//
// A PCA9685 backlight keeps its PWM after exit, so it is left at the faded
// level; a backpack-bit backlight is switched fully on.
//
// Writers are joined before any teardown write so the goodbye cannot race
// queued work.
func (s *Suite) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	s.fan.stopTicker()
	if s.motion != nil {
		s.motion.close()
	}
	if s.lamps != nil {
		s.lamps.stop()
	}
	if s.mcp != nil {
		s.mcp.stop() // still the fan relay's writer when the PCA drives the lamps
	}
	for i, sc := range s.screens {
		if sc != nil {
			sc.close(s.pwmBacklight(i) < 0)
		}
	}
	if s.closer != nil {
		return s.closer.Close()
	}
	return nil
}

// SetButtonLights updates individual buttons; only=true darkens unmentioned
// buttons, otherwise they keep their level.
func (s *Suite) SetButtonLights(index int, lights hardware.ButtonLights, only bool) {
	s.setSeatLevels(index, func(cur hardware.LampLevels) hardware.LampLevels {
		if only {
			cur = hardware.LampLevels{}
		}
		for btn, name := range hardware.LightButtons {
			if on, mentioned := lights[name]; mentioned {
				cur[btn] = 0
				if on {
					cur[btn] = hardware.LampLevelMax
				}
			}
		}
		return cur
	})
}

func (s *Suite) SetLampLevels(index int, levels hardware.LampLevels) {
	s.setSeatLevels(index, func(hardware.LampLevels) hardware.LampLevels { return levels })
}

func (s *Suite) SetBacklight(on bool) {
	level := uint8(0)
	if on {
		level = hardware.LampLevelMax
	}
	s.SetBacklightLevel(level)
}

// SetBacklightLevel dims both LCD backlights, 0 (dark) to LampLevelMax (full).
// Intermediate levels are modulated by each screen's worker, or by the PCA9685
// channel when the backlight hangs off one, in which case the backpack bit
// only gates the supply.
//
// The channel is written before the gate; opening the gate over a channel
// still at its lit end flashes the panel at full.
func (s *Suite) SetBacklightLevel(level uint8) {
	s.mu.Lock()
	s.backlightLevel = min(level, hardware.LampLevelMax)
	s.mu.Unlock()
	for i, sc := range s.screens {
		backpack := level
		if ch := s.pwmBacklight(i); ch >= 0 {
			s.writeBacklightChannel(ch, level)
			backpack = s.backpackGate()
		}
		if sc != nil {
			sc.setBacklightLevel(backpack)
		}
	}
}

func (s *Suite) BacklightLevel() uint8 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.backlightLevel
}

// backpackGate is the level the backpack's backlight bit is pinned to while a
// PCA channel carries the brightness.
func (s *Suite) backpackGate() uint8 {
	if s.backlightsBackpackLow {
		return 0
	}
	return hardware.LampLevelMax
}

func (s *Suite) writeBacklightChannel(ch int, level uint8) {
	if ch < 0 || s.pca == nil {
		return
	}
	if err := s.pca.setChannelInverted(ch, level, s.backlightsInvert); err != nil {
		s.log("error", fmt.Sprintf("backlight: pca channel %d: %v", ch, err))
	}
}

func (s *Suite) pwmBacklight(i int) int {
	if s.pca == nil {
		return -1
	}
	return s.backlights[i]
}

var (
	_ hardware.BacklightDimmer = (*Suite)(nil)
	_ hardware.BacklightReader = (*Suite)(nil)
)

func (s *Suite) MotionActive() bool {
	if s.motion == nil {
		return false
	}
	return s.motion.State()
}

func (s *Suite) CPUTempF() (degF float64, ok bool) { return s.fan.tempF() }

func (s *Suite) FanOn() bool { return s.fan.isOn() }

// SetFanConfig updates the cooling threshold (°F) and the Auto/On/Off mode,
// applied immediately.
func (s *Suite) SetFanConfig(thresholdF float64, mode hardware.FanMode) {
	s.fan.setConfig(thresholdF, mode)
}

func (s *Suite) FanMode() hardware.FanMode { return s.fan.mode() }

var (
	_ hardware.MicSource     = (*Suite)(nil)
	_ hardware.MotionSensor  = (*Suite)(nil)
	_ hardware.MotionHistory = (*Suite)(nil)
	_ hardware.InputSpy      = (*Suite)(nil)
	_ hardware.ThermalSensor = (*Suite)(nil)
	_ hardware.CoolingFan    = (*Suite)(nil)
)

// LastMotion reports the last PIR rising edge; ok is false with no sensor or
// before the first edge.
func (s *Suite) LastMotion() (time.Time, bool) {
	if s.motion == nil {
		return time.Time{}, false
	}
	return s.motion.lastMotion()
}

func (s *Suite) InputSignal() {
	if s.motion == nil {
		return
	}
	s.motion.mu.Lock()
	defer s.motion.mu.Unlock()
	now := time.Now()
	s.motion.d.edge(true, now)
	s.motion.d.edge(false, now) // arms the 5s clear deadline
}
