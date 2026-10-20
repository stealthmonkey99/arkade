// SPDX-License-Identifier: GPL-2.0-or-later

package dsp

import (
	"math"
	"math/rand"
	"testing"
)

func sineSignal(freq, sampleRate float64, n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = math.Sin(2 * math.Pi * freq * float64(i) / sampleRate)
	}
	return s
}

func rmsAmplitude(x []float64) float64 {
	var sum float64
	for _, v := range x {
		sum += v * v
	}
	return math.Sqrt(2 * sum / float64(len(x)))
}

func TestDecimatorPassbandAmplitude(t *testing.T) {
	const sampleRate, factor, taps = 44100.0, 3, 127
	in := sineSignal(1000, sampleRate, 20000)
	d := NewDecimator(factor, taps)
	out := d.Process(in, nil)

	settle := 2 * taps / factor
	if len(out) <= settle+1000 {
		t.Fatalf("not enough output samples: %d", len(out))
	}
	amp := rmsAmplitude(out[settle:])
	if math.Abs(amp-1) > 0.02 {
		t.Fatalf("1kHz@44100 decimated by 3: amplitude %.4f, want 1 +/- 2%%", amp)
	}
}

func TestDecimatorStopbandAttenuation(t *testing.T) {
	const sampleRate, factor, taps = 44100.0, 3, 127
	in := sineSignal(6500, sampleRate, 20000)
	d := NewDecimator(factor, taps)
	out := d.Process(in, nil)

	settle := 2 * taps / factor
	amp := rmsAmplitude(out[settle:])
	dB := 20 * math.Log10(amp)
	if dB > -40 {
		t.Fatalf("6.5kHz attenuation %.1f dB, want <= -40 dB (amp=%.5f)", dB, amp)
	}
}

func TestDecimatorChunkingIsExact(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	in := make([]float64, 5000)
	for i := range in {
		in[i] = rng.Float64()*2 - 1
	}

	whole := NewDecimator(3, 63).Process(in, nil)

	chunked := NewDecimator(3, 63)
	chunkSizes := []int{1, 7, 13, 100, 1, 4000, 379}
	var out []float64
	pos := 0
	for _, sz := range chunkSizes {
		end := pos + sz
		if end > len(in) {
			end = len(in)
		}
		out = chunked.Process(in[pos:end], out)
		pos = end
		if pos >= len(in) {
			break
		}
	}
	if pos < len(in) {
		out = chunked.Process(in[pos:], out)
	}

	if len(out) != len(whole) {
		t.Fatalf("chunked len %d != whole len %d", len(out), len(whole))
	}
	for i := range out {
		if out[i] != whole[i] {
			t.Fatalf("sample %d: chunked=%v whole=%v", i, out[i], whole[i])
		}
	}
}

func TestDecimatorReset(t *testing.T) {
	in := sineSignal(1000, 44100, 2000)
	d := NewDecimator(3, 63)
	first := d.Process(in, nil)
	d.Reset()
	second := d.Process(in, nil)

	if len(first) != len(second) {
		t.Fatalf("len mismatch after Reset: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("sample %d differs after Reset: %v vs %v", i, first[i], second[i])
		}
	}
}

func TestDecimatorPanics(t *testing.T) {
	cases := []struct {
		factor, taps int
	}{
		{0, 63},
		{-1, 63},
		{3, 0},
		{3, 64},
	}
	for _, c := range cases {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("NewDecimator(%d, %d) did not panic", c.factor, c.taps)
				}
			}()
			NewDecimator(c.factor, c.taps)
		}()
	}
}
