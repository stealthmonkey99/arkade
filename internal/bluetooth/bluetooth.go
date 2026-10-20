// SPDX-License-Identifier: GPL-2.0-or-later

// Package bluetooth pairs wireless controllers through BlueZ's D-Bus API.
// A paired pad appears as a hidraw node and needs no other engine support;
// the package exists because BlueZ refuses a pairing unless an agent object
// is exported on the bus, which no shell-out can do.
package bluetooth

import "context"

// HIDServiceUUID is the Human Interface Device service, advertised by
// gamepads and not by most other Bluetooth devices.
const HIDServiceUUID = "00001124-0000-1000-8000-00805f9b34fb"

// Device is one discovered or remembered Bluetooth device.
type Device struct {
	// Address is the MAC, which hidapi reports as the pad's serial and which
	// survives a reconnect.
	Address string
	Name    string
	Paired  bool
	Trusted bool
	// Connected is the Bluetooth link. The engine can read the pad only if
	// the kernel also built a HID device on top, which silently fails when
	// the uhid module is missing.
	Connected bool
	// Input means the device advertises HIDServiceUUID.
	Input bool
	// RSSI is signal strength in dBm, 0 when remembered but not advertising.
	RSSI int16
}

// Pairer is the Bluetooth capability the settings app uses. Methods without
// a context give up after five seconds.
type Pairer interface {
	// Available reports whether pairing is possible, false where BlueZ is
	// absent.
	Available() bool
	// StartScan powers the adapter on, registers the pairing agent and
	// begins discovery. Safe to call when already scanning.
	StartScan(ctx context.Context) error
	// StopScan ends discovery; pairing and connecting still work.
	StopScan()
	// Devices is a snapshot in BlueZ's order; callers rank it with Ranker.
	// A failed bus call returns an error, not an empty slice.
	Devices() ([]Device, error)
	// Pair pairs, trusts and connects one device, in that order. Trust lets
	// the pad reconnect on its own button.
	Pair(ctx context.Context, address string) error
	// Forget removes the pairing entirely.
	Forget(ctx context.Context, address string) error
	// DisconnectAll drops every connected device but keeps the pairings.
	DisconnectAll(ctx context.Context) (int, error)
	// Close releases the bus connection and unregisters the agent.
	Close() error
}
