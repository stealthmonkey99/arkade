// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/stealthmonkey99/arkade/internal/flight"
)

// FlightRingEvents is the default ring capacity: 131072 events of 16 bytes is 2 MiB. At 30fps frame
// events alone fill it in about 72 minutes; real play, with input and random events, is roughly 40-55.
const FlightRingEvents = 1 << 17

// flightRecorder is the ring plus the session it belongs to, one cartridge launch. It is observe-only:
// it never injects input, touches standby, loads anything or draws from the engine RNG. Methods are
// nil-safe and disabled-safe, so hot paths call them unguarded.
type flightRecorder struct {
	// mu is held for one array store on the record path and for a copy on the dump path, never across
	// the gzip or write.
	mu sync.Mutex

	buf   []flight.Event
	head  int
	count int
	// dropped counts events the ring overwrote; non-zero means the recording no longer starts at the
	// session start.
	dropped uint64

	// active: a cartridge session is live and recording. stale: standby parked the session, so the ring
	// is kept for a dump but nothing new is recorded.
	active                            bool
	stale                             bool
	header                            flight.Header
	frames, inputs, randoms, channels uint32
	// dumpedForFault caps automatic crash dumps at one per session: a trapping guest re-traps every
	// frame and would rotate the evidence out.
	dumpedForFault bool
	// dumpedForCombo likewise caps the pad-entered dump, since a game may use the combo as a gameplay
	// shortcut.
	dumpedForCombo bool

	combo comboDetector

	store *flight.Store
	log   func(level, msg string)
	// now is wall-clock, used only for header timestamps and never fed back into the sim.
	now func() time.Time
}

// newFlightRecorder builds a recorder. dir == "" disables it: no ring, no dumps, every record call
// returns immediately. Only cmd/arkade enables it.
func newFlightRecorder(dir string, ringEvents, keep int, log func(level, msg string)) *flightRecorder {
	r := &flightRecorder{log: log, now: time.Now}
	if dir == "" {
		return r
	}
	if ringEvents <= 0 {
		ringEvents = FlightRingEvents
	}
	if ringEvents > flight.MaxEvents {
		ringEvents = flight.MaxEvents
	}
	r.buf = make([]flight.Event, ringEvents)
	r.store = flight.NewStore(dir, keep)
	return r
}

// enabled reports whether this recorder has a ring at all.
func (r *flightRecorder) enabled() bool { return r != nil && r.buf != nil }

