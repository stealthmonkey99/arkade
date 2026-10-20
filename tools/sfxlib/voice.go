// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import "math"

// Wave selects a Voice's oscillator.
type Wave int

const (
	WaveSine Wave = iota
	WaveSquare
	WaveTriangle
	WaveSaw
	// WaveNoise is white noise; frequency, slide and NoiseMix do not apply.
	WaveNoise
	// WaveWood is a struck bar: the fundamental plus a faster-decaying partial at BarModeRatio.
	WaveWood
)

// waveNames is the spelling of each Wave, in declaration order.
var waveNames = [...]string{"sine", "square", "triangle", "saw", "noise", "wood"}

// String returns the wave's lowercase name.
func (w Wave) String() string {
	if w < 0 || int(w) >= len(waveNames) {
		return "sine"
	}
	return waveNames[w]
}

// GoName returns the Go identifier of the wave, for generators that print source.
func (w Wave) GoName() string {
	switch w {
	case WaveSquare:
		return "sfxlib.WaveSquare"
	case WaveTriangle:
		return "sfxlib.WaveTriangle"
	case WaveSaw:
		return "sfxlib.WaveSaw"
	case WaveNoise:
		return "sfxlib.WaveNoise"
	case WaveWood:
		return "sfxlib.WaveWood"
	default:
		return "sfxlib.WaveSine"
	}
}

// ParseWave maps a wave name to its constant, reporting false for an unknown one.
func ParseWave(s string) (Wave, bool) {
	for i, n := range waveNames {
		if n == s {
			return Wave(i), true
		}
	}
	return WaveSine, false
}

// BarModeRatio is the frequency ratio of the first two bending modes of a
// free-free bar. It separates a wooden hit from a metallic one and must not be
// rounded to a harmonic ratio.
const BarModeRatio = 2.756

// woodPartialGain is the bar-mode partial's level against the fundamental.
const woodPartialGain = 0.42

// voiceFadeS is the unconditional closing fade; filters and echo can leave a
// step at the last sample.
const voiceFadeS = 0.003

// Voice is one one-shot sound: an oscillator with an optional pitch slide,
// an ADSR inside a fixed duration, a noise blend, filters, an echo and
// normalization. Zero values: SlideTo <= 0 is no slide, SlideCurve <= 0 is
// linear, Peak <= 0 skips normalization, and SustainLevel is used as given.
type Voice struct {
	Wave Wave
	// Freq is the starting frequency in Hz; ignored by WaveNoise.
	Freq float64
	// SlideTo is the frequency reached at the end of Duration. The pitch at
	// fraction u is Freq + (SlideTo-Freq)·u^SlideCurve.
	SlideTo, SlideCurve float64
	// Duration is the total length in seconds; non-positive renders nothing.
	Duration float64
	// Attack, Decay and Release are in seconds; sustain fills the rest of
	// Duration, and stages that exceed it are scaled down to fit.
	Attack, Decay, Release float64
	// SustainLevel in [0,1] is the level held after the decay.
	SustainLevel float64
	// NoiseMix in [0,1] crossfades the oscillator toward white noise.
	NoiseMix float64
	// LowpassHz and HighpassHz are one-pole corners applied after the envelope; 0 is off.
	LowpassHz, HighpassHz float64
	// EchoSec, EchoFeedback and EchoMix are the post-filter delay send; repeats
	// are cut off at Duration.
	EchoSec, EchoFeedback, EchoMix float64
	// Peak is the normalization target; <= 0 leaves the mix unnormalized.
	Peak float64
	// Seed seeds the noise.
	Seed uint64
}

// Envelope returns the ADSR the stage lengths and Duration describe, with
// sustain filling the remainder.
func (v Voice) Envelope() ADSR {
	a, d, r := math.Max(0, v.Attack), math.Max(0, v.Decay), math.Max(0, v.Release)
	if sum := a + d + r; sum > v.Duration && sum > 0 {
		scale := v.Duration / sum
		a, d, r = a*scale, d*scale, r*scale
	}
	sus := v.Duration - a - d - r
	if sus < 0 {
		sus = 0
	}
	return ADSR{AttackS: a, DecayS: d, SustainS: sus, ReleaseS: r, SustainLevel: v.SustainLevel}
}

// Render synthesizes the Voice deterministically.
func (v Voice) Render(sampleRate int) []float64 {
	if sampleRate <= 0 || v.Duration <= 0 {
		return nil
	}
	n := int(math.Round(v.Duration * float64(sampleRate)))
	if n <= 0 {
		return nil
	}
	env := v.Envelope()
	rng := NewRNG(v.Seed)
	curve := v.SlideCurve
	if curve <= 0 {
		curve = 1
	}
	noiseMix := clamp01(v.NoiseMix)

	out := make([]float64, n)
	// Both phases advance every sample so the wood partial stays locked through a slide.
	var ph, phPartial float64
	for i := range out {
		t := SampleTime(sampleRate, i)
		e := env.At(t)
		f := v.Freq
		if v.SlideTo > 0 && v.Freq > 0 {
			f = v.Freq + (v.SlideTo-v.Freq)*math.Pow(t/v.Duration, curve)
		}

		var s float64
		switch v.Wave {
		case WaveNoise:
			s = e * NoiseSample(rng)
		case WaveWood:
			// Squaring the envelope makes the upper mode die faster.
			s = e*sinePhase(ph) + woodPartialGain*e*e*sinePhase(phPartial)
			if noiseMix > 0 {
				s = (1-noiseMix)*s + noiseMix*e*NoiseSample(rng)
			}
		default:
			s = e * shapeAt(v.Wave, ph)
			if noiseMix > 0 {
				s = (1-noiseMix)*s + noiseMix*e*NoiseSample(rng)
			}
		}
		out[i] = s

		ph = advance(ph, f, sampleRate)
		phPartial = advance(phPartial, f*BarModeRatio, sampleRate)
	}

	Lowpass(out, sampleRate, v.LowpassHz)
	Highpass(out, sampleRate, v.HighpassHz)
	Echo(out, sampleRate, v.EchoSec, v.EchoFeedback, v.EchoMix)
	FadeOut(out, sampleRate, voiceFadeS)
	if v.Peak > 0 {
		Normalize(out, v.Peak)
	}
	return out
}

// advance moves a normalized phase forward one sample, folded into [0,1).
func advance(ph, freq float64, sampleRate int) float64 {
	ph += freq / float64(sampleRate)
	if ph >= 1 || ph < 0 {
		ph -= math.Floor(ph)
	}
	return ph
}

// shapeAt evaluates a unit oscillator at normalized phase ph in [0,1).
func shapeAt(w Wave, ph float64) float64 {
	switch w {
	case WaveSquare:
		if ph < 0.5 {
			return 1
		}
		return -1
	case WaveTriangle:
		switch {
		case ph < 0.25:
			return 4 * ph
		case ph < 0.75:
			return 2 - 4*ph
		default:
			return 4*ph - 4
		}
	case WaveSaw:
		return 2*ph - 1
	default:
		return sinePhase(ph)
	}
}

func sinePhase(ph float64) float64 { return math.Sin(2 * math.Pi * ph) }

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
