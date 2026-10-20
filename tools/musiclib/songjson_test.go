// SPDX-License-Identifier: GPL-2.0-or-later

package musiclib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

func n(midi int, start, dur, vel float64) Note {
	return Note{Midi: midi, Start: start, Dur: dur, Vel: vel}
}

func funkSong() Song {
	var bass, clav, hats, thumps []Note
	for bar := 0; bar < 4; bar++ {
		at := float64(bar * 4)
		for _, s := range []struct{ at, dur, vel float64 }{{0, 0.5, 0.62}, {0.75, 0.25, 0.5}, {1, 0.25, 0.58}, {2.75, 0.25, 0.5}} {
			bass = append(bass, n(45, at+s.at, s.dur, s.vel))
		}
		clav = append(clav, n(64, at+0.75, 0.2, 0.32), n(69, at+1.75, 0.2, 0.28))
		thumps = append(thumps, n(45, at, 0.8, 0.55), n(45, at+2.5, 0.6, 0.45))
	}
	for beat := 0.0; beat < 16; beat += 0.5 {
		vel := 0.26
		if beat == math.Trunc(beat) {
			vel = 0.4
		}
		hats = append(hats, n(80, beat, 0.4, vel), n(82, beat+0.67, 0.3, 0.2))
	}
	return Song{
		BPM:   112,
		Beats: 16,
		Tracks: []Track{
			{Name: "lead", Gain: 1.0, Inst: Instrument{Wave: WavePulse, Duty: 0.5, AttackMs: 4, ReleaseMs: 25, LowpassHz: 2800},
				Notes: []Note{n(76, 0, 1, 0.95), n(71, 1, 0.67, 0.8), n(72, 1.67, 0.33, 0.8), n(74, 2, 1, 0.9), n(72, 3, 0.5, 0.8), n(71, 3.5, 0.5, 0.8), n(69, 4, 1.75, 0.9), n(80, 12, 3, 0.9)}},
			{Name: "clav", Gain: 0.65, Inst: Instrument{Wave: WavePulse, Duty: 0.125, AttackMs: 2, DecayMs: 60, SustainLevel: 0.25, ReleaseMs: 20, LowpassHz: 3200}, Notes: clav},
			{Name: "bass", Gain: 0.85, Inst: Instrument{Wave: WaveTriangle, AttackMs: 5, ReleaseMs: 22, LowpassHz: 950, Gain: 1.6}, Notes: bass},
			{Name: "hats", Gain: 0.5, Inst: Instrument{Wave: WaveNoise, AttackMs: 1, DecayMs: 22, SustainLevel: 0.001, ReleaseMs: 10, NoiseTone: 0.85, Gain: 0.4}, Notes: hats},
			{Name: "thump", Gain: 0.5, Inst: Instrument{Wave: WaveNoise, AttackMs: 1, DecayMs: 55, SustainLevel: 0.001, ReleaseMs: 25, NoiseTone: -0.35, Gain: 0.6}, Notes: thumps},
		},
	}
}

