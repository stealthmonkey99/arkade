// SPDX-License-Identifier: GPL-2.0-or-later

package dsp

import "math"

// DTW returns the dynamic-time-warping distance between two feature sequences
// of equal-length frame vectors, normalized by len(a)+len(b). The Sakoe–Chiba
// band is band frames wide (band ≤ 0 is unconstrained) and widened to at least
// |len(a)-len(b)|. Returns +Inf if either sequence is empty. scratch may be
// nil or a reusable buffer; it is grown as needed and returned.
func DTW(a, b [][]float64, band int, scratch []float64) (float64, []float64) {
	na, nb := len(a), len(b)
	if na == 0 || nb == 0 {
		return math.Inf(1), scratch
	}

	unconstrained := band <= 0
	w := band
	if diff := absInt(na - nb); w < diff {
		w = diff
	}

	cols := nb + 1
	need := (na + 1) * cols
	if cap(scratch) < need {
		scratch = make([]float64, need)
	}
	scratch = scratch[:need]
	inf := math.Inf(1)
	for i := range scratch {
		scratch[i] = inf
	}
	scratch[0] = 0

	for i := 1; i <= na; i++ {
		lo, hi := 1, nb
		if !unconstrained {
			if lo = i - w; lo < 1 {
				lo = 1
			}
			if hi = i + w; hi > nb {
				hi = nb
			}
		}
		for j := lo; j <= hi; j++ {
			best := scratch[(i-1)*cols+j-1]
			if v := scratch[(i-1)*cols+j]; v < best {
				best = v
			}
			if v := scratch[i*cols+j-1]; v < best {
				best = v
			}
			scratch[i*cols+j] = euclidean(a[i-1], b[j-1]) + best
		}
	}

	return scratch[na*cols+nb] / float64(na+nb), scratch
}

func euclidean(x, y []float64) float64 {
	var s float64
	for i := range x {
		d := x[i] - y[i]
		s += d * d
	}
	return math.Sqrt(s)
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
