// SPDX-License-Identifier: GPL-2.0-or-later

package musiclib

import "math"

// Wave selects a chip-style oscillator for an Instrument.
type Wave int

const (
	// WavePulse is a rectangular pulse wave; Instrument.Duty sets the duty cycle.
	WavePulse Wave = iota
	// WaveTriangle is a piecewise-linear triangle.
	WaveTriangle
	// WaveNoise is a deterministic LFSR noise voice; Note.Midi shifts its brightness.
	WaveNoise
)

// String is the wave's name as a song recipe spells it: "pulse",
// "triangle" or "noise". An out-of-range value has no name.
func (w Wave) String() string { return waveNames[w] }

// ParseWave is the inverse of String; ok is false for any other word.
func ParseWave(s string) (w Wave, ok bool) {
	for k, v := range waveNames {
		if v == s {
			return k, true
		}
	}
	return 0, false
}

var waveNames = map[Wave]string{
	WavePulse:    "pulse",
	WaveTriangle: "triangle",
	WaveNoise:    "noise",
}

// Instrument is one channel's voice: a waveform plus an ADSR envelope
// evaluated per note. Zero-value fields take the defaults noted on each
// field, so Instrument{Wave: WavePulse} is playable.
type Instrument struct {
	Wave Wave
	// Duty is the pulse duty cycle in (0,1); 0 defaults to 0.5. Ignored by triangle and noise.
	Duty float64
	// AttackMs ramps the note 0 to 1; 0 defaults to 2 ms. DecayMs then falls
	// 1 to SustainLevel; SustainLevel 0 defaults to 1, so a plucky voice needs
	// both DecayMs > 0 and SustainLevel < 1 (0.001 for decay to silence).
	AttackMs, DecayMs float64
	SustainLevel      float64
	// ReleaseMs fades SustainLevel to 0 at the note's end; 0 defaults to
	// 10 ms. The release is gated early if needed to finish inside the note.
	ReleaseMs float64
	// NoiseTone biases WaveNoise brightness as a one-pole filter mix in
	// [-1,1]: positive is highpassed, negative lowpassed, 0 white. Note.Midi
	// also scales the LFSR clock, with MIDI 60 at full rate.
	NoiseTone float64
	// Gain scales the whole instrument; 0 defaults to 1.
	Gain float64
	// VibratoHz and VibratoSemis add sinusoidal pitch vibrato; both 0 is none.
	// Phase restarts each note.
	VibratoHz, VibratoSemis float64
	// LowpassHz applies a one-pole lowpass to the voice; 0 is off. Filter
	// state resets each note.
	LowpassHz float64
	// DetuneCents spreads the voice into an equal-weighted three-oscillator
	// unison at pitch, +DetuneCents and -DetuneCents; 0 is a single
	// oscillator. Pulse and triangle only.
	DetuneCents float64
}

// Note is one event on a track: MIDI pitch, start and duration in beats
// (fractional allowed), and velocity in (0,1] where 0 defaults to 1.
type Note struct {
	Midi       int
	Start, Dur float64
	Vel        float64
	// Seed pins the 16-bit LFSR state a WaveNoise note starts from, in
	// 1..65535. 0 derives it from the note's position (see NoiseSeed), so it
	// changes when the note or its track changes index. Pitched waves ignore it.
	Seed int
}

// NoiseSeed is the LFSR state a WaveNoise note starts from: Seed when set,
// otherwise derived from track index ti and note index ni.
func (n Note) NoiseSeed(ti, ni int) uint16 {
	if n.Seed > 0 {
		return uint16(n.Seed)
	}
	// Changing these constants changes every unseeded noise note.
	s := uint16(0xACE1) ^ uint16(ti*0x9E37) ^ uint16(ni*0x79B9)
	if s == 0 {
		return 0xACE1
	}
	return s
}

// Track is one parallel channel: an instrument, an optional mix gain
// (0 defaults to 1), and its notes. Overlapping notes sum.
type Track struct {
	Name  string
	Inst  Instrument
	Gain  float64
	Notes []Note
}

// Echo is a post-mix delay send; the zero value is off. Taps index the loop
// buffer circularly, so the tail wraps into the loop start.
type Echo struct {
	// DelayBeats is the tap spacing in beats; <= 0 disables the echo.
	DelayBeats float64
	// Feedback in [0,1) scales each repeat; Mix in (0,1] is the wet level
	// relative to dry, and Mix <= 0 disables the echo.
	Feedback, Mix float64
}

// Song is a loopable piece: tempo, loop length in beats, parallel tracks and
// an optional Echo. Beats 0 means the last note end across all tracks,
// rounded up to a whole beat. Peak is the level RenderLoop normalizes the
// mix to; 0 means DefaultPeak.
type Song struct {
	BPM    float64
	Beats  float64
	Tracks []Track
	Echo   Echo
	Peak   float64
}

// PeakLevel returns Peak if set, otherwise DefaultPeak.
func (s Song) PeakLevel() float64 {
	if s.Peak > 0 {
		return s.Peak
	}
	return DefaultPeak
}

// LengthBeats returns Beats if set, otherwise the auto length described on Song.
func (s Song) LengthBeats() float64 {
	if s.Beats > 0 {
		return s.Beats
	}
	end := 0.0
	for _, tr := range s.Tracks {
		for _, n := range tr.Notes {
			if e := n.Start + n.Dur; e > end {
				end = e
			}
		}
	}
	return math.Ceil(end - 1e-9)
}

// LengthSeconds returns the loop length in seconds at the song's BPM.
func (s Song) LengthSeconds() float64 {
	return s.LengthBeats() * 60 / s.BPM
}

// NoteCount returns the total number of notes across all tracks.
func (s Song) NoteCount() int {
	n := 0
	for _, tr := range s.Tracks {
		n += len(tr.Notes)
	}
	return n
}
