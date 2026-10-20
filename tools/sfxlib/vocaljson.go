// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"encoding/json"
	"fmt"
)

// VocalParamsKind is the params.kind value a vocal node carries.
const VocalParamsKind = "vocal"

// vocalWire is the JSON twin of Vocal. Times are in milliseconds, formants in
// hertz, and zero values that carry meaning in Go round-trip as written.
type vocalWire struct {
	Kind         string         `json:"kind"`
	Name         string         `json:"name,omitempty"`
	SourceHz     float64        `json:"sourceHz"`
	F1Q          float64        `json:"f1Q"`
	F2Q          float64        `json:"f2Q"`
	F2Gain       float64        `json:"f2Gain"`
	PitchLift    float64        `json:"pitchLift,omitempty"`
	PitchSagPerS float64        `json:"pitchSagPerS,omitempty"`
	Murmur       float64        `json:"murmur,omitempty"`
	NasalHz      float64        `json:"nasalHz,omitempty"`
	NasalDuckHz  float64        `json:"nasalDuckHz,omitempty"`
	Peak         float64        `json:"peak,omitempty"`
	Syllables    []syllableWire `json:"syllables"`
}

type syllableWire struct {
	AtMs   float64     `json:"atMs,omitempty"`
	Hz     float64     `json:"hz"`
	Gain   float64     `json:"gain,omitempty"`
	Phones []phoneWire `json:"phones"`
}

type phoneWire struct {
	Ms      float64 `json:"ms"`
	F1Hz    float64 `json:"f1Hz"`
	F2Hz    float64 `json:"f2Hz"`
	Open    float64 `json:"open,omitempty"`
	Nasal   float64 `json:"nasal,omitempty"`
	GlideMs float64 `json:"glideMs,omitempty"`
}

// MarshalParams renders the vocal as a recipe node's params document, with
// name carried inside it.
func (v Vocal) MarshalParams(name string) (json.RawMessage, error) {
	w := vocalWire{
		Kind:         VocalParamsKind,
		Name:         name,
		SourceHz:     v.SourceHz,
		F1Q:          v.F1Q,
		F2Q:          v.F2Q,
		F2Gain:       v.F2Gain,
		PitchLift:    v.PitchLift,
		PitchSagPerS: v.PitchSagPerS,
		Murmur:       v.Murmur,
		NasalHz:      v.NasalHz,
		NasalDuckHz:  v.NasalDuckHz,
		Peak:         v.Peak,
		Syllables:    make([]syllableWire, 0, len(v.Syllables)),
	}
	for _, s := range v.Syllables {
		sw := syllableWire{AtMs: s.AtMs, Hz: s.Hz, Gain: s.Gain, Phones: make([]phoneWire, 0, len(s.Phones))}
		for _, p := range s.Phones {
			sw.Phones = append(sw.Phones, phoneWire{
				Ms: p.Ms, F1Hz: p.F1Hz, F2Hz: p.F2Hz, Open: p.Open, Nasal: p.Nasal, GlideMs: p.GlideMs,
			})
		}
		w.Syllables = append(w.Syllables, sw)
	}
	return json.Marshal(w)
}

// UnmarshalVocalParams reads a vocal from a recipe node's params. A document
// that is not a vocal returns (nil, nil); unreadable JSON is an error.
func UnmarshalVocalParams(raw json.RawMessage) (*Vocal, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var probe struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("sfxlib: vocal params: %w", err)
	}
	if probe.Kind != VocalParamsKind {
		return nil, nil
	}
	var w vocalWire
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, fmt.Errorf("sfxlib: vocal params: %w", err)
	}
	v := &Vocal{
		SourceHz:     w.SourceHz,
		F1Q:          w.F1Q,
		F2Q:          w.F2Q,
		F2Gain:       w.F2Gain,
		PitchLift:    w.PitchLift,
		PitchSagPerS: w.PitchSagPerS,
		Murmur:       w.Murmur,
		NasalHz:      w.NasalHz,
		NasalDuckHz:  w.NasalDuckHz,
		Peak:         w.Peak,
	}
	for _, sw := range w.Syllables {
		s := Syllable{AtMs: sw.AtMs, Hz: sw.Hz, Gain: sw.Gain}
		for _, pw := range sw.Phones {
			s.Phones = append(s.Phones, Phone{
				Ms: pw.Ms, F1Hz: pw.F1Hz, F2Hz: pw.F2Hz, Open: pw.Open, Nasal: pw.Nasal, GlideMs: pw.GlideMs,
			})
		}
		v.Syllables = append(v.Syllables, s)
	}
	return v, nil
}
