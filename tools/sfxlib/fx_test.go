// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"math"
	"testing"

	"github.com/stealthmonkey99/arkade/assetfile"
)

func ramp(n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = float64(i) / float64(n)
	}
	return s
}

func TestApplyFxNilAndZeroLeaveTheSourceAlone(t *testing.T) {
	s := ramp(1000)
	if out := ApplyFx(s, 1000, nil); &out[0] != &s[0] {
		t.Error("nil fx copied the source")
	}
	out := ApplyFx(s, 1000, &assetfile.SoundFx{})
	if len(out) != len(s) {
		t.Fatalf("zero fx changed the length to %d", len(out))
	}
	for i := range s {
		if out[i] != s[i] {
			t.Fatalf("zero fx moved sample %d", i)
		}
	}
	out[0] = 99
	if s[0] == 99 {
		t.Error("ApplyFx wrote into its input")
	}
}

func TestApplyFxSlicesThenReverses(t *testing.T) {
	s := ramp(1000) // 1 s at 1 kHz
	out := ApplyFx(s, 1000, &assetfile.SoundFx{StartMs: 200, EndMs: 500, Reverse: true})
	if len(out) != 300 {
		t.Fatalf("slice length %d, want 300", len(out))
	}
	if out[0] != s[499] || out[299] != s[200] {
		t.Errorf("reversed slice runs %v..%v, want %v..%v", out[0], out[299], s[499], s[200])
	}
	if got := len(ApplyFx(s, 1000, &assetfile.SoundFx{StartMs: 900})); got != 100 {
		t.Errorf("an open-ended slice kept %d samples, want 100", got)
	}
}

func TestApplyFxStretchKeepsPitchUnlessVarispeed(t *testing.T) {
	const rate = 24000
	s := make([]float64, rate)
	for i := range s {
		s[i] = 0.5 * math.Sin(2*math.Pi*200*float64(i)/rate)
	}
	held := ApplyFx(s, rate, &assetfile.SoundFx{Stretch: 2})
	if len(held) != 2*rate {
		t.Errorf("stretch 2 gave %d samples, want %d", len(held), 2*rate)
	}
	if hz := crossings(held, rate); math.Abs(hz-200) > 4 {
		t.Errorf("a stretched tone is %.1f Hz, want 200", hz)
	}
	tape := ApplyFx(s, rate, &assetfile.SoundFx{Stretch: 2, Varispeed: true})
	if len(tape) != 2*rate {
		t.Errorf("varispeed 2 gave %d samples, want %d", len(tape), 2*rate)
	}
	if hz := crossings(tape, rate); math.Abs(hz-100) > 2 {
		t.Errorf("a tape-slowed tone is %.1f Hz, want 100", hz)
	}
}

func TestApplyFxFadesReachSilence(t *testing.T) {
	s := make([]float64, 1000)
	for i := range s {
		s[i] = 1
	}
	out := ApplyFx(s, 1000, &assetfile.SoundFx{FadeInMs: 100, FadeOutMs: 100})
	if out[0] != 0 || out[len(out)-1] != 0 || out[500] != 1 {
		t.Errorf("fades: first %v, last %v, middle %v", out[0], out[len(out)-1], out[500])
	}
}
