// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"context"

	"github.com/tetratelabs/wazero/api"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/cartridge/rcart"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// Lamp-sequence caps, mirrored in the browser host so both refuse the same
// cartridge.
const (
	maxLightSequences = rcart.MaxMountedLights
	maxLightsPathLen  = 512                      // same bundle path space as sounds and sheets
	maxLampFadeMs     = assetfile.MaxLightFadeMs // a file's fade and a hand fade share one ceiling
	lampLevelMax      = 15                       // hardware.LampLevelABIMax, pinned by a test
	lampTouchAll      = 0x3f                     // hardware.AllLights, pinned by a test
)

// LightsLoopMode is host_lights_play's loop argument: the file's own mode, or
// an override. There is deliberately no forced pingpong, since a pingpong is
// a property of how the frames were drawn.
type LightsLoopMode uint32

const (
	LightsLoopFile  LightsLoopMode = 0 // the file's own loop mode
	LightsLoopOnce  LightsLoopMode = 1 // force a single pass
	LightsLoopCycle LightsLoopMode = 2 // force endless repetition
)

type lightSeq struct {
	path   string
	frames []assetfile.LightFrame
	loop   string
	seats  [2]bool // which seats any frame names
}

// lightPlayback is one seat's running sequence. The order is precomputed so
// every loop mode is one walk; a frame appearing twice holds twice.
type lightPlayback struct {
	seq    *lightSeq
	order  []int // frame indices, one period
	endsAt []int // cumulative ms at which each order entry ends; the last is the period
	once   bool
	// elapsedUs stays in microseconds so per-tick rounding never drifts.
	elapsedUs uint64
	step      int // last resolved index into order, -1 before the first; lamps are written only when it moves
}

type lampHost interface {
	bundleReader
	Lamps
}

// lightsRegistry is one cartridge's sequences and the two seats' playback.
type lightsRegistry struct {
	host    lampHost
	seqs    map[string]*lightSeq
	playing [2]*lightPlayback
}

func newLightsRegistry(host lampHost) *lightsRegistry {
	return &lightsRegistry{host: host, seqs: map[string]*lightSeq{}}
}

func (l *lightsRegistry) loadBundle(paths []string) {
	for _, p := range paths {
		if len(l.seqs) >= maxLightSequences {
			hostWarnf(l.host, "lights %q: refused, already holding %d sequences", p, maxLightSequences)
			continue
		}
		if _, dup := l.seqs[p]; dup {
			continue
		}
		env, ok := readBundleEnvelope(l.host, "lights", p, assetfile.KindLights)
		if !ok {
			continue
		}
		seq := &lightSeq{path: p, frames: env.Frames, loop: env.Loop}
		for _, f := range env.Frames {
			for seat := 0; seat < 2; seat++ {
				if _, named := f.Lamps.Mask(seat); named {
					seq.seats[seat] = true
				}
			}
		}
		l.seqs[p] = seq
	}
}

// play returns 1 when at least one seat now follows the sequence, else warns
// and returns 0. Each seat follows only the frames that name it; a seat
// argument narrows to that share and never moves a p1 cue onto p2.
func (l *lightsRegistry) play(path string, wireSeat int32, loop LightsLoopMode) uint32 {
	seats, ok := seatArg(l.host, "host_lights_play: seat", wireSeat)
	if !ok {
		return 0
	}
	seq, ok := l.seqs[path]
	if !ok {
		hostWarnf(l.host, "host_lights_play: %q is not one of this cartridge's assets.lights sequences", path)
		return 0
	}
	mode := seq.loop
	switch loop {
	case LightsLoopFile:
	case LightsLoopOnce:
		mode = assetfile.LoopOnce
	case LightsLoopCycle:
		mode = assetfile.LoopCycle
	default:
		hostWarnf(l.host, "host_lights_play: loop %d is not 0 (the file's mode), 1 (once) or 2 (cycle)", uint32(loop))
		return 0
	}

	started := false
	for s := 0; s < 2; s++ {
		if !seats.Has(input.SeatOf(s)) || !seq.seats[s] {
			continue
		}
		l.playing[s] = newLightPlayback(seq, mode)
		// Frame 0 lights now, so a cue started in cart_update shows that frame.
		l.resolve(s)
		started = true
	}
	if !started {
		hostWarnf(l.host, "host_lights_play: %q has no frames for the seat(s) asked for (%d)", path, wireSeat)
		return 0
	}
	return 1
}

func newLightPlayback(seq *lightSeq, mode string) *lightPlayback {
	n := len(seq.frames)
	order := make([]int, 0, 2*n)
	for i := 0; i < n; i++ {
		order = append(order, i)
	}
	if mode == assetfile.LoopPingPong {
		// Turnaround frames are not repeated: three frames bounce 0,1,2,1.
		for i := n - 2; i >= 1; i-- {
			order = append(order, i)
		}
	}
	endsAt := make([]int, len(order))
	total := 0
	for i, f := range order {
		total += seq.frames[f].HoldMs
		endsAt[i] = total
	}
	return &lightPlayback{
		seq:    seq,
		order:  order,
		endsAt: endsAt,
		once:   mode == assetfile.LoopOnce,
		step:   -1,
	}
}

