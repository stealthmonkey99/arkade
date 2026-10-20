// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import "math"

// ExpDecay is a one-sided exponential decay envelope: 1 at t=0, decaying
// toward 0 as e^(-t*rate). Larger rate decays faster.
func ExpDecay(t, rate float64) float64 {
	return math.Exp(-t * rate)
}

// ADSR is a linear attack/decay/sustain/release envelope: it rises 0->1
// over AttackS, falls to SustainLevel over DecayS, holds for SustainS,
// then falls to 0 over ReleaseS. Any stage may be zero.
type ADSR struct {
	AttackS, DecayS, SustainS, ReleaseS float64
	// SustainLevel is the held value during SustainS, in [0,1].
	SustainLevel float64
}

// Duration is the envelope's total length in seconds.
func (e ADSR) Duration() float64 {
	return e.AttackS + e.DecayS + e.SustainS + e.ReleaseS
}

// At returns the envelope's value in [0,1] at t seconds; 0 outside
// [0, Duration()].
func (e ADSR) At(t float64) float64 {
	switch {
	case t < 0:
		return 0
	case t < e.AttackS:
		if e.AttackS <= 0 {
			return 1
		}
		return t / e.AttackS
	case t < e.AttackS+e.DecayS:
		if e.DecayS <= 0 {
			return e.SustainLevel
		}
		frac := (t - e.AttackS) / e.DecayS
		return 1 + (e.SustainLevel-1)*frac
	case t < e.AttackS+e.DecayS+e.SustainS:
		return e.SustainLevel
	case t < e.Duration():
		if e.ReleaseS <= 0 {
			return 0
		}
		rel := t - (e.AttackS + e.DecayS + e.SustainS)
		return e.SustainLevel * (1 - rel/e.ReleaseS)
	default:
		return 0
	}
}

// Ramp is a raised-cosine ramp from 0 to 1: 0 at or before t0, 1 at or
// after t1, zero slope at both ends. A window with t1 <= t0 is widened to a
// microsecond, so it acts as a step.
func Ramp(t, t0, t1 float64) float64 {
	d := t1 - t0
	if d < 1e-6 {
		d = 1e-6
	}
	u := (t - t0) / d
	switch {
	case u < 0:
		u = 0
	case u > 1:
		u = 1
	}
	return 0.5 - 0.5*math.Cos(math.Pi*u)
}
