// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stealthmonkey99/arkade/assetfile"
)

// The sample set is synthesised so classes are known by construction: three
// "words" stand in for spoken commands, with length, pitch and level drift
// between a class's clips.

type synth struct {
	out   []float64
	phase float64
	rng   uint64
}

func (s *synth) noise() float64 {
	s.rng ^= s.rng << 13
	s.rng ^= s.rng >> 7
	s.rng ^= s.rng << 17
	return float64(s.rng>>11)/float64(uint64(1)<<53)*2 - 1
}

func (s *synth) tone(hz, secs, amp, wobbleHz float64) {
	n := int(secs * Rate)
	for i := 0; i < n; i++ {
		a := amp
		if wobbleHz > 0 {
			a *= 0.55 + 0.45*math.Sin(2*math.Pi*wobbleHz*float64(i)/Rate)
		}
		s.out = append(s.out, a*math.Sin(s.phase))
		s.phase += 2 * math.Pi * hz / Rate
	}
}

func (s *synth) lowNoise(secs, amp float64) {
	n := int(secs * Rate)
	y, sum := 0.0, 0.0
	buf := make([]float64, n)
	for i := range buf {
		y += (s.noise() - y) * 0.2
		buf[i] = y
		sum += y * y
	}
	if rms := math.Sqrt(sum / float64(n)); rms > 0 {
		for i := range buf {
			buf[i] *= amp * 0.5 / rms
		}
	}
	s.out = append(s.out, buf...)
}

// clipOf builds one utterance of word (0..2) with the j-th variation's
// duration, pitch and level, padded with the silence the endpointer needs to
// see an onset and a close.
func clipOf(word int, j float64) []float64 {
	d := 1 + (j-2.5)*0.032
	p := 1 + (j-2.5)*0.012
	amp := 0.2 + 0.04*j
	s := &synth{rng: uint64(word)*1013 + uint64(j*7) + 1}
	switch word {
	case 0:
		s.tone(600*p, 0.18*d, amp, 0)
		s.tone(900*p, 0.18*d, amp, 0)
	case 1:
		s.tone(1500*p, 0.40*d, amp, 4)
	default:
		s.lowNoise(0.15*d, amp)
		s.tone(2200*p, 0.20*d, amp, 0)
	}
	fade(s.out, int(0.02*Rate))
	return pad(s.out, int(0.3*Rate))
}

func noiseClip(seed uint64, secs, amp float64) []float64 {
	s := &synth{rng: seed}
	n := int(secs * Rate)
	for i := 0; i < n; i++ {
		s.out = append(s.out, amp*s.noise())
	}
	if amp > 0.05 {
		fade(s.out, int(0.02*Rate))
		return pad(s.out, int(0.3*Rate))
	}
	return s.out
}

func fade(x []float64, n int) {
	n = min(n, len(x)/2)
	for i := 0; i < n; i++ {
		w := 0.5 - 0.5*math.Cos(math.Pi*float64(i)/float64(n))
		x[i] *= w
		x[len(x)-1-i] *= w
	}
}

func pad(x []float64, n int) []float64 {
	out := make([]float64, 0, len(x)+2*n)
	out = append(out, make([]float64, n)...)
	out = append(out, x...)
	return append(out, make([]float64, n)...)
}

func writeSamples(t *testing.T, dir, name string, samples []float64) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteWAVFile(filepath.Join(dir, name), samples, Rate); err != nil {
		t.Fatal(err)
	}
}

var trainWords = []string{"alpha", "bravo", "charlie"}

func trainSamples(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for w, name := range trainWords {
		for j := 0; j < 6; j++ {
			writeSamples(t, filepath.Join(dir, name), fmt.Sprintf("%02d.wav", j), clipOf(w, float64(j)))
		}
	}
	for j := 0; j < 4; j++ {
		writeSamples(t, filepath.Join(dir, "_noise"), fmt.Sprintf("quiet%02d.wav", j), noiseClip(uint64(j)+91, 1.2, 0.01))
	}
	writeSamples(t, filepath.Join(dir, "_noise"), "burst.wav", noiseClip(777, 0.35, 0.3))
	return dir
}

func trainForTest(t *testing.T) (*assetfile.Envelope, *Report) {
	t.Helper()
	env, rep, err := Train(TrainOptions{
		Dir:       trainSamples(t),
		Name:      "words",
		Classes:   trainWords,
		Generator: assetfile.Generator{Tool: "micmodel", Version: "1.0.0"},
	})
	if err != nil {
		t.Fatalf("Train: %v", err)
	}
	return env, rep
}

