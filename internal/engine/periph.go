// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// PeriphInputRingEvents is how many input records the ring holds: several seconds of busy two-stick
// play, so a stalled reader sees a reported gap.
const PeriphInputRingEvents = 512

// PeriphInput is one observed input event: the post-remap input.Event the app was about to receive,
// plus source and time. The JSON tags are the wire format of GET /api/periph/stream input events.
type PeriphInput struct {
	// Seq is the event's session-long sequence number; gaps mean the reader fell behind the ring.
	Seq uint64 `json:"seq"`
	// Frame is the engine tick the event was observed on.
	Frame uint64 `json:"frame"`
	// AtMs is engine-clock milliseconds since the Runtime was built, the same clock as Status().UptimeMs.
	AtMs float64 `json:"atMs"`
	// Kind/Name/Position mirror input.Event. Position is empty for key
	// events, Name empty for a bare joystick change.
	Kind     string `json:"kind"`
	Name     string `json:"name,omitempty"`
	Position string `json:"position,omitempty"`
	Ctrl     bool   `json:"ctrl,omitempty"`
	Shift    bool   `json:"shift,omitempty"`
	Meta     bool   `json:"meta,omitempty"`
	// Source is "keyboard" or "controller"; DeviceID is the stable device
	// identity (HID path, or "virtual-N" for a keyboard-driven pad).
	Source   string `json:"source"`
	DeviceID string `json:"deviceId,omitempty"`
	// Slot is the controller's attach slot and Seat where it plays; the wire
	// carries them as the zero-based attach index and "P1"/"P2", each null
	// for a source without one.
	Slot input.Slot `json:"-"`
	Seat input.Seat `json:"-"`
}

type periphInputWire PeriphInput

type periphInputJSON struct {
	periphInputWire
	Slot *int    `json:"slot"`
	Seat *string `json:"seat"`
}

func (p PeriphInput) MarshalJSON() ([]byte, error) {
	w := periphInputJSON{periphInputWire: periphInputWire(p)}
	if p.Slot != input.NoSlot {
		i := p.Slot.Index()
		w.Slot = &i
	}
	if p.Seat.Valid() {
		s := p.Seat.String()
		w.Seat = &s
	}
	return json.Marshal(w)
}

func (p *PeriphInput) UnmarshalJSON(b []byte) error {
	var w periphInputJSON
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*p = PeriphInput(w.periphInputWire)
	if w.Slot != nil {
		p.Slot = input.SlotOf(*w.Slot)
	}
	if w.Seat != nil {
		p.Seat = seatNamed(*w.Seat)
		if p.Seat == input.NoSeat {
			return fmt.Errorf("periph input: unknown seat %q", *w.Seat)
		}
	}
	return nil
}

func seatNamed(name string) input.Seat {
	for i := range input.SeatCount {
		if s := input.SeatOf(i); s.String() == name {
			return s
		}
	}
	return input.NoSeat
}

// PeriphSnapshot is the continuous half of the observation (state rather than events), published once
// per tick while watched.
type PeriphSnapshot struct {
	// Watching is false in the zero value returned before the first tick after arming.
	Watching bool   `json:"watching"`
	Frame    uint64 `json:"frame"`
	// Seq is the sequence the next observed input will get, the ring's high-water mark.
	Seq uint64 `json:"seq"`
	// Dropped counts input events the ring has overwritten since the session began.
	Dropped uint64 `json:"dropped"`
	// Lights is each seat's lamp state as the app set it (the hardware shadow); the pins cannot be read back.
	Lights [2]hardware.LightMask `json:"lights"`
	// Levels is the same state with brightness kept.
	Levels [2]hardware.LampLevels `json:"levels"`
	// LightShowActive reports whether a lab choreography owns the lamps; Lights still reports the app's
	// state, which the show hands back.
	LightShowActive bool `json:"lightShowActive"`
	// LightShowStep is the running choreography's step index, LightShowMask what it has on the lamps.
	LightShowStep int                   `json:"lightShowStep"`
	LightShowMask [2]hardware.LightMask `json:"lightShowMask"`
	// Mic is the latest level frame, MicOK whether the suite produced one, MicInfo the capture path.
	Mic     hardware.MicLevel `json:"mic"`
	MicOK   bool              `json:"micOk"`
	MicInfo hardware.MicInfo  `json:"micInfo"`
}

// periphRing holds observed input: a pre-allocated buffer under one mutex, with a drop counter so a
// lagging reader is told what it missed. Input is discrete, so it gets a ring; lamps, motion and mic are
// continuous and go in the snapshot.
type periphRing struct {
	mu      sync.Mutex
	buf     []PeriphInput
	head    int
	count   int
	seq     uint64 // sequence the next recorded event will carry
	dropped uint64
}

// note appends one event. Hot path: one lock, one store, no allocation.
func (r *periphRing) note(ev PeriphInput) {
	r.mu.Lock()
	if r.buf == nil {
		r.buf = make([]PeriphInput, PeriphInputRingEvents)
	}
	ev.Seq = r.seq
	r.seq++
	if r.count == len(r.buf) {
		r.dropped++
	} else {
		r.count++
	}
	r.buf[r.head] = ev
	r.head++
	if r.head == len(r.buf) {
		r.head = 0
	}
	r.mu.Unlock()
}

