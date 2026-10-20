// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"math"
	"strings"
	"testing"
)

func TestPatchParamsRoundTrip(t *testing.T) {
	want := Patch{
		Duration: 0.24,
		Peak:     0.9,
		Seed:     0xC0FF,
		Layers: []Layer{
			{
				Name: "body", Wave: WaveSquare, Freq: 300, SlideTo: 110,
				Level: 0.5, SlideCurve: 1.5,
				Env:  Env{Kind: EnvExpDecay, Rate: 14},
				AM:   &LFO{Hz: 30, Base: 0.8, Depth: 0.2},
				Gate: &Gate{From: 0.01, To: 0.2},
				Gain: 0.75,
				Partials: []Partial{
					{Ratio: 1.5, Gain: 0.22, Wave: WaveTriangle, Decay: 9},
					{Ratio: BarModeRatio, Gain: 0.42},
				},
			},
			{
				Wave: WaveNoise, Sweep: SweepPhase, NoiseMix: 0.3,
				Env:    Env{Kind: EnvAttackDecay, Rate: 40, AttackS: 0.004},
				Filter: Filter{Kind: FilterBandPass, Hz: 380, SweepTo: 900, Arc: true, Q: 1.1, Gain: 2.2, BeforeEnv: true},
			},
		},
	}

	raw, err := want.MarshalParams("squash")
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalPatchParams(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("a patch document did not read back as a patch")
	}
	requireSameSamples(t, want.Render(SampleRate44100), got.Render(SampleRate44100))
}

func TestPatchParamsIgnoresOtherGenerators(t *testing.T) {
	for _, raw := range []string{`{}`, `{"kind":"vocal","sourceHz":1600,"syllables":[]}`, `{"hz":880,"ms":60}`} {
		got, err := UnmarshalPatchParams([]byte(raw))
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if got != nil {
			t.Fatalf("%s: read back as a patch", raw)
		}
	}
	if _, err := UnmarshalPatchParams([]byte(`{"kind":"patch","layers":[{"env":{"kind":"wobble"}}]}`)); err == nil {
		t.Fatal("an unknown envelope name was accepted rather than refused")
	}
}

