// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// Registers and bits from the NXP PCA9685 datasheet (Rev 4).
const (
	pcaDefaultAddr   = 0x40
	pcaDefaultFreqHz = 1000

	pcaMode1     = 0x00
	pcaMode2     = 0x01
	pcaLED0OnL   = 0x06 // four bytes per channel: ON_L, ON_H, OFF_L, OFF_H
	pcaAllLEDOnL = 0xfa
	pcaPreScale  = 0xfe // writable only while SLEEP is set

	pcaRestart = 0x80 // MODE1
	pcaAutoInc = 0x20
	pcaSleep   = 0x10

	pcaTotemPole = 0x04 // MODE2 OUTDRV
	pcaInvert    = 0x10 // MODE2 INVRT

	pcaFullBit = 0x10 // bit 4 of ON_H (full on) or OFF_H (full off)

	pcaChannels    = 16
	pcaSteps       = 4096
	pcaOscHz       = 25_000_000 // internal oscillator
	pcaMinPrescale = 3
	pcaMaxPrescale = 255

	pcaWakeDelay = 500 * time.Microsecond // oscillator settle after SLEEP clears

	// gamma applied to duty so levels look perceptually even
	pcaGammaExp = 2.2
)

// pca9685 is a 16-channel 12-bit PWM driver. Channel state is cached so a
// repeated level costs no bus traffic.
type pca9685 struct {
	bus  I2CBus
	addr uint16
	duty []uint16

	mu     sync.Mutex
	levels [pcaChannels]uint8
	inv    [pcaChannels]bool
}

// newPCA9685 probes addr, sets the PWM frequency and leaves every channel
// full-off.
func newPCA9685(bus I2CBus, addr uint16, freqHz int, invert bool, sleep func(time.Duration)) (*pca9685, error) {
	mode1 := make([]byte, 1)
	if err := bus.Tx(addr, []byte{pcaMode1}, mode1); err != nil {
		return nil, fmt.Errorf("pca9685: no device at %#x: %w", addr, err)
	}
	p := &pca9685{bus: bus, addr: addr, duty: pcaGammaTable()}
	mode2 := byte(pcaTotemPole)
	if invert {
		mode2 |= pcaInvert
	}
	for _, w := range [][]byte{
		{pcaMode1, (mode1[0] &^ pcaRestart) | pcaSleep},
		{pcaPreScale, pcaPrescale(freqHz)},
		// ALLCALL stays off so address 0x70 is free to probe a second board
		{pcaMode1, pcaAutoInc},
	} {
		if err := p.write(w...); err != nil {
			return nil, err
		}
	}
	sleep(pcaWakeDelay)
	if mode1[0]&pcaRestart != 0 {
		if err := p.write(pcaMode1, pcaAutoInc|pcaRestart); err != nil {
			return nil, err
		}
	}
	if err := p.write(pcaMode2, mode2); err != nil {
		return nil, err
	}
	if err := p.write(pcaAllLEDOnL, 0, 0, 0, pcaFullBit); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *pca9685) write(b ...byte) error { return p.bus.Tx(p.addr, b, nil) }

func (p *pca9685) setChannel(ch int, level uint8) error {
	return p.setChannelInverted(ch, level, false)
}

// setChannelInverted is setChannel for a load lit while the output is low.
func (p *pca9685) setChannelInverted(ch int, level uint8, inverted bool) error {
	if ch < 0 || ch >= pcaChannels {
		return fmt.Errorf("pca9685: channel %d out of range", ch)
	}
	if level > hardware.LampLevelMax {
		level = hardware.LampLevelMax
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.levels[ch] == level && p.inv[ch] == inverted {
		return nil
	}
	on, off := p.pulse(level)
	if inverted {
		on, off = p.pulseInverted(level)
	}
	if err := p.write(pcaLED0OnL+byte(4*ch), byte(on), byte(on>>8), byte(off), byte(off>>8)); err != nil {
		return err
	}
	p.levels[ch] = level
	p.inv[ch] = inverted
	return nil
}

func (p *pca9685) pulse(level uint8) (on, off uint16) {
	switch level {
	case 0:
		return 0, pcaFullBit << 8
	case hardware.LampLevelMax:
		return pcaFullBit << 8, 0
	}
	return 0, p.duty[level]
}

// pulseInverted mirrors pulse so the load is lit for the same duty.
func (p *pca9685) pulseInverted(level uint8) (on, off uint16) {
	switch level {
	case 0:
		return pcaFullBit << 8, 0
	case hardware.LampLevelMax:
		return 0, pcaFullBit << 8
	}
	return 0, pcaSteps - p.duty[level]
}

func pcaGammaTable() []uint16 {
	t := make([]uint16, int(hardware.LampLevelMax)+1)
	for i := range t {
		frac := float64(i) / float64(hardware.LampLevelMax)
		t[i] = uint16(math.Round(float64(pcaSteps-1) * math.Pow(frac, pcaGammaExp)))
	}
	return t
}

func pcaPrescale(freqHz int) byte {
	if freqHz <= 0 {
		freqHz = pcaDefaultFreqHz
	}
	v := int(math.Round(float64(pcaOscHz)/float64(pcaSteps*freqHz))) - 1
	if v < pcaMinPrescale {
		v = pcaMinPrescale
	}
	if v > pcaMaxPrescale {
		v = pcaMaxPrescale
	}
	return byte(v)
}

type pcaLamps struct {
	dev    *pca9685
	logger func(level, msg string)
	chans  [2][lightsPerPlayer]int
}

func newPCALamps(dev *pca9685, chans [2][lightsPerPlayer]int, logger func(level, msg string)) *pcaLamps {
	return &pcaLamps{dev: dev, logger: logger, chans: chans}
}

func (p *pcaLamps) setLamps(seat int, levels hardware.LampLevels) {
	if seat < 0 || seat > 1 {
		return
	}
	clamped := levels.Clamp()
	for i, ch := range p.chans[seat] {
		if ch < 0 {
			continue
		}
		if err := p.dev.setChannel(ch, clamped[i]); err != nil {
			p.logger("error", fmt.Sprintf("lamps: pca channel %d: %v", ch, err))
		}
	}
}

// stop is a no-op; the chip holds its PWM.
func (p *pcaLamps) stop() {}