// drain copies out every retained event with Seq >= cursor, oldest first, with the next cursor and the
// count lost since cursor.
func (r *periphRing) drain(cursor uint64) (out []PeriphInput, next, missed uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.count == 0 {
		return nil, r.seq, 0
	}
	oldest := r.seq - uint64(r.count)
	if cursor < oldest {
		missed = oldest - cursor
		cursor = oldest
	}
	if cursor >= r.seq {
		return nil, r.seq, missed
	}
	n := int(r.seq - cursor)
	out = make([]PeriphInput, 0, n)
	start := (r.head - r.count + len(r.buf)) % len(r.buf)
	for i := int(cursor - oldest); i < r.count; i++ {
		out = append(out, r.buf[(start+i)%len(r.buf)])
	}
	return out, r.seq, missed
}

func (r *periphRing) stats() (seq, dropped uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seq, r.dropped
}

// periphObserver is the dashboard's window on the peripherals: a watcher
// count that arms it, the ring of observed inputs, and the snapshot
// published once a tick. Disarmed, an input costs one atomic load and a
// tick nothing. Safe from any goroutine.
type periphObserver struct {
	watchers atomic.Int32
	inputs   periphRing
	snap     atomic.Pointer[PeriphSnapshot]
}

// watch arms observation and returns an idempotent release. Watchers are
// counted, so one stream closing does not disarm another; the last one out
// drops the snapshot so a stale frame is not mistaken for a live one.
func (o *periphObserver) watch() (release func()) {
	o.watchers.Add(1)
	var once sync.Once
	return func() {
		once.Do(func() {
			if o.watchers.Add(-1) <= 0 {
				o.snap.Store(nil)
			}
		})
	}
}

func (o *periphObserver) watched() bool { return o.watchers.Load() > 0 }

// note records one event while watched and drops it otherwise.
func (o *periphObserver) note(ev PeriphInput) {
	if !o.watched() {
		return
	}
	o.inputs.note(ev)
}

func (o *periphObserver) publish(s PeriphSnapshot) { o.snap.Store(&s) }

// snapshot is the latest published snapshot, or the zero value (Watching
// false) before the first tick after arming.
func (o *periphObserver) snapshot() PeriphSnapshot {
	if p := o.snap.Load(); p != nil {
		return *p
	}
	return PeriphSnapshot{}
}

// WatchPeriph arms peripheral observation and returns an idempotent release. Safe from any goroutine;
// watchers are counted, so one stream closing does not disarm another.
func (rt *Runtime) WatchPeriph() (release func()) { return rt.periph.watch() }

// PeriphWatched reports whether anything is currently observing.
func (rt *Runtime) PeriphWatched() bool { return rt.periph.watched() }

// PeriphSnapshot returns the latest published snapshot, safe from any goroutine; the zero value
// (Watching false) before the first tick after arming.
func (rt *Runtime) PeriphSnapshot() PeriphSnapshot { return rt.periph.snapshot() }

// DrainPeriphInputs returns every observed input event from cursor onwards, the next cursor, and how
// many events the ring overwrote before this call. Pass cursor 0 first. Safe from any goroutine: it only
// reads under the ring's mutex, so like DumpFlight it bypasses the engine loop.
func (rt *Runtime) DrainPeriphInputs(cursor uint64) (events []PeriphInput, next, missed uint64) {
	return rt.periph.inputs.drain(cursor)
}

// notePeriphInput records one observed event, post-remap and pre-dispatch, on the engine goroutine.
// Observe-only: it must not inject, consume or delay input, draw RNG, touch standby or write a
// peripheral, so golden frames never depend on a watcher. Events consumed above this point
// (player-strategy drops, attract's swallowed BACK/green-hold) are not seen.
func (rt *Runtime) notePeriphInput(ev input.Event) {
	if !rt.periph.watched() {
		return
	}
	rt.periph.note(PeriphInput{
		Frame:    rt.frames.ticks,
		AtMs:     float64(rt.Clock.Now().Sub(rt.bootTime).Microseconds()) / 1000,
		Kind:     string(ev.Kind),
		Name:     string(ev.Name),
		Position: string(ev.Position),
		Ctrl:     ev.Ctrl,
		Shift:    ev.Shift,
		Meta:     ev.Meta,
		Source:   string(ev.Source.Type),
		DeviceID: ev.Source.ID,
		Slot:     ev.Source.Slot,
		Seat:     ev.Source.Seat,
	})
}

// publishPeriph builds and atomically publishes the continuous snapshot from the publishStatus defer;
// a no-op when nothing is watching. Runs on the engine goroutine.
func (rt *Runtime) publishPeriph() {
	if !rt.periph.watched() {
		return
	}
	seq, dropped := rt.periph.inputs.stats()
	snap := PeriphSnapshot{
		Watching: true,
		Frame:    rt.frames.ticks,
		Seq:      seq,
		Dropped:  dropped,
		Lights:   rt.hwShadow.lightMasks(),
		Levels:   rt.hwShadow.lampLevels(),
	}
	if st := rt.lightShow; st != nil {
		snap.LightShowActive = true
		snap.LightShowStep = st.idx
		snap.LightShowMask = st.steps[st.idx].Seats
	}
	if m, ok := rt.Hardware.(hardware.MicSource); ok {
		snap.MicInfo = m.MicInfo()
		snap.Mic, snap.MicOK = m.MicLevel()
	}
	rt.periph.publish(snap)
}
