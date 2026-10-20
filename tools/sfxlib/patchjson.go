// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"encoding/json"
	"fmt"
)

// PatchParamsKind is the params.kind value a patch node carries.
const PatchParamsKind = "patch"

// patchWire is the JSON twin of Patch. Enums cross as words, and fields whose
// zero reads as 1 in Go must round-trip exactly as written.
type patchWire struct {
	Kind     string      `json:"kind"`
	Name     string      `json:"name,omitempty"`
	Duration float64     `json:"durationS"`
	Peak     float64     `json:"peak,omitempty"`
	FadeOut  float64     `json:"fadeOutS,omitempty"`
	Seed     uint64      `json:"seed,omitempty"`
	Layers   []layerWire `json:"layers"`
}

type layerWire struct {
	Name     string  `json:"name,omitempty"`
	Duration float64 `json:"durationS,omitempty"`
	Delay    float64 `json:"delayS,omitempty"`
	Wave     string  `json:"wave,omitempty"`
	Freq     float64 `json:"hz,omitempty"`
	// Pointer: absent is no slide, 0 is a slide to zero hertz.
	SlideTo    *float64      `json:"slideToHz,omitempty"`
	SlideCurve float64       `json:"slideCurve,omitempty"`
	SlideS     float64       `json:"slideS,omitempty"`
	SlideDelay float64       `json:"slideDelayS,omitempty"`
	SlideRate  float64       `json:"slideRate,omitempty"`
	Sweep      string        `json:"sweep,omitempty"`
	Duty       float64       `json:"duty,omitempty"`
	Level      float64       `json:"level,omitempty"`
	Partials   []partialWire `json:"partials,omitempty"`
	NoiseMix   float64       `json:"noiseMix,omitempty"`
	Env        envWire       `json:"env"`
	Gate       *gateWire     `json:"gate,omitempty"`
	AM         *lfoWire      `json:"am,omitempty"`
	NoiseAM    *noiseAMWire  `json:"noiseAM,omitempty"`
	Vibrato    *vibratoWire  `json:"vibrato,omitempty"`
	Filter     *filterWire   `json:"filter,omitempty"`
	Filters    []filterWire  `json:"filters,omitempty"`
	Gain       float64       `json:"gain,omitempty"`
}

type partialWire struct {
	Ratio float64 `json:"ratio,omitempty"`
	Hz    float64 `json:"hz,omitempty"`
	Gain  float64 `json:"gain"`
	Wave  string  `json:"wave,omitempty"`
	Decay float64 `json:"decay,omitempty"`
}

type envWire struct {
	Kind         string  `json:"kind"`
	Attack       float64 `json:"attackS,omitempty"`
	StartLevel   float64 `json:"startLevel,omitempty"`
	Decay        float64 `json:"decayS,omitempty"`
	Sustain      float64 `json:"sustainS,omitempty"`
	Release      float64 `json:"releaseS,omitempty"`
	SustainLevel float64 `json:"sustainLevel,omitempty"`
	Rate         float64 `json:"rate,omitempty"`
	Curve        float64 `json:"curve,omitempty"`
}

type gateWire struct {
	From   float64 `json:"fromS,omitempty"`
	To     float64 `json:"toS,omitempty"`
	Period float64 `json:"periodS,omitempty"`
	On     float64 `json:"onS,omitempty"`
}

type vibratoWire struct {
	Hz      float64 `json:"hz"`
	DepthHz float64 `json:"depthHz,omitempty"`
	Depth   float64 `json:"depth,omitempty"`
	Wave    string  `json:"wave,omitempty"`
}

type noiseAMWire struct {
	Base  float64 `json:"base"`
	Depth float64 `json:"depth"`
}

type lfoWire struct {
	Hz    float64 `json:"hz"`
	Base  float64 `json:"base"`
	Depth float64 `json:"depth"`
}

type filterWire struct {
	Kind      string  `json:"kind"`
	Hz        float64 `json:"hz"`
	SweepTo   float64 `json:"sweepToHz,omitempty"`
	Arc       bool    `json:"arc,omitempty"`
	Q         float64 `json:"q,omitempty"`
	Gain      float64 `json:"gain,omitempty"`
	BeforeEnv bool    `json:"beforeEnv,omitempty"`
}

var envNames = map[EnvKind]string{
	EnvADSR:        "adsr",
	EnvExpDecay:    "expdecay",
	EnvAttackDecay: "attackdecay",
	EnvFadeOut:     "fadeout",
	EnvArch:        "arch",
	EnvConst:       "const",
}