func padSong() Song {
	const arpDrag, ostDrag = 0.02, 0.035
	var pads, arps, ost, brush []Note
	for bar, triad := range [][3]int{{69, 73, 76}, {66, 69, 73}, {62, 66, 69}, {64, 68, 71}} {
		b0 := float64(bar * 4)
		for _, p := range triad {
			pads = append(pads, n(p, b0, 3.9, 0.3))
		}
		for k := 0; k < 8; k++ {
			arps = append(arps, n(triad[k%3]+12, b0+0.5*float64(k)+arpDrag, 0.45, 0.34))
			ost = append(ost, n(triad[0]-12, b0+0.5*float64(k)+ostDrag, 0.4, 0.2))
		}
		brush = append(brush, n(70, b0, 0.22, 0.62), n(58, b0+1, 0.35, 0.45))
	}
	return Song{
		BPM: 110,
		Tracks: []Track{
			{Name: "lead", Gain: 0.95, Inst: Instrument{Wave: WavePulse, Duty: 0.25, AttackMs: 4, ReleaseMs: 25, LowpassHz: 3500, VibratoHz: 5, VibratoSemis: 0.18},
				Notes: []Note{n(81, 0, 1.5, 0.9), n(83, 2, 1, 0.85), n(85, 3, 1, 0.9), n(83, 4, 2, 0.9), n(78, 6, 1.5, 0.8), n(81, 8, 1, 0.85), n(76, 10, 2, 0.85)}},
			{Name: "pads", Gain: 0.5, Inst: Instrument{Wave: WavePulse, Duty: 0.5, AttackMs: 30, ReleaseMs: 80, LowpassHz: 1400, DetuneCents: 10}, Notes: pads},
			{Name: "arps", Gain: 0.55, Inst: Instrument{Wave: WavePulse, Duty: 0.125, AttackMs: 2, DecayMs: 35, SustainLevel: 0.25, ReleaseMs: 20, LowpassHz: 3200}, Notes: arps},
			{Name: "ostinato", Gain: 0.6, Inst: Instrument{Wave: WavePulse, Duty: 0.5, AttackMs: 4, DecayMs: 70, SustainLevel: 0.10, ReleaseMs: 30, LowpassHz: 1400, Gain: 0.5}, Notes: ost},
			{Name: "keys", Gain: 0.7, Inst: Instrument{Wave: WaveTriangle, AttackMs: 4, DecayMs: 260, SustainLevel: 0.25, ReleaseMs: 60, LowpassHz: 1700, DetuneCents: 5, Gain: 1.5},
				Notes: []Note{n(62, 0, 0.9, 0.32), n(66, 1, 0.9, 0.32), n(69, 2, 0.9, 0.32), n(73, 3, 0.9, 0.32)}},
			{Name: "brush", Gain: 0.42, Inst: Instrument{Wave: WaveNoise, AttackMs: 2, DecayMs: 60, SustainLevel: 0.001, ReleaseMs: 40, NoiseTone: -0.1, Gain: 0.35}, Notes: brush},
		},
		Echo: Echo{DelayBeats: 0.75, Feedback: 0.3, Mix: 0.06},
	}
}

func droneSong() Song {
	return Song{
		BPM:   48,
		Beats: 32,
		Tracks: []Track{
			{Name: "pedal", Gain: 0.9, Inst: Instrument{Wave: WaveTriangle, AttackMs: 300, DecayMs: 1500, SustainLevel: 0.6, ReleaseMs: 600, LowpassHz: 700, Gain: 1.6},
				Notes: []Note{n(48, 0, 8, 0.55), n(48, 8, 8, 0.48), n(48, 16, 8, 0.55), n(48, 24, 8, 0.48)}},
			{Name: "fifth", Gain: 0.55, Inst: Instrument{Wave: WaveTriangle, AttackMs: 600, DecayMs: 2000, SustainLevel: 0.55, ReleaseMs: 900, LowpassHz: 800, Gain: 1.3},
				Notes: []Note{n(55, 0, 16, 0.28), n(55, 16, 16, 0.25)}},
			{Name: "swell", Gain: 0.8, Inst: Instrument{Wave: WavePulse, Duty: 0.5, AttackMs: 1200, DecayMs: 0, SustainLevel: 1, ReleaseMs: 900, LowpassHz: 1200, DetuneCents: 14, Gain: 0.55},
				Notes: []Note{n(63, 0, 7.5, 0.5), n(65, 8, 7.5, 0.45), n(61, 16, 7.5, 0.5), n(60, 24, 7.5, 0.4)}},
			{Name: "box", Gain: 0.45, Inst: Instrument{Wave: WaveTriangle, AttackMs: 3, DecayMs: 800, SustainLevel: 0.02, ReleaseMs: 250, DetuneCents: 4, LowpassHz: 5000, Gain: 1.0},
				Notes: []Note{n(75, 0, 2, 0.22), n(72, 16, 2, 0.18), n(76, 30, 1.5, 0.6), n(72, 31.5, 0.5, 0.5)}},
			{Name: "bare", Inst: Instrument{}, Notes: []Note{{Midi: 60, Start: 4, Dur: 1}, {Midi: 0, Start: 20, Dur: 0.5}}},
			{Name: "tick", Gain: 0.22, Inst: Instrument{Wave: WaveNoise, AttackMs: 1, DecayMs: 25, SustainLevel: 0.001, ReleaseMs: 15, NoiseTone: 0.85, Gain: 0.4},
				Notes: []Note{n(68, 0, 0.2, 0.36), n(74, 0.5, 0.2, 0.22), n(74, 1.5, 0.2, 0.22), n(66, 12, 0.5, 0.25)}},
		},
		Echo: Echo{DelayBeats: 1.0, Feedback: 0.4, Mix: 0.15},
	}
}

