// SPDX-License-Identifier: GPL-2.0-or-later

package musiclib

import (
	"math"
	"os"
	"testing"

	"github.com/stealthmonkey99/arkade/tools/sfxlib"
)

func testSong() Song {
	return Song{
		BPM:   120,
		Beats: 4,
		Tracks: []Track{
			{
				Name: "lead",
				Inst: Instrument{Wave: WavePulse, Duty: 0.25},
				Notes: []Note{
					{Midi: 76, Start: 0, Dur: 1},
					{Midi: 71, Start: 1, Dur: 0.5},
					{Midi: 72, Start: 1.5, Dur: 0.5},
					{Midi: 74, Start: 2, Dur: 1},
					{Midi: 72, Start: 3, Dur: 0.5},
					{Midi: 71, Start: 3.5, Dur: 0.5},
				},
			},
			{
				Name: "bass",
				Inst: Instrument{Wave: WaveTriangle},
				Notes: []Note{
					{Midi: 40, Start: 0, Dur: 2},
					{Midi: 45, Start: 2, Dur: 2},
				},
			},
			{
				Name: "perc",
				Inst: Instrument{Wave: WaveNoise, NoiseTone: 0.8, ReleaseMs: 30},
				Gain: 0.4,
				Notes: []Note{
					{Midi: 72, Start: 0, Dur: 0.25, Vel: 0.9},
					{Midi: 72, Start: 1, Dur: 0.25, Vel: 0.6},
					{Midi: 72, Start: 2, Dur: 0.25, Vel: 0.9},
					{Midi: 72, Start: 3, Dur: 0.25, Vel: 0.6},
				},
			},
		},
	}
}

func TestRenderLength(t *testing.T) {
	s := testSong()
	buf := RenderLoop(s, SampleRate22050)
	want := 2 * SampleRate22050
	if len(buf) != want {
		t.Fatalf("len = %d, want %d", len(buf), want)
	}
}

func TestAutoBeatsRoundsUpToWholeBeat(t *testing.T) {
	s := Song{BPM: 100, Tracks: []Track{{
		Inst:  Instrument{Wave: WavePulse},
		Notes: []Note{{Midi: 60, Start: 0, Dur: 3.5}},
	}}}
	if got := s.LengthBeats(); got != 4 {
		t.Fatalf("LengthBeats = %v, want 4", got)
	}
}

func TestDeterminism(t *testing.T) {
	a := RenderLoop(testSong(), SampleRate22050)
	b := RenderLoop(testSong(), SampleRate22050)
	if len(a) != len(b) {
		t.Fatalf("length mismatch %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d differs: %v vs %v", i, a[i], b[i])
		}
	}
}

func TestHeadroom(t *testing.T) {
	s := Song{BPM: 120, Beats: 2}
	tr := Track{Inst: Instrument{Wave: WavePulse}}
	for m := 48; m < 72; m += 3 {
		tr.Notes = append(tr.Notes, Note{Midi: m, Start: 0, Dur: 2, Vel: 1})
	}
	s.Tracks = []Track{tr, tr, tr}
	buf := RenderLoop(s, SampleRate22050)
	peak := 0.0
	for _, v := range buf {
		if a := math.Abs(v); a > peak {
			peak = a
		}
	}
	if peak > DefaultPeak+1e-9 {
		t.Fatalf("peak %v exceeds %v", peak, DefaultPeak)
	}
	if peak < DefaultPeak-1e-9 {
		t.Fatalf("peak %v: normalization should hit the target exactly", peak)
	}
}

func TestLoopSeam(t *testing.T) {
	buf := RenderLoop(testSong(), SampleRate22050)
	n := len(buf)

	maxInterior := 0.0
	for i := 1; i < n; i++ {
		if d := math.Abs(buf[i] - buf[i-1]); d > maxInterior {
			maxInterior = d
		}
	}
	seam := math.Abs(buf[0] - buf[n-1])
	if seam > maxInterior {
		t.Errorf("seam jump %v exceeds max interior jump %v", seam, maxInterior)
	}

	win := SampleRate22050 / 2000 // 0.5 ms
	rms := func(s []float64) float64 {
		sum := 0.0
		for _, v := range s {
			sum += v * v
		}
		return math.Sqrt(sum / float64(len(s)))
	}
	songRMS := rms(buf)
	if head, tail := rms(buf[:win]), rms(buf[n-win:]); head > songRMS/2 || tail > songRMS/2 {
		t.Errorf("seam neighborhood too loud: head %v tail %v songRMS %v", head, tail, songRMS)
	}
}

