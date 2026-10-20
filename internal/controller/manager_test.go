// SPDX-License-Identifier: GPL-2.0-or-later

package controller

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/input"
)

type fakeDevice struct {
	reports    chan []byte
	once       sync.Once
	closeCalls atomic.Int32
}

func newFakeDevice() *fakeDevice {
	return &fakeDevice{reports: make(chan []byte, 16)}
}

func (f *fakeDevice) Read() ([]byte, error) {
	data, ok := <-f.reports
	if !ok {
		return nil, errors.New("device gone")
	}
	return data, nil
}

func (f *fakeDevice) Close() error {
	f.closeCalls.Add(1)
	f.once.Do(func() { close(f.reports) })
	return nil
}

type fakeEnum struct {
	mu      sync.Mutex
	present []DeviceInfo
	devices map[string]*fakeDevice
}

func newFakeEnum() *fakeEnum {
	return &fakeEnum{devices: map[string]*fakeDevice{}}
}

func (e *fakeEnum) plug(info DeviceInfo) *fakeDevice {
	e.mu.Lock()
	defer e.mu.Unlock()
	dev := newFakeDevice()
	e.present = append(e.present, info)
	e.devices[info.Path] = dev
	return dev
}

func (e *fakeEnum) Enumerate() []DeviceInfo {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]DeviceInfo(nil), e.present...)
}

func (e *fakeEnum) Open(path string) (Device, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if dev, ok := e.devices[path]; ok {
		return dev, nil
	}
	return nil, errors.New("no such device")
}