func testSongs() map[string]Song {
	return map[string]Song{"funk": funkSong(), "pad": padSong(), "drone": droneSong(), "test": testSong(), "pinned": pinSeeds(funkSong())}
}

func pinSeeds(s Song) Song {
	tracks := make([]Track, len(s.Tracks))
	for ti, tr := range s.Tracks {
		tracks[ti] = tr
		tracks[ti].Notes = append([]Note(nil), tr.Notes...)
		if tr.Inst.Wave != WaveNoise {
			continue
		}
		for ni := range tracks[ti].Notes {
			tracks[ti].Notes[ni].Seed = int(tr.Notes[ni].NoiseSeed(ti, ni))
		}
	}
	s.Tracks = tracks
	return s
}

func roundTrip(t *testing.T, s Song) (Song, json.RawMessage) {
	t.Helper()
	raw, err := s.MarshalParams("bed")
	if err != nil {
		t.Fatalf("MarshalParams: %v", err)
	}
	back, err := UnmarshalSongParams(raw)
	if err != nil {
		t.Fatalf("UnmarshalSongParams: %v", err)
	}
	if back == nil {
		t.Fatal("UnmarshalSongParams returned nil for a song document")
	}
	return *back, raw
}

func TestSongParamsRoundTripRendersIdentically(t *testing.T) {
	for name, s := range testSongs() {
		t.Run(name, func(t *testing.T) {
			back, _ := roundTrip(t, s)
			want := RenderLoop(s, SampleRate22050)
			got := RenderLoop(back, SampleRate22050)
			if len(got) != len(want) {
				t.Fatalf("%d samples after the trip, %d before", len(got), len(want))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("sample %d: %v after the trip, %v before", i, got[i], want[i])
				}
			}
		})
	}
}

func TestSongParamsRoundTripsEveryField(t *testing.T) {
	for name, s := range testSongs() {
		t.Run(name, func(t *testing.T) {
			back, raw := roundTrip(t, s)
			want := s
			want.Peak = s.PeakLevel()
			if len(back.Tracks) != len(want.Tracks) {
				t.Fatalf("%d tracks after the trip, %d before", len(back.Tracks), len(want.Tracks))
			}
			for i := range want.Tracks {
				a, b := back.Tracks[i], want.Tracks[i]
				if a.Name != b.Name || a.Gain != b.Gain || a.Inst != b.Inst || len(a.Notes) != len(b.Notes) {
					t.Fatalf("track %d differs after the trip:\n got %+v\nwant %+v", i, a, b)
				}
				for j := range b.Notes {
					if a.Notes[j] != b.Notes[j] {
						t.Fatalf("track %d note %d: %+v after the trip, %+v before", i, j, a.Notes[j], b.Notes[j])
					}
				}
			}
			if back.BPM != want.BPM || back.Beats != want.Beats || back.Peak != want.Peak || back.Echo != want.Echo {
				t.Fatalf("song header differs after the trip:\n got %+v\nwant %+v", back, want)
			}
			again, err := back.MarshalParams("bed")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(again, raw) {
				t.Fatalf("re-marshaling the read-back song wrote a different document:\n%s\n%s", again, raw)
			}
		})
	}
}