func TestTailWrap(t *testing.T) {
	s := Song{BPM: 60, Beats: 2, Tracks: []Track{{
		Inst:  Instrument{Wave: WaveTriangle},
		Notes: []Note{{Midi: 57, Start: 1.5, Dur: 1}},
	}}}
	buf := RenderLoop(s, SampleRate22050)
	spb := SampleRate22050 // 60 BPM
	energy := 0.0
	for _, v := range buf[:spb/4] {
		energy += v * v
	}
	if energy == 0 {
		t.Fatalf("tail did not wrap into the loop start")
	}
	maxInterior := 0.0
	for i := spb + spb/2 + 100; i < 2*spb; i++ {
		if d := math.Abs(buf[i] - buf[i-1]); d > maxInterior {
			maxInterior = d
		}
	}
	seam := math.Abs(buf[0] - buf[len(buf)-1])
	if seam > 2*maxInterior {
		t.Errorf("wrapped seam jump %v vs interior motion %v", seam, maxInterior)
	}
}

func TestNoiseSeedDerivation(t *testing.T) {
	for _, tc := range []struct {
		ti, ni int
		want   uint16
	}{
		{0, 0, 0xACE1},
		{1, 0, 0xACE1 ^ 0x9E37},
		{0, 1, 0xACE1 ^ 0x79B9},
		{2, 3, 0xACE1 ^ (2*0x9E37)&0xFFFF ^ (3*0x79B9)&0xFFFF},
		{15, 2047, 0xACE1 ^ (15*0x9E37)&0xFFFF ^ (2047*0x79B9)&0xFFFF},
	} {
		if got := (Note{}).NoiseSeed(tc.ti, tc.ni); got != tc.want {
			t.Errorf("NoiseSeed(%d, %d) = %#x, want %#x", tc.ti, tc.ni, got, tc.want)
		}
	}
	if got := (Note{Seed: 0x1234}).NoiseSeed(7, 9); got != 0x1234 {
		t.Errorf("a pinned seed must be used as written, got %#x", got)
	}
	for ti := 0; ti < 16; ti++ {
		for ni := 0; ni < 2048; ni++ {
			if (Note{}).NoiseSeed(ti, ni) == 0 {
				t.Fatalf("NoiseSeed(%d, %d) is the zero state, which locks the LFSR", ti, ni)
			}
		}
	}
}

