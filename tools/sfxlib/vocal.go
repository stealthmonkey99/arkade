// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import "math"

const (
	// vocalHum2 and vocalHum3 are the murmur's second and third harmonic levels.
	vocalHum2 = 0.35
	vocalHum3 = 0.12
	// vocalNasalDuck is how much of the murmur an open mouth takes away.
	vocalNasalDuck = 0.6
	// vocalEnvFloor is a syllable's level with the mouth shut.
	vocalEnvFloor = 0.6
	// vocalOnsetS is the opening fade, so a phrase does not start on a step.
	vocalOnsetS = 0.012
)

// Phone is one articulatory position, held for Ms including its glide.
type Phone struct {
	Ms float64
	// F1Hz and F2Hz are the formant frequencies. F1 rises as the jaw opens
	// (about 300 Hz shut, 640 open); F2 follows the tongue.
	F1Hz, F2Hz float64
	// Open is how much sound leaves by the mouth, 0 shut to 1 wide.
	Open float64
	// Nasal is how much leaves by the nose.
	Nasal float64
	// GlideMs is how long the articulators take to arrive, from the phone's
	// start; 0 cuts straight to it. On a syllable's first phone the formants
	// are already in place and Open and Nasal rise from 0 over it.
	GlideMs float64
}

// Syllable is a run of phones at one pitch, placed in the phrase.
type Syllable struct {
	// AtMs is where the syllable starts in the phrase; syllables may overlap.
	AtMs float64
	// Hz is the base pitch before the phrase's lift and sag.
	Hz float64
	// Gain scales the syllable; 0 reads as 1.
	Gain   float64
	Phones []Phone
}

// Vocal is a glottal source heard through two moving formants and a nasal
// murmur, saying a phrase.
type Vocal struct {
	// SourceHz is the source's lowpass corner; about 1600 reads as a person.
	SourceHz float64
	// F1Q and F2Q are the resonances' sharpness; 3.5 and 5 sound like a mouth.
	F1Q, F2Q float64
	// F2Gain is the second formant's level against the first.
	F2Gain float64
	// PitchLift is the fractional pitch rise with an open jaw.
	PitchLift float64
	// PitchSagPerS is the exponential pitch droop across a syllable.
	PitchSagPerS float64
	// Murmur is the nasal hum's level.
	Murmur float64
	// NasalHz is the murmur's corner with the mouth shut; NasalDuckHz is how
	// far it drops as the mouth opens.
	NasalHz, NasalDuckHz float64
	Syllables            []Syllable
	// Peak is the normalization target; <= 0 leaves the phrase as rendered.
	Peak float64
}

// DefaultVocal is an ordinary voice with no syllables.
func DefaultVocal() Vocal {
	return Vocal{
		SourceHz: 1600, F1Q: 3.5, F2Q: 5, F2Gain: 0.5,
		PitchLift: 0.08, PitchSagPerS: 0.9, Murmur: 1.2,
		NasalHz: 650, NasalDuckHz: 300,
		Peak: 0.9,
	}
}

// LengthMs is the end of the last-ending syllable.
func (v Vocal) LengthMs() float64 {
	endMs := 0.0
	for _, s := range v.Syllables {
		if e := s.AtMs + s.lenMs(); e > endMs {
			endMs = e
		}
	}
	return endMs
}

// Render synthesizes the phrase and normalizes it to Peak. It is
// deterministic. A non-positive rate or an empty phrase renders nil.
func (v Vocal) Render(sampleRate int) []float64 {
	if sampleRate <= 0 || len(v.Syllables) == 0 {
		return nil
	}
	n := int(float64(sampleRate) * v.LengthMs() / 1000)
	if n <= 0 {
		return nil
	}
	out := make([]float64, n)
	for _, s := range v.Syllables {
		at := int(float64(sampleRate) * s.AtMs / 1000)
		MixAt(out, v.syllable(s, sampleRate), at)
	}
	if v.Peak > 0 {
		Normalize(out, v.Peak)
	}
	return out
}