func TestPatchParamsRoundTripCascadeAndPhaseLead(t *testing.T) {
	want := Patch{
		Duration: 0.1, Seed: 7, Peak: 0.9,
		Layers: []Layer{
			{
				Wave: WaveNoise, Gain: 0.3,
				Env: Env{Kind: EnvExpDecay, Rate: 200},
				Filters: []Filter{
					{Kind: FilterLowpass, Hz: 9000, BeforeEnv: true},
					{Kind: FilterLowpass, Hz: 9000, BeforeEnv: true},
					{Kind: FilterHighpass, Hz: 1800, BeforeEnv: true},
					{Kind: FilterBandPass, Hz: 1200, Q: 2, Gain: 1.5},
				},
			},
			{
				Freq: 500, SlideTo: 800, Sweep: SweepPhaseLead,
				Partials: []Partial{{Ratio: 2, Gain: 0.25}},
				Env:      Env{Kind: EnvExpDecay, Rate: 12},
			},
		},
	}
	raw, err := want.MarshalParams("nail")
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{`"filters":[`, `"sweep":"phaselead"`} {
		if !strings.Contains(string(raw), word) {
			t.Fatalf("document lacks %s:\n%s", word, raw)
		}
	}
	got, err := UnmarshalPatchParams(raw)
	if err != nil {
		t.Fatal(err)
	}
	requireSameSamples(t, want.Render(SampleRate44100), got.Render(SampleRate44100))

	one := Patch{Duration: 0.05, Layers: []Layer{{Wave: WaveNoise, Env: Env{Kind: EnvConst},
		Filters: []Filter{{Kind: FilterLowpass, Hz: 3000}}}}}
	raw, err = one.MarshalParams("one")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"filter":{`) || strings.Contains(string(raw), `"filters"`) {
		t.Fatalf("a cascade of one stage is not written as the one-stage shorthand:\n%s", raw)
	}

	both, err := UnmarshalPatchParams([]byte(`{"kind":"patch","durationS":0.05,"seed":5,"layers":[{"wave":"noise","env":{"kind":"expdecay","rate":60},` +
		`"filter":{"kind":"lowpass","hz":3000,"beforeEnv":true},"filters":[{"kind":"highpass","hz":300}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	chain := Patch{Duration: 0.05, Seed: 5, Layers: []Layer{{Wave: WaveNoise, Env: Env{Kind: EnvExpDecay, Rate: 60},
		Filters: []Filter{{Kind: FilterLowpass, Hz: 3000, BeforeEnv: true}, {Kind: FilterHighpass, Hz: 300}}}}}
	requireSameSamples(t, chain.Render(SampleRate44100), both.Render(SampleRate44100))

	for what, doc := range map[string]string{
		"an unknown sweep word":          `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"sweep":"phaselag"}]}`,
		"an unknown word inside filters": `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"filters":[{"kind":"notch","hz":100}]}]}`,
		"a cascade out of signal order":  `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"filters":[{"kind":"lowpass","hz":100},{"kind":"highpass","hz":50,"beforeEnv":true}]}]}`,
	} {
		if _, err := UnmarshalPatchParams([]byte(doc)); err == nil {
			t.Errorf("%s was accepted rather than refused", what)
		}
	}
}

func TestPatchParamsFadeOut(t *testing.T) {
	want := Patch{
		Duration: 0.1, Peak: 0.6, FadeOutS: 0.004, Seed: 3,
		Layers: []Layer{{Wave: WaveNoise, Env: Env{Kind: EnvExpDecay, Rate: 20}}},
	}
	raw, err := want.MarshalParams("tail")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"fadeOutS":0.004`) {
		t.Fatalf("document lacks the fade:\n%s", raw)
	}
	got, err := UnmarshalPatchParams(raw)
	if err != nil {
		t.Fatal(err)
	}
	requireSameSamples(t, want.Render(SampleRate44100), got.Render(SampleRate44100))

	plain := want
	plain.FadeOutS = 0
	raw, err = plain.MarshalParams("tail")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "fadeOutS") {
		t.Fatalf("a patch without a fade wrote one:\n%s", raw)
	}

	for what, doc := range map[string]string{
		"a negative fade":              `{"kind":"patch","durationS":0.1,"fadeOutS":-0.01,"layers":[]}`,
		"a fade longer than the patch": `{"kind":"patch","durationS":0.1,"fadeOutS":0.2,"layers":[]}`,
	} {
		if _, err := UnmarshalPatchParams([]byte(doc)); err == nil {
			t.Errorf("%s was accepted rather than refused", what)
		}
	}
	if _, err := UnmarshalPatchParams([]byte(`{"kind":"patch","durationS":0.1,"fadeOutS":0.1,"layers":[]}`)); err != nil {
		t.Errorf("a fade exactly the patch's length was refused: %v", err)
	}
}

func TestPatchParamsSlideAndDuty(t *testing.T) {
	want := Patch{
		Duration: 0.2, Peak: 0.9,
		Layers: []Layer{
			{Wave: WaveSquare, Duty: 0.3, Freq: 420, SlideTo: 820, SlideS: 0.05, Env: Env{Kind: EnvConst}},
			{Freq: 262, SlideTo: 200, SlideRate: 28, Sweep: SweepPhase, Env: Env{Kind: EnvExpDecay, Rate: 20}},
		},
	}
	raw, err := want.MarshalParams("drop")
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{`"duty":0.3`, `"slideS":0.05`, `"slideRate":28`} {
		if !strings.Contains(string(raw), word) {
			t.Fatalf("document lacks %s:\n%s", word, raw)
		}
	}
	got, err := UnmarshalPatchParams(raw)
	if err != nil {
		t.Fatal(err)
	}
	requireSameSamples(t, want.Render(SampleRate44100), got.Render(SampleRate44100))

	plain, err := Patch{Duration: 0.1, Layers: []Layer{{Wave: WaveSquare, Freq: 440, Env: Env{Kind: EnvConst}}}}.MarshalParams("sq")
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"duty", "slideS", "slideRate"} {
		if strings.Contains(string(plain), word) {
			t.Fatalf("a plain square wrote %s:\n%s", word, plain)
		}
	}

	for what, doc := range map[string]string{
		"a duty of 1":                 `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"wave":"square","hz":440,"duty":1}]}`,
		"a negative duty":             `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"wave":"square","hz":440,"duty":-0.2}]}`,
		"a negative glide length":     `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"hz":440,"slideToHz":880,"slideS":-0.1}]}`,
		"a negative slide rate":       `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"hz":440,"slideToHz":880,"slideRate":-3}]}`,
		"a slide rate beside a curve": `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"hz":440,"slideToHz":880,"slideRate":3,"slideCurve":2}]}`,
		"a slide rate beside a glide": `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"hz":440,"slideToHz":880,"slideRate":3,"slideS":0.05}]}`,
	} {
		if _, err := UnmarshalPatchParams([]byte(doc)); err == nil {
			t.Errorf("%s was accepted rather than refused", what)
		}
	}
}

func TestPatchParamsSlideToZero(t *testing.T) {
	const dur, rate = 0.3, 3.9
	fall := Patch{Duration: dur, Peak: 0.9, Layers: []Layer{{
		Wave: WaveSquare, Freq: 650, SlideToZero: true, SlideRate: rate, Sweep: SweepPhaseLead, Env: Env{Kind: EnvConst},
	}}}
	raw, err := fall.MarshalParams("drop")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"hz":650,"slideToHz":0,"slideRate":3.9,`) {
		t.Fatalf("a rest at zero hertz is not written as 0:\n%s", raw)
	}
	got, err := UnmarshalPatchParams(raw)
	if err != nil {
		t.Fatal(err)
	}
	l := got.Layers[0]
	if !l.SlideToZero || l.SlideTo != 0 {
		t.Errorf("read back as SlideTo %v, SlideToZero %v", l.SlideTo, l.SlideToZero)
	}
	if f, want := l.freqAt(dur, dur), 650*ExpDecay(dur, rate); f != want {
		t.Errorf("pitch at the end is %v, want %v", f, want)
	}
	requireSameSamples(t, fall.Render(SampleRate44100), got.Render(SampleRate44100))
	if again, _ := got.MarshalParams("drop"); string(again) != string(raw) {
		t.Errorf("a rest at zero hertz does not re-marshal as written:\n got %s\nwant %s", again, raw)
	}

	for what, doc := range map[string]string{
		"a rate":  `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"hz":440,"slideRate":3}]}`,
		"a curve": `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"hz":440,"slideCurve":2,"slideS":0.05}]}`,
	} {
		p, err := UnmarshalPatchParams([]byte(doc))
		if err != nil {
			t.Fatal(err)
		}
		if p.Layers[0].slides() || p.Layers[0].freqAt(0.05, 0.1) != 440 {
			t.Errorf("%s without a slideToHz moved the pitch", what)
		}
		if again, _ := p.MarshalParams(""); strings.Contains(string(again), "slideToHz") {
			t.Errorf("%s without a slideToHz grew one:\n%s", what, again)
		}
	}

	toZero, err := UnmarshalPatchParams([]byte(`{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"hz":440,"slideToHz":0}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if f := toZero.Layers[0].freqAt(0.05, 0.1); f != 220 {
		t.Errorf("a power-law slide to zero is at %v halfway, want 220", f)
	}
	if _, err := UnmarshalPatchParams([]byte(`{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"hz":440,"slideToHz":-5}]}`)); err == nil {
		t.Error("a negative slideToHz was accepted rather than refused")
	}

	plain := Patch{Duration: 0.1, Layers: []Layer{{Freq: 440, SlideTo: 220, Env: Env{Kind: EnvConst}}}}
	flagged := Patch{Duration: 0.1, Layers: []Layer{{Freq: 440, SlideTo: 220, SlideToZero: true, Env: Env{Kind: EnvConst}}}}
	a, _ := plain.MarshalParams("x")
	b, _ := flagged.MarshalParams("x")
	if string(a) != string(b) {
		t.Errorf("SlideToZero beside a positive SlideTo changed the document:\n%s\n%s", a, b)
	}
}

func TestPatchParamsVibratoAndGlideStart(t *testing.T) {
	want := Patch{
		Duration: 0.5, Peak: 0.9,
		Layers: []Layer{
			{Freq: 700, SlideTo: 950, SlideS: 0.25, Vibrato: &Vibrato{Hz: 9, Depth: 0.02}, Env: Env{Kind: EnvExpDecay, Rate: 4}},
			{Freq: 950, SlideTo: 770, SlideDelayS: 0.25, Vibrato: &Vibrato{Hz: 5, DepthHz: 14, Wave: WaveTriangle}, Env: Env{Kind: EnvExpDecay, Rate: 4}},
			{Freq: 440, SlideTo: 349, SlideDelayS: 0.3, SlideS: 0.2, Sweep: SweepPhase, Env: Env{Kind: EnvConst}},
		},
	}
	raw, err := want.MarshalParams("whine")
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{`"vibrato":{"hz":9,"depth":0.02}`, `"vibrato":{"hz":5,"depthHz":14,"wave":"triangle"}`, `"slideDelayS":0.25`, `"slideS":0.2,"slideDelayS":0.3`} {
		if !strings.Contains(string(raw), word) {
			t.Fatalf("document lacks %s:\n%s", word, raw)
		}
	}
	got, err := UnmarshalPatchParams(raw)
	if err != nil {
		t.Fatal(err)
	}
	requireSameSamples(t, want.Render(SampleRate44100), got.Render(SampleRate44100))

	sine := want
	sine.Layers = []Layer{{Freq: 340, Vibrato: &Vibrato{Hz: 5, DepthHz: 14}, Env: Env{Kind: EnvConst}}}
	tri := sine
	tri.Layers = []Layer{{Freq: 340, Vibrato: &Vibrato{Hz: 5, DepthHz: 14, Wave: WaveTriangle}, Env: Env{Kind: EnvConst}}}
	if a, b := sine.Render(SampleRate44100), tri.Render(SampleRate44100); a[1000] == b[1000] {
		t.Error("a triangle wobble rendered as a sine one")
	}
	raw, err = sine.MarshalParams("hum")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"vibrato":{"hz":5,"depthHz":14}}`) || strings.Contains(string(raw), "slideDelayS") {
		t.Fatalf("a plain sine wobble is not written as it always was:\n%s", raw)
	}

	held := Patch{Duration: 0.5, Layers: []Layer{{Freq: 440, SlideTo: 220, SlideDelayS: 0.3, Env: Env{Kind: EnvConst}}}}
	if f := held.Layers[0].freqAt(0.29, 0.5); f != 440 {
		t.Errorf("pitch before the glide start is %v, want 440", f)
	}
	if f := held.Layers[0].freqAt(0.4, 0.5); f != 330 {
		t.Errorf("pitch halfway through the delayed glide is %v, want 330", f)
	}

	for what, doc := range map[string]string{
		"a wobble with two widths":     `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"hz":440,"vibrato":{"hz":6,"depthHz":5,"depth":0.02}}]}`,
		"a wobble as wide as the note": `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"hz":440,"vibrato":{"hz":6,"depth":1}}]}`,
		"a negative wobble width":      `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"hz":440,"vibrato":{"hz":6,"depth":-0.1}}]}`,
		"a square wobble":              `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"hz":440,"vibrato":{"hz":6,"depthHz":5,"wave":"square"}}]}`,
		"a noise wobble":               `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"hz":440,"vibrato":{"hz":6,"depthHz":5,"wave":"noise"}}]}`,
		"a negative glide start":       `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const"},"hz":440,"slideToHz":880,"slideDelayS":-0.1}]}`,
	} {
		if _, err := UnmarshalPatchParams([]byte(doc)); err == nil {
			t.Errorf("%s was accepted rather than refused", what)
		}
	}
}

