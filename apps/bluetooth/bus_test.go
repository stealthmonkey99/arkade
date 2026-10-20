// SPDX-License-Identifier: GPL-2.0-or-later

package bluetooth

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	btlib "github.com/stealthmonkey99/arkade/internal/bluetooth"
)

func TestNoScreenBuildsItsOwnConnectionToBlueZ(t *testing.T) {
	for _, name := range []string{"devices.go", "scan.go", "bluetooth.go"} {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		for _, call := range []string{"btlib.New(", "bluetooth.New("} {
			if strings.Contains(string(src), call) {
				t.Errorf("%s calls %s directly; connecting belongs to the bus, off the frame loop", name, call)
			}
		}
	}
}

// fakePairer is a btlib.Pairer with controllable delay, gate, devices and
// error, and call counters.
type fakePairer struct {
	mu           sync.Mutex
	available    bool
	devices      []btlib.Device
	err          error
	delay        time.Duration
	gate         chan struct{}
	devicesCalls int
	closeCalls   int
}

func (f *fakePairer) Available() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.available
}

func (f *fakePairer) setDevices(d []btlib.Device) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.devices = d
}

func (f *fakePairer) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakePairer) setDelay(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delay = d
}

func (f *fakePairer) setGate(gate chan struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gate = gate
}

func (f *fakePairer) Devices() ([]btlib.Device, error) {
	f.mu.Lock()
	f.devicesCalls++
	delay, gate, devs, err := f.delay, f.gate, f.devices, f.err
	f.mu.Unlock()

	if gate != nil {
		<-gate
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	return devs, err
}

func (f *fakePairer) devicesCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.devicesCalls
}

func (f *fakePairer) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeCalls++
	return nil
}

func (f *fakePairer) closeCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closeCalls
}

func (f *fakePairer) StartScan(context.Context) error            { return nil }
func (f *fakePairer) StopScan()                                  {}
func (f *fakePairer) Pair(context.Context, string) error         { return nil }
func (f *fakePairer) Forget(context.Context, string) error       { return nil }
func (f *fakePairer) DisconnectAll(context.Context) (int, error) { return 0, nil }

func slowConstructor(delay time.Duration, p btlib.Pairer) func() btlib.Pairer {
	return func() btlib.Pairer {
		time.Sleep(delay)
		return p
	}
}

func countingConstructor(p btlib.Pairer) (ctor func() btlib.Pairer, count func() int) {
	var mu sync.Mutex
	calls := 0
	ctor = func() btlib.Pairer {
		mu.Lock()
		calls++
		mu.Unlock()
		return p
	}
	count = func() int {
		mu.Lock()
		defer mu.Unlock()
		return calls
	}
	return ctor, count
}

func waitUntil(t *testing.T, cond func() bool, timeoutMsg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal(timeoutMsg)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func busIsIdle(b *bus) func() bool {
	return func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		return !b.opening && !b.polling
	}
}

func TestOpenReturnsBeforeThePairerFinishesConnecting(t *testing.T) {
	fake := &fakePairer{available: true}
	b := &bus{newPairer: slowConstructor(300*time.Millisecond, fake)}

	start := time.Now()
	b.open()
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("open() took %v to return; a screen calling it every frame would stall", elapsed)
	}

	if state, _ := b.snapshot(); state != busOpening {
		t.Fatalf("snapshot() reported %v right after open(), want busOpening", state)
	}

	waitUntil(t, func() bool {
		state, _ := b.snapshot()
		return state == busReady
	}, "the bus never reached busReady once the slow connect finished")
}

func TestPollReturnsBeforeDevicesFinishes(t *testing.T) {
	fake := &fakePairer{available: true}
	fake.setDelay(300 * time.Millisecond)
	fake.setDevices([]btlib.Device{{Address: "AA:AA", Name: "Pad"}})
	b := &bus{pairer: fake, state: busReady}

	start := time.Now()
	b.poll()
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("poll() took %v to return; the frame loop would stall on a slow radio", elapsed)
	}

	waitUntil(t, busIsIdle(b), "poll never finished in the background")
	if _, devs := b.snapshot(); len(devs) != 1 {
		t.Fatalf("snapshot has %d devices after the poll finished, want 1", len(devs))
	}
}