func TestTrainSeparatesThreeWords(t *testing.T) {
	env, rep := trainForTest(t)
	if err := assetfile.Validate(env, assetfile.KindMic, 0, 0); err != nil {
		t.Fatalf("trained envelope does not validate: %v", err)
	}
	if len(env.Classes) != len(trainWords) {
		t.Fatalf("got %d classes, want %d", len(env.Classes), len(trainWords))
	}
	for k, c := range env.Classes {
		if c.Cue != k || c.Name != trainWords[k] {
			t.Errorf("class %d is %q cue %d, want %q cue %d", k, c.Name, c.Cue, trainWords[k], k)
		}
		if len(c.Templates) == 0 {
			t.Errorf("class %q kept no template", c.Name)
		}
	}
	for _, c := range rep.Classes {
		if c.Accepted < 5 {
			t.Errorf("class %q accepted %d of %d clips (threshold %.3f, pos max %.3f, neg min %.3f)",
				c.Name, c.Accepted, c.Clips, c.Threshold, c.PosMax, c.NegMin)
		}
		if c.FalseAlarm != 0 {
			t.Errorf("class %q fired on %d negatives (threshold %.3f, neg min %.3f)", c.Name, c.FalseAlarm, c.Threshold, c.NegMin)
		}
	}

	n := len(trainWords)
	for k := range trainWords {
		row := rep.Confusion[k]
		for j, count := range row {
			if j != k && j != n && count != 0 {
				t.Errorf("%s was heard as %s %d times", rep.Labels[k], rep.Labels[j], count)
			}
		}
		if row[k] < 5 {
			t.Errorf("%s: confusion diagonal is %d, want the class to dominate its own row (%v)", rep.Labels[k], row[k], row)
		}
	}
	for j, count := range rep.Confusion[n] {
		if j != n && count != 0 {
			t.Errorf("noise fired %s %d times", rep.Labels[j], count)
		}
	}
	if rep.Confusion[n][n] == 0 {
		t.Error("no negative was scored at all; the noise clips never reached the matcher")
	}
}

func TestEvaluateAgreesWithTrain(t *testing.T) {
	dir := trainSamples(t)
	env, rep, err := Train(TrainOptions{Dir: dir, Name: "words", Classes: trainWords,
		Generator: assetfile.Generator{Tool: "micmodel", Version: "1.0.0"}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Evaluate(env, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Classes) != len(rep.Classes) {
		t.Fatalf("Evaluate reports %d classes, Train %d", len(got.Classes), len(rep.Classes))
	}
	for i, c := range got.Classes {
		want := rep.Classes[i]
		if diff := c.Accepted - want.Accepted; diff < -1 || diff > 1 {
			t.Errorf("class %q: Evaluate accepted %d, Train %d", c.Name, c.Accepted, want.Accepted)
		}
		if c.FalseAlarm != 0 {
			t.Errorf("class %q: Evaluate found %d false alarms", c.Name, c.FalseAlarm)
		}
	}
}

func TestClassifierStreamsFreshClip(t *testing.T) {
	env, _ := trainForTest(t)

	fire := func(samples []float64) []Hit {
		cls, _, err := NewClassifier([]*assetfile.Envelope{env})
		if err != nil {
			t.Fatal(err)
		}
		var hits []Hit
		for off := 0; off < len(samples); off += 256 {
			end := min(off+256, len(samples))
			if h := cls.Write(samples[off:end], false); h.Cue >= 0 {
				hits = append(hits, h)
			}
		}
		return hits
	}

	hits := fire(clipOf(0, 2.2))
	if len(hits) != 1 || hits[0].Cue != 0 {
		t.Fatalf("a fresh %q fired %v, want one hit on cue 0", trainWords[0], hits)
	}
	if h := fire(noiseClip(4242, 1.5, 0.01)); len(h) != 0 {
		t.Errorf("quiet noise fired %v", h)
	}
	if h := fire(noiseClip(9001, 0.35, 0.3)); len(h) != 0 {
		t.Errorf("a broadband burst fired %v", h)
	}
}

func TestSegmentsFindsTheWord(t *testing.T) {
	env, _ := trainForTest(t)
	segs := Segments(env, clipOf(1, 3))
	if len(segs) != 1 {
		t.Fatalf("got %d segments, want the one word", len(segs))
	}
	if len(segs[0]) < 15 {
		t.Errorf("segment is %d frames, want at least the 150 ms minimum", len(segs[0]))
	}
	if len(segs[0][0]) != env.Features.Coeffs {
		t.Errorf("frame has %d coefficients, want %d", len(segs[0][0]), env.Features.Coeffs)
	}
}

func TestReadClipFileRefusesOtherRates(t *testing.T) {
	dir := t.TempDir()
	writeSamples(t, dir, "ok.wav", clipOf(0, 1))
	path := filepath.Join(dir, "odd.wav")
	if err := WriteWAVFile(path, make([]float64, 8000), 22050); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadClipFile(path); err == nil {
		t.Error("a 22050 Hz clip was accepted")
	}
	if _, err := ReadClipFile(filepath.Join(dir, "ok.wav")); err != nil {
		t.Error(err)
	}
}
