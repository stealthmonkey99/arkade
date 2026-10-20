// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/flight"
	"github.com/stealthmonkey99/arkade/internal/input"
)

func newTestRecorder(t *testing.T, ringEvents int) *flightRecorder {
	t.Helper()
	r := newFlightRecorder(t.TempDir(), ringEvents, 0, func(string, string) {})
	if !r.enabled() {
		t.Fatal("recorder with a dump directory reports itself disabled")
	}
	r.now = func() time.Time { return time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC) }
	r.start(flight.Header{Slug: "cart/flight", WasmSHA256: "deadbeef"})
	return r
}

func TestRecorderWithoutADirectoryHasNoRingAndNoDumps(t *testing.T) {
	r := newFlightRecorder("", 0, 0, func(string, string) {})
	if r.enabled() {
		t.Fatal("a recorder with no directory reports itself enabled")
	}
	if r.buf != nil {
		t.Error("a disabled recorder allocated a ring")
	}
	r.start(flight.Header{Slug: "x"})
	r.note(1, flight.KindFrame, 33_333)
	r.suspend(4)
	r.stop()
	if _, ok := r.snapshot(flight.ReasonManual, ""); ok {
		t.Error("a disabled recorder produced a snapshot")
	}
}

func TestRingWrapsAndCountsDrops(t *testing.T) {
	const size = 8
	r := newTestRecorder(t, size)

	for i := 0; i < 5; i++ {
		r.note(uint32(i), flight.KindFrame, uint64(i))
	}
	rec, ok := r.snapshot(flight.ReasonManual, "")
	if !ok {
		t.Fatal("snapshot of a live session reported nothing to dump")
	}
	if len(rec.Events) != 5 || rec.Header.Dropped != 0 || rec.Header.Truncated {
		t.Fatalf("under-filled ring: %d events, %d dropped, truncated=%v; want 5/0/false",
			len(rec.Events), rec.Header.Dropped, rec.Header.Truncated)
	}
	for i, e := range rec.Events {
		if e.Data != uint64(i) {
			t.Fatalf("event %d = %d, want %d (ring is not oldest-first)", i, e.Data, i)
		}
	}

	for i := 5; i < 13; i++ {
		r.note(uint32(i), flight.KindFrame, uint64(i))
	}
	rec, _ = r.snapshot(flight.ReasonManual, "")
	if len(rec.Events) != size {
		t.Fatalf("overflowed ring holds %d events, want exactly %d", len(rec.Events), size)
	}
	if rec.Header.Dropped != 5 || !rec.Header.Truncated {
		t.Fatalf("dropped = %d truncated = %v, want 5/true", rec.Header.Dropped, rec.Header.Truncated)
	}
	for i, e := range rec.Events {
		if want := uint64(5 + i); e.Data != want {
			t.Fatalf("event %d = %d, want %d (wrapped ring is out of order)", i, e.Data, want)
		}
	}
}

func TestRetainedCountsFollowTheRing(t *testing.T) {
	r := newTestRecorder(t, 4)
	r.note(1, flight.KindFrame, 1)
	r.note(1, flight.KindInput, 2)
	r.note(1, flight.KindRandom, 3)
	r.note(2, flight.KindFrame, 4)
	// The next two evict the first two (a frame and an input).
	r.note(2, flight.KindFrame, 5)
	r.note(2, flight.KindFrame, 6)

	rec, _ := r.snapshot(flight.ReasonManual, "")
	if rec.Header.Frames != 3 || rec.Header.Inputs != 0 || rec.Header.Randoms != 1 {
		t.Errorf("retained counts = %d frames / %d inputs / %d randoms, want 3/0/1",
			rec.Header.Frames, rec.Header.Inputs, rec.Header.Randoms)
	}
	if int(rec.Header.Frames+rec.Header.Inputs+rec.Header.Randoms) != len(rec.Events) {
		t.Errorf("counts sum to %d but the recording holds %d events",
			rec.Header.Frames+rec.Header.Inputs+rec.Header.Randoms, len(rec.Events))
	}
}

// The ring is allocated once at session start; the record path only overwrites it.
func TestRecorderDoesNotAllocate(t *testing.T) {
	r := newTestRecorder(t, 1024)
	var frame uint32
	allocs := testing.AllocsPerRun(2000, func() {
		frame++
		r.note(frame, flight.KindFrame, 33_333)
		r.note(frame, flight.KindInput, 0x0102030405060708)
		r.note(frame, flight.KindRandom, uint64(frame)*2654435761)
	})
	if allocs != 0 {
		t.Errorf("recording three events allocated %.1f times per run, want 0", allocs)
	}
}

