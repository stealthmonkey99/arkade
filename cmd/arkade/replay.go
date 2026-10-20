// SPDX-License-Identifier: GPL-2.0-or-later

package main

// Replay feeds a recording's dt, input and random streams back into a real
// engine at the recorded cadence (docs/FLIGHT_RECORDER.md). Recorded input is
// post-remap, so it goes straight to the guest through engine.RawInputSink,
// never through Runtime.Input. Randomness is served back as recorded values
// (SetCartRandomSource); reseeding cannot reproduce it. Controls are read
// from the raw terminal because an engine keypress would reach the cartridge.

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	"github.com/stealthmonkey99/arkade/internal/audio"
	"github.com/stealthmonkey99/arkade/internal/cartridge"
	"github.com/stealthmonkey99/arkade/internal/cartridge/wasm"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/flight"
	"github.com/stealthmonkey99/arkade/internal/render"
)

// replayOptions configures one replay; renderer onward are test seams.
type replayOptions struct {
	path   string
	force  bool
	paused bool

	dataDir       string
	cartridgeDirs []string
	cols, rows    int

	renderer    render.Renderer
	interactive bool // real-time pacing, terminal controls and badge
	onFrame     func(index int, frame display.Matrix)
	maxFrames   int // 0 replays everything
}

// replayTick is one recorded engine tick: its duration, input records and
// channel events.
type replayTick struct {
	frame    uint32
	dt       uint64
	inputs   []uint64
	channels []flight.Event
}

// runReplay loads a dump, verifies it against the installed cartridge and
// replays it.
func runReplay(o replayOptions) error {
	rec, err := loadRecording(o.path)
	if err != nil {
		return err
	}
	describeRecording(rec)

	cartridge.NewRegistry(cartridge.Options{
		Dirs: o.cartridgeDirs, Width: o.cols, Height: o.rows,
		Logger: func(level, msg string) { fmt.Fprintf(os.Stderr, "[%s] %s\n", level, msg) },
	}).Scan()
	m, err := engine.Lookup(rec.Header.Slug)
	if err != nil {
		return fmt.Errorf("cartridge %q from the recording is not installed here: %w\n"+
			"  put its .rcart/.wasm in one of %s, or pass --cartridges",
			rec.Header.Slug, err, strings.Join(o.cartridgeDirs, ", "))
	}
	if err := verifyReplayHash(rec, m, o.force); err != nil {
		return err
	}

	ticks, randoms, channelBinds := planReplay(rec)
	if len(ticks) == 0 {
		return fmt.Errorf("%s holds no frame events: there is nothing to replay", o.path)
	}

	rend := o.renderer
	var console *render.Console
	if rend == nil {
		console = render.NewConsole()
		rend = console
	} else {
		console, _ = rend.(*render.Console)
	}
	clk := clock.NewManualClock()
	rt, err := engine.New(engine.Options{
		Renderer:  rend,
		Cols:      rec.Header.Cols,
		Rows:      rec.Header.Rows,
		TargetFPS: rec.Header.TargetFPS,
		Clock:     clk,
		// Silent: the recording carries no audio state.
		Audio:    audio.NewRecorder(),
		DataDir:  o.dataDir,
		StartApp: rec.Header.Slug,
		// The cartridge's randomness comes from SetCartRandomSource, not the seed.
		RandSeed: rec.Header.EngineSeed,
		Version:  version,
		Logger:   func(level, msg string) { fmt.Fprintf(os.Stderr, "[%s] %s\n", level, msg) },
		// Zero disables standby, which would suspend the replayed cartridge.
		AttractAfter: 0,
	})
	if err != nil {
		return err
	}

	next := 0
	rt.SetCartRandomSource(func() (uint64, bool) {
		if next >= len(randoms) {
			return 0, false
		}
		v := randoms[next]
		next++
		return v, true
	})

	// Installed before Init, where the binds happen, so a session recorded
	// with a spinner attached replays on a machine without one.
	channels := wasm.NewChannelReplay(channelBinds)
	rt.SetCartAppHook(func(app engine.App) {
		if wa, ok := app.(*wasm.App); ok {
			wa.SetChannelReplay(channels)
		}
	})

	state := &replayState{paused: o.paused, total: len(ticks)}
	if console != nil && o.interactive {
		console.InputBadge = state.badge
	}
	if o.interactive {
		stop := state.readControls()
		defer stop()
	}

	if err := rt.Init(); err != nil {
		return err
	}
	defer func() { _ = rt.Renderer.Close() }()

	limit := len(ticks)
	if o.maxFrames > 0 && o.maxFrames < limit {
		limit = o.maxFrames
	}
	for i := 0; i < limit; i++ {
		t := ticks[i]
		if o.interactive && !state.waitToRun() {
			break
		}
		state.set(i+1, t.frame)

		// Input goes first, matching a real tick's handle_input, update, render order.
		if len(t.inputs) > 0 {
			sink, ok := rt.CurrentRawInputSink()
			if !ok {
				return fmt.Errorf("frame %d: the running app is not a cartridge any more "+
					"(it exited, or the recording outlived its session)", t.frame)
			}
			if err := sink.DispatchRawRecords(packReplayInputs(t.inputs)); err != nil {
				return fmt.Errorf("frame %d: feeding input: %w", t.frame, err)
			}
		}

		// Channels are recorded only on change; a quiet frame keeps its levels.
		for _, e := range t.channels {
			switch e.Kind {
			case flight.KindChannel:
				handle, _, value := flight.UnpackChannel(e.Data)
				channels.SetValue(handle, value)
			case flight.KindChannelBlob:
				handle, chunk, payload := flight.UnpackChannelBlob(e.Data)
				channels.SetBlobChunk(handle, chunk, payload[:])
			}
		}

		started := time.Now()
		clk.Advance(time.Duration(t.dt) * time.Microsecond)
		if err := rt.Tick(); err != nil {
			return fmt.Errorf("frame %d: %w", t.frame, err)
		}
		if o.onFrame != nil {
			if f := lastFrame(rend); f != nil {
				o.onFrame(i, f)
			}
		}
		if rt.QuitRequested() || state.quit() {
			break
		}
		if o.interactive {
			if sleep := time.Duration(t.dt)*time.Microsecond - time.Since(started); sleep > 0 {
				time.Sleep(sleep)
			}
		}
	}
	fmt.Fprintf(os.Stderr, "replay: finished after %d of %d recorded frames\n", state.index(), len(ticks))
	return nil
}

