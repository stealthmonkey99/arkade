// SPDX-License-Identifier: GPL-2.0-or-later

// Command gen-stinger synthesizes the boot stinger (internal/audio/stinger.wav,
// 44.1kHz 16-bit mono, peak 0.35 FS) via tools/sfxlib. Regenerate with
// go run ./tools/gen-stinger from apps/boot.
//
// The stinger is a chord and also a calibration signal: the echo canceller
// (internal/mic) needs a clear onset to find the speaker-to-microphone delay
// and learns the path only at frequencies that have played, so an exponential
// sweep from 150 Hz to 7 kHz runs inside the chord's swell.
//
// Five voices (A2, E3, A3, C#4, E4) enter 60 ms apart under a 250 ms attack.
// The 0.7 s sweep sits inside that attack, where it is masked, a placement
// chosen by ear. It leaves the bands above 4 kHz near -60 dBFS, so the
// spectral gate carries the treble until games have played.
package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"

	"github.com/stealthmonkey99/arkade/tools/sfxlib"
)

const (
	sampleRate = sfxlib.SampleRate44100
	durationS  = 1.8
	sweepStart = 0.05
	sweepDur   = 0.7
	sweepLoHz  = 150.0
	sweepHiHz  = 7000.0
	sweepLevel = 0.9 // voices are 0.8 each, before normalization
	sweepCurve = 0.7 // <1 slows the top end, where a fixed-width band is a thin octave slice
)

var chord = []float64{220, 329.63, 440, 554.37, 659.25}

// gains for harmonics 2 upward
var harmonics = []float64{0.6, 0.5, 0.45, 0.4, 0.36, 0.33, 0.3, 0.28, 0.26, 0.24, 0.22, 0.2}

var voiceEnv = sfxlib.ADSR{
	AttackS:      0.250,
	DecayS:       0.200,
	SustainS:     0.600,
	ReleaseS:     0.600,
	SustainLevel: 0.7,
}

func main() {
	_, thisFile, _, _ := runtime.Caller(0)
	outDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "internal", "audio"))

	samples := gen()
	sfxlib.Normalize(samples, 0.35)
	assertSpectrum(samples)

	path := filepath.Join(outDir, "stinger.wav")
	if err := sfxlib.WriteFile(path, samples, sampleRate); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s (%d samples, %.0fms)\n", path, len(samples), 1000*float64(len(samples))/sampleRate)
}

func gen() []float64 {
	n := int(durationS*sampleRate) + 1
	out := make([]float64, n)
	for i, hz := range chord {
		start := float64(i) * 0.06
		for j := range out {
			t := sfxlib.SampleTime(sampleRate, j) - start
			if t < 0 {
				continue
			}
			env := voiceEnv.At(t)
			if env == 0 {
				continue
			}
			v := sfxlib.Sine(t, hz)
			for k, g := range harmonics {
				v += g * sfxlib.Sine(t, hz*float64(k+2))
			}
			out[j] += 0.8 * env * v
		}
	}
	// Exponential law: equal time per octave, roughly what the mel bands see.
	phase := 0.0
	ratio := math.Log(sweepHiHz / sweepLoHz)
	for j := range out {
		t := sfxlib.SampleTime(sampleRate, j) - sweepStart
		if t < 0 || t > sweepDur {
			continue
		}
		hz := sweepLoHz * math.Exp(ratio*math.Pow(t/sweepDur, sweepCurve))
		phase += 2 * math.Pi * hz / sampleRate
		fade := math.Max(0, math.Min(1, math.Min(t/0.03, (sweepDur-t)/0.06)))
		out[j] += sweepLevel * fade * math.Sin(phase)
	}
	return out
}

// assertSpectrum applies the small-speaker rule, then requires every 750 Hz
// band from 200 Hz to 6.2 kHz to sit above -62 dBFS, the faintest the chosen
// voicing reaches.
func assertSpectrum(samples []float64) {
	err := sfxlib.AssertSpectrum(samples, sampleRate, 20, 10000, []sfxlib.SpectrumCheck{
		{Name: "sub-150Hz", Lo: 20, Hi: 150, Max: 0.5},
		{Name: "mid-200-2000Hz", Lo: 200, Hi: 2000, Min: 0.3},
	})
	if err != nil {
		fatal(err)
	}
	total := 0.0
	for _, v := range samples {
		total += v * v
	}
	total /= float64(len(samples))
	for lo := 200.0; lo < 6200; lo += 750 {
		frac := sfxlib.BandFraction(samples, sampleRate, lo, lo+750, 20, 10000, 0)
		db := 10 * math.Log10(math.Max(frac*total, 1e-12))
		fmt.Printf("  %4.0f-%4.0f Hz: %4.0f dBFS\n", lo, lo+750, db)
		if db < -62 {
			fatal(fmt.Errorf("band %.0f-%.0f Hz at %.0f dBFS, under the -62 the canceller needs a first pass from", lo, lo+750, db))
		}
	}
	fmt.Println("stinger spectrum ok")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "gen-stinger:", err)
	os.Exit(1)
}
