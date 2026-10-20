// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"math"
	"time"

	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/hardware/pi"
)

// Brightnesses on the lamps' 0..255 scale. The idle backlight is the level the whole boot runs at.
const (
	backlightIdle   uint8 = 76  // 30%
	backlightBreath uint8 = 140 // 55%, the top of the crash card's breath
	redLampPeak     uint8 = 102 // 40%

	breathPeriod = 2 * time.Second
	pulsePeriod  = 2 * time.Second // the crash card's red lamps, 0.5 Hz

	finishHold = 300 * time.Millisecond
)

// redButton is the red lamp's index in hardware.LightButtons order, which is
// also settings.json's channel order.
const redButton = 1

// cabinet is the two panels and the PWM board; setText skips unchanged text to spare I2C.
type cabinet struct {
	cfg      pi.PCA9685Config
	pwm      *pi.PWM
	panels   [2]*pi.Panel
	lastText [2][2]string
}

func openCabinet(bus pi.I2CBus, cfg pi.Config) *cabinet {
	c := &cabinet{cfg: cfg.PCA9685}
	pwm, err := pi.NewPWM(bus, cfg.PCA9685)
	if err != nil {
		logf("no PCA9685 at %#x: lamps and backlights stay on their backpacks (%v)", cfg.PCA9685.Addr, err)
	}
	c.pwm = pwm
	c.darkenBacklights()
	for i, addr := range pi.LCDAddrs() {
		panel, err := pi.NewPanel(bus, addr, bootGlyphs, func(level, msg string) { logf("%s: %s", level, msg) })
		if err != nil {
			logf("lcd at %#x: %v", addr, err)
			continue
		}
		c.panels[i] = panel
	}
	return c
}

func (c *cabinet) live() bool { return c.panels[0] != nil || c.panels[1] != nil }

func (c *cabinet) setText(index int, lines [2]string) {
	if c.lastText[index] == lines {
		return
	}
	c.lastText[index] = lines
	if p := c.panels[index]; p != nil {
		p.SetText(lines[0], lines[1])
	}
}

// setBacklight follows the engine's split: a PCA-driven backlight carries the
// brightness and the backpack bit is pinned open.
func (c *cabinet) setBacklight(level uint8) {
	for i, p := range c.panels {
		backpack := level
		if ch := c.cfg.Backlights[i]; ch >= 0 && c.pwm != nil {
			c.logErr(c.pwm.SetChannel(ch, level, c.cfg.BacklightsInvert))
			backpack = hardware.LampLevelMax
			if c.cfg.BacklightsBackpackLow {
				backpack = 0
			}
		}
		if p != nil {
			p.SetBacklightLevel(backpack)
		}
	}
}

func (c *cabinet) setLamp(seat, button int, level uint8) {
	c.logErr(c.pwm.SetChannel(c.cfg.Lamps[seat][button], level, false))
}

func (c *cabinet) darkenLamps() {
	for seat := range c.cfg.Lamps {
		for button := range c.cfg.Lamps[seat] {
			c.setLamp(seat, button, 0)
		}
	}
}

// darkenBacklights runs before either panel wakes: an inverted backlight
// channel's reset state is lit, and the PCF8574 gating it powers up high.
func (c *cabinet) darkenBacklights() {
	for _, ch := range c.cfg.Backlights {
		c.logErr(c.pwm.SetChannel(ch, 0, c.cfg.BacklightsInvert))
	}
}

// finish shows the full bar and holds it briefly. The backlight stays at
// backlightIdle, the level cmd/arkade's startup card continues from.
func (c *cabinet) finish(stage string) {
	c.setText(0, finalLines(stage))
	c.setBacklight(backlightIdle)
	time.Sleep(finishHold)
}

func (c *cabinet) close() {
	for _, p := range c.panels {
		if p != nil {
			p.Close()
		}
	}
}

func (c *cabinet) logErr(err error) {
	if err != nil {
		logf("%v", err)
	}
}

// wave is a 0..1 sine over one period, starting and ending at the bottom.
func wave(t, period time.Duration) float64 {
	return 0.5 - 0.5*math.Cos(2*math.Pi*float64(t)/float64(period))
}

func lerpLevel(lo, hi uint8, frac float64) uint8 {
	return uint8(math.Round(float64(lo) + (float64(hi)-float64(lo))*clamp01(frac)))
}