// stop leaves the lamps as the sequence last left them. Any explicit lamp
// write stops the addressed seats' sequences, so the cartridge can always
// take the lamps back.
func (l *lightsRegistry) stop(seats input.Seats) {
	for s := range l.playing {
		if seats.Has(input.SeatOf(s)) {
			l.playing[s] = nil
		}
	}
}

// advance runs once per tick before the guest, on the same dt_micros the
// guest receives, never a wall clock or a goroutine, so a replay lights the
// same lamps on the same frames.
func (l *lightsRegistry) advance(dtMicros uint64) {
	for s := 0; s < 2; s++ {
		if p := l.playing[s]; p != nil {
			p.elapsedUs += dtMicros
			l.resolve(s)
		}
	}
}

// resolve is shared by play and advance, so starting and running cannot disagree.
func (l *lightsRegistry) resolve(seat int) {
	p := l.playing[seat]
	if p == nil {
		return
	}
	elapsedMs := int(p.elapsedUs / 1000)
	period := p.endsAt[len(p.endsAt)-1]
	finished := false
	switch {
	case period <= 0:
		// Unreachable through assetfile's 10 ms floor; guards the modulo.
		elapsedMs, finished = 0, true
	case p.once && elapsedMs >= period:
		elapsedMs, finished = period-1, true
	default:
		elapsedMs %= period
	}
	step := 0
	for step < len(p.endsAt)-1 && elapsedMs >= p.endsAt[step] {
		step++
	}
	if step != p.step {
		p.step = step
		frame := p.seq.frames[p.order[step]]
		if mask, named := frame.Lamps.Mask(seat); named {
			l.write(seat, mask, frame.FadeMs)
		}
	}
	if finished {
		l.playing[seat] = nil
	}
}

// write pushes a frame as a whole state, so unlit lamps turn off. A cut stays
// a SetButtonLights call, which the goldens record.
func (l *lightsRegistry) write(seat, mask, fadeMs int) {
	seats := input.SeatsOf(input.SeatOf(seat))
	if fadeMs > 0 {
		l.host.SetLampLevels(seats, lampLevelsOf(mask), lampTouchAll, fadeMs)
		return
	}
	lights := make(map[string]bool, len(buttonOrdinal))
	for i, name := range buttonOrdinal {
		lights[name] = mask&(1<<uint(i)) != 0
	}
	l.host.SetButtonLights(seats, lights, true)
}

// lampLevelsOf is hardware.LampLevelsOf without the hardware types.
func lampLevelsOf(mask int) [6]uint8 {
	var levels [6]uint8
	for i := range levels {
		if mask&(1<<uint(i)) != 0 {
			levels[i] = lampLevelMax
		}
	}
	return levels
}

// unpackLampLevels reads lamp i at bits 4i..4i+3 and ignores bits 24..31, so
// junk lights the same lamps on every host.
func unpackLampLevels(packed uint32) [6]uint8 {
	var levels [6]uint8
	for i := range levels {
		levels[i] = uint8(packed>>(4*uint(i))) & 0xf
	}
	return levels
}

// teardown drops playback and sequences but leaves the lamps, as a plain
// host_set_button_lights is left.
func (l *lightsRegistry) teardown() {
	l.playing[0], l.playing[1] = nil, nil
	l.seqs = map[string]*lightSeq{}
}

// lampImports stops a seat's sequence on every explicit lamp write to it.
type lampImports struct{ *lightsRegistry }

func (l lampImports) setButtonLights(_ context.Context, _ api.Module, player int32, mask uint32, only int32) {
	seats, ok := seatArg(l.host, "host_set_button_lights: player", player)
	if !ok {
		return
	}
	l.stop(seats)
	isOnly := only != 0
	lights := make(map[string]bool, len(buttonOrdinal))
	for i, name := range buttonOrdinal {
		bit := mask&(1<<uint(i)) != 0
		if isOnly {
			lights[name] = bit
		} else if bit {
			lights[name] = true
		}
	}
	l.host.SetButtonLights(seats, lights, isOnly)
}

func (l lampImports) setLights(_ context.Context, _ api.Module, player, on int32) {
	seats, ok := seatArg(l.host, "host_set_lights: player", player)
	if !ok {
		return
	}
	l.stop(seats)
	l.host.SetLights(seats, on != 0)
}

// setLampLevels with an empty touch mask is a full no-op and, alone among
// lamp calls, stops no sequence.
func (l lampImports) setLampLevels(_ context.Context, _ api.Module, player int32, levels, touch, fadeMs uint32) {
	seats, ok := seatArg(l.host, "host_set_lamp_levels: seat", player)
	if !ok {
		return
	}
	touched := touch & lampTouchAll
	if touched == 0 {
		return
	}
	l.stop(seats)
	l.host.SetLampLevels(seats, unpackLampLevels(levels), uint8(touched), int(min(fadeMs, maxLampFadeMs)))
}

func (l lampImports) playSequence(_ context.Context, mod api.Module, pathPtr, pathLen uint32, seat int32, loop uint32) uint32 {
	path, ok := guestString(l.host, mod, "host_lights_play:", pathPtr, pathLen, maxLightsPathLen)
	if !ok {
		return 0
	}
	return l.play(path, seat, LightsLoopMode(loop))
}

func (l lampImports) stopSequence(_ context.Context, _ api.Module, seat int32) {
	if seats, ok := seatArg(l.host, "host_lights_stop: seat", seat); ok {
		l.stop(seats)
	}
}
