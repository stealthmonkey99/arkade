// SPDX-License-Identifier: GPL-2.0-or-later

package dsp

import "testing"

func TestNoAllocHotPaths(t *testing.T) {
	f := NewFFT(256)
	re := make([]float64, 256)
	im := make([]float64, 256)
	if n := testing.AllocsPerRun(100, func() { f.Forward(re, im); f.Inverse(re, im) }); n != 0 {
		t.Errorf("FFT Forward/Inverse allocs = %v, want 0", n)
	}

	d := NewDecimator(3, 63)
	in := sineSignal(1000, 44100, 300)
	out := make([]float64, 0, 200)
	if n := testing.AllocsPerRun(100, func() { out = d.Process(in, out[:0]) }); n != 0 {
		t.Errorf("Decimator.Process allocs = %v, want 0", n)
	}

	m := newTestMFCC()
	samples := sineSignal(1000, 16000, 512)
	coef := make([]float64, 13)
	mel := make([]float64, 26)
	if n := testing.AllocsPerRun(100, func() { m.Frame(samples, coef); m.LogMel(samples, mel) }); n != 0 {
		t.Errorf("MFCC Frame/LogMel allocs = %v, want 0", n)
	}
}
