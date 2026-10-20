// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"fmt"
	"math"
)

// GoertzelPower returns |X(f)|^2 for the single DFT bin at f Hz.
func GoertzelPower(samples []float64, sampleRate, f float64) float64 {
	w := 2 * math.Pi * f / sampleRate
	c := 2 * math.Cos(w)
	var s0, s1, s2 float64
	for _, x := range samples {
		s0 = x + c*s1 - s2
		s2 = s1
		s1 = s0
	}
	return s1*s1 + s2*s2 - c*s1*s2
}

// DefaultBandStep is the bin spacing in Hz used when a step <= 0 is passed.
const DefaultBandStep = 20.0

// BandEnergy sums GoertzelPower over bins from lo to hi Hz, step Hz apart.
func BandEnergy(samples []float64, sampleRate, lo, hi, step float64) float64 {
	if step <= 0 {
		step = DefaultBandStep
	}
	e := 0.0
	for f := lo; f <= hi; f += step {
		e += GoertzelPower(samples, sampleRate, f)
	}
	return e
}

// BandFraction returns the fraction of energy in [lo,hi] Hz relative to the
// total over [allLo,allHi] Hz, or 0 if that range holds none.
func BandFraction(samples []float64, sampleRate, lo, hi, allLo, allHi, step float64) float64 {
	all := BandEnergy(samples, sampleRate, allLo, allHi, step)
	if all == 0 {
		return 0
	}
	return BandEnergy(samples, sampleRate, lo, hi, step) / all
}

// SpectrumCheck asserts that the fraction of a sound's energy in [Lo,Hi] Hz
// lies within [Min,Max]. Max <= 0 means no upper bound and Min of 0 means no
// lower bound. Name labels the check in error text.
type SpectrumCheck struct {
	Name     string
	Lo, Hi   float64
	Min, Max float64
}

// AssertSpectrum evaluates every check against samples, with fractions taken
// relative to the energy in [allLo,allHi] Hz, and returns one error listing
// every violation, or nil. Run it on every generated sound before writing it,
// so a sound the cabinet's speakers cannot reproduce fails the build.
func AssertSpectrum(samples []float64, sampleRate, allLo, allHi float64, checks []SpectrumCheck) error {
	fracs := make([]float64, len(checks))
	for i, c := range checks {
		fracs[i] = BandFraction(samples, sampleRate, c.Lo, c.Hi, allLo, allHi, 0)
	}
	return checkFractions(fracs, checks)
}

// checkFractions is shared by the whole-signal and framed asserts so their
// error text agrees.
func checkFractions(fracs []float64, checks []SpectrumCheck) error {
	var violations []string
	for i, c := range checks {
		frac := fracs[i]
		if c.Max > 0 && frac > c.Max {
			violations = append(violations, fmt.Sprintf(
				"%s: %.0f%% of energy in %.0f-%.0fHz, want <=%.0f%%",
				c.Name, 100*frac, c.Lo, c.Hi, 100*c.Max))
		}
		if frac < c.Min {
			violations = append(violations, fmt.Sprintf(
				"%s: %.0f%% of energy in %.0f-%.0fHz, want >=%.0f%%",
				c.Name, 100*frac, c.Lo, c.Hi, 100*c.Min))
		}
	}
	if len(violations) == 0 {
		return nil
	}
	msg := "sfxlib: spectrum assertion failed:"
	for _, v := range violations {
		msg += "\n  " + v
	}
	return fmt.Errorf("%s", msg)
}