func (s Syllable) lenMs() float64 {
	total := 0.0
	for _, p := range s.Phones {
		total += p.Ms
	}
	return total
}

func (v Vocal) syllable(s Syllable, sampleRate int) []float64 {
	if len(s.Phones) == 0 {
		return nil
	}
	totalMs := s.lenMs()
	n := int(float64(sampleRate) * totalMs / 1000)
	if n <= 0 {
		return nil
	}
	total := totalMs / 1000

	f1 := v.control(s, sampleRate, n, func(p Phone) float64 { return p.F1Hz })
	f2 := v.control(s, sampleRate, n, func(p Phone) float64 { return p.F2Hz })
	open := v.control(s, sampleRate, n, func(p Phone) float64 { return p.Open })
	nasal := v.control(s, sampleRate, n, func(p Phone) float64 { return p.Nasal })

	freq := make([]float64, n)
	nasalHz := make([]float64, n)
	for i := range freq {
		t := SampleTime(sampleRate, i)
		freq[i] = s.Hz * (1 + v.PitchLift*open[i]) * ExpDecay(t, v.PitchSagPerS)
		nasalHz[i] = v.NasalHz - v.NasalDuckHz*open[i]
	}

	phase := AccumPhase(freq, sampleRate)
	src := Glottal(phase, sampleRate, v.SourceHz)
	first := BandPass(src, sampleRate, f1, v.F1Q)
	second := BandPass(src, sampleRate, f2, v.F2Q)

	// The murmur bypasses the formants: harmonics of the same phase through a moving lowpass.
	hum := make([]float64, n)
	for i, ph := range phase {
		hum[i] = math.Sin(2*math.Pi*ph) + vocalHum2*math.Sin(4*math.Pi*ph) + vocalHum3*math.Sin(6*math.Pi*ph)
	}
	LowpassVarying(hum, sampleRate, nasalHz)

	gain := s.Gain
	if gain == 0 {
		gain = 1
	}
	releaseAt := total - s.Phones[len(s.Phones)-1].Ms/1000*0.6
	out := make([]float64, n)
	for i := range out {
		t := SampleTime(sampleRate, i)
		vowel := (first[i] + v.F2Gain*second[i]) * open[i]
		murmur := hum[i] * nasal[i] * (1 - vocalNasalDuck*open[i]) * v.Murmur
		env := Ramp(t, 0, vocalOnsetS) *
			(vocalEnvFloor + (1-vocalEnvFloor)*open[i]) *
			(1 - Ramp(t, releaseAt, total))
		out[i] = (vowel + murmur) * env * gain
	}
	return out
}

// control builds one articulatory signal across a syllable: each phone's
// value, reached over its glide. The first phone starts with the formants
// already in place.
func (v Vocal) control(s Syllable, sampleRate, n int, pick func(Phone) float64) []float64 {
	out := make([]float64, n)
	bounds := make([]float64, len(s.Phones)+1)
	for i, p := range s.Phones {
		bounds[i+1] = bounds[i] + p.Ms/1000
	}
	rest := Phone{F1Hz: s.Phones[0].F1Hz, F2Hz: s.Phones[0].F2Hz}
	for i := range out {
		t := SampleTime(sampleRate, i)
		idx := len(s.Phones) - 1
		for j := range s.Phones {
			if t < bounds[j+1] {
				idx = j
				break
			}
		}
		cur := pick(s.Phones[idx])
		g := s.Phones[idx].GlideMs / 1000
		if idx == 0 && g <= 0 {
			out[i] = cur
			continue
		}
		prev := pick(rest)
		if idx > 0 {
			prev = pick(s.Phones[idx-1])
		}
		r := Ramp(t, bounds[idx], bounds[idx]+g)
		out[i] = prev + (cur-prev)*r
	}
	return out
}
