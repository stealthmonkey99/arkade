// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import "fmt"

// Validate reports the first layer field combination the patch document
// grammar refuses, so a Go-literal patch meets the rules UnmarshalPatchParams
// enforces. FadeOutS is not checked.
func (p Patch) Validate() error {
	for i := range p.Layers {
		if err := p.Layers[i].validate(); err != nil {
			return fmt.Errorf("sfxlib: layer %d: %w", i, err)
		}
	}
	return nil
}

func (l *Layer) validate() error {
	if l.SlideTo < 0 {
		return fmt.Errorf("slideToHz %v is negative", l.SlideTo)
	}
	if l.SlideS < 0 {
		return fmt.Errorf("slideS %v is negative", l.SlideS)
	}
	if l.SlideDelayS < 0 {
		return fmt.Errorf("slideDelayS %v is negative", l.SlideDelayS)
	}
	if l.SlideRate < 0 {
		return fmt.Errorf("slideRate %v is negative", l.SlideRate)
	}
	if l.SlideRate > 0 && (l.SlideCurve != 0 || l.SlideS != 0) {
		return fmt.Errorf("slideRate excludes slideCurve and slideS")
	}
	if l.Duty < 0 || l.Duty >= 1 {
		return fmt.Errorf("duty %v is outside [0,1)", l.Duty)
	}
	if sl := l.Env.StartLevel; sl < 0 || sl >= 1 {
		return fmt.Errorf("startLevel %v is outside [0,1)", sl)
	}
	attack := l.Env.ADSR.AttackS
	if l.Env.Kind == EnvAttackDecay {
		attack = l.Env.AttackS
	}
	if l.Env.StartLevel > 0 && ((l.Env.Kind != EnvADSR && l.Env.Kind != EnvAttackDecay) || attack <= 0) {
		return fmt.Errorf("startLevel needs an attackS on an adsr or attackdecay envelope to rise from")
	}
	for pi, pa := range l.Partials {
		if pa.Wave == WaveNoise || pa.Wave == WaveWood {
			return fmt.Errorf("partial %d: %q cannot be a partial", pi, pa.Wave)
		}
	}
	if v := l.Vibrato; v != nil {
		if v.Depth < 0 || v.Depth >= 1 {
			return fmt.Errorf("vibrato depth %v is outside [0,1)", v.Depth)
		}
		if v.Depth != 0 && v.DepthHz != 0 {
			return fmt.Errorf("vibrato names its width twice, depth and depthHz")
		}
		if v.Wave != WaveSine && v.Wave != WaveTriangle {
			return fmt.Errorf("a vibrato is %q or %q, not %q", WaveSine, WaveTriangle, v.Wave)
		}
	}
	afterEnv := false
	for fi, f := range l.filterChain() {
		if afterEnv && f.BeforeEnv {
			return fmt.Errorf("filter stage %d is beforeEnv after a stage that is not; a cascade is written in signal order", fi)
		}
		afterEnv = afterEnv || !f.BeforeEnv
	}
	return nil
}