// start opens a new session, discarding the previous ring contents. Engine goroutine (loadApp).
func (r *flightRecorder) start(h flight.Header) {
	if !r.enabled() {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.head, r.count, r.dropped = 0, 0, 0
	r.frames, r.inputs, r.randoms = 0, 0, 0
	r.active, r.stale = true, false
	r.dumpedForFault, r.dumpedForCombo = false, false
	r.combo.reset()
	r.header = h
}

// suspend stops recording but keeps the ring, marking the frame standby parked the cartridge at.
// Recording does not resume, and a replay says it ends early.
func (r *flightRecorder) suspend(frame uint32) {
	if !r.enabled() {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.active {
		return
	}
	r.active, r.stale = false, true
	r.header.SuspendedAtFrame = frame
}

// stop ends the session and clears the ring; a later dump gets nothing, since the cabinet no longer
// matches what was recorded.
func (r *flightRecorder) stop() {
	if !r.enabled() {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active, r.stale = false, false
	r.head, r.count = 0, 0
	r.frames, r.inputs, r.randoms, r.channels = 0, 0, 0, 0
}

// note appends one event. Hot path: no allocation, no logging.
func (r *flightRecorder) note(frame uint32, kind uint8, data uint64) {
	if r == nil || r.buf == nil {
		return
	}
	r.mu.Lock()
	if !r.active {
		r.mu.Unlock()
		return
	}
	if r.count == len(r.buf) {
		r.dropped++
		// The slot being overwritten is the oldest; decrement its kind so the header counts describe what
		// is retained.
		switch r.buf[r.head].Kind {
		case flight.KindFrame:
			r.frames--
		case flight.KindInput:
			r.inputs--
		case flight.KindRandom:
			r.randoms--
		case flight.KindChannelBind, flight.KindChannel, flight.KindChannelBlob:
			r.channels--
		}
	} else {
		r.count++
	}
	r.buf[r.head] = flight.Event{Frame: frame, Kind: kind, Data: data}
	r.head++
	if r.head == len(r.buf) {
		r.head = 0
	}
	switch kind {
	case flight.KindFrame:
		r.frames++
	case flight.KindInput:
		r.inputs++
	case flight.KindRandom:
		r.randoms++
	case flight.KindChannelBind, flight.KindChannel, flight.KindChannelBlob:
		r.channels++
	}
	r.mu.Unlock()
}

// snapshot copies the ring into a Recording, oldest first, with the dump-time header. The lock covers
// only the copy; encode and write happen outside it.
func (r *flightRecorder) snapshot(reason, note string) (*flight.Recording, bool) {
	if !r.enabled() {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.active && !r.stale {
		return nil, false
	}
	rec := &flight.Recording{Header: r.header, Events: make([]flight.Event, r.count)}
	start := (r.head - r.count + len(r.buf)) % len(r.buf)
	for i := 0; i < r.count; i++ {
		rec.Events[i] = r.buf[(start+i)%len(r.buf)]
	}
	rec.Header.Reason = reason
	rec.Header.Note = note
	rec.Header.DumpedAt = r.now()
	rec.Header.Dropped = r.dropped
	rec.Header.Truncated = r.dropped > 0
	rec.Header.Frames, rec.Header.Inputs, rec.Header.Randoms = r.frames, r.inputs, r.randoms
	rec.Header.Channels = r.channels
	return rec, true
}

// FlightDump reports a completed dump: enough for an HTTP response, a log line and a bug report.
type FlightDump struct {
	Path      string `json:"path"`
	Slug      string `json:"slug"`
	Reason    string `json:"reason"`
	Events    int    `json:"events"`
	Frames    uint32 `json:"frames"`
	Inputs    uint32 `json:"inputs"`
	Randoms   uint32 `json:"randoms"`
	Dropped   uint64 `json:"droppedEvents"`
	Truncated bool   `json:"truncated"`
	Bytes     int64  `json:"bytes"`
}

// ErrNoFlightSession is returned by DumpFlight when there is nothing to
// dump: the recorder is disabled, or no cartridge session is live.
type ErrNoFlightSession struct{ Reason string }

func (e ErrNoFlightSession) Error() string { return e.Reason }

// DumpFlight writes the current ring to a file and returns what it wrote. Safe from any goroutine; the
// HTTP handler and the crash hook both call it. It does not queue for the engine loop: it only reads
// under the recorder's mutex, and queueing would keep the caller from learning the filename.
func (rt *Runtime) DumpFlight(reason, note string) (FlightDump, error) {
	r := rt.flight
	if !r.enabled() {
		return FlightDump{}, ErrNoFlightSession{"flight recorder is disabled (no dump directory configured)"}
	}
	rec, ok := r.snapshot(reason, note)
	if !ok {
		return FlightDump{}, ErrNoFlightSession{"no cartridge session is being recorded"}
	}
	path, err := r.store.Write(rec, rec.Header.DumpedAt)
	if err != nil {
		return FlightDump{}, err
	}
	out := FlightDump{
		Path: path, Slug: rec.Header.Slug, Reason: reason,
		Events: len(rec.Events), Frames: rec.Header.Frames,
		Inputs: rec.Header.Inputs, Randoms: rec.Header.Randoms,
		Dropped: rec.Header.Dropped, Truncated: rec.Header.Truncated,
	}
	if fi, err := os.Stat(path); err == nil {
		out.Bytes = fi.Size()
	}
	return out, nil
}

// FlightRecording reports whether a cartridge session is currently being recorded.
func (rt *Runtime) FlightRecording() bool {
	r := rt.flight
	if !r.enabled() {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active
}

// startFlightSession opens a recording for a freshly loaded cartridge, on the engine goroutine after
// frames.reset so frame numbering starts with the tick counter.
func (rt *Runtime) startFlightSession(m Manifest) {
	if !rt.flight.enabled() {
		return
	}
	if m.WasmSHA256 == "" {
		// Not a cartridge: nothing to pin a replay to; close any stale recording.
		rt.flight.stop()
		return
	}
	cols, rows := rt.Display.Size()
	rt.flight.start(flight.Header{
		Slug:            m.Slug,
		Name:            m.Name,
		ManifestVersion: m.Version,
		WasmSHA256:      m.WasmSHA256,
		EngineSeed:      rt.randSeed,
		TargetFPS:       rt.targetFPS,
		Cols:            cols,
		Rows:            rows,
		StartedAt:       rt.flight.now(),
		EngineVersion:   rt.buildVersion,
		Platform:        runtime.GOOS + "/" + runtime.GOARCH,
	})
	rt.Log("info", fmt.Sprintf("flight: recording %s (%s)", m.Slug, shortHash(m.WasmSHA256)))
}

// RecordCartridgeChannelBind files one successful host_input_bind. Called during cart_init so it lands
// at frame 0 and a replay reads the bind table before the guest's first tick; without it a replay on a
// machine lacking the device would bind -1 and diverge.
func (rt *Runtime) RecordCartridgeChannelBind(handle int32, kind uint8, blobLen int) {
	rt.flight.note(uint32(rt.frames.ticks), flight.KindChannelBind, flight.PackChannelBind(handle, kind, blobLen))
}

// RecordCartridgeChannel files one non-panel channel's sampled value for this frame. The caller invokes
// it only when the value has something to say, so idle channels cost nothing.
func (rt *Runtime) RecordCartridgeChannel(handle int32, kind uint8, value int32) {
	rt.flight.note(uint32(rt.frames.ticks), flight.KindChannel, flight.PackChannel(handle, kind, value))
}

// RecordCartridgeChannelBlob files a changed blob channel as a run of
// six-byte chunks, in order.
func (rt *Runtime) RecordCartridgeChannelBlob(handle int32, blob []byte) {
	for chunk := 0; chunk*flight.ChannelBlobChunkBytes < len(blob); chunk++ {
		off := chunk * flight.ChannelBlobChunkBytes
		end := off + flight.ChannelBlobChunkBytes
		if end > len(blob) {
			end = len(blob)
		}
		rt.flight.note(uint32(rt.frames.ticks), flight.KindChannelBlob, flight.PackChannelBlob(handle, chunk, blob[off:end]))
	}
}

// RecordCartridgeInput files one packed 8-byte ABI input record against the current frame. The
// cartridge host calls it as records reach the guest (post-remap, standby gating and player-strategy
// gate), so it records what the cartridge saw.
func (rt *Runtime) RecordCartridgeInput(packed uint64) {
	rt.flight.note(uint32(rt.frames.ticks), flight.KindInput, packed)
}

// CartRandomU64 is the cartridge's host_random_u64: the engine RNG by default, the recorded stream
// during a replay, recorded either way. The engine RNG is shared with the shell, attract and
// screensavers, so the seed alone cannot reproduce a session; hence SetCartRandomSource.
func (rt *Runtime) CartRandomU64() uint64 {
	var v uint64
	if rt.replay.rand != nil {
		if recorded, ok := rt.replay.rand(); ok {
			v = recorded
		} else {
			v = rt.Rand.Uint64()
		}
	} else {
		v = rt.Rand.Uint64()
	}
	rt.flight.note(uint32(rt.frames.ticks), flight.KindRandom, v)
	return v
}

// SetCartRandomSource installs a replacement for the host_random_u64 stream: src returns the next
// recorded value, or ok=false once the recording runs out, after which the engine RNG resumes. Replay
// driver only; nil restores normal behaviour.
func (rt *Runtime) SetCartRandomSource(src func() (uint64, bool)) { rt.replay.rand = src }

// NoteCartridgeFault is the crash trigger, called when a cart_* call returns an error (a TinyGo
// -panic=trap build surfaces guest panics as wazero errors). The host latches after the first fault
// and never re-enters the guest; a later host-side error still arrives here, and only the first
// fault per session dumps.
func (rt *Runtime) NoteCartridgeFault(phase string, err error) {
	r := rt.flight
	if !r.enabled() {
		return
	}
	r.mu.Lock()
	first := (r.active || r.stale) && !r.dumpedForFault
	if first {
		r.dumpedForFault = true
	}
	r.mu.Unlock()
	if !first {
		return
	}
	rt.dumpAndLog(flight.ReasonCrash, fmt.Sprintf("%s: %v", phase, err))
}

// noteFlightFrame files this tick's dtMicros, so a replay runs at the session's real cadence.
func (rt *Runtime) noteFlightFrame(dtMicros uint64) {
	rt.flight.note(uint32(rt.frames.ticks), flight.KindFrame, dtMicros)
}

// dumpAndLog performs a dump and logs the outcome (the dashboard's audit trail), for triggers with no
// caller to return an error to.
func (rt *Runtime) dumpAndLog(reason, note string) {
	out, err := rt.DumpFlight(reason, note)
	if err != nil {
		var none ErrNoFlightSession
		if errors.As(err, &none) {
			return // nothing was being recorded; not worth a line
		}
		rt.Log("error", fmt.Sprintf("flight: %s dump failed: %v", reason, err))
		return
	}
	rt.Log("warn", fmt.Sprintf("flight: %s dump written to %s (%d events, %d frames%s)",
		reason, out.Path, out.Events, out.Frames, truncatedNote(out.Truncated)))
}

func truncatedNote(truncated bool) string {
	if truncated {
		return ", TRUNCATED — the session start was overwritten"
	}
	return ""
}

// RawInputSink is implemented by an App that accepts ABI input records directly, bypassing the input
// stack (wasm.App only). Replay needs it: recorded records are already post-remap, so pushing them
// through Runtime.Input would remap them twice.
type RawInputSink interface {
	// DispatchRawRecords delivers packed 8-byte ABI records to the guest,
	// exactly as HandleInput would have.
	DispatchRawRecords(packed []byte) error
}

// CartAppHook is called with each cartridge App before its Init runs, so a replay can serve recorded
// input binds, which a cartridge makes during cart_init. Nil on a cabinet.
type CartAppHook func(app App)

// replayHooks are the replay debugger's hooks into a cartridge session; both
// nil on the cabinet.
type replayHooks struct {
	rand func() (uint64, bool)
	app  CartAppHook
}

// SetCartAppHook installs or, with nil, clears the CartAppHook.
func (rt *Runtime) SetCartAppHook(f CartAppHook) { rt.replay.app = f }

// NotifyCartApp runs the hook, if any. Called by internal/cartridge's
// launch closure the moment the App exists.
func (rt *Runtime) NotifyCartApp(app App) {
	if rt.replay.app != nil {
		rt.replay.app(app)
	}
}

// CurrentRawInputSink returns the running app as a RawInputSink, or
// ok=false when nothing is loaded or the current app is native.
func (rt *Runtime) CurrentRawInputSink() (RawInputSink, bool) {
	if rt.cur == nil {
		return nil, false
	}
	sink, ok := rt.cur.(RawInputSink)
	return sink, ok
}

// shortHash abbreviates a content hash for a log line.
func shortHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12]
}
