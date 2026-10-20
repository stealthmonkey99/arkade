// SPDX-License-Identifier: GPL-2.0-or-later

package musiclib

import (
	"math"
	"testing"
)

func TestMidiFreq(t *testing.T) {
	cases := []struct {
		midi float64
		want float64
	}{
		{69, 440},         // A4
		{57, 220},         // A3
		{81, 880},         // A5
		{60, 261.6255653}, // C4 (middle C)
		{64, 329.6275569}, // E4
		{40, 82.40688923}, // E2 (bass guitar low E)
		{76, 659.2551138}, // E5
	}
	for _, c := range cases {
		got := MidiFreq(c.midi)
		if math.Abs(got-c.want) > 1e-4 {
			t.Errorf("MidiFreq(%v) = %v, want %v", c.midi, got, c.want)
		}
	}
}

func TestFreqMidiRoundTrip(t *testing.T) {
	for m := 21; m <= 108; m++ {
		got := FreqMidi(MidiFreq(float64(m)))
		if math.Abs(got-float64(m)) > 1e-9 {
			t.Errorf("FreqMidi(MidiFreq(%d)) = %v", m, got)
		}
	}
}

func TestParseNote(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{"C4", 60}, {"A4", 69}, {"E5", 76}, {"B4", 71},
		{"C#4", 61}, {"Db4", 61}, {"Bb3", 58}, {"A#3", 58},
		{"C-1", 0}, {"G9", 127}, {"e5", 76}, {"F#2", 42},
	}
	for _, c := range cases {
		got, err := ParseNote(c.s)
		if err != nil {
			t.Errorf("ParseNote(%q) error: %v", c.s, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseNote(%q) = %d, want %d", c.s, got, c.want)
		}
	}
	for _, bad := range []string{"", "H4", "C", "C99", "4C"} {
		if _, err := ParseNote(bad); err == nil {
			t.Errorf("ParseNote(%q) should fail", bad)
		}
	}
}

func TestNoteNameRoundTrip(t *testing.T) {
	for m := 0; m <= 127; m++ {
		back, err := ParseNote(NoteName(m))
		if err != nil || back != m {
			t.Errorf("ParseNote(NoteName(%d)) = %d, %v", m, back, err)
		}
	}
}
