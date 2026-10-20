// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"math"
	"testing"
)

func tone(freq, dur float64, sampleRate int) []float64 {
	n := int(dur * float64(sampleRate))
	s := make([]float64, n)
	for i := range s {
		s[i] = Sine(SampleTime(sampleRate, i), freq)
	}
	return s
}

func rms(s []float64) float64 {
	if len(s) == 0 {
		return 0
	}
	half := s[len(s)/2:]
	sum := 0.0
	for _, v := range half {
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(half)))
}

func TestLowpassAttenuatesAboveCutoff(t *testing.T) {
	const rate = SampleRate44100
	low, high := tone(200, 0.2, rate), tone(8000, 0.2, rate)
	Lowpass(low, rate, 1000)
	Lowpass(high, rate, 1000)

	if got := rms(low); got < 0.6 {
		t.Errorf("200 Hz through a 1 kHz lowpass: rms %.3f, want >0.6", got)
	}
	if got := rms(high); got > 0.15 {
		t.Errorf("8 kHz through a 1 kHz lowpass: rms %.3f, want <0.15", got)
	}
}

func TestHighpassAttenuatesBelowCutoff(t *testing.T) {
	const rate = SampleRate44100
	low, high := tone(60, 0.2, rate), tone(4000, 0.2, rate)
	Highpass(low, rate, 800)
	Highpass(high, rate, 800)

	if got := rms(low); got > 0.15 {
		t.Errorf("60 Hz through an 800 Hz highpass: rms %.3f, want <0.15", got)
	}
	if got := rms(high); got < 0.6 {
		t.Errorf("4 kHz through an 800 Hz highpass: rms %.3f, want >0.6", got)
	}
}

func TestFiltersAreNoOpsWhenOff(t *testing.T) {
	orig := tone(440, 0.02, SampleRate44100)
	for _, f := range []func(s []float64){
		func(s []float64) { Lowpass(s, SampleRate44100, 0) },
		func(s []float64) { Highpass(s, SampleRate44100, -1) },
		func(s []float64) { Echo(s, SampleRate44100, 0.01, 0.5, 0) },
		func(s []float64) { Echo(s, SampleRate44100, 0, 0.5, 0.5) },
		func(s []float64) { FadeOut(s, SampleRate44100, 0) },
	} {
		s := append([]float64(nil), orig...)
		f(s)
		for i := range s {
			if s[i] != orig[i] {
				t.Fatalf("a disabled effect modified sample %d (%v -> %v)", i, orig[i], s[i])
			}
		}
	}
}

func TestEchoPlacesRepeatsAtTheDelay(t *testing.T) {
	const rate = 1000
	s := make([]float64, 500)
	s[0] = 1 // a single impulse: every later nonzero sample is an echo tap
	Echo(s, rate, 0.1, 0.5, 0.8)

	for i, want := range map[int]float64{100: 0.8, 200: 0.4, 300: 0.2, 400: 0.1} {
		if math.Abs(s[i]-want) > 1e-9 {
			t.Errorf("echo tap at %d = %v, want %v", i, s[i], want)
		}
	}
	if s[0] != 1 {
		t.Errorf("echo changed the dry impulse: %v", s[0])
	}
	if s[150] != 0 {
		t.Errorf("energy between taps at 150: %v, want 0", s[150])
	}
}

func TestEchoFeedbackIsClampedBelowOne(t *testing.T) {
	s := make([]float64, 4096)
	s[0] = 1
	Echo(s, 1000, 0.01, 5, 1) // absurd feedback
	if last := s[len(s)-1]; math.Abs(last) > 1 {
		t.Errorf("runaway feedback: last sample %v", last)
	}
}

func TestFadeOutEndsInSilence(t *testing.T) {
	const rate = 1000
	s := make([]float64, rate)
	for i := range s {
		s[i] = 1
	}
	FadeOut(s, rate, 0.1) // last 100 samples
	if s[len(s)-1] != 0 {
		t.Errorf("last sample = %v, want 0", s[len(s)-1])
	}
	if s[899] != 1 {
		t.Errorf("sample before the fade = %v, want 1 (fade reached too far back)", s[899])
	}
	if mid := s[950]; math.Abs(mid-0.49) > 0.02 {
		t.Errorf("mid-fade sample = %v, want ~0.49", mid)
	}
	short := []float64{1, 1, 1}
	FadeOut(short, rate, 10)
	if short[2] != 0 {
		t.Errorf("over-long fade left %v at the end", short[2])
	}
}

