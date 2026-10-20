// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"math"
	"strings"
	"testing"
)

func TestFramedAnalysisFindsALongTone(t *testing.T) {
	const rate = SampleRate44100
	hum, voice := tone(100, 2.0, rate), tone(885, 2.0, rate)
	s := make([]float64, len(hum))
	for i := range s {
		s[i] = 0.3*hum[i] + voice[i]
	}

	whole := BandFraction(s, rate, 20, 150, 20, 10000, 0)
	framed := FrameSignal(s, rate, DefaultBandStep, 0).BandFraction(20, 150, 20, 10000)

	if framed > 0.2 {
		t.Errorf("framed: sub-150 Hz share %.2f, want <0.2 — the 885 Hz voice dominates this signal", framed)
	}
	if whole < 0.8 {
		t.Errorf("whole-signal: sub-150 Hz share %.2f; the fixture is meant to demonstrate that this reading is wrong (>0.8)", whole)
	}
	if got := FrameSignal(s, rate, DefaultBandStep, 0).BandFraction(640, 1580, 20, 10000); got < 0.7 {
		t.Errorf("framed: only %.0f%% of the energy landed in the band holding the 885 Hz voice", 100*got)
	}
}

func TestFramedAnalysisMatchesWholeForShortSounds(t *testing.T) {
	const rate = SampleRate44100
	s := tone(1000, 0.02, rate) // shorter than one 20 Hz frame

	fs := FrameSignal(s, rate, DefaultBandStep, 0)
	if fs.Count() != 1 {
		t.Fatalf("a sub-frame signal produced %d frames, want 1", fs.Count())
	}
	framed := fs.BandFraction(640, 1580, 20, 10000)
	if framed < 0.5 {
		t.Errorf("1 kHz tone measured %.2f in its own band", framed)
	}
}

func TestFrameSetBinsTileTheRange(t *testing.T) {
	const rate = 8000
	fs := FrameSignal(tone(500, 0.5, rate), rate, 20, 8)
	bins := fs.Bins(20, 10000)
	if len(bins) == 0 {
		t.Fatal("no bins")
	}
	if bins[0].Hz != 20 {
		t.Errorf("first bin at %v Hz, want 20", bins[0].Hz)
	}
	for _, b := range bins {
		if b.Hz >= rate/2 {
			t.Fatalf("bin at %v Hz is at or above the %d Hz Nyquist", b.Hz, rate/2)
		}
	}
	sum := 0.0
	for _, b := range bins {
		sum += b.Power
	}
	if got := fs.BandEnergy(20, 10000); math.Abs(sum-got) > 1e-9*math.Max(1, got) {
		t.Errorf("sum of bins %v != BandEnergy %v", sum, got)
	}
}

func TestFrameSetAssertSpectrumSpeaksTheSameWords(t *testing.T) {
	const rate = SampleRate44100
	sub := tone(55, 1.0, rate) // the classic inaudible-on-cabinet sound
	fs := FrameSignal(sub, rate, DefaultBandStep, 0)

	err := fs.AssertSpectrum(20, 10000, []SpectrumCheck{
		{Name: "sub-150Hz", Lo: 20, Hi: 150, Max: 0.5},
	})
	if err == nil {
		t.Fatal("a 55 Hz tone passed the sub-150 Hz check")
	}
	if !strings.Contains(err.Error(), "sfxlib: spectrum assertion failed") ||
		!strings.Contains(err.Error(), "sub-150Hz") {
		t.Errorf("unexpected error text: %v", err)
	}

	if err := fs.AssertSpectrum(20, 10000, []SpectrumCheck{
		{Name: "sub-150Hz", Lo: 20, Hi: 150, Min: 0.5},
	}); err != nil {
		t.Errorf("a 55 Hz tone failed a >=50%% sub-bass check: %v", err)
	}
}

func TestFrameSignalEdges(t *testing.T) {
	if got := FrameSignal(nil, SampleRate44100, 20, 0).Count(); got != 0 {
		t.Errorf("empty signal produced %d frames", got)
	}
	if got := FrameSignal([]float64{1, 2, 3}, 0, 20, 0).Count(); got != 0 {
		t.Errorf("zero sample rate produced %d frames", got)
	}
	fs := FrameSignal(tone(440, 10, SampleRate44100), SampleRate44100, 20, 6)
	if fs.Count() != 6 {
		t.Errorf("frame count = %d, want the 6 asked for", fs.Count())
	}
	if fs.StepHz() != 20 {
		t.Errorf("step = %v, want 20", fs.StepHz())
	}
}

func TestHannTapersToZeroAndCopies(t *testing.T) {
	in := []float64{1, 1, 1, 1, 1}
	out := Hann(in)
	if out[0] != 0 || out[len(out)-1] != 0 {
		t.Errorf("Hann window ends = %v, %v; want 0, 0", out[0], out[len(out)-1])
	}
	if out[2] != 1 {
		t.Errorf("Hann window centre = %v, want 1", out[2])
	}
	for i, v := range in {
		if v != 1 {
			t.Fatalf("Hann modified its input at %d (%v)", i, v)
		}
	}
}
