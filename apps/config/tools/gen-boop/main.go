// SPDX-License-Identifier: GPL-2.0-or-later

// Command gen-boop synthesizes the volume-preview "boop" (internal/audio/boop.wav,
// 44.1kHz 16-bit mono) via tools/sfxlib. Regenerate with `go run ./tools/gen-boop`
// from apps/config.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/stealthmonkey99/arkade/tools/sfxlib"
)

const sampleRate = sfxlib.SampleRate44100

// below the usual 0.9: it plays at every volume level including 100%
const previewPeak = 0.7

// F#5: inside the 300-1200Hz band the cabinet's small speakers reproduce well.
const fundamental = 740.0

// 150ms total, finishing before a human can repeat-press up/down.
var envelope = sfxlib.ADSR{
	AttackS:      0.010,
	DecayS:       0.020,
	SustainS:     0.050,
	ReleaseS:     0.070,
	SustainLevel: 0.55,
}

func main() {
	_, thisFile, _, _ := runtime.Caller(0)
	outDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "internal", "audio"))

	samples := gen()
	sfxlib.Normalize(samples, previewPeak)
	assertSpectrum(samples)

	path := filepath.Join(outDir, "boop.wav")
	if err := sfxlib.WriteFile(path, samples, sampleRate); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s (%d samples, %.0fms)\n", path, len(samples), 1000*float64(len(samples))/sampleRate)
}

func gen() []float64 {
	n := int(envelope.Duration()*sampleRate) + 1
	out := make([]float64, n)
	for i := range out {
		t := sfxlib.SampleTime(sampleRate, i)
		env := envelope.At(t)
		v := sfxlib.Sine(t, fundamental) + 0.18*sfxlib.Sine(t, fundamental*2)
		out[i] = v * env
	}
	return out
}

// assertSpectrum applies the small-speaker rule from GAME_AUTHORING_AI.md.
func assertSpectrum(samples []float64) {
	err := sfxlib.AssertSpectrum(samples, sampleRate, 20, 10000, []sfxlib.SpectrumCheck{
		{Name: "sub-150Hz", Lo: 20, Hi: 150, Max: 0.5},
		{Name: "mid-200-2000Hz", Lo: 200, Hi: 2000, Min: 0.3},
	})
	if err != nil {
		fatal(err)
	}
	low := sfxlib.BandFraction(samples, sampleRate, 20, 150, 20, 10000, 0)
	mid := sfxlib.BandFraction(samples, sampleRate, 200, 2000, 20, 10000, 0)
	fmt.Printf("boop spectrum ok: %.0f%% <150Hz, %.0f%% in 200-2000Hz\n", 100*low, 100*mid)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "gen-boop:", err)
	os.Exit(1)
}
