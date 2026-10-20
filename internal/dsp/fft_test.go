// SPDX-License-Identifier: GPL-2.0-or-later

package dsp

import (
	"math"
	"math/rand"
	"strconv"
	"testing"
)

var fftSizes = []int{2, 4, 8, 256, 1024}

func TestFFTImpulseIsFlat(t *testing.T) {
	for _, n := range fftSizes {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			f := NewFFT(n)
			re := make([]float64, n)
			im := make([]float64, n)
			re[0] = 1
			f.Forward(re, im)
			for k := 0; k < n; k++ {
				mag := math.Hypot(re[k], im[k])
				if math.Abs(mag-1) > 1e-9 {
					t.Fatalf("bin %d: |X|=%v, want 1", k, mag)
				}
			}
		})
	}
}

func TestFFTSinePeaksAtBin(t *testing.T) {
	for _, n := range fftSizes {
		if n < 8 {
			continue // need room for a distinct bin away from DC/Nyquist
		}
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			k := n / 4
			f := NewFFT(n)
			re := make([]float64, n)
			im := make([]float64, n)
			for i := range re {
				re[i] = math.Sin(2 * math.Pi * float64(k) * float64(i) / float64(n))
			}
			f.Forward(re, im)

			peakBin, peakMag := -1, -1.0
			for i := 0; i < n; i++ {
				mag := math.Hypot(re[i], im[i])
				if mag > peakMag {
					peakMag, peakBin = mag, i
				}
			}
			if peakBin != k {
				t.Fatalf("peak at bin %d, want %d", peakBin, k)
			}
			for i := 0; i < n; i++ {
				if i == k || i == n-k {
					continue
				}
				if mag := math.Hypot(re[i], im[i]); mag > 1e-6*peakMag {
					t.Fatalf("bin %d leaked energy: |X|=%v vs peak %v", i, mag, peakMag)
				}
			}
		})
	}
}

func TestFFTRoundTrip(t *testing.T) {
	for _, n := range fftSizes {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			rng := rand.New(rand.NewSource(int64(n)))
			re := make([]float64, n)
			im := make([]float64, n)
			origRe := make([]float64, n)
			origIm := make([]float64, n)
			for i := range re {
				re[i] = rng.Float64()*2 - 1
				im[i] = rng.Float64()*2 - 1
				origRe[i], origIm[i] = re[i], im[i]
			}
			f := NewFFT(n)
			f.Forward(re, im)
			f.Inverse(re, im)
			for i := range re {
				if math.Abs(re[i]-origRe[i]) > 1e-9 {
					t.Fatalf("re[%d] = %v, want %v", i, re[i], origRe[i])
				}
				if math.Abs(im[i]-origIm[i]) > 1e-9 {
					t.Fatalf("im[%d] = %v, want %v", i, im[i], origIm[i])
				}
			}
		})
	}
}

func TestFFTSize(t *testing.T) {
	for _, n := range fftSizes {
		if got := NewFFT(n).Size(); got != n {
			t.Errorf("Size() = %d, want %d", got, n)
		}
	}
}

func TestFFTPanicsOnInvalidSize(t *testing.T) {
	for _, n := range []int{0, 1, 3, 6, 100} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("NewFFT(%d) did not panic", n)
				}
			}()
			NewFFT(n)
		}()
	}
}
