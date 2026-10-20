// SPDX-License-Identifier: GPL-2.0-or-later

package speech

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/stealthmonkey99/arkade/tools/sfxlib"
)

// MaxPitchSemitones bounds PitchSemitones either way.
const MaxPitchSemitones = 12

// ParamsKind is the params.kind value a speech node carries in a sound
// asset's recipe, beside sfxlib's "patch" and "vocal".
const ParamsKind = "speech"

// Defaults for the document, and the speed range the model honours.
const (
	DefaultVoice = "bm_george"
	DefaultSpeed = 1.0
	MinSpeed     = 0.5
	MaxSpeed     = 2.0
	DefaultPeak  = 0.9
)

// Params is one spoken line as data: what to say, who says it, and how
// the result is shaped for the cabinet. Its zero values read as the
// defaults: Voice "" is DefaultVoice, Speed 0 is DefaultSpeed, Peak 0 is
// DefaultPeak, HighpassHz and PitchSemitones 0 leave the voice alone, and
// BlendVoice "" speaks in Voice alone.
//
// BlendVoice and BlendWeight mix a second voice into the first by
// interpolating the two voices' style vectors: BlendWeight 0 is Voice
// alone, 1 is BlendVoice alone, and values between are a voice neither
// of them is. The words are read with Voice's accent at any weight.
// BlendWeight is clamped to [0, 1] and ignored without a BlendVoice.
type Params struct {
	Text           string
	Voice          string
	BlendVoice     string
	BlendWeight    float64
	Speed          float64
	PitchSemitones float64 // shifts pitch without changing length
	HighpassHz     float64 // applied twice, 12 dB/octave
	Peak           float64
}

type paramsWire struct {
	Kind        string  `json:"kind"`
	Name        string  `json:"name,omitempty"`
	Text        string  `json:"text"`
	Voice       string  `json:"voice,omitempty"`
	BlendVoice  string  `json:"blendVoice,omitempty"`
	BlendWeight float64 `json:"blendWeight,omitempty"`
	Speed       float64 `json:"speed,omitempty"`
	Pitch       float64 `json:"pitchSemitones,omitempty"`
	HighpassHz  float64 `json:"highpassHz,omitempty"`
	Peak        float64 `json:"peak,omitempty"`
}

// MarshalParams renders p as a recipe node's params document, with the
// sound's name carried inside. A Params without a BlendVoice writes no
// blend fields, so its document is the one an unblended line always had.
func (p Params) MarshalParams(name string) (json.RawMessage, error) {
	return json.Marshal(paramsWire{
		Kind: ParamsKind, Name: name, Text: p.Text, Voice: p.Voice,
		BlendVoice: p.BlendVoice, BlendWeight: p.BlendWeight,
		Speed: p.Speed, Pitch: p.PitchSemitones, HighpassHz: p.HighpassHz, Peak: p.Peak,
	})
}

// UnmarshalParams reads a recipe node's params. A document whose kind is
// not "speech" is not this package's and yields nil, nil.
func UnmarshalParams(raw json.RawMessage) (*Params, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var w paramsWire
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, fmt.Errorf("speech: params: %w", err)
	}
	if w.Kind != ParamsKind {
		return nil, nil
	}
	return &Params{
		Text: w.Text, Voice: w.Voice, BlendVoice: w.BlendVoice, BlendWeight: w.BlendWeight,
		Speed: w.Speed, PitchSemitones: w.Pitch, HighpassHz: w.HighpassHz, Peak: w.Peak,
	}, nil
}

// Resolved returns p with every default filled in, Speed clamped to
// [MinSpeed, MaxSpeed] (the range the model honours), PitchSemitones to
// ±MaxPitchSemitones, and BlendWeight to [0, 1], or to 0 when there is no
// BlendVoice.
func (p Params) Resolved() Params {
	if p.Voice == "" {
		p.Voice = DefaultVoice
	}
	if p.BlendVoice == "" || !(p.BlendWeight > 0) {
		p.BlendWeight = 0
	}
	p.BlendWeight = math.Min(1, p.BlendWeight)
	if p.Speed == 0 {
		p.Speed = DefaultSpeed
	}
	p.Speed = math.Min(MaxSpeed, math.Max(MinSpeed, p.Speed))
	if p.Peak <= 0 {
		p.Peak = DefaultPeak
	}
	p.Peak = math.Min(1, p.Peak)
	p.HighpassHz = math.Max(0, p.HighpassHz)
	p.PitchSemitones = math.Min(MaxPitchSemitones, math.Max(-MaxPitchSemitones, p.PitchSemitones))
	return p
}

// Render speaks p through r and shapes the result: pitch shift,
// high-pass, silence trimmed to 20 ms either side, 5 ms fades, normalized
// to Peak. Empty text renders as no samples.
func (p Params) Render(r *Renderer, sampleRate int) ([]float64, error) {
	p = p.Resolved()
	if p.Text == "" {
		return nil, nil
	}
	s, err := r.Speak(p.voice(), p.Text, sampleRate)
	if err != nil {
		return nil, err
	}
	return p.shape(s, sampleRate), nil
}

func (p Params) voice() Voice {
	return Voice{Name: p.Voice, Speed: p.Speed, Blend: p.BlendVoice, BlendWeight: p.BlendWeight}
}

func (p Params) shape(s []float64, rate int) []float64 {
	s = sfxlib.PitchShift(s, rate, p.PitchSemitones)
	if p.HighpassHz > 0 {
		sfxlib.Highpass(s, rate, p.HighpassHz)
		sfxlib.Highpass(s, rate, p.HighpassHz)
	}
	s = trim(s, rate)
	fade(s, rate*5/1000)
	sfxlib.Normalize(s, p.Peak)
	return s
}

// trim drops leading and trailing frames quieter than 5% of the loudest
// 5 ms frame, keeping 20 ms of the quiet either side.
func trim(s []float64, rate int) []float64 {
	n := rate * 5 / 1000
	if n == 0 || len(s) == 0 {
		return s
	}
	var frames []float64
	peak := 0.0
	for i := 0; i < len(s); i += n {
		end := min(i+n, len(s))
		sum := 0.0
		for _, v := range s[i:end] {
			sum += v * v
		}
		f := math.Sqrt(sum / float64(end-i))
		frames = append(frames, f)
		peak = math.Max(peak, f)
	}
	lo, hi := -1, -1
	for i, f := range frames {
		if f > 0.05*peak {
			if lo < 0 {
				lo = i
			}
			hi = i
		}
	}
	if lo < 0 {
		return nil
	}
	start := max(0, (lo-4)*n)
	end := min(len(s), (hi+5)*n)
	return append([]float64(nil), s[start:end]...)
}

func fade(s []float64, n int) {
	n = min(n, len(s)/2)
	for i := 0; i < n; i++ {
		g := float64(i) / float64(n)
		s[i] *= g
		s[len(s)-1-i] *= g
	}
}
