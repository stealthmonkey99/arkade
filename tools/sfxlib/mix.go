// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import "math"

// Normalize scales s in place so its peak absolute sample equals peak. A
// silent slice is left untouched.
func Normalize(s []float64, peak float64) {
	max := 0.0
	for _, v := range s {
		if a := math.Abs(v); a > max {
			max = a
		}
	}
	if max == 0 {
		return
	}
	g := peak / max
	for i := range s {
		s[i] *= g
	}
}

// Mix sums tracks sample by sample into a new slice as long as the longest.
func Mix(tracks ...[]float64) []float64 {
	n := 0
	for _, tr := range tracks {
		if len(tr) > n {
			n = len(tr)
		}
	}
	out := make([]float64, n)
	for _, tr := range tracks {
		for i, v := range tr {
			out[i] += v
		}
	}
	return out
}

// MixAt adds src into dst in place starting at offset; samples outside dst
// are dropped.
func MixAt(dst, src []float64, offset int) {
	for i, v := range src {
		j := offset + i
		if j < 0 || j >= len(dst) {
			continue
		}
		dst[j] += v
	}
}
