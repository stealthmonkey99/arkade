// SPDX-License-Identifier: GPL-2.0-or-later

// Package hiddev implements controller.Enumerator over HID devices via
// sstallion/go-hid (cgo hidapi). It is the only package importing the native
// library, which keeps cgo out of the controller core.
package hiddev

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sstallion/go-hid"
	"github.com/stealthmonkey99/arkade/assetfile"

	"github.com/stealthmonkey99/arkade/internal/controller"
)

// readTimeout bounds blocking reads so Close cannot wedge on a quiet device.
const readTimeout = 500 * time.Millisecond

type Enumerator struct{}

func New() (*Enumerator, error) {
	if err := hid.Init(); err != nil {
		return nil, fmt.Errorf("hidapi init: %w", err)
	}
	return &Enumerator{}, nil
}

func (e *Enumerator) Close() error { return hid.Exit() }

func (e *Enumerator) Enumerate() []controller.DeviceInfo {
	var out []controller.DeviceInfo
	_ = hid.Enumerate(hid.VendorIDAny, hid.ProductIDAny, func(info *hid.DeviceInfo) error {
		out = append(out, controller.DeviceInfo{
			VID:    info.VendorID,
			PID:    info.ProductID,
			Path:   info.Path,
			Bus:    busName(info.BusType),
			Serial: info.SerialNbr,
		})
		return nil
	})
	return out
}

// busName maps hidapi's bus type onto the profile vocabulary; other buses
// report "", which matches only profiles naming no bus.
func busName(b hid.BusType) string {
	switch b {
	case hid.BusUSB:
		return assetfile.BusUSB
	case hid.BusBluetooth:
		return assetfile.BusBluetooth
	default:
		return ""
	}
}

func (e *Enumerator) Open(path string) (controller.Device, error) {
	dev, err := hid.OpenPath(path)
	if err != nil {
		return nil, err
	}
	return &device{dev: dev}, nil
}

type device struct {
	dev *hid.Device
	// readMu keeps hid_close from freeing the handle during a cgo read;
	// closed stops the loop at the next slice.
	readMu    sync.Mutex
	closed    atomic.Bool
	closeOnce sync.Once
	closeErr  error
}

var errClosed = errors.New("device closed")

// Read blocks until an input report arrives. A timeout is not a disconnect,
// since idle controllers report only on state change; any other device error
// ends the read.
func (d *device) Read() ([]byte, error) {
	buf := make([]byte, 64)
	for {
		d.readMu.Lock()
		if d.closed.Load() {
			d.readMu.Unlock()
			return nil, errClosed
		}
		n, err := d.dev.ReadWithTimeout(buf, readTimeout)
		d.readMu.Unlock()
		if err != nil {
			if errors.Is(err, hid.ErrTimeout) || transientRead(err) {
				continue
			}
			return nil, err
		}
		if n > 0 {
			return buf[:n], nil
		}
	}
}

// transientRead reports whether a read error is a retryable EINTR (Go's
// preemption signal) or EAGAIN, which hidapi surfaces as strerror text.
func transientRead(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "interrupted") ||
		strings.Contains(msg, "temporarily unavailable") ||
		strings.Contains(msg, "try again")
}

// Close is safe from any goroutine and idempotent: hid_close frees the
// handle, so a second call is a double free in glibc. It may block for one
// readTimeout while a read slice finishes.
func (d *device) Close() error {
	d.closeOnce.Do(func() {
		d.closed.Store(true)
		d.readMu.Lock()
		d.closeErr = d.dev.Close()
		d.readMu.Unlock()
	})
	return d.closeErr
}

var _ controller.Enumerator = (*Enumerator)(nil)
