// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"math"
	"testing"
)

func testPhrase() Vocal {
	v := DefaultVocal()
	syllable := func(atMs, hz float64) Syllable {
		return Syllable{AtMs: atMs, Hz: hz, Phones: []Phone{
			{Ms: 50, F1Hz: 300, F2Hz: 1000, Nasal: 1, GlideMs: 30},
			{Ms: 90, F1Hz: 640, F2Hz: 1100, Open: 1, Nasal: 0.3, GlideMs: 60},
			{Ms: 30, F1Hz: 300, F2Hz: 1000, Nasal: 1, GlideMs: 30},
			{Ms: 100, F1Hz: 300, F2Hz: 1000, Nasal: 1},
		}}
	}
	v.Syllables = []Syllable{syllable(0, 220), syllable(320, 200)}
	return v
}

func TestVocalRendersAPhrase(t *testing.T) {
	v := testPhrase()
	got := v.Render(SampleRate44100)
	if len(got) == 0 {
		t.Fatal("rendered nothing")
	}
	if peak := peakOf(got); math.Abs(peak-v.Peak) > 1e-9 {
		t.Fatalf("peak %.6f, want %.2f", peak, v.Peak)
	}
	if gotMs, wantMs := 1000*float64(len(got))/SampleRate44100, 590.0; math.Abs(gotMs-wantMs) > 1 {
		t.Errorf("phrase is %.0f ms, want about %.0f", gotMs, wantMs)
	}
	if got, want := v.LengthMs(), 590.0; got != want {
		t.Errorf("LengthMs = %v, want %v", got, want)
	}
}

func TestVocalRenderIsDeterministic(t *testing.T) {
	v := testPhrase()
	requireSameSamples(t, v.Render(SampleRate44100), v.Render(SampleRate44100))
}

func TestVocalFirstPhoneGlidesInFromRest(t *testing.T) {
	const rate = SampleRate44100
	render := func(glideMs float64) []float64 {
		v := DefaultVocal()
		v.Peak = 0 // the comparison is between raw levels
		v.Syllables = []Syllable{{Hz: 200, Phones: []Phone{
			{Ms: 300, F1Hz: 300, F2Hz: 1000, Nasal: 1, GlideMs: glideMs},
		}}}
		return v.Render(rate)
	}
	cut, swell := render(0), render(80)
	early, held := MsToSamples(rate, 25), MsToSamples(rate, 150)
	if a, b := peakAround(swell, early, 200), peakAround(cut, early, 200); a >= b*0.5 {
		t.Errorf("25 ms into an 80 ms onset the voice is at %.4f, the cut one at %.4f — the glide did not soften it", a, b)
	}
	if a, b := peakAround(swell, held, 200), peakAround(cut, held, 200); math.Abs(a-b) > 0.02*b {
		t.Errorf("once arrived the two differ (%.4f against %.4f) — the onset leaked past the glide", a, b)
	}
}

func TestVocalSyllablesAreSeparate(t *testing.T) {
	const rate = SampleRate44100
	s := testPhrase().Render(rate)
	gap, loud := MsToSamples(rate, 300), MsToSamples(rate, 100)
	if peakAround(s, gap, 200) >= peakAround(s, loud, 200) {
		t.Errorf("no quieter between syllables (%.4f at the gap, %.4f mid-syllable)",
			peakAround(s, gap, 200), peakAround(s, loud, 200))
	}
}

func TestVocalEmpty(t *testing.T) {
	if got := (Vocal{}).Render(SampleRate44100); got != nil {
		t.Error("a voice with nothing to say rendered something")
	}
	if got := testPhrase().Render(0); got != nil {
		t.Error("a non-positive sample rate rendered something")
	}
	v := DefaultVocal()
	v.Syllables = []Syllable{{Hz: 200, Phones: nil}}
	if got := v.Render(SampleRate44100); len(got) != 0 {
		t.Error("a syllable with no phones rendered something")
	}
	if got := (Vocal{}).LengthMs(); got != 0 {
		t.Errorf("an empty voice is %v ms long", got)
	}
}

func peakOf(s []float64) float64 {
	max := 0.0
	for _, v := range s {
		if a := math.Abs(v); a > max {
			max = a
		}
	}
	return max
}

func peakAround(s []float64, centre, half int) float64 {
	lo, hi := max(centre-half, 0), min(centre+half, len(s))
	if lo >= hi {
		return 0
	}
	return peakOf(s[lo:hi])
}
