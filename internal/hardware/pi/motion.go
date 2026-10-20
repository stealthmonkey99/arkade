// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"sync"
	"time"

	"periph.io/x/conn/v3/gpio"
)

const motionLossTimeout = 5 * time.Second

var motionAnimChars = []string{"←", "↖", "↑", "↗", "→", "↘", "↓", "↙"}

// debounce is the PIR state machine: a rising edge asserts motion and cancels
// any pending clear; a falling edge arms a 5s deadline after which motion
// drops.
type debounce struct {
	sensor     bool
	motion     bool
	deadline   time.Time // zero when no clear is pending
	lastActive time.Time // last rising edge; zero until the first one
}

func (d *debounce) edge(rising bool, now time.Time) {
	d.sensor = rising
	if rising {
		d.motion = true
		d.deadline = time.Time{}
		d.lastActive = now
	} else {
		d.deadline = now.Add(motionLossTimeout)
	}
}

func (d *debounce) expire(now time.Time) {
	if d.deadline.IsZero() || now.Before(d.deadline) {
		return
	}
	d.motion = false
	d.deadline = time.Time{}
}

// motionSensor wires debounce to the PIR on GPIO5. The loss deadline is
// evaluated on reads, so no timer is needed.
type motionSensor struct {
	mu        sync.Mutex
	pin       gpio.PinIO
	logger    func(level, msg string)
	d         debounce
	animFrame int
	stop      chan struct{}
}

func newMotionSensor(pin gpio.PinIO, logger func(level, msg string)) *motionSensor {
	return &motionSensor{pin: pin, logger: logger, stop: make(chan struct{})}
}

func (m *motionSensor) run() {
	for {
		ok := m.pin.WaitForEdge(-1)
		select {
		case <-m.stop:
			return
		default:
		}
		if !ok {
			continue
		}
		rising := m.pin.Read() == gpio.High
		m.mu.Lock()
		m.d.edge(rising, time.Now())
		m.mu.Unlock()
	}
}

func (m *motionSensor) State() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.d.expire(time.Now())
	return m.d.motion
}

func (m *motionSensor) glyph() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.d.expire(time.Now())
	if !m.d.motion {
		return "⏾"
	}
	m.animFrame = (m.animFrame + 1) % len(motionAnimChars)
	return motionAnimChars[m.animFrame]
}

func (m *motionSensor) lastMotion() (time.Time, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.d.lastActive.IsZero() {
		return time.Time{}, false
	}
	return m.d.lastActive, true
}

func (m *motionSensor) close() {
	close(m.stop)
	// unblocks WaitForEdge so run sees stop
	_ = m.pin.Halt()
}
