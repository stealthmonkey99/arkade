// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"math"
	"testing"
)

func sineTone(freq, durationS, sampleRate float64) []float64 {
	n := int(durationS * sampleRate)
	out := make([]float64, n)
	for i := range out {
		out[i] = Sine(SampleTime(int(sampleRate), i), freq)
	}
	return out
}

func TestGoertzelPowerPeaksAtTargetFrequency(t *testing.T) {
	const sampleRate = 44100
	s := sineTone(440, 0.2, sampleRate)

	onFreq := GoertzelPower(s, sampleRate, 440)
	offFreq := GoertzelPower(s, sampleRate, 4000)

	if onFreq <= offFreq*100 {
		t.Fatalf("GoertzelPower(440Hz) = %.1f, GoertzelPower(4000Hz) = %.1f; want the tone's own bin to dominate by >100x", onFreq, offFreq)
	}
}

func TestBandFractionConcentratesAtToneFrequency(t *testing.T) {
	const sampleRate = 44100
	s := sineTone(440, 0.2, sampleRate)

	near := BandFraction(s, sampleRate, 300, 600, 20, 10000, 0)
	far := BandFraction(s, sampleRate, 20, 150, 20, 10000, 0)

	if near < 0.95 {
		t.Errorf("BandFraction(300-600Hz) = %.3f, want >= 0.95 (tone is at 440Hz)", near)
	}
	if far > 0.05 {
		t.Errorf("BandFraction(20-150Hz) = %.3f, want <= 0.05 (tone is at 440Hz)", far)
	}
}

func TestBandFractionSilence(t *testing.T) {
	s := make([]float64, MsToSamples(44100, 100))
	frac := BandFraction(s, 44100, 300, 600, 20, 10000, 0)
	if frac != 0 {
		t.Errorf("BandFraction(silence) = %v, want 0", frac)
	}
}

func TestAssertSpectrumPassAndFail(t *testing.T) {
	const sampleRate = 44100
	s := sineTone(440, 0.2, sampleRate)

	err := AssertSpectrum(s, sampleRate, 20, 10000, []SpectrumCheck{
		{Name: "sub-150", Lo: 20, Hi: 150, Max: 0.5},
		{Name: "mid-200-2000", Lo: 200, Hi: 2000, Min: 0.3},
	})
	if err != nil {
		t.Errorf("AssertSpectrum with satisfiable checks returned an error: %v", err)
	}

	err = AssertSpectrum(s, sampleRate, 20, 10000, []SpectrumCheck{
		{Name: "impossible-highband", Lo: 5000, Hi: 10000, Min: 0.5},
	})
	if err == nil {
		t.Fatal("AssertSpectrum with an unsatisfiable check returned nil, want an error")
	}
}

func TestNoiseSampleRange(t *testing.T) {
	rng := NewRNG(1)
	for i := 0; i < 10000; i++ {
		v := NoiseSample(rng)
		if v < -1 || v >= 1 {
			t.Fatalf("NoiseSample() = %v, want in [-1,1)", v)
		}
	}
}

func TestNewRNGDeterministic(t *testing.T) {
	a := NewRNG(42)
	b := NewRNG(42)
	for i := 0; i < 100; i++ {
		va, vb := a(), b()
		if va != vb {
			t.Fatalf("draw %d: %v != %v", i, va, vb)
		}
	}
}

func TestADSREnvelopeShape(t *testing.T) {
	e := ADSR{AttackS: 0.01, DecayS: 0.02, SustainS: 0.3, ReleaseS: 0.05, SustainLevel: 0.7}

	if v := e.At(-0.001); v != 0 {
		t.Errorf("At(negative) = %v, want 0", v)
	}
	if v := e.At(0); v != 0 {
		t.Errorf("At(0) = %v, want 0", v)
	}
	if v := e.At(e.AttackS); math.Abs(v-1) > 1e-9 {
		t.Errorf("At(end of attack) = %v, want 1", v)
	}
	if v := e.At(e.AttackS + e.DecayS); math.Abs(v-e.SustainLevel) > 1e-9 {
		t.Errorf("At(end of decay) = %v, want SustainLevel %v", v, e.SustainLevel)
	}
	mid := e.AttackS + e.DecayS + e.SustainS/2
	if v := e.At(mid); math.Abs(v-e.SustainLevel) > 1e-9 {
		t.Errorf("At(mid-sustain) = %v, want SustainLevel %v", v, e.SustainLevel)
	}
	if v := e.At(e.Duration()); v != 0 {
		t.Errorf("At(Duration()) = %v, want 0", v)
	}
	if v := e.At(e.Duration() + 1); v != 0 {
		t.Errorf("At(past Duration()) = %v, want 0", v)
	}
}
