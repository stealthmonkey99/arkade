// SPDX-License-Identifier: GPL-2.0-or-later

// Package mic is the cabinet's microphone: capture, echo cancellation
// against the mixer's own output, and recognition that reduces audio to a
// handful of discrete events: a gesture (whistle by default; clap and blow
// when enabled) from Goertzel filter-bank thresholds, or a cue from a
// cartridge's trained sound models.
//
// Events travel as ordinary input records; a cartridge never sees a sample,
// which would break golden frames and replay-validated scores.
//
// The Detector is a pure function of its samples and the Suppress and
// SetTaint calls between them, so a captured WAV replays bit-identically.
package mic

import (
	"fmt"
	"math"

	"github.com/stealthmonkey99/arkade/internal/dsp"
)

// Event is one recognised acoustic gesture.
type Event uint8

// The recognised gestures.
const (
	EventNone Event = iota
	EventWhistle
	EventClap
	EventBlow
)

func (e Event) String() string {
	switch e {
	case EventWhistle:
		return "whistle"
	case EventClap:
		return "clap"
	case EventBlow:
		return "blow"
	}
	return "none"
}

// Config holds every tunable. Zero values are not useful; start from
// DefaultConfig and override.
type Config struct {
	SampleRate int // Rate: the decimated analysis rate
	FrameLen   int // samples per analysis frame; 256 @ 14.7 kHz = 17 ms

	// MinRMS is the absolute floor an event must clear, in 0..1 full-scale.
	// The cabinet's silent baseline sits near 0.004.
	MinRMS float64
	// SNR is how far above the tracked noise floor a sustained event must sit.
	SNR float64

	// WhistleMinHz and WhistleMaxHz bound the search; human whistles sit
	// around 1-3 kHz.
	WhistleMinHz float64
	WhistleMaxHz float64
	// Tonality is the share of band energy the peak bin (plus its two
	// neighbours) must hold. A whistle scores high; chip music, speech and
	// clapping score low.
	Tonality float64
	// PitchDriftBins is how far the peak may move between frames and still
	// count as the same whistle.
	PitchDriftBins int
	// HoldFrames is how many consecutive qualifying frames a sustained event
	// (whistle, blow) needs before it fires; at 17 ms/frame, 4 is 70 ms.
	HoldFrames int

	// ClapAttack is how many times the short-term RMS must exceed the recent
	// average to count as a transient.
	ClapAttack float64
	// ClapMaxTonality is the tonality ceiling for a clap or a blow, keeping a
	// whistle's onset from registering as either.
	ClapMaxTonality float64

	// BlowRatio is how far low-band energy (125-500 Hz) must dominate the
	// whistle band, per bin. Flat broadband sits near 1.0.
	BlowRatio float64

	// RefractoryMS is the per-event dead time.
	RefractoryMS int

	// EnableClap and EnableBlow emit those gestures. Both default to false:
	// on the cabinet the clap detector fired on joystick clicks and game
	// sound effects, and the thresholds are untuned without a labelled
	// recording of a real room.
	EnableClap bool
	EnableBlow bool
}

// DefaultConfig is tuned against the cabinet's measured silent baseline
// (rms ~0.004) and an active room (peak ~0.36).
func DefaultConfig() Config {
	return Config{
		SampleRate:      Rate,
		FrameLen:        256,
		MinRMS:          0.02,
		SNR:             4.0,
		WhistleMinHz:    700,
		WhistleMaxHz:    4000,
		Tonality:        0.55,
		PitchDriftBins:  2,
		HoldFrames:      4,
		ClapAttack:      8.0,
		ClapMaxTonality: 0.40,
		BlowRatio:       3.0,
		RefractoryMS:    400,
	}
}

// silentFloor is the cabinet's measured silent baseline, the noise floor's start.
const silentFloor = 0.004

// Frame is everything the detector learned from one analysis frame.
type Frame struct {
	// Index counts frames since Reset; multiply by FrameLen/SampleRate for a
	// timestamp.
	Index int

	RMS   float64 // 0..1 full-scale
	Floor float64 // the tracked noise floor, same units

	PeakHz   float64 // dominant frequency in the whistle band
	Tonality float64 // 0..1, share of band energy at the peak
	LowRatio float64 // low-band energy per bin / whistle-band energy per bin

	// Event is EventNone except on the frame an event actually fires.
	Event Event
	// PitchHz is the whistle's pitch when Event is EventWhistle.
	PitchHz float64
}

// bank is a Goertzel filter bank over a contiguous run of DFT bins.
type bank struct {
	k0    int // first DFT bin index
	coeff []float64
	pow   []float64
}

func newBank(k0, k1, n int) *bank {
	b := &bank{k0: k0, coeff: make([]float64, k1-k0+1), pow: make([]float64, k1-k0+1)}
	for i := range b.coeff {
		b.coeff[i] = 2 * math.Cos(2*math.Pi*float64(k0+i)/float64(n))
	}
	return b
}