func TestOnlyOnePollIsEverInFlight(t *testing.T) {
	fake := &fakePairer{available: true}
	fake.setDevices([]btlib.Device{{Address: "AA:AA", Name: "Pad"}})
	gate := make(chan struct{})
	fake.setGate(gate)
	b := &bus{pairer: fake, state: busReady}

	b.poll()
	b.poll()
	b.poll()
	b.poll()

	close(gate)
	waitUntil(t, busIsIdle(b), "the in-flight poll never finished after the gate was released")

	if n := fake.devicesCallCount(); n != 1 {
		t.Fatalf("Devices() was called %d times for four overlapping polls, want 1", n)
	}
}

func TestAnUnavailablePairerEndsMissingAndIsClosed(t *testing.T) {
	fake := &fakePairer{available: false}
	b := &bus{newPairer: func() btlib.Pairer { return fake }}

	b.open()

	waitUntil(t, func() bool {
		state, _ := b.snapshot()
		return state == busMissing
	}, "an unavailable pairer never reached busMissing")

	if state, _ := b.snapshot(); state == busReady {
		t.Fatal("a cabinet with no radio reported busReady")
	}
	waitUntil(t, func() bool { return fake.closeCallCount() == 1 },
		"an unavailable pairer was never Closed, which leaks the agent's object path")
}

func TestSnapshotHandsBackACopyNotTheLiveSlice(t *testing.T) {
	original := []btlib.Device{{Address: "AA:AA"}, {Address: "BB:BB"}}
	b := &bus{state: busReady, devices: original}

	_, snap := b.snapshot()
	sort.Slice(snap, func(i, j int) bool { return snap[i].Address > snap[j].Address })

	_, snap2 := b.snapshot()
	if snap2[0].Address != "AA:AA" || snap2[1].Address != "BB:BB" {
		t.Fatalf("the bus's own device order changed to %v after the caller sorted its copy", snap2)
	}
}

func TestAFailedPollKeepsTheLastGoodListOnScreen(t *testing.T) {
	fake := &fakePairer{available: true}
	good := []btlib.Device{{Address: "AA:AA", Name: "Pad"}}
	fake.setDevices(good)
	b := &bus{pairer: fake, state: busReady}

	b.poll()
	waitUntil(t, busIsIdle(b), "the first poll never finished")
	if _, devs := b.snapshot(); len(devs) != 1 {
		t.Fatalf("snapshot has %d devices after a good poll, want 1", len(devs))
	}

	fake.setDevices(nil)
	fake.setErr(errors.New("dbus: boom"))
	b.poll()
	waitUntil(t, busIsIdle(b), "the failing poll never finished")

	if _, devs := b.snapshot(); len(devs) != 1 || devs[0].Address != "AA:AA" {
		t.Fatalf("a failed poll left %v on screen, want the last good list preserved", devs)
	}
}

func TestCloseIsIdempotentAndStopsAnyLaterPoll(t *testing.T) {
	never := &bus{}
	never.close()
	never.close()

	fake := &fakePairer{available: true}
	fake.setDevices([]btlib.Device{{Address: "AA:AA"}})
	b := &bus{pairer: fake, state: busReady}

	b.close()
	b.close()

	b.poll()
	if n := fake.devicesCallCount(); n != 0 {
		t.Fatalf("poll() called Devices() %d times after close(), want 0", n)
	}
}

func TestOpenDoesNotRetryOnceTheBusIsMissing(t *testing.T) {
	fake := &fakePairer{available: false}
	ctor, callCount := countingConstructor(fake)
	b := &bus{newPairer: ctor}

	b.open()
	waitUntil(t, func() bool {
		state, _ := b.snapshot()
		return state == busMissing
	}, "the bus never settled on busMissing")

	b.open()
	b.open()
	b.open()

	if n := callCount(); n != 1 {
		t.Fatalf("newPairer ran %d times after busMissing, want exactly 1", n)
	}
}
