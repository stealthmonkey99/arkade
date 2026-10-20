// SPDX-License-Identifier: GPL-2.0-or-later

package dsp

import "math"

// DelayEstimate returns the lag in [0, maxLag] at which mic best matches ref
// delayed by lag (mic[t] ≈ g·ref[t-lag]), and the normalized cross-correlation
// score at that lag in 0..1 (1 = perfect scaled copy). ref and mic must be the
// same length. Returns (0, 0) for silent or degenerate input.
func DelayEstimate(ref, mic []float64, maxLag int) (lag int, score float64) {
	n := len(ref)
	if n == 0 || n != len(mic) || maxLag < 0 {
		return 0, 0
	}

	m := nextPow2(n + maxLag)
	f := NewFFT(m)

	refRe, refIm := make([]float64, m), make([]float64, m)
	micRe, micIm := make([]float64, m), make([]float64, m)
	copy(refRe, ref)
	copy(micRe, mic)
	f.Forward(refRe, refIm)
	f.Forward(micRe, micIm)

	// m pads past len(ref)+maxLag, so the circular correlation is linear for lag in [0, maxLag].
	for i := 0; i < m; i++ {
		mr, mi := micRe[i], micIm[i]
		rr, ri := refRe[i], -refIm[i]
		micRe[i] = mr*rr - mi*ri
		micIm[i] = mr*ri + mi*rr
	}
	f.Inverse(micRe, micIm)

	// Prefix sums make each lag's normalization O(1).
	refPrefix := make([]float64, n+1)
	for i := 0; i < n; i++ {
		refPrefix[i+1] = refPrefix[i] + ref[i]*ref[i]
	}
	micSuffix := make([]float64, n+1)
	for i := n - 1; i >= 0; i-- {
		micSuffix[i] = micSuffix[i+1] + mic[i]*mic[i]
	}

	limit := maxLag
	if limit > n {
		limit = n
	}
	bestLag, bestScore := 0, 0.0
	for l := 0; l <= limit; l++ {
		denom := math.Sqrt(refPrefix[n-l] * micSuffix[l])
		if denom <= 0 {
			continue
		}
		s := micRe[l] / denom
		if s > bestScore {
			bestScore, bestLag = s, l
		}
	}
	if bestScore <= 0 {
		return 0, 0
	}
	if bestScore > 1 {
		bestScore = 1
	}
	return bestLag, bestScore
}

func nextPow2(x int) int {
	p := 2
	for p < x {
		p <<= 1
	}
	return p
}

// DelayEstimateWhole is DelayEstimate with the score normalized by the two
// signals' whole energies rather than the overlap's at each lag, so a lag that
// slides the only onset out of the overlap scores near zero. Use it on sparse
// envelopes.
func DelayEstimateWhole(ref, mic []float64, maxLag int) (lag int, score float64) {
	n := len(ref)
	if n == 0 || n != len(mic) || maxLag < 0 {
		return 0, 0
	}
	m := nextPow2(n + maxLag)
	f := NewFFT(m)
	refRe, refIm := make([]float64, m), make([]float64, m)
	micRe, micIm := make([]float64, m), make([]float64, m)
	copy(refRe, ref)
	copy(micRe, mic)
	f.Forward(refRe, refIm)
	f.Forward(micRe, micIm)
	for i := 0; i < m; i++ {
		mr, mi := micRe[i], micIm[i]
		rr, ri := refRe[i], -refIm[i]
		micRe[i] = mr*rr - mi*ri
		micIm[i] = mr*ri + mi*rr
	}
	f.Inverse(micRe, micIm)
	refPow, micPow := 0.0, 0.0
	for i := 0; i < n; i++ {
		refPow += ref[i] * ref[i]
		micPow += mic[i] * mic[i]
	}
	denom := math.Sqrt(refPow * micPow)
	if denom <= 0 {
		return 0, 0
	}
	limit := maxLag
	if limit > n {
		limit = n
	}
	bestLag, bestScore := 0, 0.0
	for l := 0; l <= limit; l++ {
		if s := micRe[l] / denom; s > bestScore {
			bestScore, bestLag = s, l
		}
	}
	if bestScore <= 0 {
		return 0, 0
	}
	return bestLag, math.Min(bestScore, 1)
}
