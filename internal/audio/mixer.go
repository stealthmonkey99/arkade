// SPDX-License-Identifier: GPL-2.0-or-later

package audio

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
	"github.com/gopxl/beep/v2/wav"
)

// Mixer is the native audio backend: WAVs decode once into RAM and play
// through a beep mixer. It is device-free; audio/device connects Reader() to
// an output.
type Mixer struct {
	mu       sync.Mutex
	rate     beep.SampleRate
	mix      *beep.Mixer
	vol      *effects.Volume
	resolver Resolver
	lister   Lister
	logger   func(level, msg string)

	cache   map[string]cachedSound
	missing map[string]bool
	active  []*handle

	volPct int
	muted  bool

	tap func(frames [][2]float64)
}

type cachedSound struct {
	buf    *beep.Buffer
	format beep.Format
}

// handle is a stoppable streamer registered in the mixer.
type handle struct {
	owner    string
	label    string
	src      beep.Streamer
	stopped  bool
	finished bool
}

func (h *handle) Stream(samples [][2]float64) (int, bool) {
	if h.stopped {
		h.finished = true
		return 0, false
	}
	n, ok := h.src.Stream(samples)
	// beep's mixer drops a short read without a final call, so note it here.
	if !ok || n < len(samples) {
		h.finished = true
	}
	return n, ok
}

func (h *handle) Err() error { return nil }

// MixRate is the output sample rate; sources at other rates resample.
const MixRate = beep.SampleRate(44100)

// deviceBuffer is the target device latency; raise to 40ms if a platform underruns.
const deviceBuffer = 20 * time.Millisecond

// NewMixer builds the mixing core. The lister is optional; PreloadOwner needs it.
func NewMixer(resolver Resolver, lister Lister, logger func(level, msg string)) *Mixer {
	m := newCore(resolver, logger)
	m.lister = lister
	return m
}

// PreloadOwner decodes all of an owner's sounds into the cache. Decoding runs
// outside the mutex so it never blocks the device pull.
func (m *Mixer) PreloadOwner(owner string) {
	if m.lister == nil {
		return
	}
	for _, file := range m.lister(owner) {
		m.preload(owner, file)
	}
}

func (m *Mixer) preload(owner, file string) {
	key := owner + "\x00" + file
	m.mu.Lock()
	_, cached := m.cache[key]
	skip := cached || m.missing[key]
	m.mu.Unlock()
	if skip {
		return
	}
	sound, ok := m.decode(owner, file)
	m.mu.Lock()
	if ok {
		m.cache[key] = sound
	} else {
		m.missing[key] = true
	}
	m.mu.Unlock()
}

// decode reads and decodes a WAV without touching mixer state (no lock).
func (m *Mixer) decode(owner, file string) (cachedSound, bool) {
	rc, err := m.resolver(owner, file)
	if err != nil {
		m.logger("warn", fmt.Sprintf("audio: %s: %v (sound disabled)", owner, err))
		return cachedSound{}, false
	}
	defer rc.Close()
	sound, err := decodeWAV(rc)
	if err != nil {
		m.logger("warn", fmt.Sprintf("audio: %s %s: decode: %v", owner, file, err))
		return cachedSound{}, false
	}
	return sound, true
}

func decodeWAV(r io.Reader) (cachedSound, error) {
	streamer, format, err := wav.Decode(r)
	if err != nil {
		return cachedSound{}, err
	}
	buf := beep.NewBuffer(format)
	buf.Append(streamer)
	return cachedSound{buf: buf, format: format}, nil
}

// LoadBytes decodes data as a WAV and caches it under owner/file, so a later
// Play finds it without calling the resolver.
func (m *Mixer) LoadBytes(owner, file string, data []byte) error {
	sound, err := decodeWAV(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("audio: %s %s: decode: %w", owner, file, err)
	}
	key := owner + "\x00" + file
	m.mu.Lock()
	m.cache[key] = sound
	delete(m.missing, key)
	m.mu.Unlock()
	return nil
}

func newCore(resolver Resolver, logger func(level, msg string)) *Mixer {
	if logger == nil {
		logger = func(string, string) {}
	}
	mix := &beep.Mixer{}
	// Stream silence when idle so the device player never sees a drained source.
	mix.KeepAlive(true)
	m := &Mixer{
		rate:     MixRate,
		mix:      mix,
		vol:      &effects.Volume{Streamer: mix, Base: 2, Volume: 0},
		resolver: resolver,
		logger:   logger,
		cache:    map[string]cachedSound{},
		missing:  map[string]bool{},
		volPct:   100,
	}
	return m
}

// Reader exposes the mixed output as interleaved stereo float32 LE PCM at
// SampleRate().
func (m *Mixer) Reader() io.Reader {
	return &pcmReader{mu: &m.mu, src: m.vol, tap: m.outputTap}
}

// SetOutputTap registers fn to receive every stereo frame the device pulls,
// after volume, on the device's pull goroutine. It is the reference signal for
// echo cancellation. fn must not block; nil removes the tap.
func (m *Mixer) SetOutputTap(fn func(frames [][2]float64)) {
	m.mu.Lock()
	m.tap = fn
	m.mu.Unlock()
}

