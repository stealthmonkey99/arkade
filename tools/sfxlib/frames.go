// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import "math"

// DefaultMaxFrames is the frame budget FrameSignal uses when maxFrames <= 0.
const DefaultMaxFrames = 16

// minFrameLen keeps a very low sample rate from yielding a meaningless
// Goertzel window.
const minFrameLen = 16

// Bin is one analysis bin: its centre frequency and the summed Goertzel
// power across every frame of the signal.
type Bin struct {
	Hz    float64
	Power float64
}

// FrameSet is a signal prepared for band-energy analysis: Hann-windowed
// frames sized so the analysis step is one DFT bin. Whole-signal Goertzel
// sampling on a 20 Hz grid steps over the peaks of a signal seconds long,
// so anything longer than a short transient should be framed. Build one
// with FrameSignal.
type FrameSet struct {
	frames [][]float64
	rate   float64
	step   float64
}

// FrameSignal splits samples into at most maxFrames (0 takes
// DefaultMaxFrames) Hann-windowed frames of sampleRate/stepHz samples,
// spread evenly across the signal. stepHz <= 0 takes DefaultBandStep. A
// signal shorter than one frame is analysed whole.
func FrameSignal(samples []float64, sampleRate int, stepHz float64, maxFrames int) FrameSet {
	if stepHz <= 0 {
		stepHz = DefaultBandStep
	}
	if maxFrames <= 0 {
		maxFrames = DefaultMaxFrames
	}
	fs := FrameSet{rate: float64(sampleRate), step: stepHz}
	if len(samples) == 0 || sampleRate <= 0 {
		return fs
	}
	n := int(math.Round(float64(sampleRate) / stepHz))
	if n < minFrameLen {
		n = minFrameLen
	}
	if len(samples) <= n {
		fs.frames = [][]float64{Hann(samples)}
		return fs
	}
	count := (len(samples) + n - 1) / n
	if count > maxFrames {
		count = maxFrames
	}
	fs.frames = make([][]float64, 0, count)
	span := len(samples) - n
	for i := 0; i < count; i++ {
		start := 0
		if count > 1 {
			start = i * span / (count - 1)
		}
		fs.frames = append(fs.frames, Hann(samples[start:start+n]))
	}
	return fs
}

// Count returns how many frames the analysis is spread across.
func (fs FrameSet) Count() int { return len(fs.frames) }

// StepHz returns the analysis bin spacing the frames were sized for.
func (fs FrameSet) StepHz() float64 { return fs.step }

// Bins returns one Bin per analysis frequency from lo to hi Hz inclusive,
// StepHz apart, each holding the power summed across frames. Frequencies at
// or above Nyquist are skipped. Callers needing several figures should sum
// these once rather than call BandEnergy repeatedly.
func (fs FrameSet) Bins(lo, hi float64) []Bin {
	if len(fs.frames) == 0 || fs.step <= 0 {
		return nil
	}
	nyquist := fs.rate / 2
	var out []Bin
	for f := lo; f <= hi && f < nyquist; f += fs.step {
		p := 0.0
		for _, fr := range fs.frames {
			p += GoertzelPower(fr, fs.rate, f)
		}
		out = append(out, Bin{Hz: f, Power: p})
	}
	return out
}

// BandEnergy sums the framed power over [lo,hi] Hz.
func (fs FrameSet) BandEnergy(lo, hi float64) float64 {
	e := 0.0
	for _, b := range fs.Bins(lo, hi) {
		e += b.Power
	}
	return e
}

// BandFraction returns the share of energy in [lo,hi] Hz relative to the
// total over [allLo,allHi] Hz, or 0 if that range holds none.
func (fs FrameSet) BandFraction(lo, hi, allLo, allHi float64) float64 {
	all := fs.BandEnergy(allLo, allHi)
	if all == 0 {
		return 0
	}
	return fs.BandEnergy(lo, hi) / all
}

// AssertSpectrum is the framed counterpart of the package-level
// AssertSpectrum, with the same checks and error text. Use it for sounds
// longer than a few tens of milliseconds.
func (fs FrameSet) AssertSpectrum(allLo, allHi float64, checks []SpectrumCheck) error {
	all := fs.BandEnergy(allLo, allHi)
	fracs := make([]float64, len(checks))
	for i, c := range checks {
		if all != 0 {
			fracs[i] = fs.BandEnergy(c.Lo, c.Hi) / all
		}
	}
	return checkFractions(fracs, checks)
}

// Hann returns a Hann-tapered copy of s.
func Hann(s []float64) []float64 {
	out := make([]float64, len(s))
	if len(s) < 2 {
		copy(out, s)
		return out
	}
	for i, v := range s {
		out[i] = v * 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(len(s)-1)))
	}
	return out
}
