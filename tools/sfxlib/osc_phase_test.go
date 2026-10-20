// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"math"
	"testing"
)

func TestAccumPhaseCountsCycles(t *testing.T) {
	const rate = 1000
	freq := make([]float64, rate) // one second at 100 Hz
	for i := range freq {
		freq[i] = 100
	}
	ph := AccumPhase(freq, rate)
	if math.Abs(ph[0]-0.1) > 1e-12 {
		t.Errorf("first sample is at %.6f cycles, want 0.1", ph[0])
	}
	if math.Abs(ph[len(ph)-1]-100) > 1e-9 {
		t.Errorf("a second of 100 Hz ends at %.6f cycles, want 100", ph[len(ph)-1])
	}
}

func TestAccumPhaseIsContinuousThroughASweep(t *testing.T) {
	const rate = SampleRate44100
	freq := make([]float64, rate/10)
	for i := range freq {
		freq[i] = 200 + 3000*float64(i)/float64(len(freq))
	}
	ph := AccumPhase(freq, rate)
	worst := 0.0
	for i := 1; i < len(ph); i++ {
		d := ph[i] - ph[i-1]
		if d <= 0 {
			t.Fatalf("phase went backwards at sample %d (%.9f -> %.9f)", i, ph[i-1], ph[i])
		}
		if d > worst {
			worst = d
		}
	}
	if worst > 3200/float64(rate) {
		t.Errorf("largest step is %.9f cycles, more than the top frequency justifies", worst)
	}
}

func TestAccumPhaseDegenerate(t *testing.T) {
	if got := AccumPhase(nil, SampleRate44100); len(got) != 0 {
		t.Errorf("no frequencies produced %d phases", len(got))
	}
	for _, v := range AccumPhase([]float64{100, 100}, 0) {
		if v != 0 {
			t.Errorf("a zero rate produced phase %.6f, want zeros", v)
		}
	}
}

func TestGlottalIsASawWithItsTopTakenOff(t *testing.T) {
	const rate = SampleRate44100
	freq := make([]float64, rate/5)
	for i := range freq {
		freq[i] = 220
	}
	ph := AccumPhase(freq, rate)
	raw := Glottal(ph, rate, 0)      // cutoff 0: Lowpass is a no-op, so this is the bare saw
	rolled := Glottal(ph, rate, 800) // and this is the same saw with a larynx's rolloff

	for i, v := range raw {
		if v < -1.0000001 || v > 1.0000001 {
			t.Fatalf("bare saw sample %d is %.6f, outside [-1,1]", i, v)
		}
	}
	band := func(s []float64, lo, hi float64) float64 {
		return BandEnergy(s, rate, lo, hi, 20)
	}
	if got := band(rolled, 180, 260) / band(raw, 180, 260); got < 0.9 {
		t.Errorf("the fundamental kept only %.2f of its energy, want the rolloff to leave it alone", got)
	}
	if got := band(rolled, 4000, 12000) / band(raw, 4000, 12000); got > 0.1 {
		t.Errorf("the top octaves kept %.2f of their energy, want the rolloff to take most of it", got)
	}
}