// A stop makes the ring undumpable; a suspend keeps it and marks where the recording ends.
func TestSessionLifecycle(t *testing.T) {
	r := newTestRecorder(t, 16)
	r.note(1, flight.KindFrame, 1)

	r.suspend(9)
	r.note(2, flight.KindFrame, 2) // ignored: recording has stopped
	rec, ok := r.snapshot(flight.ReasonCrash, "")
	if !ok {
		t.Fatal("a suspended session should still be dumpable — that is the point of keeping the ring")
	}
	if len(rec.Events) != 1 {
		t.Errorf("suspended session recorded %d events, want 1 (nothing after the suspension)", len(rec.Events))
	}
	if rec.Header.SuspendedAtFrame != 9 {
		t.Errorf("SuspendedAtFrame = %d, want 9", rec.Header.SuspendedAtFrame)
	}

	r.stop()
	if _, ok := r.snapshot(flight.ReasonManual, ""); ok {
		t.Error("a stopped session is still dumpable; it must not be")
	}

	r.start(flight.Header{Slug: "cart/other"})
	rec, ok = r.snapshot(flight.ReasonManual, "")
	if !ok || len(rec.Events) != 0 || rec.Header.SuspendedAtFrame != 0 {
		t.Errorf("a fresh session inherited state from the previous one: %+v", rec.Header)
	}
}

// comboRecord builds a packed record the way the wire does; kept local because internal/cartridge/wasm imports this package.
func comboRecord(kind, key, joy, flags uint8, physical, index uint8) uint64 {
	return uint64(kind) | uint64(key)<<8 | uint64(1)<<16 | uint64(joy)<<24 |
		uint64(1)<<32 | uint64(physical)<<40 | uint64(index)<<48 | uint64(flags)<<56
}

// joySeq is the physical Konami gesture as a stick reports it: eight joystick records. Rotation r simulates the cabinet's seat-mount remap (see comboRotations).
func joySeq(rotation int) []uint64 {
	dirs := []comboDir{
		comboDirUp, comboDirUp, comboDirDown, comboDirDown,
		comboDirLeft, comboDirRight, comboDirLeft, comboDirRight,
	}
	joyFor := map[comboDir]uint8{
		comboDirUp: comboJoyN, comboDirDown: comboJoyS,
		comboDirLeft: comboJoyW, comboDirRight: comboJoyE,
	}
	var out []uint64
	for _, d := range dirs {
		out = append(out, comboRecord(comboKindJoystick, 0, joyFor[comboRotate(d, rotation)], 0, 0, 0))
	}
	return out
}

func TestComboFiresAtEveryRotation(t *testing.T) {
	for rotation := 0; rotation < comboRotations; rotation++ {
		var c comboDetector
		c.feed(comboRecord(comboKindKeydown, comboKeyAux1, 0, 0, 0, 0)) // hold 🟣
		fired := false
		for i, rec := range joySeq(rotation) {
			seat, ok := c.feed(rec)
			if ok {
				if i != comboSteps-1 {
					t.Fatalf("rotation %d: fired early, at step %d", rotation, i)
				}
				if seat != input.SeatP1 {
					t.Errorf("rotation %d: seat = %v, want P1", rotation, seat)
				}
				fired = true
			}
		}
		if !fired {
			t.Errorf("rotation %d: the physical gesture did not fire the combo", rotation)
		}
	}
}

func TestComboNeedsTheModifier(t *testing.T) {
	var c comboDetector
	for _, rec := range joySeq(0) {
		if _, ok := c.feed(rec); ok {
			t.Fatal("the combo fired with no modifier held")
		}
	}
}

func TestComboReleasingTheModifierDisarms(t *testing.T) {
	var c comboDetector
	c.feed(comboRecord(comboKindKeydown, comboKeyAux1, 0, 0, 0, 0))
	seq := joySeq(0)
	for _, rec := range seq[:4] {
		c.feed(rec)
	}
	c.feed(comboRecord(comboKindKeyup, comboKeyAux1, 0, 0, 0, 0))
	for _, rec := range seq[4:] {
		if _, ok := c.feed(rec); ok {
			t.Fatal("the combo completed after the modifier was released")
		}
	}
}

// A terminal cannot report key release or two keys down at once, so the dev console arms with an Aux1 press that lasts as long as the attempt.
func TestComboKeyboardLatch(t *testing.T) {
	var c comboDetector
	c.feed(comboRecord(comboKindKeypress, comboKeyAux1, 0, comboFlagVirtual, input.NoSeat.Byte(), input.NoSlot.Byte()))
	dirs := []uint8{comboKeyUp, comboKeyUp, comboKeyDown, comboKeyDown,
		comboKeyLeft, comboKeyRight, comboKeyLeft, comboKeyRight}
	fired := false
	for _, k := range dirs {
		if seat, ok := c.feed(comboRecord(comboKindKeypress, k, 0, comboFlagVirtual, input.NoSeat.Byte(), input.NoSlot.Byte())); ok {
			fired = true
			if seat != input.NoSeat {
				t.Errorf("keyboard entry reported %v, want no seat (a keyboard has none)", seat)
			}
		}
	}
	if !fired {
		t.Error("the keyboard latch path did not fire the combo")
	}
}

