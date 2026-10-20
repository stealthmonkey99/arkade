// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"math"
	"testing"

	"github.com/stealthmonkey99/arkade/tools/sfxlib"
)

// A render whose trim kept one more 5 ms frame of lead-in is the same
// sound: measured where it lines up, not sample for sample.
func TestMeasureLinesUpAShiftedRender(t *testing.T) {
	ref := tone(440, 0.8, 8000)
	lead := rate * 5 / 1000
	late := append(make([]float64, lead), ref...)
	d, err := measureAudio(sfxlib.Encode(late, rate), sfxlib.Encode(ref, rate))
	if err != nil {
		t.Fatal(err)
	}
	if d.maxDelta != 0 || !math.IsInf(d.snr, 1) || d.shiftMs != 5 {
		t.Fatalf("measured %v, want identical at +5.0 ms", d)
	}
	d, err = measureAudio(sfxlib.Encode(ref, rate), sfxlib.Encode(late, rate))
	if err != nil {
		t.Fatal(err)
	}
	if d.maxDelta != 0 || !math.IsInf(d.snr, 1) || d.shiftMs != -5 {
		t.Fatalf("measured %v, want identical at -5.0 ms", d)
	}
}

func TestMeasureDoesNotChaseAShiftPastTheWindow(t *testing.T) {
	ref := tone(440, 0.8, 8000)
	late := append(make([]float64, rate*maxShiftMs/1000+rate/100), ref...)
	d, err := measureAudio(sfxlib.Encode(late, rate), sfxlib.Encode(ref, rate))
	if err != nil {
		t.Fatal(err)
	}
	if d.within(tolerance{defaultMaxDelta, defaultMinSNR}) {
		t.Fatalf("a 60 ms shift passed: %v", d)
	}
}