func TestSongParamsWireShape(t *testing.T) {
	s := Song{
		BPM:   120,
		Beats: 2,
		Tracks: []Track{
			{Name: "lead", Gain: 0.8, Inst: Instrument{Wave: WavePulse, Duty: 0.25, AttackMs: 4, DecayMs: 60, SustainLevel: 0.25, ReleaseMs: 20, LowpassHz: 3200, DetuneCents: 3, VibratoHz: 5, VibratoSemis: 0.18, Gain: 0.9},
				Notes: []Note{{Midi: 76, Dur: 1, Vel: 0.9}, {Midi: 72, Start: 1.67, Dur: 0.33}}},
			{Name: "hat", Inst: Instrument{Wave: WaveNoise, NoiseTone: -0.35}, Notes: []Note{{Midi: 80, Start: 0.5, Dur: 0.25, Vel: 0.4}}},
			{Inst: Instrument{Wave: WaveTriangle}},
		},
		Echo: Echo{DelayBeats: 0.5, Feedback: 0.3, Mix: 0.12},
	}
	raw, err := s.MarshalParams("bed")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		`{"kind":"song","name":"bed","bpm":120,"beats":2,"peak":0.9,"tracks":[`,
		`{"name":"lead","instrument":{"wave":"pulse","duty":0.25,"attackMs":4,"decayMs":60,"sustainLevel":0.25,"releaseMs":20,"gain":0.9,"vibratoHz":5,"vibratoSemis":0.18,"lowpassHz":3200,"detuneCents":3},"gain":0.8,`,
		`"notes":[{"midi":76,"dur":1,"vel":0.9},{"midi":72,"start":1.67,"dur":0.33}]},`,
		`{"name":"hat","instrument":{"wave":"noise","noiseTone":-0.35},"notes":[{"midi":80,"start":0.5,"dur":0.25,"vel":0.4}]},`,
		`{"instrument":{"wave":"triangle"},"notes":[]}`,
		`],"echo":{"delayBeats":0.5,"feedback":0.3,"mix":0.12}}`,
	}, "")
	if string(raw) != want {
		t.Fatalf("wire shape moved:\n got %s\nwant %s", raw, want)
	}
}

func TestSongParamsOmitsEchoWhenOff(t *testing.T) {
	raw, err := testSong().MarshalParams("")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"echo"`, `"kind":"song","name"`, `"vel":0,`, `"vel":0}`, `"duty":0,`, `"duty":0}`, `"start":0,`, `"seed"`} {
		if bytes.Contains(raw, []byte(key)) {
			t.Errorf("document carries %s for a song that has none:\n%s", key, raw)
		}
	}
}

func TestSongParamsPeakIsPinnedInTheDocument(t *testing.T) {
	back, raw := roundTrip(t, testSong())
	if !bytes.Contains(raw, []byte(`"peak":0.9`)) {
		t.Fatalf("an unset peak must be written as DefaultPeak, got:\n%s", raw)
	}
	if back.Peak != DefaultPeak {
		t.Fatalf("Peak read back as %v, want %v", back.Peak, DefaultPeak)
	}
	loud := testSong()
	loud.Peak = 0.5
	back, _ = roundTrip(t, loud)
	if back.Peak != 0.5 {
		t.Fatalf("Peak read back as %v, want 0.5", back.Peak)
	}
	if got := RenderLoop(back, SampleRate22050); peakOf(got) > 0.5+1e-12 {
		t.Fatalf("a song with Peak 0.5 rendered to %v", peakOf(got))
	}
}

func peakOf(s []float64) float64 {
	m := 0.0
	for _, v := range s {
		m = math.Max(m, math.Abs(v))
	}
	return m
}

func TestSongParamsReadsAbsentWaveAsPulse(t *testing.T) {
	back, err := UnmarshalSongParams(json.RawMessage(`{"kind":"song","bpm":100,"tracks":[{"instrument":{},"notes":[{"midi":60,"dur":1}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if back.Tracks[0].Inst.Wave != WavePulse {
		t.Fatalf("wave read as %v, want pulse", back.Tracks[0].Inst.Wave)
	}
}

func TestSongParamsRefusesUnknownWave(t *testing.T) {
	_, err := UnmarshalSongParams(json.RawMessage(`{"kind":"song","bpm":100,"tracks":[{"instrument":{"wave":"saw"},"notes":[]}]}`))
	if err == nil || !strings.Contains(err.Error(), `"saw"`) {
		t.Fatalf("an unknown wave must be refused by name, got %v", err)
	}
}

func TestSongParamsLeavesOtherKindsAlone(t *testing.T) {
	for _, raw := range []string{"", `{"kind":"patch","durationS":1,"layers":[]}`, `{"hz":880}`} {
		back, err := UnmarshalSongParams(json.RawMessage(raw))
		if err != nil || back != nil {
			t.Fatalf("%q: got (%v, %v), want (nil, nil)", raw, back, err)
		}
	}
	if _, err := UnmarshalSongParams(json.RawMessage(`{"kind":"song",`)); err == nil {
		t.Fatal("unreadable JSON must be an error")
	}
}

func TestNoiseSeedPinsTheDocumentNotTheAudio(t *testing.T) {
	for name, s := range map[string]Song{"funk": funkSong(), "pad": padSong(), "drone": droneSong(), "test": testSong()} {
		t.Run(name, func(t *testing.T) {
			pinned := pinSeeds(s)
			before, err := s.MarshalParams("bed")
			if err != nil {
				t.Fatal(err)
			}
			after, err := pinned.MarshalParams("bed")
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(before, []byte(`"seed"`)) {
				t.Fatalf("an unpinned song must write no seed:\n%s", before)
			}
			if n := bytes.Count(after, []byte(`"seed":`)); n != noiseNotes(s) {
				t.Fatalf("the pinned document carries %d seeds for %d noise notes:\n%s", n, noiseNotes(s), after)
			}
			want := RenderLoop(s, SampleRate22050)
			got := RenderLoop(pinned, SampleRate22050)
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("sample %d: %v pinned, %v derived", i, got[i], want[i])
				}
			}
			back, _ := roundTrip(t, pinned)
			got = RenderLoop(back, SampleRate22050)
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("sample %d: %v after the pinned document's round trip, %v before", i, got[i], want[i])
				}
			}
		})
	}
}

