// SPDX-License-Identifier: GPL-2.0-or-later

// Package musiclib turns note-sequence songs into seamlessly looping 16-bit
// mono WAV data. A Song is a tempo, a loop length in beats and parallel
// Tracks, each with one chip-style Instrument (pulse, triangle or LFSR noise)
// and a list of Notes. A Composition places rendered loops and one-shots on a
// timeline with crossfaded junctions.
//
// Loop seams are click-free because every note's envelope closes inside its
// own duration, tails crossing the loop end are written modulo the loop
// length, and each note starts from zero phase with a short attack.
//
// This is host-only build-time code and is never compiled into a cartridge.
package musiclib

// Common sample rates; every renderer entry point takes an explicit rate.
const (
	SampleRate22050 = 22050
	SampleRate44100 = 44100
)
