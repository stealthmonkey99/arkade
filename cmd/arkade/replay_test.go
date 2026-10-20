// SPDX-License-Identifier: GPL-2.0-or-later

package main

// The fixture, internal/cartridge/wasm/testdata/flight.wat, paints from all
// three recorded streams: input and accumulated dt move the sprite, and the
// cart_init random draw is its colour.

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/audio"
	"github.com/stealthmonkey99/arkade/internal/cartridge"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/flight"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/render"
)

const flightFixtureWasm = "../../internal/cartridge/wasm/testdata/flight.wasm"

const flightFixtureSlug = "cart/flight"

// installFlightFixture scans the fixture from a scratch directory and
// unregisters it on cleanup.
func installFlightFixture(t *testing.T) string {
	t.Helper()
	wasmBytes, err := os.ReadFile(flightFixtureWasm)
	if err != nil {
		t.Fatalf("reading the flight fixture cartridge: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "flight.wasm"), wasmBytes, 0o644); err != nil {
		t.Fatalf("seeding the cartridge directory: %v", err)
	}
	t.Cleanup(func() { engine.Unregister(flightFixtureSlug) })
	cartridge.NewRegistry(cartridge.Options{Dirs: []string{dir}}).Scan()
	if _, err := engine.Lookup(flightFixtureSlug); err != nil {
		t.Fatalf("the fixture did not register: %v", err)
	}
	return dir
}

// scriptedPresses moves the sprite to a position only the recorded input explains.
var scriptedPresses = map[int]input.KeyName{
	3: input.KeyRight, 4: input.KeyRight, 5: input.KeyRight,
	9: input.KeyLeft, 14: input.KeyRight, 15: input.KeyRight,
}

// recordSession runs the scripted session and returns the dump path and per-tick frame hashes.
func recordSession(t *testing.T, ticks int, extra map[int]input.KeyName) (dumpPath string, hashes []string) {
	t.Helper()
	dumpDir := t.TempDir()
	cap := render.NewCapture(false)
	clk := clock.NewManualClock()
	rt, err := engine.New(engine.Options{
		Renderer: cap, Clock: clk, Audio: audio.NewRecorder(),
		DataDir: t.TempDir(), StartApp: flightFixtureSlug,
		RandSeed: 20260905, FlightDir: dumpDir, Version: "test",
		Logger: func(string, string) {},
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}

	// Puts the cartridge's draw at a stream position re-seeding cannot reach.
	for i := 0; i < 7; i++ {
		rt.Rand.Uint64()
	}

	if err := rt.Init(); err != nil {
		t.Fatalf("engine.Init: %v", err)
	}
	if !rt.FlightRecording() {
		t.Fatal("launching a cartridge did not open a flight recording")
	}

	for i := 1; i <= ticks; i++ {
		if key, ok := scriptedPresses[i]; ok {
			pressForRecord(rt, key)
		}
		if key, ok := extra[i]; ok {
			pressForRecord(rt, key)
		}
		clk.Advance(rt.FrameBudget())
		if err := rt.Tick(); err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
		hashes = append(hashes, hashFrame(t, cap.LastFrame().Data))
	}

	out, err := rt.DumpFlight(flight.ReasonManual, "recorded by the replay test")
	if err != nil {
		t.Fatalf("DumpFlight: %v", err)
	}
	if out.Truncated {
		t.Fatalf("the test session overflowed the ring (%d dropped); it is meant to fit", out.Dropped)
	}
	return out.Path, hashes
}

// pressForRecord pushes a player 1 keypress through the engine's input path.
func pressForRecord(rt *engine.Runtime, name input.KeyName) {
	rt.Input.Push(input.Event{
		Kind: input.KindKeypress,
		Name: name,
		Source: input.PlayerSource{
			Type: input.SourceController, Slot: input.SlotOf(0), Seat: input.SeatP1,
		},
	})
}

func hashFrame(t *testing.T, m display.Matrix) string {
	t.Helper()
	h := sha256.New()
	for _, row := range m {
		for _, p := range row {
			_, _ = h.Write([]byte{p.R, p.G, p.B, p.A})
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func TestReplayReproducesTheRecordedSessionFrameForFrame(t *testing.T) {
	cartDir := installFlightFixture(t)
	const ticks = 40
	dump, recorded := recordSession(t, ticks, nil)

	var replayed []string
	err := runReplay(replayOptions{
		path: dump, dataDir: t.TempDir(), cartridgeDirs: []string{cartDir},
		cols: display.DefaultWidth, rows: display.DefaultHeight,
		renderer: render.NewCapture(false),
		onFrame: func(_ int, frame display.Matrix) {
			replayed = append(replayed, hashFrame(t, frame))
		},
	})
	if err != nil {
		t.Fatalf("runReplay: %v", err)
	}

	if len(replayed) != len(recorded) {
		t.Fatalf("replay painted %d frames, the recording had %d", len(replayed), len(recorded))
	}
	for i := range recorded {
		if replayed[i] != recorded[i] {
			t.Fatalf("frame %d differs:\n recorded %s\n replayed %s", i, recorded[i], replayed[i])
		}
	}

	// Identical frames prove nothing unless the fixture moves.
	distinct := map[string]bool{}
	for _, h := range recorded {
		distinct[h] = true
	}
	if len(distinct) < 5 {
		t.Fatalf("the recorded session only produced %d distinct frames; the fixture is not "+
			"exercising input/dt/rng and this test proves nothing", len(distinct))
	}
}

func TestReplayServesTheRecordedRandomStream(t *testing.T) {
	cartDir := installFlightFixture(t)
	dump, _ := recordSession(t, 5, nil)

	rec := decodeDump(t, dump)
	var recorded []uint64
	for _, e := range rec.Events {
		if e.Kind == flight.KindRandom {
			recorded = append(recorded, e.Data)
		}
	}
	if len(recorded) != 1 {
		t.Fatalf("the fixture drew %d random values, want exactly 1 (at cart_init)", len(recorded))
	}

	// What a bare re-seed would have produced instead.
	fresh := freshSeedFirstDraw(t, rec.Header.EngineSeed)
	if fresh == recorded[0] {
		t.Fatal("the recorded draw happens to equal a fresh stream's first draw; " +
			"this test cannot distinguish the two paths (adjust the pre-draws)")
	}

	var served []uint64
	rt := newProbeRuntime(t, rec.Header.EngineSeed)
	next := 0
	rt.SetCartRandomSource(func() (uint64, bool) {
		if next >= len(recorded) {
			return 0, false
		}
		v := recorded[next]
		next++
		return v, true
	})
	served = append(served, rt.CartRandomU64())
	if served[0] != recorded[0] {
		t.Errorf("CartRandomU64 served %d, want the recorded %d", served[0], recorded[0])
	}
	// Past the end of the recording it falls back to the engine RNG.
	if got := rt.CartRandomU64(); got != fresh {
		t.Errorf("past the end of the recording CartRandomU64 gave %d, want the engine RNG's %d", got, fresh)
	}
	_ = cartDir
}

func TestReplayRefusesAHashMismatch(t *testing.T) {
	cartDir := installFlightFixture(t)
	dump, _ := recordSession(t, 5, nil)

	// A rewritten hash looks like a rebuilt cartridge.
	rec := decodeDump(t, dump)
	realHash := rec.Header.WasmSHA256
	rec.Header.WasmSHA256 = strings.Repeat("00", 32)
	rec.Header.ManifestVersion = "9.9.9"
	mismatched := filepath.Join(t.TempDir(), "mismatched"+flight.Extension)
	writeDump(t, mismatched, rec)

	opts := replayOptions{
		path: mismatched, dataDir: t.TempDir(), cartridgeDirs: []string{cartDir},
		cols: display.DefaultWidth, rows: display.DefaultHeight,
		renderer: render.NewCapture(false),
	}
	err := runReplay(opts)
	if err == nil {
		t.Fatal("runReplay accepted a recording whose wasm hash does not match the installed cartridge")
	}
	for _, want := range []string{rec.Header.WasmSHA256, realHash, "9.9.9", "--replay-force"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q:\n%s", want, err)
		}
	}

	opts.force = true
	if err := runReplay(opts); err != nil {
		t.Fatalf("--replay-force still refused: %v", err)
	}
}

func TestReplayRejectsAFileThatIsNotARecording(t *testing.T) {
	cartDir := installFlightFixture(t)
	junk := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(junk, []byte("this is not a flight recording"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runReplay(replayOptions{
		path: junk, dataDir: t.TempDir(), cartridgeDirs: []string{cartDir},
		cols: display.DefaultWidth, rows: display.DefaultHeight,
		renderer: render.NewCapture(false),
	})
	if err == nil || !strings.Contains(err.Error(), "not an Årkade flight recording") {
		t.Fatalf("err = %v, want a clear 'not a recording' message", err)
	}
}

func TestReplayReportsAMissingCartridge(t *testing.T) {
	installFlightFixture(t)
	dump, _ := recordSession(t, 3, nil)
	engine.Unregister(flightFixtureSlug)

	emptyDir := t.TempDir()
	err := runReplay(replayOptions{
		path: dump, dataDir: t.TempDir(), cartridgeDirs: []string{emptyDir},
		cols: display.DefaultWidth, rows: display.DefaultHeight,
		renderer: render.NewCapture(false),
	})
	if err == nil {
		t.Fatal("runReplay accepted a recording for a cartridge that is not installed")
	}
	if !strings.Contains(err.Error(), flightFixtureSlug) || !strings.Contains(err.Error(), emptyDir) {
		t.Errorf("err = %v, want it to name the slug and where it looked", err)
	}
}

func TestReplayHonoursMaxFrames(t *testing.T) {
	cartDir := installFlightFixture(t)
	dump, _ := recordSession(t, 20, nil)

	painted := 0
	err := runReplay(replayOptions{
		path: dump, dataDir: t.TempDir(), cartridgeDirs: []string{cartDir},
		cols: display.DefaultWidth, rows: display.DefaultHeight,
		renderer:  render.NewCapture(false),
		maxFrames: 6,
		onFrame:   func(int, display.Matrix) { painted++ },
	})
	if err != nil {
		t.Fatalf("runReplay: %v", err)
	}
	if painted != 6 {
		t.Errorf("replayed %d frames, want 6", painted)
	}
}

// TestGuestTrapWritesACrashDump drives the fixture into `unreachable`, which
// is what a TinyGo -panic=trap build does on a runtime panic.
func TestGuestTrapWritesACrashDump(t *testing.T) {
	installFlightFixture(t)
	dumpDir := t.TempDir()
	cap := render.NewCapture(false)
	clk := clock.NewManualClock()
	var logs []string
	rt, err := engine.New(engine.Options{
		Renderer: cap, Clock: clk, Audio: audio.NewRecorder(),
		DataDir: t.TempDir(), StartApp: flightFixtureSlug,
		RandSeed: 7, FlightDir: dumpDir, Version: "test",
		Logger: func(level, msg string) { logs = append(logs, level+": "+msg) },
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	if err := rt.Init(); err != nil {
		t.Fatalf("engine.Init: %v", err)
	}

	// Five clean frames, then the key that arms the trap; the extra ticks find it faulted.
	for i := 1; i <= 5; i++ {
		clk.Advance(rt.FrameBudget())
		if err := rt.Tick(); err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
	}
	pressForRecord(rt, input.KeyClear)
	for i := 0; i < 10; i++ {
		clk.Advance(rt.FrameBudget())
		// A trapping guest is logged, not returned.
		if err := rt.Tick(); err != nil {
			t.Fatalf("a trapping cartridge propagated an error out of Tick: %v", err)
		}
	}

	entries, err := os.ReadDir(dumpDir)
	if err != nil {
		t.Fatalf("reading the dump directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("a trapping cartridge produced %d dumps, want exactly 1 (the re-trap cap)", len(entries))
	}

	rec := decodeDump(t, filepath.Join(dumpDir, entries[0].Name()))
	if rec.Header.Reason != flight.ReasonCrash {
		t.Errorf("dump reason = %q, want %q", rec.Header.Reason, flight.ReasonCrash)
	}
	if !strings.Contains(rec.Header.Note, "unreachable") {
		t.Errorf("dump note = %q, want the trap message", rec.Header.Note)
	}
	if rec.Header.Slug != flightFixtureSlug || rec.Header.WasmSHA256 == "" {
		t.Errorf("dump header = %+v, want the slug and wasm hash of the crashing cartridge", rec.Header)
	}
	if rec.Header.Frames < 5 {
		t.Errorf("the crash dump holds only %d frames; the run before the trap is the evidence",
			rec.Header.Frames)
	}
	if !anyContains(logs, "crash dump written") {
		t.Errorf("the automatic dump was not logged; logs = %v", logs)
	}
}

func decodeDump(t *testing.T, path string) *flight.Recording {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	rec, err := flight.Decode(f)
	if err != nil {
		t.Fatalf("decoding %s: %v", path, err)
	}
	return rec
}

func writeDump(t *testing.T, path string, rec *flight.Recording) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	if err := flight.Encode(f, rec); err != nil {
		t.Fatalf("encoding %s: %v", path, err)
	}
}

func newProbeRuntime(t *testing.T, seed int64) *engine.Runtime {
	t.Helper()
	rt, err := engine.New(engine.Options{
		Renderer: render.NewCapture(false), Clock: clock.NewManualClock(),
		Audio: audio.NewRecorder(), DataDir: t.TempDir(), RandSeed: seed,
		Logger: func(string, string) {},
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	return rt
}

func freshSeedFirstDraw(t *testing.T, seed int64) uint64 {
	t.Helper()
	return newProbeRuntime(t, seed).Rand.Uint64()
}

func anyContains(lines []string, want string) bool {
	for _, l := range lines {
		if strings.Contains(l, want) {
			return true
		}
	}
	return false
}
