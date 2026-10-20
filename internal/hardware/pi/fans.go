// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

const (
	tempCheckTick = 10 * time.Second
	tempThreshold = 120.0 // °F

	DefaultFanThresholdF = tempThreshold
)

var fanAnimChars = []string{"◐", "◓", "◑", "◒"}

// fanSink drives the relay input to a logic level: the expander's output pins
// or a PCA9685 channel held static.
type fanSink interface {
	setFan(on bool) error
}

// expanderFan puts the relay on MCP23017 pins, riding along with the lamp
// writes.
type expanderFan struct {
	w    *lampWriter
	pins []int
}

func (e *expanderFan) setFan(on bool) error {
	for _, pin := range e.pins {
		e.w.setPin(pin, on)
	}
	return nil
}

// pcaFan puts the relay on PCA9685 channels, only ever full-on or full-off
// since an intermediate duty would chatter the coil.
type pcaFan struct {
	dev    *pca9685
	chans  []int
	invert bool
}

func (p *pcaFan) setFan(on bool) error {
	level := uint8(0)
	if on {
		level = hardware.LampLevelMax
	}
	var firstErr error
	for _, ch := range p.chans {
		if err := p.dev.setChannelInverted(ch, level, p.invert); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// fanStep decides the relay transition for one temperature sample; changed is
// false when the relay is already right. FanOn and FanOff pin the relay and
// ignore the temperature.
func fanStep(on bool, tempF, threshold float64, mode hardware.FanMode) (newOn, changed bool) {
	switch mode {
	case hardware.FanOn:
		return true, !on
	case hardware.FanOff:
		return false, on
	}
	if !on && tempF >= threshold {
		return true, true
	}
	if on && tempF < threshold {
		return false, true
	}
	return on, false
}

// fan polls the CPU temperature and drives the relay through its sink, which
// is nil when no driver came up.
type fan struct {
	mu        sync.Mutex
	sink      fanSink
	logger    func(level, msg string)
	on        bool
	lastTemp  float64 // -Inf until the first sample
	fanMode   hardware.FanMode
	threshold float64 // °F
	animFrame int
	stop      chan struct{}
}

func newFan(sink fanSink, logger func(level, msg string)) *fan {
	if logger == nil {
		logger = func(string, string) {}
	}
	return &fan{sink: sink, logger: logger, lastTemp: math.Inf(-1), fanMode: hardware.FanAuto, threshold: tempThreshold}
}

func (f *fan) start() {
	f.stop = make(chan struct{})
	go func() {
		t := time.NewTicker(tempCheckTick)
		defer t.Stop()
		for {
			select {
			case <-f.stop:
				return
			case <-t.C:
				f.check(CPUTempF())
			}
		}
	}()
}

func (f *fan) stopTicker() {
	if f.stop != nil {
		close(f.stop)
		f.stop = nil
	}
}

func (f *fan) check(tempF float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastTemp = tempF
	f.applyLocked(tempF)
}

// setConfig updates the threshold and mode and re-evaluates the relay against
// the last sample at once. Before the first sample lastTemp is -Inf, which
// keeps the relay off.
func (f *fan) setConfig(thresholdF float64, mode hardware.FanMode) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.threshold = thresholdF
	f.fanMode = mode
	f.applyLocked(f.lastTemp)
}

// applyLocked runs one fanStep and writes the relay on a transition. Caller
// holds f.mu.
func (f *fan) applyLocked(tempF float64) {
	on, changed := fanStep(f.on, tempF, f.threshold, f.fanMode)
	if !changed {
		return
	}
	f.on = on
	if f.sink == nil {
		return
	}
	if err := f.sink.setFan(on); err != nil {
		f.logger("error", fmt.Sprintf("fan: relay write: %v", err))
	}
}

func (f *fan) tempF() (degF float64, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if math.IsInf(f.lastTemp, -1) {
		return 0, false
	}
	return f.lastTemp, true
}

func (f *fan) isOn() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.on
}

func (f *fan) mode() hardware.FanMode {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fanMode
}

// status renders the header indicator: a spinner frame per read while
// running, ✻ when idle, and the last temperature ("~°F" before a sample).
func (f *fan) status() (glyph, temp string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	glyph = "✻"
	if f.on {
		f.animFrame = (f.animFrame + 1) % len(fanAnimChars)
		glyph = fanAnimChars[f.animFrame]
	}
	if math.IsInf(f.lastTemp, -1) {
		return glyph, "~°F"
	}
	return glyph, fmt.Sprintf("%.0f°F", f.lastTemp)
}
