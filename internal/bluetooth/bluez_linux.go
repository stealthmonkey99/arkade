// SPDX-License-Identifier: GPL-2.0-or-later

//go:build linux

package bluetooth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	bluezService   = "org.bluez"
	adapterIface   = "org.bluez.Adapter1"
	deviceIface    = "org.bluez.Device1"
	agentMgrIface  = "org.bluez.AgentManager1"
	agentIface     = "org.bluez.Agent1"
	propsIface     = "org.freedesktop.DBus.Properties"
	objMgrIface    = "org.freedesktop.DBus.ObjectManager"
	agentPath      = dbus.ObjectPath("/org/arkade/btagent")
	agentMgrPath   = dbus.ObjectPath("/org/bluez")
	pairTimeout    = 30 * time.Second
	connectTimeout = 20 * time.Second
	// A call to a stopped bluetoothd blocks for dbus's 25s service start
	// timeout; callTimeout keeps ordinary calls well under that.
	callTimeout = 5 * time.Second
)

// New returns the BlueZ-backed pairer, or an unsupported stub when there is
// no system bus or no adapter. Never returns nil.
func New() Pairer {
	// Private, not the shared bus: closing the shared connection breaks
	// every other holder, including the power-down hook.
	conn, err := dbus.SystemBusPrivate()
	if err != nil {
		return unsupported{reason: "no system bus"}
	}
	if err := conn.Auth(nil); err != nil {
		_ = conn.Close()
		return unsupported{reason: "bus auth: " + err.Error()}
	}
	if err := conn.Hello(); err != nil {
		_ = conn.Close()
		return unsupported{reason: "bus hello: " + err.Error()}
	}
	adapter, err := findAdapter(conn)
	if err != nil {
		_ = conn.Close()
		return unsupported{reason: err.Error()}
	}
	return &bluez{conn: conn, adapter: adapter}
}

var errClosed = errors.New("bluetooth: pairer is closed")

type bluez struct {
	adapter dbus.ObjectPath

	mu       sync.Mutex
	conn     *dbus.Conn // nil once closed
	agentOn  bool
	scanning bool
}

func (b *bluez) Available() bool { return true }

// bus snapshots the connection for methods that do not hold mu across
// their calls; a Close racing them fails their calls instead of nil-derefing.
func (b *bluez) bus() (*dbus.Conn, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn == nil {
		return nil, errClosed
	}
	return b.conn, nil
}

func findAdapter(conn *dbus.Conn) (dbus.ObjectPath, error) {
	objs, err := managedObjects(conn)
	if err != nil {
		return "", err
	}
	for path, ifaces := range objs {
		if _, ok := ifaces[adapterIface]; ok {
			return path, nil
		}
	}
	return "", errors.New("no Bluetooth adapter")
}

// callBounded runs a D-Bus call under callTimeout.
func callBounded(obj dbus.BusObject, method string, args ...any) *dbus.Call {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	return obj.CallWithContext(ctx, method, 0, args...)
}

func managedObjects(conn *dbus.Conn) (map[dbus.ObjectPath]map[string]map[string]dbus.Variant, error) {
	var objs map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	obj := conn.Object(bluezService, dbus.ObjectPath("/"))
	if err := callBounded(obj, objMgrIface+".GetManagedObjects").Store(&objs); err != nil {
		return nil, fmt.Errorf("bluetooth: listing objects: %w", err)
	}
	return objs, nil
}

func setProp(conn *dbus.Conn, path dbus.ObjectPath, iface, name string, v any) error {
	obj := conn.Object(bluezService, path)
	return callBounded(obj, propsIface+".Set", iface, name, dbus.MakeVariant(v)).Err
}

func (b *bluez) StartScan(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn == nil {
		return errClosed
	}
	if err := setProp(b.conn, b.adapter, adapterIface, "Powered", true); err != nil {
		return fmt.Errorf("bluetooth: powering the adapter: %w", err)
	}
	if !b.agentOn {
		if err := b.registerAgent(); err != nil {
			return err
		}
		b.agentOn = true
	}
	if b.scanning {
		return nil
	}
	obj := b.conn.Object(bluezService, b.adapter)
	if err := obj.CallWithContext(ctx, adapterIface+".StartDiscovery", 0).Err; err != nil {
		// Another client may already be discovering.
		if !strings.Contains(err.Error(), "InProgress") {
			return fmt.Errorf("bluetooth: starting discovery: %w", err)
		}
	}
	b.scanning = true
	return nil
}

func (b *bluez) StopScan() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopScanLocked()
}

func (b *bluez) stopScanLocked() {
	if !b.scanning || b.conn == nil {
		return
	}
	b.scanning = false
	obj := b.conn.Object(bluezService, b.adapter)
	_ = callBounded(obj, adapterIface+".StopDiscovery").Err
}

func (b *bluez) Devices() ([]Device, error) {
	conn, err := b.bus()
	if err != nil {
		return nil, err
	}
	return b.devices(conn)
}