func noiseNotes(s Song) int {
	n := 0
	for _, tr := range s.Tracks {
		if tr.Inst.Wave == WaveNoise {
			n += len(tr.Notes)
		}
	}
	return n
}

func TestSongParamsSeedWireShape(t *testing.T) {
	s := Song{BPM: 120, Beats: 1, Tracks: []Track{{Inst: Instrument{Wave: WaveNoise}, Notes: []Note{{Midi: 80, Dur: 0.25, Vel: 0.4, Seed: 4321}, {Midi: 80, Start: 0.5, Dur: 0.25}}}}}
	raw, err := s.MarshalParams("")
	if err != nil {
		t.Fatal(err)
	}
	want := `"notes":[{"midi":80,"dur":0.25,"vel":0.4,"seed":4321},{"midi":80,"start":0.5,"dur":0.25}]`
	if !bytes.Contains(raw, []byte(want)) {
		t.Fatalf("seed wire shape moved:\n got %s\nwant …%s…", raw, want)
	}
	back, _ := roundTrip(t, s)
	if got := back.Tracks[0].Notes; got[0].Seed != 4321 || got[1].Seed != 0 {
		t.Fatalf("seeds read back as %d, %d", got[0].Seed, got[1].Seed)
	}
}

func TestSongParamsRefusesSeedOutOfRange(t *testing.T) {
	for _, seed := range []int{-1, 0x10000} {
		s := testSong()
		s.Tracks[2].Notes[1].Seed = seed
		if _, err := s.MarshalParams(""); err == nil || !strings.Contains(err.Error(), "seed") {
			t.Errorf("seed %d must be refused on write, got %v", seed, err)
		}
		raw := fmt.Sprintf(`{"kind":"song","bpm":100,"tracks":[{"instrument":{"wave":"noise"},"notes":[{"midi":60,"dur":1,"seed":%d}]}]}`, seed)
		if _, err := UnmarshalSongParams(json.RawMessage(raw)); err == nil || !strings.Contains(err.Error(), "seed") {
			t.Errorf("seed %d must be refused on read, got %v", seed, err)
		}
	}
	for _, seed := range []int{1, 0xACE1, 0xFFFF} {
		raw := fmt.Sprintf(`{"kind":"song","bpm":100,"tracks":[{"instrument":{"wave":"noise"},"notes":[{"midi":60,"dur":1,"seed":%d}]}]}`, seed)
		back, err := UnmarshalSongParams(json.RawMessage(raw))
		if err != nil || back.Tracks[0].Notes[0].Seed != seed {
			t.Errorf("seed %d: got (%+v, %v)", seed, back, err)
		}
	}
}

func TestSongParamsRefusesNonFinite(t *testing.T) {
	s := testSong()
	s.Tracks[0].Notes[0].Vel = math.NaN()
	if _, err := s.MarshalParams(""); err == nil {
		t.Fatal("a NaN must not be written")
	}
}