func TestLowpassVaryingSweepsItsCorner(t *testing.T) {
	const rate = SampleRate44100
	s := tone(6000, 0.4, rate)
	corner := make([]float64, len(s))
	for i := range corner {
		corner[i] = 16000 - 15800*float64(i)/float64(len(corner))
	}
	LowpassVarying(s, rate, corner)
	// rms reads the back half, so these are the second tenth of the sweep and the last twentieth.
	open, shut := rms(s[:len(s)/5]), rms(s[len(s)*9/10:])
	if open < 0.3 {
		t.Errorf("rms %.3f while the corner is open, want most of the tone through", open)
	}
	if shut > open/4 {
		t.Errorf("rms %.4f once the corner has closed, want well under a quarter of %.3f", shut, open)
	}
}

func TestLowpassVaryingMatchesLowpassAtAFixedCorner(t *testing.T) {
	const rate = SampleRate44100
	fixed, varying := tone(3000, 0.1, rate), tone(3000, 0.1, rate)
	corner := make([]float64, len(varying))
	for i := range corner {
		corner[i] = 900
	}
	Lowpass(fixed, rate, 900)
	LowpassVarying(varying, rate, corner)
	for i := range fixed {
		if fixed[i] != varying[i] {
			t.Fatalf("sample %d: fixed corner gave %.17g, varying gave %.17g — they are the same filter and must agree exactly",
				i, fixed[i], varying[i])
		}
	}
}

func TestLowpassVaryingHoldsAShortControlSignal(t *testing.T) {
	const rate = SampleRate44100
	held, full := tone(3000, 0.1, rate), tone(3000, 0.1, rate)
	long := make([]float64, len(full))
	for i := range long {
		long[i] = 900
	}
	LowpassVarying(held, rate, []float64{900, 900, 900})
	LowpassVarying(full, rate, long)
	for i := range held {
		if held[i] != full[i] {
			t.Fatalf("sample %d: a three-entry control signal must hold its last value (%.17g vs %.17g)", i, held[i], full[i])
		}
	}
}

func TestBandPassKeepsItsCentreAndRejectsTheRest(t *testing.T) {
	const rate = SampleRate44100
	centre := make([]float64, int(0.3*rate))
	for i := range centre {
		centre[i] = 1000
	}
	at := rms(BandPass(tone(1000, 0.3, rate), rate, centre, 4))
	below := rms(BandPass(tone(150, 0.3, rate), rate, centre, 4))
	above := rms(BandPass(tone(6000, 0.3, rate), rate, centre, 4))
	if at < 0.5 {
		t.Errorf("rms %.3f at the centre frequency, want the tone essentially through", at)
	}
	if below > at/4 || above > at/4 {
		t.Errorf("rms %.4f below and %.4f above the centre, want both well under a quarter of %.3f", below, above, at)
	}
}

func TestBandPassDoesNotTouchItsInput(t *testing.T) {
	const rate = SampleRate44100
	in := tone(1000, 0.05, rate)
	before := append([]float64(nil), in...)
	centre := make([]float64, len(in))
	for i := range centre {
		centre[i] = 1000
	}
	_ = BandPass(in, rate, centre, 4)
	_ = BandPass(in, rate, centre, 8)
	for i := range in {
		if in[i] != before[i] {
			t.Fatalf("sample %d was modified in place (%.17g, was %.17g)", i, in[i], before[i])
		}
	}
}

func TestBandPassSweepFollowsTheCentre(t *testing.T) {
	const rate = SampleRate44100
	s := tone(3000, 0.6, rate)
	centre := make([]float64, len(s))
	for i := range centre {
		centre[i] = 500 + 5500*float64(i)/float64(len(centre))
	}
	out := BandPass(s, rate, centre, 6)
	third := len(out) / 3
	start, middle, end := rms(out[:third]), rms(out[third:2*third]), rms(out[2*third:])
	if middle <= start || middle <= end {
		t.Errorf("rms %.4f / %.4f / %.4f across the sweep, want the middle loudest — the resonance is not tracking",
			start, middle, end)
	}
}

func TestBandPassPassesThroughWhenOff(t *testing.T) {
	const rate = SampleRate44100
	in := tone(1000, 0.02, rate)
	centre := make([]float64, len(in))
	for i := range centre {
		centre[i] = 1000
	}
	for _, tc := range []struct {
		name   string
		rate   int
		centre []float64
		q      float64
	}{
		{"no centre", rate, nil, 4},
		{"zero q", rate, centre, 0},
		{"zero rate", 0, centre, 4},
	} {
		out := BandPass(in, tc.rate, tc.centre, tc.q)
		for i := range in {
			if out[i] != in[i] {
				t.Fatalf("%s: sample %d is %.17g, want the input %.17g unfiltered", tc.name, i, out[i], in[i])
			}
		}
	}
}