func (b *bluez) devices(conn *dbus.Conn) ([]Device, error) {
	objs, err := managedObjects(conn)
	if err != nil {
		return nil, err
	}
	var out []Device
	for path, ifaces := range objs {
		props, ok := ifaces[deviceIface]
		if !ok {
			continue
		}
		if !strings.HasPrefix(string(path), string(b.adapter)) {
			continue // a device on some other adapter
		}
		out = append(out, deviceFromProps(props))
	}
	return out, nil
}

func deviceFromProps(props map[string]dbus.Variant) Device {
	var d Device
	if v, ok := props["Address"]; ok {
		_ = v.Store(&d.Address)
	}
	if v, ok := props["Name"]; ok {
		_ = v.Store(&d.Name)
	}
	if v, ok := props["Alias"]; ok && d.Name == "" {
		_ = v.Store(&d.Name)
	}
	if v, ok := props["Paired"]; ok {
		_ = v.Store(&d.Paired)
	}
	if v, ok := props["Trusted"]; ok {
		_ = v.Store(&d.Trusted)
	}
	if v, ok := props["Connected"]; ok {
		_ = v.Store(&d.Connected)
	}
	if v, ok := props["RSSI"]; ok {
		_ = v.Store(&d.RSSI)
	}
	if v, ok := props["UUIDs"]; ok {
		var uuids []string
		if v.Store(&uuids) == nil {
			for _, u := range uuids {
				if strings.EqualFold(u, HIDServiceUUID) {
					d.Input = true
					break
				}
			}
		}
	}
	return d
}

// devicePath looks up the object path for a MAC, failing early if absent.
func devicePath(conn *dbus.Conn, address string) (dbus.ObjectPath, error) {
	objs, err := managedObjects(conn)
	if err != nil {
		return "", err
	}
	for path, ifaces := range objs {
		props, ok := ifaces[deviceIface]
		if !ok {
			continue
		}
		var addr string
		if v, ok := props["Address"]; ok {
			_ = v.Store(&addr)
		}
		if strings.EqualFold(addr, address) {
			return path, nil
		}
	}
	return "", fmt.Errorf("bluetooth: %s is not in range", address)
}

func (b *bluez) Pair(ctx context.Context, address string) error {
	conn, err := b.bus()
	if err != nil {
		return err
	}
	path, err := devicePath(conn, address)
	if err != nil {
		return err
	}
	obj := conn.Object(bluezService, path)

	pairCtx, cancel := context.WithTimeout(ctx, pairTimeout)
	defer cancel()
	if err := obj.CallWithContext(pairCtx, deviceIface+".Pair", 0).Err; err != nil {
		// An existing bond is the state Pair is after.
		if !strings.Contains(err.Error(), "AlreadyExists") {
			return fmt.Errorf("bluetooth: pairing %s: %w", address, err)
		}
	}
	if err := setProp(conn, path, deviceIface, "Trusted", true); err != nil {
		return fmt.Errorf("bluetooth: trusting %s: %w", address, err)
	}
	connCtx, cancelConn := context.WithTimeout(ctx, connectTimeout)
	defer cancelConn()
	if err := obj.CallWithContext(connCtx, deviceIface+".Connect", 0).Err; err != nil {
		return fmt.Errorf("bluetooth: connecting %s: %w", address, err)
	}
	return nil
}

func (b *bluez) Forget(ctx context.Context, address string) error {
	conn, err := b.bus()
	if err != nil {
		return err
	}
	path, err := devicePath(conn, address)
	if err != nil {
		return err
	}
	obj := conn.Object(bluezService, b.adapter)
	if err := obj.CallWithContext(ctx, adapterIface+".RemoveDevice", 0, path).Err; err != nil {
		return fmt.Errorf("bluetooth: forgetting %s: %w", address, err)
	}
	return nil
}

func (b *bluez) DisconnectAll(ctx context.Context) (int, error) {
	conn, err := b.bus()
	if err != nil {
		return 0, err
	}
	devs, err := b.devices(conn)
	if err != nil {
		return 0, err
	}
	var n int
	var firstErr error
	for _, d := range devs {
		if !d.Connected {
			continue
		}
		path, err := devicePath(conn, d.Address)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		obj := conn.Object(bluezService, path)
		if err := obj.CallWithContext(ctx, deviceIface+".Disconnect", 0).Err; err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("bluetooth: disconnecting %s: %w", d.Address, err)
			}
			continue
		}
		n++
	}
	return n, firstErr
}

func (b *bluez) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn == nil {
		return nil
	}
	b.stopScanLocked()
	if b.agentOn {
		obj := b.conn.Object(bluezService, agentMgrPath)
		_ = callBounded(obj, agentMgrIface+".UnregisterAgent", agentPath).Err
		b.agentOn = false
	}
	// An unclosed connection keeps the agent path registered, and the next
	// connection gets AlreadyExists.
	_ = b.conn.Close()
	b.conn = nil
	return nil
}