// As in cartlib, a wrong press that is the sequence's first element restarts that rotation at 1, not 0.
func TestComboPartialMatchReset(t *testing.T) {
	var c comboDetector
	c.feed(comboRecord(comboKindKeydown, comboKeyAux1, 0, 0, 0, 0))
	up := comboRecord(comboKindJoystick, 0, comboJoyN, 0, 0, 0)

	c.feed(up)
	c.feed(up) // two matched: idx[0] == 2
	c.feed(up) // wrong for step 2, but IS step 0 — restart at 1, not 0
	if c.idx[0] != 1 {
		t.Fatalf("after a wrong press that is the sequence's own first element, "+
			"rotation 0 is at %d; want 1 (the partial-match reset is gone)", c.idx[0])
	}

	// The fumbled journey still completes: UP UP (UP UP) DOWN DOWN LEFT RIGHT LEFT RIGHT.
	rest := joySeq(0)[1:] // one more UP, then the rest of the code
	fired := false
	for _, rec := range rest {
		if _, ok := c.feed(rec); ok {
			fired = true
		}
	}
	if !fired {
		t.Error("a fumbled opening (two extra UPs) broke the combo")
	}
}

func TestDumpWritesAFileWithTheSessionHeader(t *testing.T) {
	dir := t.TempDir()
	rt := newDumpRuntime(t, dir)
	rt.flight.start(flight.Header{Slug: "cart/flight", ManifestVersion: "2.1.0", WasmSHA256: "abc123"})
	rt.flight.note(1, flight.KindFrame, 33_333)
	rt.flight.note(1, flight.KindInput, 0x09)

	out, err := rt.DumpFlight(flight.ReasonManual, "that was weird")
	if err != nil {
		t.Fatalf("DumpFlight: %v", err)
	}
	if filepath.Dir(out.Path) != dir {
		t.Errorf("dump written to %s, want a file in %s", out.Path, dir)
	}
	if out.Events != 2 || out.Slug != "cart/flight" || out.Bytes == 0 {
		t.Errorf("dump summary = %+v, want 2 events for cart/flight with a non-zero size", out)
	}
}

func TestDumpWithNoSessionIsARefusalNotAnError(t *testing.T) {
	rt := newDumpRuntime(t, t.TempDir())
	_, err := rt.DumpFlight(flight.ReasonManual, "")
	var none ErrNoFlightSession
	if err == nil || !errors.As(err, &none) {
		t.Fatalf("DumpFlight with no session = %v, want an ErrNoFlightSession", err)
	}
	if !strings.Contains(err.Error(), "no cartridge session") {
		t.Errorf("error = %q, want it to say no session is being recorded", err)
	}
}

func TestDisabledRecorderRefusesDumps(t *testing.T) {
	rt := newDumpRuntime(t, "")
	_, err := rt.DumpFlight(flight.ReasonManual, "")
	var none ErrNoFlightSession
	if err == nil || !errors.As(err, &none) {
		t.Fatalf("DumpFlight on a disabled recorder = %v, want an ErrNoFlightSession", err)
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("error = %q, want it to say the recorder is disabled", err)
	}
}

// A trapping guest re-traps every frame; the cap keeps the first evidence from being rotated off disk.
func TestOnlyOneCrashDumpPerSession(t *testing.T) {
	dir := t.TempDir()
	rt := newDumpRuntime(t, dir)
	rt.flight.start(flight.Header{Slug: "cart/flight", WasmSHA256: "abc"})
	rt.flight.note(1, flight.KindFrame, 1)

	for i := 0; i < 30; i++ {
		rt.NoteCartridgeFault("update", errFake{})
	}
	files, err := rt.flight.store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("30 faults produced %d dumps, want exactly 1", len(files))
	}

	// A new session re-arms the cap.
	rt.flight.start(flight.Header{Slug: "cart/flight", WasmSHA256: "abc"})
	rt.flight.note(1, flight.KindFrame, 1)
	rt.NoteCartridgeFault("update", errFake{})
	if files, _ := rt.flight.store.List(); len(files) != 2 {
		t.Errorf("a fault in a fresh session produced %d dumps total, want 2", len(files))
	}
}

type errFake struct{}

func (errFake) Error() string { return "wasm error: unreachable" }

// newDumpRuntime builds the minimum Runtime a dump needs: a recorder and a log sink. DumpFlight reads only the recorder's state.
func newDumpRuntime(t *testing.T, dir string) *Runtime {
	t.Helper()
	log := func(string, string) {}
	return &Runtime{logger: log, flight: newFlightRecorder(dir, 64, 0, log)}
}
