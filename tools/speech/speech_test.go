// SPDX-License-Identifier: GPL-2.0-or-later

package speech

import (
	"math"
	"os"
	"testing"

	"github.com/stealthmonkey99/arkade/tools/sfxlib"
)

func tone(hz float64, rate, n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = math.Sin(2 * math.Pi * hz * float64(i) / float64(rate))
	}
	return s
}

func rms(s []float64) float64 {
	sum := 0.0
	for _, v := range s {
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(s)))
}

func TestResampleKeepsThePassband(t *testing.T) {
	out := Resample(tone(1000, 24000, 24000), 24000, 16000)
	if len(out) != 16000 {
		t.Fatalf("len %d, want 16000", len(out))
	}
	mid := out[1000:15000]
	if got := rms(mid); math.Abs(got-1/math.Sqrt2) > 0.01 {
		t.Errorf("1 kHz rms %.4f, want %.4f", got, 1/math.Sqrt2)
	}
	if err := sfxlib.AssertSpectrum(mid, 16000, 20, 7800, []sfxlib.SpectrumCheck{
		{Name: "1kHz", Lo: 900, Hi: 1100, Min: 0.99},
	}); err != nil {
		t.Error(err)
	}
}

// A 10 kHz tone is above the 8 kHz output Nyquist; without the low-pass
// it would fold back to 6 kHz.
func TestResampleRejectsAliases(t *testing.T) {
	out := Resample(tone(10000, 24000, 24000), 24000, 16000)
	if got := rms(out[1000:15000]); got > 0.01 {
		t.Errorf("aliased energy rms %.4f, want ~0", got)
	}
}

// Needs `npm ci` here and fetches ~330 MB of model on first run.
func TestRenderSpeaks(t *testing.T) {
	if os.Getenv("ARKADE_SPEECH_TEST") == "" {
		t.Skip("set ARKADE_SPEECH_TEST=1 to run the Kokoro renderer")
	}
	out, err := Render(Voice{Name: "bm_george", Speed: 1.3}, []string{"fifteen two"}, 16000)
	if err != nil {
		t.Fatal(err)
	}
	if d := float64(len(out[0])) / 16000; d < 0.4 || d > 2 {
		t.Errorf("\"fifteen two\" lasted %.2fs", d)
	}
	if err := sfxlib.AssertSpectrum(out[0], 16000, 20, 7800, []sfxlib.SpectrumCheck{
		{Name: "speech-300-3400Hz", Lo: 300, Hi: 3400, Min: 0.25},
	}); err != nil {
		t.Error(err)
	}
}