// loadRecording opens and decodes a dump.
func loadRecording(path string) (*flight.Recording, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening the recording: %w", err)
	}
	defer func() { _ = f.Close() }()
	rec, err := flight.Decode(f)
	if err != nil {
		if errors.Is(err, flight.ErrNotARecording) {
			return nil, fmt.Errorf("%s is not an Årkade flight recording (expected a %s file): %w",
				path, flight.Extension, err)
		}
		return nil, err
	}
	return rec, nil
}

// describeRecording prints the recording's header to stderr.
func describeRecording(rec *flight.Recording) {
	h := rec.Header
	fmt.Fprintf(os.Stderr, "replay: %s %q version %q\n", h.Slug, h.Name, h.ManifestVersion)
	fmt.Fprintf(os.Stderr, "replay: recorded %s on %s (%s), dumped %s because: %s\n",
		h.StartedAt.Format(time.RFC3339), h.Platform, h.EngineVersion,
		h.DumpedAt.Format(time.RFC3339), h.Reason)
	if h.Note != "" {
		fmt.Fprintf(os.Stderr, "replay: note: %s\n", h.Note)
	}
	fmt.Fprintf(os.Stderr, "replay: %d events (%d frames, %d input records, %d rng draws), seed %d, %dfps\n",
		len(rec.Events), h.Frames, h.Inputs, h.Randoms, h.EngineSeed, h.TargetFPS)
	if h.Truncated {
		fmt.Fprintf(os.Stderr, "replay: WARNING — TRUNCATED: the ring overwrote %d events, so this "+
			"recording starts mid-session. The replay will begin from a state the cartridge "+
			"never reached this way and will NOT reproduce the session.\n", h.Dropped)
	}
	if h.SuspendedAtFrame != 0 {
		fmt.Fprintf(os.Stderr, "replay: WARNING — standby suspended this cartridge at frame %d; "+
			"recording stopped there and nothing after it exists.\n", h.SuspendedAtFrame)
	}
}

// verifyReplayHash requires the installed wasm to match the recording's hash,
// since different bytes under the same slug would replay a different game.
// force downgrades the refusal to a warning, for replaying against a rebuild.
func verifyReplayHash(rec *flight.Recording, m engine.Manifest, force bool) error {
	if rec.Header.WasmSHA256 == "" {
		return fmt.Errorf("the recording carries no wasm hash: it predates the pinning rule and cannot be trusted to replay")
	}
	if rec.Header.WasmSHA256 == m.WasmSHA256 {
		return nil
	}
	msg := fmt.Sprintf("wasm mismatch for %s:\n"+
		"  recorded: %s (manifest version %q)\n"+
		"  installed: %s (manifest version %q)\n"+
		"  The recording was made against different bytes, so replaying it here reproduces\n"+
		"  nothing. Note that a REBUILD of the same source can differ byte for byte across\n"+
		"  machines and toolchain versions (docs/FLIGHT_RECORDER.md, \"Limits\") — the fix is\n"+
		"  usually to fetch the .wasm from the cabinet, not to rebuild it.",
		rec.Header.Slug,
		rec.Header.WasmSHA256, rec.Header.ManifestVersion,
		m.WasmSHA256, m.Version)
	if !force {
		return errors.New(msg + "\n  Pass --replay-force to replay anyway.")
	}
	fmt.Fprintf(os.Stderr, "replay: WARNING — %s\n  --replay-force given: replaying anyway.\n", msg)
	return nil
}

