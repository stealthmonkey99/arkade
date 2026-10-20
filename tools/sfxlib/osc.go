// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import "math"

// SampleTime returns the time in seconds of sample index i.
func SampleTime(sampleRate, i int) float64 {
	return float64(i) / float64(sampleRate)
}

// MsToSamples converts milliseconds to a sample count.
func MsToSamples(sampleRate, ms int) int {
	return sampleRate * ms / 1000
}

// Sine returns a unit-amplitude sine at time t seconds and freq Hz.
func Sine(t, freq float64) float64 {
	return math.Sin(2 * math.Pi * freq * t)
}

// Square returns a unit-amplitude square wave, the sign of Sine.
func Square(t, freq float64) float64 {
	if math.Sin(2*math.Pi*freq*t) >= 0 {
		return 1
	}
	return -1
}

// Triangle returns a unit-amplitude triangle wave, phase-aligned with Sine.
func Triangle(t, freq float64) float64 {
	return (2 / math.Pi) * math.Asin(math.Sin(2*math.Pi*freq*t))
}

// Saw returns a unit-amplitude rising sawtooth wave.
func Saw(t, freq float64) float64 {
	ph := phase01(t, freq)
	return 2*ph - 1
}

// phase01 folds freq*t into [0,1).
func phase01(t, freq float64) float64 {
	ph := math.Mod(t*freq, 1)
	if ph < 0 {
		ph += 1
	}
	return ph
}

// AccumPhase integrates a per-sample frequency into a running phase in
// cycles, not folded into [0,1). A swept oscillator needs this; freq*t with
// a changing freq tears the waveform. The division is applied to the running
// sum to keep rounding error down.
func AccumPhase(freqHz []float64, sampleRate int) []float64 {
	out := make([]float64, len(freqHz))
	if sampleRate <= 0 {
		return out
	}
	acc := 0.0
	for i, f := range freqHz {
		acc += f
		out[i] = acc / float64(sampleRate)
	}
	return out
}

// Glottal returns a sawtooth at the given phase (see AccumPhase) through a
// one-pole lowpass at cutoffHz, as a new slice. The saw supplies the
// harmonics a formant filter needs.
func Glottal(phase []float64, sampleRate int, cutoffHz float64) []float64 {
	out := make([]float64, len(phase))
	for i, ph := range phase {
		p := math.Mod(ph, 1)
		if p < 0 {
			p += 1
		}
		out[i] = 2*p - 1
	}
	Lowpass(out, sampleRate, cutoffHz)
	return out
}
