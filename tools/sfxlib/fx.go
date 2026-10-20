// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"math"

	"github.com/stealthmonkey99/arkade/assetfile"
)

// ApplyFx gives s the treatment a compose node's fx block describes, in
// the order assetfile.SoundFx lists its fields, and returns the treated
// samples. A nil fx returns s unchanged; s itself is never modified.
// The input is expected to have passed assetfile.ValidateSoundRecipe.
func ApplyFx(s []float64, rate int, fx *assetfile.SoundFx) []float64 {
	if fx == nil {
		return s
	}
	out := slice(s, rate, fx.StartMs, fx.EndMs)
	if fx.Reverse {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	if fx.Stretch != 0 && fx.Stretch != 1 && len(out) > 0 {
		if fx.Varispeed {
			out = Varispeed(out, 1/fx.Stretch)
		} else {
			out = TimeStretch(out, rate, fx.Stretch)
		}
	}
	out = PitchShift(out, rate, fx.PitchSemitones)
	if fx.HighpassHz > 0 {
		Highpass(out, rate, fx.HighpassHz)
		Highpass(out, rate, fx.HighpassHz)
	}
	if fx.LowpassHz > 0 {
		Lowpass(out, rate, fx.LowpassHz)
		Lowpass(out, rate, fx.LowpassHz)
	}
	FadeIn(out, rate, fx.FadeInMs/1000)
	FadeOut(out, rate, fx.FadeOutMs/1000)
	if fx.GainDb != 0 {
		g := (assetfile.SoundNode{GainDb: fx.GainDb}).Gain()
		for i := range out {
			out[i] *= g
		}
	}
	return out
}

// slice copies s between startMs and endMs (0 is the end), clamped to s.
func slice(s []float64, rate int, startMs, endMs float64) []float64 {
	lo := min(len(s), int(math.Round(startMs*float64(rate)/1000)))
	hi := len(s)
	if endMs > 0 {
		hi = max(lo, min(len(s), int(math.Round(endMs*float64(rate)/1000))))
	}
	return append([]float64(nil), s[lo:hi]...)
}

// FadeIn ramps the first sec seconds of s linearly up from zero, in
// place; the mirror of FadeOut.
func FadeIn(s []float64, sampleRate int, sec float64) {
	if len(s) == 0 || sec <= 0 || sampleRate <= 0 {
		return
	}
	n := min(len(s), int(math.Round(sec*float64(sampleRate))))
	for i := 0; i < n; i++ {
		s[i] *= float64(i) / float64(n)
	}
}
