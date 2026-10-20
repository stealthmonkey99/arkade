// SPDX-License-Identifier: GPL-2.0-or-later

package bluetooth

import (
	"sync"

	btlib "github.com/stealthmonkey99/arkade/internal/bluetooth"
)

// bus is the app's one connection to BlueZ. Calls run off the frame loop and
// screens read snapshots: a D-Bus call blocks ~25s while dbus tries to activate
// a missing org.bluez.
//
// Methods tolerate a nil receiver so screen logic tests need no engine.
type bus struct {
	// btlib.New in production; tests inject their own
	newPairer func() btlib.Pairer
	log       func(string)

	mu      sync.Mutex
	state   busState
	pairer  btlib.Pairer
	devices []btlib.Device
	lastErr error
	opening bool
	polling bool
	closed  bool
}

// busState separates an empty device list from a cabinet with no radio.
type busState int

const (
	busOpening busState = iota
	busReady
	busMissing
)

// open connects once and is safe to call every frame. A missing radio is not
// retried for the life of the app.
func (b *bus) open() {
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.closed || b.opening || b.pairer != nil || b.state == busMissing {
		b.mu.Unlock()
		return
	}
	b.opening = true
	connect := b.newPairer
	b.mu.Unlock()

	if connect == nil {
		connect = btlib.New
	}
	go func() {
		p := connect()
		b.mu.Lock()
		defer b.mu.Unlock()
		b.opening = false
		switch {
		case b.closed:
			go p.Close() // the app went away while we were connecting
		case !p.Available():
			b.state = busMissing
			go p.Close()
		default:
			b.pairer, b.state = p, busReady
		}
	}()
}

// poll refreshes the device list with at most one call in flight; a request
// during one is dropped.
func (b *bus) poll() {
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.closed || b.polling || b.pairer == nil {
		b.mu.Unlock()
		return
	}
	b.polling = true
	p := b.pairer
	b.mu.Unlock()

	go func() {
		devs, err := p.Devices()
		b.mu.Lock()
		defer b.mu.Unlock()
		b.polling = false
		if b.closed {
			return
		}
		if err != nil {
			// The last good list stays on screen.
			b.lastErr = err
			b.logf("bluetooth: listing devices: " + err.Error())
			return
		}
		b.devices, b.lastErr = devs, nil
	}()
}

// snapshot is what a screen paints from. The slice is a copy because callers
// sort it in place.
func (b *bus) snapshot() (busState, []btlib.Device) {
	if b == nil {
		return busOpening, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state, append([]btlib.Device(nil), b.devices...)
}

// takeErr returns the last poll's failure and clears it, so a stale failure
// does not abort the next visit to the scan screen.
func (b *bus) takeErr() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	err := b.lastErr
	b.lastErr = nil
	return err
}

// live is nil until the bus is ready.
func (b *bus) live() btlib.Pairer {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.pairer
}

// close releases the connection and its pairing agent; a Pairer left open
// makes the next registration at the same object path fail with AlreadyExists.
func (b *bus) close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	p := b.pairer
	b.pairer = nil
	b.mu.Unlock()

	if p == nil {
		return
	}
	go func() {
		if err := p.Close(); err != nil {
			b.logf("bluetooth: closing: " + err.Error())
		}
	}()
}

func (b *bus) logf(msg string) {
	if b.log != nil {
		b.log(msg)
	}
}