// planReplay turns the event list into per-tick scripts, the random stream
// in draw order and the channel binds. Inputs are bucketed by frame number,
// not list position, because the recorder files a tick's dt before its
// input drain.
func planReplay(rec *flight.Recording) ([]replayTick, []uint64, []wasm.ChannelBind) {
	inputs := map[uint32][]uint64{}
	channels := map[uint32][]flight.Event{}
	var randoms []uint64
	var binds []wasm.ChannelBind
	var ticks []replayTick
	for _, e := range rec.Events {
		switch e.Kind {
		case flight.KindInput:
			inputs[e.Frame] = append(inputs[e.Frame], e.Data)
		case flight.KindRandom:
			randoms = append(randoms, e.Data)
		case flight.KindFrame:
			ticks = append(ticks, replayTick{frame: e.Frame, dt: e.Data})
		case flight.KindChannelBind:
			handle, kind, blobLen := flight.UnpackChannelBind(e.Data)
			// Indexed by handle; a gap from a truncated ring leaves a zero
			// entry that fails the bind on replay.
			for int(handle) >= len(binds) {
				binds = append(binds, wasm.ChannelBind{})
			}
			binds[handle] = wasm.ChannelBind{Kind: wasm.ChannelKind(kind), BlobLen: blobLen}
		case flight.KindChannel, flight.KindChannelBlob:
			channels[e.Frame] = append(channels[e.Frame], e)
		}
	}
	for i := range ticks {
		ticks[i].inputs = inputs[ticks[i].frame]
		ticks[i].channels = channels[ticks[i].frame]
	}
	return ticks, randoms, binds
}

// packReplayInputs packs recorded words into the ABI's 8-byte-per-record buffer.
func packReplayInputs(words []uint64) []byte {
	recs := make([]wasm.InputRecord, len(words))
	for i, w := range words {
		recs[i] = wasm.UnpackRecord(w)
	}
	return wasm.PackRecords(recs)
}

// lastFrame returns the capture renderer's latest frame, or nil for any other renderer.
func lastFrame(r render.Renderer) display.Matrix {
	cap, ok := r.(*render.Capture)
	if !ok {
		return nil
	}
	f := cap.LastFrame()
	if f == nil {
		return nil
	}
	return f.Data
}

// replayState is shared between the replay loop and the terminal reader.
type replayState struct {
	mu       sync.Mutex
	cond     *sync.Cond
	paused   bool
	step     bool
	stopped  bool
	idx      int
	frame    uint32
	total    int
	initOnce sync.Once
}

func (s *replayState) init() {
	s.initOnce.Do(func() { s.cond = sync.NewCond(&s.mu) })
}

// waitToRun blocks while paused, returning false when the user quit. A step
// consumes itself, so `.` advances exactly one frame and pauses again.
func (s *replayState) waitToRun() bool {
	s.init()
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.paused && !s.step && !s.stopped {
		s.cond.Wait()
	}
	s.step = false
	return !s.stopped
}

func (s *replayState) set(idx int, frame uint32) {
	s.init()
	s.mu.Lock()
	s.idx, s.frame = idx, frame
	s.mu.Unlock()
}

func (s *replayState) index() int {
	s.init()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.idx
}

func (s *replayState) quit() bool {
	s.init()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped
}

// badge is the frame counter shown in the console's input-badge slot.
func (s *replayState) badge() string {
	s.init()
	s.mu.Lock()
	defer s.mu.Unlock()
	mode := "\x1b[1;30;46m REPLAY \x1b[0m"
	if s.paused {
		mode = "\x1b[1;30;43m PAUSED \x1b[0m"
	}
	return fmt.Sprintf("%s\x1b[90m %d/%d (f%d) \x1b[0m", mode, s.idx, s.total, s.frame)
}

// readControls reads single keys in raw mode until the returned stop is called.
func (s *replayState) readControls() func() {
	s.init()
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return func() {}
	}
	old, err := term.MakeRaw(fd)
	if err != nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 8)
		for {
			select {
			case <-done:
				return
			default:
			}
			n, err := os.Stdin.Read(buf)
			if err != nil {
				return
			}
			for _, b := range buf[:n] {
				s.key(b)
			}
		}
	}()
	return func() {
		close(done)
		_ = term.Restore(fd, old)
	}
}

func (s *replayState) key(b byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch b {
	case ' ':
		s.paused = !s.paused
	case '.', 'n':
		s.paused, s.step = true, true
	case 'q', 3 /* ctrl-c */, 4 /* ctrl-d */ :
		s.stopped = true
	}
	s.cond.Broadcast()
}
