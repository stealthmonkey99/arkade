// SPDX-License-Identifier: GPL-2.0-or-later

// Package dsp holds the signal-processing primitives shared by the mic echo
// canceller and the keyword classifier: FFT, resampling, cross-correlation
// delay estimation, MFCCs and DTW. Pure Go, no external dependencies, and
// allocation-free once a type's constructor has run.
package dsp

import (
	"math"
	"math/bits"
)

// FFT is a radix-2 complex FFT of fixed size n (power of two).
type FFT struct {
	n   int
	rev []int
	wRe []float64 // wRe[j], wIm[j] = cos/sin(-2*pi*j/n), j = 0..n/2-1
	wIm []float64
}

// NewFFT builds an FFT plan for size n, which must be a power of two ≥ 2.
// Twiddles and the bit-reversal table are built here so Forward and Inverse
// don't allocate.
func NewFFT(n int) *FFT {
	if n < 2 || n&(n-1) != 0 {
		panic("dsp: FFT size must be a power of two >= 2")
	}
	f := &FFT{
		n:   n,
		rev: make([]int, n),
		wRe: make([]float64, n/2),
		wIm: make([]float64, n/2),
	}
	logn := bits.Len(uint(n)) - 1
	for i := range f.rev {
		f.rev[i] = bitReverse(i, logn)
	}
	for j := range f.wRe {
		angle := -2 * math.Pi * float64(j) / float64(n)
		f.wRe[j] = math.Cos(angle)
		f.wIm[j] = math.Sin(angle)
	}
	return f
}

func bitReverse(x, logn int) int {
	r := 0
	for i := 0; i < logn; i++ {
		r = (r << 1) | (x & 1)
		x >>= 1
	}
	return r
}

// Size returns the transform length n.
func (f *FFT) Size() int { return f.n }

// Forward computes the in-place DFT of (re, im), each of length n.
func (f *FFT) Forward(re, im []float64) { f.transform(re, im, false) }

// Inverse computes the in-place inverse DFT of (re, im), scaled by 1/n so
// Inverse(Forward(x)) == x.
func (f *FFT) Inverse(re, im []float64) { f.transform(re, im, true) }

func (f *FFT) transform(re, im []float64, inverse bool) {
	n := f.n
	for i, j := range f.rev {
		if j > i {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		half := size / 2
		step := n / size
		for start := 0; start < n; start += size {
			for k := 0; k < half; k++ {
				idx := k * step
				wr, wi := f.wRe[idx], f.wIm[idx]
				if inverse {
					wi = -wi
				}
				i0, i1 := start+k, start+k+half
				tr := re[i1]*wr - im[i1]*wi
				ti := re[i1]*wi + im[i1]*wr
				re[i1] = re[i0] - tr
				im[i1] = im[i0] - ti
				re[i0] += tr
				im[i0] += ti
			}
		}
	}
	if inverse {
		scale := 1 / float64(n)
		for i := range re {
			re[i] *= scale
			im[i] *= scale
		}
	}
}