// bankLayout is the bin ranges a Config's frames are analysed over: a low
// bank for breath and a tone bank spanning the whistle range with margin.
// The echo gate shares it so its taint mask indexes the tone bins one to one.
type bankLayout struct {
	n              int
	lowK0, lowK1   int // 125-500 Hz
	toneK0, toneK1 int // WhistleMinHz·0.9 to WhistleMaxHz·1.05
}

func newBankLayout(cfg Config) bankLayout {
	n := cfg.FrameLen
	binHz := float64(cfg.SampleRate) / float64(n)
	return bankLayout{
		n:      n,
		lowK0:  clampBin(125/binHz, n),
		lowK1:  clampBin(500/binHz, n),
		toneK0: clampBin(cfg.WhistleMinHz*0.9/binHz, n),
		toneK1: clampBin(cfg.WhistleMaxHz*1.05/binHz, n),
	}
}

func (l bankLayout) banks() (low, tone *bank) {
	return newBank(l.lowK0, l.lowK1, l.n), newBank(l.toneK0, l.toneK1, l.n)
}

func (l bankLayout) toneBins() int { return l.toneK1 - l.toneK0 + 1 }

// run fills b.pow from windowed samples and returns the total power.
func (b *bank) run(win []float64) float64 {
	total := 0.0
	for i, c := range b.coeff {
		var s1, s2 float64
		for _, x := range win {
			s0 := x + c*s1 - s2
			s2, s1 = s1, s0
		}
		p := max(s1*s1+s2*s2-c*s1*s2, 0) // rounding can dip a silent bin below zero
		b.pow[i] = p
		total += p
	}
	return total
}

// Detector is the streaming state machine. Feed it samples with Write; it
// calls back once per completed frame. Not safe for concurrent use.
type Detector struct {
	cfg Config

	window []float64 // precomputed Hann
	buf    []float64 // windowed frame, reused
	pend   []float64 // partial frame carried between Writes

	layout bankLayout
	low    *bank // for blow
	tone   *bank // for whistle

	frame      int
	floor      float64
	suppressed bool
	taint      []bool // tone-bank bins the echo gate has marked as the cabinet's own playback, or nil

	avgRMS  float64 // slow average, for clap attack ratio
	primed  bool
	lastBin int
	held    int
	blowHed int

	// latched marks a sustained event that has fired and must see its
	// condition break before it can fire again, so one whistle is one event.
	latched map[Event]bool

	refractory map[Event]int // frame index each event becomes eligible again
}

// NewDetector builds a detector. FrameLen and SampleRate are fixed for its
// lifetime.
func NewDetector(cfg Config) *Detector {
	n := cfg.FrameLen
	d := &Detector{
		cfg:        cfg,
		layout:     newBankLayout(cfg),
		window:     dsp.SymmetricHann(n),
		buf:        make([]float64, n),
		pend:       make([]float64, 0, n),
		floor:      silentFloor,
		lastBin:    -1,
		latched:    map[Event]bool{},
		refractory: map[Event]int{},
	}
	d.low, d.tone = d.layout.banks()
	return d
}

func clampBin(f float64, n int) int {
	return min(max(int(math.Round(f)), 1), n/2-1)
}

// Reset returns the detector to its initial state, except that the taint mask
// set by SetTaint is kept.
func (d *Detector) Reset() {
	d.frame, d.floor, d.avgRMS = 0, silentFloor, 0
	d.primed, d.lastBin, d.held, d.blowHed = false, -1, 0, 0
	d.suppressed = false
	d.pend = d.pend[:0]
	for k := range d.refractory {
		delete(d.refractory, k)
	}
	for k := range d.latched {
		delete(d.latched, k)
	}
}

// Suppress stops the detector emitting events while on, without stopping the
// analysis: the noise floor keeps tracking, so listening resumes calibrated.
// A synthesised whistle scores 1.00 on tonality where a person scores 0.94,
// so no threshold separates the cabinet's own sounds from a player's.
// Sustained-gesture progress is discarded on every change, so a tone spanning
// the boundary must re-qualify.
func (d *Detector) Suppress(on bool) {
	if on != d.suppressed {
		d.held, d.blowHed, d.lastBin = 0, 0, -1
		for k := range d.latched {
			d.latched[k] = false
		}
	}
	d.suppressed = on
}

// SetTaint marks tone-bank bins as carrying the cabinet's own playback (one
// entry per bin, same order as the bank; nil clears). A whistle whose peak
// lands in a marked bin is not counted. A mask of the wrong length clears the
// taint and returns an error.
func (d *Detector) SetTaint(mask []bool) error {
	if mask != nil && len(mask) != d.layout.toneBins() {
		d.taint = nil
		return fmt.Errorf("mic: taint mask has %d bins, the tone bank %d", len(mask), d.layout.toneBins())
	}
	d.taint = mask
	return nil
}

