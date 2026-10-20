// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"math"
	"testing"
)

func templateCases() map[string]Patch {
	return map[string]Patch{
		"tap":    Tap(440, 60, 0.09),
		"impact": Impact(380, 1.1, 11, []Tom{{Hz: 160, Gain: 0.5, Decay: 14}, {Hz: 90, Gain: 0.3, Decay: 18}}, 0.3),
		"sweep":  Sweep(280, 520, 9, 0.11),
		"bonk":   Bonk(BonkLowHz, BonkHighHz, WaveSquare),
		"scrape": Scrape(700, 4, 0.2),
		"motif":  Motif(Run([]float64{523.25, 659.25, 783.99, 1046.5}, 0.11, 3, WaveSquare), 5),
	}
}

func TestTemplatesRenderAudibly(t *testing.T) {
	for name, p := range templateCases() {
		got := p.Render(SampleRate44100)
		if len(got) == 0 {
			t.Errorf("%s: rendered nothing", name)
			continue
		}
		if peak := peakOf(got); peak < 0.01 {
			t.Errorf("%s: rendered near-silence (peak %v)", name, peak)
		}
		want := int(math.Round(p.Duration * SampleRate44100))
		if len(got) != want {
			t.Errorf("%s: %d samples, want %d", name, len(got), want)
		}
	}
}

func TestTemplatesSurviveARecipe(t *testing.T) {
	for name, p := range templateCases() {
		raw, err := p.MarshalParams(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		back, err := UnmarshalPatchParams(raw)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if back == nil {
			t.Errorf("%s: did not read back as a patch", name)
			continue
		}
		before, after := p.Render(SampleRate44100), back.Render(SampleRate44100)
		if len(before) != len(after) {
			t.Errorf("%s: %d samples became %d", name, len(before), len(after))
			continue
		}
		for i := range before {
			if before[i] != after[i] {
				t.Errorf("%s: sample %d moved through the recipe", name, i)
				break
			}
		}
	}
}

func TestRunLaysNotesEndToEnd(t *testing.T) {
	// Variables, not literals: a constant 0.1*3 folds exactly, unlike the
	// runtime multiply in Run.
	beat, lastScale := 0.1, 3.0
	notes := Run([]float64{300, 400, 500}, beat, lastScale, WaveSine)
	if len(notes) != 3 {
		t.Fatalf("got %d notes, want 3", len(notes))
	}
	at := 0.0
	for i, n := range notes {
		if n.Delay != at {
			t.Errorf("note %d starts at %v, want %v", i, n.Delay, at)
		}
		at += n.Duration
	}
	if notes[2].Duration != beat*lastScale {
		t.Errorf("the last note holds for %v, want %v", notes[2].Duration, beat*lastScale)
	}
	for i, n := range notes[:2] {
		if n.Chord != nil {
			t.Errorf("note %d carries the held chord; only the arrival should", i)
		}
	}
	if len(notes[2].Chord) != len(MotifChord) {
		t.Error("the arrival does not carry the held chord")
	}
}

func TestTapIsBodyThenClick(t *testing.T) {
	p := Tap(440, 60, 0.09)
	if len(p.Layers) != 2 {
		t.Fatalf("got %d layers, want 2", len(p.Layers))
	}
	if p.Layers[0].Wave == WaveNoise {
		t.Error("the body layer is noise")
	}
	if p.Layers[1].Wave != WaveNoise || p.Layers[1].Gate == nil {
		t.Error("the second layer is not a gated noise transient")
	}
}
