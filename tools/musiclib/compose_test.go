// SPDX-License-Identifier: GPL-2.0-or-later

package musiclib

import (
	"math"
	"testing"
)

const composeTestRate = SampleRate22050

func otherSong() Song {
	return Song{
		BPM:   90,
		Beats: 2,
		Tracks: []Track{{
			Name: "horn",
			Inst: Instrument{Wave: WavePulse, Duty: 0.5, SustainLevel: 1},
			Notes: []Note{
				{Midi: 55, Start: 0, Dur: 1},
				{Midi: 60, Start: 1, Dur: 1},
			},
		}},
	}
}

func TestCompositionLength(t *testing.T) {
	a := RenderLoop(testSong(), composeTestRate)  // 4 beats at 120 = 2 s
	b := RenderLoop(otherSong(), composeTestRate) // 2 beats at 90 = 1.333 s
	aSec := float64(len(a)) / composeTestRate

	c := Composition{Segments: []Segment{
		{Samples: a, Loop: true, Repeat: 2, Start: 0},
		{Samples: b, Loop: true, Start: 2 * aSec},
	}}
	want := 2*len(a) + len(b)
	if got := c.LengthSamples(composeTestRate); got != want {
		t.Errorf("length = %d samples, want %d", got, want)
	}
	if got, wantSec := c.LengthSeconds(composeTestRate), float64(want)/composeTestRate; math.Abs(got-wantSec) > 1e-9 {
		t.Errorf("length = %v s, want %v", got, wantSec)
	}
	if got := len(RenderComposition(c, composeTestRate)); got != want {
		t.Errorf("rendered %d samples, want %d", got, want)
	}

	c.Seconds = 1
	if got := c.LengthSamples(composeTestRate); got != composeTestRate {
		t.Errorf("explicit length = %d samples, want %d", got, composeTestRate)
	}
}

func TestCompositionRepeatIsExactTiling(t *testing.T) {
	a := RenderLoop(testSong(), composeTestRate)
	got := RenderCompositionPeak(Composition{
		Segments: []Segment{{Samples: a, Loop: true, Repeat: 3}},
	}, composeTestRate, -1)
	if len(got) != 3*len(a) {
		t.Fatalf("A×3 rendered %d samples, want %d", len(got), 3*len(a))
	}
	for i, v := range got {
		if math.Abs(v-a[i%len(a)]) > 1e-12 {
			t.Fatalf("sample %d = %v, want %v: a repeat is not exact tiling", i, v, a[i%len(a)])
		}
	}
}

func TestCompositionJoinIsTransparentForIdenticalMaterial(t *testing.T) {
	a := RenderLoop(testSong(), composeTestRate)
	sec := float64(len(a)) / composeTestRate
	got := RenderCompositionPeak(Composition{
		Segments: []Segment{
			{Samples: a, Loop: true, Start: 0},
			{Samples: a, Loop: true, Start: sec},
		},
	}, composeTestRate, -1)
	if len(got) != 2*len(a) {
		t.Fatalf("rendered %d samples, want %d", len(got), 2*len(a))
	}
	worst := 0.0
	for i, v := range got {
		if d := math.Abs(v - a[i%len(a)]); d > worst {
			worst = d
		}
	}
	if worst > 1e-12 {
		t.Errorf("the junction altered identical material by up to %v", worst)
	}
}

