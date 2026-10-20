// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import "math"

// Resample converts s from one sample rate to another with a
// Blackman-windowed sinc, low-passed at 92% of the lower Nyquist so
// downsampling does not alias. Equal rates return a copy.
func Resample(s []float64, from, to int) []float64 {
	if from == to {
		return append([]float64(nil), s...)
	}
	return resample(s, float64(from), float64(to), len(s)*to/from)
}

// Varispeed plays s at speed times its own rate, as tape does: 2 is an
// octave up and half as long, 0.5 an octave down and twice as long.
func Varispeed(s []float64, speed float64) []float64 {
	if speed <= 0 || speed == 1 {
		return append([]float64(nil), s...)
	}
	return resample(s, speed, 1, int(float64(len(s))/speed))
}

// resample reads s at every num/den of a sample, n times.
func resample(s []float64, num, den float64, n int) []float64 {
	const halfTaps = 32
	scale := math.Min(1, den/num)
	fc := 0.92 * scale
	w := halfTaps / scale
	out := make([]float64, n)
	for i := range out {
		t := float64(i) * num / den
		lo := int(math.Ceil(t - w))
		hi := int(math.Floor(t + w))
		sum := 0.0
		for k := max(lo, 0); k <= min(hi, len(s)-1); k++ {
			x := t - float64(k)
			sum += s[k] * fc * sinc(fc*x) * blackman(x/w)
		}
		out[i] = sum
	}
	return out
}

func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	return math.Sin(math.Pi*x) / (math.Pi * x)
}

// blackman is the window over u in [-1,1].
func blackman(u float64) float64 {
	return 0.42 + 0.5*math.Cos(math.Pi*u) + 0.08*math.Cos(2*math.Pi*u)
}

// PitchShift moves s by semitones while keeping its length: stretch it
// in time by the pitch ratio without changing pitch, then read the
// stretch back at that ratio, which restores the length and carries the
// pitch with it. A zero shift returns s itself.
//
// Within about ±4 semitones a voice keeps its character; further out its
// formants travel with the pitch (the chipmunk, the giant).
func PitchShift(s []float64, rate int, semitones float64) []float64 {
	if semitones == 0 || len(s) == 0 {
		return s
	}
	r := math.Pow(2, semitones/12)
	return resample(TimeStretch(s, rate, r), r, 1, len(s))
}

// TimeStretch makes s factor times longer at the same pitch by WSOLA:
// Hann frames laid down at a fixed output hop, each read from near where
// the timeline says it should come from, nudged to the offset that best
// continues the previous frame's waveform so no cycle is cut mid-period.
func TimeStretch(s []float64, rate int, factor float64) []float64 {
	n := rate * 30 / 1000 // frame
	hop := n / 2
	tol := rate * 8 / 1000 // search either side of the nominal read point
	win := make([]float64, n)
	for i := range win {
		win[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n))
	}
	outLen := int(float64(len(s))*factor) + n
	out := make([]float64, outLen)
	norm := make([]float64, outLen)
	at := func(i int) float64 {
		if i < 0 || i >= len(s) {
			return 0
		}
		return s[i]
	}
	prev := 0
	for k := 0; k*hop < outLen-n; k++ {
		nominal := int(math.Round(float64(k*hop) / factor))
		pick := nominal
		if k > 0 {
			target := prev + hop
			best := math.Inf(-1)
			for c := nominal - tol; c <= nominal+tol; c++ {
				sum := 0.0
				for j := 0; j < n; j += 2 {
					sum += at(c+j) * at(target+j)
				}
				if sum > best {
					best, pick = sum, c
				}
			}
		}
		for j := 0; j < n; j++ {
			out[k*hop+j] += at(pick+j) * win[j]
			norm[k*hop+j] += win[j]
		}
		prev = pick
	}
	for i := range out {
		if norm[i] > 1e-3 {
			out[i] /= norm[i]
		}
	}
	return out[:int(float64(len(s))*factor)]
}
