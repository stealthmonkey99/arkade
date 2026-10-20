// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"reflect"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

func testLampWriter(t *testing.T) (*lampWriter, *fakeBus) {
	t.Helper()
	bus := &fakeBus{}
	m, err := newMCP23017(bus, muxAddr)
	if err != nil {
		t.Fatal(err)
	}
	bus.reset()
	return newLampWriter(m, func(string, string) {}, noSleep), bus
}

func latchWrites(a, b byte) []txRecord {
	return []txRecord{{muxAddr, []byte{regOLATA, a, b}}}
}

func TestLampWriterBinaryIsOneTransaction(t *testing.T) {
	w, bus := testLampWriter(t)

	w.setLamps(0, hardware.LampLevelsOf(hardware.AllLights))
	if want := latchWrites(0x3f, 0x00); !reflect.DeepEqual(bus.forAddr(muxAddr), want) {
		t.Errorf("seat 0 full = %v, want %v", bus.forAddr(muxAddr), want)
	}

	bus.reset()
	w.setLamps(1, hardware.LampLevels{hardware.LampLevelMax, 0, 0, 0, 0, hardware.LampLevelMax})
	if want := latchWrites(0x3f, 0x21); !reflect.DeepEqual(bus.forAddr(muxAddr), want) {
		t.Errorf("seat 1 partial = %v, want %v", bus.forAddr(muxAddr), want)
	}
}

func TestLampWriterFrameBitAngles(t *testing.T) {
	w, bus := testLampWriter(t)

	w.setLamps(0, hardware.LampLevels{hardware.LampLevelFromABI(5)})
	want := []txRecord{
		{muxAddr, []byte{regOLATA, 0x01, 0x00}},
		{muxAddr, []byte{regOLATA, 0x00, 0x00}},
		{muxAddr, []byte{regOLATA, 0x01, 0x00}},
		{muxAddr, []byte{regOLATA, 0x00, 0x00}},
	}
	if got := bus.forAddr(muxAddr); !reflect.DeepEqual(got, want) {
		t.Errorf("quantized level 5 frame = %v, want %v", got, want)
	}
}

func TestLampWriterFanRidesEveryWrite(t *testing.T) {
	w, bus := testLampWriter(t)

	w.setPin(fanPin, true)
	if want := latchWrites(0x80, 0x00); !reflect.DeepEqual(bus.forAddr(muxAddr), want) {
		t.Fatalf("fan alone = %v, want %v", bus.forAddr(muxAddr), want)
	}

	bus.reset()
	w.setLamps(0, hardware.LampLevels{hardware.LampLevelFromABI(5)})
	want := []txRecord{
		{muxAddr, []byte{regOLATA, 0x81, 0x00}},
		{muxAddr, []byte{regOLATA, 0x80, 0x00}},
		{muxAddr, []byte{regOLATA, 0x81, 0x00}},
		{muxAddr, []byte{regOLATA, 0x80, 0x00}},
	}
	if got := bus.forAddr(muxAddr); !reflect.DeepEqual(got, want) {
		t.Errorf("modulating frame with the fan on = %v, want %v", got, want)
	}

	bus.reset()
	w.setPin(fanPin, false)
	for i, r := range bus.forAddr(muxAddr) {
		if r.w[1]&0x80 != 0 {
			t.Errorf("slot %d still lights the fan bit: %#x", i, r.w[1])
		}
	}
}

func TestLampWriterSlotDeadlinesAreAnchored(t *testing.T) {
	for slot, want := range []time.Duration{bamUnit, 3 * bamUnit, 7 * bamUnit, 15 * bamUnit} {
		if got := slotEnd(slot); got != want {
			t.Errorf("slotEnd(%d) = %v, want %v", slot, got, want)
		}
	}
	if slotEnd(bamSlots-1) > bamFrame {
		t.Errorf("frame runs long: %v > %v", slotEnd(bamSlots-1), bamFrame)
	}
}

func TestLampWriterStopRoundsToBinary(t *testing.T) {
	w, bus := testLampWriter(t)
	w.sleep = time.Sleep
	w.start()
	w.setLamps(0, hardware.LampLevels{hardware.LampLevelFromABI(9), hardware.LampLevelFromABI(7), hardware.LampLevelFromABI(8)})
	time.Sleep(25 * time.Millisecond)
	bus.reset()
	w.stop()

	got := bus.forAddr(muxAddr)
	if len(got) == 0 {
		t.Fatal("stop should leave a final latch write")
	}
	if last := got[len(got)-1]; !reflect.DeepEqual(last.w, []byte{regOLATA, 0x05, 0x00}) {
		t.Errorf("final latches = %v, want levels 9 and 8 lit, 7 dark", last.w)
	}
}

func TestLampWriterReturnsToIdle(t *testing.T) {
	w, bus := testLampWriter(t)
	w.sleep = time.Sleep
	w.start()
	t.Cleanup(w.stop)

	w.setLamps(0, hardware.LampLevels{hardware.LampLevelFromABI(5)})
	time.Sleep(35 * time.Millisecond)
	if n := len(bus.forAddr(muxAddr)); n < 8 {
		t.Fatalf("three frames should be ~12 writes, got %d", n)
	}

	w.setLamps(0, hardware.LampLevelsOf(hardware.AllLights))
	time.Sleep(30 * time.Millisecond) // let the frame in flight finish
	settled := bus.forAddr(muxAddr)
	if last := settled[len(settled)-1]; !reflect.DeepEqual(last.w, []byte{regOLATA, 0x3f, 0x00}) {
		t.Fatalf("idle state = %v, want all six lamps latched on", last.w)
	}
	time.Sleep(30 * time.Millisecond)
	if n := len(bus.forAddr(muxAddr)); n != len(settled) {
		t.Errorf("idle writer kept writing: %d writes, want %d", n, len(settled))
	}
}

func TestLampWriterLogsBusErrors(t *testing.T) {
	bus := &fakeBus{}
	m, err := newMCP23017(bus, muxAddr)
	if err != nil {
		t.Fatal(err)
	}
	var logged []string
	w := newLampWriter(m, func(level, msg string) { logged = append(logged, level+": "+msg) }, noSleep)
	bus.fail(errFakeBus)

	w.setLamps(0, hardware.LampLevelsOf(hardware.AllLights))
	if len(logged) != 1 {
		t.Fatalf("a failing latch write should log once, got %v", logged)
	}
	w.setPin(99, true)
	if len(logged) != 2 {
		t.Errorf("an out-of-range pin should log, got %v", logged)
	}
}
