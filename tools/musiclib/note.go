// SPDX-License-Identifier: GPL-2.0-or-later

package musiclib

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// MidiFreq returns the equal-tempered frequency in Hz of MIDI note m
// (A4 = MIDI 69 = 440 Hz). Fractional m is allowed (detune).
func MidiFreq(m float64) float64 {
	return 440 * math.Pow(2, (m-69)/12)
}

// FreqMidi is the inverse of MidiFreq: the (fractional) MIDI note whose
// equal-tempered frequency is f Hz. f must be > 0.
func FreqMidi(f float64) float64 {
	return 69 + 12*math.Log2(f/440)
}

var noteIndex = map[byte]int{
	'C': 0, 'D': 2, 'E': 4, 'F': 5, 'G': 7, 'A': 9, 'B': 11,
}

// ParseNote converts scientific pitch notation ("C4", "F#3", "Bb5",
// "A#2") to a MIDI note number. Octave -1..9; C4 = MIDI 60, A4 = 69.
func ParseNote(s string) (int, error) {
	orig := s
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return 0, fmt.Errorf("musiclib: bad note %q", orig)
	}
	letter := s[0] &^ 0x20
	semis, ok := noteIndex[letter]
	if !ok {
		return 0, fmt.Errorf("musiclib: bad note letter in %q", orig)
	}
	rest := s[1:]
	for len(rest) > 0 {
		if rest[0] == '#' {
			semis++
			rest = rest[1:]
		} else if rest[0] == 'b' {
			semis--
			rest = rest[1:]
		} else {
			break
		}
	}
	oct, err := strconv.Atoi(rest)
	if err != nil || oct < -1 || oct > 9 {
		return 0, fmt.Errorf("musiclib: bad octave in %q", orig)
	}
	return (oct+1)*12 + semis, nil
}

// MustNote is ParseNote that panics on error.
func MustNote(s string) int {
	m, err := ParseNote(s)
	if err != nil {
		panic(err)
	}
	return m
}

// NoteName returns the scientific pitch name of MIDI note m using
// sharps ("C4", "F#3"). Inverse of ParseNote for sharp spellings.
func NoteName(m int) string {
	names := [12]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	oct := m/12 - 1
	return names[((m%12)+12)%12] + strconv.Itoa(oct)
}
