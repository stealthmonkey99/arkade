// SPDX-License-Identifier: GPL-2.0-or-later

package dsp

import (
	"math"
	"testing"
)

func newTestMFCC() *MFCC {
	return NewMFCC(16000, 512, 26, 13, 20, 8000)
}

func TestMFCCFrameDeterministic(t *testing.T) {
	m := newTestMFCC()
	samples := sineSignal(1000, 16000, 512)

	a := make([]float64, 13)
	b := make([]float64, 13)
	m.Frame(samples, a)
	m.Frame(samples, b)

	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("coeff %d: %v != %v on repeat call", i, a[i], b[i])
		}
	}
}

func TestMFCCDistinguishesFrequencies(t *testing.T) {
	m := newTestMFCC()
	low := sineSignal(1000, 16000, 512)
	high := sineSignal(3000, 16000, 512)

	a := make([]float64, 13)
	b := make([]float64, 13)
	m.Frame(low, a)
	m.Frame(high, b)

	var dist float64
	for i := range a {
		d := a[i] - b[i]
		dist += d * d
	}
	dist = math.Sqrt(dist)
	if dist < 1 {
		t.Fatalf("1kHz and 3kHz coefficients too similar: distance %v", dist)
	}
}

func TestMFCCLogMelOfSilence(t *testing.T) {
	m := newTestMFCC()
	samples := make([]float64, 512)
	out := make([]float64, 26)
	m.LogMel(samples, out)

	want := math.Log(1e-10)
	for i, v := range out {
		if math.Abs(v-want) > 1e-9 {
			t.Fatalf("band %d: LogMel(silence) = %v, want %v", i, v, want)
		}
	}
}

func TestMFCCOutputLengths(t *testing.T) {
	cases := []struct {
		frameLen, bands, coeffs int
	}{
		{256, 20, 13},
		{512, 26, 13},
		{1024, 40, 20},
	}
	for _, c := range cases {
		m := NewMFCC(16000, c.frameLen, c.bands, c.coeffs, 0, 8000)
		samples := sineSignal(440, 16000, c.frameLen)

		mel := make([]float64, c.bands)
		for i := range mel {
			mel[i] = math.NaN()
		}
		m.LogMel(samples, mel)
		for i, v := range mel {
			if math.IsNaN(v) {
				t.Fatalf("frameLen=%d: LogMel band %d unwritten", c.frameLen, i)
			}
		}

		coef := make([]float64, c.coeffs)
		for i := range coef {
			coef[i] = math.NaN()
		}
		m.Frame(samples, coef)
		for i, v := range coef {
			if math.IsNaN(v) {
				t.Fatalf("frameLen=%d: Frame coeff %d unwritten", c.frameLen, i)
			}
		}
	}
}

func TestMFCCPanicsOnNonPowerOfTwoFrameLen(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewMFCC did not panic on non-power-of-two frameLen")
		}
	}()
	NewMFCC(16000, 500, 26, 13, 20, 8000)
}