func TestNoiseSeedSurvivesReindexing(t *testing.T) {
	hats := func() Track {
		tr := Track{Name: "hats", Inst: Instrument{Wave: WaveNoise, NoiseTone: 0.8}}
		for i := 0; i < 8; i++ {
			tr.Notes = append(tr.Notes, Note{Midi: 80, Start: 0.5 * float64(i), Dur: 0.25, Vel: 0.5})
		}
		return tr
	}
	song := func(tracks ...Track) Song { return Song{BPM: 120, Beats: 4, Tracks: tracks} }
	render := func(s Song) []float64 { return RenderLoopPeak(s, SampleRate22050, 0) }
	silent := Track{Name: "lead", Inst: Instrument{Wave: WavePulse}}
	spb := 60.0 / 120 * SampleRate22050
	noteSpan := func(n Note) (int, int) {
		return int(math.Round(n.Start * spb)), int(math.Round(n.Start*spb)) + int(math.Round(n.Dur*spb))
	}
	same := func(a, b []float64, skipLo, skipHi int) bool {
		for i := range a {
			if i >= skipLo && i < skipHi {
				continue
			}
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	derived := render(song(hats()))
	if same(derived, render(song(silent, hats())), 0, 0) {
		t.Fatal("moving a derived-seed track to another index rendered the same audio; the trap this test guards against is gone, so is the reason for Seed")
	}
	removed := hats()
	removed.Notes = append(removed.Notes[:2:2], removed.Notes[3:]...)
	lo, hi := noteSpan(hats().Notes[2])
	if same(derived, render(song(removed)), lo, hi) {
		t.Fatal("removing a derived-seed hat left the hats after it unchanged; the trap is gone")
	}

	pinnedSong := pinSeeds(song(hats()))
	pinned := render(pinnedSong)
	if !same(pinned, derived, 0, 0) {
		t.Fatal("pinning the derived seeds changed the render")
	}
	moved := song(silent, pinnedSong.Tracks[0])
	if !same(pinned, render(moved), 0, 0) {
		t.Error("a pinned track renders differently at another track index")
	}
	removedPinned := pinnedSong.Tracks[0]
	removedPinned.Notes = append(removedPinned.Notes[:2:2], removedPinned.Notes[3:]...)
	got := render(song(removedPinned))
	if !same(pinned, got, lo, hi) {
		t.Error("removing a pinned hat changed the hats after it")
	}
	for i := lo; i < hi; i++ {
		if got[i] != 0 {
			t.Fatalf("sample %d: the removed hat still sounds", i)
		}
	}
}

func TestEnvelopeCloses(t *testing.T) {
	for _, inst := range []Instrument{
		{Wave: WavePulse},
		{Wave: WaveTriangle, ReleaseMs: 40},
		{Wave: WaveNoise},
	} {
		s := Song{BPM: 120, Beats: 2, Tracks: []Track{{
			Inst:  inst,
			Notes: []Note{{Midi: 69, Start: 0, Dur: 1}},
		}}}
		buf := RenderLoopPeak(s, SampleRate22050, 0)
		end := SampleRate22050 / 2 // note occupies first half of buffer
		if v := math.Abs(buf[end-1]); v > 0.01 {
			t.Errorf("inst %v: last note sample %v, want ~0", inst.Wave, v)
		}
		for i := end; i < len(buf); i++ {
			if buf[i] != 0 {
				t.Errorf("inst %v: nonzero sample %v after note end", inst.Wave, buf[i])
				break
			}
		}
	}
}

func TestPulseDuty(t *testing.T) {
	s := Song{BPM: 60, Beats: 1, Tracks: []Track{{
		Inst:  Instrument{Wave: WavePulse, Duty: 0.25, AttackMs: 0.0001, ReleaseMs: 0.0001},
		Notes: []Note{{Midi: int(math.Round(FreqMidi(100))), Start: 0, Dur: 1}},
	}}}
	buf := RenderLoopPeak(s, SampleRate44100, 0)
	hi := 0
	mid := buf[1000 : 1000+4410] // 10 full periods, away from attack
	for _, v := range mid {
		if v > 0 {
			hi++
		}
	}
	frac := float64(hi) / float64(len(mid))
	if math.Abs(frac-0.25) > 0.02 {
		t.Errorf("duty fraction %v, want ~0.25", frac)
	}
}

func TestWriteLoopWAVDeterministic(t *testing.T) {
	dir := t.TempDir()
	p1, p2 := dir+"/a.wav", dir+"/b.wav"
	if err := WriteLoopWAV(p1, testSong(), SampleRate22050); err != nil {
		t.Fatal(err)
	}
	if err := WriteLoopWAV(p2, testSong(), SampleRate22050); err != nil {
		t.Fatal(err)
	}
	b1, b2 := mustRead(t, p1), mustRead(t, p2)
	if len(b1) != len(b2) {
		t.Fatalf("file sizes differ")
	}
	for i := range b1 {
		if b1[i] != b2[i] {
			t.Fatalf("byte %d differs", i)
		}
	}
	if want := 44 + 2*2*SampleRate22050; len(b1) != want {
		t.Fatalf("file size %d, want %d", len(b1), want)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEcho(t *testing.T) {
	base := testSong()
	buf0 := RenderLoop(base, SampleRate22050)

	off := base
	off.Echo = Echo{} // zero value: off
	bufOff := RenderLoop(off, SampleRate22050)
	for i := range buf0 {
		if buf0[i] != bufOff[i] {
			t.Fatalf("zero Echo changed output at %d", i)
		}
	}

	on := base
	on.Echo = Echo{DelayBeats: 0.5, Feedback: 0.4, Mix: 0.3}
	a := RenderLoop(on, SampleRate22050)
	b := RenderLoop(on, SampleRate22050)
	diff := false
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("echo render not deterministic at %d", i)
		}
		if a[i] != buf0[i] {
			diff = true
		}
	}
	if !diff {
		t.Fatalf("echo had no effect")
	}

	n := len(a)
	maxInterior := 0.0
	for i := 1; i < n; i++ {
		if d := math.Abs(a[i] - a[i-1]); d > maxInterior {
			maxInterior = d
		}
	}
	if seam := math.Abs(a[0] - a[n-1]); seam > maxInterior {
		t.Errorf("echo seam jump %v exceeds max interior jump %v", seam, maxInterior)
	}
}

func TestEchoTapPlacement(t *testing.T) {
	s := Song{BPM: 60, Beats: 2, Tracks: []Track{{
		Inst: Instrument{Wave: WaveTriangle},
		// Note in the SECOND half; its echo (delay 1.5 beats) lands at
		// beat 2.5+ i.e. wraps to beat 0.5+ of the loop.
		Notes: []Note{{Midi: 69, Start: 1.0, Dur: 0.25}},
	}}}
	s.Echo = Echo{DelayBeats: 1.5, Feedback: 0, Mix: 0.5}
	buf := RenderLoopPeak(s, SampleRate22050, 0)
	sr := SampleRate22050
	energy := func(fromBeat, toBeat float64) float64 {
		e := 0.0
		for i := int(fromBeat * float64(sr)); i < int(toBeat*float64(sr)); i++ {
			e += buf[i] * buf[i]
		}
		return e
	}
	dryE := energy(1.0, 1.25)
	wetE := energy(0.5, 0.75) // wrapped echo tap
	if wetE == 0 {
		t.Fatalf("echo tap did not wrap through the loop seam")
	}
	ratio := wetE / dryE
	if math.Abs(ratio-0.25) > 0.05 { // Mix 0.5 -> energy 0.25
		t.Errorf("wrapped tap energy ratio %v, want ~0.25", ratio)
	}
	if e := energy(0.0, 0.4); e > 0.001*dryE {
		t.Errorf("unexpected energy outside dry+tap regions: %v", e)
	}
}

func TestLowpass(t *testing.T) {
	mk := func(lp float64) []float64 {
		s := Song{BPM: 60, Beats: 1, Tracks: []Track{{
			Inst:  Instrument{Wave: WavePulse, Duty: 0.5, LowpassHz: lp},
			Notes: []Note{{Midi: 69, Start: 0, Dur: 1}},
		}}}
		return RenderLoopPeak(s, SampleRate44100, 0)
	}
	h3rel := func(buf []float64) float64 {
		f0 := MidiFreq(69)
		h1 := sfxlib.GoertzelPower(buf, SampleRate44100, f0)
		h3 := sfxlib.GoertzelPower(buf, SampleRate44100, 3*f0)
		return h3 / h1
	}
	raw := h3rel(mk(0))
	soft := h3rel(mk(800))
	if soft >= raw*0.6 {
		t.Errorf("lowpass did not attenuate h3: raw %v soft %v", raw, soft)
	}
}

func TestDetune(t *testing.T) {
	mk := func(cents float64) []float64 {
		s := Song{BPM: 60, Beats: 2, Tracks: []Track{{
			Inst:  Instrument{Wave: WavePulse, Duty: 0.5, DetuneCents: cents},
			Notes: []Note{{Midi: 69, Start: 0, Dur: 2}},
		}}}
		return RenderLoopPeak(s, SampleRate22050, 0)
	}
	plain := mk(0)
	det := mk(10)

	winVar := func(buf []float64) float64 {
		win := SampleRate22050 / 20
		var rms []float64
		for i := 0; i+win <= len(buf); i += win {
			e := 0.0
			for _, v := range buf[i : i+win] {
				e += v * v
			}
			rms = append(rms, math.Sqrt(e/float64(win)))
		}
		mean, v := 0.0, 0.0
		for _, r := range rms {
			mean += r
		}
		mean /= float64(len(rms))
		for _, r := range rms {
			v += (r - mean) * (r - mean)
		}
		return v / float64(len(rms))
	}
	if winVar(det) <= winVar(plain)*2 {
		t.Errorf("detune produced no beating: var %v vs %v", winVar(det), winVar(plain))
	}
	det2 := mk(10)
	for i := range det {
		if det[i] != det2[i] {
			t.Fatalf("detuned render not deterministic")
		}
	}
	end := 2 * SampleRate22050
	if v := math.Abs(det[end-1]); v > 0.02 {
		t.Errorf("detuned envelope did not close: %v", v)
	}
}