// Write feeds signed 16-bit mono samples, calling fn once per completed
// analysis frame. Leftover samples are carried to the next Write, so the
// caller's read size does not affect the result.
func (d *Detector) Write(samples []int16, fn func(Frame)) {
	for _, s := range samples {
		d.pend = append(d.pend, float64(s)/32768)
		if len(d.pend) == d.cfg.FrameLen {
			fn(d.analyze(d.pend))
			d.pend = d.pend[:0]
		}
	}
}

// WriteFloat is Write over samples already scaled to -1..1.
func (d *Detector) WriteFloat(samples []float64, fn func(Frame)) {
	for _, s := range samples {
		d.pend = append(d.pend, s)
		if len(d.pend) == d.cfg.FrameLen {
			fn(d.analyze(d.pend))
			d.pend = d.pend[:0]
		}
	}
}

func (d *Detector) analyze(raw []float64) Frame {
	n := d.cfg.FrameLen

	sum := 0.0
	for i, x := range raw {
		sum += x * x
		d.buf[i] = x * d.window[i]
	}
	rms := math.Sqrt(sum / float64(n))

	toneTotal := d.tone.run(d.buf)
	lowTotal := d.low.run(d.buf)

	// Hann spreads a pure tone across three bins, so tonality sums peak±1.
	peak, peakI := 0.0, 0
	for i, p := range d.tone.pow {
		if p > peak {
			peak, peakI = p, i
		}
	}
	lobe := peak
	if peakI > 0 {
		lobe += d.tone.pow[peakI-1]
	}
	if peakI < len(d.tone.pow)-1 {
		lobe += d.tone.pow[peakI+1]
	}
	tonality := 0.0
	if toneTotal > 0 {
		tonality = lobe / toneTotal
	}
	// Per-bin averages: the banks span 8 and 63 bins at the defaults, so
	// totals would measure bandwidth, not spectral tilt.
	lowRatio := 0.0
	if toneTotal > 0 {
		lowRatio = (lowTotal / float64(len(d.low.pow))) / (toneTotal / float64(len(d.tone.pow)))
	}
	binHz := float64(d.cfg.SampleRate) / float64(n)
	peakHz := float64(d.tone.k0+peakI) * binHz

	f := Frame{
		Index: d.frame, RMS: rms, Floor: d.floor,
		PeakHz: peakHz, Tonality: tonality, LowRatio: lowRatio,
	}

	loud := !d.suppressed && rms > d.cfg.MinRMS && rms > d.floor*d.cfg.SNR

	// Clap first: it is judged against the previous average, before this
	// frame is absorbed into it.
	if d.cfg.EnableClap && d.primed && loud && rms > d.avgRMS*d.cfg.ClapAttack && tonality < d.cfg.ClapMaxTonality {
		if d.fire(EventClap) {
			f.Event = EventClap
		}
	}

	inBand := peakHz >= d.cfg.WhistleMinHz && peakHz <= d.cfg.WhistleMaxHz
	ours := d.taint != nil && d.taint[peakI]
	if loud && inBand && !ours && tonality >= d.cfg.Tonality {
		bin := d.tone.k0 + peakI
		if d.lastBin >= 0 && abs(bin-d.lastBin) <= d.cfg.PitchDriftBins {
			d.held++
		} else {
			d.held = 1
		}
		d.lastBin = bin
		if d.held >= d.cfg.HoldFrames && f.Event == EventNone && !d.latched[EventWhistle] {
			if d.fire(EventWhistle) {
				f.Event, f.PitchHz = EventWhistle, peakHz
				d.latched[EventWhistle] = true
			}
		}
	} else {
		d.held, d.lastBin = 0, -1
		d.latched[EventWhistle] = false
	}

	if d.cfg.EnableBlow && loud && lowRatio >= d.cfg.BlowRatio && tonality < d.cfg.ClapMaxTonality {
		d.blowHed++
		if d.blowHed >= d.cfg.HoldFrames && f.Event == EventNone && !d.latched[EventBlow] {
			if d.fire(EventBlow) {
				f.Event = EventBlow
				d.latched[EventBlow] = true
			}
		}
	} else {
		d.blowHed = 0
		d.latched[EventBlow] = false
	}

	// Floor falls fast and rises slowly so a long loud passage cannot deafen
	// the detector.
	if rms < d.floor {
		d.floor += (rms - d.floor) * 0.25
	} else {
		d.floor += (rms - d.floor) * 0.002
	}
	if d.primed {
		d.avgRMS += (rms - d.avgRMS) * 0.12
	} else {
		d.avgRMS, d.primed = rms, true
	}

	d.frame++
	return f
}

// fire reports whether an event is out of its refractory period, arming the
// next one when it is.
func (d *Detector) fire(e Event) bool {
	if d.frame < d.refractory[e] {
		return false
	}
	frames := d.cfg.RefractoryMS * d.cfg.SampleRate / 1000 / d.cfg.FrameLen
	d.refractory[e] = d.frame + frames
	return true
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
