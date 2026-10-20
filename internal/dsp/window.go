// SPDX-License-Identifier: GPL-2.0-or-later

package dsp

import "math"

// Hann returns a periodic Hann window of length n, for STFT analysis frames.
func Hann(n int) []float64 { return hann(n, n) }

// SymmetricHann returns a Hann window of length n ≥ 2 that is zero at both
// ends, as FIR design and the gesture detector's Goertzel banks expect.
func SymmetricHann(n int) []float64 { return hann(n, n-1) }

func hann(n, period int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(period))
	}
	return w
}
