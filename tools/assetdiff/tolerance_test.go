// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stealthmonkey99/arkade/tools/sfxlib"
)

const rate = 16000

func tone(hz, amp float64, n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = amp * math.Sin(2*math.Pi*hz*float64(i)/rate)
	}
	return s
}

func plus(a, b []float64) []float64 {
	out := append([]float64(nil), a...)
	for i := range out {
		out[i] += b[i]
	}
	return out
}

func writeWAV(t *testing.T, name string, s []float64) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, sfxlib.Encode(s, rate), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMeasureIdenticalIsInfinite(t *testing.T) {
	w := sfxlib.Encode(tone(440, 0.5, 1600), rate)
	d, err := measureAudio(w, w)
	if err != nil {
		t.Fatal(err)
	}
	if d.maxDelta != 0 || !math.IsInf(d.snr, 1) {
		t.Fatalf("identical audio measured %v", d)
	}
}

func TestMeasureLowBitNoisePassesTheDefault(t *testing.T) {
	ref := tone(440, 0.8, 8000)
	near := plus(ref, tone(3100, 0.01, 8000))
	d, err := measureAudio(sfxlib.Encode(ref, rate), sfxlib.Encode(near, rate))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(d.snr-38.1) > 0.2 || math.Abs(d.maxDelta-0.01) > 0.001 {
		t.Fatalf("measured %v, want about max delta 0.01 and SNR 38.1 dB", d)
	}
	if !d.within(tolerance{defaultMaxDelta, defaultMinSNR}) {
		t.Fatalf("%v should pass the default tolerance", d)
	}
}

func TestMeasureAnotherSoundFailsTheDefault(t *testing.T) {
	d, err := measureAudio(sfxlib.Encode(tone(440, 0.8, 8000), rate), sfxlib.Encode(tone(523, 0.8, 8000), rate))
	if err != nil {
		t.Fatal(err)
	}
	if d.within(tolerance{defaultMaxDelta, defaultMinSNR}) {
		t.Fatalf("a different pitch passed: %v", d)
	}
}

func TestMeasureCountsALengthChangeAsError(t *testing.T) {
	ref := tone(440, 0.8, 8000)
	d, err := measureAudio(sfxlib.Encode(ref, rate), sfxlib.Encode(ref[:4000], rate))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(d.snr-3.01) > 0.1 {
		t.Fatalf("losing half the sound measured SNR %.2f dB, want about 3", d.snr)
	}
}

func TestMeasureRefusesAnotherFormat(t *testing.T) {
	s := tone(440, 0.5, 1600)
	if _, err := measureAudio(sfxlib.Encode(s, rate), sfxlib.Encode(s, rate/2)); err == nil {
		t.Fatal("a sample-rate mismatch was measured")
	}
}

func TestReportIsExactUnlessTolerant(t *testing.T) {
	ref := tone(440, 0.8, 8000)
	a := writeWAV(t, "line.wav", ref)
	b := writeWAV(t, "line.wav", plus(ref, tone(3100, 0.01, 8000)))
	if report("line.wav", a, b, true, nil) {
		t.Fatal("exact mode passed audio that differs")
	}
	if !report("line.wav", a, b, true, &tolerance{defaultMaxDelta, defaultMinSNR}) {
		t.Fatal("tolerance mode failed audio within the default")
	}
	if report("line.wav", a, b, true, &tolerance{0.001, defaultMinSNR}) {
		t.Fatal("a max delta tighter than the difference passed")
	}
	if report("line.wav", a, b, true, &tolerance{defaultMaxDelta, 60}) {
		t.Fatal("an SNR floor above the difference passed")
	}
}