var filterNames = map[FilterKind]string{
	FilterNone:     "none",
	FilterLowpass:  "lowpass",
	FilterHighpass: "highpass",
	FilterBandPass: "bandpass",
}

var sweepNames = map[SweepMode]string{
	SweepPointwise: "pointwise",
	SweepPhase:     "phase",
	SweepPhaseLead: "phaselead",
}

// lookup inverts a name table; an unknown word is an error.
func lookup[K comparable](table map[K]string, word string, what string) (K, error) {
	for k, v := range table {
		if v == word {
			return k, nil
		}
	}
	var zero K
	return zero, fmt.Errorf("sfxlib: %q is not a known %s", word, what)
}

// MarshalParams renders the patch as a recipe node's params document, with
// name carried inside it.
func (p Patch) MarshalParams(name string) (json.RawMessage, error) {
	w := patchWire{
		Kind:     PatchParamsKind,
		Name:     name,
		Duration: p.Duration,
		Peak:     p.Peak,
		FadeOut:  p.FadeOutS,
		Seed:     p.Seed,
		Layers:   make([]layerWire, 0, len(p.Layers)),
	}
	for _, l := range p.Layers {
		lw := layerWire{
			Name:       l.Name,
			Duration:   l.Duration,
			Delay:      l.Delay,
			Wave:       l.Wave.String(),
			Freq:       l.Freq,
			SlideCurve: l.SlideCurve,
			SlideS:     l.SlideS,
			SlideDelay: l.SlideDelayS,
			SlideRate:  l.SlideRate,
			Sweep:      sweepNames[l.Sweep],
			Duty:       l.Duty,
			Level:      l.Level,
			NoiseMix:   l.NoiseMix,
			Gain:       l.Gain,
			Env: envWire{
				Kind:         envNames[l.Env.Kind],
				Attack:       l.Env.ADSR.AttackS,
				StartLevel:   l.Env.StartLevel,
				Decay:        l.Env.ADSR.DecayS,
				Sustain:      l.Env.ADSR.SustainS,
				Release:      l.Env.ADSR.ReleaseS,
				SustainLevel: l.Env.ADSR.SustainLevel,
				Rate:         l.Env.Rate,
				Curve:        l.Env.Curve,
			},
		}
		if l.Env.Kind == EnvAttackDecay {
			lw.Env.Attack = l.Env.AttackS
		}
		if l.slides() {
			to := l.SlideTo
			lw.SlideTo = &to
		}
		for _, pa := range l.Partials {
			lw.Partials = append(lw.Partials, partialWire{
				Ratio: pa.Ratio, Hz: pa.Hz, Gain: pa.Gain, Wave: pa.Wave.String(), Decay: pa.Decay,
			})
		}
		if g := l.Gate; g != nil {
			lw.Gate = &gateWire{From: g.From, To: g.To, Period: g.Period, On: g.On}
		}
		if a := l.AM; a != nil {
			lw.AM = &lfoWire{Hz: a.Hz, Base: a.Base, Depth: a.Depth}
		}
		if nm := l.NoiseAM; nm != nil {
			lw.NoiseAM = &noiseAMWire{Base: nm.Base, Depth: nm.Depth}
		}
		if v := l.Vibrato; v != nil {
			lw.Vibrato = &vibratoWire{Hz: v.Hz, DepthHz: v.DepthHz, Depth: v.Depth}
			if v.Wave != WaveSine {
				lw.Vibrato.Wave = v.Wave.String()
			}
		}
		// One stage is written as "filter", a cascade as "filters", whichever Go fields held them.
		chain := l.filterChain()
		switch len(chain) {
		case 0:
		case 1:
			fw := filterWireOf(chain[0])
			lw.Filter = &fw
		default:
			for _, f := range chain {
				lw.Filters = append(lw.Filters, filterWireOf(f))
			}
		}
		w.Layers = append(w.Layers, lw)
	}
	return json.Marshal(w)
}

func filterWireOf(f Filter) filterWire {
	return filterWire{
		Kind: filterNames[f.Kind], Hz: f.Hz, SweepTo: f.SweepTo, Arc: f.Arc,
		Q: f.Q, Gain: f.Gain, BeforeEnv: f.BeforeEnv,
	}
}

func filterOf(fw filterWire) (Filter, error) {
	fk, err := lookup(filterNames, fw.Kind, "filter")
	if err != nil {
		return Filter{}, err
	}
	return Filter{
		Kind: fk, Hz: fw.Hz, SweepTo: fw.SweepTo, Arc: fw.Arc,
		Q: fw.Q, Gain: fw.Gain, BeforeEnv: fw.BeforeEnv,
	}, nil
}

