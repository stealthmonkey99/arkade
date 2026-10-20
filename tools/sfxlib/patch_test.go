// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"math"
	"testing"
)

// The float64() conversions in the references block arm64 fused multiply-add,
// which would round differently from the renderer.

func requireSameSamples(t *testing.T, want, got []float64) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("length: want %d samples, got %d", len(want), len(got))
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("sample %d: want %v, got %v", i, want[i], got[i])
		}
	}
}

func TestPatchReproducesPointwiseChirp(t *testing.T) {
	const rate = SampleRate44100
	env := ADSR{AttackS: 0.004, DecayS: 0.02, SustainS: 0.01, ReleaseS: 0.045, SustainLevel: 0.8}

	want := make([]float64, int(env.Duration()*rate)+1)
	for i := range want {
		tt := SampleTime(rate, i)
		want[i] = Sine(tt, 350+200*(tt/env.Duration())) * env.At(tt)
	}
	Normalize(want, 0.9)

	got := Patch{
		Duration: env.Duration(),
		Peak:     0.9,
		Layers: []Layer{{
			Freq:    350,
			SlideTo: 550,
			Sweep:   SweepPointwise,
			Env:     Env{Kind: EnvADSR, ADSR: env},
		}},
	}.Render(rate)

	requireSameSamples(t, want, got)
}

func TestPatchReproducesPartialStack(t *testing.T) {
	const rate = SampleRate44100
	const freq, dur, decay = 440.0, 0.3, 13.0

	n := int(math.Round(dur * rate))
	want := make([]float64, n)
	for i := range want {
		tt := SampleTime(rate, i)
		s := Sine(tt, freq)
		s += float64(float64(0.34*Sine(tt, freq*2)) * ExpDecay(tt, 1.9*decay))
		s += float64(float64(0.15*Sine(tt, freq*3)) * ExpDecay(tt, 2.7*decay))
		want[i] = s * ExpDecay(tt, decay)
	}
	Normalize(want, 0.9)

	got := Patch{
		Duration: dur,
		Peak:     0.9,
		Layers: []Layer{{
			Freq: freq,
			Env:  Env{Kind: EnvExpDecay, Rate: decay},
			Partials: []Partial{
				{Ratio: 2, Gain: 0.34, Decay: 1.9 * decay},
				{Ratio: 3, Gain: 0.15, Decay: 2.7 * decay},
			},
		}},
	}.Render(rate)

	requireSameSamples(t, want, got)
}

func TestPatchReproducesGatedTransient(t *testing.T) {
	const rate = SampleRate44100
	const dur, clickS = 0.12, 0.006

	n := int(math.Round(dur * rate))
	clickN := int(math.Round(clickS * rate))
	rng := NewRNG(0x51A7)
	want := make([]float64, n)
	for i := range want {
		tt := SampleTime(rate, i)
		body := float64(Sine(tt, 620) * ExpDecay(tt, 24))
		var click float64
		if i < clickN {
			click = float64(0.5*NoiseSample(rng)) * ExpDecay(tt, 90)
		}
		want[i] = body + click
	}
	got := Patch{
		Duration: dur,
		Seed:     0x51A7,
		Layers: []Layer{
			{Freq: 620, Env: Env{Kind: EnvExpDecay, Rate: 24}},
			{
				Wave:     WaveNoise,
				Gain:     0.5,
				Duration: dur,
				Gate:     &Gate{To: clickS},
				Env:      Env{Kind: EnvExpDecay, Rate: 90},
			},
		},
	}.Render(rate)

	requireSameSamples(t, want, got)
}

