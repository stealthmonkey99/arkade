// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"math"
	"testing"
)

func basicVoice() Voice {
	return Voice{
		Wave: WaveSine, Freq: 880, Duration: 0.25,
		Attack: 0.005, Decay: 0.05, Release: 0.05, SustainLevel: 0.4,
		Peak: 0.9, Seed: 0x51DE,
	}
}

func TestVoiceRenderIsDeterministic(t *testing.T) {
	v := basicVoice()
	v.Wave = WaveNoise // the only stage with a random source in it
	a := v.Render(SampleRate44100)
	b := v.Render(SampleRate44100)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("lengths %d and %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d differs between renders: %v vs %v", i, a[i], b[i])
		}
	}
	v.Seed++
	if c := v.Render(SampleRate44100); c[len(c)/2] == a[len(a)/2] {
		t.Error("changing the seed did not change the noise")
	}
}

func TestVoiceObeysDurationAndPeak(t *testing.T) {
	v := basicVoice()
	s := v.Render(SampleRate44100)
	if want := int(math.Round(0.25 * SampleRate44100)); len(s) != want {
		t.Errorf("rendered %d samples, want %d", len(s), want)
	}
	peak := 0.0
	for _, x := range s {
		peak = math.Max(peak, math.Abs(x))
	}
	if math.Abs(peak-0.9) > 1e-6 {
		t.Errorf("peak = %v, want 0.9", peak)
	}
	if s[0] != 0 {
		t.Errorf("first sample = %v, want silence (the attack starts at 0)", s[0])
	}
	if s[len(s)-1] != 0 {
		t.Errorf("last sample = %v, want silence", s[len(s)-1])
	}
}

func TestVoiceEnvelopeFitsInsideDuration(t *testing.T) {
	v := Voice{Duration: 0.1, Attack: 0.2, Decay: 0.2, Release: 0.1, Freq: 440, Peak: 0.9}
	e := v.Envelope()
	if math.Abs(e.Duration()-0.1) > 1e-9 {
		t.Errorf("squeezed envelope lasts %v s, want 0.1", e.Duration())
	}
	if e.AttackS <= 0 || e.ReleaseS <= 0 {
		t.Errorf("squeezing dropped a stage: %+v", e)
	}
	v2 := Voice{Duration: 1, Attack: 0.1, Decay: 0.2, Release: 0.3}
	if got := v2.Envelope().SustainS; math.Abs(got-0.4) > 1e-9 {
		t.Errorf("sustain = %v s, want 0.4", got)
	}
}

func TestVoiceSlideMovesThePitch(t *testing.T) {
	const rate = SampleRate44100
	v := Voice{
		Wave: WaveSine, Freq: 220, SlideTo: 1760, SlideCurve: 1,
		Duration: 0.5, Attack: 0.001, Release: 0.001, SustainLevel: 1,
		Peak: 0.9, Seed: 1,
	}
	s := v.Render(rate)
	first, second := s[:len(s)/2], s[len(s)/2:]
	lowFirst := BandFraction(first, rate, 150, 700, 20, 10000, 0)
	lowSecond := BandFraction(second, rate, 150, 700, 20, 10000, 0)
	if lowFirst <= lowSecond {
		t.Errorf("low-band share did not fall across the slide: %.2f then %.2f", lowFirst, lowSecond)
	}
}

func TestVoiceWoodIsInharmonic(t *testing.T) {
	const rate = SampleRate44100
	v := Voice{
		Wave: WaveWood, Freq: 600, Duration: 0.2,
		Attack: 0.001, Decay: 0.15, Release: 0.04, SustainLevel: 0,
		Peak: 0.9, Seed: 7,
	}
	s := v.Render(rate)
	mode2 := BandEnergy(s, rate, 1600, 1700, 20)
	octave := BandEnergy(s, rate, 1160, 1240, 20)
	if mode2 <= octave {
		t.Errorf("bar-mode partial (%.3g) is not louder than the octave (%.3g); wood sounds metallic", mode2, octave)
	}
}

func TestVoiceHighpassRescuesASubBassSound(t *testing.T) {
	const rate = SampleRate44100
	v := Voice{
		Wave: WaveSine, Freq: 60, Duration: 0.3,
		Attack: 0.002, Decay: 0.1, Release: 0.05, SustainLevel: 0.5,
		NoiseMix: 0.3, Peak: 0.9, Seed: 3,
	}
	before := FrameSignal(v.Render(rate), rate, DefaultBandStep, 0).BandFraction(20, 150, 20, 10000)
	v.HighpassHz = 300
	after := FrameSignal(v.Render(rate), rate, DefaultBandStep, 0).BandFraction(20, 150, 20, 10000)
	if before < 0.5 {
		t.Fatalf("the 60 Hz fixture is not sub-bass heavy (%.2f)", before)
	}
	if after >= before {
		t.Errorf("highpass did not reduce the sub-150 Hz share: %.2f -> %.2f", before, after)
	}
}

func TestVoiceEmptyRenders(t *testing.T) {
	if s := (Voice{Freq: 440}).Render(SampleRate44100); s != nil {
		t.Errorf("a zero-duration voice rendered %d samples", len(s))
	}
	if s := basicVoice().Render(0); s != nil {
		t.Errorf("a zero sample rate rendered %d samples", len(s))
	}
}

func TestWaveNames(t *testing.T) {
	for i, name := range waveNames {
		w, ok := ParseWave(name)
		if !ok || w != Wave(i) {
			t.Errorf("ParseWave(%q) = %v,%v; want %v,true", name, w, ok, Wave(i))
		}
		if w.String() != name {
			t.Errorf("Wave(%d).String() = %q, want %q", i, w.String(), name)
		}
		if got := w.GoName(); got == "" {
			t.Errorf("Wave(%d) has no GoName", i)
		}
	}
	if _, ok := ParseWave("kazoo"); ok {
		t.Error("ParseWave accepted an unknown wave")
	}
}