func TestPatchParamsStartLevel(t *testing.T) {
	want := Patch{
		Duration: 0.3, Peak: 0.5, Seed: 9,
		Layers: []Layer{
			{Wave: WaveNoise, Env: Env{Kind: EnvADSR, StartLevel: 0.2, ADSR: ADSR{AttackS: 0.15, SustainS: 0.1, ReleaseS: 0.05, SustainLevel: 1}}},
			{Freq: 1200, Env: Env{Kind: EnvAttackDecay, StartLevel: 0.4, AttackS: 0.02, Rate: 12}},
		},
	}
	raw, err := want.MarshalParams("gust")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw), `"startLevel":`) != 2 {
		t.Fatalf("document lacks the start levels:\n%s", raw)
	}
	got, err := UnmarshalPatchParams(raw)
	if err != nil {
		t.Fatal(err)
	}
	requireSameSamples(t, want.Render(SampleRate44100), got.Render(SampleRate44100))

	plain := want
	plain.Layers = []Layer{{Wave: WaveNoise, Env: Env{Kind: EnvADSR, ADSR: ADSR{AttackS: 0.15, SustainLevel: 1}}}}
	raw, err = plain.MarshalParams("gust")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "startLevel") {
		t.Fatalf("an attack from silence wrote a start level:\n%s", raw)
	}

	for what, doc := range map[string]string{
		"a start level of 1":            `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"adsr","attackS":0.05,"startLevel":1}}]}`,
		"a negative start level":        `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"adsr","attackS":0.05,"startLevel":-0.1}}]}`,
		"a start level with no attack":  `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"adsr","sustainS":0.1,"startLevel":0.2}}]}`,
		"a start level on an expdecay":  `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"expdecay","rate":10,"startLevel":0.2}}]}`,
		"a start level on a const":      `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"const","startLevel":0.2}}]}`,
		"a start level on a fadeout":    `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"fadeout","attackS":0.05,"startLevel":0.2}}]}`,
		"an attackdecay with no attack": `{"kind":"patch","durationS":0.1,"layers":[{"env":{"kind":"attackdecay","rate":10,"startLevel":0.2}}]}`,
	} {
		if _, err := UnmarshalPatchParams([]byte(doc)); err == nil {
			t.Errorf("%s was accepted rather than refused", what)
		}
	}
}