// UnmarshalPatchParams reads a patch from a recipe node's params. A document
// that is not a patch returns (nil, nil).
func UnmarshalPatchParams(raw json.RawMessage) (*Patch, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var probe struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("sfxlib: patch params: %w", err)
	}
	if probe.Kind != PatchParamsKind {
		return nil, nil
	}
	var w patchWire
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, fmt.Errorf("sfxlib: patch params: %w", err)
	}
	if w.FadeOut < 0 || w.FadeOut > w.Duration {
		return nil, fmt.Errorf("sfxlib: patch params: fadeOutS %v is outside the patch's 0–%v s", w.FadeOut, w.Duration)
	}
	p := &Patch{Duration: w.Duration, Peak: w.Peak, FadeOutS: w.FadeOut, Seed: w.Seed}
	for i, lw := range w.Layers {
		l := Layer{
			Name: lw.Name, Duration: lw.Duration, Delay: lw.Delay,
			Freq: lw.Freq, SlideCurve: lw.SlideCurve, SlideS: lw.SlideS, SlideDelayS: lw.SlideDelay,
			SlideRate: lw.SlideRate, Duty: lw.Duty, Level: lw.Level, NoiseMix: lw.NoiseMix, Gain: lw.Gain,
		}
		if to := lw.SlideTo; to != nil {
			l.SlideTo, l.SlideToZero = *to, *to == 0
		}
		if lw.Wave != "" {
			wave, ok := ParseWave(lw.Wave)
			if !ok {
				return nil, fmt.Errorf("sfxlib: layer %d: %q is not a known wave", i, lw.Wave)
			}
			l.Wave = wave
		}
		if lw.Sweep != "" {
			s, err := lookup(sweepNames, lw.Sweep, "sweep mode")
			if err != nil {
				return nil, fmt.Errorf("sfxlib: layer %d: %w", i, err)
			}
			l.Sweep = s
		}
		kind, err := lookup(envNames, lw.Env.Kind, "envelope")
		if err != nil {
			return nil, fmt.Errorf("sfxlib: layer %d: %w", i, err)
		}
		// attackS is the ADSR attack or the EnvAttackDecay onset, by kind.
		l.Env = Env{Kind: kind, Rate: lw.Env.Rate, Curve: lw.Env.Curve, StartLevel: lw.Env.StartLevel}
		if kind == EnvAttackDecay {
			l.Env.AttackS = lw.Env.Attack
		} else {
			l.Env.ADSR = ADSR{
				AttackS: lw.Env.Attack, DecayS: lw.Env.Decay,
				SustainS: lw.Env.Sustain, ReleaseS: lw.Env.Release,
				SustainLevel: lw.Env.SustainLevel,
			}
		}
		for pi, pw := range lw.Partials {
			pa := Partial{Ratio: pw.Ratio, Hz: pw.Hz, Gain: pw.Gain, Decay: pw.Decay}
			if pw.Wave != "" {
				wave, ok := ParseWave(pw.Wave)
				if !ok {
					return nil, fmt.Errorf("sfxlib: layer %d partial %d: %q is not a known wave", i, pi, pw.Wave)
				}
				pa.Wave = wave
			}
			l.Partials = append(l.Partials, pa)
		}
		if g := lw.Gate; g != nil {
			l.Gate = &Gate{From: g.From, To: g.To, Period: g.Period, On: g.On}
		}
		if a := lw.AM; a != nil {
			l.AM = &LFO{Hz: a.Hz, Base: a.Base, Depth: a.Depth}
		}
		if nm := lw.NoiseAM; nm != nil {
			l.NoiseAM = &NoiseAM{Base: nm.Base, Depth: nm.Depth}
		}
		if v := lw.Vibrato; v != nil {
			l.Vibrato = &Vibrato{Hz: v.Hz, DepthHz: v.DepthHz, Depth: v.Depth}
			if v.Wave != "" {
				wave, ok := ParseWave(v.Wave)
				if !ok {
					return nil, fmt.Errorf("sfxlib: layer %d: a vibrato is %q or %q, not %q", i, WaveSine, WaveTriangle, v.Wave)
				}
				l.Vibrato.Wave = wave
			}
		}
		if f := lw.Filter; f != nil {
			l.Filter, err = filterOf(*f)
			if err != nil {
				return nil, fmt.Errorf("sfxlib: layer %d: %w", i, err)
			}
		}
		for fi, fw := range lw.Filters {
			f, err := filterOf(fw)
			if err != nil {
				return nil, fmt.Errorf("sfxlib: layer %d filters[%d]: %w", i, fi, err)
			}
			l.Filters = append(l.Filters, f)
		}
		p.Layers = append(p.Layers, l)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}