func TestPatchLayerOrderIsObservable(t *testing.T) {
	const rate = SampleRate44100
	base := Patch{
		Duration: 0.05,
		Seed:     9,
		Layers: []Layer{
			{Wave: WaveNoise, Env: Env{Kind: EnvConst}},
			{Wave: WaveNoise, Gain: 0.5, Env: Env{Kind: EnvConst}},
		},
	}
	swapped := base
	swapped.Layers = []Layer{base.Layers[1], base.Layers[0]}

	a, b := base.Render(rate), swapped.Render(rate)
	same := true
	for i := range a {
		if a[i] != b[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("swapping two noise layers changed nothing: they are no longer sharing one stream in declared order")
	}
}

func TestPatchRenderIsDeterministic(t *testing.T) {
	p := Patch{
		Duration: 0.08,
		Peak:     0.9,
		Seed:     0xBEEF,
		Layers: []Layer{
			{Freq: 300, SlideTo: 900, Sweep: SweepPhase, Env: Env{Kind: EnvExpDecay, Rate: 18}},
			{Wave: WaveNoise, Gain: 0.3, Env: Env{Kind: EnvFadeOut, Curve: 2},
				Filter: Filter{Kind: FilterBandPass, Hz: 900, SweepTo: 1800, Q: 3, Gain: 2}},
		},
	}
	requireSameSamples(t, p.Render(SampleRate44100), p.Render(SampleRate44100))
}

func TestPatchEmpty(t *testing.T) {
	for _, p := range []Patch{
		{},
		{Duration: 0.1},
		{Duration: -1, Layers: []Layer{{Freq: 440}}},
	} {
		if got := p.Render(SampleRate44100); len(got) > 0 {
			for _, v := range got {
				if v != 0 {
					t.Fatalf("patch %+v rendered audible samples", p)
				}
			}
		}
	}
	if got := (Patch{Duration: 0.1, Layers: []Layer{{Freq: 440}}}).Render(0); got != nil {
		t.Fatal("a non-positive sample rate should render nothing")
	}
}

func TestPatchReproducesFilterCascade(t *testing.T) {
	const rate = SampleRate44100
	const dur, loHz, hiHz, decay = 0.006, 1975.54, 9000.0, 320.0

	n := int(math.Round(dur * rate))
	rng := NewRNG(0x3E17)
	aHi := 1 - math.Exp(-2*math.Pi*hiHz/rate)
	aLo := 1 - math.Exp(-2*math.Pi*loHz/rate)
	var lp1, lp2, hp float64
	want := make([]float64, n)
	for i := range want {
		x := float64(0.25 * NoiseSample(rng))
		lp1 += aHi * (x - lp1)
		lp2 += aHi * (lp1 - lp2)
		hp += aLo * (lp2 - hp)
		want[i] = float64(lp2-hp) * ExpDecay(SampleTime(rate, i), decay)
	}

	got := Patch{
		Duration: dur,
		Seed:     0x3E17,
		Layers: []Layer{{
			Wave: WaveNoise,
			Gain: 0.25,
			Env:  Env{Kind: EnvExpDecay, Rate: decay},
			Filters: []Filter{
				{Kind: FilterLowpass, Hz: hiHz, BeforeEnv: true},
				{Kind: FilterLowpass, Hz: hiHz, BeforeEnv: true},
				{Kind: FilterHighpass, Hz: loHz, BeforeEnv: true},
			},
		}},
	}.Render(rate)

	requireSameSamples(t, want, got)
}

func TestPatchCascadeStraddlesTheEnvelope(t *testing.T) {
	const rate = SampleRate44100
	const dur = 0.03

	n := int(math.Round(dur * rate))
	rng := NewRNG(5)
	want := make([]float64, n)
	for i := range want {
		want[i] = NoiseSample(rng)
	}
	Lowpass(want, rate, 3000)
	for i := range want {
		want[i] *= ExpDecay(SampleTime(rate, i), 60)
	}
	Highpass(want, rate, 300)

	shorthand := Layer{
		Wave:    WaveNoise,
		Env:     Env{Kind: EnvExpDecay, Rate: 60},
		Filter:  Filter{Kind: FilterLowpass, Hz: 3000, BeforeEnv: true},
		Filters: []Filter{{Kind: FilterHighpass, Hz: 300}},
	}
	cascade := shorthand
	cascade.Filter = Filter{}
	cascade.Filters = []Filter{{Kind: FilterLowpass, Hz: 3000, BeforeEnv: true}, {Kind: FilterHighpass, Hz: 300}}

	for _, l := range []Layer{shorthand, cascade} {
		got := Patch{Duration: dur, Seed: 5, Layers: []Layer{l}}.Render(rate)
		requireSameSamples(t, want, got)
	}
}

func TestPatchFadeOutClosesTheTail(t *testing.T) {
	const rate = SampleRate44100
	const dur, fadeS, peak = 0.1, 0.004, 0.6

	base := Patch{
		Duration: dur, Seed: 0x3E17,
		Layers: []Layer{
			{Freq: 987.77, Env: Env{Kind: EnvExpDecay, Rate: 32}},
			{Wave: WaveNoise, Duration: 0.006, Gain: 0.25, Env: Env{Kind: EnvExpDecay, Rate: 320}},
		},
	}
	want := base.Render(rate)
	n := int(math.Round(fadeS * rate))
	start := len(want) - n
	for i := start; i < len(want); i++ {
		want[i] *= float64(len(want)-1-i) / float64(n)
	}
	Normalize(want, peak)

	faded := base
	faded.FadeOutS, faded.Peak = fadeS, peak
	got := faded.Render(rate)
	requireSameSamples(t, want, got)
	if got[len(got)-1] != 0 {
		t.Fatalf("last sample is %v, want silence", got[len(got)-1])
	}

	ref := base.Render(rate)
	for i := range ref {
		ref[i] *= float64(len(ref)-1-i) / float64(len(ref))
	}
	whole := base
	whole.FadeOutS = 1
	requireSameSamples(t, ref, whole.Render(rate))
}

func TestPatchSwellsFromItsStartLevel(t *testing.T) {
	const rate = SampleRate44100
	const dur, attack, start = 0.1, 0.04, 0.25

	n := int(math.Round(dur * rate))
	adsr := ADSR{AttackS: attack, SustainS: 0.03, ReleaseS: 0.03, SustainLevel: 1}
	wantADSR := make([]float64, n)
	wantAD := make([]float64, n)
	for i := range wantADSR {
		tt := SampleTime(rate, i)
		onset := 1.0
		if tt < attack {
			onset = start + float64((1-start)*(tt/attack))
		}
		s := Sine(tt, 440)
		if tt < attack {
			wantADSR[i] = s * onset
		} else {
			wantADSR[i] = s * adsr.At(tt)
		}
		wantAD[i] = s * (onset * ExpDecay(tt, 30))
	}
	render := func(env Env) []float64 {
		return Patch{Duration: dur, Layers: []Layer{{Freq: 440, Env: env}}}.Render(rate)
	}
	requireSameSamples(t, wantADSR, render(Env{Kind: EnvADSR, ADSR: adsr, StartLevel: start}))
	requireSameSamples(t, wantAD, render(Env{Kind: EnvAttackDecay, AttackS: attack, Rate: 30, StartLevel: start}))

	plain := render(Env{Kind: EnvADSR, ADSR: adsr})
	for i := range plain {
		tt := SampleTime(rate, i)
		if want := Sine(tt, 440) * adsr.At(tt); plain[i] != want {
			t.Fatalf("sample %d: an attack from silence moved to %v, want %v", i, plain[i], want)
		}
	}
}

func TestPatchGlideTimeHoldsAfterArrival(t *testing.T) {
	const rate = SampleRate44100
	const f0, f1, dur, glide = 300.0, 500.0, 0.2, 0.05

	n := int(math.Round(dur * rate))
	want := make([]float64, n)
	for i := range want {
		tt := SampleTime(rate, i)
		u := tt / glide
		if u > 1 {
			u = 1
		}
		want[i] = Sine(tt, f0+(f1-f0)*u)
	}
	got := Patch{
		Duration: dur,
		Layers:   []Layer{{Freq: f0, SlideTo: f1, SlideS: glide, Env: Env{Kind: EnvConst}}},
	}.Render(rate)
	requireSameSamples(t, want, got)
}

func TestPatchReproducesPulseDuty(t *testing.T) {
	const rate = SampleRate44100
	const freq, dur, duty, decay = 1200.0, 0.05, 0.25, 55.0

	n := int(math.Round(dur * rate))
	want := make([]float64, n)
	ph := 0.0
	for i := range want {
		v := -1.0
		if ph < duty {
			v = 1
		}
		want[i] = v * ExpDecay(SampleTime(rate, i), decay)
		ph += freq / rate
		ph -= math.Floor(ph)
	}
	got := Patch{
		Duration: dur,
		Layers: []Layer{{
			Wave: WaveSquare, Duty: duty, Freq: freq, Sweep: SweepPhase,
			Env: Env{Kind: EnvExpDecay, Rate: decay},
		}},
	}.Render(rate)
	requireSameSamples(t, want, got)

	// Pointwise, duty 0.5 differs from the square at sample 147 of 1200 Hz,
	// where sin(8π) rounds negative.
	square := func(sweep SweepMode, duty float64) []float64 {
		return Patch{Duration: dur, Layers: []Layer{{
			Wave: WaveSquare, Duty: duty, Freq: freq, Sweep: sweep, Env: Env{Kind: EnvConst},
		}}}.Render(rate)
	}
	requireSameSamples(t, square(SweepPhase, 0), square(SweepPhase, 0.5))
	a, b := square(SweepPointwise, 0), square(SweepPointwise, 0.5)
	if a[147] == b[147] {
		t.Fatal("the pointwise square and a 0.5 pulse agreed on a cycle boundary; the documented edge has moved")
	}
}

func TestPatchReproducesExponentialSlide(t *testing.T) {
	const rate = SampleRate44100
	const floor, depth, bend, dur, decay = 200.0, 62.0, 28.0, 0.2, 20.0

	n := int(math.Round(dur * rate))
	want := make([]float64, n)
	ph := 0.0
	for i := range want {
		tt := SampleTime(rate, i)
		want[i] = math.Sin(2*math.Pi*ph) * ExpDecay(tt, decay)
		ph += (floor + depth*ExpDecay(tt, bend)) / rate
		if ph >= 1 {
			ph -= math.Floor(ph)
		}
	}
	got := Patch{
		Duration: dur,
		Layers: []Layer{{
			Freq: floor + depth, SlideTo: floor, SlideRate: bend, Sweep: SweepPhase,
			Env: Env{Kind: EnvExpDecay, Rate: decay},
		}},
	}.Render(rate)
	requireSameSamples(t, want, got)
}

func TestPatchReproducesPhaseLead(t *testing.T) {
	const rate = SampleRate44100
	const f0, f1, dur, decay = 620.0, 940.0, 0.11, 16.0

	n := int(math.Round(dur * rate))
	want := make([]float64, n)
	ph := 0.0
	for i := range want {
		tt := SampleTime(rate, i)
		f := f0 + (f1-f0)*(tt/dur)
		ph += f / rate
		if ph >= 1 {
			ph -= 1
		}
		want[i] = math.Sin(2*math.Pi*ph) * ExpDecay(tt, decay)
	}

	lead := Patch{
		Duration: dur,
		Layers: []Layer{{
			Freq: f0, SlideTo: f1, Sweep: SweepPhaseLead,
			Env: Env{Kind: EnvExpDecay, Rate: decay},
		}},
	}
	got := lead.Render(rate)
	requireSameSamples(t, want, got)

	atRest := lead
	atRest.Layers = []Layer{lead.Layers[0]}
	atRest.Layers[0].Sweep = SweepPhase
	if first := atRest.Render(rate)[0]; first != 0 {
		t.Fatalf("SweepPhase's first sample is %v, want silence at phase zero", first)
	}
	if got[0] == 0 {
		t.Fatal("SweepPhaseLead's first sample is silence: the phase did not advance before it was read")
	}
}