func (m *Mixer) outputTap() func(frames [][2]float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tap
}

// SampleRate returns the output rate for device configuration.
func (m *Mixer) SampleRate() int { return int(m.rate) }

// DeviceBuffer returns the recommended device buffer duration.
func (m *Mixer) DeviceBuffer() time.Duration { return deviceBuffer }

// load returns a cached sound, decoding lazily. Callers hold m.mu, so a lazy
// decode blocks the audio pull.
func (m *Mixer) load(owner, file string) (cachedSound, bool) {
	key := owner + "\x00" + file
	if s, ok := m.cache[key]; ok {
		return s, true
	}
	if m.missing[key] {
		return cachedSound{}, false
	}
	sound, ok := m.decode(owner, file)
	if !ok {
		m.missing[key] = true
		return cachedSound{}, false
	}
	m.cache[key] = sound
	return sound, true
}

// Play starts a sound. loop: 0 = once, n>0 = n times, LoopForever = until
// stopped.
func (m *Mixer) Play(owner, label, file string, loop int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sound, ok := m.load(owner, file)
	if !ok {
		return
	}

	var s beep.Streamer = sound.buf.Streamer(0, sound.buf.Len())
	if loop != 0 {
		s = beep.Loop(loop, s.(beep.StreamSeeker))
	}
	if sound.format.SampleRate != m.rate {
		s = beep.Resample(3, sound.format.SampleRate, m.rate, s)
	}
	m.addHandle(owner, label, s)
}

// PlayBytes decodes wav and plays it once, attenuated by gain ahead of the
// master volume. It never caches.
func (m *Mixer) PlayBytes(owner, label string, wav []byte, gain float64) error {
	sound, err := decodeWAV(bytes.NewReader(wav))
	if err != nil {
		return fmt.Errorf("audio: %s %s: decode: %w", owner, label, err)
	}
	gain = max(0, min(1, gain))

	var s beep.Streamer = sound.buf.Streamer(0, sound.buf.Len())
	if sound.format.SampleRate != m.rate {
		s = beep.Resample(3, sound.format.SampleRate, m.rate, s)
	}
	att := &effects.Volume{Streamer: s, Base: 2, Silent: gain <= 0}
	if gain > 0 {
		att.Volume = math.Log2(gain)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.addHandle(owner, label, att)
	return nil
}

// addHandle registers a stoppable sound. Callers hold m.mu.
func (m *Mixer) addHandle(owner, label string, s beep.Streamer) {
	h := &handle{owner: owner, label: label, src: s}
	m.prune()
	m.active = append(m.active, h)
	m.mix.Add(h)
}

// Stop stops sounds for owner; empty label stops all of them.
func (m *Mixer) Stop(owner, label string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, h := range m.active {
		if h.owner == owner && (label == "" || h.label == label) {
			h.stopped = true
		}
	}
	m.prune()
}

// prune drops finished or stopped handles. Callers hold m.mu.
func (m *Mixer) prune() {
	keep := m.active[:0]
	for _, h := range m.active {
		if !h.finished && !h.stopped {
			keep = append(keep, h)
		}
	}
	m.active = keep
}

// Playing reports whether any sound for owner (and label, if non-empty) is
// still registered and un-stopped.
func (m *Mixer) Playing(owner, label string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, h := range m.active {
		if h.finished || h.stopped {
			continue
		}
		if h.owner == owner && (label == "" || h.label == label) {
			return true
		}
	}
	return false
}

// VolumeControl is the optional volume interface backends may implement.
type VolumeControl interface {
	// SetVolumePercent sets output volume 0–100.
	SetVolumePercent(pct int)
	// SetMuted mutes/unmutes without losing the volume setting.
	SetMuted(muted bool)
}

// SetVolumePercent sets output volume 0-100, clamped.
func (m *Mixer) SetVolumePercent(pct int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.volPct = max(0, min(100, pct))
	m.applyVolume()
}

// SetMuted mutes/unmutes playback.
func (m *Mixer) SetMuted(muted bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.muted = muted
	m.applyVolume()
}

// applyVolume sets a linear pct/100 gain, as a log2 exponent. Callers hold m.mu.
func (m *Mixer) applyVolume() {
	m.vol.Silent = m.muted || m.volPct <= 0
	if m.volPct > 0 {
		m.vol.Volume = math.Log2(float64(m.volPct) / 100)
	}
}

// readPCM pulls frames through the volume and mixer chain the device reads. Test hook.
func (m *Mixer) readPCM(frames int) [][2]float64 {
	out := make([][2]float64, frames)
	done := 0
	for done < frames {
		m.mu.Lock()
		n, _ := m.vol.Stream(out[done:])
		m.mu.Unlock()
		if n == 0 {
			break
		}
		done += n
	}
	return out
}

var _ Service = (*Mixer)(nil)
var _ VolumeControl = (*Mixer)(nil)
