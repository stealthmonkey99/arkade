// SPDX-License-Identifier: GPL-2.0-or-later

package musiclib

import (
	"encoding/json"
	"fmt"
)

// SongParamsKind is the params.kind value a song node carries.
const SongParamsKind = "song"

type songWire struct {
	Kind   string      `json:"kind"`
	Name   string      `json:"name,omitempty"`
	BPM    float64     `json:"bpm"`
	Beats  float64     `json:"beats,omitempty"`
	Peak   float64     `json:"peak"`
	Tracks []trackWire `json:"tracks"`
	Echo   *echoWire   `json:"echo,omitempty"`
}

type trackWire struct {
	Name       string         `json:"name,omitempty"`
	Instrument instrumentWire `json:"instrument"`
	Gain       float64        `json:"gain,omitempty"`
	Notes      []noteWire     `json:"notes"`
}

type instrumentWire struct {
	Wave         string  `json:"wave"`
	Duty         float64 `json:"duty,omitempty"`
	AttackMs     float64 `json:"attackMs,omitempty"`
	DecayMs      float64 `json:"decayMs,omitempty"`
	SustainLevel float64 `json:"sustainLevel,omitempty"`
	ReleaseMs    float64 `json:"releaseMs,omitempty"`
	NoiseTone    float64 `json:"noiseTone,omitempty"`
	Gain         float64 `json:"gain,omitempty"`
	VibratoHz    float64 `json:"vibratoHz,omitempty"`
	VibratoSemis float64 `json:"vibratoSemis,omitempty"`
	LowpassHz    float64 `json:"lowpassHz,omitempty"`
	DetuneCents  float64 `json:"detuneCents,omitempty"`
}

type noteWire struct {
	Midi  int     `json:"midi"`
	Start float64 `json:"start,omitempty"`
	Dur   float64 `json:"dur"`
	Vel   float64 `json:"vel,omitempty"`
	Seed  int     `json:"seed,omitempty"`
}

// maxSeed is the largest 16-bit LFSR state; larger seeds are refused, not wrapped.
const maxSeed = 0xFFFF

type echoWire struct {
	DelayBeats float64 `json:"delayBeats,omitempty"`
	Feedback   float64 `json:"feedback,omitempty"`
	Mix        float64 `json:"mix,omitempty"`
}

// MarshalParams renders the song as a recipe node's params document, with name
// carried inside. Floats use their shortest round-tripping form, so the
// document reads back to the same Song; a NaN or infinity is an error. Zero
// fields are omitted except peak, which is always written.
func (s Song) MarshalParams(name string) (json.RawMessage, error) {
	w := songWire{
		Kind:   SongParamsKind,
		Name:   name,
		BPM:    s.BPM,
		Beats:  s.Beats,
		Peak:   s.PeakLevel(),
		Tracks: make([]trackWire, 0, len(s.Tracks)),
	}
	for _, tr := range s.Tracks {
		wave := tr.Inst.Wave.String()
		if wave == "" {
			return nil, fmt.Errorf("musiclib: track %q: wave %d has no name", tr.Name, tr.Inst.Wave)
		}
		tw := trackWire{
			Name: tr.Name,
			Gain: tr.Gain,
			Instrument: instrumentWire{
				Wave:         wave,
				Duty:         tr.Inst.Duty,
				AttackMs:     tr.Inst.AttackMs,
				DecayMs:      tr.Inst.DecayMs,
				SustainLevel: tr.Inst.SustainLevel,
				ReleaseMs:    tr.Inst.ReleaseMs,
				NoiseTone:    tr.Inst.NoiseTone,
				Gain:         tr.Inst.Gain,
				VibratoHz:    tr.Inst.VibratoHz,
				VibratoSemis: tr.Inst.VibratoSemis,
				LowpassHz:    tr.Inst.LowpassHz,
				DetuneCents:  tr.Inst.DetuneCents,
			},
			Notes: make([]noteWire, 0, len(tr.Notes)),
		}
		for j, n := range tr.Notes {
			if n.Seed < 0 || n.Seed > maxSeed {
				return nil, fmt.Errorf("musiclib: track %q note %d: seed %d is not a 16-bit LFSR state", tr.Name, j, n.Seed)
			}
			tw.Notes = append(tw.Notes, noteWire{Midi: n.Midi, Start: n.Start, Dur: n.Dur, Vel: n.Vel, Seed: n.Seed})
		}
		w.Tracks = append(w.Tracks, tw)
	}
	if s.Echo != (Echo{}) {
		w.Echo = &echoWire{DelayBeats: s.Echo.DelayBeats, Feedback: s.Echo.Feedback, Mix: s.Echo.Mix}
	}
	return json.Marshal(w)
}

// UnmarshalSongParams reads a song back out of a recipe node's params. It
// returns (nil, nil) for a document whose kind is not "song", and an error for
// unreadable JSON, an unknown wave or a seed outside 16 bits. Other numbers
// are not range-checked.
func UnmarshalSongParams(raw json.RawMessage) (*Song, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var probe struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("musiclib: song params: %w", err)
	}
	if probe.Kind != SongParamsKind {
		return nil, nil
	}
	var w songWire
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, fmt.Errorf("musiclib: song params: %w", err)
	}
	s := &Song{BPM: w.BPM, Beats: w.Beats, Peak: w.Peak}
	for i, tw := range w.Tracks {
		tr := Track{
			Name: tw.Name,
			Gain: tw.Gain,
			Inst: Instrument{
				Duty:         tw.Instrument.Duty,
				AttackMs:     tw.Instrument.AttackMs,
				DecayMs:      tw.Instrument.DecayMs,
				SustainLevel: tw.Instrument.SustainLevel,
				ReleaseMs:    tw.Instrument.ReleaseMs,
				NoiseTone:    tw.Instrument.NoiseTone,
				Gain:         tw.Instrument.Gain,
				VibratoHz:    tw.Instrument.VibratoHz,
				VibratoSemis: tw.Instrument.VibratoSemis,
				LowpassHz:    tw.Instrument.LowpassHz,
				DetuneCents:  tw.Instrument.DetuneCents,
			},
		}
		if tw.Instrument.Wave != "" {
			wave, ok := ParseWave(tw.Instrument.Wave)
			if !ok {
				return nil, fmt.Errorf("musiclib: track %d: %q is not a known wave", i, tw.Instrument.Wave)
			}
			tr.Inst.Wave = wave
		}
		for j, nw := range tw.Notes {
			if nw.Seed < 0 || nw.Seed > maxSeed {
				return nil, fmt.Errorf("musiclib: track %d note %d: seed %d is not a 16-bit LFSR state", i, j, nw.Seed)
			}
			tr.Notes = append(tr.Notes, Note{Midi: nw.Midi, Start: nw.Start, Dur: nw.Dur, Vel: nw.Vel, Seed: nw.Seed})
		}
		s.Tracks = append(s.Tracks, tr)
	}
	if e := w.Echo; e != nil {
		s.Echo = Echo{DelayBeats: e.DelayBeats, Feedback: e.Feedback, Mix: e.Mix}
	}
	return s, nil
}