func TestPatchAbsolutePartialsAndNoiseAM(t *testing.T) {
	const rate = SampleRate44100
	const dur = 0.42

	n := int(math.Round(dur * rate))
	rng := NewRNG(33)
	want := make([]float64, n)
	for i := range want {
		tt := SampleTime(rate, i)
		s := Sine(tt, 260)
		for _, f := range []float64{420, 650, 900, 1300} {
			s += float64(1 * Sine(tt, f))
		}
		s *= 0.6 + 0.4*NoiseSample(rng)
		want[i] = float64(0.16*s) * ExpDecay(tt, 9)
	}

	got := Patch{
		Duration: dur, Seed: 33,
		Layers: []Layer{{
			Freq: 260, Gain: 0.16,
			Partials: []Partial{
				{Hz: 420, Gain: 1}, {Hz: 650, Gain: 1},
				{Hz: 900, Gain: 1}, {Hz: 1300, Gain: 1},
			},
			NoiseAM: &NoiseAM{Base: 0.6, Depth: 0.4},
			Env:     Env{Kind: EnvExpDecay, Rate: 9},
		}},
	}.Render(rate)

	requireSameSamples(t, want, got)
}

func TestPatchAbsolutePartialIgnoresTheSlide(t *testing.T) {
	base := Patch{
		Duration: 0.05,
		Layers: []Layer{{
			Freq: 200, SlideTo: 800,
			Partials: []Partial{{Hz: 500, Gain: 1}},
			Env:      Env{Kind: EnvConst},
		}},
	}
	moved := base
	moved.Layers = []Layer{base.Layers[0]}
	moved.Layers[0].Partials = []Partial{{Ratio: 2.5, Gain: 1}}

	a, b := base.Render(SampleRate44100), moved.Render(SampleRate44100)
	for i := range a {
		if a[i] != b[i] {
			return // they diverge, which is the point
		}
	}
	t.Fatal("an absolute partial tracked the slide as a ratio would")
}
