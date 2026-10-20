// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"fmt"
	"io"
	"time"

	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/host/v3"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// OpenBus opens I2C bus 1 and returns it with its closer, for tools that
// drive the panels without an engine.
func OpenBus() (I2CBus, io.Closer, error) {
	if _, err := host.Init(); err != nil {
		return nil, nil, fmt.Errorf("hardware/pi: host init: %w", err)
	}
	bus, err := i2creg.Open("1")
	if err != nil {
		return nil, nil, fmt.Errorf("hardware/pi: open i2c bus 1: %w", err)
	}
	return bus, bus, nil
}

func LCDAddrs() [2]uint16 { return lcdAddrs }

// Panel is one 16x2 LCD with its own I/O goroutine and no marquee; the caller
// writes 16 columns a row and animates them itself.
type Panel struct{ s *screen }

// NewPanel wakes the HD44780 at addr with the backlight dark throughout init.
// glyphs adds 5x8 bitmaps for runes the built-in table lacks, or replaces
// ones it has; nil draws with the built-in table alone.
func NewPanel(bus I2CBus, addr uint16, glyphs map[rune]hardware.GlyphRows, logger func(level, msg string)) (*Panel, error) {
	dev, err := newHD44780At(bus, addr, time.Sleep, 0)
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = func(string, string) {}
	}
	table := &glyphTable{}
	table.setCart(glyphs)
	p := &Panel{s: newScreen(dev, table, logger)}
	p.s.startWorker(false)
	return p, nil
}

func (p *Panel) SetText(line1, line2 string) { p.s.setText(line1, line2) }

// SetBacklightLevel queues a brightness, 0 to hardware.LampLevelMax. The
// panel's worker modulates intermediate levels.
func (p *Panel) SetBacklightLevel(level uint8) { p.s.setBacklightLevel(level) }

// Close joins the worker and leaves the display as it stands, with no goodbye
// text.
func (p *Panel) Close() { p.s.stopWorker() }

// PWM is the PCA9685 without a Suite.
type PWM struct{ dev *pca9685 }

// NewPWM initialises the board at cfg.Addr, leaving every channel full-off.
// It returns nil, nil when the config disables the board, and an error when
// nothing answers the probe; a nil *PWM is safe to use.
func NewPWM(bus I2CBus, cfg PCA9685Config) (*PWM, error) {
	if !cfg.enabled() {
		return nil, nil
	}
	dev, err := newPCA9685(bus, cfg.Addr, cfg.FrequencyHz, cfg.Invert, time.Sleep)
	if err != nil {
		return nil, err
	}
	return &PWM{dev: dev}, nil
}

// SetChannel drives one channel at a lamp level, mirrored for a load lit while
// the output is low. A nil receiver or channel -1 does nothing.
func (p *PWM) SetChannel(ch int, level uint8, inverted bool) error {
	if p == nil || ch < 0 {
		return nil
	}
	return p.dev.setChannelInverted(ch, level, inverted)
}