func TestCompositionJoinRemovesTheStep(t *testing.T) {
	level := func(v float64, n int) []float64 {
		s := make([]float64, n)
		for i := range s {
			s[i] = v
		}
		return s
	}
	a, b := level(0.8, 4410), level(-0.8, 2205)
	naive := 1.6

	got := RenderCompositionPeak(Composition{
		Segments: []Segment{
			{Samples: a, Loop: true, Start: 0},
			{Samples: b, Loop: true, Start: float64(len(a)) / composeTestRate},
		},
	}, composeTestRate, -1)
	if len(got) != len(a)+len(b) {
		t.Fatalf("rendered %d samples, want %d", len(got), len(a)+len(b))
	}

	step := math.Abs(got[0] - got[len(got)-1]) // the loop point
	for i := 1; i < len(got); i++ {
		if d := math.Abs(got[i] - got[i-1]); d > step {
			step = d
		}
	}
	if want := naive / 20; step > want {
		t.Errorf("largest step is %v, want under %v — the junctions are not crossfaded", step, want)
	}

	if mid := got[len(a)/2]; math.Abs(mid-0.8) > 1e-12 {
		t.Errorf("the middle of A is %v, want 0.8 — the fade is smearing the body", mid)
	}
}

func TestCompositionOneShotsAreVerbatim(t *testing.T) {
	hit := make([]float64, 400)
	for i := range hit {
		hit[i] = 1 - float64(i)/float64(len(hit)) // instant attack, linear decay
	}
	got := RenderCompositionPeak(Composition{
		Seconds:  1,
		Segments: []Segment{{Samples: hit, Start: 0.25}},
	}, composeTestRate, -1)
	off := int(math.Round(0.25 * composeTestRate))
	for i, v := range hit {
		if math.Abs(got[off+i]-v) > 1e-12 {
			t.Fatalf("one-shot sample %d = %v, want %v", i, got[off+i], v)
		}
	}
	if got[off-1] != 0 {
		t.Errorf("something was written before the one-shot started: %v", got[off-1])
	}
}

func TestCompositionWrapsAtTheLoopPoint(t *testing.T) {
	hit := make([]float64, 400)
	for i := range hit {
		hit[i] = 1
	}
	got := RenderCompositionPeak(Composition{
		Seconds:  1,
		Segments: []Segment{{Samples: hit, Start: 1 - 100.0/composeTestRate}},
	}, composeTestRate, -1)
	if got[len(got)-1] != 1 {
		t.Errorf("the one-shot did not reach the composition's end")
	}
	if got[0] != 1 || got[299] != 1 {
		t.Errorf("the tail did not wrap into the beginning: got[0]=%v got[299]=%v", got[0], got[299])
	}
	if got[300] != 0 {
		t.Errorf("the wrapped tail ran too long: got[300]=%v", got[300])
	}
}

func TestCompositionGainAndDeterminism(t *testing.T) {
	a := RenderLoop(testSong(), composeTestRate)
	half := RenderCompositionPeak(Composition{
		Segments: []Segment{{Samples: a, Loop: true, Gain: 0.5}},
	}, composeTestRate, -1)
	for i := range half {
		if math.Abs(half[i]-0.5*a[i]) > 1e-12 {
			t.Fatalf("gain 0.5 sample %d = %v, want %v", i, half[i], 0.5*a[i])
		}
	}

	c := Composition{Segments: []Segment{
		{Samples: a, Loop: true, Repeat: 2},
		{Samples: RenderLoop(otherSong(), composeTestRate), Loop: true, Start: 1},
	}}
	x := RenderComposition(c, composeTestRate)
	y := RenderComposition(c, composeTestRate)
	for i := range x {
		if x[i] != y[i] {
			t.Fatalf("two renders of one composition differ at sample %d", i)
		}
	}
	peak := 0.0
	for _, v := range x {
		if m := math.Abs(v); m > peak {
			peak = m
		}
	}
	if math.Abs(peak-DefaultPeak) > 1e-9 {
		t.Errorf("peak = %v, want the %v headroom every asset ships with", peak, DefaultPeak)
	}
}

func TestEmptyCompositionRendersNothing(t *testing.T) {
	if got := RenderComposition(Composition{}, composeTestRate); got != nil {
		t.Errorf("an empty composition rendered %d samples", len(got))
	}
	if got := RenderComposition(Composition{Segments: []Segment{{Samples: nil, Loop: true}}}, composeTestRate); got != nil {
		t.Errorf("a segment with no material rendered %d samples", len(got))
	}
}