func drainEventually(t *testing.T, h *input.Handler) []input.Event {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if events := h.Drain(); len(events) > 0 {
			return events
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no events arrived from controller read loop")
	return nil
}

func TestManagerRoutesDecodedEvents(t *testing.T) {
	handler := input.NewHandler()
	enum := newFakeEnum()
	dev := enum.plug(DeviceInfo{VID: Speedlink.VID, PID: Speedlink.PID, Path: "usb-1"})

	m := NewManager(handler, enum, nil)
	m.Start()
	defer m.Stop()

	dev.reports <- speedlinkReport(0, 128, 128)      // prime
	dev.reports <- speedlinkReport(0b0001, 128, 128) // JOY_A down

	events := drainEventually(t, handler)
	if len(events) != 2 || events[0].Kind != input.KindKeydown || events[1].Name != input.KeyJoyA {
		t.Fatalf("unexpected events: %+v", events)
	}
	src := events[0].Source
	if src.Type != input.SourceController || src.Slot != input.SlotOf(0) || src.Seat != input.SeatP1 {
		t.Errorf("first controller should be slot 0 at seat P1, got %+v", src)
	}
}

func TestManagerAssignsSlotsInAttachOrder(t *testing.T) {
	handler := input.NewHandler()
	enum := newFakeEnum()
	dev1 := enum.plug(DeviceInfo{VID: Speedlink.VID, PID: Speedlink.PID, Path: "usb-1"})
	dev2 := enum.plug(DeviceInfo{VID: Buffalo.VID, PID: Buffalo.PID, Path: "usb-2"})

	m := NewManager(handler, enum, nil)
	m.Start()
	defer m.Stop()

	if got := len(m.Controllers()); got != 2 {
		t.Fatalf("expected 2 controllers attached, got %d", got)
	}

	dev2.reports <- []byte{128, 128, 0, 0}
	dev2.reports <- []byte{128, 128, 0, 0b100}
	events := drainEventually(t, handler)
	if events[0].Source.Slot != input.SlotOf(1) {
		t.Errorf("second controller should be slot 1, got %v", events[0].Source.Slot)
	}
	_ = dev1
}

func TestManagerIgnoresUnknownDevices(t *testing.T) {
	handler := input.NewHandler()
	enum := newFakeEnum()
	enum.plug(DeviceInfo{VID: 0xdead, PID: 0xbeef, Path: "usb-x"})

	m := NewManager(handler, enum, nil)
	m.Start()
	defer m.Stop()

	if got := len(m.Controllers()); got != 0 {
		t.Errorf("unknown devices must not attach, got %d", got)
	}
}

func TestManagerDetachOnReadError(t *testing.T) {
	handler := input.NewHandler()
	enum := newFakeEnum()
	dev := enum.plug(DeviceInfo{VID: Speedlink.VID, PID: Speedlink.PID, Path: "usb-1"})

	m := NewManager(handler, enum, nil)
	m.Start()
	defer m.Stop()

	dev.Close() // read loop errors → detach
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(m.Controllers()) == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("device should detach after read error")
}

func TestStopClosesDevicesExactlyOnce(t *testing.T) {
	handler := input.NewHandler()
	enum := newFakeEnum()
	dev1 := enum.plug(DeviceInfo{VID: Speedlink.VID, PID: Speedlink.PID, Path: "usb-1"})
	dev2 := enum.plug(DeviceInfo{VID: Buffalo.VID, PID: Buffalo.PID, Path: "usb-2"})

	m := NewManager(handler, enum, nil)
	m.Start()
	if got := len(m.Controllers()); got != 2 {
		t.Fatalf("expected 2 controllers attached, got %d", got)
	}

	m.Stop()
	if got := dev1.closeCalls.Load(); got != 1 {
		t.Errorf("dev1 closed %d times, want exactly 1", got)
	}
	if got := dev2.closeCalls.Load(); got != 1 {
		t.Errorf("dev2 closed %d times, want exactly 1", got)
	}
	m.Stop() // idempotent: a second Stop must not re-close or panic
	if got := dev1.closeCalls.Load(); got != 1 {
		t.Errorf("second Stop re-closed dev1 (%d times total)", got)
	}
}

func TestManagerHotplugPicksUpNewDevices(t *testing.T) {
	handler := input.NewHandler()
	enum := newFakeEnum()

	m := NewManager(handler, enum, nil)
	m.Start()
	defer m.Stop()

	if len(m.Controllers()) != 0 {
		t.Fatal("no devices expected at start")
	}
	enum.plug(DeviceInfo{VID: DualShock.VID, PID: DualShock.PID, Path: "usb-9"})
	m.scan()
	if got := len(m.Controllers()); got != 1 {
		t.Fatalf("hot-plugged device should attach, got %d", got)
	}
}

func TestManagerPairSeatSwapsSeats(t *testing.T) {
	handler := input.NewHandler()
	enum := newFakeEnum()
	dev1 := enum.plug(DeviceInfo{VID: Speedlink.VID, PID: Speedlink.PID, Path: "usb-1"})
	dev2 := enum.plug(DeviceInfo{VID: Buffalo.VID, PID: Buffalo.PID, Path: "usb-2"})

	m := NewManager(handler, enum, nil)
	m.Start()
	defer m.Stop()

	m.PairSeat("usb-2", input.SeatP1)
	list := m.List()
	if list[0].Seat != input.SeatP2 || list[1].Seat != input.SeatP1 {
		t.Fatalf("expected swapped seats, got %+v", list)
	}

	dev2.reports <- []byte{128, 128, 0, 0}
	dev2.reports <- []byte{128, 128, 0b1, 0}
	events := drainEventually(t, handler)
	if events[0].Source.Seat != input.SeatP1 || events[0].Source.ID != "usb-2" {
		t.Errorf("paired controller should report seat P1 with its ID, got %+v", events[0].Source)
	}
	_ = dev1
}

func TestManagerPairUnknownIDIsNoop(t *testing.T) {
	handler := input.NewHandler()
	enum := newFakeEnum()
	enum.plug(DeviceInfo{VID: Speedlink.VID, PID: Speedlink.PID, Path: "usb-1"})

	m := NewManager(handler, enum, nil)
	m.Start()
	defer m.Stop()

	m.PairSeat("virtual-0", input.SeatP1) // keyboard IDs are not HID devices
	if list := m.List(); list[0].Seat != input.SeatP1 {
		t.Errorf("unknown ID pairing must not disturb slots, got %+v", list)
	}
}

func TestProfileForPrefersTheMatchingBus(t *testing.T) {
	m := NewManager(input.NewHandler(), newFakeEnum(), nil)

	const vid, pid = 0x054c, 0x09cc
	usb := DeviceInfo{VID: vid, PID: pid, Path: "/usb", Bus: "usb"}
	bt := DeviceInfo{VID: vid, PID: pid, Path: "/bt", Bus: "bluetooth"}

	got, ok := m.profileFor(bt)
	if !ok {
		t.Fatal("no profile matched the Bluetooth pad")
	}
	if got.Name != DualShockBT.Name {
		t.Errorf("Bluetooth pad matched %q, want %q", got.Name, DualShockBT.Name)
	}
	if DualShockBT.Spec.Channels[0].field.byteAt != DualShock.Spec.Channels[0].field.byteAt+2 {
		t.Errorf("Bluetooth offset %d, want the USB offset %d plus the two header bytes",
			DualShockBT.Spec.Channels[0].field.byteAt, DualShock.Spec.Channels[0].field.byteAt)
	}

	got, ok = m.profileFor(usb)
	if !ok {
		t.Fatal("no profile matched the USB pad")
	}
	if got.Name != DualShock.Name {
		t.Errorf("USB pad matched %q, want %q", got.Name, DualShock.Name)
	}

	got, ok = m.profileFor(DeviceInfo{VID: vid, PID: pid, Path: "/unknown"})
	if !ok || got.Name != DualShock.Name {
		t.Errorf("bus-less device matched %q (ok=%v), want the bus-less profile %q", got.Name, ok, DualShock.Name)
	}

	dr := DeviceInfo{VID: DragonRise.VID, PID: DragonRise.PID, Path: "/dr", Bus: "usb"}
	if got, ok := m.profileFor(dr); !ok || got.Name != DragonRise.Name {
		t.Errorf("DragonRise over USB matched %q (ok=%v), want %q", got.Name, ok, DragonRise.Name)
	}
}

func TestStableIDPrefersSerial(t *testing.T) {
	wired := DeviceInfo{Path: "/dev/hidraw1"}
	if got := wired.StableID(); got != "/dev/hidraw1" {
		t.Errorf("wired StableID = %q, want the path", got)
	}
	wireless := DeviceInfo{Path: "/dev/hidraw3", Serial: "02:1a:2b:3c:4d:03"}
	if got := wireless.StableID(); got != "02:1a:2b:3c:4d:03" {
		t.Errorf("wireless StableID = %q, want the serial", got)
	}
}
