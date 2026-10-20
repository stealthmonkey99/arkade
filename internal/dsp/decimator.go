// SPDX-License-Identifier: GPL-2.0-or-later

package dsp

import "math"

// Decimator is a streaming FIR low-pass followed by keep-every-factor-th
// sample. It carries filter state and the phase between calls, so feeding a
// signal in arbitrary chunk sizes yields exactly the same output as feeding
// it at once.
type Decimator struct {
	factor int
	h      []float64
	hist   []float64 // ring buffer of the last len(h) input samples
	pos    int       // next slot in hist to overwrite
	count  int64     // total samples seen; output fires when count%factor == 0
}

// NewDecimator builds a decimator by factor with a windowed-sinc (Hann)
// low-pass of the given odd tap count, cutoff at 0.45 of the output Nyquist,
// and unity DC gain.
func NewDecimator(factor, taps int) *Decimator {
	if factor < 1 {
		panic("dsp: decimator factor must be >= 1")
	}
	if taps < 1 || taps%2 == 0 {
		panic("dsp: decimator taps must be odd")
	}

	win := []float64{1}
	if taps > 1 {
		win = SymmetricHann(taps)
	}

	fc := 0.225 / float64(factor) // cutoff as a fraction of the input rate
	center := float64(taps-1) / 2
	h := make([]float64, taps)
	sum := 0.0
	for i := range h {
		x := float64(i) - center
		var s float64
		if x == 0 {
			s = 2 * fc
		} else {
			s = math.Sin(2*math.Pi*fc*x) / (math.Pi * x)
		}
		h[i] = s * win[i]
		sum += h[i]
	}
	for i := range h {
		h[i] /= sum
	}

	return &Decimator{factor: factor, h: h, hist: make([]float64, taps)}
}

// Process filters in and appends the decimated samples to out, returning the
// extended slice.
func (d *Decimator) Process(in []float64, out []float64) []float64 {
	n := len(d.h)
	for _, x := range in {
		d.hist[d.pos] = x
		d.pos++
		if d.pos == n {
			d.pos = 0
		}
		if d.count%int64(d.factor) == 0 {
			var y float64
			for k := 0; k < n; k++ {
				y += d.h[k] * d.hist[(d.pos-1-k+n)%n]
			}
			out = append(out, y)
		}
		d.count++
	}
	return out
}

// Reset clears filter history and phase, as if newly constructed.
func (d *Decimator) Reset() {
	for i := range d.hist {
		d.hist[i] = 0
	}
	d.pos, d.count = 0, 0
}
